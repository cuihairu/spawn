package logic

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"
)

// sampleGamesJSON 覆盖不同 genre/platform/tag/trending 的 8 款游戏，
// trending 降序即 id 列出顺序：g-rpg-1(90) g-rpg-2(85) g-fps-1(80) g-act-1(75)
// g-rpg-3(70) g-fps-2(65) g-act-2(60) g-puz-1(55)。
const sampleGamesJSON = `[
	{"id":"g-rpg-1","title":"Alpha RPG","description":"deep role playing","genres":["RPG"],"platforms":["PC"],"tags":["story"],"score":9.0,"trending_score":90,"release_date":"2024-01-01"},
	{"id":"g-rpg-2","title":"Beta Quest","description":"RPG adventure","genres":["RPG","Action"],"platforms":["PC","PS5"],"tags":["coop"],"score":8.0,"trending_score":85,"release_date":"2023-05-01"},
	{"id":"g-fps-1","title":"Gamma Strike","description":"fast shooter","genres":["FPS"],"platforms":["PC"],"tags":["pvp"],"score":8.8,"trending_score":80,"release_date":"2025-01-01"},
	{"id":"g-act-1","title":"Delta Action","description":"explosions","genres":["Action"],"platforms":["PS5"],"tags":["coop"],"score":7.5,"trending_score":75,"release_date":"2022-01-01"},
	{"id":"g-rpg-3","title":"Epsilon Tale","description":"story rich","genres":["RPG"],"platforms":["Switch"],"tags":["story"],"score":7.0,"trending_score":70,"release_date":"2024-06-01"},
	{"id":"g-fps-2","title":"Zeta Ops","description":"tactical shooter","genres":["FPS"],"platforms":["Xbox"],"tags":[],"score":6.5,"trending_score":65,"release_date":"2021-01-01"},
	{"id":"g-act-2","title":"Eta Rage","description":"brawler","genres":["Action"],"platforms":["PC"],"tags":["pvp"],"score":6.0,"trending_score":60,"release_date":"2020-01-01"},
	{"id":"g-puz-1","title":"Theta Puzzle","description":"brain teaser","genres":["Puzzle"],"platforms":["Switch"],"tags":["cozy"],"score":5.5,"trending_score":55,"release_date":"2019-01-01"}
]`

var trendingOrder = []string{"g-rpg-1", "g-rpg-2", "g-fps-1", "g-act-1", "g-rpg-3", "g-fps-2", "g-act-2", "g-puz-1"}

// newLogicSvcCtx 用样本数据文件构造仅含 GameRepository 的 ServiceContext
// （logic 层不触达 Config/Auth）。
func newLogicSvcCtx(t *testing.T) *svc.ServiceContext {
	t.Helper()
	path := filepath.Join(t.TempDir(), "games.json")
	if err := writeFile(path, sampleGamesJSON); err != nil {
		t.Fatalf("write sample games: %v", err)
	}
	repo, err := model.NewGameRepository(path)
	if err != nil {
		t.Fatalf("NewGameRepository: %v", err)
	}
	return &svc.ServiceContext{GameRepository: repo}
}

