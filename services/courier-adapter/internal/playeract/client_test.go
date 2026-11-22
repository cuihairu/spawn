package playeract

// GP-303 活跃上报客户端/重投测试：未配置基址 = 不装配（nil，会话主流程 no-op）；
// 同步首投受理 → delivered（并带 X-Internal-Token 与固定路径）；首投失败 →
// pending 留痕 attempts=1；Relay 到期重投成功 → delivered；重试耗尽 → dropped。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
