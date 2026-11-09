package handler

// GP-201 区服目录与充值目标关联集成测试：
// 合作方维护自有游戏的区服（增/改/列），玩家公开读开放区服；
// 跨合作方越权 403；操作落审计。

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestRoutes_GameServers(t *testing.T) {
	base, sign := newAuthedTestServer(t)
	admin := sign(claimsFor(1, "admin"))

	// 造两个合作方：各拥有一款已上架游戏（游戏先建档并上架，公开区服列表
	// 与详情同口径只对已上架游戏开放）；返回 (游戏id, 合作方令牌)
	mkPartnerGame := func(title, partnerName string) (string, string) {
		gameBody := `{"title":"` + title + `","description":"gp-201","genres":["MMO"],"platforms":["Android"],"release_date":"2026-11-09","developer":"d","publisher":"p","tags":[],"score":8,"cover_image":"https://example.com/cover.jpg"}`
		code, body := doAuthed(t, http.MethodPost, base+"/games", admin, gameBody)
		if code != http.StatusOK {
			t.Fatalf("create game %s = %d body=%s", title, code, body)
		}
		var gameResp struct {
			Game struct {
				Id string `json:"id"`
			} `json:"game"`
		}
		if err := json.Unmarshal([]byte(body), &gameResp); err != nil || gameResp.Game.Id == "" {
			t.Fatalf("decode created game: %v body=%s", err, body)
		}
		gameID := gameResp.Game.Id
		_, body = doAuthed(t, http.MethodPost, base+"/admin/partners", admin, `{"name":"`+partnerName+`"}`)
		var partnerResp struct {
			Id int64 `json:"id"`
		}
		if err := json.Unmarshal([]byte(body), &partnerResp); err != nil {
			t.Fatalf("decode partner: %v body=%s", err, body)
		}
		userID := partnerResp.Id + 100 // 测试用户 id 与合作方 id 错开即可
		if code, body := doAuthed(t, http.MethodPost, base+"/admin/roles", admin,
			`{"user_id":`+jsonNumber(userID)+`,"role":"partner","partner_id":`+jsonNumber(partnerResp.Id)+`}`); code != http.StatusOK {
			t.Fatalf("grant partner = %d body=%s", code, body)
		}
		if code, _ := doAuthed(t, http.MethodPost, base+"/admin/games/"+gameID+"/ownership", admin,
			`{"partner_id":`+jsonNumber(partnerResp.Id)+`}`); code != http.StatusOK {
			t.Fatalf("assign ownership %s = %d", gameID, code)
		}
		if code, _ := doAuthed(t, http.MethodPost, base+"/admin/games/"+gameID+"/publish", admin, ""); code != http.StatusOK {
			t.Fatalf("publish %s = %d", gameID, code)
		}
		return gameID, sign(claimsFor(userID, "partner-"+partnerName))
	}
	gameA, tokA := mkPartnerGame("区服游戏A", "区服厂A")
	_, tokB := mkPartnerGame("区服游戏B", "区服厂B")

	// 1. 公开列表：未建区服时空数组（游戏存在）
	code, body := doAuthed(t, http.MethodGet, base+"/games/"+gameA+"/servers", "", "")
	if code != http.StatusOK {
		t.Fatalf("public list servers status = %d body=%s", code, body)
	}
	if strings.TrimSpace(body) != `{"items":[]}` {
		t.Fatalf("empty servers body = %s", body)
	}

	// 2. 合作方 A 建区服（充值目标）：开放 + 维护中
	code, body = doAuthed(t, http.MethodPost, base+"/partner/games/"+gameA+"/servers", tokA,
		`{"name":"电信一区","region":"cn-east","sort":1}`)
	if code != http.StatusOK {
		t.Fatalf("create server status = %d body=%s", code, body)
	}
	var created struct {
		Id     int64  `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil || created.Id <= 0 {
		t.Fatalf("decode created server: %v body=%s", err, body)
	}
	if created.Status != "open" {
		t.Fatalf("default status = %s, want open", created.Status)
	}
	if code, _ := doAuthed(t, http.MethodPost, base+"/partner/games/"+gameA+"/servers", tokA,
		`{"name":"网通二区","region":"cn-north","status":"maintenance","sort":2}`); code != http.StatusOK {
		t.Fatalf("create maintenance server status = %d", code)
	}

	// 3. 跨合作方越权：B 操作 A 的游戏 → 403
	if code, _ = doAuthed(t, http.MethodPost, base+"/partner/games/"+gameA+"/servers", tokB,
		`{"name":"越权区"}`); code != http.StatusForbidden {
		t.Fatalf("cross-partner create status = %d, want 403", code)
	}
	if code, _ = doAuthed(t, http.MethodPatch, base+"/partner/games/"+gameA+"/servers/"+strconv.FormatInt(created.Id, 10), tokB,
		`{"status":"closed"}`); code != http.StatusForbidden {
		t.Fatalf("cross-partner patch status = %d, want 403", code)
	}
	if code, _ = doAuthed(t, http.MethodGet, base+"/partner/games/"+gameA+"/servers", tokB, ""); code != http.StatusForbidden {
		t.Fatalf("cross-partner list status = %d, want 403", code)
	}

	// 4. 合作方列表：全部状态可见
	code, body = doAuthed(t, http.MethodGet, base+"/partner/games/"+gameA+"/servers", tokA, "")
	if code != http.StatusOK {
		t.Fatalf("partner list status = %d body=%s", code, body)
	}
	var list struct {
		Items []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("decode partner list: %v body=%s", err, body)
	}
	if len(list.Items) != 2 || list.Items[0].Name != "电信一区" || list.Items[1].Status != "maintenance" {
		t.Fatalf("partner list mismatch: %+v", list.Items)
	}

	// 5. 改状态（开放 → 关闭）后玩家侧隐藏 closed
	if code, _ = doAuthed(t, http.MethodPatch, base+"/partner/games/"+gameA+"/servers/"+strconv.FormatInt(created.Id, 10), tokA,
		`{"status":"closed"}`); code != http.StatusOK {
		t.Fatalf("patch server status = %d", code)
	}
	code, body = doAuthed(t, http.MethodGet, base+"/games/"+gameA+"/servers", "", "")
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("decode public list: %v body=%s", err, body)
	}
	if len(list.Items) != 1 || list.Items[0].Name != "网通二区" {
		t.Fatalf("public list should hide closed: %+v", list.Items)
	}

	// 6. 匿名建区服 → 401
	if code, _ = doAuthed(t, http.MethodPost, base+"/partner/games/"+gameA+"/servers", "",
		`{"name":"anon"}`); code != http.StatusUnauthorized {
		t.Fatalf("anonymous create status = %d, want 401", code)
	}

	// 7. 审计：game_server_create / game_server_update
	if code, body = doAuthed(t, http.MethodGet, base+"/admin/audit", admin, ""); code != http.StatusOK ||
		!strings.Contains(body, "game_server_create") || !strings.Contains(body, "game_server_update") {
		t.Fatalf("audit list = %d body=%s", code, body)
	}
}
