// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package message

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// dmMaxContent 私信正文上限（与表列 VARCHAR(2000) 对齐，按 rune 计）。
const dmMaxContent = 2000

type SendDirectMessageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSendDirectMessageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SendDirectMessageLogic {
	return &SendDirectMessageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// SendDirectMessage 发私信：落库成功后向收件人推 `message` 实时帧
// （在线即时送达，离线由 hub 推送桥接缝兜底）。收件人存在性不校验
// （与关注同款语义，本地部署无用户表反查）；自我发送与空/超长正文 400。
func (l *SendDirectMessageLogic) SendDirectMessage(req *types.SendDirectMessageReq) (resp *types.SendMessageResp, err error) {
	userId, username, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	content := strings.TrimSpace(req.Content)
	switch {
	case req.RecipientId <= 0:
		return nil, httperr.BadRequest("recipient id is required")
	case req.RecipientId == userId:
		return nil, httperr.BadRequest("cannot message yourself")
	case content == "":
		return nil, httperr.BadRequest("content is required")
	case utf8.RuneCountInString(content) > dmMaxContent:
		return nil, httperr.BadRequest("content too long (max 2000 chars)")
	}

	msg := &types.DirectMessage{
		SenderId:    userId,
		RecipientId: req.RecipientId,
		Content:     content,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := l.svcCtx.MessageRepo.Send(msg); err != nil {
		return nil, err
	}

	l.svcCtx.Realtime.Publish("message", []int64{req.RecipientId}, map[string]any{
		"message_id": msg.Id,
		"from_id":    userId,
		"from_name":  username,
		"content":    msg.Content,
		"created_at": msg.CreatedAt,
	})

	return &types.SendMessageResp{Message: msg}, nil
}