func gameIds(games []types.Game) []string {
	ids := make([]string, 0, len(games))
	for _, g := range games {
		ids = append(ids, g.Id)
	}
	return ids
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// asAPIError 断言 err 为 logic 层 apiError 并返回 (code, message)。
func asAPIError(t *testing.T, err error) (int, string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ae *apiError
	if !errors.As(err, &ae) {
		t.Fatalf("err %v is not *apiError", err)
	}
	return ae.StatusCode(), ae.Error()
}

// --- apiError ---

func TestAPIError(t *testing.T) {
	err := newAPIError(404, "游戏不存在")
	var ae *apiError
	if !errors.As(err, &ae) {
		t.Fatal("newAPIError must produce *apiError")
	}
	if ae.StatusCode() != 404 {
		t.Fatalf("StatusCode = %d, want 404", ae.StatusCode())
	}
	if ae.Error() != "游戏不存在" {
		t.Fatalf("Error = %q", ae.Error())
	}
}

// --- mapper ---

func TestToTypesGame_Nil(t *testing.T) {
	got := toTypesGame(nil)
	if got.Id != "" || got.Title != "" || got.Genres != nil || got.Platforms != nil || got.Tags != nil {
		t.Fatalf("toTypesGame(nil) = %+v, want zero value", got)
	}
}

func TestToTypesGame_MapsAllFieldsAndCopiesSlices(t *testing.T) {
	src := &model.Game{
		Id: "g-1", Title: "T", Description: "D",
		Genres: []string{"RPG", "Action"}, Platforms: []string{"PC"},
		ReleaseDate: "2024-01-01", Developer: "Dev", Publisher: "Pub",
		Tags: []string{"a", "b"}, Score: 9.1, TrendingScore: 88, CoverImage: "c.png",
	}
	got := toTypesGame(src)

	if got.Id != "g-1" || got.Title != "T" || got.Description != "D" ||
		got.ReleaseDate != "2024-01-01" || got.Developer != "Dev" ||
		got.Publisher != "Pub" || got.Score != 9.1 || got.TrendingScore != 88 ||
		got.CoverImage != "c.png" {
		t.Fatalf("scalar fields not mapped: %+v", got)
	}
	if !reflect.DeepEqual(got.Genres, []string{"RPG", "Action"}) ||
		!reflect.DeepEqual(got.Platforms, []string{"PC"}) ||
		!reflect.DeepEqual(got.Tags, []string{"a", "b"}) {
		t.Fatalf("slice fields not mapped: %+v", got)
	}

	// 深拷贝：改动映射结果不得影响仓储内部数据
	got.Genres[0] = "MUTATED"
	got.Tags[0] = "MUTATED"
	if src.Genres[0] != "RPG" || src.Tags[0] != "a" {
		t.Fatalf("mapper must copy slices, source mutated: %+v", src)
	}
}

func TestToTypesGames_EmptyAndNil(t *testing.T) {
	if got := toTypesGames(nil); len(got) != 0 {
		t.Fatalf("toTypesGames(nil) len = %d, want 0", len(got))
	}
	if got := toTypesGames([]*model.Game{}); len(got) != 0 {
		t.Fatalf("toTypesGames(empty) len = %d, want 0", len(got))
	}
}

// --- CreateGame ---

func TestCreateGameValidation(t *testing.T) {
	cases := []struct {
		name    string
		req     types.CreateGameRequest
		wantMsg string
	}{
		{"empty title", types.CreateGameRequest{Description: "d", Genres: []string{"RPG"}, Platforms: []string{"PC"}}, "标题和描述不能为空"},
		{"blank title", types.CreateGameRequest{Title: "   ", Description: "d", Genres: []string{"RPG"}, Platforms: []string{"PC"}}, "标题和描述不能为空"},
		{"empty description", types.CreateGameRequest{Title: "t", Genres: []string{"RPG"}, Platforms: []string{"PC"}}, "标题和描述不能为空"},
		{"blank description", types.CreateGameRequest{Title: "t", Description: "  ", Genres: []string{"RPG"}, Platforms: []string{"PC"}}, "标题和描述不能为空"},
		{"no genres", types.CreateGameRequest{Title: "t", Description: "d", Platforms: []string{"PC"}}, "至少需要一个游戏类型"},
		{"no platforms", types.CreateGameRequest{Title: "t", Description: "d", Genres: []string{"RPG"}}, "至少需要一个平台信息"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svcCtx := newLogicSvcCtx(t)
			l := NewCreateGameLogic(t.Context(), svcCtx)
			_, err := l.CreateGame(&tc.req)
			code, msg := asAPIError(t, err)
			if code != 400 {
				t.Fatalf("code = %d, want 400", code)
			}
			if msg != tc.wantMsg {
				t.Fatalf("message = %q, want %q", msg, tc.wantMsg)
			}
		})
	}
}

