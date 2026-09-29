package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestRoutes_GetGameDetail 读路由：默认种子库详情 + 404。
func TestRoutes_GetGameDetail(t *testing.T) {
	base, _ := newAuthedTestServer(t)

	resp, err := http.Get(base + "/games")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	body := readAll(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d body=%s", resp.StatusCode, body)
	}
	var list struct {
		Games []struct {
			Id    string `json:"id"`
			Title string `json:"title"`
		} `json:"games"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("decode list: %v (%s)", err, body)
	}
	if list.Total == 0 || len(list.Games) == 0 {
		t.Fatalf("expected seeded games, body=%s", body)
	}
	firstId := list.Games[0].Id

	// 详情（匿名公开）
	resp, err = http.Get(base + "/games/" + firstId)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	body = readAll(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"id":"`+firstId+`"`) {
		t.Fatalf("detail status=%d body=%s", resp.StatusCode, body)
	}

	// 不存在 → 404（apiError.StatusCode 映射）
	resp, err = http.Get(base + "/games/no-such-game")
	if err != nil {
		t.Fatalf("missing detail: %v", err)
	}
	body = readAll(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing detail status = %d, want 404 body=%s", resp.StatusCode, body)
	}
}

// TestRoutes_GetFeaturedGames featured 路由 + limit 语义。
func TestRoutes_GetFeaturedGames(t *testing.T) {
	base, _ := newAuthedTestServer(t)

	resp, err := http.Get(base + "/games/featured?limit=2")
	if err != nil {
		t.Fatalf("featured: %v", err)
	}
	body := readAll(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("featured status = %d body=%s", resp.StatusCode, body)
	}
	var featured struct {
		Games []json.RawMessage `json:"games"`
	}
	if err := json.Unmarshal([]byte(body), &featured); err != nil {
		t.Fatalf("decode featured: %v (%s)", err, body)
	}
	if len(featured.Games) != 2 {
		t.Fatalf("games = %d, want 2 (%s)", len(featured.Games), body)
	}

	// 默认 limit
	resp, err = http.Get(base + "/games/featured")
	if err != nil {
		t.Fatalf("featured default: %v", err)
	}
	body = readAll(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("featured default status = %d", resp.StatusCode)
	}
}

// TestRoutes_GetRecommendations 推荐路由：genres 解析与 limit 缺省。
func TestRoutes_GetRecommendations(t *testing.T) {
	base, _ := newAuthedTestServer(t)

	// 带 genres 过滤 + 显式 limit
	resp, err := http.Get(base + "/games/recommendations?userId=u1&genres=RPG,FPS&limit=3")
	if err != nil {
		t.Fatalf("recommendations: %v", err)
	}
	body := readAll(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("recommendations status = %d body=%s", resp.StatusCode, body)
	}
	var reco struct {
		Games []struct {
			Id     string   `json:"id"`
			Genres []string `json:"genres"`
		} `json:"games"`
	}
	if err := json.Unmarshal([]byte(body), &reco); err != nil {
		t.Fatalf("decode recommendations: %v (%s)", err, body)
	}
	if len(reco.Games) == 0 {
		t.Fatalf("expected recommendations, body=%s", body)
	}
	for _, g := range reco.Games {
		if len(g.Genres) == 0 {
			t.Fatalf("genre filter leaked: %s (%s)", g.Id, body)
		}
		matched := false
		for _, genre := range g.Genres {
			if genre == "RPG" || genre == "FPS" {
				matched = true
			}
		}
		if !matched {
			t.Fatalf("game %s genres %v outside filter", g.Id, g.Genres)
		}
	}

	// 空参数：limit 缺省 5、genres 空 → 不报错
	resp, err = http.Get(base + "/games/recommendations")
	if err != nil {
		t.Fatalf("recommendations default: %v", err)
	}
	body = readAll(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("recommendations default status = %d body=%s", resp.StatusCode, body)
	}

	// 非法 limit → Parse 400
	resp, err = http.Get(base + "/games/recommendations?limit=abc")
	if err != nil {
		t.Fatalf("bad limit: %v", err)
	}
	readAll(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad limit status = %d, want 400", resp.StatusCode)
	}
}

// TestRoutes_ListGamesQueryErrors 列表/精选路由的坏 query → Parse 400。
func TestRoutes_ListGamesQueryErrors(t *testing.T) {
	base, _ := newAuthedTestServer(t)

	for _, path := range []string{"/games?limit=abc", "/games?offset=abc", "/games/featured?limit=abc"} {
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		readAll(t, resp)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("GET %s status = %d, want 400", path, resp.StatusCode)
		}
	}

	// 合法过滤参数不报错
	resp, err := http.Get(base + "/games?keyword=a&genre=RPG&platform=PC&tag=story&limit=5&offset=0&sort=popularity")
	if err != nil {
		t.Fatalf("filtered list: %v", err)
	}
	body := readAll(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("filtered list status = %d body=%s", resp.StatusCode, body)
	}
}

// TestRoutes_PostGamesParseError 补 POST /games 的解析失败分支（缺必填字段）。
func TestRoutes_PostGamesParseError(t *testing.T) {
	base, sign := newAuthedTestServer(t)

	req, _ := http.NewRequest(http.MethodPost, base+"/games", strings.NewReader(`{bad json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sign(validRouteClaims()))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("bad json POST: %v", err)
	}
	body := readAll(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad json POST status = %d, want 400 body=%s", resp.StatusCode, body)
	}

	// 字段齐全但标题/描述为空 → logic 400
	req2, _ := http.NewRequest(http.MethodPost, base+"/games", strings.NewReader(
		`{"title":"","description":"","genres":[],"platforms":[],"release_date":"2001-01-01","developer":"d","publisher":"p","tags":[],"score":0,"cover_image":"c.png"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+sign(validRouteClaims()))
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("empty POST: %v", err)
	}
	body2 := readAll(t, resp2)
	if resp2.StatusCode != http.StatusBadRequest || !strings.Contains(body2, "标题和描述不能为空") {
		t.Fatalf("empty POST status = %d body=%s", resp2.StatusCode, body2)
	}
}
