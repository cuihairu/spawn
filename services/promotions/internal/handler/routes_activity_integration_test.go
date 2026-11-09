package handler

// GP-301 活动基础集成测试：状态机 draft→published→disabled→revise、起止
// 时间/时区、游戏/区服/渠道/人群、触发/限次/预算/叠加/奖励配置、发布版本
// 快照（不可变，旧版本保留）；管理面角色经 game-catalog /me 权威判定。

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/tappi/tappi/services/promotions/internal/config"
	"github.com/tappi/tappi/services/promotions/internal/logic"
	"github.com/tappi/tappi/services/promotions/internal/svc"
	"github.com/tappi/tappi/services/promotions/utils"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

const testJWTSecret = "promotions-integration-secret"

type testActivityEnv struct {
	base   string
	svcCtx *svc.ServiceContext
	sign   func(claims utils.JWTClaims) string
}

func newActivityServer(t *testing.T) *testActivityEnv {
	t.Helper()

	// game-catalog /me 存根：user 1 = platform_admin，其余 = player
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me" {
			http.NotFound(w, r)
			return
		}
		role := "player"
		if claims, ok := decodeJWT(r.Header.Get("Authorization")); ok && claims.UserId == 1 {
			role = "platform_admin"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"role": role, "partner_id": 1})
	}))
	t.Cleanup(catalog.Close)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	var c config.Config
	c.Port = port
	c.MySQL.DataSource = "file:" + t.TempDir() + "/activity.db"
	c.Auth.JWTSecret = testJWTSecret
	c.GameCatalog.BaseURL = catalog.URL

	svcCtx := svc.NewServiceContext(c)
	httpx.SetErrorHandlerCtx(logic.ErrorHandler)
	server := rest.MustNewServer(c.RestConf)
	RegisterHandlers(server, svcCtx)
	go server.Start()
	t.Cleanup(server.Stop)

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("promotions server not ready")
		}
		time.Sleep(20 * time.Millisecond)
	}

	env := &testActivityEnv{base: base, svcCtx: svcCtx}
	env.sign = func(claims utils.JWTClaims) string {
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := tok.SignedString([]byte(testJWTSecret))
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		return signed
	}
	return env
}

func decodeJWT(authHeader string) (*utils.JWTClaims, bool) {
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, false
	}
	tok, _, err := jwt.NewParser().ParseUnverified(strings.TrimSpace(authHeader[len("Bearer "):]), &utils.JWTClaims{})
	if err != nil {
		return nil, false
	}
	claims, ok := tok.Claims.(*utils.JWTClaims)
	return claims, ok
}

func do(t *testing.T, method, url, token, body string) (int, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, url, r)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

const validActivityBody = `{
	"name":"国庆倍率活动","game_id":"game-a","timezone":"Asia/Shanghai",
	"start_at":"2026-10-01T00:00:00+08:00","end_at":"2026-10-07T23:59:59+08:00",
	"server_ids":[1,2],"channel":"sandbox",
	"audience":{"type":"all"},
	"trigger":{"type":"order","min_amount_cents":1000},
	"limits":{"per_user_count":3,"budget_cents":100000,"global_count":1000},
	"stackable":false,"stack_group":"national-day",
	"rewards":{"type":"multiplier","rate_bp":1500}
}`