func TestCreateGameSuccess(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	l := NewCreateGameLogic(t.Context(), svcCtx)

	resp, err := l.CreateGame(&types.CreateGameRequest{
		Title:       "  New Game  ",
		Description: "  A fresh entry  ",
		Genres:      []string{"RPG"},
		Platforms:   []string{"PC"},
		ReleaseDate: "2026-01-01",
		Developer:   "Dev",
		Publisher:   "Pub",
		Tags:        []string{"new"},
		Score:       8.8,
		CoverImage:  "cover.png",
	})
	if err != nil {
		t.Fatalf("CreateGame: %v", err)
	}

	g := resp.Game
	if g.Title != "New Game" || g.Description != "A fresh entry" {
		t.Fatalf("title/description must be trimmed: %+v", g)
	}
	if len(g.Id) <= len("game-") || g.Id[:5] != "game-" {
		t.Fatalf("id = %q, want game-<uuid> prefix", g.Id)
	}
	if g.TrendingScore != 60 {
		t.Fatalf("trending score = %d, want 60 (create default)", g.TrendingScore)
	}
	if !equalStrings(g.Genres, []string{"RPG"}) || !equalStrings(g.Platforms, []string{"PC"}) {
		t.Fatalf("genres/platforms not passed through: %+v", g)
	}

	// 已持久化，可按新 id 查询
	dl := NewGetGameDetailLogic(t.Context(), svcCtx)
	detail, err := dl.GetGameDetail(&types.GetGameDetailRequest{Id: g.Id})
	if err != nil || detail.Game.Title != "New Game" {
		t.Fatalf("created game not retrievable: %+v, %v", detail, err)
	}
}

// --- GetGameDetail ---

func TestGetGameDetail_Found(t *testing.T) {
	l := NewGetGameDetailLogic(t.Context(), newLogicSvcCtx(t))

	resp, err := l.GetGameDetail(&types.GetGameDetailRequest{Id: "g-rpg-1"})
	if err != nil {
		t.Fatalf("GetGameDetail: %v", err)
	}
	g := resp.Game
	if g.Id != "g-rpg-1" || g.Title != "Alpha RPG" || g.Score != 9.0 || g.TrendingScore != 90 {
		t.Fatalf("detail = %+v", g)
	}
	if !equalStrings(g.Genres, []string{"RPG"}) {
		t.Fatalf("genres = %v", g.Genres)
	}
}

func TestGetGameDetail_NotFound(t *testing.T) {
	l := NewGetGameDetailLogic(t.Context(), newLogicSvcCtx(t))

	_, err := l.GetGameDetail(&types.GetGameDetailRequest{Id: "missing"})
	code, msg := asAPIError(t, err)
	if code != 404 {
		t.Fatalf("code = %d, want 404", code)
	}
	if msg != "游戏不存在" {
		t.Fatalf("message = %q", msg)
	}
}

// --- ListGames ---

func TestListGames_DefaultsAndOrder(t *testing.T) {
	l := NewListGamesLogic(t.Context(), newLogicSvcCtx(t))

	// 零值请求：Sort 为空走默认热度排序，Limit=0 兜底为 20（全量返回）
	resp, err := l.ListGames(&types.ListGamesRequest{})
	if err != nil {
		t.Fatalf("ListGames: %v", err)
	}
	if resp.Total != 8 {
		t.Fatalf("total = %d, want 8", resp.Total)
	}
	if !equalStrings(gameIds(resp.Games), trendingOrder) {
		t.Fatalf("default order = %v, want trending desc", gameIds(resp.Games))
	}
}

