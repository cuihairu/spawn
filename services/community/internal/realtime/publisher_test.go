package realtime

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestNewPublisherEmptyURLNoop Url 空 → no-op，Publish 不 panic 不阻塞。
func TestNewPublisherEmptyURLNoop(t *testing.T) {
	NewPublisher("", "tok").Publish("notification", []int64{7}, map[string]any{"n": 1})
}

// TestPublishDeliversToHub 端到端：stub hub 收到 POST /internal/publish，
// 头与 body 形状与 realtime-hub 契约一致。
func TestPublishDeliversToHub(t *testing.T) {
	type received struct {
		body    []byte
		token   string
		content string
	}
	got := make(chan received, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/publish" {
			t.Errorf("path = %s, want /internal/publish", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		got <- received{body: b, token: r.Header.Get("X-Internal-Token"), content: r.Header.Get("Content-Type")}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewPublisher(srv.URL, "tok-123")
	p.Publish("notification", []int64{7, 8}, map[string]any{"n": 1})

	select {
	case r := <-got:
		if r.token != "tok-123" || r.content != "application/json" {
			t.Fatalf("headers = %q/%q, want tok-123/application/json", r.token, r.content)
		}
		var body publishBody
		if err := json.Unmarshal(r.body, &body); err != nil {
			t.Fatalf("body %s: %v", r.body, err)
		}
		if body.Event.Type != "notification" || len(body.UserIds) != 2 || body.UserIds[1] != 8 {
			t.Fatalf("body = %+v, want notification to [7 8]", body)
		}
		if body.Event.Payload.(map[string]any)["n"].(float64) != 1 {
			t.Fatalf("payload = %v, want n=1", body.Event.Payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("hub stub received no publish within 3s")
	}
}

// TestPublishMarshalErrorReturnsSync 非法 payload（channel 不可序列化）
// 同步返回，不 panic 不投递。
func TestPublishMarshalErrorReturnsSync(t *testing.T) {
	p := NewPublisher("http://127.0.0.1:1", "tok")
	p.Publish("notification", []int64{7}, make(chan int)) // 不可序列化
}
