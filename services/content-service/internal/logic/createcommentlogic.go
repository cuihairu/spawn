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

type CreateCommentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateCommentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateCommentLogic {
	return &CreateCommentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateCommentLogic) CreateComment(req *types.CreateCommentRequest) (resp *types.CreateCommentResponse, err error) {
	// 从上下文中获取当前用户ID和名称
	userId, ok := l.ctx.Value("user_id").(int64)
	if !ok || userId == 0 {
		l.Logger.Error("无法从上下文获取用户ID")
		return &types.CreateCommentResponse{
			Code:    http.StatusUnauthorized,
			Message: "用户认证失败",
		}, nil
	}

	// 草稿攻略不允许评论（避免通过评论接口探测草稿存在）
	if req.TargetType == "guide" && req.TargetId > 0 {
		guide, err := l.svcCtx.GuideRepository.Get(req.TargetId)
		if err != nil {
			if errors.Is(err, model.ErrGuideNotFound) {
				return &types.CreateCommentResponse{
					Code:    http.StatusNotFound,
					Message: "攻略不存在",
				}, nil
			}
			l.Logger.Errorf("get guide failed: %v", err)
			return &types.CreateCommentResponse{
				Code:    http.StatusInternalServerError,
				Message: "创建评论失败",
			}, nil
		}
		if !guide.IsPublished {
			return &types.CreateCommentResponse{
				Code:    http.StatusNotFound,
				Message: "攻略不存在",
			}, nil
		}
	}

	username, ok := l.ctx.Value("username").(string)
	if !ok || username == "" {
		username = "未知用户"
	}

	comment := &model.Comment{
		TargetType: req.TargetType,
		TargetId:   req.TargetId,
		UserId:     userId,
		UserName:   username,
		Content:    req.Content,
		ParentId:   req.ParentId,
		ReplyToId:  req.ReplyToId,
	}

	created, err := l.svcCtx.CommentRepository.Create(comment)
	if err != nil {
		l.Logger.Errorf("create comment failed: %v", err)
		return &types.CreateCommentResponse{
			Code:    http.StatusInternalServerError,
			Message: "创建评论失败",
		}, nil
	}

	return &types.CreateCommentResponse{
		Code:    http.StatusOK,
		Message: "评论成功",
		Data:    modelCommentToType(created),
	}, nil
}
