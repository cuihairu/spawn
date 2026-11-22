package analytics

// client/relay 补漏分支：NewClient 的 no-op 臂（空 baseURL）、Emit 的
// outbox 落队列/同步首投/失败留 pending/写失败仅记日志臂、post 的请求
// 构建失败/Do 失败/非 200 臂、Relay 的 nil/扫描失败/重试耗尽 dropped/
// 退避 MarkFailed/投递成功臂、NewRelay 的默认值与 noop/nil outbox 返回
// nil 臂，以及 truncate 与 UserIDOf 的边界。
//
// 登记不可达（不测）：无（全部臂经上述场景触达）。

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tappi/tappi/services/promotions/model"

	_ "github.com/mattn/go-sqlite3"
)

// newOutbox 打开临时文件 SQLite 并建 analytics outbox 表（连接保持打开）。
func newOutbox(t *testing.T) *model.AnalyticsOutboxModel {
	t.Helper()
	_, m := openOutbox(t, "file:"+filepath.Join(t.TempDir(), "analytics-outbox.db"))
	return m
}

// newClosedOutbox 建表后关闭连接：出队读/标记写全部失败（写失败仅记日志臂）。
func newClosedOutbox(t *testing.T) *model.AnalyticsOutboxModel {
	t.Helper()
	return newOutboxOn(t, "file:"+filepath.Join(t.TempDir(), "analytics-closed.db"), false)
}

// newOutboxOn 建表；keepOpen=false 时立即关闭底层连接（后续查询/写入全失败）。
func newOutboxOn(t *testing.T, dsn string, keepOpen bool) *model.AnalyticsOutboxModel {
	t.Helper()
	if !keepOpen {
		db, m := openOutbox(t, dsn)
		if err := db.Close(); err != nil {
			t.Fatalf("close sqlite: %v", err)
		}
		return m
	}
	_, m := openOutbox(t, dsn)
	return m
}

// openOutbox 打开（或新建）文件库并建表；返回连接与模型。
func openOutbox(t *testing.T, dsn string) (*sql.DB, *model.AnalyticsOutboxModel) {
	t.Helper()
	db, err := model.Open(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	m := model.NewAnalyticsOutboxModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create analytics outbox tables: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, m
}

// dataPanel 起一个 /internal/events 接收桩；hits() 返回已收请求数。
func dataPanel(t *testing.T, status int) (string, func() int) {
	t.Helper()
	var mu sync.Mutex
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() int {
		mu.Lock()
		defer mu.Unlock()
		return n
	}
}

func TestNewClient_NoopAndEnabled(t *testing.T) {
	// 空 baseURL → no-op（Enabled false）
	if NewClient("", "tok", 1000, nil).Enabled() {
		t.Fatal("no-op client Enabled() = true")
	}
	// 有 baseURL → 实客户端（Enabled true）
	if !NewClient("http://127.0.0.1:1", "tok", 1000, nil).Enabled() {
		t.Fatal("client Enabled() = false")
	}
	// no-op Emit 静默空转
	noop{}.Emit(Event{EventId: "evt:noop"})
}

// newReadonlyDueOutbox 建可写库落一条 due 行后，以 ?mode=ro 重开同文件：
// ListRetry 读成功、MarkX 写失败（重投标记写失败仅记日志臂）。
func newReadonlyDueOutbox(t *testing.T, eventId string) *model.AnalyticsOutboxModel {
	t.Helper()
	path := filepath.Join(t.TempDir(), "analytics-ro.db")
	db, writable := openOutbox(t, "file:"+path)
	if err := writable.Enqueue(eventId, "t", `{"x":1}`, nowUTC()); err != nil {
		t.Fatalf("enqueue due row: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close writable: %v", err)
	}
	ro := newOutboxOn(t, "file:"+path+"?mode=ro", true)
	if rows := mustListRetry(t, ro); len(rows) != 1 {
		t.Fatalf("readonly due rows = %d, want 1", len(rows))
	}
	return ro
}

// TestRelay_MarkWriteFailLogArms 只读库（读成写败）触达投递后标记写失败
// 的三个仅记日志臂：MarkDropped / MarkFailed / MarkDelivered。
func TestRelay_MarkWriteFailLogArms(t *testing.T) {
	badURL, _ := dataPanel(t, http.StatusForbidden)
	okURL, _ := dataPanel(t, http.StatusOK)
	badClient := func() *client {
		return &client{url: badURL + "/internal/events", token: "tok", client: &http.Client{}}
	}
	okClient := func() *client {
		return &client{url: okURL + "/internal/events", token: "tok", client: &http.Client{}}
	}

	// dropped 路径标记写失败（attempts 已用尽）
	drop := &Relay{outbox: newReadonlyDueOutbox(t, "evt:ro-drop"), client: badClient(), maxAttempts: 1}
	if a, d := drop.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("ro drop round = %d/%d, want 1/0", a, d)
	}
	// failed 路径标记写失败（退避）
	fail := &Relay{outbox: newReadonlyDueOutbox(t, "evt:ro-fail"), client: badClient(), maxAttempts: 3}
	if a, d := fail.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("ro fail round = %d/%d, want 1/0", a, d)
	}
	// delivered 路径标记写失败（投递成功）
	ok := &Relay{outbox: newReadonlyDueOutbox(t, "evt:ro-ok"), client: okClient(), maxAttempts: 3}
	if a, d := ok.RelayOnce(context.Background()); a != 1 || d != 1 {
		t.Fatalf("ro ok round = %d/%d, want 1/1", a, d)
	}
}

