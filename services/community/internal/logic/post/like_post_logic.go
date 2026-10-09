// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package post

import (
	"context"
	"time"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type LikePostLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLikePostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LikePostLogic {
	return &LikePostLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LikePostLogic) LikePost(req *types.LikePostReq) (resp *types.CommonResp, err error) {
	if req == nil || req.Id <= 0 {
		return nil, httperr.BadRequest("id required")
	}
	userId, username, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	post, err := l.svcCtx.PostRepo.Like(req.Id, userId)
	if err != nil {
		return nil, err
	}

	// 通知帖子作者（自己点赞自己不通知）；唯一键去重，重复点赞不刷屏；
	// 写失败不回滚点赞本体（与计数自增同款契约），记日志即可。
	if post.AuthorId != userId {
		if err := l.svcCtx.NotificationRepo.Create(&types.Notification{
			UserId:    post.AuthorId,
			ActorId:   userId,
			ActorName: username,
			Type:      "like_post",
			TargetId:  post.Id,
			Content:   "点赞了你的帖子《" + truncateRunes(post.Title, 30) + "》",
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}); err != nil {
			l.Errorf("create like notification for post %d: %v", post.Id, err)
		}
	}

	return &types.CommonResp{Code: 0, Message: "ok"}, nil
}

// truncateRunes 按 rune 截断字符串（通知摘要防超长；中文按字符不按字节）。
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
