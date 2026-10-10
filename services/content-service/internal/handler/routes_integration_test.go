package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/tappi/tappi/services/content-service/client"
	"github.com/tappi/tappi/services/content-service/internal/svc"
	"github.com/tappi/tappi/services/content-service/middleware"
	"github.com/tappi/tappi/services/content-service/model"
	"github.com/tappi/tappi/services/content-service/utils"

	_ "github.com/mattn/go-sqlite3"
	"github.com/zeromicro/go-zero/rest"
)

const testSecret = "content-handler-integration-secret"

const seedGuides = `[
	{"id":1,"game_id":"game-a","game_title":"Game A","title":"Guide One","content":"c1","summary":"s1","author_id":1001,"author_name":"alice","tags":["t1"],"is_published":true,"views":10,"likes":5,"created_at":"2020-01-01T00:00:00Z","updated_at":"2020-01-01T00:00:00Z"},
	{"id":2,"game_id":"game-b","game_title":"Game B","title":"Guide Two","content":"c2","author_id":2002,"author_name":"bob","tags":[],"is_published":false,"views":20,"likes":6,"created_at":"2020-01-02T00:00:00Z","updated_at":"2020-01-02T00:00:00Z"}
]`

const seedComments = `[
	{"id":1,"target_type":"guide","target_id":1,"user_id":2002,"user_name":"bob","content":"first!","likes":3,"created_at":"2020-01-01T00:00:00Z","updated_at":"2020-01-01T00:00:00Z"},
	{"id":2,"target_type":"guide","target_id":2,"user_id":1001,"user_name":"alice","content":"nice","likes":1,"created_at":"2020-01-02T00:00:00Z","updated_at":"2020-01-02T00:00:00Z"}
]`

