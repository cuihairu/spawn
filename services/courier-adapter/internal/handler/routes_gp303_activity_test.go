package handler

// GP-303 回流判定源上报链路集成测试：游戏会话签发成功即向 promotions 上报
// 账号×游戏活跃事实（S2S，X-Internal-Token，outbox 先落再同步首投）。
// PROMOTIONS_URL 未配置 = 上报器不装配（会话主流程照常）；已配置 = 会话
// 签发 → promotions 收到一次事实（账号 = 平台 user_id，游戏 = spawn game id）。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/tappi/tappi/services/courier-adapter/internal/config"
)

// promotionsRecorder 记录 /internal/player-activity 收到的上报。
type promotionsRecorder struct {
	mu     sync.Mutex
	token  string
	authz  string
	bodies []string
}

func (p *promotionsRecorder) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.token = r.Header.Get("X-Internal-Token")
		p.authz = r.Header.Get("Authorization")
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		p.bodies = append(p.bodies, string(buf[:n]))
		p.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}
}

func (p *promotionsRecorder) snapshot() (string, string, []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.token, p.authz, append([]string(nil), p.bodies...)
}

// setupGameMapping 以 operator 建好 g1 接入（login 能力开）。
func setupGameMapping(t *testing.T, env *testEnv) {
	t.Helper()
	operator := signToken(t, 42, "ops")
	if code, body := doJSON(t, http.MethodPut, env.base+"/admin/games/g1/mapping", operator,
		`{"courierGameId":"cg1","env":"prod","partnerId":1,"capabilities":{"login":true}}`); code != http.StatusOK {
		t.Fatalf("setup g1 mapping status = %d body=%s", code, body)
	}
}

func TestRoutes_GameSessionReportsPlayerActivity(t *testing.T) {
	rec := &promotionsRecorder{}
	prom := httptest.NewServer(rec.handler())
	t.Cleanup(prom.Close)

	env := newTestEnvCfg(t, func(cfg *config.Config) {
		cfg.Promotions.BaseURL = prom.URL
		cfg.Promotions.InternalToken = "gp303-s2s-token"
	})
	base := env.base
	player := signToken(t, 77, "player")
	setupGameMapping(t, env)

	// 会话签发成功 → 活跃事实已上报（先落 outbox 再同步首投）
	if code, body := doJSON(t, http.MethodPost, base+"/games/g1/session", player, ""); code != http.StatusOK {
		t.Fatalf("game session status = %d body=%s", code, body)
	}
	token, authz, bodies := rec.snapshot()
	if len(bodies) != 1 {
		t.Fatalf("promotions 收到 %d 次上报, want 1（会话签发 = 一次活跃事实）: %v", len(bodies), bodies)
	}
	if token != "gp303-s2s-token" {
		t.Fatalf("X-Internal-Token = %q, want gp303-s2s-token", token)
	}
	if authz != "" {
		t.Fatalf("S2S 上报不应透传玩家 JWT, got %q", authz)
	}
	var got struct {
		AccountId string `json:"account_id"`
		GameId    string `json:"game_id"`
		ActiveAt  string `json:"active_at"`
	}
	if err := json.Unmarshal([]byte(bodies[0]), &got); err != nil {
		t.Fatalf("decode report: %v body=%s", err, bodies[0])
	}
	if got.AccountId != "77" || got.GameId != "g1" || !strings.Contains(got.ActiveAt, "T") {
		t.Fatalf("report = %+v, want account 77 / game g1 / RFC3339 时刻", got)
	}

	// 二次会话同样上报（每次登录都是一条活跃事实）
	if code, _ := doJSON(t, http.MethodPost, base+"/games/g1/session", player, ""); code != http.StatusOK {
		t.Fatalf("second session status = %d", code)
	}
	_, _, bodies = rec.snapshot()
	if len(bodies) != 2 {
		t.Fatalf("第二次会话后上报次数 = %d, want 2", len(bodies))
	}
}

func TestRoutes_GameSessionActivityUnconfiguredNoop(t *testing.T) {
	// PROMOTIONS_URL 未配置：上报器不装配，会话主流程照常
	env := newTestEnv(t)
	player := signToken(t, 77, "player")
	setupGameMapping(t, env)
	if code, body := doJSON(t, http.MethodPost, env.base+"/games/g1/session", player, ""); code != http.StatusOK {
		t.Fatalf("unconfigured session status = %d body=%s", code, body)
	}
}
