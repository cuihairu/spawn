// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package post

import (
	"context"
	"strings"

	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetHotPostsLogic struct {
	logx.Logger
	ctx        context.Context
	svcCtx     *svc.ServiceContext
	authHeader string
}

// NewGetHotPostsLogic 热榜是公开端点，但登录用户的令牌可带来个性化提权
// （关注话题/作者 ×1.5）：authHeader 传原始 Authorization 头，缺失或无效
// 一律按匿名热榜处理，不报错。
func NewGetHotPostsLogic(ctx context.Context, svcCtx *svc.ServiceContext, authHeader string) *GetHotPostsLogic {
	return &GetHotPostsLogic{
		Logger:     logx.WithContext(ctx),
		ctx:        ctx,
		svcCtx:     svcCtx,
		authHeader: authHeader,
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

	// 可选身份：令牌有效且能取到关注集合 → 个性化热榜；否则匿名热榜。
	if posts, personalized := l.hotForCaller(limit); personalized {
		return &types.PostsResp{Posts: posts, Total: int64(len(posts))}, nil
	}
	posts := l.svcCtx.PostRepo.Hot(limit)
	return &types.PostsResp{Posts: posts, Total: int64(len(posts))}, nil
}

// hotForCaller 解析可选令牌并取关注集合；未登录/令牌无效/无关注均回落
// 匿名热榜（personalized=false，省两次关注表查询）。
func (l *GetHotPostsLogic) hotForCaller(limit int64) ([]types.Post, bool) {
	token := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l.authHeader), "Bearer"))
	if token == "" || l.svcCtx.Jwt == nil {
		return nil, false
	}
	claims, err := l.svcCtx.Jwt.ParseToken(token)
	if err != nil {
		return nil, false
	}
	topicIds := l.svcCtx.FollowRepo.ListFollowingTopicIds(claims.UserId)
	userIds := l.svcCtx.FollowRepo.ListFollowingUserIds(claims.UserId)
	if len(topicIds) == 0 && len(userIds) == 0 {
		return nil, false
	}
	return l.svcCtx.PostRepo.HotForUser(topicIds, userIds, limit), true
}
