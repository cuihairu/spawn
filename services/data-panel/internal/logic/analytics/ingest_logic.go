package analytics

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/tappi/tappi/services/data-panel/internal/httperr"
	"github.com/tappi/tappi/services/data-panel/internal/logic/common"
	"github.com/tappi/tappi/services/data-panel/internal/model"
	"github.com/tappi/tappi/services/data-panel/internal/svc"
	"github.com/tappi/tappi/services/data-panel/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

const ingestBatchLimit = 500

type IngestLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewIngestLogic(ctx context.Context, svcCtx *svc.ServiceContext) *IngestLogic {
	return &IngestLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// CheckInternalToken S2S 摄入鉴权：X-Internal-Token 与配置严格比对；
// 配置侧令牌为空时整体拒绝（fail-closed，不留默认口令）。
func CheckInternalToken(svcCtx *svc.ServiceContext, presented string) error {
	want := svcCtx.Config.Internal.Token
	if want == "" {
		return httperr.Forbidden("internal ingest disabled")
	}
	if presented == "" || presented != want {
		return httperr.Unauthorized("invalid internal token")
	}
	return nil
}

// Ingest 批量摄入平台事实事件（event_id 幂等）。金额/订单等事实字段由
// 产生方负责，摄入侧仅做类型白名单与字段卫生校验（负数金额拒绝、
// occurred_at 必须可解析）。
func (l *IngestLogic) Ingest(req *types.IngestAnalyticsReq) (*types.IngestAnalyticsResp, error) {
	if req == nil || len(req.Events) == 0 {
		return nil, httperr.BadRequest("events required")
	}
	if len(req.Events) > ingestBatchLimit {
		return nil, httperr.BadRequest("too many events in one batch")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	events := make([]*model.AnalyticsEvent, 0, len(req.Events))
	for i := range req.Events {
		in := &req.Events[i]
		eventId := strings.TrimSpace(in.EventId)
		if eventId == "" {
			return nil, httperr.BadRequest("event_id is required")
		}
		if len(eventId) > 64 {
			return nil, httperr.BadRequest("event_id too long")
		}
		if !model.ValidEventType(in.EventType) {
			return nil, httperr.BadRequest("unknown event_type: " + in.EventType)
		}
		if in.UserId < 0 || in.ActivityId < 0 || in.AmountCents < 0 {
			return nil, httperr.BadRequest("user_id/activity_id/amount_cents must be non-negative")
		}
		if in.OccurredAt != "" {
			if _, err := time.Parse(time.RFC3339, in.OccurredAt); err != nil {
				return nil, httperr.BadRequest("occurred_at must be RFC3339")
			}
		}
		if len(in.GameId) > 64 {
			return nil, httperr.BadRequest("game_id too long")
		}
		if len(in.OrderId) > 64 {
			return nil, httperr.BadRequest("order_id too long")
		}
		events = append(events, &model.AnalyticsEvent{
			EventId:     eventId,
			EventType:   in.EventType,
			UserId:      in.UserId,
			GameId:      in.GameId,
			ActivityId:  in.ActivityId,
			OrderId:     in.OrderId,
			AmountCents: in.AmountCents,
			OccurredAt:  in.OccurredAt,
		})
	}

	accepted, err := l.svcCtx.AnalyticsSvc.IngestEvents(events, now)
	if err != nil {
		return nil, err
	}
	l.Infof("analytics ingest: batch=%d accepted=%d", len(req.Events), accepted)
	return &types.IngestAnalyticsResp{Accepted: int64(accepted)}, nil
}

// adminUserStr 提取登录用户 ID 的字符串形态（权限名单比对用）。
func adminUserStr(ctx context.Context) (string, error) {
	id, _, err := common.UserFromContext(ctx)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(id, 10), nil
}

// requireAnalyticsAdmin 查询域统一权限闸：不在 AdminUserIds 名单即 403。
func requireAnalyticsAdmin(svcCtx *svc.ServiceContext, ctx context.Context) error {
	user, err := adminUserStr(ctx)
	if err != nil {
		return err
	}
	if !svcCtx.IsAnalyticsAdmin(user) {
		return httperr.Forbidden("analytics admin only")
	}
	return nil
}
