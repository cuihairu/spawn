package model

// GP-404 业务通知 outbox 模型单测：入队幂等、到期重投筛选、交付/失败/
// 耗尽留痕、终态行清理（与 analytics outbox 同款口径）。

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// newNotifyOutboxModel 打开临时文件 SQLite 并建 outbox 表。
func newNotifyOutboxModel(t *testing.T) *NotifyOutboxModel {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "notify-outbox.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	m := NewNotifyOutboxModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create notify outbox tables: %v", err)
	}
	return m
}

func TestNotifyOutbox_EnqueueIdempotentAndListRetry(t *testing.T) {
	m := newNotifyOutboxModel(t)

	if err := m.Enqueue("notify:abc", "statement_dispute_resolved", `{"user_ids":[7]}`, "2026-11-12T09:00:00Z"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// 幂等：同 notify_key 重复入队忽略，不重置既有状态
	if err := m.Enqueue("notify:abc", "statement_dispute_resolved", `{"user_ids":[7]}`, "2026-11-12T09:00:00Z"); err != nil {
		t.Fatalf("enqueue replay: %v", err)
	}
	rows, err := m.ListRetry("2026-11-12T09:30:00Z", 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("due list = %d err=%v, want 1", len(rows), err)
	}
	if rows[0].NotifyKey != "notify:abc" || rows[0].Status != NotifyOutboxPending || rows[0].Attempts != 0 {
		t.Fatalf("row = %+v", rows[0])
	}
	// 未到期不重投（next_retry_at 在未来）
	rows, _ = m.ListRetry("2026-11-12T08:59:00Z", 100)
	if len(rows) != 0 {
		t.Fatalf("not-yet-due list = %d, want 0", len(rows))
	}
	// 失败后退避延期：再筛不命中，拨回过去即命中
	if err := m.MarkFailed("notify:abc", 1, "boom", "2026-11-12T09:05:00Z"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	rows, _ = m.ListRetry("2026-11-12T09:03:00Z", 100)
	if len(rows) != 0 {
		t.Fatalf("backoff list = %d, want 0", len(rows))
	}
	rows, _ = m.ListRetry("2026-11-12T09:05:00Z", 100)
	if len(rows) != 1 || rows[0].Attempts != 1 || rows[0].LastError != "boom" {
		t.Fatalf("due after backoff = %+v, want attempts=1", rows)
	}
}

func TestNotifyOutbox_DeliveredAndExhausted(t *testing.T) {
	m := newNotifyOutboxModel(t)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	_ = m.Enqueue("notify:ok", "statement_dispute_resolved", `{"user_ids":[7]}`, past)
	_ = m.Enqueue("notify:bad", "statement_dispute_resolved", `{"user_ids":[7]}`, past)

	// 交付：pending → delivered，退出重投队列
	if err := m.MarkDelivered("notify:ok", nowUTC()); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	rows, _ := m.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 1 || rows[0].NotifyKey != "notify:bad" {
		t.Fatalf("after delivered = %+v, want only notify:bad", rows)
	}
	// 重复交付幂等（已非 pending，条件 UPDATE 不命中，无错误）
	if err := m.MarkDelivered("notify:ok", nowUTC()); err != nil {
		t.Fatalf("re-deliver: %v", err)
	}

	// 重试耗尽：置 dropped 留痕，退出重投队列
	if err := m.MarkDropped("notify:bad", 10, "give up"); err != nil {
		t.Fatalf("mark dropped: %v", err)
	}
	rows, _ = m.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 0 {
		t.Fatalf("after dropped = %d, want 0", len(rows))
	}

	// 终态清理：保留窗口（before）之前的删除，窗口内保留。
	old := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	if _, err := m.db.Exec(`UPDATE notify_outbox SET delivered_at = ? WHERE notify_key = 'notify:ok'`, old); err != nil {
		t.Fatalf("backdate delivered: %v", err)
	}
	n, err := m.Prune(time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339))
	if err != nil || n != 1 {
		t.Fatalf("prune old = %d err=%v, want 1", n, err)
	}
	n, _ = m.Prune(old)
	if n != 0 {
		t.Fatalf("prune nothing = %d, want 0", n)
	}
}