func TestRoutes_ActivityBasics(t *testing.T) {
	env := newActivityServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	// 1. 建活动（草稿）
	code, body := do(t, http.MethodPost, env.base+"/admin/activities", admin, validActivityBody)
	if code != http.StatusOK {
		t.Fatalf("create activity = %d body=%s", code, body)
	}
	var resp struct {
		Activity struct {
			Id     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"activity"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil || resp.Activity.Status != "draft" || resp.Activity.Id == 0 {
		t.Fatalf("parse create = %v body=%s", err, body)
	}
	id := resp.Activity.Id

	// 2. 草稿可更新
	code, _ = do(t, http.MethodPut, fmt.Sprintf("%s/admin/activities/%d", env.base, id), admin, validActivityBody)
	if code != http.StatusOK {
		t.Fatalf("update draft = %d", code)
	}

	// 3. 发布 → 版本快照 v1
	code, body = do(t, http.MethodPost, fmt.Sprintf("%s/admin/activities/%d/publish", env.base, id), admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"status":"published"`) || !strings.Contains(body, `"current_version":1`) {
		t.Fatalf("publish = %d body=%s", code, body)
	}
	code, body = do(t, http.MethodGet, fmt.Sprintf("%s/admin/activities/%d/versions", env.base, id), admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"version":1`) || !strings.Contains(body, `"snapshot"`) {
		t.Fatalf("versions = %d body=%s", code, body)
	}

	// 4. 已发布不可直接编辑（须 revise）
	code, _ = do(t, http.MethodPut, fmt.Sprintf("%s/admin/activities/%d", env.base, id), admin, validActivityBody)
	if code != http.StatusConflict {
		t.Fatalf("update published = %d, want 409", code)
	}

	// 5. disable → 停用；revise → 回草稿；改配置再发布 → v2（v1 快照不可变）
	code, _ = do(t, http.MethodPost, fmt.Sprintf("%s/admin/activities/%d/disable", env.base, id), admin, "")
	if code != http.StatusOK {
		t.Fatalf("disable = %d", code)
	}
	code, _ = do(t, http.MethodPost, fmt.Sprintf("%s/admin/activities/%d/revise", env.base, id), admin, "")
	if code != http.StatusOK {
		t.Fatalf("revise = %d", code)
	}
	code, _ = do(t, http.MethodPut, fmt.Sprintf("%s/admin/activities/%d", env.base, id), admin, validActivityBody)
	if code != http.StatusOK {
		t.Fatalf("update after revise = %d", code)
	}
	code, body = do(t, http.MethodPost, fmt.Sprintf("%s/admin/activities/%d/publish", env.base, id), admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"current_version":2`) {
		t.Fatalf("publish v2 = %d body=%s", code, body)
	}
	code, body = do(t, http.MethodGet, fmt.Sprintf("%s/admin/activities/%d/versions", env.base, id), admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"version":2`) {
		t.Fatalf("versions after v2 = %d body=%s", code, body)
	}

	// 6. 列表筛选 + 详情 + 越权
	code, body = do(t, http.MethodGet, env.base+"/admin/activities?game_id=game-a&status=published", admin, "")
	if code != http.StatusOK || !strings.Contains(body, fmt.Sprintf(`"id":%d`, id)) {
		t.Fatalf("list = %d body=%s", code, body)
	}
	code, _ = do(t, http.MethodPost, env.base+"/admin/activities", player, validActivityBody)
	if code != http.StatusForbidden {
		t.Fatalf("player create = %d, want 403", code)
	}
	code, _ = do(t, http.MethodGet, env.base+"/admin/activities", player, "")
	if code != http.StatusForbidden {
		t.Fatalf("player list = %d, want 403", code)
	}

	// 7. 校验：结束早于开始 400；不存在的活动 404；草稿不可停用 409
	badBody := strings.Replace(validActivityBody, `"start_at":"2026-10-01T00:00:00+08:00","end_at":"2026-10-07T23:59:59+08:00"`, `"start_at":"2026-10-07T23:59:59+08:00","end_at":"2026-10-01T00:00:00+08:00"`, 1)
	code, _ = do(t, http.MethodPost, env.base+"/admin/activities", admin, badBody)
	if code != http.StatusBadRequest {
		t.Fatalf("bad period = %d, want 400", code)
	}
	code, _ = do(t, http.MethodGet, env.base+"/admin/activities/999999", admin, "")
	if code != http.StatusNotFound {
		t.Fatalf("get missing = %d, want 404", code)
	}
	// 建个新草稿，disable 应 409
	code, body = do(t, http.MethodPost, env.base+"/admin/activities", admin, validActivityBody)
	var r2 struct {
		Activity struct {
			Id int64 `json:"id"`
		} `json:"activity"`
	}
	_ = json.Unmarshal([]byte(body), &r2)
	code, _ = do(t, http.MethodPost, fmt.Sprintf("%s/admin/activities/%d/disable", env.base, r2.Activity.Id), admin, "")
	if code != http.StatusConflict {
		t.Fatalf("disable draft = %d, want 409", code)
	}

	// 8. 审计含 create/update/publish/disable/revise
	code, body = do(t, http.MethodGet, env.base+"/admin/activity-audit?activity_id="+strconv.FormatInt(id, 10), admin, "")
	if code != http.StatusOK {
		t.Fatalf("audit = %d", code)
	}
	for _, action := range []string{"activity_create", "activity_update", "activity_publish", "activity_disable", "activity_revise"} {
		if !strings.Contains(body, action) {
			t.Fatalf("audit lacks %s: %s", action, body)
		}
	}
}
