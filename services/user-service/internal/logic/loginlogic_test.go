package logic

import (
	"context"
	"testing"
	"time"

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
	if resp == nil || resp.UserInfo == nil {
		t.Fatalf("expected user info in response")
	}
	if resp.UserInfo.Username != "demo" {
		t.Fatalf("unexpected username %s", resp.UserInfo.Username)
	}
	if resp.Token == "" {
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
	if err == nil {
		t.Fatalf("expected error for wrong password")
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

	if _, err := logic.Login(&types.LoginRequest{}); err == nil {
		t.Fatalf("expected validation error for missing username")
	}
	if _, err := logic.Login(&types.LoginRequest{Username: "demo"}); err == nil {
		t.Fatalf("expected validation error for missing password")
	}
}
