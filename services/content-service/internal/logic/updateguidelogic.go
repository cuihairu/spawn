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

type UpdateGuideLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateGuideLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateGuideLogic {
	return &UpdateGuideLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateGuideLogic) UpdateGuide(req *types.UpdateGuideRequest) (resp *types.UpdateGuideResponse, err error) {
	updates := make(map[string]interface{})

	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.Content != "" {
		updates["content"] = req.Content
	}
	if req.Summary != "" {
		updates["summary"] = req.Summary
	}
	if req.CoverImage != "" {
		updates["cover_image"] = req.CoverImage
	}
	if len(req.Tags) > 0 {
		updates["tags"] = req.Tags
	}

	updated, err := l.svcCtx.GuideRepository.Update(req.Id, updates)
	if err != nil {
		if errors.Is(err, model.ErrGuideNotFound) {
			return &types.UpdateGuideResponse{
				Code:    http.StatusNotFound,
				Message: "攻略不存在",
			}, nil
		}
		l.Logger.Errorf("update guide failed: %v", err)
		return &types.UpdateGuideResponse{
			Code:    http.StatusInternalServerError,
			Message: "更新失败",
		}, nil
	}

	return &types.UpdateGuideResponse{
		Code:    http.StatusOK,
		Message: "更新成功",
		Data:    modelGuideToType(updated),
	}, nil
}
