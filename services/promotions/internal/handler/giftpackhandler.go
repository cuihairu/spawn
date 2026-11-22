package handler

// GP-304 礼包 handler：管理面（建/改/发布/停用/回草稿/列表/详情/码导入/
// 码列表/领取记录）、玩家侧（展示/领取/我的领取）、S2S 核销回调。
// 管理面角色经 game-catalog /me 权威判定（requireAdminRole）；玩家侧仅平台
// JWT（AuthMiddleware）；S2S 验签基于原始请求体先于解析。

import (
	"io"
	"net/http"

	"github.com/tappi/tappi/services/promotions/internal/logic"
	"github.com/tappi/tappi/services/promotions/internal/svc"
	"github.com/tappi/tappi/services/promotions/internal/types"

	"github.com/zeromicro/go-zero/rest/httpx"
)

func CreateGiftPackHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CreateGiftPackRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewGiftPackLogic(r.Context(), svcCtx).Create(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func UpdateGiftPackHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.UpdateGiftPackRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewGiftPackLogic(r.Context(), svcCtx).Update(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func PublishGiftPackHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GiftPackPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewGiftPackLogic(r.Context(), svcCtx).Publish(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func DisableGiftPackHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GiftPackPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewGiftPackLogic(r.Context(), svcCtx).Disable(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ReviseGiftPackHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GiftPackPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewGiftPackLogic(r.Context(), svcCtx).Revise(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ListGiftPacksHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GiftPackListRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewGiftPackLogic(r.Context(), svcCtx).List(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func GetGiftPackHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GiftPackPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewGiftPackLogic(r.Context(), svcCtx).Get(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ImportGiftPackCodesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ImportCodesRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewGiftPackLogic(r.Context(), svcCtx).ImportCodes(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ListGiftPackCodesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CodeListRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewGiftPackLogic(r.Context(), svcCtx).ListCodes(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ListGiftClaimsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ClaimListAdminRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewGiftPackLogic(r.Context(), svcCtx).ListClaims(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// ---- 玩家侧 ----

func ListPlayerGiftPacksHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID := r.URL.Query().Get("game_id")
		var serverID int64
		if s := r.URL.Query().Get("server_id"); s != "" {
			serverID = parseInt64(s)
		}
		resp, err := logic.NewGiftPackLogic(r.Context(), svcCtx).PlayerList(gameID, serverID, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func GetPlayerGiftPackHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GiftPackPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewGiftPackLogic(r.Context(), svcCtx).PlayerGet(req.Id, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ClaimGiftPackHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ClaimGiftPackRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewGiftPackLogic(r.Context(), svcCtx).Claim(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func MyGiftClaimsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := logic.NewGiftPackLogic(r.Context(), svcCtx).MyClaims(authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// RedeemGiftPackHandler S2S 核销回调：验签基于原始请求体（HMAC 先于解析）。
func RedeemGiftPackHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawBody, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		err = logic.NewGiftPackLogic(r.Context(), svcCtx).Redeem(rawBody, r.Header.Get("X-GiftPack-Signature"))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0,"message":"ok"}`))
	}
}

func parseInt64(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	return n
}
