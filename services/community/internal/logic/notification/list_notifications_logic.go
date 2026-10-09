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

type ListNotificationsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListNotificationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListNotificationsLogic {
	return &ListNotificationsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListNotifications 我的站内通知（登录，收件人恒为当前用户）：id 倒序分页，
// 空结果规整为空切片非 nil。分页钳制与 GetPosts 同款。
func (l *ListNotificationsLogic) ListNotifications(req *types.ListNotificationsReq) (resp *types.NotificationsResp, err error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req == nil {
		req = &types.ListNotificationsReq{}
	}

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

	list, total, err := l.svcCtx.NotificationRepo.ListByUser(userId, limit, offset)
	if err != nil {
		return nil, err
	}

	return &types.NotificationsResp{Notifications: list, Total: total}, nil
}
