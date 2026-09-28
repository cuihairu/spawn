package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
)

// AuthMiddleware JWT 认证中间件：校验 Bearer 令牌并注入用户信息。
// 用于受保护的写接口（POST /games）；令牌由 user-service 签发，本服务仅校验。
// 签名对齐 go-zero rest.Middleware（社区服务同款），可经 rest.WithMiddleware 挂到单条路由。
func AuthMiddleware(svcCtx *svc.ServiceContext) rest.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"code":401,"message":"缺少认证令牌"}`))
				return
			}

			if !strings.HasPrefix(authHeader, "Bearer ") {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"code":401,"message":"无效的令牌格式"}`))
				return
			}

			token := strings.TrimSpace(authHeader[len("Bearer "):])
			if token == "" {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"code":401,"message":"令牌不能为空"}`))
				return
			}

			claims, err := svcCtx.Auth.ParseToken(token)
			if err != nil {
				logx.Errorw("令牌验证失败", logx.Field("error", err))
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"code":401,"message":"无效的令牌"}`))
				return
			}

			ctx := context.WithValue(r.Context(), "user_id", claims.UserId)
			ctx = context.WithValue(ctx, "username", claims.Username)

			next(w, r.WithContext(ctx))
		}
	}
}
