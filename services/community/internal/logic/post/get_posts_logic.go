// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package post

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/model"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPostsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetPostsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostsLogic {
	return &GetPostsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetPostsLogic) GetPosts(req *types.GetPostsReq) (resp *types.PostsResp, err error) {
	// defaults are already defined in api, but keep defensive guards here.
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	posts, total := l.svcCtx.PostRepo.List(model.PostListFilter{
		TopicId:  req.TopicId,
		AuthorId: req.AuthorId,
		Type:     req.Type,
		Status:   req.Status,
		IsHot:    req.IsHot,
		Limit:    limit,
		Offset:   offset,
	})

	return &types.PostsResp{Posts: posts, Total: total}, nil
}
