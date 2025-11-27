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

type LikeCommentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLikeCommentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LikeCommentLogic {
	return &LikeCommentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LikeCommentLogic) LikeComment(req *types.LikeCommentRequest) (resp *types.LikeCommentResponse, err error) {
	likes, err := l.svcCtx.CommentRepository.Like(req.Id)
	if err != nil {
		if errors.Is(err, model.ErrCommentNotFound) {
			return &types.LikeCommentResponse{
				Code:    http.StatusNotFound,
				Message: "评论不存在",
			}, nil
		}
		l.Logger.Errorf("like comment failed: %v", err)
		return &types.LikeCommentResponse{
			Code:    http.StatusInternalServerError,
			Message: "点赞失败",
		}, nil
	}

	return &types.LikeCommentResponse{
		Code:    http.StatusOK,
		Message: "点赞成功",
		Likes:   likes,
	}, nil
}
