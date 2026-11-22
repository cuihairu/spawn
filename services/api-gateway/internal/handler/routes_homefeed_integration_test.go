package handler

import (
	"net/http"
	"testing"
)

func TestRoutes_HomeFeed(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	got := call(t, http.MethodGet, base+"/home/feed?limit=4", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("home feed status = %d, body = %s", got.status, got.body)
	}

	// 精选游戏：裁剪后的字段在、大字段不在。
	games := list(t, got.json, "featured_games")
	if len(games) != 2 {
		t.Fatalf("featured_games = %#v", games)
	}
	first, _ := games[0].(map[string]interface{})
	if first["id"] != "g-1" || first["title"] != "Alpha RPG" || first["score"] != float64(9.5) {
		t.Fatalf("first game = %#v", first)
	}
	for _, dropped := range []string{"description", "developer", "publisher", "tags"} {
		if _, keep := first[dropped]; keep {
			t.Fatalf("featured_games 裁剪失败：%q 不应下发", dropped)
		}
	}

	// 热帖：正文不下发，摘要截断到 60 rune + …；计数类字段保留。
	posts := list(t, got.json, "hot_posts")
	if len(posts) != 1 {
		t.Fatalf("hot_posts = %#v", posts)
	}
	post, _ := posts[0].(map[string]interface{})
	if _, keep := post["content"]; keep {
		t.Fatal("hot_posts 裁剪失败：content 不应下发")
	}
	summary, _ := post["summary"].(string)
	if got := []rune(summary); len(got) != 61 || string(got[len(got)-1]) != "…" {
		t.Fatalf("summary = %q (%d runes), want 60 runes + …", summary, len(got))
	}
	if post["id"] != float64(11) || post["like_count"] != float64(12) || post["comment_count"] != float64(3) {
		t.Fatalf("post = %#v", post)
	}

	// 话题：description 不下发。
	topics := list(t, got.json, "topics")
	if len(topics) != 1 {
		t.Fatalf("topics = %#v", topics)
	}
	topic, _ := topics[0].(map[string]interface{})
	if _, keep := topic["description"]; keep {
		t.Fatal("topics 裁剪失败：description 不应下发")
	}
	if topic["name"] != "星陨圈" || topic["is_official"] != true {
		t.Fatalf("topic = %#v", topic)
	}

	// 攻略：content 不下发，summary 用官方摘要。
	guides := list(t, got.json, "guides")
	if len(guides) != 1 {
		t.Fatalf("guides = %#v", guides)
	}
	guide, _ := guides[0].(map[string]interface{})
	if _, keep := guide["content"]; keep {
		t.Fatal("guides 裁剪失败：content 不应下发")
	}
	if guide["summary"] != "官方摘要" || guide["title"] != "全收集攻略" {
		t.Fatalf("guide = %#v", guide)
	}

	// 正常路径 degraded 必须是空数组。
	if degraded := list(t, got.json, "degraded"); len(degraded) != 0 {
		t.Fatalf("degraded = %#v, want empty", degraded)
	}

	// 上游契约：game featured 带 limit=4；community hot+topics 各一次。
	sent := stubs.game.last(t)
	if sent.path != "/games/featured" || sent.query != "limit=4" {
		t.Fatalf("game upstream = %+v", sent)
	}
	seen := map[string]int{}
	for _, c := range stubs.community.calls {
		seen[c.path]++
	}
	if seen["/api/v1/posts/hot"] != 1 || seen["/api/v1/topics"] != 1 {
		t.Fatalf("community upstream calls = %v", seen)
	}
}

func TestRoutes_HomeFeedDegradesPerGroup(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// limit=9 触发 game 上游 502 → 仅 featured_games 降级，其余组照常返回。
	got := call(t, http.MethodGet, base+"/home/feed?limit=9", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("degraded status = %d, want 200 (body=%s)", got.status, got.body)
	}
	if games := list(t, got.json, "featured_games"); len(games) != 0 {
		t.Fatalf("featured_games = %#v, want empty", games)
	}
	degraded := list(t, got.json, "degraded")
	if len(degraded) != 1 || degraded[0] != "featured_games" {
		t.Fatalf("degraded = %#v, want [featured_games]", degraded)
	}
	// 其余三组仍有数据。
	if posts := list(t, got.json, "hot_posts"); len(posts) != 1 {
		t.Fatalf("hot_posts = %#v", posts)
	}
	if topics := list(t, got.json, "topics"); len(topics) != 1 {
		t.Fatalf("topics = %#v", topics)
	}
	if guides := list(t, got.json, "guides"); len(guides) != 1 {
		t.Fatalf("guides = %#v", guides)
	}
}

func TestRoutes_HomeFeedRouteBoundary(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 只注册 GET：POST → 405 且不触达上游；limit 非法 → 400。
	got := call(t, http.MethodPost, base+"/home/feed", "", `{}`)
	if got.status != http.StatusMethodNotAllowed {
		t.Fatalf("POST /home/feed status = %d, want 405", got.status)
	}
	got = call(t, http.MethodGet, base+"/home/feed?limit=abc", "", "")
	if got.status != http.StatusBadRequest {
		t.Fatalf("bad limit status = %d, want 400 (body=%s)", got.status, got.body)
	}
}

// --- BFF 第一阶段：user/game 反代（mobile-app 统一入口） ---
