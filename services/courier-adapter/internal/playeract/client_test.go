package playeract

// GP-303 活跃上报客户端/重投测试：未配置基址 = 不装配（nil，会话主流程 no-op）；
// 同步首投受理 → delivered（并带 X-Internal-Token 与固定路径）；首投失败 →
// pending 留痕 attempts=1；Relay 到期重投成功 → delivered；重试耗尽 → dropped。

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tappi/tappi/services/courier-adapter/model"
)

// promotionsStub 可切换成败的 promotions 替身（记录路径/令牌/载荷）。
type promotionsStub struct {
	mu       sync.Mutex
	fail     bool
	path     string
	token    string
	lastBody string
	n        int
}

func (s *promotionsStub) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.path = r.URL.Path
		s.token = r.Header.Get("X-Internal-Token")
		body, _ := io.ReadAll(r.Body)
		s.lastBody = string(body)
		s.n++
		fail := s.fail
		s.mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (s *promotionsStub) snapshot() (string, string, string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.path, s.token, s.lastBody, s.n
}

func setFail(s *promotionsStub, fail bool) {
	s.mu.Lock()
	s.fail = fail
	s.mu.Unlock()
}

// aoDB 建出 outbox 表并返回模型。
func aoDB(t *testing.T) *model.ActivityOutboxModel {
	t.Helper()
	db, err := model.Open("file:" + t.TempDir() + "/playeract-test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := model.NewActivityOutboxModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create outbox tables: %v", err)
	}
	return m
}

// aoEnv 活跃 outbox 测试环境：模型 + 原始连接（关库/触发器驱动 DB 失败臂）。
type aoEnv struct {
	m   *model.ActivityOutboxModel
	raw *sql.DB
}

func newAOEnv(t *testing.T) *aoEnv {
	t.Helper()
	db, err := model.Open("file:" + t.TempDir() + "/playeract-env.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := model.NewActivityOutboxModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create outbox tables: %v", err)
	}
	return &aoEnv{m: m, raw: db}
}

// closeDB 关底层连接（后续读/写均失败；驱动 DB-error 失败臂）。
func (e *aoEnv) closeDB() {
	_ = e.raw.Close()
}

// addTrigger 建触发器（RAISE(ABORT)）驱动对应 UPDATE 失败臂；ddl 为完整
// CREATE TRIGGER 语句（WHEN 子句限定仅目标状态转移中止，其余写不受影响）。
func (e *aoEnv) addTrigger(t *testing.T, ddl string) {
	t.Helper()
	if _, err := e.raw.Exec(ddl); err != nil {
		t.Fatalf("add trigger: %v", err)
	}
}

func TestNewClientAndReporterDisabledWhenUnset(t *testing.T) {
	if c := NewClient("", "tok", 3000); c != nil {
		t.Fatalf("NewClient(\"\") = %v, want nil（未接促销服务不装配）", c)
	}
	if r := NewReporter("", "tok", 3000, aoDB(t)); r != nil {
		t.Fatalf("NewReporter(\"\") = %v, want nil", r)
	}
	// nil 上报器可直接调用（会话主流程不判空也能安全跑）
	(*Reporter)(nil).Report(context.Background(), "u1", "g1", time.Now().UTC().Format(time.RFC3339Nano))
}

func TestReporter_FirstAttemptDelivered(t *testing.T) {
	stub := &promotionsStub{}
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	outbox := aoDB(t)
	r := NewReporter(srv.URL, "s2s-token", 2000, outbox)
	if r == nil {
		t.Fatal("NewReporter = nil, want 已装配")
	}
	at := "2026-11-22T08:00:00.000000000Z"
	r.Report(context.Background(), "u1", "g1", at)

	path, token, body, n := stub.snapshot()
	if path != "/internal/player-activity" || token != "s2s-token" || n != 1 {
		t.Fatalf("stub = path %q token %q calls %d, want /internal/player-activity s2s-token 1", path, token, n)
	}
	var got Report
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode report body: %v body=%s", err, body)
	}
	if got.AccountId != "u1" || got.GameId != "g1" || got.ActiveAt != at {
		t.Fatalf("report = %+v, want 账号/游戏/时刻原样", got)
	}
	rows, err := outbox.ListRetry(time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), 100)
	if err != nil {
		t.Fatalf("list retry: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("首投成功仍留在重投队列: %+v", rows)
	}
}

