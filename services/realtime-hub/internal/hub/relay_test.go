package hub

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// newRelayedHub 起一个挂 RedisRelay 的 hub（deliver 接本 hub 本地投递）。
func newRelayedHub(t *testing.T, addr string) (*Hub, *RedisRelay) {
	t.Helper()
	h := New(Options{QueueSize: 8, WriteWait: time.Second, PingPeriod: 2 * time.Second})
	relay, err := NewRedisRelay(addr, "", RedisChannel, h.DeliverLocal)
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}
	h.SetRelay(relay)
	t.Cleanup(func() { _ = relay.Close() })
	return h, relay
}

// recvFrame 带 deadline 收一帧（收到非目标帧继续等）。
func recvFrame(t *testing.T, c *Client) []byte {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		select {
		case data := <-c.send:
			return data
		case <-time.After(time.Until(deadline)):
			t.Fatal("no frame within 2s")
			return nil
		}
	}
}

// TestRedisRelayCrossDelivery 实例 B 投递 → 频道 → 实例 A 本地连接收帧；
// 发布方自身不回环（A 本地投递只产生一帧）。
func TestRedisRelayCrossDelivery(t *testing.T) {
	mr := miniredisRunT(t)
	hubA, _ := newRelayedHub(t, mr.Addr())
	hubB, _ := newRelayedHub(t, mr.Addr())

	client := &Client{UserId: 7}
	hubA.Register(client)

	frame := mustFrame(t, "notification", map[string]string{"k": "v"})

	// B 发：A 的连接收一帧（跨实例），B 本地无连接 delivered=0
	if n := hubB.SendToUsers([]int64{7}, frame); n != 0 {
		t.Fatalf("B delivered = %d, want 0", n)
	}
	got := recvFrame(t, client)
	var f Frame
	if err := json.Unmarshal(got, &f); err != nil || f.Type != "notification" {
		t.Fatalf("frame = %s (%v)", got, err)
	}

	// A 本地发：本地投递一帧，自身回环被剔除，不得二次入队
	if n := hubA.SendToUsers([]int64{7}, frame); n != 1 {
		t.Fatalf("A delivered = %d, want 1", n)
	}
	_ = recvFrame(t, client)
	select {
	case extra := <-client.send:
		t.Fatalf("unexpected extra frame %s (self loop not skipped)", extra)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestRedisRelayNilUsers 空 user_ids 广播不 panic（畸形信封防线外溢）。
func TestRedisRelayNilUsers(t *testing.T) {
	mr := miniredisRunT(t)
	hubA, relay := newRelayedHub(t, mr.Addr())
	relay.Broadcast(nil, []byte("{}"))
	if n := hubA.SendToUsers(nil, []byte("{}")); n != 0 {
		t.Fatalf("delivered = %d, want 0", n)
	}
}

// TestNewRedisRelayBadAddr 坏地址启动期报错（不做静默半可用实例）。
func TestNewRedisRelayBadAddr(t *testing.T) {
	if _, err := NewRedisRelay("127.0.0.1:1", "", RedisChannel, func([]int64, []byte) {}); err == nil {
		t.Fatal("expected ping error for dead redis")
	}
}

// miniredisRunT 起进程内 Redis（测试专用，支持 pub/sub）。
func miniredisRunT(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	mr := miniredis.RunT(t)
	return mr
}

// mustFrame 组一帧（type + data 包装，与 publish handler 同形）。
func mustFrame(t *testing.T, typ string, data any) []byte {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	frame, err := json.Marshal(Frame{Type: typ, Data: raw})
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	return frame
}
