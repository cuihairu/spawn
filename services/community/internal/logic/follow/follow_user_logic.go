// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package follow

import (
	"context"
	"time"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type FollowUserLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewFollowUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FollowUserLogic {
	return &FollowUserLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *FollowUserLogic) FollowUser(req *types.FollowUserReq) (resp *types.CommonResp, err error) {
	userId, username, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.UserId <= 0 {
		return nil, httperr.BadRequest("user_id required")
	}
	if req.UserId == userId {
		return nil, httperr.BadRequest("cannot follow yourself")
	}

	// 重复关注幂等返回 false：仅首次关注产生通知；写失败不回滚关注本体，记日志即可。
	if followed := l.svcCtx.FollowRepo.FollowUser(userId, req.UserId); !followed {
		l.Infof("follow user %d by %d: already following, skip notification", req.UserId, userId)
	} else if notifyErr := l.svcCtx.NotificationRepo.Create(&types.Notification{
		UserId:    req.UserId,
		ActorId:   userId,
		ActorName: username,
		Type:      "follow_user",
		TargetId:  req.UserId,
		Content:   "关注了你",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); notifyErr != nil {
		l.Errorf("create follow notification for user %d: %v", req.UserId, notifyErr)
	}
	return &types.CommonResp{Code: 0, Message: "ok"}, nil
}
