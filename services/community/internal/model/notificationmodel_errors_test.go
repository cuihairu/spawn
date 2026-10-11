package model

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"

	_ "github.com/mattn/go-sqlite3"
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

// newRawNotificationModel 建临时库并按给定 schema 建表（不走 CreateNotificationsTable），
// 供缺列故障注入用例使用——关库会让先执行的 COUNT 先失败，够不到列表查询分支。
func newRawNotificationModel(t *testing.T, schema string, inserts ...string) *NotificationModel {
	t.Helper()

	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "community.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("exec schema: %v", err)
	}
	for _, q := range inserts {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	return NewNotificationModel(db)
}

// TestNotificationListByUser_MissingColumnQueryFails 仅有 user_id 的表：
// COUNT(*) 正常、具名列 SELECT 报 no such column → 列表查询错误分支。
func TestNotificationListByUser_MissingColumnQueryFails(t *testing.T) {
	m := newRawNotificationModel(t,
		`CREATE TABLE notifications (id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL)`,
		`INSERT INTO notifications (id, user_id) VALUES (1, 7)`)

	if _, _, err := m.ListByUser(7, 20, 0); err == nil {
		t.Fatal("ListByUser on column-missing table must error")
	}
}

// TestNotificationListByUser_DirtyRowScanFails 列齐全但 is_read 存文本：
// COUNT 与 SELECT 均成功，Scan 到 int64 失败 → 行扫描错误分支。
func TestNotificationListByUser_DirtyRowScanFails(t *testing.T) {
	m := newNotificationTestDB(t)
	if _, err := m.db.Exec(`INSERT INTO notifications
		(id, user_id, actor_id, actor_name, type, target_id, content, is_read, created_at)
		VALUES (1, 7, 8, '昵称', 'like_post', 100, 'c', 'x', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert dirty row: %v", err)
	}

	if _, _, err := m.ListByUser(7, 20, 0); err == nil {
		t.Fatal("ListByUser with dirty row must error")
	}
}
