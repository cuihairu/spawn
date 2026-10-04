// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package post

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetLikedPostsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetLikedPostsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetLikedPostsLogic {
	return &GetLikedPostsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetLikedPosts 我的点赞：点赞过的帖子按点赞时间倒序分页。
// 无点赞 → 空列表 + total 0（空态由客户端呈现）。
func (l *GetLikedPostsLogic) GetLikedPosts(req *types.GetLikedPostsReq) (resp *types.PostsResp, err error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	posts, total := l.svcCtx.PostRepo.ListLikedPosts(userId, req.Limit, req.Offset)
	if posts == nil {
		posts = []types.Post{}
	}

	return &types.PostsResp{Posts: posts, Total: total}, nil
}
