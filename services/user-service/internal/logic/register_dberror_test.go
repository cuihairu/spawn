package logic

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/tappi/tappi/services/user-service/internal/svc"
	"github.com/tappi/tappi/services/user-service/internal/types"
	"github.com/tappi/tappi/services/user-service/model"
	"github.com/tappi/tappi/services/user-service/utils"

	_ "github.com/mattn/go-sqlite3"
)

// TestRegisterLogic_EmailCheckError 用户名存在性检查通过、邮箱存在性检查报错
// → 500 信封（envelope 错误仍为 nil）。触达方式：users 表缺 email 列——
// CheckUsernameExists 按 username 查询正常，CheckEmailExists 报
// "no such column: email"。闭库会让先执行的用户名检查先失败，够不到此分支。
func TestRegisterLogic_EmailCheckError(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// 故意省略 email 列：仅邮箱列的查询报错，其余查询不受影响
	if _, err := db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password TEXT NOT NULL,
		nickname TEXT,
		created_at TIMESTAMP,
		updated_at TIMESTAMP
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	svcCtx := &svc.ServiceContext{
		DB:        db,
		UserModel: model.NewUserModel(db),
		Auth:      utils.NewAuth("db-error-secret", 24*time.Hour),
	}

	l := NewRegisterLogic(context.Background(), svcCtx)
	resp, err := l.Register(&types.RegisterRequest{
		Username: "carol", Email: "carol@example.com", Password: "pw-carol",
	})
	if err != nil {
		t.Fatalf("expected envelope, got error %v", err)
	}
	if resp.Code != 500 || resp.Message != "服务器内部错误" {
		t.Fatalf("resp = %+v, want 500 服务器内部错误", resp)
	}
}
