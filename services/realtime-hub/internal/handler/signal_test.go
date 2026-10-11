package handler

import (
	"encoding/json"
	"testing"
	"time"
)

// TestSignalEndToEnd 真实 WS 链路：A 上行 signal → B 收
// {"type":"signal","data":{from,room,data}}；畸形上行静默忽略不断连。
func TestSignalEndToEnd(t *testing.T) {
	srv, h := newTestServer(t)
	a, _, err := dialWS(t, srv.URL, signToken(t, 1, time.Hour))
	if err != nil {
		t.Fatalf("dial a: %v", err)
	}
	defer a.Close()
	b, _, err := dialWS(t, srv.URL, signToken(t, 2, time.Hour))
	if err != nil {
		t.Fatalf("dial b: %v", err)
	}
	defer b.Close()
	waitConnections(t, h, map[int64]int{1: 1, 2: 1})

	up, _ := json.Marshal(map[string]any{
		"action": "signal", "to": 2, "room": "r9",
		"data": map[string]string{"sdp": "offer-payload"},
	})
	wsSend(t, a, string(up))

	for {
		f := readFrame(t, b, 5*time.Second)
		if f.Type != "signal" {
			continue
		}
		var p struct {
			From int64           `json:"from"`
			Room string          `json:"room"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(f.Data, &p); err != nil {
			t.Fatalf("signal payload %s: %v", f.Data, err)
		}
		if p.From != 1 || p.Room != "r9" || string(p.Data) != `{"sdp":"offer-payload"}` {
			t.Fatalf("signal = %+v", p)
		}
		break
	}

	// 畸形上行：静默忽略，连接仍可继续收发
	wsSend(t, a, `{"action":"signal"`)         // 截断 JSON
	wsSend(t, a, `{"action":"signal","to":0}`) // 非法目标
	wsSend(t, a, `{"action":"query","user_ids":[1]}`)
	for {
		f := readFrame(t, a, 5*time.Second)
		if f.Type == "presence" {
			break
		}
	}
}
