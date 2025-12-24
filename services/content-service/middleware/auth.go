package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/content-service/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// AuthMiddleware JWT 认证中间件
func AuthMiddleware(svcCtx *svc.ServiceContext) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 跳过不需要认证的路径
			if skipAuth(r.Method, r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// 获取 Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"code":401,"message":"缺少认证令牌"}`))
				return
			}

			// 检查 Bearer 前缀
			if !strings.HasPrefix(authHeader, "Bearer ") {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"code":401,"message":"无效的令牌格式"}`))
				return
			}

			// 提取 token
			token := authHeader[7:]
			if token == "" {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"code":401,"message":"令牌不能为空"}`))
				return
			}

			// 验证 token
			claims, err := svcCtx.Auth.ParseToken(token)
			if err != nil {
				logx.Errorw("令牌验证失败", logx.Field("error", err))
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"code":401,"message":"无效的令牌"}`))
				return
			}

			// 将用户信息存储到请求上下文
			ctx := context.WithValue(r.Context(), "user_id", claims.UserId)
			ctx = context.WithValue(ctx, "username", claims.Username)

			// 继续处理请求
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// skipAuth 检查是否跳过认证
func skipAuth(method, path string) bool {
	// 基础健康检查接口始终放行
	if path == "/ping" || path == "/health" {
		return true
	}

	// 只读接口允许匿名访问
	if method != http.MethodGet {
		return false
	}

	return strings.HasPrefix(path, "/api/v1/guides") || strings.HasPrefix(path, "/api/v1/comments")
}
