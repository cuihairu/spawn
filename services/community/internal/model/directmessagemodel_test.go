package model

import (
	"path/filepath"
	"testing"

	"database/sql"

	"github.com/tappi/tappi/services/community/internal/types"
)

func newDirectMessageTestDB(t *testing.T) *DirectMessageModel {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "community.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	m := NewDirectMessageModel(db)
	if err := m.CreateDirectMessagesTable(); err != nil {
		t.Fatalf("create direct_messages table: %v", err)
	}
	return m
}

func dmMsg(sender, recipient int64, content string) *types.DirectMessage {
	return &types.DirectMessage{
		SenderId: sender, RecipientId: recipient, Content: content,
		CreatedAt: "2026-10-12T00:00:00Z",
	}
}

// TestDirectMessageSendAssignsId Send 回填自增 id；重复建表幂等。
func TestDirectMessageSendAssignsId(t *testing.T) {
	m := newDirectMessageTestDB(t)
	if err := m.CreateDirectMessagesTable(); err != nil {
		t.Fatalf("recreate table: %v", err)
	}

	msg := dmMsg(7, 8, "你好")
	if err := m.Send(msg); err != nil {
		t.Fatalf("send: %v", err)
	}
	if msg.Id <= 0 {
		t.Fatalf("id not backfilled: %d", msg.Id)
	}
}

// TestDirectMessageThread 会话往来倒序 + total + 双方可见同一线。
func TestDirectMessageThread(t *testing.T) {
	m := newDirectMessageTestDB(t)
	for _, s := range []struct{ from, to int64 }{{7, 8}, {8, 7}, {7, 8}, {9, 8}} {
		if err := m.Send(dmMsg(s.from, s.to, "m")); err != nil {
			t.Fatalf("send: %v", err)
		}
	}

	list, total, err := m.Thread(7, 8, 50, 0)
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	if total != 3 || len(list) != 3 {
		t.Fatalf("total=%d len=%d, want 3/3", total, len(list))
	}
	// 倒序：最新在前；且第三条来自 peer 9 的消息不掺入
	if list[0].SenderId != 7 || list[2].SenderId != 7 {
		t.Fatalf("order mismatch: %+v", list)
	}

	// 分页：limit 1 offset 1 → 中间一条
	page, total, err := m.Thread(7, 8, 1, 1)
	if err != nil || total != 3 || len(page) != 1 {
		t.Fatalf("page: total=%d len=%d err=%v", total, len(page), err)
	}
	if page[0].Id != list[1].Id {
		t.Fatalf("page offset mismatch")
	}
}

// TestDirectMessageConversations 会话聚合：末条消息、未读数、活跃倒序、隔离。
func TestDirectMessageConversations(t *testing.T) {
	m := newDirectMessageTestDB(t)
	send := func(from, to int64, content string) {
		t.Helper()
		if err := m.Send(dmMsg(from, to, content)); err != nil {
			t.Fatalf("send: %v", err)
		}
	}
	send(8, 7, "早")  // peer 8 → 7
	send(7, 8, "早！") // peer 8 会话更新
	send(9, 7, "在吗") // peer 9 → 7（未读）
	send(7, 9, "在")  // peer 9 会话更新（我最后发言）

	convs, err := m.Conversations(7)
	if err != nil {
		t.Fatalf("conversations: %v", err)
	}
	if len(convs) != 2 {
		t.Fatalf("len=%d, want 2: %+v", len(convs), convs)
	}
	// 活跃倒序：9 的会话（最后动作更新）在前
	if convs[0].PeerId != 9 || convs[1].PeerId != 8 {
		t.Fatalf("peer order: %+v", convs)
	}
	// 末条消息与未读数（7 收到的两条都未读）
	if convs[0].LastMessage != "在" || convs[0].Unread != 1 {
		t.Fatalf("conv 9: %+v", convs[0])
	}
	if convs[1].LastMessage != "早！" || convs[1].Unread != 1 {
		t.Fatalf("conv 8: %+v", convs[1])
	}

	// 对端视角：8 看到 7 的会话，未读 1（"早！"是 7 发的）
	conv8, err := m.Conversations(8)
	if err != nil || len(conv8) != 1 || conv8[0].PeerId != 7 || conv8[0].Unread != 1 {
		t.Fatalf("conv8: %+v err=%v", conv8, err)
	}

	// 无会话用户 → 空非 nil
	empty, err := m.Conversations(42)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty: %#v err=%v", empty, err)
	}
}

// TestDirectMessageMarkPeerRead 翻转仅收件方行；幂等；未读数联动。
func TestDirectMessageMarkPeerRead(t *testing.T) {
	m := newDirectMessageTestDB(t)
	if err := m.Send(dmMsg(8, 7, "a")); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := m.Send(dmMsg(8, 7, "b")); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := m.Send(dmMsg(7, 8, "c")); err != nil {
		t.Fatalf("send: %v", err)
	}

	if n, err := m.CountUnread(7); err != nil || n != 2 {
		t.Fatalf("unread 7 = %d err=%v, want 2", n, err)
	}

	// 首次置已读 → 翻转 2 行（我发的 "c" 不受影响）
	n, err := m.MarkPeerRead(7, 8)
	if err != nil || n != 2 {
		t.Fatalf("mark read: n=%d err=%v, want 2", n, err)
	}
	// 幂等重标 → 0 行
	if n, err := m.MarkPeerRead(7, 8); err != nil || n != 0 {
		t.Fatalf("re-mark: n=%d err=%v, want 0", n, err)
	}
	if n, err := m.CountUnread(7); err != nil || n != 0 {
		t.Fatalf("unread 7 after = %d err=%v, want 0", n, err)
	}
	// 发件方视角未读不受收件方已读影响
	if n, err := m.CountUnread(8); err != nil || n != 1 {
		t.Fatalf("unread 8 = %d err=%v, want 1", n, err)
	}
}
