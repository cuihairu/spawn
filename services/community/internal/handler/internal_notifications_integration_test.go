package handler

// GP-306 内部通知端点集成测试：S2S 令牌 fail-closed（未配置/不匹配 403，
// 校验先于 body 解析）、按 user_ids 扇出落库（列表/未读可见）、落库成功后
// 经 realtime-hub 远程推送一帧（stub hub 收到 publish，验收远程通知口径）。

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tappi/tappi/services/community/internal/realtime"
)

type hubStub struct {
	mu   sync.Mutex
	raws [][]byte
	srv  *httptest.Server
}

func newHubStub() *hubStub {
	h := &hubStub{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/publish", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		h.mu.Lock()
		h.raws = append(h.raws, raw)
		h.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"delivered":1}`))
	})
	h.srv = httptest.NewServer(mux)
	return h
}

func (h *hubStub) Calls() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.raws)
}

func (h *hubStub) Last() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.raws) == 0 {
		return nil
	}
	return h.raws[len(h.raws)-1]
}

func TestInternalNotifications(t *testing.T) {
	svcCtx, _ := newTestServiceContext(t)

	// stub realtime-hub + 打开内部令牌（默认未配置 = fail-closed）
	hub := newHubStub()
	defer hub.srv.Close()
	svcCtx.Config.Internal.Token = "gp306-token"
	svcCtx.Realtime = realtime.NewPublisher(hub.srv.URL, "hub-token")

	post := func(token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/internal/notifications", bytes.NewBufferString(body))
		if token != "" {
			req.Header.Set("X-Internal-Token", token)
		}
		rr := httptest.NewRecorder()
		InternalNotificationsHandler(svcCtx).ServeHTTP(rr, req)
		return rr
	}

	// 1. 令牌缺失/不匹配 → 403（fail-closed，先于 body 解析）
	if rr := post("", `{"user_ids":[42]}`); rr.Code != http.StatusForbidden {
		t.Fatalf("no token = %d, want 403", rr.Code)
	}
	if rr := post("wrong", `{"user_ids":[42]}`); rr.Code != http.StatusForbidden {
		t.Fatalf("wrong token = %d, want 403", rr.Code)
	}

	// 2. 缺 user_ids/type/content → 400
	if rr := post("gp306-token", `{"user_ids":[],"type":"x","content":"c"}`); rr.Code != http.StatusBadRequest {
		t.Fatalf("empty user_ids = %d, want 400", rr.Code)
	}
	if rr := post("gp306-token", `{"user_ids":[42],"type":"","content":"c"}`); rr.Code != http.StatusBadRequest {
		t.Fatalf("empty type = %d, want 400", rr.Code)
	}

	// 3. 扇出两人 → created 2；通知落库（列表可查、未读）
	rr := post("gp306-token", `{"user_ids":[42,43,0],"type":"server_open","target_id":7,"content":"S1 区服已开服","actor_name":"运营"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("fanout = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Created int `json:"created"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil || resp.Created != 2 {
		t.Fatalf("decode = %v body=%s, want created 2", err, rr.Body.String())
	}
	for _, uid := range []int64{42, 43} {
		items, total, err := svcCtx.NotificationRepo.ListByUser(uid, 10, 0)
		if err != nil || total < 1 {
			t.Fatalf("list notifications user %d = %v total=%d err=%v", uid, items, total, err)
		}
		n := items[0]
		if n.Type != "server_open" || n.TargetId != 7 || n.Content != "S1 区服已开服" || n.IsRead {
			t.Fatalf("notification user %d = %+v", uid, n)
		}
		unread, err := svcCtx.NotificationRepo.CountUnread(uid)
		if err != nil || unread != 1 {
			t.Fatalf("unread user %d = %d err=%v", uid, unread, err)
		}
	}

	// 4. 远程推送验收：stub hub 收到 publish，user_ids 全量、事件类型正确
	deadline := time.Now().Add(3 * time.Second)
	for hub.Calls() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if hub.Calls() == 0 {
		t.Fatalf("realtime hub received no publish")
	}
	var pub struct {
		UserIds []int64 `json:"user_ids"`
		Event   struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		} `json:"event"`
	}
	if err := json.Unmarshal(hub.Last(), &pub); err != nil {
		t.Fatalf("decode publish: %v body=%s", err, hub.Last())
	}
	if pub.Event.Type != "notification" || len(pub.UserIds) != 2 ||
		pub.UserIds[0] != 42 || pub.UserIds[1] != 43 {
		t.Fatalf("publish = %+v body=%s", pub, hub.Last())
	}
	if !bytes.Contains(pub.Event.Payload, []byte(`"target_id":7`)) {
		t.Fatalf("publish payload = %s", pub.Event.Payload)
	}
}
