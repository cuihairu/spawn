package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// gameBody 构造 CreateGameRequest 完整必填体（release_date 等为必填，缺则 Parse 400）。
func gameBody(title string) string {
	return `{"title":"` + title + `","description":"d","genres":["action"],` +
		`"platforms":["android"],"release_date":"2026-01-01","developer":"X",` +
		`"publisher":"Y","tags":[],"score":0,"cover_image":"https://e.com/c.jpg"}`
}

// TestRoutes_GameMutationPermission GP-004 最小权限：创建游戏与运营标签维护为
// platform_admin/operator 动作，player（缺省角色）直写应 403；operator 允许。
// 此前 POST /games 与 PATCH /games/:id/tags 仅挂 JWT 无角色守卫（任何登录用户可写）。
func TestRoutes_GameMutationPermission(t *testing.T) {
	base, sign := newAuthedTestServer(t)
	admin := sign(claimsFor(1, "admin"))
	player := sign(claimsFor(7, "bob"))

	// player 创建游戏 → 403（最小权限收紧）
	if code, body := doAuthed(t, http.MethodPost, base+"/games", player,
		gameBody("T")); code != http.StatusForbidden {
		t.Fatalf("player POST /games = %d body=%s, want 403", code, body)
	}

	// admin 创建游戏 → 200，取 id
	code, body := doAuthed(t, http.MethodPost, base+"/games", admin, gameBody("GP004"))
	if code != http.StatusOK {
		t.Fatalf("admin POST /games = %d body=%s, want 200", code, body)
	}
	var created struct {
		Game struct {
			Id string `json:"id"`
		} `json:"game"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil || created.Game.Id == "" {
		t.Fatalf("parse created game id: err=%v body=%s", err, body)
	}
	gid := created.Game.Id

	// player 改标签 → 403
	if code, body := doAuthed(t, http.MethodPatch, base+"/games/"+gid+"/tags", player,
		`{"tags":["联运"]}`); code != http.StatusForbidden {
		t.Fatalf("player PATCH tags = %d body=%s, want 403", code, body)
	}
	// admin 改标签 → 200
	if code, body := doAuthed(t, http.MethodPatch, base+"/games/"+gid+"/tags", admin,
		`{"tags":["联运"]}`); code != http.StatusOK {
		t.Fatalf("admin PATCH tags = %d body=%s, want 200", code, body)
	}

	// operator（user 2 经角色绑定）创建游戏 → 200
	if code, body := doAuthed(t, http.MethodPost, base+"/admin/roles", admin,
		`{"user_id":2,"role":"operator"}`); code != http.StatusOK {
		t.Fatalf("grant operator = %d body=%s, want 200", code, body)
	}
	operator := sign(claimsFor(2, "op"))
	if code, body := doAuthed(t, http.MethodPost, base+"/games", operator,
		gameBody("OP")); code != http.StatusOK {
		t.Fatalf("operator POST /games = %d body=%s, want 200", code, body)
	}

	// 审计：admin 创建与改标签均入 audit_logs（动作 game_create / game_tag_update）
	if code, body := doAuthed(t, http.MethodGet, base+"/admin/audit", admin, ""); code != http.StatusOK {
		t.Fatalf("GET /admin/audit = %d body=%s, want 200", code, body)
	} else if !strings.Contains(body, "game_create") || !strings.Contains(body, "game_tag_update") {
		t.Fatalf("audit missing game_create/game_tag_update: %s", body)
	}
}
