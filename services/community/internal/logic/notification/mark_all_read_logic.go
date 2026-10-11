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

type MarkAllReadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewMarkAllReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MarkAllReadLogic {
	return &MarkAllReadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// MarkAllRead 当前用户全部未读通知置为已读（幂等，无未读时同样返回 ok）。
func (l *MarkAllReadLogic) MarkAllRead() (resp *types.CommonResp, err error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	if err := l.svcCtx.NotificationRepo.MarkAllRead(userId); err != nil {
		return nil, err
	}

	// 已读回执广播：本人全部在线连接（其他端）收到即重拉未读数，徽标
	// 多端即时归零；帧只是信号，数值以服务端为准（重播幂等无害）。
	l.svcCtx.Realtime.Publish("notification.read", []int64{userId}, map[string]any{"unread_count": 0})

	return &types.CommonResp{Code: 0, Message: "ok"}, nil
}
