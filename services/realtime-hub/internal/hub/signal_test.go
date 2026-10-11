package hub

import (
	"encoding/json"
	"testing"
	"time"
)

// signalTo 组上行 signal 控制消息字节。
func signalTo(t *testing.T, to int64, room string, data any) []byte {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	b, err := json.Marshal(clientMsg{Action: "signal", To: to, Room: room, Data: raw})
	if err != nil {
		t.Fatalf("marshal msg: %v", err)
	}
	return b
}

// expectSignal 带超时收一帧并断言是目标信令；返回 (from, room, data)。
func expectSignal(t *testing.T, c *Client) (int64, string, json.RawMessage) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case raw := <-c.send:
			var f Frame
			if err := json.Unmarshal(raw, &f); err != nil {
				t.Fatalf("frame %s: %v", raw, err)
			}
			if f.Type != signalFrameType {
				continue
			}
			var p signalPayload
			if err := json.Unmarshal(f.Data, &p); err != nil {
				t.Fatalf("signal payload %s: %v", f.Data, err)
			}
			return p.From, p.Room, p.Data
		case <-deadline:
			t.Fatal("no signal frame within 2s")
			return 0, "", nil
		}
	}
}

func assertNoFrame(t *testing.T, c *Client) {
	t.Helper()
	select {
	case extra := <-c.send:
		t.Fatalf("unexpected frame %s", extra)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestSignalRelayBetweenUsers A → B 转发（from/room/data 原样），发端不收。
func TestSignalRelayBetweenUsers(t *testing.T) {
	h := New(Options{QueueSize: 8, WriteWait: time.Second, PingPeriod: 2 * time.Second})
	a := &Client{UserId: 1}
	b := &Client{UserId: 2}
	h.Register(a)
	h.Register(b)

	h.handleClientMsg(a, signalTo(t, 2, "room-1", map[string]string{"type": "offer"}))

	from, room, data := expectSignal(t, b)
	if from != 1 || room != "room-1" {
		t.Fatalf("from=%d room=%q", from, room)
	}
	var body map[string]string
	if err := json.Unmarshal(data, &body); err != nil || body["type"] != "offer" {
		t.Fatalf("data = %s (%v)", data, err)
	}
	assertNoFrame(t, a)
}

// TestSignalSelfMultiDevice 自信号：本人其他连接收，发端连接不回环。
func TestSignalSelfMultiDevice(t *testing.T) {
	h := New(Options{QueueSize: 8, WriteWait: time.Second, PingPeriod: 2 * time.Second})
	a1 := &Client{UserId: 5}
	a2 := &Client{UserId: 5}
	h.Register(a1)
	h.Register(a2)

	h.handleClientMsg(a1, signalTo(t, 5, "", map[string]int{"ice": 1}))
	expectSignal(t, a2)
	assertNoFrame(t, a1)
}

// TestSignalInvalidIgnored 非法目标静默忽略：不 panic、无帧、hub 可用。
func TestSignalInvalidIgnored(t *testing.T) {
	h := New(Options{QueueSize: 8, WriteWait: time.Second, PingPeriod: 2 * time.Second})
	a := &Client{UserId: 1}
	h.Register(a)

	h.handleClientMsg(a, signalTo(t, 0, "", nil))
	h.handleClientMsg(a, signalTo(t, -3, "", nil))
	assertNoFrame(t, a)

	// 目标离线：静默丢弃，发端仍可继续用
	h.handleClientMsg(a, signalTo(t, 42, "", nil))
	assertNoFrame(t, a)
	if h.Connections(1) != 1 {
		t.Fatalf("conn lost, connections = %d", h.Connections(1))
	}
}

// TestSignalCrossInstanceViaRelay Relay 模式：目标在他实例时兜底广播送达。
func TestSignalCrossInstanceViaRelay(t *testing.T) {
	mr := miniredisRunT(t)
	hubA, _ := newRelayedHub(t, mr.Addr())
	hubB, _ := newRelayedHub(t, mr.Addr())

	bConn := &Client{UserId: 2}
	hubB.Register(bConn)
	aConn := &Client{UserId: 1}
	hubA.Register(aConn)

	// A 实例发 signal 给用户 2（只在 B 实例在线）
	hubA.handleClientMsg(aConn, signalTo(t, 2, "room-x", map[string]string{"t": "answer"}))
	expectSignal(t, bConn)
}
