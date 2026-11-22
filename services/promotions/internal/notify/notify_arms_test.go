package notify

// notify/relay 补漏分支：Enabled 的 nil/未配置臂、Publish 的关闭跳过/
// outbox 落队列/首投失败留 pending/写失败仅记日志臂、post 的请求构建
// 失败/Do 失败/非 200 臂、ReservationUsers 的 nil/未配置/请求失败/非 200/
// 解码失败/成功臂、notifyKey 前缀、Relay 的 nil/扫描失败/耗尽 dropped/
// 退避封顶/投递成功/标记写失败臂与 Run 的周期体（attempted>0、Prune
// n>0、Prune 失败仅记日志）。
//
// 登记不可达（不测）：Publish 的 json.Marshal 失败臂（固定结构体），
// notifyKey 的 crypto/rand.Read 失败回退臂。

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

// newOutbox 打开临时文件 SQLite 并建 notify outbox 表（连接保持打开）。
func newOutbox(t *testing.T) *model.NotifyOutboxModel {
	t.Helper()
	_, m := openOutbox(t, "file:"+filepath.Join(t.TempDir(), "notify-outbox.db"))
	return m
}

// newClosedOutbox 建表后关闭连接：出队读/标记写全部失败（写失败仅记日志臂）。
func newClosedOutbox(t *testing.T) *model.NotifyOutboxModel {
	t.Helper()
	return newOutboxOn(t, "file:"+filepath.Join(t.TempDir(), "notify-closed.db"), false)
}

