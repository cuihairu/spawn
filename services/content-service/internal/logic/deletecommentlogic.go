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

type DeleteCommentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteCommentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteCommentLogic {
	return &DeleteCommentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteCommentLogic) DeleteComment(req *types.DeleteCommentRequest) (resp *types.DeleteCommentResponse, err error) {
	userId, ok := l.ctx.Value("user_id").(int64)
	if !ok || userId == 0 {
		return &types.DeleteCommentResponse{
			Code:    http.StatusUnauthorized,
			Message: "用户认证失败",
		}, nil
	}

	comment, err := l.svcCtx.CommentRepository.Get(req.Id)
	if err != nil {
		if errors.Is(err, model.ErrCommentNotFound) {
			return &types.DeleteCommentResponse{
				Code:    http.StatusNotFound,
				Message: "评论不存在",
			}, nil
		}
		l.Logger.Errorf("get comment failed: %v", err)
		return &types.DeleteCommentResponse{
			Code:    http.StatusInternalServerError,
			Message: "删除失败",
		}, nil
	}
	if comment.UserId != userId {
		return &types.DeleteCommentResponse{
			Code:    http.StatusForbidden,
			Message: "无权限操作",
		}, nil
	}

	err = l.svcCtx.CommentRepository.Delete(req.Id)
	if err != nil {
		if errors.Is(err, model.ErrCommentNotFound) {
			return &types.DeleteCommentResponse{
				Code:    http.StatusNotFound,
				Message: "评论不存在",
			}, nil
		}
		l.Logger.Errorf("delete comment failed: %v", err)
		return &types.DeleteCommentResponse{
			Code:    http.StatusInternalServerError,
			Message: "删除失败",
		}, nil
	}

	return &types.DeleteCommentResponse{
		Code:    http.StatusOK,
		Message: "删除成功",
	}, nil
}
