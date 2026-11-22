package handler

import (
	"net/http"

	"github.com/tappi/tappi/services/game-catalog/internal/logic"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// GP-201 区服目录/充值目标：合作方维护 + 玩家公开读。

// PartnerCreateServerHandler 合作方为自有游戏新建区服。
func PartnerCreateServerHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CreateServerRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewPartnerLogic(withClientIP(r), svcCtx).CreateServer(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// PartnerUpdateServerHandler 合作方更新区服（改名/迁移/状态变更）。
func PartnerUpdateServerHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.UpdateServerRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewPartnerLogic(withClientIP(r), svcCtx).UpdateServer(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// PartnerListServersHandler 合作方视角区服列表（全部状态可见）。
func PartnerListServersHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.IdPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewPartnerLogic(withClientIP(r), svcCtx).ListPartnerServers(req.Id)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// PublicListServersHandler 玩家公开区服列表（已上架游戏，隐藏 closed）。
func PublicListServersHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GetGameDetailRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewGetGameDetailLogic(r.Context(), svcCtx).ListServers(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