func TestClient_EmitArms(t *testing.T) {
	// 1. 无 outbox + 后端 200 → 同步投递成功（不写队列）
	url, hits := dataPanel(t, http.StatusOK)
	NewClient(url, "tok", 1000, nil).Emit(Event{EventId: "evt:1", EventType: EventRewardGranted})
	if hits() != 1 {
		t.Fatalf("nil-outbox hits = %d, want 1", hits())
	}

	// 2. 有 outbox + 后端 200 → 落队列后同步投递 → delivered
	outbox2 := newOutbox(t)
	NewClient(url, "tok", 1000, outbox2).Emit(Event{EventId: "evt:2", EventType: EventRewardGranted})
	if rows := mustListRetry(t, outbox2); len(rows) != 0 {
		t.Fatalf("delivered rows still due: %d", len(rows))
	}

	// 3. 有 outbox + 后端 500 → 首投失败，pending 留队列待 Relay
	outbox3 := newOutbox(t)
	badURL, _ := dataPanel(t, http.StatusInternalServerError)
	NewClient(badURL, "tok", 1000, outbox3).Emit(Event{EventId: "evt:3", EventType: EventRewardGranted})
	if rows := mustListRetry(t, outbox3); len(rows) != 1 {
		t.Fatalf("failed emit rows = %d, want 1", len(rows))
	}

	// 4. outbox 写失败（闭库）→ 仅记日志，仍尝试直投
	dead := NewClient(url, "tok", 1000, newClosedOutbox(t))
	dead.Emit(Event{EventId: "evt:4", EventType: EventRewardGranted}) // 不 panic
}

func TestClient_PostArms(t *testing.T) {
	// 请求构建失败（非法 URL）→ NewRequest 出错
	c := &client{url: "://bad", token: "tok", client: &http.Client{}}
	if err := c.post([]byte(`{}`)); err == nil {
		t.Fatal("post bad url = nil, want error")
	}
	// 后端不可达（closed port）→ Do 出错
	d := &client{url: "http://127.0.0.1:1/internal/events", token: "tok", client: &http.Client{Timeout: time.Second}}
	if err := d.post([]byte(`{}`)); err == nil {
		t.Fatal("post unreachable = nil, want error")
	}
	// 非 200 → 出错
	badURL, _ := dataPanel(t, http.StatusForbidden)
	b := &client{url: badURL + "/internal/events", token: "tok", client: &http.Client{Timeout: time.Second}}
	if err := b.post([]byte(`{}`)); err == nil {
		t.Fatal("post 403 = nil, want error")
	}
	// 200 → nil
	okURL, _ := dataPanel(t, http.StatusOK)
	ok := &client{url: okURL + "/internal/events", token: "tok", client: &http.Client{Timeout: time.Second}}
	if err := ok.post([]byte(`{}`)); err != nil {
		t.Fatalf("post ok = %v", err)
	}
}

