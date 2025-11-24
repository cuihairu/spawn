package logic

import (
	"context"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateGameLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateGameLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateGameLogic {
	return &CreateGameLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateGameLogic) CreateGame(req *types.CreateGameRequest) (*types.GameDetailResponse, error) {
	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Description) == "" {
		return nil, newAPIError(http.StatusBadRequest, "标题和描述不能为空")
	}

	if len(req.Genres) == 0 {
		return nil, newAPIError(http.StatusBadRequest, "至少需要一个游戏类型")
	}

	if len(req.Platforms) == 0 {
		return nil, newAPIError(http.StatusBadRequest, "至少需要一个平台信息")
	}

	game := &model.Game{
		Title:         strings.TrimSpace(req.Title),
		Description:   strings.TrimSpace(req.Description),
		Genres:        req.Genres,
		Platforms:     req.Platforms,
		ReleaseDate:   req.ReleaseDate,
		Developer:     req.Developer,
		Publisher:     req.Publisher,
		Tags:          req.Tags,
		Score:         req.Score,
		CoverImage:    req.CoverImage,
		TrendingScore: 60,
	}

	entity, err := l.svcCtx.GameRepository.Create(game)
	if err != nil {
		l.Errorf("create game failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "创建失败")
	}

	return &types.GameDetailResponse{
		Game: toTypesGame(entity),
	}, nil
}
