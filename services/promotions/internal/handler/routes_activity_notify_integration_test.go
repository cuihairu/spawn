package handler

// GP-306 活动消息触发集成测试：活动发布 → 从 game-catalog 拉该游戏预约人群
// （S2S stub）→ community /internal/notifications 扇出 game_activity（站内信
// + realtime-hub 远程推送在 community 侧，另有独立验收）；无人预约静默降级、
// 依赖未配置时发布不受影响。

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
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

const gp306NotifyToken = "gp306-promotions-internal-token"

type communityRecorder struct {
	mu     sync.Mutex
	tokens []string
	reqs   [][]byte
	srv    *httptest.Server
}

func newCommunityRecorder() *communityRecorder {
	c := &communityRecorder{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/notifications", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.tokens = append(c.tokens, r.Header.Get("X-Internal-Token"))
		c.reqs = append(c.reqs, raw)
		c.mu.Unlock()
		if r.Header.Get("X-Internal-Token") != gp306NotifyToken {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		var body struct {
			UserIds []int64 `json:"user_ids"`
		}
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"created":` + strconv.Itoa(len(body.UserIds)) + `}`))
	})
	c.srv = httptest.NewServer(mux)
	return c
}

func (c *communityRecorder) Calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.reqs)
}

func (c *communityRecorder) Last() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.reqs) == 0 {
		return nil
	}
	return c.reqs[len(c.reqs)-1]
}

// newActivityNotifyServer 与 newActivityServer 同款装配：/me 角色权威存根
// 之外，catalog 存根另应 S2S 预约人群查询（game-a → 7,8；其余 → 空）。
func newActivityNotifyServer(t *testing.T, communityURL string) *testActivityEnv {
	t.Helper()

	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/me" {
			role := "player"
			if claims, ok := decodeJWT(r.Header.Get("Authorization")); ok && claims.UserId == 1 {
				role = "platform_admin"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"role": role, "partner_id": 1})
			return
		}
		if game, ok := strings.CutPrefix(r.URL.Path, "/internal/games/"); ok && strings.HasSuffix(game, "/reservation-users") {
			if r.Header.Get("X-Internal-Token") != gp306NotifyToken {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			gameID := strings.TrimSuffix(game, "/reservation-users")
			users := []int64{}
			if gameID == "game-a" {
				users = []int64{7, 8}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"game_id": gameID, "user_ids": users})
			return
		}
		http.NotFound(w, r)
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
	c.MySQL.DataSource = "file:" + t.TempDir() + "/activity-notify.db"
	c.Auth.JWTSecret = testJWTSecret
	c.GameCatalog.BaseURL = catalog.URL
	c.Notify.CommunityUrl = communityURL
	c.Notify.CatalogUrl = catalog.URL
	c.Notify.Token = gp306NotifyToken
	c.Reward.Secret = testRewardSecret

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

func TestRoutes_ActivityPublishNotify(t *testing.T) {
	community := newCommunityRecorder()
	defer community.srv.Close()
	env := newActivityNotifyServer(t, community.srv.URL)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})

	// 1. 建草稿（game-a：stub 受众 7,8）→ 发布 → game_activity 扇出一次
	code, body := do(t, http.MethodPost, env.base+"/admin/activities", admin, validActivityBody)
	if code != http.StatusOK {
		t.Fatalf("create activity = %d body=%s", code, body)
	}
	var resp struct {
		Activity struct {
			Id int64 `json:"id"`
		} `json:"activity"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil || resp.Activity.Id == 0 {
		t.Fatalf("parse create = %v body=%s", err, body)
	}
	id := resp.Activity.Id

	code, body = do(t, http.MethodPost, fmt.Sprintf("%s/admin/activities/%d/publish", env.base, id), admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"status":"published"`) {
		t.Fatalf("publish = %d body=%s", code, body)
	}
	if community.Calls() != 1 {
		t.Fatalf("publish should notify once, got %d calls", community.Calls())
	}
	raw := community.Last()
	var fanout struct {
		UserIds  []int64 `json:"user_ids"`
		Type     string  `json:"type"`
		TargetId int64   `json:"target_id"`
		Content  string  `json:"content"`
	}
	if err := json.Unmarshal(raw, &fanout); err != nil || fanout.Type != "game_activity" ||
		fanout.TargetId != id || len(fanout.UserIds) != 2 ||
		fanout.UserIds[0] != 7 || fanout.UserIds[1] != 8 || !strings.Contains(fanout.Content, "国庆倍率活动") {
		t.Fatalf("fanout = %s err=%v", raw, err)
	}

	// 2. 无预约人群的游戏（stub 返回空）→ 发布不扇出（静默降级）
	emptyBody := strings.Replace(validActivityBody, `"game_id":"game-a"`, `"game_id":"game-empty"`, 1)
	code, body = do(t, http.MethodPost, env.base+"/admin/activities", admin, emptyBody)
	if code != http.StatusOK {
		t.Fatalf("create empty-audience activity = %d body=%s", code, body)
	}
	var r2 struct {
		Activity struct {
			Id int64 `json:"id"`
		} `json:"activity"`
	}
	_ = json.Unmarshal([]byte(body), &r2)
	code, _ = do(t, http.MethodPost, fmt.Sprintf("%s/admin/activities/%d/publish", env.base, r2.Activity.Id), admin, "")
	if code != http.StatusOK {
		t.Fatalf("publish empty-audience = %d", code)
	}
	if community.Calls() != 1 {
		t.Fatalf("empty audience must not notify, got %d calls", community.Calls())
	}

	// 3. 未配置投递（CommunityUrl 空）→ 发布成功且零请求（noop 客户端）
	env2 := newActivityNotifyServer(t, "")
	admin2 := env2.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	code, body = do(t, http.MethodPost, env2.base+"/admin/activities", admin2, validActivityBody)
	if code != http.StatusOK {
		t.Fatalf("create on noop env = %d body=%s", code, body)
	}
	var r3 struct {
		Activity struct {
			Id int64 `json:"id"`
		} `json:"activity"`
	}
	_ = json.Unmarshal([]byte(body), &r3)
	code, body = do(t, http.MethodPost, fmt.Sprintf("%s/admin/activities/%d/publish", env2.base, r3.Activity.Id), admin2, "")
	if code != http.StatusOK || !strings.Contains(body, `"status":"published"`) {
		t.Fatalf("publish on noop env = %d body=%s", code, body)
	}
	if community.Calls() != 1 {
		t.Fatalf("noop env must not touch community, got %d calls", community.Calls())
	}
}
