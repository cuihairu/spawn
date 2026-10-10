package model

import (
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"
)

// newClosedNotificationModel 建表后立即关闭底层库，专打各方法的错误返回路径。
func newClosedNotificationModel(t *testing.T) *NotificationModel {
	t.Helper()
	m := newNotificationTestDB(t)
	if err := m.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	return m
}

func TestCreateNotificationsTable_ClosedDBErrors(t *testing.T) {
	m := newClosedNotificationModel(t)
	if err := m.CreateNotificationsTable(); err == nil {
		t.Fatal("CreateNotificationsTable on closed db must error")
	}
}

func TestNotificationCreate_ClosedDBErrors(t *testing.T) {
	m := newClosedNotificationModel(t)
	if err := m.Create(notif(7, 8, "like_post", 100, false)); err == nil {
		t.Fatal("Create on closed db must error")
	}
}

func TestNotificationListByUser_ClosedDBErrors(t *testing.T) {
	m := newClosedNotificationModel(t)
	if _, _, err := m.ListByUser(7, 20, 0); err == nil {
		t.Fatal("ListByUser on closed db must error")
	}
}

func TestNotificationCountUnread_ClosedDBErrors(t *testing.T) {
	m := newClosedNotificationModel(t)
	if _, err := m.CountUnread(7); err == nil {
		t.Fatal("CountUnread on closed db must error")
	}
}

func TestNotificationMarkAllRead_ClosedDBErrors(t *testing.T) {
	m := newClosedNotificationModel(t)
	if err := m.MarkAllRead(7); err == nil {
		t.Fatal("MarkAllRead on closed db must error")
	}
}

// TestNotificationCreate_DedupAfterError 唯一键冲突静默忽略（INSERT OR IGNORE），
// 重复写入不报错。
func TestNotificationCreate_DedupAfterError(t *testing.T) {
	m := newNotificationTestDB(t)
	n := &types.Notification{
		UserId: 7, ActorId: 8, ActorName: "昵称", Type: "like_post",
		TargetId: 100, Content: "测试通知", CreatedAt: "2026-10-10T00:00:00Z",
	}
	if err := m.Create(n); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := m.Create(n); err != nil {
		t.Fatalf("duplicate create must be silent: %v", err)
	}
}
