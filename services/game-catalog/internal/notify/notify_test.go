package notify

// notify.go 补漏分支：NewPublisher 构造守卫、publisher.Publish 全路径（outbox
// 入队/首投/标记 delivered）、publisher.post 低层错误臂与 noop 空实现。成功
// 投递与 Relay 重投路径已由 relay_test.go 覆盖，此处补发布器侧其余臂。

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/model"
)

// NewPublisher 空 url → noop；非空 → *publisher。
func TestPublisher_NewPublisherGuards(t *testing.T) {
	noop := NewPublisher("", "tok", nil)
	if noop.Enabled() {
		t.Fatal("noop publisher Enabled() = true, want false")
	}
	noop.Publish(Notification{Type: "server_open"}) // 必须不 panic

	p := NewPublisher("http://community:8901", "tok", nil)
	pub, ok := p.(*publisher)
	if !ok {
		t.Fatalf("NewPublisher(url) = %T, want *publisher", p)
	}
	if !pub.Enabled() {
		t.Fatal("configured publisher Enabled() = false, want true")
	}
	if pub.url != "http://community:8901/internal/notifications" {
		t.Fatalf("url = %q, want %q", pub.url, "http://community:8901/internal/notifications")
	}
	if pub.token != "tok" {
		t.Fatalf("token = %q, want tok", pub.token)
	}
}

// Publish 全链成功：Enqueue → post 200 → MarkDelivered（行终态 delivered）。
func TestPublisher_PublishDelivers(t *testing.T) {
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

	p := NewPublisher(ts.URL, "tok", outbox)
	p.Publish(Notification{Type: "server_open", UserIds: []int64{7, 8}})

	if !strings.Contains(gotBody, `"user_ids":[7,8]`) {
		t.Fatalf("post body = %q, want user_ids [7,8]", gotBody)
	}
	rows, err := outbox.ListRetry(nowUTC(), 100)
	if err != nil || len(rows) != 0 {
		t.Fatalf("row should be delivered (not retryable): n=%d err=%v", len(rows), err)
	}
}

// Publish 首投失败（非 200）→ 行留 pending 待 Relay 重投。
func TestPublisher_PublishPostFailsKeepsPending(t *testing.T) {
	outbox, _ := newRelayOutbox(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	p := NewPublisher(ts.URL, "tok", outbox)
	p.Publish(Notification{Type: "server_open"})

	rows, err := outbox.ListRetry(nowUTC(), 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("want 1 pending row, got %d err=%v", len(rows), err)
	}
	if rows[0].Status != model.NotifyOutboxPending {
		t.Fatalf("status = %q, want pending", rows[0].Status)
	}
}

// Publish 无 outbox（补偿未启用）：直投 200 成功。
func TestPublisher_PublishNoOutbox(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	p := NewPublisher(ts.URL, "tok", nil)
	p.Publish(Notification{Type: "server_open"}) // 不 panic
}

// Publish outbox 写失败（关库）→ 记日志仍尝试直投；post 成功后再标记
// delivered 亦失败（关库）→ 仅记日志，不 panic。
func TestPublisher_PublishOutboxErrors(t *testing.T) {
	outbox, db := newRelayOutbox(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	db.Close()

	p := NewPublisher(ts.URL, "tok", outbox)
	p.Publish(Notification{Type: "server_open"}) // Enqueue + MarkDelivered 双失败，不 panic
}

// post 低层：非 200 → error。
func TestPublisher_PostNon200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	if err := NewPublisher(ts.URL, "tok", nil).(*publisher).post([]byte(`{}`)); err == nil {
		t.Fatal("post on 500 = nil error, want error")
	}
}

// post 低层：不可达地址 → error（Do 失败）。
func TestPublisher_PostUnreachable(t *testing.T) {
	p := NewPublisher("http://127.0.0.1:1", "tok", nil).(*publisher)
	if err := p.post([]byte(`{}`)); err == nil {
		t.Fatal("post on unreachable = nil error, want error")
	}
}

// post 低层：畸形 URL → NewRequest 失败 → error。
func TestPublisher_PostBadURL(t *testing.T) {
	p := &publisher{url: "://bad/internal/notifications", client: &http.Client{}}
	if err := p.post([]byte(`{}`)); err == nil {
		t.Fatal("post on malformed url = nil error, want error")
	}
}
