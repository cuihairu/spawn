package handler

// GP-306 开服预约 handler：玩家预约/取消/我的预约（平台 JWT，player 即可）+
// S2S 预约人群查询（X-Internal-Token 守卫，供 promotions 活动通知拉受众）。

import (
	"encoding/json"
	"net/http"

	"github.com/tappi/tappi/services/game-catalog/internal/logic"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"

	"github.com/zeromicro/go-zero/rest/httpx"
)

func ReserveServerHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ServerPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewReservationLogic(r.Context(), svcCtx).Reserve(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func CancelReservationHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ServerPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		if err := logic.NewReservationLogic(r.Context(), svcCtx).Cancel(&req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]string{"message": "ok"})
	}
}

func MyReservationsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := logic.NewReservationLogic(r.Context(), svcCtx).MyReservations()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// InternalReservationUsersHandler GET /internal/games/:id/reservation-users：
// X-Internal-Token 校验先于 body 解析；令牌未配置恒 403（fail-closed）。
func InternalReservationUsersHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svcCtx.Config.Internal.Token == "" || r.Header.Get("X-Internal-Token") != svcCtx.Config.Internal.Token {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"code":403,"message":"forbidden"}`))
			return
		}
		var req types.GetGameDetailRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewReservationLogic(r.Context(), svcCtx).ListReservationUsers(req.Id)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
