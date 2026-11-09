package handler

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/promotions/internal/logic"
	"github.com/tappi/tappi/services/promotions/internal/svc"
	"github.com/tappi/tappi/services/promotions/internal/types"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// RegisterHandlers 活动配置管理面路由（GP-301）：平台 JWT + 角色经
// game-catalog /me 权威判定（platform_admin/operator）。
func RegisterHandlers(server *rest.Server, svcCtx *svc.ServiceContext) {
	auth := AuthMiddleware(svcCtx)

	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/healthz", Handler: HealthHandler()},
	})

	adminRoutes := []rest.Route{
		{Method: http.MethodPost, Path: "/admin/activities", Handler: CreateActivityHandler(svcCtx)},
		{Method: http.MethodPut, Path: "/admin/activities/:id", Handler: UpdateActivityHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/admin/activities/:id/publish", Handler: PublishActivityHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/admin/activities/:id/disable", Handler: DisableActivityHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/admin/activities/:id/revise", Handler: ReviseActivityHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/activities", Handler: ListActivitiesHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/activities/:id", Handler: GetActivityHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/activities/:id/versions", Handler: ListActivityVersionsHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/activity-audit", Handler: ListActivityAuditHandler(svcCtx)},
		// GP-302 奖励发放账本查询（管理面；评估入口为 S2S 专用不在此组）
		{Method: http.MethodGet, Path: "/admin/reward-grants", Handler: ListRewardGrantsHandler(svcCtx)},
		// GP-304 礼包管理面：配置/状态机 + 码库存 + 领取记录
		{Method: http.MethodPost, Path: "/admin/gift-packs", Handler: CreateGiftPackHandler(svcCtx)},
		{Method: http.MethodPut, Path: "/admin/gift-packs/:id", Handler: UpdateGiftPackHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/admin/gift-packs/:id/publish", Handler: PublishGiftPackHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/admin/gift-packs/:id/disable", Handler: DisableGiftPackHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/admin/gift-packs/:id/revise", Handler: ReviseGiftPackHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/gift-packs", Handler: ListGiftPacksHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/gift-packs/:id", Handler: GetGiftPackHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/admin/gift-packs/:id/codes", Handler: ImportGiftPackCodesHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/gift-packs/:id/codes", Handler: ListGiftPackCodesHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/gift-packs/:id/claims", Handler: ListGiftClaimsHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/gift-claims", Handler: ListGiftClaimsHandler(svcCtx)},
	}
	server.AddRoutes(rest.WithMiddleware(auth, adminRoutes...))

	// GP-304 玩家侧（平台 JWT，无需管理角色）：展示/领取/我的领取
	playerRoutes := []rest.Route{
		{Method: http.MethodGet, Path: "/gift-packs", Handler: ListPlayerGiftPacksHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/gift-packs/:id", Handler: GetPlayerGiftPackHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/gift-packs/:id/claim", Handler: ClaimGiftPackHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/gift-claims/me", Handler: MyGiftClaimsHandler(svcCtx)},
	}
	server.AddRoutes(rest.WithMiddleware(auth, playerRoutes...))

	// GP-302/304 S2S（payments PAID 推送 / 游戏服核销回调；HMAC 验签，不进网关）
	server.AddRoutes([]rest.Route{
		{Method: http.MethodPost, Path: "/internal/reward-evaluations", Handler: EvaluateRewardHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/internal/gift-pack-redemptions", Handler: RedeemGiftPackHandler(svcCtx)},
	})
}

// AuthMiddleware 平台 JWT 认证（user-service 签发，本服务仅校验；注入原始
// Authorization 头供角色 /me 查询透传）。
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

func authHeaderFrom(r *http.Request) string {
	if v, ok := r.Context().Value("auth_header").(string); ok {
		return v
	}
	return ""
}

func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

func CreateActivityHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CreateActivityRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewActivityLogic(r.Context(), svcCtx).Create(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func UpdateActivityHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.UpdateActivityRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewActivityLogic(r.Context(), svcCtx).Update(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func PublishActivityHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ActivityPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewActivityLogic(r.Context(), svcCtx).Publish(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func DisableActivityHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ActivityPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewActivityLogic(r.Context(), svcCtx).Disable(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ReviseActivityHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ActivityPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewActivityLogic(r.Context(), svcCtx).Revise(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ListActivitiesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ActivityListRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewActivityLogic(r.Context(), svcCtx).List(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func GetActivityHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ActivityPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewActivityLogic(r.Context(), svcCtx).Get(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ListActivityVersionsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ActivityPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewActivityLogic(r.Context(), svcCtx).Versions(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ListActivityAuditHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AuditListRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewActivityLogic(r.Context(), svcCtx).ListAudit(req.ActivityId, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// EvaluateRewardHandler GP-302 S2S 奖励评估：验签基于原始请求体
// （HMAC 先于解析），与 payments 发货接缝同款。
func EvaluateRewardHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawBody, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewRewardLogic(r.Context(), svcCtx).Evaluate(rawBody, r.Header.Get("X-Reward-Signature"))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// ListRewardGrantsHandler GP-302 奖励账本管理面查询。
func ListRewardGrantsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.RewardGrantListRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewRewardLogic(r.Context(), svcCtx).ListGrants(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
