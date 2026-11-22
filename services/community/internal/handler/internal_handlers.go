package handler

// GP-306 内部通知端点（S2S，不进网关）：业务侧（game-catalog 开服/版本、
// promotions 活动）扇出站内通知并经 realtime-hub 远程推送。复用既有通知
// 体系（notifications 表 + Realtime 发布口），不建新存储。
//
// 安全边界：X-Internal-Token 校验先于 body 解析；令牌未配置恒 403
//（默认拒绝，与 realtime-hub /internal/* 同款 fail-closed）。

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"
)

type internalNotificationReq struct {
	UserIds   []int64 `json:"user_ids"`
	Type      string  `json:"type"`
	TargetId  int64   `json:"target_id"`
	Content   string  `json:"content"`
	ActorId   int64   `json:"actor_id"`
	ActorName string  `json:"actor_name"`
}

type internalNotificationResp struct {
	Created int `json:"created"`
}

// InternalNotificationsHandler POST /internal/notifications：按 user_ids 扇出
// 站内通知（逐条落库，单条失败不阻断其余），成功后经 realtime-hub 远程推送
// 一帧（fire-and-forget，客户端以重拉未读数为准）。
func InternalNotificationsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svcCtx.Config.Internal.Token == "" || r.Header.Get("X-Internal-Token") != svcCtx.Config.Internal.Token {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"code":403,"message":"forbidden"}`))
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":400,"message":"invalid body"}`))
			return
		}
		var req internalNotificationReq
		if err := json.Unmarshal(raw, &req); err != nil || len(req.UserIds) == 0 ||
			req.Type == "" || req.Content == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":400,"message":"user_ids/type/content required"}`))
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		created := 0
		notified := make([]int64, 0, len(req.UserIds))
		for _, uid := range req.UserIds {
			if uid <= 0 {
				continue
			}
			notif := &types.Notification{
				UserId: uid, ActorId: req.ActorId, ActorName: req.ActorName,
				Type: req.Type, TargetId: req.TargetId, Content: req.Content,
				CreatedAt: now,
			}
			if err := svcCtx.NotificationRepo.Create(notif); err != nil {
				// 单条失败仅计数跳过（后续触发重放可补；不阻断扇出）
				continue
			}
			created++
			notified = append(notified, uid)
		}
		if created > 0 {
			// 远程推送一帧（GP-306 验收口径：只推实际落库的用户；失败仅日志）
			svcCtx.Realtime.Publish("notification", notified, map[string]any{
				"type": req.Type, "target_id": req.TargetId, "content": req.Content,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(internalNotificationResp{Created: created})
	}
}
