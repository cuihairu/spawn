package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tappi/tappi/services/user-service/internal/httperr"
	"github.com/tappi/tappi/services/user-service/internal/svc"
	"github.com/tappi/tappi/services/user-service/internal/types"
	"github.com/tappi/tappi/services/user-service/model"
	"github.com/tappi/tappi/services/user-service/utils"
)

func TestLoginLogicSuccess(t *testing.T) {
	db := setupTestDB(t)
	hashed, err := utils.HashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	insertTestUser(t, db, "demo", "demo@example.com", hashed, "Demo")

	svcCtx := &svc.ServiceContext{
		DB:        db,
		UserModel: model.NewUserModel(db),
		Auth:      utils.NewAuth("secret", 24*time.Hour),
	}

	logic := NewLoginLogic(context.Background(), svcCtx)
	resp, err := logic.Login(&types.LoginRequest{
		Username: "demo",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp == nil || resp.Code != 200 {
		t.Fatalf("expected 200 envelope, got %+v", resp)
	}
	data, ok := resp.Data.(*types.LoginResponse)
	if !ok || data.UserInfo == nil {
		t.Fatalf("expected LoginResponse data, got %+v", resp.Data)
	}
	if data.UserInfo.Username != "demo" {
		t.Fatalf("unexpected username %s", data.UserInfo.Username)
	}
	if data.Token == "" {
		t.Fatalf("token should not be empty")
	}
}

func TestLoginLogicWrongPassword(t *testing.T) {
	db := setupTestDB(t)
	hashed, _ := utils.HashPassword("password123")
	insertTestUser(t, db, "demo", "demo@example.com", hashed, "Demo")

	svcCtx := &svc.ServiceContext{
		DB:        db,
		UserModel: model.NewUserModel(db),
		Auth:      utils.NewAuth("secret", 24*time.Hour),
	}

	logic := NewLoginLogic(context.Background(), svcCtx)
	_, err := logic.Login(&types.LoginRequest{
		Username: "demo",
		Password: "wrong",
	})
	var he *httperr.Error
	if !errors.As(err, &he) || he.Status != 401 || he.Message != "用户名或密码错误" {
		t.Fatalf("err = %v, want 401 用户名或密码错误", err)
	}
}

func TestLoginLogicValidation(t *testing.T) {
	db := setupTestDB(t)
	svcCtx := &svc.ServiceContext{
		DB:        db,
		UserModel: model.NewUserModel(db),
		Auth:      utils.NewAuth("secret", 24*time.Hour),
	}
	logic := NewLoginLogic(context.Background(), svcCtx)

	var he *httperr.Error
	if _, err := logic.Login(&types.LoginRequest{}); !errors.As(err, &he) || he.Status != 400 {
		t.Fatalf("err = %v, want 400 for missing username", err)
	}
	if _, err := logic.Login(&types.LoginRequest{Username: "demo"}); !errors.As(err, &he) || he.Status != 400 {
		t.Fatalf("err = %v, want 400 for missing password", err)
	}
}
