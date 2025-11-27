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

type PublishGuideLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPublishGuideLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PublishGuideLogic {
	return &PublishGuideLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PublishGuideLogic) PublishGuide(req *types.PublishGuideRequest) (resp *types.PublishGuideResponse, err error) {
	err = l.svcCtx.GuideRepository.Publish(req.Id)
	if err != nil {
		if errors.Is(err, model.ErrGuideNotFound) {
			return &types.PublishGuideResponse{
				Code:    http.StatusNotFound,
				Message: "攻略不存在",
			}, nil
		}
		l.Logger.Errorf("publish guide failed: %v", err)
		return &types.PublishGuideResponse{
			Code:    http.StatusInternalServerError,
			Message: "发布失败",
		}, nil
	}

	return &types.PublishGuideResponse{
		Code:    http.StatusOK,
		Message: "发布成功",
	}, nil
}
