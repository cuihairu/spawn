package handler

import (
	"net/http"
	"strings"
	"testing"
)

func TestRoutes_FeaturedGames(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	got := call(t, http.MethodGet, base+"/games/featured", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("featured status = %d, body = %s", got.status, got.body)
	}
	games := list(t, got.json, "games")
	if len(games) != 2 {
		t.Fatalf("games = %#v", games)
	}
	first, _ := games[0].(map[string]interface{})
	if first["id"] != "g-1" || first["title"] != "Alpha RPG" {
		t.Fatalf("first game = %#v", first)
	}

	// FeaturedRequest 的 limit 带 default=6：未传时 logic 兜底为 6。
	if q := stubs.game.last(t).query; q != "limit=6" {
		t.Fatalf("upstream query = %q, want limit=6", q)
	}
}

func TestRoutes_FeaturedGamesLimitPassthrough(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	got := call(t, http.MethodGet, base+"/games/featured?limit=3", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("featured status = %d, body = %s", got.status, got.body)
	}
	sent := stubs.game.last(t)
	if sent.method != http.MethodGet || sent.path != "/games/featured" {
		t.Fatalf("upstream request = %s %s", sent.method, sent.path)
	}
	if sent.query != "limit=3" {
		t.Fatalf("upstream query = %q, want limit=3", sent.query)
	}
}

func TestRoutes_FeaturedGamesUpstreamFailure(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	got := call(t, http.MethodGet, base+"/games/featured?limit=9", "", "")
	if got.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", got.status)
	}
	if !strings.Contains(got.body, "game catalog status 502") {
		t.Fatalf("body = %q, want 上游 502 说明", got.body)
	}
}

func TestRoutes_FeaturedGamesParseError(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	got := call(t, http.MethodGet, base+"/games/featured?limit=abc", "", "")
	if got.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", got.status, got.body)
	}
	if n := stubs.game.count(t); n != 0 {
		t.Fatalf("上游被调用 %d 次，parse 失败时不应有调用", n)
	}
}

// --- GET /users/:id/recommendations ---

func TestRoutes_UserRecommendations(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	got := call(t, http.MethodGet, base+"/users/7/recommendations?limit=5&genres=RPG", "Bearer gw-token", "")
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", got.status, got.body)
	}
	if got.json["code"] != float64(200) || got.json["message"] != "ok" {
		t.Fatalf("envelope = %#v", got.json)
	}
	data := object(t, got.json, "data")
	if games := list(t, data, "games"); len(games) != 1 {
		t.Fatalf("data.games = %#v", games)
	}

	// 上游契约：路径含 path 参数 id，query 只带显式参数，Bearer 透传。
	sent := stubs.user.last(t)
	if sent.method != http.MethodGet || sent.path != "/users/7/recommendations" {
		t.Fatalf("upstream request = %s %s", sent.method, sent.path)
	}
	if sent.query != "genres=RPG&limit=5" {
		t.Fatalf("upstream query = %q", sent.query)
	}
	if sent.auth != "Bearer gw-token" {
		t.Fatalf("upstream authorization = %q", sent.auth)
	}
}

func TestRoutes_UserRecommendationsTokenHandling(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	cases := []struct {
		name string
		auth string
		want string
	}{
		{"无 Authorization 头时不下发", "", ""},
		{"Bearer 前缀后需去空白", "Bearer   padded  ", "Bearer padded"},
		{"无 Bearer 前缀时整段当令牌", "raw-token", "Bearer raw-token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := call(t, http.MethodGet, base+"/users/7/recommendations", tc.auth, ""); got.status != http.StatusOK {
				t.Fatalf("status = %d, body = %s", got.status, got.body)
			}
			if auth := stubs.user.last(t).auth; auth != tc.want {
				t.Fatalf("upstream authorization = %q, want %q", auth, tc.want)
			}
		})
	}

	// 无 query 时 limit/genres 不得出现在上游查询串里（0/空值不传）。
	call(t, http.MethodGet, base+"/users/7/recommendations", "", "")
	if q := stubs.user.last(t).query; q != "" {
		t.Fatalf("upstream query = %q, want empty", q)
	}
}

func TestRoutes_UserRecommendationsUpstreamFailure(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	got := call(t, http.MethodGet, base+"/users/99/recommendations", "", "")
	if got.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", got.status)
	}
	if !strings.Contains(got.body, "recommendation failed: boom") {
		t.Fatalf("body = %q, want 上游原文", got.body)
	}
}

func TestRoutes_UserRecommendationsParseError(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 路径参数 id 非数字 → httpx.Parse 失败，且不得打到上游。
	got := call(t, http.MethodGet, base+"/users/abc/recommendations", "", "")
	if got.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", got.status, got.body)
	}
	if n := stubs.user.count(t); n != 0 {
		t.Fatalf("上游被调用 %d 次，parse 失败时不应有调用", n)
	}
}

// --- GET /s/p/:id 分享卡 ---

func TestRoutes_GamesListAndDetail(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 列表 + 搜索/分页 query 透传。
	got := call(t, http.MethodGet, base+"/games?keyword=rpg&limit=20&offset=0", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("games list status = %d, body = %s", got.status, got.body)
	}
	if games := list(t, got.json, "games"); len(games) != 1 {
		t.Fatalf("games = %#v", games)
	}
	sent := stubs.game.last(t)
	if sent.path != "/games" || sent.query != "keyword=rpg&limit=20&offset=0" {
		t.Fatalf("upstream request = %+v", sent)
	}

	// 详情路径参数透传。
	got = call(t, http.MethodGet, base+"/games/g-1", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("game detail status = %d, body = %s", got.status, got.body)
	}
	if game := object(t, got.json, "game"); game["title"] != "Alpha RPG" {
		t.Fatalf("game = %#v", game)
	}
	sent = stubs.game.last(t)
	if sent.path != "/games/g-1" {
		t.Fatalf("upstream request = %+v", sent)
	}

	// 缺失 → 上游 404 原样透传。
	got = call(t, http.MethodGet, base+"/games/nope", "", "")
	if got.status != http.StatusNotFound {
		t.Fatalf("missing status = %d, want 404", got.status)
	}
}

// --- data-panel 反代：/api/v1/stats（战绩面板） ---
