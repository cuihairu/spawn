package analytics

// client.go 补漏分支：NewClient 构造守卫、client.Emit 全路径（outbox 入队/
// 首投/标记 delivered）、client.post 低层错误臂与 noop 空实现。成功投递与
// Relay 重投路径已由 relay_test.go 覆盖，此处补客户端侧其余臂。

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/model"
)

// NewClient 空 baseURL → noop（数据面板未部署零开销）；非空 → *client。
func TestClient_NewClientGuards(t *testing.T) {
	noop := NewClient("", "tok", 1000, nil)
	if noop.Enabled() {
		t.Fatal("noop client Enabled() = true, want false")
	}
	noop.Emit(Event{EventType: "download"}) // 必须不 panic

	c := NewClient("http://panel:8896", "tok", 1000, nil)
	client, ok := c.(*client)
	if !ok {
		t.Fatalf("NewClient(url) = %T, want *client", c)
	}
	if !client.Enabled() {
		t.Fatal("configured client Enabled() = false, want true")
	}
	if client.url != "http://panel:8896/internal/events" {
		t.Fatalf("url = %q, want %q", client.url, "http://panel:8896/internal/events")
	}
	if client.token != "tok" {
		t.Fatalf("token = %q, want tok", client.token)
	}
}

// Emit 全链成功：Enqueue → post 200 → MarkDelivered（行终态 delivered）。
func TestClient_EmitDelivers(t *testing.T) {
	outbox, _ := newRelayOutbox(t)
	var gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Token") != "tok" {
			t.Errorf("missing X-Internal-Token header")
		}
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL, outbox)
	c.token = "tok"
	c.Emit(Event{EventId: "e1", EventType: "download", GameId: "g1"})

	if !strings.Contains(gotBody, `"event_id":"e1"`) {
		t.Fatalf("post body = %q, want event_id e1", gotBody)
	}
	row, err := outbox.ListRetry(nowUTC(), 100)
	if err != nil || len(row) != 0 {
		t.Fatalf("row should be delivered (not retryable): n=%d err=%v", len(row), err)
	}
}

// Emit 首投失败（非 200）→ 行留 pending 待 Relay 重投，不标记 delivered。
func TestClient_EmitPostFailsKeepsPending(t *testing.T) {
	outbox, _ := newRelayOutbox(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL, outbox)
	c.Emit(Event{EventId: "e2", EventType: "download"})

	rows, err := outbox.ListRetry(nowUTC(), 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("want 1 pending row, got %d err=%v", len(rows), err)
	}
	if rows[0].Status != model.AnalyticsOutboxPending {
		t.Fatalf("status = %q, want pending", rows[0].Status)
	}
}

// Emit 无 outbox（补偿未启用）：直投 200 成功，不触碰 outbox。
func TestClient_EmitNoOutbox(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := newTestClient(ts.URL, nil)
	c.Emit(Event{EventId: "e3", EventType: "download"}) // 不 panic
}

// Emit outbox 写失败（关库）→ 记日志仍尝试直投；post 成功后再标记 delivered
// 亦失败（关库）→ 仅记日志，不 panic。
func TestClient_EmitOutboxErrors(t *testing.T) {
	outbox, db := newRelayOutbox(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	db.Close()

	c := newTestClient(ts.URL, outbox)
	c.Emit(Event{EventId: "e4", EventType: "download"}) // Enqueue + MarkDelivered 双失败，不 panic
}

// post 低层：非 200 → error。
func TestClient_PostNon200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	if err := newTestClient(ts.URL, nil).post([]byte(`{}`)); err == nil {
		t.Fatal("post on 500 = nil error, want error")
	}
}

// post 低层：不可达地址 → error（Do 失败）。
func TestClient_PostUnreachable(t *testing.T) {
	c := newTestClient("http://127.0.0.1:1", nil)
	if err := c.post([]byte(`{}`)); err == nil {
		t.Fatal("post on unreachable = nil error, want error")
	}
}

// post 低层：畸形 URL → NewRequest 失败 → error。
func TestClient_PostBadURL(t *testing.T) {
	c := &client{url: "://bad/internal/events", client: &http.Client{}}
	if err := c.post([]byte(`{}`)); err == nil {
		t.Fatal("post on malformed url = nil error, want error")
	}
}
