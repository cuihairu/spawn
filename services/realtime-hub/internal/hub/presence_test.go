package hub

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type presenceFrame struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func drainFrame(t *testing.T, c *Client) presenceFrame {
	t.Helper()
	var f presenceFrame
	select {
	case raw := <-c.send:
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatalf("frame %s: %v", raw, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no frame within 2s")
	}
	return f
}

// TestPresenceOnlineOnFirstConnOfflineOnLast 首连发 online、末断发 offline，
// 多端第二连/中间断不触发转换沿事件。
func TestPresenceOnlineOnFirstConnOfflineOnLast(t *testing.T) {
	h := newTestHub(4)
	watcher := clientFor(1)
	h.Register(watcher)
	h.Watch(watcher, []int64{7})

	first := clientFor(7)
	if !h.Register(first) {
		t.Fatal("first conn must report online transition")
	}
	f := drainFrame(t, watcher)
	if f.Type != "presence" || !containsOnline(t, f.Data, 7, true) {
		t.Fatalf("frame = %s/%s, want presence online 7", f.Type, f.Data)
	}

	// 第二端上线：不触发事件
	second := clientFor(7)
	if h.Register(second) {
		t.Fatal("second conn must not report transition")
	}
	select {
	case raw := <-watcher.send:
		t.Fatalf("unexpected frame on second conn: %s", raw)
	case <-time.After(2 * time.Second):
	}

	// 第一端断开：仍在线，不触发
	h.Leave(first)
	select {
	case raw := <-watcher.send:
		t.Fatalf("unexpected frame on mid disconnect: %s", raw)
	case <-time.After(2 * time.Second):
	}

	// 最后连接断开：offline（第二端先断不触发，末断才触发）
	last := clientFor(7)
	h.Register(last)
	h.Leave(second)
	h.Leave(last)
	f = drainFrame(t, watcher)
	if !containsOnline(t, f.Data, 7, false) {
		t.Fatalf("frame = %s, want presence offline 7", f.Data)
	}
}

// TestWatchDedupAndCap watch 幂等；超上限静默截断。
func TestWatchDedupAndCap(t *testing.T) {
	h := newTestHub(4)
	c := clientFor(1)
	h.Register(c)

	ids := make([]int64, maxWatchPerClient+50)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	h.Watch(c, ids) // 截断至 200
	if len(c.watching) != maxWatchPerClient {
		t.Fatalf("watching = %d, want cap %d", len(c.watching), maxWatchPerClient)
	}
	h.Watch(c, []int64{1, 2}) // 重复幂等
	if len(c.watching) != maxWatchPerClient {
		t.Fatalf("duplicate watch grew set: %d", len(c.watching))
	}
	if len(h.watchers[1]) != 1 {
		t.Fatalf("watchers[7] size = %d, want 1", len(h.watchers[1]))
	}
}

// TestLeaveCleansWatch 断连自动清除 watch 关系（事件不再送达）。
func TestLeaveCleansWatch(t *testing.T) {
	h := newTestHub(4)
	watcher := clientFor(1)
	h.Register(watcher)
	h.Watch(watcher, []int64{7})
	h.Leave(watcher)
	if len(h.watchers[7]) != 0 || len(watcher.watching) != 0 {
		t.Fatalf("watch not cleaned: watchers=%v watching=%v", h.watchers[7], watcher.watching)
	}

	h.Register(clientFor(7)) // 上线事件：无人应收
	select {
	case raw := <-watcher.send:
		t.Fatalf("dead watcher got frame: %s", raw)
	case <-time.After(2 * time.Second):
	}
}

// TestPresenceSnapshot 混合快照：在线 2 连接带 since、离线 0，重复 id 去重。
func TestPresenceSnapshot(t *testing.T) {
	h := newTestHub(4)
	h.Register(clientFor(7))
	h.Register(clientFor(7))
	h.Register(clientFor(8))

	got := h.Presence([]int64{7, 7, 8, 9})
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (dup dropped)", len(got))
	}
	if !got[0].Online || got[0].Connections != 2 || got[0].Since == "" {
		t.Fatalf("user 7 = %+v, want online 2 conns with since", got[0])
	}
	if !got[1].Online || got[1].Connections != 1 {
		t.Fatalf("user 8 = %+v, want online 1 conn (still registered)", got[1])
	}
	if got[2].Online || got[2].UserId != 9 {
		t.Fatalf("user 9 = %+v, want offline", got[2])
	}

	// 上限截断
	big := make([]int64, maxIDList+10)
	for i := range big {
		big[i] = int64(i)
	}
	if got := h.Presence(big); len(got) != maxIDList {
		t.Fatalf("Presence cap = %d, want %d", len(got), maxIDList)
	}
}

// TestReplyPresence 只回请求连接一帧快照。
func TestReplyPresence(t *testing.T) {
	h := newTestHub(4)
	requester := clientFor(1)
	other := clientFor(2)
	h.Register(requester)
	h.Register(other)
	h.Register(clientFor(7))

	h.replyPresence(requester, []int64{7, 2})
	f := drainFrame(t, requester)
	if f.Type != "presence" {
		t.Fatalf("type = %q, want presence", f.Type)
	}
	var body struct {
		Items []PresenceStatus `json:"items"`
	}
	if err := json.Unmarshal(f.Data, &body); err != nil {
		t.Fatalf("data %s: %v", f.Data, err)
	}
	if len(body.Items) != 2 || !body.Items[0].Online || body.Items[1].Connections != 1 {
		t.Fatalf("items = %+v", body.Items)
	}
	select {
	case raw := <-other.send:
		t.Fatalf("frame leaked to non-requester: %s", raw)
	case <-time.After(2 * time.Second):
	}
}

// TestUnwatch 解除后不再收事件。
func TestUnwatch(t *testing.T) {
	h := newTestHub(4)
	watcher := clientFor(1)
	h.Register(watcher)
	h.Watch(watcher, []int64{7})
	h.Unwatch(watcher, []int64{7})
	h.Unwatch(watcher, []int64{7}) // 幂等
	h.Register(clientFor(7))
	select {
	case raw := <-watcher.send:
		t.Fatalf("unwatched user still got frame: %s", raw)
	case <-time.After(2 * time.Second):
	}
}

// TestConcurrentWatchAndLeave 并发 watch/unwatch/register/leave 无数据竞争。
func TestConcurrentWatchAndLeave(t *testing.T) {
	h := newTestHub(8)
	clients := make([]*Client, 0, 12)
	for i := 0; i < 12; i++ {
		c := clientFor(int64(i%3) + 1)
		h.Register(c)
		h.Watch(c, []int64{1, 2, 3})
		clients = append(clients, c)
	}
	var wg sync.WaitGroup
	for i, c := range clients {
		wg.Add(1)
		go func(i int, c *Client) {
			defer wg.Done()
			if i%2 == 0 {
				h.Watch(c, []int64{4})
				h.Unwatch(c, []int64{1})
			} else {
				h.Leave(c)
			}
		}(i, c)
	}
	wg.Wait()
}

func containsOnline(t *testing.T, data json.RawMessage, userId int64, online bool) bool {
	t.Helper()
	var evt presenceEvt
	if err := json.Unmarshal(data, &evt); err != nil {
		t.Fatalf("unmarshal %s: %v", data, err)
	}
	return evt.UserId == userId && evt.Online == online
}
