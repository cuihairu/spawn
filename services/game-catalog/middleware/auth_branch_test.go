package middleware

// AuthMiddleware 读平台角色失败臂：GetRole 报错 → 记日志并回退 player 角色，
// 不阻断请求（缺省角色语义）。

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/model"
	"github.com/tappi/tappi/services/game-catalog/utils"

	_ "github.com/mattn/go-sqlite3"
)

// 角色读失败：关库 AdminModel.GetRole 报错 → 中间件按 player 放行。
func TestAuthMiddleware_GetRoleFails(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/auth.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	admin := model.NewAdminModel(db)
	if err := admin.CreateAdminTables(); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	db.Close()
	svcCtx := &svc.ServiceContext{Auth: utils.NewAuth(testSecret), AdminRepository: admin}

	var gotRole string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v, ok := r.Context().Value("role").(string); ok {
			gotRole = v
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := AuthMiddleware(svcCtx)(next)

	req := httptest.NewRequest(http.MethodPost, "/games", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, testSecret, validClaims()))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (role read failure must not block)", rr.Code)
	}
	if gotRole != model.RolePlayer {
		t.Fatalf("role = %q, want player (fallback)", gotRole)
	}
}
