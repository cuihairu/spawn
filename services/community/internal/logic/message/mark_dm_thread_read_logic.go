// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package message

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type MarkDMThreadReadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewMarkDMThreadReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MarkDMThreadReadLogic {
	return &MarkDMThreadReadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// MarkDMThreadRead 将会话置已读（幂等）；成功后向本人全部连接广播
// `message.read` 回执（带剩余未读数），多端徽标即时同步。
func (l *MarkDMThreadReadLogic) MarkDMThreadRead(req *types.DMThreadReadReq) (resp *types.UnreadCountResp, err error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req.PeerId <= 0 {
		return nil, httperr.BadRequest("peer id is required")
	}

	if _, err := l.svcCtx.MessageRepo.MarkPeerRead(userId, req.PeerId); err != nil {
		return nil, err
	}

	// 未读统计失败必须显式报错：静默回落 0 会让多端徽标被清零。
	remaining, err := l.svcCtx.MessageRepo.CountUnread(userId)
	if err != nil {
		l.Errorf("count unread after mark dm read: %v", err)
		return nil, httperr.Internal("count unread failed")
	}

	l.svcCtx.Realtime.Publish("message.read", []int64{userId}, map[string]any{
		"unread_count": remaining,
		"peer_id":      req.PeerId,
	})

	return &types.UnreadCountResp{Count: remaining}, nil
}
