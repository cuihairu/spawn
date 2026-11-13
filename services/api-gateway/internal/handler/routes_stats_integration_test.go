package handler

import (
	"net/http"
	"strings"
	"testing"
)

func TestRoutes_StatsSummaryAndGames(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 汇总公开透传。
	got := call(t, http.MethodGet, base+"/api/v1/stats/users/1001/summary", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("summary status = %d, body = %s", got.status, got.body)
	}
	if summary := object(t, got.json, "summary"); summary["total_matches"] != float64(42) ||
		summary["game_count"] != float64(2) {
		t.Fatalf("summary = %#v", summary)
	}
	sent := stubs.dataPanel.last(t)
	if sent.method != http.MethodGet || sent.path != "/api/v1/stats/users/1001/summary" {
		t.Fatalf("upstream request = %s %s", sent.method, sent.path)
	}

	// 明细列表：query 分页透传。
	got = call(t, http.MethodGet, base+"/api/v1/stats/users/1001/games?limit=10&offset=0", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("games status = %d, body = %s", got.status, got.body)
	}
	if games := list(t, got.json, "games"); len(games) != 1 {
		t.Fatalf("games = %#v", games)
	}
	if q := stubs.dataPanel.last(t).query; q != "limit=10&offset=0" {
		t.Fatalf("upstream query = %q, want limit=10&offset=0", q)
	}

	// 缺失游戏 → 上游 404 原样透传。
	got = call(t, http.MethodGet, base+"/api/v1/stats/users/1001/games/nope", "", "")
	if got.status != http.StatusNotFound {
		t.Fatalf("missing game status = %d, want 404", got.status)
	}
}

func TestRoutes_StatsRecordIngest(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 摄入：Bearer 透传，body 原样转发（归属由 data-panel 从令牌取）。
	got := call(t, http.MethodPost, base+"/api/v1/stats/records", "Bearer gw-token",
		`{"game_id":"game-valorant","matches":1,"wins":1}`)
	if got.status != http.StatusOK {
		t.Fatalf("record status = %d, body = %s", got.status, got.body)
	}
	if got.json["code"] != float64(200) {
		t.Fatalf("envelope = %#v", got.json)
	}
	if stat := object(t, got.json, "stat"); stat["matches"] != float64(88) {
		t.Fatalf("stat = %#v", stat)
	}
	sent := stubs.dataPanel.last(t)
	if sent.method != http.MethodPost || sent.path != "/api/v1/stats/records" {
		t.Fatalf("upstream request = %s %s", sent.method, sent.path)
	}
	if sent.auth != "Bearer gw-token" {
		t.Fatalf("upstream authorization = %q", sent.auth)
	}
	if !strings.Contains(sent.body, `"game_id":"game-valorant"`) {
		t.Fatalf("upstream body = %q", sent.body)
	}

	// 匿名摄入 → 上游 401 原样透传。
	got = call(t, http.MethodPost, base+"/api/v1/stats/records", "",
		`{"game_id":"game-valorant","matches":1}`)
	if got.status != http.StatusUnauthorized {
		t.Fatalf("anon record status = %d, want 401", got.status)
	}
}

// --- community 反代：/api/v1/notifications（站内通知） ---
