package logic

import (
	"context"
	"net/http"

	"github.com/tappi/tappi/services/content-service/internal/svc"
	"github.com/tappi/tappi/services/content-service/internal/types"
	"github.com/tappi/tappi/services/content-service/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListGuidesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListGuidesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListGuidesLogic {
	return &ListGuidesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListGuidesLogic) ListGuides(req *types.ListGuidesRequest) (resp *types.ListGuidesResponse, err error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 {
		req.PageSize = 20
	}

	filter := model.GuideFilter{
		GameId:   req.GameId,
		AuthorId: req.AuthorId,
		Tag:      req.Tag,
		Page:     req.Page,
		PageSize: req.PageSize,
	}

	guides, total := l.svcCtx.GuideRepository.List(filter)

	return &types.ListGuidesResponse{
		Code:    http.StatusOK,
		Message: "获取成功",
		Data:    modelGuidesToTypes(guides),
		Total:   total,
		Page:    req.Page,
	}, nil
}
