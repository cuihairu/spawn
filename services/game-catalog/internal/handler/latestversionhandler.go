package handler

import (
	"net/http"

	"github.com/tappi/tappi/services/game-catalog/internal/logic"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/go-zero/rest/pathvar"
)

// LatestVersionHandler GP-105 公开读取游戏当前已发布版本元数据；
// 客户端据此决定平台受控下载或商店链接承接（F08）。匿名可读（与游戏详情一致）。
func LatestVersionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameId := pathvar.Vars(r)["id"]
		resp, err := logic.NewLatestVersionLogic(withClientIP(r), svcCtx).Latest(gameId)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