// newReadonlyDueOutbox 建可写库落一条 due 行后以 ?mode=ro 重开同文件：
// ListRetry 读成功、MarkX 写失败（标记写失败仅记日志臂）。
func newReadonlyDueOutbox(t *testing.T, key string) *model.NotifyOutboxModel {
	t.Helper()
	path := filepath.Join(t.TempDir(), "notify-ro.db")
	db, writable := openOutbox(t, "file:"+path)
	if err := writable.Enqueue(key, "t", `{"x":1}`, nowUTC()); err != nil {
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

// openOutbox 打开（或新建）文件库并建表；返回连接与模型。
func openOutbox(t *testing.T, dsn string) (*sql.DB, *model.NotifyOutboxModel) {
	t.Helper()
	db, err := model.Open(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	m := model.NewNotifyOutboxModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create notify outbox tables: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, m
}

// newOutboxOn 建表；keepOpen=false 时立即关闭底层连接（后续查询/写入全失败）。
func newOutboxOn(t *testing.T, dsn string, keepOpen bool) *model.NotifyOutboxModel {
	t.Helper()
	db, m := openOutbox(t, dsn)
	if keepOpen {
		return m
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}
	return m
}

// community 起一个 /internal/notifications 接收桩；hits() 返回已收请求数。
func community(t *testing.T, status int) (string, func() int) {
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

func TestClient_EnabledArms(t *testing.T) {
	var nilC *Client
	if nilC.Enabled() {
		t.Fatal("nil client Enabled() = true")
	}
	if New("", "cat", "tok", nil).Enabled() {
		t.Fatal("unconfigured client Enabled() = true")
	}
	if !New("http://127.0.0.1:1", "cat", "tok", nil).Enabled() {
		t.Fatal("configured client Enabled() = false")
	}
}

func TestClient_PublishArms(t *testing.T) {
	// 1. community 未配置 → 直接跳过（不请求、不落队列）
	New("", "cat", "tok", nil).Publish(Notification{UserIds: []int64{7}, Type: "t"})

	// 2. 无 outbox + 200 → 同步直投
	url, hits := community(t, http.StatusOK)
	New(url, "", "tok", nil).Publish(Notification{UserIds: []int64{7}, Type: "activity_published", TargetId: 1})
	if hits() != 1 {
		t.Fatalf("nil-outbox hits = %d, want 1", hits())
	}

	// 3. 有 outbox + 200 → 落队列 + 投递成功 → delivered
	outbox2 := newOutbox(t)
	New(url, "", "tok", outbox2).Publish(Notification{UserIds: []int64{7}, Type: "activity_published"})
	if rows := mustListRetry(t, outbox2); len(rows) != 0 {
		t.Fatalf("delivered rows still due: %d", len(rows))
	}

	// 4. 有 outbox + 500 → 首投失败，pending 留队列
	outbox3 := newOutbox(t)
	badURL, _ := community(t, http.StatusInternalServerError)
	New(badURL, "", "tok", outbox3).Publish(Notification{UserIds: []int64{7}, Type: "t"})
	if rows := mustListRetry(t, outbox3); len(rows) != 1 {
		t.Fatalf("failed publish rows = %d, want 1", len(rows))
	}

	// 5. outbox 写失败（闭库）→ 仅记日志，仍尝试直投
	New(url, "", "tok", newClosedOutbox(t)).Publish(Notification{UserIds: []int64{7}, Type: "t"})
}

func TestClient_PostArms(t *testing.T) {
	// 请求构建失败（非法 URL）
	c := &Client{communityURL: "://bad", client: &http.Client{}}
	if err := c.post([]byte(`{}`)); err == nil {
		t.Fatal("post bad url = nil, want error")
	}
	// 后端不可达
	d := &Client{communityURL: "http://127.0.0.1:1", client: &http.Client{Timeout: time.Second}}
	if err := d.post([]byte(`{}`)); err == nil {
		t.Fatal("post unreachable = nil, want error")
	}
	// 非 200
	badURL, _ := community(t, http.StatusForbidden)
	if err := (&Client{communityURL: badURL, client: &http.Client{Timeout: time.Second}}).post([]byte(`{}`)); err == nil {
		t.Fatal("post 403 = nil, want error")
	}
	// 200
	okURL, _ := community(t, http.StatusOK)
	if err := (&Client{communityURL: okURL, client: &http.Client{Timeout: time.Second}}).post([]byte(`{}`)); err != nil {
		t.Fatalf("post ok = %v", err)
	}
}

func TestNotifyKey_Arms(t *testing.T) {
	k := notifyKey("t")
	if len(k) < len("notify:") || k[:7] != "notify:" {
		t.Fatalf("notifyKey = %q, want notify: prefix", k)
	}
}

func TestClient_ReservationUsersArms(t *testing.T) {
	// nil / 未配置 → 错误
	var nilC *Client
	if _, err := nilC.ReservationUsers("g1"); err == nil {
		t.Fatal("nil client = nil err, want error")
	}
	if _, err := New("", "", "tok", nil).ReservationUsers("g1"); err == nil {
		t.Fatal("unconfigured catalog = nil err, want error")
	}
	// 请求构建失败
	if _, err := (&Client{catalogURL: "://bad", client: &http.Client{}}).ReservationUsers("g1"); err == nil {
		t.Fatal("bad url = nil err, want error")
	}
	// 不可达
	if _, err := (&Client{catalogURL: "http://127.0.0.1:1", client: &http.Client{Timeout: time.Second}}).ReservationUsers("g1"); err == nil {
		t.Fatal("unreachable = nil err, want error")
	}
	// 非 200
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(bad.Close)
	if _, err := (&Client{catalogURL: bad.URL, client: &http.Client{Timeout: time.Second}}).ReservationUsers("g1"); err == nil {
		t.Fatal("403 = nil err, want error")
	}
	// 解码失败
	junk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	t.Cleanup(junk.Close)
	if _, err := (&Client{catalogURL: junk.URL, client: &http.Client{Timeout: time.Second}}).ReservationUsers("g1"); err == nil {
		t.Fatal("junk body = nil err, want error")
	}
	// 成功
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Token") != "tok" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"game_id":"g1","user_ids":[7,8]}`))
	}))
	t.Cleanup(ok.Close)
	ids, err := (&Client{catalogURL: ok.URL, token: "tok", client: &http.Client{Timeout: time.Second}}).ReservationUsers("g1")
	if err != nil || len(ids) != 2 || ids[0] != 7 || ids[1] != 8 {
		t.Fatalf("reservation users = %v/%v, want [7 8]", ids, err)
	}
}

func TestRelay_NewRelayArms(t *testing.T) {
	// nil 客户端 / 未启用 / nil outbox → nil
	if r := NewRelay(newOutbox(t), nil, time.Minute, 10, time.Hour); r != nil {
		t.Fatal("relay from nil client = non-nil")
	}
	if r := NewRelay(newOutbox(t), New("", "", "tok", nil), time.Minute, 10, time.Hour); r != nil {
		t.Fatal("relay from disabled client = non-nil")
	}
	if r := NewRelay(nil, New("http://127.0.0.1:1", "", "tok", nil), time.Minute, 10, time.Hour); r != nil {
		t.Fatal("relay with nil outbox = non-nil")
	}
	// 非正 interval/maxAttempts/retention → 默认值
	r := NewRelay(newOutbox(t), New("http://127.0.0.1:1", "", "tok", nil), 0, 0, 0)
	if r == nil || r.interval != time.Minute || r.maxAttempts != 10 || r.retention != 7*24*time.Hour {
		t.Fatalf("relay defaults = %+v", r)
	}
}

func TestRelay_RelayOnceArms(t *testing.T) {
	// nil relay / nil outbox → 0,0
	var nilRelay *Relay
	if a, d := nilRelay.RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("nil relay = %d/%d", a, d)
	}
	if a, d := (&Relay{}).RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("nil outbox = %d/%d", a, d)
	}
	// 扫描失败（闭库）→ 0,0
	scanFail := &Relay{outbox: newClosedOutbox(t), client: New("http://127.0.0.1:1", "", "tok", nil), maxAttempts: 10}
	if a, d := scanFail.RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("scan fail = %d/%d", a, d)
	}

	badURL, _ := community(t, http.StatusForbidden)
	okURL, _ := community(t, http.StatusOK)
	badClient := New(badURL, "", "tok", nil)
	okClient := New(okURL, "", "tok", nil)

	// 重试耗尽 → dropped
	outbox2 := newOutbox(t)
	if err := outbox2.Enqueue("nk:drop", "t", `{"x":1}`, nowUTC()); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := outbox2.MarkFailed("nk:drop", 1, "prior", nowUTC()); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	dropRelay := &Relay{outbox: outbox2, client: badClient, maxAttempts: 1}
	if a, d := dropRelay.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("drop round = %d/%d, want 1/0", a, d)
	}
	if rows := mustListRetry(t, outbox2); len(rows) != 0 {
		t.Fatalf("dropped row still due: %d", len(rows))
	}

	// 退避 MarkFailed（attempts < maxAttempts）
	outbox3 := newOutbox(t)
	if err := outbox3.Enqueue("nk:fail", "t", `{"x":2}`, nowUTC()); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	failRelay := &Relay{outbox: outbox3, client: badClient, maxAttempts: 3}
	if a, d := failRelay.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("backoff round = %d/%d, want 1/0", a, d)
	}
	if rows := mustListRetry(t, outbox3); len(rows) != 0 {
		t.Fatalf("backoff row immediately due: %d", len(rows))
	}

	// 指数退避封顶臂：attempts>=7 时 mult 钳到 6
	db4, outbox4 := openOutbox(t, "file:"+filepath.Join(t.TempDir(), "notify-cap.db"))
	if err := outbox4.Enqueue("nk:cap", "t", `{"x":3}`, nowUTC()); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := db4.Exec(`UPDATE notify_outbox SET attempts = 6 WHERE notify_key = 'nk:cap'`); err != nil {
		t.Fatalf("seed attempts: %v", err)
	}
	capRelay := &Relay{outbox: outbox4, client: badClient, maxAttempts: 10}
	if a, _ := capRelay.RelayOnce(context.Background()); a != 1 {
		t.Fatalf("cap round attempted = %d, want 1", a)
	}
	var attempts int
	if err := db4.QueryRow(`SELECT attempts FROM notify_outbox WHERE notify_key='nk:cap'`).Scan(&attempts); err != nil {
		t.Fatalf("read attempts: %v", err)
	}
	if attempts != 7 {
		t.Fatalf("cap row attempts = %d, want 7", attempts)
	}

	// 投递成功 → delivered
	outbox5 := newOutbox(t)
	if err := outbox5.Enqueue("nk:ok", "t", `{"x":4}`, nowUTC()); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	okRelay := &Relay{outbox: outbox5, client: okClient, maxAttempts: 3}
	if a, d := okRelay.RelayOnce(context.Background()); a != 1 || d != 1 {
		t.Fatalf("deliver round = %d/%d, want 1/1", a, d)
	}
	if rows := mustListRetry(t, outbox5); len(rows) != 0 {
		t.Fatalf("delivered row still due: %d", len(rows))
	}

	// 标记写失败仅记日志三臂（只读库：读成写败）
	drop2 := &Relay{outbox: newReadonlyDueOutbox(t, "nk:ro-drop"), client: badClient, maxAttempts: 1}
	if a, d := drop2.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("ro drop round = %d/%d, want 1/0", a, d)
	}
	fail2 := &Relay{outbox: newReadonlyDueOutbox(t, "nk:ro-fail"), client: badClient, maxAttempts: 3}
	if a, d := fail2.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("ro fail round = %d/%d, want 1/0", a, d)
	}
	ok2 := &Relay{outbox: newReadonlyDueOutbox(t, "nk:ro-ok"), client: okClient, maxAttempts: 3}
	if a, d := ok2.RelayOnce(context.Background()); a != 1 || d != 1 {
		t.Fatalf("ro ok round = %d/%d, want 1/1", a, d)
	}
}