func TestReporter_FirstAttemptFailedLeavesPending(t *testing.T) {
	stub := &promotionsStub{}
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	setFail(stub, true)
	outbox := aoDB(t)
	r := NewReporter(srv.URL, "s2s-token", 2000, outbox)
	if r == nil {
		t.Fatal("NewReporter = nil")
	}
	// 上报失败不得 panic、不得阻断（会话主流程口径）
	r.Report(context.Background(), "u2", "g1", "2026-11-22T08:00:00.000000000Z")

	rows, err := outbox.ListRetry(time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), 100)
	if err != nil {
		t.Fatalf("list retry: %v", err)
	}
	if len(rows) != 1 || rows[0].Status != model.ActivityOutboxPending || rows[0].Attempts != 1 {
		t.Fatalf("rows = %+v, want 1 条 pending attempts=1（首投失败留痕待重投）", rows)
	}
}

func TestRelay_RecoversAndExhausts(t *testing.T) {
	stub := &promotionsStub{}
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	setFail(stub, true)
	outbox := aoDB(t)
	client := NewClient(srv.URL, "s2s-token", 2000)
	relay := NewRelay(outbox, client, time.Second, 2, 24*time.Hour)
	if relay == nil {
		t.Fatal("NewRelay = nil")
	}
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := outbox.Enqueue("act:u3:g1:p1", "u3", "g1", "t3", `{"account_id":"u3","game_id":"g1","active_at":"t3"}`, past); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := outbox.Enqueue("act:u4:g1:p2", "u4", "g1", "t4", `{"account_id":"u4","game_id":"g1","active_at":"t4"}`, past); err != nil {
		t.Fatalf("enqueue 2: %v", err)
	}

	ctx := context.Background()
	// 全失败：attempts 1 → 仍 pending 退避
	setFail(stub, true)
	attempted, delivered := relay.RelayOnce(ctx)
	if attempted != 2 || delivered != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 2/0", attempted, delivered)
	}
	rows, _ := outbox.ListRetry(time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), 100)
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want 2 条仍待重投（未耗尽）", rows)
	}

	// 退避窗口到期（把 next_retry_at 拨回过去，模拟等待期满）
	for _, row := range rows {
		if err := outbox.MarkFailed(row.ActKey, 1, row.LastError, past); err != nil {
			t.Fatalf("rewind backoff: %v", err)
		}
	}

	// 恢复后重投成功 → delivered
	setFail(stub, false)
	attempted, delivered = relay.RelayOnce(ctx)
	if attempted != 2 || delivered != 2 {
		t.Fatalf("attempted=%d delivered=%d, want 2/2（恢复即补齐）", attempted, delivered)
	}
	rows, _ = outbox.ListRetry(time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), 100)
	if len(rows) != 0 {
		t.Fatalf("恢复后仍留在重投队列: %+v", rows)
	}

	// 重试耗尽 → dropped 留痕（maxAttempts=2）
	setFail(stub, true)
	if err := outbox.Enqueue("act:u5:g1:p3", "u5", "g1", "t5", `{"account_id":"u5","game_id":"g1","active_at":"t5"}`, past); err != nil {
		t.Fatalf("enqueue 3: %v", err)
	}
	for i := 0; i < 3; i++ {
		_, _ = relay.RelayOnce(ctx)
		// 退避窗口到期（模拟等待期满；已 dropped 的行 MarkFailed 为 no-op）
		pending, _ := outbox.ListRetry(time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), 100)
		for _, row := range pending {
			_ = outbox.MarkFailed(row.ActKey, row.Attempts, row.LastError, past)
		}
	}
	rows, _ = outbox.ListRetry(time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), 100)
	if len(rows) != 0 {
		t.Fatalf("耗尽后仍出现在重投队列: %+v", rows)
	}
}

