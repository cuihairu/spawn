// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/community/utils"
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
			http.Error(w, "missing authorization token", http.StatusUnauthorized)
			return
		}
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "invalid token format", http.StatusUnauthorized)
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if token == "" {
			http.Error(w, "empty token", http.StatusUnauthorized)
			return
		}

		if m.auth == nil {
			http.Error(w, "auth not configured", http.StatusInternalServerError)
			return
		}

		claims, err := m.auth.ParseToken(token)
		if err != nil {
			// Keep the surface area small; callers only need to know it failed.
			http.Error(w, errors.New("invalid token").Error(), http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), "user_id", claims.UserId)
		ctx = context.WithValue(ctx, "username", claims.Username)

		next(w, r.WithContext(ctx))
	}
}