func TestRelay_RunArms(t *testing.T) {
	okURL, _ := community(t, http.StatusOK)
	okClient := New(okURL, "", "tok", nil)

	// 1. 极短周期跑数轮后取消退出：due 行 → attempted>0 臂；
	//    旧终态行 → Prune n>0 臂
	db, outbox := openOutbox(t, "file:"+filepath.Join(t.TempDir(), "notify-run.db"))
	if err := outbox.Enqueue("nk:old", "t", `{"x":1}`, nowUTC()); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := outbox.Enqueue("nk:live", "t", `{"x":2}`, nowUTC()); err != nil {
		t.Fatalf("enqueue live: %v", err)
	}
	if _, err := db.Exec(`UPDATE notify_outbox SET status='dropped', created_at=? WHERE notify_key='nk:old'`,
		time.Now().UTC().Add(-2*time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatalf("seed stale dropped: %v", err)
	}
	r := &Relay{outbox: outbox, client: okClient, maxAttempts: 3, interval: 2 * time.Millisecond, retention: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("notify relay Run did not return after ctx cancel")
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
		t.Fatal("closed-db notify relay Run did not return after ctx cancel")
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

func TestNowUTC_Arms(t *testing.T) {
	if got := nowUTC(); len(got) < 20 {
		t.Fatalf("nowUTC = %q", got)
	}
}

// mustListRetry 取出当前 due 行；读失败即测试失败。
func mustListRetry(t *testing.T, m *model.NotifyOutboxModel) []*model.NotifyOutboxRow {
	t.Helper()
	rows, err := m.ListRetry(nowUTC(), 100)
	if err != nil {
		t.Fatalf("list retry: %v", err)
	}
	return rows
}