func TestListGames_Filters(t *testing.T) {
	cases := []struct {
		name    string
		req     types.ListGamesRequest
		wantIds []string
	}{
		{"keyword matches title", types.ListGamesRequest{Keyword: "alpha"}, []string{"g-rpg-1"}},
		{"keyword case-insensitive on description", types.ListGamesRequest{Keyword: "SHOOTER"}, []string{"g-fps-1", "g-fps-2"}},
		{"genre case-insensitive", types.ListGamesRequest{Genre: "rpg"}, []string{"g-rpg-1", "g-rpg-2", "g-rpg-3"}},
		{"platform", types.ListGamesRequest{Platform: "PS5"}, []string{"g-rpg-2", "g-act-1"}},
		{"tag", types.ListGamesRequest{Tag: "coop"}, []string{"g-rpg-2", "g-act-1"}},
		{"combined genre+platform", types.ListGamesRequest{Genre: "RPG", Platform: "PC"}, []string{"g-rpg-1", "g-rpg-2"}},
		{"filter miss", types.ListGamesRequest{Genre: "Moba"}, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewListGamesLogic(t.Context(), newLogicSvcCtx(t))
			resp, err := l.ListGames(&tc.req)
			if err != nil {
				t.Fatalf("ListGames: %v", err)
			}
			if resp.Total != len(tc.wantIds) {
				t.Fatalf("total = %d, want %d", resp.Total, len(tc.wantIds))
			}
			if !equalStrings(gameIds(resp.Games), tc.wantIds) {
				t.Fatalf("ids = %v, want %v", gameIds(resp.Games), tc.wantIds)
			}
		})
	}
}

func TestListGames_Pagination(t *testing.T) {
	l := NewListGamesLogic(t.Context(), newLogicSvcCtx(t))

	resp, err := l.ListGames(&types.ListGamesRequest{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("ListGames: %v", err)
	}
	if resp.Total != 8 {
		t.Fatalf("total = %d, want 8", resp.Total)
	}
	if !equalStrings(gameIds(resp.Games), []string{"g-fps-1", "g-act-1"}) {
		t.Fatalf("page = %v", gameIds(resp.Games))
	}
	if resp.Limit != 2 || resp.Offset != 2 {
		t.Fatalf("limit/offset = %d/%d, want 2/2", resp.Limit, resp.Offset)
	}

	// Limit<=0 兜底 20：未裁剪时返回全量（若不兜底则应返回空页）
	zero, err := l.ListGames(&types.ListGamesRequest{Limit: 0})
	if err != nil {
		t.Fatalf("ListGames: %v", err)
	}
	if len(zero.Games) != 8 {
		t.Fatalf("limit=0 must fall back to 20, got %d games", len(zero.Games))
	}

	// 越界 offset：空页但 total 不变
	far, err := l.ListGames(&types.ListGamesRequest{Limit: 5, Offset: 50})
	if err != nil {
		t.Fatalf("ListGames: %v", err)
	}
	if len(far.Games) != 0 || far.Total != 8 {
		t.Fatalf("offset beyond range: games=%d total=%d", len(far.Games), far.Total)
	}
}

func TestListGames_SortByReleaseDate(t *testing.T) {
	l := NewListGamesLogic(t.Context(), newLogicSvcCtx(t))

	resp, err := l.ListGames(&types.ListGamesRequest{Sort: "release_date"})
	if err != nil {
		t.Fatalf("ListGames: %v", err)
	}
	want := []string{"g-fps-1", "g-rpg-3", "g-rpg-1", "g-rpg-2", "g-act-1", "g-fps-2", "g-act-2", "g-puz-1"}
	if !equalStrings(gameIds(resp.Games), want) {
		t.Fatalf("release order = %v, want %v", gameIds(resp.Games), want)
	}
}

// --- GetFeaturedGames ---

func TestGetFeaturedGames(t *testing.T) {
	cases := []struct {
		name    string
		limit   int64
		wantIds []string
	}{
		{"top 3", 3, []string{"g-rpg-1", "g-rpg-2", "g-fps-1"}},
		{"limit 0 falls back to 6", 0, trendingOrder[:6]},
		{"negative limit falls back to 6", -5, trendingOrder[:6]},
		{"oversized limit clamped to catalog", 99, trendingOrder},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewGetFeaturedGamesLogic(t.Context(), newLogicSvcCtx(t))
			resp, err := l.GetFeaturedGames(&types.FeaturedGamesRequest{Limit: tc.limit})
			if err != nil {
				t.Fatalf("GetFeaturedGames: %v", err)
			}
			if !equalStrings(gameIds(resp.Games), tc.wantIds) {
				t.Fatalf("ids = %v, want %v", gameIds(resp.Games), tc.wantIds)
			}
		})
	}
}