func TestRelay_NewRelayArms(t *testing.T) {
	// noop 客户端 → nil（不启动扫描）
	noopC := NewClient("", "tok", 1000, nil)
	if r := NewRelay(newOutbox(t), noopC, time.Minute, 10, time.Hour); r != nil {
		t.Fatal("relay from noop client = non-nil")
	}
	// nil outbox → nil
	real := NewClient("http://127.0.0.1:1", "tok", 1000, nil)
	if r := NewRelay(nil, real, time.Minute, 10, time.Hour); r != nil {
		t.Fatal("relay with nil outbox = non-nil")
	}
	// interval/maxAttempts/retention 非正 → 默认值
	r := NewRelay(newOutbox(t), real, 0, 0, 0)
	if r == nil {
		t.Fatal("relay nil with defaults")
	}
	if r.interval != time.Minute || r.maxAttempts != 10 || r.retention != 7*24*time.Hour {
		t.Fatalf("relay defaults = %v/%d/%v", r.interval, r.maxAttempts, r.retention)
	}
}

func TestRelay_RelayOnceArms(t *testing.T) {
	// 1. nil relay → 0,0
	var nilRelay *Relay
	if a, d := nilRelay.RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("nil relay = %d/%d, want 0/0", a, d)
	}
	// 2. nil outbox → 0,0
	if a, d := (&Relay{}).RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("nil outbox = %d/%d, want 0/0", a, d)
	}
	// 3. 扫描失败（闭库）→ 0,0
	scanFail := &Relay{outbox: newClosedOutbox(t), client: &client{}, maxAttempts: 10}
	if a, d := scanFail.RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("scan fail = %d/%d, want 0/0", a, d)
	}

	// 4. 重试耗尽 → dropped 留痕
	outbox2 := newOutbox(t)
	badURL, _ := dataPanel(t, http.StatusForbidden)
	if err := outbox2.Enqueue("evt:drop", "t", `{"x":1}`, nowUTC()); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// 上一轮已用尽一次（attempts=1），本轮再失败即 dropped（maxAttempts=1）
	if err := outbox2.MarkFailed("evt:drop", 1, "prior", nowUTC()); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	dropRelay := &Relay{
		outbox:      outbox2,
		client:      &client{url: badURL + "/internal/events", token: "tok", client: &http.Client{}},
		maxAttempts: 1,
	}
	if a, d := dropRelay.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("drop round = %d/%d, want 1/0", a, d)
	}
	if rows := mustListRetry(t, outbox2); len(rows) != 0 {
		t.Fatalf("dropped row still due: %d", len(rows))
	}

	// 5. 退避 MarkFailed（attempts < maxAttempts）+ 投递成功两分支
	outbox3 := newOutbox(t)
	okURL, _ := dataPanel(t, http.StatusOK)
	if err := outbox3.Enqueue("evt:fail", "t", `{"x":2}`, nowUTC()); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	failRelay := &Relay{
		outbox:      outbox3,
		client:      &client{url: badURL + "/internal/events", token: "tok", client: &http.Client{}},
		maxAttempts: 3,
	}
	// 一轮：evt:fail 失败 → MarkFailed（退避排期到未来）
	if a, d := failRelay.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("backoff round = %d/%d, want 1/0", a, d)
	}
	// 下一轮：evt:ok 入队（此时 due）→ 成功投递 → delivered
	if err := outbox3.Enqueue("evt:ok", "t", `{"x":3}`, nowUTC()); err != nil {
		t.Fatalf("enqueue ok: %v", err)
	}
	okRelay := &Relay{
		outbox:      outbox3,
		client:      &client{url: okURL + "/internal/events", token: "tok", client: &http.Client{}},
		maxAttempts: 3,
	}
	if a, d := okRelay.RelayOnce(context.Background()); a != 1 || d != 1 {
		t.Fatalf("deliver round = %d/%d, want 1/1", a, d)
	}
	// evt:fail 排期在未来不再命中；evt:ok 已终态不再命中
	if rows := mustListRetry(t, outbox3); len(rows) != 0 {
		t.Fatalf("rows due after both = %d, want 0", len(rows))
	}

	// 6. 指数退避封顶臂：attempts>=7 时 mult 钳到 6
	db4, outbox4 := openOutbox(t, "file:"+filepath.Join(t.TempDir(), "analytics-cap.db"))
	badURL4, _ := dataPanel(t, http.StatusForbidden)
	if err := outbox4.Enqueue("evt:cap", "t", `{"x":4}`, nowUTC()); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := db4.Exec(`UPDATE analytics_outbox SET attempts = 6 WHERE event_id = 'evt:cap'`); err != nil {
		t.Fatalf("seed attempts: %v", err)
	}
	capRelay := &Relay{
		outbox:      outbox4,
		client:      &client{url: badURL4 + "/internal/events", token: "tok", client: &http.Client{}},
		maxAttempts: 10,
	}
	if a, _ := capRelay.RelayOnce(context.Background()); a != 1 {
		t.Fatalf("cap round attempted = %d, want 1", a)
	}
	var attempts int
	if err := db4.QueryRow(`SELECT attempts FROM analytics_outbox WHERE event_id='evt:cap'`).Scan(&attempts); err != nil {
		t.Fatalf("read attempts: %v", err)
	}
	if attempts != 7 {
		t.Fatalf("cap row attempts = %d, want 7", attempts)
	}
}

