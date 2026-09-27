package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestServer 启动一个模拟 game-catalog 的测试服务器，并返回对应的客户端。
func newTestServer(t *testing.T, handler http.HandlerFunc) *GameCatalogClient {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return NewGameCatalogClient(server.URL, 5*time.Second)
}

func TestGetGameById_BareGameResponse(t *testing.T) {
	t.Parallel()

	var gotPath string
	cl := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"game": map[string]any{
				"id":          "game-001",
				"title":       "星穹幻境",
				"cover_image": "https://cdn.example.com/game-001.jpg",
				"genres":      []string{"RPG"},
				"platforms":   []string{"PC", "iOS"},
			},
		})
	})

	game, err := cl.GetGameById(context.Background(), "game-001")
	if err != nil {
		t.Fatalf("GetGameById() error = %v", err)
	}
	if gotPath != "/games/game-001" {
		t.Fatalf("expected request path /games/game-001, got %q", gotPath)
	}
	if game.Id != "game-001" || game.Title != "星穹幻境" {
		t.Fatalf("unexpected game: %+v", game)
	}
	if game.CoverImage != "https://cdn.example.com/game-001.jpg" {
		t.Fatalf("unexpected cover image: %q", game.CoverImage)
	}
	if len(game.Genres) != 1 || game.Genres[0] != "RPG" {
		t.Fatalf("unexpected genres: %v", game.Genres)
	}
	if len(game.Platforms) != 2 {
		t.Fatalf("unexpected platforms: %v", game.Platforms)
	}
}

func TestGetGameById_LegacyWrappedResponse(t *testing.T) {
	t.Parallel()

	cl := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusOK,
			"message": "ok",
			"data": map[string]any{
				"id":    "game-002",
				"title": "旧格式游戏",
			},
		})
	})

	game, err := cl.GetGameById(context.Background(), "game-002")
	if err != nil {
		t.Fatalf("GetGameById() error = %v", err)
	}
	if game.Id != "game-002" || game.Title != "旧格式游戏" {
		t.Fatalf("unexpected game: %+v", game)
	}
}

func TestGetGameById_Non200Status(t *testing.T) {
	t.Parallel()

	cl := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "游戏不存在", http.StatusNotFound)
	})

	game, err := cl.GetGameById(context.Background(), "missing")
	if err == nil {
		t.Fatalf("expected error for 404 response, got game %+v", game)
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected status code in error, got %q", err.Error())
	}
}

func TestGetGameById_BusinessErrorCode(t *testing.T) {
	t.Parallel()

	cl := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusInternalServerError,
			"message": "查询失败",
		})
	})

	game, err := cl.GetGameById(context.Background(), "game-003")
	if err == nil {
		t.Fatalf("expected error for business error code, got game %+v", game)
	}
	if !strings.Contains(err.Error(), "查询失败") {
		t.Fatalf("expected business message in error, got %q", err.Error())
	}
}

func TestGetGameById_InvalidJSON(t *testing.T) {
	t.Parallel()

	cl := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	})

	game, err := cl.GetGameById(context.Background(), "game-004")
	if err == nil {
		t.Fatalf("expected error for invalid json, got game %+v", game)
	}
	if !strings.Contains(err.Error(), "解析响应失败") {
		t.Fatalf("expected parse error, got %q", err.Error())
	}
}

func TestGetGameById_UnrecognizedBody(t *testing.T) {
	t.Parallel()

	cl := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"unexpected": true})
	})

	game, err := cl.GetGameById(context.Background(), "game-005")
	if err == nil {
		t.Fatalf("expected error for unrecognized body, got game %+v", game)
	}
	if !strings.Contains(err.Error(), "响应格式无法识别") {
		t.Fatalf("expected unrecognized-format error, got %q", err.Error())
	}
}

func TestGetGameById_UnreachableServer(t *testing.T) {
	t.Parallel()

	// 指向一个确定不可用的地址，模拟服务下线。
	cl := NewGameCatalogClient("http://127.0.0.1:1", time.Second)

	game, err := cl.GetGameById(context.Background(), "game-006")
	if err == nil {
		t.Fatalf("expected error for unreachable server, got game %+v", game)
	}
}

func TestGetGameById_ContextCanceled(t *testing.T) {
	t.Parallel()

	cl := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"game":{"id":"game-007"}}`))
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	game, err := cl.GetGameById(ctx, "game-007")
	if err == nil {
		t.Fatalf("expected error for canceled context, got game %+v", game)
	}
}

func TestNewGameCatalogClient_TrimsTrailingSlash(t *testing.T) {
	t.Parallel()

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"game":{"id":"game-008"}}`))
	}))
	defer server.Close()

	cl := NewGameCatalogClient(server.URL+"/", time.Second)
	if _, err := cl.GetGameById(context.Background(), "game-008"); err != nil {
		t.Fatalf("GetGameById() error = %v", err)
	}
	if gotPath != "/games/game-008" {
		t.Fatalf("expected request path /games/game-008, got %q", gotPath)
	}
}