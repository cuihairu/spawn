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

type GetFollowedPostsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetFollowedPostsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFollowedPostsLogic {
	return &GetFollowedPostsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetFollowedPosts 关注流：关注话题 ∪ 关注作者的帖子，时间倒序分页。
// 无任何关注 → 空列表 + total 0（空态由客户端呈现）。
func (l *GetFollowedPostsLogic) GetFollowedPosts(req *types.GetFollowedPostsReq) (resp *types.PostsResp, err error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	topicIds := l.svcCtx.FollowRepo.ListFollowingTopicIds(userId)
	authorIds := l.svcCtx.FollowRepo.ListFollowingUserIds(userId)

	posts, total := l.svcCtx.PostRepo.ListByFollow(topicIds, authorIds, req.Limit, req.Offset)
	if posts == nil {
		posts = []types.Post{}
	}

	return &types.PostsResp{Posts: posts, Total: total}, nil
}
