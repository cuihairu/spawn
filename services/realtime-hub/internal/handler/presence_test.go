package handler

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// wsSend 客户端发一行控制消息。
func wsSend(t *testing.T, conn *websocket.Conn, payload string) {
	t.Helper()
	if err := conn.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
		t.Fatalf("ws send: %v", err)
	}
}

// readFrame 读一帧并解析（deadline 内）。
func readFrame(t *testing.T, conn *websocket.Conn, timeout time.Duration) hubFrameT {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}
	var f hubFrameT
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("frame %s: %v", data, err)
	}
	return f
}

type hubFrameT struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// waitPresenceEvent 循环读帧直至出现目标用户的目标在线状态
// （跳过路径上的 query 回执等其他帧）。
func waitPresenceEvent(t *testing.T, conn *websocket.Conn, userId int64, online bool) {
	t.Helper()
	for {
		f := readFrame(t, conn, 5*time.Second)
		if f.Type != "presence" {
			continue
		}
		var evt struct {
			UserId int64 `json:"user_id"`
			Online bool  `json:"online"`
		}
		if err := json.Unmarshal(f.Data, &evt); err != nil {
			t.Fatalf("presence data %s: %v", f.Data, err)
		}
		if evt.UserId == userId && evt.Online == online {
			return
		}
	}
}

// presenceGET 调内部查询端点。
func presenceGET(t *testing.T, srvURL, token, ids string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srvURL+"/internal/presence?user_ids="+ids, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("X-Internal-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("presence get: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// TestPresenceRESTGuards 守卫矩阵：403×2、400×2、200 空结果。
func TestPresenceRESTGuards(t *testing.T) {
	srv, _ := newTestServer(t)
	for name, tc := range map[string]struct {
		token  string
		ids    string
		status int
	}{
		"no token":   {"", "7", http.StatusForbidden},
		"wrong done": {"nope", "7", http.StatusForbidden},
		"missing":    {testTokenID, "", http.StatusBadRequest},
		"bad ids":    {testTokenID, "7,x", http.StatusBadRequest},
		"empty ok":   {testTokenID, "999", http.StatusOK},
	} {
		if got := presenceGET(t, srv.URL, tc.token, tc.ids); got.StatusCode != tc.status {
			t.Fatalf("%s: status = %d, want %d", name, got.StatusCode, tc.status)
		}
	}
}

// TestPresenceEndToEnd A watch 7 → 7 连接 A 收 online；REST 快照一致；
// 畸形上行不断连；7 断开 A 收 offline。
func TestPresenceEndToEnd(t *testing.T) {
	srv, h := newTestServer(t)
	a, _, err := dialWS(t, srv.URL, signToken(t, 1, time.Hour))
	if err != nil {
		t.Fatalf("dial a: %v", err)
	}
	defer a.Close()

	// watch 后用 query 回执当屏障（同一连接 FIFO，回执到达即 watch 已生效）
	wsSend(t, a, `{"action":"watch","user_ids":[7]}`)
	wsSend(t, a, `{"action":"query","user_ids":[8]}`)
	f := readFrame(t, a, 5*time.Second)
	if f.Type != "presence" {
		t.Fatalf("barrier frame type = %q, want presence", f.Type)
	}

	b, _, err := dialWS(t, srv.URL, signToken(t, 7, time.Hour))
	if err != nil {
		t.Fatalf("dial b: %v", err)
	}
	waitConnections(t, h, map[int64]int{7: 1})
	waitPresenceEvent(t, a, 7, true)

	// REST 快照：7 在线 1 连接带 since，8 离线
	resp := presenceGET(t, srv.URL, testTokenID, "7,8")
	var got presenceResp
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Presence) != 2 || !got.Presence[0].Online || got.Presence[0].Connections != 1 ||
		got.Presence[0].Since == "" || got.Presence[1].Online {
		t.Fatalf("presence = %+v", got.Presence)
	}

	// 畸形上行：静默忽略，连接仍可用
	wsSend(t, a, `not-json-at-all`)
	wsSend(t, a, `{"action":"query","user_ids":[7]}`)
	for {
		f = readFrame(t, a, 5*time.Second)
		if f.Type == "presence" {
			break
		}
	}

	// 7 断开 → A 收 offline
	_ = b.Close()
	waitPresenceEvent(t, a, 7, false)
}
