package logic

import (
	"context"
	"errors"
	"testing"

	"github.com/tappi/tappi/services/api-gateway/internal/integration"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/tappi/tappi/services/api-gateway/internal/types"
)

type mockUserService struct {
	loginFn func(ctx context.Context, payload integration.LoginPayload) (*integration.LoginResult, error)
	recoFn  func(ctx context.Context, userID int64, limit int64, genres string, token string) (*integration.RecommendationResponse, error)
}

func (m mockUserService) Login(ctx context.Context, payload integration.LoginPayload) (*integration.LoginResult, error) {
	if m.loginFn == nil {
		return nil, errors.New("loginFn not implemented")
	}
	return m.loginFn(ctx, payload)
}

func (m mockUserService) GetRecommendations(ctx context.Context, userID int64, limit int64, genres string, token string) (*integration.RecommendationResponse, error) {
	if m.recoFn == nil {
		return nil, errors.New("recoFn not implemented")
	}
	return m.recoFn(ctx, userID, limit, genres, token)
}

type mockGameCatalog struct {
	featuredFn func(ctx context.Context, limit int64) (map[string]interface{}, error)
}

func (m mockGameCatalog) GetFeatured(ctx context.Context, limit int64) (map[string]interface{}, error) {
	if m.featuredFn == nil {
		return nil, errors.New("featuredFn not implemented")
	}
	return m.featuredFn(ctx, limit)
}

func TestLoginLogicSuccess(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		UserService: mockUserService{
			loginFn: func(ctx context.Context, payload integration.LoginPayload) (*integration.LoginResult, error) {
				if payload.Username != "demo" || payload.Password != "secret" {
					t.Fatalf("unexpected payload: %#v", payload)
				}
				return &integration.LoginResult{
					Token:    "token-123",
					UserInfo: map[string]any{"username": payload.Username},
				}, nil
			},
			recoFn: func(ctx context.Context, userID int64, limit int64, genres string, token string) (*integration.RecommendationResponse, error) {
				return nil, errors.New("not expected")
			},
		},
	}

	logic := NewLoginLogic(context.Background(), svcCtx)
	resp, err := logic.Login(&types.LoginRequest{Username: "demo", Password: "secret"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Token != "token-123" {
		t.Fatalf("expected token value, got %s", resp.Token)
	}
}

func TestLoginLogicError(t *testing.T) {
	expected := errors.New("upstream error")
	svcCtx := &svc.ServiceContext{
		UserService: mockUserService{
			loginFn: func(ctx context.Context, payload integration.LoginPayload) (*integration.LoginResult, error) {
				return nil, expected
			},
			recoFn: func(ctx context.Context, userID int64, limit int64, genres string, token string) (*integration.RecommendationResponse, error) {
				return nil, nil
			},
		},
	}

	logic := NewLoginLogic(context.Background(), svcCtx)
	if _, err := logic.Login(&types.LoginRequest{}); !errors.Is(err, expected) {
		t.Fatalf("expected error %v, got %v", expected, err)
	}
}

func TestFeaturedGamesLogicUsesDefaultLimit(t *testing.T) {
	var capturedLimit int64
	svcCtx := &svc.ServiceContext{
		GameCatalog: mockGameCatalog{
			featuredFn: func(ctx context.Context, limit int64) (map[string]interface{}, error) {
				capturedLimit = limit
				return map[string]interface{}{
					"games": []interface{}{"a", "b"},
				}, nil
			},
		},
		UserService: mockUserService{
			loginFn: func(ctx context.Context, payload integration.LoginPayload) (*integration.LoginResult, error) {
				return nil, nil
			},
			recoFn: func(ctx context.Context, userID int64, limit int64, genres string, token string) (*integration.RecommendationResponse, error) {
				return nil, nil
			},
		},
	}

	logic := NewFeaturedGamesLogic(context.Background(), svcCtx)
	resp, err := logic.FeaturedGames(&types.FeaturedRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 6 {
		t.Fatalf("expected default limit 6, got %d", capturedLimit)
	}
	if len(resp.Games) != 2 {
		t.Fatalf("expected 2 games, got %d", len(resp.Games))
	}
}

func TestUserRecommendationsLogic(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		UserService: mockUserService{
			loginFn: func(ctx context.Context, payload integration.LoginPayload) (*integration.LoginResult, error) {
				return nil, nil
			},
			recoFn: func(ctx context.Context, userID int64, limit int64, genres string, token string) (*integration.RecommendationResponse, error) {
				if token != "token-abc" || userID != 42 || limit != 10 || genres != "RPG" {
					t.Fatalf("unexpected params: user=%d limit=%d genres=%s token=%s", userID, limit, genres, token)
				}
				return &integration.RecommendationResponse{
					Code:    200,
					Message: "ok",
					Data: map[string]any{
						"recommendations": []string{"game-1"},
					},
				}, nil
			},
		},
		GameCatalog: mockGameCatalog{
			featuredFn: func(ctx context.Context, limit int64) (map[string]interface{}, error) {
				return nil, nil
			},
		},
	}

	logic := NewUserRecommendationsLogic(context.Background(), svcCtx)
	resp, err := logic.UserRecommendations(&types.RecommendationRequest{
		Id:     42,
		Limit:  10,
		Genres: "RPG",
	}, "token-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Code != 200 || resp.Message != "ok" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	data, ok := resp.Data.(map[string]any)
	if !ok || len(data) == 0 {
		t.Fatalf("unexpected data type: %#v", resp.Data)
	}
}
