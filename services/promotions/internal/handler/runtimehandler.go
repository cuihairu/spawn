package handler

// GP-301 活动快照运行时消费（玩家侧/游戏服侧读取）：配置域此前只进不出，
// 运营发布后无读取面。玩家侧公开读只出全体人群的展示摘要（预算不下发）；
// 游戏服侧 S2S 返回完整快照（含人群/触发明细，按调用方玩家上下文消费）。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tappi/tappi/services/promotions/internal/logic"
	"github.com/tappi/tappi/services/promotions/internal/svc"
	"github.com/tappi/tappi/services/promotions/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// ListPlayerActivitiesHandler 玩家侧活动列表（公开读）：已发布 + 当前
// 窗口内 + 人群为全体；定向活动不出现（fail-closed 不泄露触达面）。
func ListPlayerActivitiesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.RuntimeGamePathRequest
		if err := httpx.ParsePath(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		items := logic.ListRuntimeActivities(svcCtx, logx.WithContext(r.Context()), req.GameId, time.Now())
		httpx.OkJsonCtx(r.Context(), w, types.RuntimeActivityListResponse{Items: items})
	}
}

// InternalListActivitiesHandler 游戏服侧活动快照（S2S 专用不进网关）：
// X-Internal-Token 先于解析严格比对，未配置恒 403（fail-closed）。
func InternalListActivitiesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Internal-Token")
		if svcCtx.Config.Internal.Token == "" || token != svcCtx.Config.Internal.Token {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"internal activity read disabled"}`))
			return
		}
		var req types.RuntimeGamePathRequest
		if err := httpx.ParsePath(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		items := logic.ListRuntimeSnapshots(svcCtx, logx.WithContext(r.Context()), req.GameId, time.Now())
		httpx.OkJsonCtx(r.Context(), w, types.RuntimeSnapshotListResponse{Items: items})
	}
}

// InternalPlayerActivityHandler GP-303 回流判定源上报（S2S 专用不进网关）：
// courier-adapter 游戏会话签发成功后上报账号×游戏活跃事实。X-Internal-Token
// 先于解析严格比对，未配置恒 403（fail-closed，与活动快照同款）。
func InternalPlayerActivityHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Internal-Token")
		if svcCtx.Config.Internal.Token == "" || token != svcCtx.Config.Internal.Token {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"internal player activity disabled"}`))
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, fmt.Errorf("读取活跃上报失败: %w", err))
			return
		}
		var req types.InternalPlayerActivityRequest
		if err := json.Unmarshal(body, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, fmt.Errorf("活跃上报解析失败"))
			return
		}
		if err := logic.RecordPlayerActivity(svcCtx, logx.WithContext(r.Context()), &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]any{"recorded": true})
	}
}