func TestRelay_RunArms(t *testing.T) {
	okURL, _ := dataPanel(t, http.StatusOK)
	okClient := &client{url: okURL + "/internal/events", token: "tok", client: &http.Client{}}

	// 1. 极短周期跑数轮后取消退出：Prune 删到旧终态行 → n>0 记日志臂；
	//    同时留一条 due pending 行 → attempted>0 记日志臂
	db, outbox := openOutbox(t, "file:"+filepath.Join(t.TempDir(), "analytics-run.db"))
	if err := outbox.Enqueue("evt:old", "t", `{"x":1}`, nowUTC()); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := outbox.Enqueue("evt:live", "t", `{"x":2}`, nowUTC()); err != nil {
		t.Fatalf("enqueue live: %v", err)
	}
	// 伪造两小时前的已终态行（dropped + created_at 旧于保留窗）
	if _, err := db.Exec(`UPDATE analytics_outbox SET status='dropped', created_at=? WHERE event_id='evt:old'`,
		time.Now().UTC().Add(-2*time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatalf("seed stale dropped: %v", err)
	}
	r := &Relay{outbox: outbox, client: okClient, maxAttempts: 3, interval: 2 * time.Millisecond, retention: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	time.Sleep(50 * time.Millisecond) // 让数轮 tick 触达（含 Prune n>0）
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("relay Run did not return after ctx cancel")
	}

	// 2. 闭库 + 数轮 tick → 扫描失败与 Prune 失败仅记日志臂
	r2 := &Relay{outbox: newClosedOutbox(t), client: okClient, maxAttempts: 3, interval: 2 * time.Millisecond, retention: time.Hour}
	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan struct{})
	go func() { r2.Run(ctx2); close(done2) }()
	time.Sleep(50 * time.Millisecond)
	cancel2()
	select {
	case <-done2:
	case <-time.After(2 * time.Second):
		t.Fatal("closed-db relay Run did not return after ctx cancel")
	}
}

func TestTruncateArms(t *testing.T) {
	if got := truncate("short", 500); got != "short" {
		t.Fatalf("truncate short = %q", got)
	}
	if got := truncate("exactly", 7); got != "exactly" {
		t.Fatalf("truncate exact = %q", got)
	}
	if got := truncate("abcdefghij", 3); got != "abc" {
		t.Fatalf("truncate long = %q, want abc", got)
	}
}

func TestUserIDOf_Arms(t *testing.T) {
	if got := UserIDOf("123"); got != 123 {
		t.Fatalf("UserIDOf(123) = %d", got)
	}
	if got := UserIDOf("abc"); got != 0 {
		t.Fatalf("UserIDOf(abc) = %d, want 0", got)
	}
	if got := UserIDOf("-1"); got != 0 {
		t.Fatalf("UserIDOf(-1) = %d, want 0", got)
	}
	if got := UserIDOf(""); got != 0 {
		t.Fatalf("UserIDOf(empty) = %d, want 0", got)
	}
}

func TestNowUTC_Arms(t *testing.T) {
	if got := nowUTC(); len(got) < 20 {
		t.Fatalf("nowUTC = %q", got)
	}
}

// mustListRetry 取出当前 due 行；读失败即测试失败。
func mustListRetry(t *testing.T, m *model.AnalyticsOutboxModel) []*model.AnalyticsOutboxRow {
	t.Helper()
	rows, err := m.ListRetry(nowUTC(), 100)
	if err != nil {
		t.Fatalf("list retry: %v", err)
	}
	return rows
}
