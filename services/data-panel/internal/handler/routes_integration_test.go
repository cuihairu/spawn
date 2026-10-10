package handler

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/tappi/tappi/services/data-panel/internal/config"
	"github.com/tappi/tappi/services/data-panel/internal/httperr"
	"github.com/tappi/tappi/services/data-panel/internal/svc"
	"github.com/tappi/tappi/services/data-panel/utils"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

const testSecret = "data-panel-handler-integration-secret"

// newHandlerTestServer 启动真实 go-zero rest 服务器（RegisterHandlers + 生产同款
// 认证中间件），SQLite 落在 t.TempDir()。
func newHandlerTestServer(t *testing.T) (string, func(int64) string) {
	t.Helper()

	// 与 data-panel.go 生产装配一致：错误信封映射
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	var conf rest.RestConf
	conf.Port = port

	var c config.Config
	c.RestConf = conf
	c.Auth.JWTSecret = testSecret
	c.MySQL.DataSource = "file:" + t.TempDir() + "/stats.db"

	svcCtx := svc.NewServiceContext(c)

	server := rest.MustNewServer(conf)
	RegisterHandlers(server, svcCtx)
	go server.Start()
	t.Cleanup(server.Stop)

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/api/v1/stats/users/1001/summary")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server not ready: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	sign := func(userId int64) string {
		now := time.Now()
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, utils.JWTClaims{
			UserId:   userId,
			Username: "tester",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
				IssuedAt:  jwt.NewNumericDate(now),
			},
		}).SignedString([]byte(testSecret))
		if err != nil {
			t.Fatalf("sign token: %v", err)
		}
		return token
	}
	return base, sign
}

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
		t.Fatalf("payload[%q] = %#v, want number", key, m[key])
	}
	return v
}

// TestRoutes_StatsFlow 四条战绩端点走真 handler：摄入（带令牌）→ 汇总 →
// 列表 → 单游戏；匿名摄入 401；坏 path 参数 400；不存在的游戏 404。
func TestRoutes_StatsFlow(t *testing.T) {
	base, sign := newHandlerTestServer(t)
	token := sign(1001)

	// 匿名摄入 → 401
	status, _ := do(t, http.MethodPost, base+"/api/v1/stats/records", "", `{"game_id":"g","matches":1}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("anon record status = %d, want 401", status)
	}

	// 摄入新游戏（归属恒取 JWT user_id，body 不收 user_id）
	status, body := do(t, http.MethodPost, base+"/api/v1/stats/records", token,
		`{"game_id":"game-hades","game_title":"Hades","matches":3,"wins":2,"kills":20,"deaths":5,"assists":4,"score":900,"rank_points":30}`)
	if status != http.StatusOK {
		t.Fatalf("record status = %d body=%v, want 200", status, body)
	}
	stat, ok := body["stat"].(map[string]interface{})
	if !ok || stat["game_id"] != "game-hades" {
		t.Fatalf("recorded stat = %v, want game-hades", body["stat"])
	}

	// 汇总：种子 2 局 + 新 1 局 = 3 局
	status, body = do(t, http.MethodGet, base+"/api/v1/stats/users/1001/summary", "", "")
	if status != http.StatusOK {
		t.Fatalf("summary status = %d", status)
	}
	summary, ok := body["summary"].(map[string]interface{})
	if !ok || num(t, summary, "game_count") != 3 {
		t.Fatalf("summary = %v, want game_count 3", body["summary"])
	}
	if uid, _ := summary["user_id"].(float64); uid != 1001 {
		t.Fatalf("summary user_id = %v, want 1001", summary["user_id"])
	}

	// 列表 + 分页钳制（limit=0 → 20）
	status, body = do(t, http.MethodGet, base+"/api/v1/stats/users/1001/games?limit=2&offset=0", "", "")
	if status != http.StatusOK || num(t, body, "total") != 3 || len(body["games"].([]interface{})) != 2 {
		t.Fatalf("games page1 = %v, want total 3 len 2", body)
	}
	status, body = do(t, http.MethodGet, base+"/api/v1/stats/users/1001/games?limit=0", "", "")
	if status != http.StatusOK || len(body["games"].([]interface{})) != 3 {
		t.Fatalf("games clamped = %v, want 3 rows", body)
	}

	// 单游戏详情；不存在的游戏 → 404
	status, body = do(t, http.MethodGet, base+"/api/v1/stats/users/1001/games/game-hades", "", "")
	if status != http.StatusOK {
		t.Fatalf("game detail status = %d body=%v", status, body)
	}
	status, _ = do(t, http.MethodGet, base+"/api/v1/stats/users/1001/games/game-none", "", "")
	if status != http.StatusNotFound {
		t.Fatalf("missing game status = %d, want 404", status)
	}
}

// TestRoutes_ParseErrors 坏 path/表单参数 → 400（go-zero 默认错误信封）。
func TestRoutes_ParseErrors(t *testing.T) {
	base, sign := newHandlerTestServer(t)
	token := sign(1001)

	cases := []struct {
		name, method, url, token, body string
	}{
		{"summary bad user", http.MethodGet, "/api/v1/stats/users/abc/summary", "", ""},
		{"games bad user", http.MethodGet, "/api/v1/stats/users/abc/games", "", ""},
		{"games bad limit", http.MethodGet, "/api/v1/stats/users/1/games?limit=abc", "", ""},
		{"detail bad user", http.MethodGet, "/api/v1/stats/users/abc/games/game-x", "", ""},
		{"record bad json", http.MethodPost, "/api/v1/stats/records", token, `{bad`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _ := do(t, tc.method, base+tc.url, tc.token, tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("%s %s status = %d, want 400", tc.method, tc.url, status)
			}
		})
	}
}
