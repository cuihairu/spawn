package handler

// GP-403 单层渠道推广 handler（F22）：渠道登记/变更历史（管理面）、推广链接
// 落地解析与渠道包下载登记（公开/玩家面）、S2S 归因解析（user-service 注册
// 时携带 click_id 回调，X-Internal-Token 守卫，不进网关）。

import (
	"encoding/json"
	"net/http"

	"github.com/tappi/tappi/services/game-catalog/internal/logic"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// ---- 管理面：渠道登记与变更历史 ----

func CreatePromoChannelHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CreatePromoChannelRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewPromoLogic(r.Context(), svcCtx)
		resp, err := l.CreatePromoChannel(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

func ListPromoChannelsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.PromoChannelListRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewPromoLogic(r.Context(), svcCtx)
		items, err := l.ListPromoChannels(req.Limit, req.Offset)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, types.PromoChannelListResponse{
			Items: items, Total: len(items),
		})
	}
}

func UpdatePromoChannelHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.UpdatePromoChannelRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewPromoLogic(r.Context(), svcCtx)
		resp, err := l.UpdatePromoChannel(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

func SetPromoChannelStatusHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.SetPromoChannelStatusRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewPromoLogic(r.Context(), svcCtx)
		resp, err := l.SetPromoChannelStatus(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// PromoChannelStatsHandler GET /admin/promo/channels/:code/stats：渠道包/
// 渠道效果计数（点击/下载/归因人数）。
func PromoChannelStatsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ChannelStatsRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewPromoLogic(r.Context(), svcCtx)
		resp, err := l.ChannelStats(req.Code, req.From, req.To)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// ---- 公开/玩家面：推广链接落地与渠道包下载 ----

// ResolvePromoLinkHandler GET /promo/resolve?channel=&click_id=&game_id=：
// 推广链接落地即记一条点击（归因窗口起算点），公开免登录（落地页可能在
// 登录前打开）；无效/停用渠道 404。
func ResolvePromoLinkHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ResolvePromoLinkRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewPromoLogic(r.Context(), svcCtx)
		resp, err := l.ResolvePromoLink(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// PromoDownloadClickHandler POST /promo/download-click?channel=&game_id=：
// 渠道包下载登记（渠道包追踪计数）。渠道无效时不阻断，仅不计计数。
func PromoDownloadClickHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.PromoDownloadClickRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		logic.NewPromoLogic(r.Context(), svcCtx).RecordDownloadClick(&req)
		httpx.OkJsonCtx(r.Context(), w, types.PromoDownloadClickResponse{})
	}
}

// ---- S2S：归因解析（user-service 注册时回调） ----

// InternalResolveClickHandler POST /internal/promo/clicks/:clickId/resolve：
// X-Internal-Token 校验先于 body 解析；令牌未配置恒 403（fail-closed）。
// 返回该点击的渠道与是否在归因窗口内（窗口判定在 model 层）。
func InternalResolveClickHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svcCtx.Config.Internal.Token == "" ||
			r.Header.Get("X-Internal-Token") != svcCtx.Config.Internal.Token {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"code":403,"message":"forbidden"}`))
			return
		}
		var req types.ResolveClickRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := logic.NewPromoLogic(r.Context(), svcCtx)
		resp, err := l.ResolveClick(req.ClickId, req.RegisteredAt, req.UserID)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