// newHandlerTestServer 启动真实 go-zero rest 服务器（RegisterHandlers + 生产同款
// server.Use 认证中间件适配），game-catalog 客户端指向本地 stub。
func newHandlerTestServer(t *testing.T) (string, func(int64, string) string) {
	t.Helper()

	// game-catalog stub：GET /games/:id → {"game":{...,"title":"Stub Game Title"}}
	gameStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"game":{"id":"stub-1","title":"Stub Game Title","cover_image":"stub.png"}}`))
	}))
	t.Cleanup(gameStub.Close)

	// 种子装载：JSON 常量 → 临时文件 SQLite（与生产同款 DB 仓储）
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "content.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	var guides []*model.Guide
	if err := json.Unmarshal([]byte(seedGuides), &guides); err != nil {
		t.Fatalf("parse seed guides: %v", err)
	}
	var comments []*model.Comment
	if err := json.Unmarshal([]byte(seedComments), &comments); err != nil {
		t.Fatalf("parse seed comments: %v", err)
	}

	guideModel := model.NewGuideModel(db)
	if err := guideModel.CreateGuidesTable(); err != nil {
		t.Fatalf("create guides table: %v", err)
	}
	if err := guideModel.Seed(guides); err != nil {
		t.Fatalf("seed guides: %v", err)
	}
	commentModel := model.NewCommentModel(db)
	if err := commentModel.CreateCommentsTable(); err != nil {
		t.Fatalf("create comments table: %v", err)
	}
	if err := commentModel.Seed(comments); err != nil {
		t.Fatalf("seed comments: %v", err)
	}
	favoriteModel := model.NewFavoriteModel(db)
	if err := favoriteModel.CreateFavoritesTable(); err != nil {
		t.Fatalf("create favorites table: %v", err)
	}

	svcCtx := &svc.ServiceContext{
		GuideRepository:    guideModel,
		CommentRepository:  commentModel,
		FavoriteRepository: favoriteModel,
		Auth:               utils.NewAuth(testSecret),
		GameCatalogClient:  client.NewGameCatalogClient(gameStub.URL, 2*time.Second),
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	var conf rest.RestConf
	conf.Port = port
	server := rest.MustNewServer(conf)
	RegisterHandlers(server, svcCtx)
	// 与 content.go 生产装配保持一致：将 Handler 风格中间件适配为 rest.Middleware
	server.Use(func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			middleware.AuthMiddleware(svcCtx)(next).ServeHTTP(w, r)
		}
	})
	go server.Start()
	t.Cleanup(server.Stop)

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/api/v1/guides")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("test server did not become ready")
		}
		time.Sleep(20 * time.Millisecond)
	}

	sign := func(userId int64, username string) string {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, utils.JWTClaims{
			UserId:   userId,
			Username: username,
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		})
		signed, err := token.SignedString([]byte(testSecret))
		if err != nil {
			t.Fatalf("sign token: %v", err)
		}
		return signed
	}
	return base, sign
}

// do 便捷请求：携带可选 Bearer 令牌，返回状态码与响应体 JSON。
func do(t *testing.T, method, url, token, body string) (int, map[string]interface{}) {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	var payload map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	return resp.StatusCode, payload
}

func num(t *testing.T, m map[string]interface{}, key string) float64 {
	t.Helper()
	v, ok := m[key].(float64)
	if !ok {
		t.Fatalf("field %q missing or not a number: %#v", key, m[key])
	}
	return v
}

// --- 攻略接口 ---

func TestRoutes_ListGuidesAnonymous(t *testing.T) {
	base, _ := newHandlerTestServer(t)

	status, body := do(t, http.MethodGet, base+"/api/v1/guides", "", "")
	if status != http.StatusOK || num(t, body, "code") != 200 {
		t.Fatalf("list guides: status=%d body=%v", status, body)
	}
	// 匿名列表只返回已发布攻略（guide 2 是草稿，被隐藏）
	if total := num(t, body, "total"); total != 1 {
		t.Fatalf("total = %v, want 1 (published only)", body["total"])
	}
	games, _ := body["data"].([]interface{})
	if len(games) != 1 {
		t.Fatalf("data = %#v", body["data"])
	}
	first, _ := games[0].(map[string]interface{})
	if first["id"].(float64) != 1 {
		t.Fatalf("first guide = %#v", first)
	}
}

func TestRoutes_ListGuidesFiltered(t *testing.T) {
	base, sign := newHandlerTestServer(t)

	status, body := do(t, http.MethodGet, base+"/api/v1/guides?game_id=game-a&tag=t1", "", "")
	if status != http.StatusOK || num(t, body, "code") != 200 {
		t.Fatalf("filtered list: status=%d body=%v", status, body)
	}
	if total := num(t, body, "total"); total != 1 {
		t.Fatalf("filtered total = %v, want 1", body["total"])
	}

	// 匿名按作者过滤：作者只有草稿 → 0 条
	_, body = do(t, http.MethodGet, base+"/api/v1/guides?author_id=2002", "", "")
	if total := num(t, body, "total"); total != 0 {
		t.Fatalf("anonymous author filter total = %v, want 0 (draft hidden)", body["total"])
	}

	// 作者本人带 author_id 过滤：可见自己的草稿
	_, body = do(t, http.MethodGet, base+"/api/v1/guides?author_id=2002", sign(2002, "bob"), "")
	if total := num(t, body, "total"); total != 1 {
		t.Fatalf("author drafts total = %v, want 1", body["total"])
	}
	drafts, _ := body["data"].([]interface{})
	draft, _ := drafts[0].(map[string]interface{})
	if draft["id"].(float64) != 2 || draft["is_published"] != false {
		t.Fatalf("draft = %#v", draft)
	}
}

func TestRoutes_GetGuide(t *testing.T) {
	base, _ := newHandlerTestServer(t)

	status, body := do(t, http.MethodGet, base+"/api/v1/guides/1", "", "")
	if status != http.StatusOK || num(t, body, "code") != 200 {
		t.Fatalf("get guide: status=%d body=%v", status, body)
	}
	data, _ := body["data"].(map[string]interface{})
	if data["title"] != "Guide One" || data["author_name"] != "alice" {
		t.Fatalf("data = %#v", data)
	}

	_, body = do(t, http.MethodGet, base+"/api/v1/guides/999", "", "")
	if code := num(t, body, "code"); code != 404 {
		t.Fatalf("missing guide code = %v, want 404", body["code"])
	}
}

func TestRoutes_CreateGuideRequiresAuth(t *testing.T) {
	base, _ := newHandlerTestServer(t)

	status, body := do(t, http.MethodPost, base+"/api/v1/guides", "",
		`{"game_id":"stub-1","title":"t","content":"c"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous create status = %d, want 401", status)
	}
	if msg, _ := body["message"].(string); msg != "缺少认证令牌" {
		t.Fatalf("message = %#v", body["message"])
	}

	status, _ = do(t, http.MethodPost, base+"/api/v1/guides", "garbage",
		`{"game_id":"stub-1","title":"t","content":"c"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("bad token create status = %d, want 401", status)
	}
}

func TestRoutes_CreateGuideWithAuth(t *testing.T) {
	base, sign := newHandlerTestServer(t)
	token := sign(1001, "alice")

	status, body := do(t, http.MethodPost, base+"/api/v1/guides", token,
		`{"game_id":"stub-1","title":"New Guide","content":"body","tags":["new"]}`)
	if status != http.StatusOK || num(t, body, "code") != 200 {
		t.Fatalf("create guide: status=%d body=%v", status, body)
	}
	data, _ := body["data"].(map[string]interface{})
	// 标题来自 game-catalog stub（跨服务调用成功路径）
	if data["game_title"] != "Stub Game Title" {
		t.Fatalf("game_title = %#v, want stub title", data["game_title"])
	}
	if data["author_id"].(float64) != 1001 || data["author_name"] != "alice" {
		t.Fatalf("author = %#v", data)
	}
	if id := data["id"].(float64); id != 3 {
		t.Fatalf("created id = %v, want 3 (seed max 2 + 1)", data["id"])
	}

	// 非法 JSON → go-zero 参数解析错误
	status, _ = do(t, http.MethodPost, base+"/api/v1/guides", token, `{bad json`)
	if status != http.StatusBadRequest {
		t.Fatalf("malformed body status = %d, want 400", status)
	}
}

func TestRoutes_UpdateGuideAuthorOnly(t *testing.T) {
	base, sign := newHandlerTestServer(t)

	// 非作者更新 → 403 业务码
	_, body := do(t, http.MethodPut, base+"/api/v1/guides/1", sign(2002, "bob"),
		`{"title":"hacked"}`)
	if code := num(t, body, "code"); code != 403 {
		t.Fatalf("non-author update code = %v, want 403", body["code"])
	}

	// 作者更新 → 0
	status, body := do(t, http.MethodPut, base+"/api/v1/guides/1", sign(1001, "alice"),
		`{"title":"Renamed","content":"updated"}`)
	if status != http.StatusOK || num(t, body, "code") != 200 {
		t.Fatalf("author update: status=%d body=%v", status, body)
	}
}

func TestRoutes_PublishAndLikeGuide(t *testing.T) {
	base, sign := newHandlerTestServer(t)

	status, body := do(t, http.MethodPost, base+"/api/v1/guides/2/publish", sign(2002, "bob"), "")
	if status != http.StatusOK || num(t, body, "code") != 200 {
		t.Fatalf("publish: status=%d body=%v", status, body)
	}

	status, body = do(t, http.MethodPost, base+"/api/v1/guides/1/like", sign(1001, "alice"), "")
	if status != http.StatusOK || num(t, body, "code") != 200 {
		t.Fatalf("like: status=%d body=%v", status, body)
	}
	if likes := num(t, body, "likes"); likes != 6 {
		t.Fatalf("likes = %v, want 6 (seed 5 + 1)", body["likes"])
	}
}

// TestRoutes_FavoriteGuideFlow 收藏端到端：收藏 → 状态（本人/匿名）→
// 我的收藏列表 → 取消收藏幂等。含认证矩阵：无令牌写操作 401。
func TestRoutes_FavoriteGuideFlow(t *testing.T) {
	base, sign := newHandlerTestServer(t)

	// 无令牌 POST → 中间件 401
	status, _ := do(t, http.MethodPost, base+"/api/v1/guides/1/favorite", "", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("favorite without token status = %d, want 401", status)
	}

	// 收藏
	status, body := do(t, http.MethodPost, base+"/api/v1/guides/1/favorite", sign(1001, "alice"), "")
	if status != http.StatusOK || num(t, body, "code") != 200 {
		t.Fatalf("favorite: status=%d body=%v", status, body)
	}
	if body["favorited"] != true || num(t, body, "count") != 1 {
		t.Fatalf("favorited=%v count=%v, want true/1", body["favorited"], body["count"])
	}

	// 状态（本人）
	_, body = do(t, http.MethodGet, base+"/api/v1/guides/1/favorite", sign(1001, "alice"), "")
	if num(t, body, "code") != 200 || body["favorited"] != true || num(t, body, "count") != 1 {
		t.Fatalf("status self: body=%v", body)
	}

	// 状态（匿名）：只有总数
	_, body = do(t, http.MethodGet, base+"/api/v1/guides/1/favorite", "", "")
	if num(t, body, "code") != 200 || body["favorited"] != false || num(t, body, "count") != 1 {
		t.Fatalf("status anon: body=%v", body)
	}

	// 我的收藏列表
	_, body = do(t, http.MethodGet, base+"/api/v1/guides/favorites", sign(1001, "alice"), "")
	if num(t, body, "code") != 200 || num(t, body, "total") != 1 {
		t.Fatalf("favorites list: body=%v", body)
	}

	// 取消收藏（幂等再删一次）
	status, body = do(t, http.MethodDelete, base+"/api/v1/guides/1/favorite", sign(1001, "alice"), "")
	if status != http.StatusOK || num(t, body, "code") != 200 || body["favorited"] != false {
		t.Fatalf("unfavorite: status=%d body=%v", status, body)
	}
	status, body = do(t, http.MethodDelete, base+"/api/v1/guides/1/favorite", sign(1001, "alice"), "")
	if status != http.StatusOK || num(t, body, "code") != 200 {
		t.Fatalf("re-unfavorite: status=%d body=%v", status, body)
	}

	// 匿名拉我的收藏列表 → logic 层 401 信封（HTTP 仍 200）
	_, body = do(t, http.MethodGet, base+"/api/v1/guides/favorites", "", "")
	if num(t, body, "code") != http.StatusUnauthorized {
		t.Fatalf("favorites list anon: body=%v, want code 401", body)
	}
}

// TestRoutes_ParseErrors 覆盖各路由 httpx.Parse 失败分支（路径参数类型错误、
// 非法 JSON）→ go-zero 默认 400。写路由需带合法令牌（中间件先于 Parse 执行）。
// 注：handler 中 logic 返回 err 的分支不可达——所有 logic 均以业务码 envelope
// 返回（各 logic 无 return nil, err 路径），已核实。
func TestRoutes_ParseErrors(t *testing.T) {
	base, sign := newHandlerTestServer(t)
	token := sign(1001, "alice")

	cases := []struct{ name, method, url, body string }{
		{"list guides bad page", http.MethodGet, "/api/v1/guides?page=abc", ""},
		{"get guide bad id", http.MethodGet, "/api/v1/guides/abc", ""},
		{"create guide bad json", http.MethodPost, "/api/v1/guides", `{bad`},
		{"update guide bad id", http.MethodPut, "/api/v1/guides/abc", `{}`},
		{"publish bad id", http.MethodPost, "/api/v1/guides/abc/publish", ""},
		{"like guide bad id", http.MethodPost, "/api/v1/guides/abc/like", ""},
		{"list comments bad target", http.MethodGet, "/api/v1/comments?target_type=guide&target_id=abc", ""},
		{"create comment bad json", http.MethodPost, "/api/v1/comments", `{bad`},
		{"like comment bad id", http.MethodPost, "/api/v1/comments/abc/like", ""},
		{"delete comment bad id", http.MethodDelete, "/api/v1/comments/abc", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _ := do(t, tc.method, base+tc.url, token, tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("%s %s status = %d, want 400", tc.method, tc.url, status)
			}
		})
	}
}

// --- 评论接口 ---

func TestRoutes_CommentFlow(t *testing.T) {
	base, sign := newHandlerTestServer(t)
	alice := sign(1001, "alice")

	// 匿名列表
	status, body := do(t, http.MethodGet, base+"/api/v1/comments?target_type=guide&target_id=1", "", "")
	if status != http.StatusOK || num(t, body, "code") != 200 {
		t.Fatalf("list comments: status=%d body=%v", status, body)
	}
	comments, _ := body["data"].([]interface{})
	if len(comments) != 1 {
		t.Fatalf("comments = %#v", body["data"])
	}

	// 创建评论
	status, body = do(t, http.MethodPost, base+"/api/v1/comments", alice,
		`{"target_type":"guide","target_id":1,"content":"great"}`)
	if status != http.StatusOK || num(t, body, "code") != 200 {
		t.Fatalf("create comment: status=%d body=%v", status, body)
	}
	data, _ := body["data"].(map[string]interface{})
	if data["content"] != "great" || data["user_name"] != "alice" {
		t.Fatalf("created comment = %#v", data)
	}
	commentId := strconv.Itoa(int(data["id"].(float64)))

	// 评论点赞
	status, body = do(t, http.MethodPost, base+"/api/v1/comments/"+commentId+"/like", alice, "")
	if status != http.StatusOK || num(t, body, "code") != 200 {
		t.Fatalf("like comment: status=%d body=%v", status, body)
	}

	// 作者删除
	status, body = do(t, http.MethodDelete, base+"/api/v1/comments/"+commentId, alice, "")
	if status != http.StatusOK || num(t, body, "code") != 200 {
		t.Fatalf("delete comment: status=%d body=%v", status, body)
	}

	// 重复删除 → 404 业务码
	_, body = do(t, http.MethodDelete, base+"/api/v1/comments/"+commentId, alice, "")
	if code := num(t, body, "code"); code != 404 {
		t.Fatalf("re-delete code = %v, want 404", body["code"])
	}
}

// TestRoutes_PublishGuideNotFound 发布不存在的攻略 → 404 业务码
func TestRoutes_PublishGuideNotFound(t *testing.T) {
	base, sign := newHandlerTestServer(t)

	status, body := do(t, http.MethodPost, base+"/api/v1/guides/999/publish", sign(1001, "alice"), "")
	if status != http.StatusOK || num(t, body, "code") != 404 {
		t.Fatalf("publish not found: status=%d body=%v", status, body)
	}
}

// TestRoutes_UpdateGuideNotFound 更新不存在的攻略 → 404 业务码
func TestRoutes_UpdateGuideNotFound(t *testing.T) {
	base, sign := newHandlerTestServer(t)

	status, body := do(t, http.MethodPut, base+"/api/v1/guides/999", sign(1001, "alice"), `{"title":"x"}`)
	if status != http.StatusOK || num(t, body, "code") != 404 {
		t.Fatalf("update not found: status=%d body=%v", status, body)
	}
}

// TestRoutes_LikeGuideNotFound 点赞不存在的攻略 → 404 业务码
func TestRoutes_LikeGuideNotFound(t *testing.T) {
	base, sign := newHandlerTestServer(t)

	status, body := do(t, http.MethodPost, base+"/api/v1/guides/999/like", sign(1001, "alice"), "")
	if status != http.StatusOK || num(t, body, "code") != 404 {
		t.Fatalf("like not found: status=%d body=%v", status, body)
	}
}
