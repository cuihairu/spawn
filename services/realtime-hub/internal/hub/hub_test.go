package hub

import (
	"sync"
	"testing"
)

func newTestHub(queue int) *Hub {
	return New(Options{QueueSize: queue, WriteWait: 1_000_000_000, PingPeriod: 2_000_000_000})
}

func clientFor(userId int64) *Client {
	return &Client{UserId: userId}
}

// TestRegisterAndLeaveCounts 入表/出表计数；出表后用户键清理。
func TestRegisterAndLeaveCounts(t *testing.T) {
	h := newTestHub(4)
	a := clientFor(7)
	b := clientFor(7)
	other := clientFor(8)

	h.Register(a)
	h.Register(b)
	h.Register(other)
	if h.Connections(7) != 2 || h.Connections(8) != 1 {
		t.Fatalf("counts = %d/%d, want 2/1", h.Connections(7), h.Connections(8))
	}

	h.Leave(a)
	if h.Connections(7) != 1 {
		t.Fatalf("after leave a: %d, want 1", h.Connections(7))
	}
	h.Leave(b) // 最后一条连接移除后键删除
	if h.Connections(7) != 0 {
		t.Fatalf("after leave b: %d, want 0", h.Connections(7))
	}
}

// TestLeaveIdempotentAndUnknown 重复出表与未知用户出表均安全。
func TestLeaveIdempotentAndUnknown(t *testing.T) {
	h := newTestHub(4)
	c := clientFor(7)
	h.Register(c)
	h.Leave(c)
	h.Leave(c) // 幂等
	h.Leave(clientFor(999))

	if h.Connections(7) != 0 {
		t.Fatalf("connections = %d, want 0", h.Connections(7))
	}
}

// TestSendToUsersFanout 同用户多连接全收、他用户不收、离线用户不计。
func TestSendToUsersFanout(t *testing.T) {
	h := newTestHub(4)
	a1, a2 := clientFor(7), clientFor(7)
	other := clientFor(8)
	for _, c := range []*Client{a1, a2, other} {
		h.Register(c)
	}

	frame := []byte(`{"type":"notification"}`)
	if got := h.SendToUsers([]int64{7, 8, 999}, frame); got != 3 {
		t.Fatalf("delivered = %d, want 3 (2 conns of 7 + 1 of 8)", got)
	}
	for name, c := range map[string]*Client{"a1": a1, "a2": a2, "other": other} {
		select {
		case got := <-c.send:
			if string(got) != string(frame) {
				t.Fatalf("%s frame = %s", name, got)
			}
		default:
			t.Fatalf("%s got no frame", name)
		}
	}
	// offline 未注册（无 send 通道），delivered=3 已证其未参与
	if h.Connections(7) != 2 {
		t.Fatalf("user 7 connections = %d, want 2", h.Connections(7))
	}
	if h.Drops() != 0 {
		t.Fatalf("drops = %d, want 0", h.Drops())
	}
}

// TestSendToUsersDropsOnFullQueue 队列满即丢弃并计数，不阻塞。
func TestSendToUsersDropsOnFullQueue(t *testing.T) {
	h := newTestHub(1)
	c := clientFor(7)
	h.Register(c)

	if got := h.SendToUsers([]int64{7}, []byte("first")); got != 1 {
		t.Fatalf("first delivered = %d, want 1", got)
	}
	if got := h.SendToUsers([]int64{7}, []byte("second")); got != 0 {
		t.Fatalf("overflow delivered = %d, want 0", got)
	}
	if h.Drops() != 1 {
		t.Fatalf("drops = %d, want 1", h.Drops())
	}
	// 队列里仍是首帧
	if got := string(<-c.send); got != "first" {
		t.Fatalf("queued frame = %q, want first", got)
	}
}

// TestConcurrentSendAndLeave 并发投递与出表不数据竞争（-race 门禁覆盖）。
func TestConcurrentSendAndLeave(t *testing.T) {
	h := newTestHub(8)
	clients := make([]*Client, 0, 16)
	for i := 0; i < 16; i++ {
		c := clientFor(int64(i%4) + 1)
		h.Register(c)
		clients = append(clients, c)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			h.SendToUsers([]int64{1, 2, 3, 4}, []byte("tick"))
		}
	}()
	go func() {
		defer wg.Done()
		for _, c := range clients {
			h.Leave(c)
		}
	}()
	wg.Wait()

	if total := len(h.conns); total != 0 {
		t.Fatalf("registry = %d users after all leaves, want 0", total)
	}
}
