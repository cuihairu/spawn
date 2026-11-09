package handler

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/courier-adapter/internal/logic"
	"github.com/tappi/tappi/services/courier-adapter/internal/svc"
	"github.com/tappi/tappi/services/courier-adapter/internal/types"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// RegisterHandlers 路由装配（手工装配，与 game-catalog 同款口径）：
// 玩家面：POST /games/:id/session（平台 JWT）、POST /session/redeem（票据即凭证）、
// POST /games/:id/account/delete(+ /cancel)（平台 JWT）；
// 管理面：/admin/games/:id/mapping 读写、/admin/audit（角色经 game-catalog /me）。
func RegisterHandlers(server *rest.Server, svcCtx *svc.ServiceContext) {
	auth := AuthMiddleware(svcCtx)

	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/healthz", Handler: HealthHandler()},
	})

	playerRoutes := []rest.Route{
		{Method: http.MethodPost, Path: "/games/:id/session", Handler: GameSessionHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/games/:id/account/delete", Handler: DeleteAccountHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/games/:id/account/delete/cancel", Handler: CancelDeleteHandler(svcCtx)},
		// GP-307 售后工单（Courier SupportProvider 适配；能力按需开通，
		// 映射 capabilities.support 未开 → 403）
		{Method: http.MethodPost, Path: "/games/:id/support/tickets", Handler: CreateSupportTicketHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/games/:id/support/tickets", Handler: ListSupportTicketsHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/games/:id/support/tickets/:ticketId", Handler: SupportTicketDetailHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/games/:id/support/tickets/:ticketId/messages", Handler: AppendSupportTicketHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/games/:id/support/faq", Handler: SupportFaqHandler(svcCtx)},
	}
	server.AddRoutes(rest.WithMiddleware(auth, playerRoutes...))

	server.AddRoutes([]rest.Route{
		{Method: http.MethodPost, Path: "/session/redeem", Handler: RedeemHandler(svcCtx)},
	})

	adminRoutes := []rest.Route{
		{Method: http.MethodGet, Path: "/admin/games/:id/mapping", Handler: GetMappingHandler(svcCtx)},
		{Method: http.MethodPut, Path: "/admin/games/:id/mapping", Handler: UpsertMappingHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/audit", Handler: ListAuditHandler(svcCtx)},
	}
	server.AddRoutes(rest.WithMiddleware(auth, adminRoutes...))
}

// AuthMiddleware 平台 JWT 认证（令牌由 user-service 签发，本服务仅校验；
// 与 game-catalog middleware/auth.go 同款，另注入原始 Authorization 头供
// 上游角色/邮箱查询透传）。
func AuthMiddleware(svcCtx *svc.ServiceContext) rest.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":401,"message":"缺少认证令牌"}`))
				return
			}
			token := strings.TrimSpace(authHeader[len("Bearer "):])
			if token == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":401,"message":"令牌不能为空"}`))
				return
			}
			claims, err := svcCtx.Auth.ParseToken(token)
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":401,"message":"无效的令牌"}`))
				return
			}
			ctx := context.WithValue(r.Context(), "user_id", claims.UserId)
			ctx = context.WithValue(ctx, "username", claims.Username)
			ctx = context.WithValue(ctx, "auth_header", authHeader)
			ctx = context.WithValue(ctx, "client_ip", clientIP(r))
			next(w, r.WithContext(ctx))
		}
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// authHeaderFrom 取中间件注入的原始 Authorization 头。
func authHeaderFrom(r *http.Request) string {
	if v, ok := r.Context().Value("auth_header").(string); ok {
		return v
	}
	return ""
}

func userIDFrom(r *http.Request) int64 {
	id, _ := r.Context().Value("user_id").(int64)
	return id
}

func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

func GameSessionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GameSessionRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewSessionLogic(r.Context(), svcCtx).GameSession(req.Id, authHeaderFrom(r), userIDFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func RedeemHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.RedeemRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewSessionLogic(r.Context(), svcCtx).Redeem(req.Ticket)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func DeleteAccountHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DeleteAccountRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewAccountLogic(r.Context(), svcCtx).Delete(req.Id, authHeaderFrom(r), userIDFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func CancelDeleteHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CancelDeleteRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		if err := logic.NewAccountLogic(r.Context(), svcCtx).CancelDelete(req.Id, req.CancelToken, userIDFrom(r)); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, struct{}{})
	}
}

func GetMappingHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GameSessionRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewAdminLogic(r.Context(), svcCtx).GetMapping(req.Id, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func UpsertMappingHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.UpsertMappingRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		if err := logic.NewAdminLogic(r.Context(), svcCtx).UpsertMapping(&req, authHeaderFrom(r), userIDFrom(r)); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, struct{}{})
	}
}

func ListAuditHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := logic.NewAdminLogic(r.Context(), svcCtx).ListAudit(authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// --- GP-307 售后工单（Courier SupportProvider 适配代理）---

func CreateSupportTicketHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CreateSupportTicketRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewSupportLogic(r.Context(), svcCtx).Create(&req, authHeaderFrom(r), userIDFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ListSupportTicketsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.SupportTicketListRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewSupportLogic(r.Context(), svcCtx).List(&req, authHeaderFrom(r), userIDFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func SupportTicketDetailHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.SupportTicketPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewSupportLogic(r.Context(), svcCtx).Detail(&req, authHeaderFrom(r), userIDFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func AppendSupportTicketHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AppendSupportTicketRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewSupportLogic(r.Context(), svcCtx).Append(&req, authHeaderFrom(r), userIDFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func SupportFaqHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.SupportFaqRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewSupportLogic(r.Context(), svcCtx).FAQ(&req, authHeaderFrom(r), userIDFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
