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

type ListDMConversationsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListDMConversationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListDMConversationsLogic {
	return &ListDMConversationsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListDMConversations 我的会话列表（对端 + 末条消息 + 未读数，按最后活跃倒序）。
func (l *ListDMConversationsLogic) ListDMConversations() (resp *types.DMConversationsResp, err error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	list, err := l.svcCtx.MessageRepo.Conversations(userId)
	if err != nil {
		return nil, err
	}
	return &types.DMConversationsResp{Conversations: list}, nil
}
