// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/utils"
	"github.com/zeromicro/go-zero/rest/httpx"
)

type AuthMiddleware struct {
	auth *utils.Auth
}

func NewAuthMiddleware(auth *utils.Auth) *AuthMiddleware {
	return &AuthMiddleware{auth: auth}
}

func (m *AuthMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Allow preflight to pass through.
		if r.Method == http.MethodOptions {
			next(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			httpx.ErrorCtx(r.Context(), w, httperr.Unauthorized("missing authorization token"))
			return
		}
		if !strings.HasPrefix(authHeader, "Bearer ") {
			httpx.ErrorCtx(r.Context(), w, httperr.Unauthorized("invalid token format"))
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if token == "" {
			httpx.ErrorCtx(r.Context(), w, httperr.Unauthorized("empty token"))
			return
		}

		if m.auth == nil {
			httpx.ErrorCtx(r.Context(), w, httperr.Internal("auth not configured"))
			return
		}

		claims, err := m.auth.ParseToken(token)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, httperr.Unauthorized("invalid token"))
			return
		}

		ctx := context.WithValue(r.Context(), "user_id", claims.UserId)
		ctx = context.WithValue(ctx, "username", claims.Username)

		next(w, r.WithContext(ctx))
	}
}
