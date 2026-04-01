package logic

import (
	"context"
	"net/http"

	"github.com/tappi/tappi/services/content-service/internal/svc"
	"github.com/tappi/tappi/services/content-service/internal/types"
	"github.com/tappi/tappi/services/content-service/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListCommentsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListCommentsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListCommentsLogic {
	return &ListCommentsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListCommentsLogic) ListComments(req *types.ListCommentsRequest) (resp *types.ListCommentsResponse, err error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 {
		req.PageSize = 20
	}

	// 草稿攻略的评论不可访问（避免通过评论接口探测草稿存在）
	if req.TargetType == "guide" && req.TargetId > 0 {
		guide, err := l.svcCtx.GuideRepository.Get(req.TargetId)
		if err != nil {
			return &types.ListCommentsResponse{
				Code:    http.StatusNotFound,
				Message: "攻略不存在",
			}, nil
		}
		if !guide.IsPublished {
			return &types.ListCommentsResponse{
				Code:    http.StatusNotFound,
				Message: "攻略不存在",
			}, nil
		}
	}

	filter := model.CommentFilter{
		TargetType: req.TargetType,
		TargetId:   req.TargetId,
		Page:       req.Page,
		PageSize:   req.PageSize,
	}

	comments, total := l.svcCtx.CommentRepository.List(filter)

	return &types.ListCommentsResponse{
		Code:    http.StatusOK,
		Message: "获取成功",
		Data:    modelCommentsToTypes(comments),
		Total:   total,
		Page:    req.Page,
	}, nil
}