func TestNewClient_DefaultTimeout(t *testing.T) {
	// timeoutMs<=0 回退 3 秒（不 panic），客户端仍装配（超时默认值臂）。
	if c := NewClient("http://127.0.0.1:1", "tok", 0); c == nil {
		t.Fatal("NewClient(url, tok, 0) = nil, want 已装配（超时回退）")
	}
	if c := NewClient("http://127.0.0.1:1", "tok", -5); c == nil {
		t.Fatal("NewClient(url, tok, -5) = nil, want 已装配（负值回退）")
	}
}

func TestClient_Report_TransportError(t *testing.T) {
	// 连接拒绝 → 不可达错误（同步首投失败臂）。
	c := NewClient("http://127.0.0.1:1", "tok", 50)
	if err := c.Report(context.Background(), []byte(`{}`)); err == nil {
		t.Fatal("err = nil, want 传输错误")
	}
}

func TestClient_Report_BadBaseURL(t *testing.T) {
	// NewRequest 失败臂：baseURL 含 URL 非法控制字符（NewClient 不校验 URL，
	// 非法字面由 http.NewRequest 构造时拒绝）。白盒直构客户端注入非法 URL。
	c := &Client{baseURL: "http://a\x01b/", token: "tok", hc: &http.Client{}}
	if err := c.Report(context.Background(), []byte(`{}`)); err == nil {
		t.Fatal("err = nil, want NewRequest 失败")
	}
}

func TestReporter_EnqueueDBError(t *testing.T) {
	// outbox 不可写（关库）：入队失败仅记日志，不 panic、不阻断（会话主流程口径）。
	stub := &promotionsStub{}
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	env := newAOEnv(t)
	env.closeDB()
	r := NewReporter(srv.URL, "tok", 2000, env.m)
	if r == nil {
		t.Fatal("NewReporter = nil")
	}
	r.Report(context.Background(), "u1", "g1", "2026-11-22T08:00:00.000000000Z")
}

func TestReporter_MarkFailedDBError(t *testing.T) {
	// 同步首投失败但 MarkFailed 写失败：仅记日志（best-effort），不 panic、
	// 不阻断——活跃事实仍留 pending，Relay 后续可再投。
	stub := &promotionsStub{}
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	setFail(stub, true)
	env := newAOEnv(t)
	env.addTrigger(t, `CREATE TRIGGER trg_mf BEFORE UPDATE ON player_activity_outbox
		WHEN NEW.status='pending' BEGIN SELECT RAISE(ABORT,'boom'); END;`)
	r := NewReporter(srv.URL, "tok", 2000, env.m)
	if r == nil {
		t.Fatal("NewReporter = nil")
	}
	r.Report(context.Background(), "u1", "g1", "2026-11-22T08:00:00.000000000Z")
}

func TestReporter_MarkDeliveredDBError(t *testing.T) {
	// 首投受理但 MarkDelivered 写失败：仅记日志（best-effort），不 panic。
	stub := &promotionsStub{}
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	env := newAOEnv(t)
	env.addTrigger(t, `CREATE TRIGGER trg_md BEFORE UPDATE ON player_activity_outbox
		WHEN NEW.status='delivered' BEGIN SELECT RAISE(ABORT,'boom'); END;`)
	r := NewReporter(srv.URL, "tok", 2000, env.m)
	if r == nil {
		t.Fatal("NewReporter = nil")
	}
	r.Report(context.Background(), "u1", "g1", "2026-11-22T08:00:00.000000000Z")
}

func TestTruncate_Long(t *testing.T) {
	// 长错误截断到 n（last_error 有界，不因超长错误撑爆列）；短串原样返回。
	long := strings.Repeat("x", 600)
	if got := truncate(long, 500); len(got) != 500 {
		t.Fatalf("truncate(len=%d) = len %d, want 500", len(long), len(got))
	}
	if got := truncate("short", 500); got != "short" {
		t.Fatalf("truncate(short) = %q, want 原样", got)
	}
}