// --- GetRecommendations ---

func TestGetRecommendations_GenreParsing(t *testing.T) {
	cases := []struct {
		name    string
		req     types.RecommendationsRequest
		wantIds []string
	}{
		{
			// limit=0 兜底 5：命中 RPG/FPS 的恰好 5 款，按热度序
			name:    "comma separated with blanks",
			req:     types.RecommendationsRequest{Genres: "RPG, , FPS"},
			wantIds: []string{"g-rpg-1", "g-rpg-2", "g-fps-1", "g-rpg-3", "g-fps-2"},
		},
		{
			// 全空白片段解析为空列表 → 全量池
			name:    "blank genres means no filter",
			req:     types.RecommendationsRequest{Genres: "   ,  "},
			wantIds: trendingOrder[:5],
		},
		{
			name:    "no matching genre falls back to full pool",
			req:     types.RecommendationsRequest{Genres: "Moba", Limit: 3},
			wantIds: []string{"g-rpg-1", "g-rpg-2", "g-fps-1"},
		},
		{
			name:    "no genres uses popularity order",
			req:     types.RecommendationsRequest{Limit: 4},
			wantIds: trendingOrder[:4],
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewGetRecommendationsLogic(t.Context(), newLogicSvcCtx(t))
			resp, err := l.GetRecommendations(&tc.req)
			if err != nil {
				t.Fatalf("GetRecommendations: %v", err)
			}
			if !equalStrings(gameIds(resp.Games), tc.wantIds) {
				t.Fatalf("ids = %v, want %v", gameIds(resp.Games), tc.wantIds)
			}
		})
	}
}

func TestGetRecommendations_UserRotationDeterministic(t *testing.T) {
	l := NewGetRecommendationsLogic(t.Context(), newLogicSvcCtx(t))

	req := &types.RecommendationsRequest{UserId: "user-42", Limit: 5}
	first, err := l.GetRecommendations(req)
	if err != nil {
		t.Fatalf("GetRecommendations: %v", err)
	}
	second, err := l.GetRecommendations(req)
	if err != nil {
		t.Fatalf("GetRecommendations: %v", err)
	}

	// 同一用户结果确定（轮转起点由 userId 哈希决定）
	if !equalStrings(gameIds(first.Games), gameIds(second.Games)) {
		t.Fatal("same user must get identical recommendations")
	}
	if len(first.Games) != 5 {
		t.Fatalf("len = %d, want 5", len(first.Games))
	}
	// 结果必须来自全量池
	pool := map[string]bool{}
	for _, id := range trendingOrder {
		pool[id] = true
	}
	for _, id := range gameIds(first.Games) {
		if !pool[id] {
			t.Fatalf("recommended id %q not in catalog pool", id)
		}
	}
}

// --- 并发（logic 层无状态，借 -race 验证仓储共享访问安全） ---

func TestLogic_ConcurrentReads(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := NewListGamesLogic(t.Context(), svcCtx).ListGames(&types.ListGamesRequest{Limit: 3}); err != nil {
				t.Errorf("ListGames: %v", err)
			}
			if _, err := NewGetFeaturedGamesLogic(t.Context(), svcCtx).GetFeaturedGames(&types.FeaturedGamesRequest{Limit: 2}); err != nil {
				t.Errorf("GetFeaturedGames: %v", err)
			}
			if _, err := NewGetRecommendationsLogic(t.Context(), svcCtx).GetRecommendations(&types.RecommendationsRequest{UserId: "u", Limit: 3}); err != nil {
				t.Errorf("GetRecommendations: %v", err)
			}
		}()
	}
	wg.Wait()
}
