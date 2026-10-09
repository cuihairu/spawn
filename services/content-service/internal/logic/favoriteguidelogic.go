package logic

import (
	"context"
	"errors"
	"net/http"

	"github.com/tappi/tappi/services/content-service/internal/svc"
	"github.com/tappi/tappi/services/content-service/internal/types"
	"github.com/tappi/tappi/services/content-service/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type FavoriteGuideLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewFavoriteGuideLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FavoriteGuideLogic {
	return &FavoriteGuideLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *FavoriteGuideLogic) FavoriteGuide(req *types.FavoriteGuideRequest) (resp *types.FavoriteGuideResponse, err error) {
	userId, ok := l.ctx.Value("user_id").(int64)
	if !ok || userId <= 0 {
		return &types.FavoriteGuideResponse{
			Code:    http.StatusUnauthorized,
			Message: "请先登录",
		}, nil
	}

	guide, err := l.svcCtx.GuideRepository.Get(req.Id)
	if err != nil {
		if errors.Is(err, model.ErrGuideNotFound) {
			return &types.FavoriteGuideResponse{
				Code:    http.StatusNotFound,
				Message: "攻略不存在",
			}, nil
		}
		l.Logger.Errorf("get guide failed: %v", err)
		return &types.FavoriteGuideResponse{
			Code:    http.StatusInternalServerError,
			Message: "收藏失败",
		}, nil
	}
	if !guide.IsPublished {
		return &types.FavoriteGuideResponse{
			Code:    http.StatusNotFound,
			Message: "攻略不存在",
		}, nil
	}

	if err := l.svcCtx.FavoriteRepository.Add(userId, req.Id); err != nil {
		l.Logger.Errorf("add favorite failed: %v", err)
		return &types.FavoriteGuideResponse{
			Code:    http.StatusInternalServerError,
			Message: "收藏失败",
		}, nil
	}

	count, err := l.svcCtx.FavoriteRepository.CountByGuide(req.Id)
	if err != nil {
		l.Logger.Errorf("count favorites failed: %v", err)
		count = 0
	}

	return &types.FavoriteGuideResponse{
		Code:      http.StatusOK,
		Message:   "已收藏",
		Favorited: true,
		Count:     count,
	}, nil
}
