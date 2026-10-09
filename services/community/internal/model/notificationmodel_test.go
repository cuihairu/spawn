package model

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"
)

func newNotificationTestDB(t *testing.T) *NotificationModel {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "community.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	m := NewNotificationModel(db)
	if err := m.CreateNotificationsTable(); err != nil {
		t.Fatalf("create notifications table: %v", err)
	}
	return m
}

func notif(recipient, actor int64, typ string, target int64, read bool) *types.Notification {
	return &types.Notification{
		UserId: recipient, ActorId: actor, ActorName: "昵称", Type: typ,
		TargetId: target, Content: "测试通知", IsRead: read, CreatedAt: "2026-10-10T00:00:00Z",
	}
}

func TestCreateNotificationsTable_Twice(t *testing.T) {
	m := newNotificationTestDB(t)
	if err := m.CreateNotificationsTable(); err != nil {
		t.Fatalf("second create should be no-op: %v", err)
	}
}

func TestNotificationCreate_DedupsSameAction(t *testing.T) {
	m := newNotificationTestDB(t)

	for i := 0; i < 2; i++ {
		if err := m.Create(notif(7, 8, "like_post", 100, false)); err != nil {
			t.Fatalf("create #%d: %v", i+1, err)
		}
	}
	list, total, err := m.ListByUser(7, 20, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("total=%d len=%d, want 1 (unique key should dedup)", total, len(list))
	}
}

func TestNotificationListByUser_OrderAndPaging(t *testing.T) {
	m := newNotificationTestDB(t)

	// 收件人 7 三条（不同 actor/target/type 不去重），收件人 8 一条互不串扰
	seed := []struct {
		recipient, actor, target int64
		typ                      string
	}{
		{7, 8, 101, "like_post"},
		{7, 9, 101, "comment_post"},
		{7, 8, 102, "comment_post"},
		{8, 7, 103, "like_post"},
	}
	for _, s := range seed {
		if err := m.Create(notif(s.recipient, s.actor, s.typ, s.target, false)); err != nil {
			t.Fatalf("seed %v: %v", s, err)
		}
	}

	list, total, err := m.ListByUser(7, 20, 0)
	if err != nil || total != 3 || len(list) != 3 {
		t.Fatalf("list total=%d len=%d err=%v, want 3", total, len(list), err)
	}
	// id 倒序：最新在前
	if list[0].TargetId != 102 || list[2].TargetId != 101 {
		t.Fatalf("order mismatch: %+v", list)
	}

	page, total, err := m.ListByUser(7, 2, 2)
	if err != nil || total != 3 || len(page) != 1 || page[0].TargetId != 101 {
		t.Fatalf("page2 total=%d len=%d err=%v, want tail item", total, len(page), err)
	}
}

func TestNotificationUnreadAndMarkAllRead(t *testing.T) {
	m := newNotificationTestDB(t)

	if err := m.Create(notif(7, 8, "like_post", 1, false)); err != nil {
		t.Fatalf("seed1: %v", err)
	}
	if err := m.Create(notif(7, 9, "follow_user", 7, false)); err != nil {
		t.Fatalf("seed2: %v", err)
	}

	count, err := m.CountUnread(7)
	if err != nil || count != 2 {
		t.Fatalf("unread=%d err=%v, want 2", count, err)
	}

	if err := m.MarkAllRead(7); err != nil {
		t.Fatalf("mark all read: %v", err)
	}
	count, err = m.CountUnread(7)
	if err != nil || count != 0 {
		t.Fatalf("unread after mark=%d err=%v, want 0", count, err)
	}

	// 幂等：再次执行不报错
	if err := m.MarkAllRead(7); err != nil {
		t.Fatalf("second mark all read: %v", err)
	}
}
