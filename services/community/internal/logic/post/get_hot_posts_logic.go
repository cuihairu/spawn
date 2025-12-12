// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package post

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetHotPostsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetHotPostsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetHotPostsLogic {
	return &GetHotPostsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetHotPostsLogic) GetHotPosts(req *types.GetPostsReq) (resp *types.PostsResp, err error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	posts := l.svcCtx.PostRepo.Hot(limit)
	return &types.PostsResp{Posts: posts, Total: int64(len(posts))}, nil
}
