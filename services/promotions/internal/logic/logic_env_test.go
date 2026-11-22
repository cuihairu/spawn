package logic

// logic 层公共测试环境：真实 SQLite 模型 + httptest game-catalog 角色桩。
// 角色权威在 platformRole/requireAdminRole（GET /me），故所有管理面用例
// 都需一个返回 role 的桩；不可达桩用于触发 502 臂。

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/promotions/internal/notify"
	"github.com/tappi/tappi/services/promotions/internal/svc"
	"github.com/tappi/tappi/services/promotions/model"

	_ "github.com/mattn/go-sqlite3"
)

// logicEnv 测试环境（db 连接保留，供闭库错误臂使用）。
type logicEnv struct {
	db      *sql.DB
	sc      *svc.ServiceContext
	catalog string
}

// newLogicEnv 建临时 SQLite + 三模型建表，返回环境。catalogURL 为角色桩
// 地址（空 = 不建桩，requireAdminRole 必然失败）。
func newLogicEnv(t *testing.T, catalogURL string) *logicEnv {
	t.Helper()
	db, err := model.Open("file:" + filepath.Join(t.TempDir(), "logic.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	var sc svc.ServiceContext
	sc.ActivityDB = model.NewActivityModel(db)
	sc.RewardDB = model.NewRewardGrantModel(db)
	sc.GiftPackDB = model.NewGiftPackModel(db)
	if err := sc.ActivityDB.CreateTables(); err != nil {
		t.Fatalf("create activity tables: %v", err)
	}
	if err := sc.RewardDB.CreateTables(); err != nil {
		t.Fatalf("create reward tables: %v", err)
	}
	if err := sc.GiftPackDB.CreateTables(); err != nil {
		t.Fatalf("create gift pack tables: %v", err)
	}
	return &logicEnv{db: db, sc: &sc}
}

// withCatalog 给环境配一个返回指定角色/状态码的 game-catalog 桩，并同步
// 写入配置 BaseURL。
func (e *logicEnv) withCatalog(t *testing.T, role string, status int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		_, _ = w.Write([]byte(`{"role":"` + role + `"}`))
	}))
	t.Cleanup(srv.Close)
	e.sc.Config.GameCatalog.BaseURL = srv.URL
	return srv.URL
}

// withRewardSecret 配置奖励评估 S2S 密钥。
func (e *logicEnv) withRewardSecret(t *testing.T, secret string) {
	t.Helper()
	e.sc.Config.Reward.Secret = secret
}

// withNotify 配置业务通知客户端（community 投递 + catalog 预约人群桩）。
func (e *logicEnv) withNotify(t *testing.T, communityURL, catalogURL string) {
	t.Helper()
	e.sc.Notify = notify.New(communityURL, catalogURL, "tok", nil)
}

// adminCtx 构造带平台身份（user_id/client_ip）的请求上下文。
func adminCtx(userID int64, ip string) context.Context {
	ctx := context.WithValue(context.Background(), "user_id", userID)
	return context.WithValue(ctx, "client_ip", ip)
}

// mustAPIError 断言 err 是 apiError 且状态码匹配。
func mustAPIError(t *testing.T, err error, code int) {
	t.Helper()
	if err == nil {
		t.Fatalf("want apiError %d, got nil err", code)
	}
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr == nil {
		t.Fatalf("want apiError %d, got %v", code, err)
	}
	if apiErr.StatusCode() != code {
		t.Fatalf("status = %d, want %d（%v）", apiErr.StatusCode(), code, err)
	}
}

// httptestOK 返回 200 的任意路径桩地址。
func httptestOK(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// communityStub 返回 200 并累计命中数的投递桩。
func communityStub(t *testing.T, hits *int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// reservationStub 返回指定正文的预约人群桩。
func reservationStub(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// seedPublishedActivity 落一条已发布活动（版本快照 JSON 原文）。
func seedPublishedActivity(t *testing.T, m *model.ActivityModel, gameID, snapshot string) int64 {
	t.Helper()
	a := &model.Activity{
		Name: "act", GameId: gameID, Status: model.ActivityStatusDraft,
		Timezone: "Asia/Shanghai", StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-12-31T00:00:00Z",
		Audience: "{}", TriggerSpec: "{}", LimitsSpec: "{}", RewardsSpec: "{}",
	}
	if err := m.Create(a); err != nil {
		t.Fatalf("seed activity: %v", err)
	}
	if err := m.PublishSnapshot(a.Id, 1, snapshot, 0); err != nil {
		t.Fatalf("publish snapshot: %v", err)
	}
	return a.Id
}

// baseSnapshot 一个窗口覆盖 2026-06、全场（multiplier 示例）的合法快照。
func baseSnapshot() string {
	return `{"name":"夏日活动","game_id":"g1","timezone":"Asia/Shanghai",` +
		`"start_at":"2026-01-01T00:00:00Z","end_at":"2026-12-31T00:00:00Z",` +
		`"server_ids":[1],"channel":"","audience":{"type":"all"},` +
		`"trigger":{"type":"order","min_amount_cents":1000},` +
		`"limits":{"per_user_count":0,"global_count":0,"budget_cents":0},` +
		`"stackable":true,"stack_group":"","rewards":{"type":"multiplier","mode":"total","rate_bp":15000}}`
}
