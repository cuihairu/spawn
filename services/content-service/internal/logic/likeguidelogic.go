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

type LikeGuideLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLikeGuideLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LikeGuideLogic {
	return &LikeGuideLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LikeGuideLogic) LikeGuide(req *types.LikeGuideRequest) (resp *types.LikeGuideResponse, err error) {
	guide, err := l.svcCtx.GuideRepository.Get(req.Id)
	if err != nil {
		if errors.Is(err, model.ErrGuideNotFound) {
			return &types.LikeGuideResponse{
				Code:    http.StatusNotFound,
				Message: "攻略不存在",
			}, nil
		}
		l.Logger.Errorf("get guide failed: %v", err)
		return &types.LikeGuideResponse{
			Code:    http.StatusInternalServerError,
			Message: "点赞失败",
		}, nil
	}
	if !guide.IsPublished {
		return &types.LikeGuideResponse{
			Code:    http.StatusNotFound,
			Message: "攻略不存在",
		}, nil
	}

	likes, err := l.svcCtx.GuideRepository.Like(req.Id)
	if err != nil {
		if errors.Is(err, model.ErrGuideNotFound) {
			return &types.LikeGuideResponse{
				Code:    http.StatusNotFound,
				Message: "攻略不存在",
			}, nil
		}
		l.Logger.Errorf("like guide failed: %v", err)
		return &types.LikeGuideResponse{
			Code:    http.StatusInternalServerError,
			Message: "点赞失败",
		}, nil
	}

	return &types.LikeGuideResponse{
		Code:    http.StatusOK,
		Message: "点赞成功",
		Likes:   likes,
	}, nil
}
