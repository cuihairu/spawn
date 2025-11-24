package logic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tappi/tappi/services/user-service/internal/integration"
	"github.com/tappi/tappi/services/user-service/internal/svc"
	"github.com/tappi/tappi/services/user-service/internal/types"
	"github.com/tappi/tappi/services/user-service/model"
	"github.com/tappi/tappi/services/user-service/utils"
)

func TestGetUserRecommendationsSuccess(t *testing.T) {
	db := setupTestDB(t)
	insertTestUser(t, db, "tester", "tester@example.com", "hashed", "Tester")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/games/recommendations" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"games": []map[string]any{
				{
					"id":          "game-1",
					"title":       "Demo Game",
					"cover_image": "https://example.com/cover.png",
					"genres":      []string{"RPG"},
					"platforms":   []string{"PC"},
					"score":       9.2,
					"tags":        []string{"Hot"},
				},
			},
		})
	}))
	defer server.Close()

	userModel := model.NewUserModel(db)

	svcCtx := &svc.ServiceContext{
		DB:                db,
		UserModel:         userModel,
		Auth:              utils.NewAuth("secret", 24*time.Hour),
		GameCatalogClient: integration.NewGameCatalogClient(server.URL, time.Second),
	}

	logic := NewGetUserRecommendationsLogic(context.Background(), svcCtx)
	resp, err := logic.GetUserRecommendations(&types.GetUserRecommendationsRequest{
		Id:    1,
		Limit: 3,
	})
	if err != nil {
		t.Fatalf("logic returned error: %v", err)
	}
	if resp.Code != 200 {
		t.Fatalf("expected success code, got %d", resp.Code)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected data type %#v", resp.Data)
	}

	recs, ok := data["recommendations"].([]types.GameRecommendation)
	if !ok || len(recs) != 1 {
		t.Fatalf("recommendations not populated: %#v", data["recommendations"])
	}
}

func TestGetUserRecommendationsGatewayError(t *testing.T) {
	db := setupTestDB(t)
	insertTestUser(t, db, "tester", "tester@example.com", "hashed", "Tester")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	svcCtx := &svc.ServiceContext{
		DB:                db,
		UserModel:         model.NewUserModel(db),
		Auth:              utils.NewAuth("secret", 24*time.Hour),
		GameCatalogClient: integration.NewGameCatalogClient(server.URL, time.Second),
	}

	logic := NewGetUserRecommendationsLogic(context.Background(), svcCtx)
	resp, err := logic.GetUserRecommendations(&types.GetUserRecommendationsRequest{
		Id:    1,
		Limit: 3,
	})
	if err != nil {
		t.Fatalf("logic returned error: %v", err)
	}
	if resp.Code != 502 {
		t.Fatalf("expected gateway error code, got %d", resp.Code)
	}
}
