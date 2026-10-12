// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package message

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetDMUnreadCountLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetDMUnreadCountLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetDMUnreadCountLogic {
	return &GetDMUnreadCountLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetDMUnreadCount 当前用户未读私信总数（web 导航信封徽标用）。
func (l *GetDMUnreadCountLogic) GetDMUnreadCount() (resp *types.UnreadCountResp, err error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	count, err := l.svcCtx.MessageRepo.CountUnread(userId)
	if err != nil {
		return nil, err
	}
	return &types.UnreadCountResp{Count: count}, nil
}
