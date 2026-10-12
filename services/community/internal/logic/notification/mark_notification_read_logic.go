// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package notification

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type MarkNotificationReadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewMarkNotificationReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MarkNotificationReadLogic {
	return &MarkNotificationReadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// MarkNotificationRead 单条通知置已读（幂等；非本人/不存在 → 404 信封）。
// 成功后广播 notification.read 回执（带剩余未读数），其余端徽标即时同步。
func (l *MarkNotificationReadLogic) MarkNotificationRead(req *types.NotificationIdReq) (resp *types.CommonResp, err error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	if err := l.svcCtx.NotificationRepo.MarkRead(userId, req.Id); err != nil {
		return nil, err
	}

	if n, err := l.svcCtx.NotificationRepo.CountUnread(userId); err == nil {
		l.svcCtx.Realtime.Publish("notification.read", []int64{userId}, map[string]any{"unread_count": n})
	} else {
		l.Errorf("count unread after mark read: %v", err)
	}

	return &types.CommonResp{Code: 0, Message: "ok"}, nil
}
