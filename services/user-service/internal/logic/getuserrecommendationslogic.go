package logic

import (
	"context"
	"fmt"
	"strings"

	"github.com/tappi/tappi/services/user-service/internal/integration"
	"github.com/tappi/tappi/services/user-service/internal/svc"
	"github.com/tappi/tappi/services/user-service/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserRecommendationsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetUserRecommendationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserRecommendationsLogic {
	return &GetUserRecommendationsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetUserRecommendationsLogic) GetUserRecommendations(req *types.GetUserRecommendationsRequest) (*types.ApiResponse, error) {
	user, err := l.svcCtx.UserModel.FindOne(req.Id)
	if err != nil {
		l.Errorf("user %d not found: %v", req.Id, err)
		return &types.ApiResponse{
			Code:    404,
			Message: "用户不存在",
		}, nil
	}

	genres := parseGenres(req.Genres)
	params := integration.RecommendationParams{
		UserID: fmt.Sprintf("%d", req.Id),
		Genres: genres,
		Limit:  clampInt(int(req.Limit), 1, 20),
	}

	recommendations, err := l.svcCtx.GameCatalogClient.GetRecommendations(l.ctx, params)
	if err != nil {
		l.Errorf("fetch recommendations failed: %v", err)
		return &types.ApiResponse{
			Code:    502,
			Message: "获取推荐列表失败，请稍后再试",
		}, nil
	}

	nickname := user.Nickname.String
	if !user.Nickname.Valid {
		nickname = user.Username
	}

	result := make([]types.GameRecommendation, 0, len(recommendations))
	for _, rec := range recommendations {
		result = append(result, types.GameRecommendation{
			Id:         rec.Id,
			Title:      rec.Title,
			CoverImage: rec.CoverImage,
			Genres:     rec.Genres,
			Platforms:  rec.Platforms,
			Score:      rec.Score,
			Tags:       rec.Tags,
		})
	}

	return &types.ApiResponse{
		Code:    200,
		Message: "获取推荐成功",
		Data: map[string]interface{}{
			"user_id":         user.Id,
			"user_nickname":   nickname,
			"recommendations": result,
		},
	}, nil
}

func parseGenres(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var genres []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		genres = append(genres, part)
	}
	return genres
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
