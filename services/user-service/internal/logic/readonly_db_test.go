package logic

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/tappi/tappi/services/user-service/internal/svc"
	"github.com/tappi/tappi/services/user-service/internal/types"
	"github.com/tappi/tappi/services/user-service/model"
	"github.com/tappi/tappi/services/user-service/utils"

	_ "github.com/mattn/go-sqlite3"
)

// newReadOnlySvcCtx 建库建表并预置用户后，以只读模式重开：
// SELECT 正常、INSERT/UPDATE 报 "attempt to write a readonly database"，
// 用于触达「前置检查通过但写库失败」的 500 分支
// （闭库会让先行的 SELECT 失败，够不到写库错误）。
func newReadOnlySvcCtx(t *testing.T) *svc.ServiceContext {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ro.db")

	writable, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open writable db: %v", err)
	}
	userModel := model.NewUserModel(writable)
	if err := userModel.CreateUsersTable(); err != nil {
		t.Fatalf("create table: %v", err)
	}
	now := time.Now()
	if _, err := writable.Exec(`INSERT INTO users (username, email, password, nickname, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"alice", "alice@example.com", "hashed", "Alice", now, now); err != nil {
		t.Fatalf("seed alice: %v", err)
	}
	if err := writable.Close(); err != nil {
		t.Fatalf("close writable db: %v", err)
	}

	ro, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open readonly db: %v", err)
	}
	t.Cleanup(func() { ro.Close() })

	return &svc.ServiceContext{
		DB:        ro,
		UserModel: model.NewUserModel(ro),
		Auth:      utils.NewAuth("ro-test-secret", 24*time.Hour),
	}
}

// TestRegisterLogic_CreateFailsReadOnly 用户名/邮箱检查（SELECT）通过后
// INSERT 失败 → 500「注册失败」envelope，error 仍为 nil。
func TestRegisterLogic_CreateFailsReadOnly(t *testing.T) {
	l := NewRegisterLogic(context.Background(), newReadOnlySvcCtx(t))

	resp, err := l.Register(&types.RegisterRequest{
		Username: "carol",
		Email:    "carol@example.com",
		Password: "pw-carol",
	})
	if err != nil {
		t.Fatalf("expected envelope, got error %v", err)
	}
	if resp.Code != 500 || resp.Message != "注册失败" {
		t.Fatalf("code=%d message=%q, want 500 注册失败", resp.Code, resp.Message)
	}
}

// TestUpdateUserInfoLogic_UpdateFailsReadOnly 前置 FindOne 成功后
// UPDATE 失败 → 500「更新失败」envelope。
func TestUpdateUserInfoLogic_UpdateFailsReadOnly(t *testing.T) {
	svcCtx := newReadOnlySvcCtx(t)
	alice, err := svcCtx.UserModel.FindByUsername("alice")
	if err != nil {
		t.Fatalf("find alice: %v", err)
	}

	l := NewUpdateUserInfoLogic(context.Background(), svcCtx)
	resp, err := l.UpdateUserInfo(&types.UpdateUserInfoRequest{
		Id:       alice.Id,
		Email:    "alice@new.com",
		Nickname: "Alicia",
	})
	if err != nil {
		t.Fatalf("expected envelope, got error %v", err)
	}
	if resp.Code != 500 || resp.Message != "更新失败" {
		t.Fatalf("code=%d message=%q, want 500 更新失败", resp.Code, resp.Message)
	}
}
