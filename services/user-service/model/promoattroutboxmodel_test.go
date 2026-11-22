package model

// GP-403 渠道归因 outbox 模型单测：入队幂等、到期重投筛选、交付/失败/
// 耗尽留痕、终态行清理（镜像 analyticsoutboxmodel_test.go）。

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// newPromoAttrOutboxModel 打开临时文件 SQLite 并建归因 outbox 表。
func newPromoAttrOutboxModel(t *testing.T) *PromoAttrOutboxModel {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "promo-attr-outbox.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	m := NewPromoAttrOutboxModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create promo attr outbox tables: %v", err)
	}
	return m
}

func TestPromoAttrOutbox_EnqueueIdempotentAndListRetry(t *testing.T) {
	m := newPromoAttrOutboxModel(t)

	if err := m.Enqueue("attr:click-9:3", "click-9", 3, "2026-11-20T00:00:00Z",
		`{"click_id":"click-9"}`, "2026-11-12T09:00:00Z"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// 幂等：同 attr_key 重复入队忽略，不重置既有状态
	if err := m.Enqueue("attr:click-9:3", "click-9", 3, "2026-11-20T00:00:00Z",
		`{"click_id":"click-9"}`, "2026-11-12T09:00:00Z"); err != nil {
		t.Fatalf("enqueue replay: %v", err)
	}
	rows, err := m.ListRetry("2026-11-12T09:30:00Z", 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("due list = %d err=%v, want 1", len(rows), err)
	}
	if rows[0].AttrKey != "attr:click-9:3" || rows[0].ClickId != "click-9" ||
		rows[0].UserId != 3 || rows[0].Status != PromoAttrOutboxPending || rows[0].Attempts != 0 {
		t.Fatalf("row = %+v", rows[0])
	}
	// 未到期不重投（next_retry_at 在未来）
	rows, _ = m.ListRetry("2026-11-12T08:59:00Z", 100)
	if len(rows) != 0 {
		t.Fatalf("not-yet-due list = %d, want 0", len(rows))
	}
	// 失败后退避延期：再筛不命中，拨回过去即命中
	if err := m.MarkFailed("attr:click-9:3", 1, "boom", "2026-11-12T09:05:00Z"); err != nil {
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

func TestPromoAttrOutbox_DeliveredAndExhausted(t *testing.T) {
	m := newPromoAttrOutboxModel(t)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	_ = m.Enqueue("attr:click-1:1", "click-1", 1, "2026-11-20T00:00:00Z", `{}`, past)
	_ = m.Enqueue("attr:click-2:2", "click-2", 2, "2026-11-20T00:00:00Z", `{}`, past)

	// 交付：pending → delivered，退出重投队列
	if err := m.MarkDelivered("attr:click-1:1", nowUTC()); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	rows, _ := m.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 1 || rows[0].AttrKey != "attr:click-2:2" {
		t.Fatalf("after delivered = %+v, want only click-2", rows)
	}
	// 重复交付幂等（已非 pending，条件 UPDATE 不命中，无错误）
	if err := m.MarkDelivered("attr:click-1:1", nowUTC()); err != nil {
		t.Fatalf("re-deliver: %v", err)
	}

	// 重试耗尽：置 dropped 留痕，退出重投队列
	if err := m.MarkDropped("attr:click-2:2", 10, "give up"); err != nil {
		t.Fatalf("mark dropped: %v", err)
	}
	rows, _ = m.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 0 {
		t.Fatalf("after dropped = %d, want 0", len(rows))
	}

	// 终态清理：保留窗口（before）之前的删除，窗口内保留。MarkDelivered 只对
	// pending 生效（交付为一次性状态转移），已交付行的 delivered_at 需直接
	// 回拨模拟窗口过期。
	old := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	if _, err := m.db.Exec(`UPDATE promo_attribution_outbox SET delivered_at = ? WHERE attr_key = 'attr:click-1:1'`, old); err != nil {
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
