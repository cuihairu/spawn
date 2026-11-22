package model

// GP-303 活跃上报 outbox 模型测试：入队幂等（act_key 唯一，重复入队不重置
// 既有退避/尝试计数）、到期扫描有界、状态转移（pending→delivered 单向；
// 终态行不再被 MarkFailed 触碰）、重试耗尽 dropped 留痕、保留窗口清理。

import (
	"testing"
	"time"
)

func aoDB(t *testing.T) *ActivityOutboxModel {
	t.Helper()
	m := NewActivityOutboxModel(openTestDB(t))
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create outbox tables: %v", err)
	}
	return m
}

func TestActivityOutbox_EnqueueIdempotentAndListRetry(t *testing.T) {
	m := aoDB(t)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	if err := m.Enqueue("act:u1:g1:t1", "u1", "g1", "t1", `{"a":1}`, past); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// 重复入队忽略（幂等），且不得把既有退避时间/尝试计数重置
	if err := m.Enqueue("act:u1:g1:t1", "u1", "g1", "t1", `{"a":1}`, future); err != nil {
		t.Fatalf("re-enqueue: %v", err)
	}
	rows, err := m.ListRetry(time.Now().UTC().Format(time.RFC3339Nano), 100)
	if err != nil {
		t.Fatalf("list retry: %v", err)
	}
	if len(rows) != 1 || rows[0].Status != ActivityOutboxPending || rows[0].Attempts != 0 {
		t.Fatalf("rows = %+v, want 1 条 pending attempts=0（重复入队忽略）", rows)
	}
	if rows[0].Payload != `{"a":1}` || rows[0].AccountId != "u1" || rows[0].ActiveAt != "t1" {
		t.Fatalf("row 内容 = %+v, want 载荷与事实字段原样保留", rows[0])
	}

	// 未到期的行不入扫描
	if err := m.Enqueue("act:u2:g1:t2", "u2", "g1", "t2", `{"a":2}`, future); err != nil {
		t.Fatalf("enqueue future: %v", err)
	}
	rows, err = m.ListRetry(time.Now().UTC().Format(time.RFC3339Nano), 100)
	if err != nil {
		t.Fatalf("list retry 2: %v", err)
	}
	if len(rows) != 1 || rows[0].ActKey != "act:u1:g1:t1" {
		t.Fatalf("rows = %+v, want 仅到期行", rows)
	}
}

func TestActivityOutbox_StateTransitions(t *testing.T) {
	m := aoDB(t)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	_ = m.Enqueue("act:u1:g1:t1", "u1", "g1", "t1", `{"a":1}`, past)

	// MarkDelivered：pending → delivered（幂等，重复调用不报错）
	if err := m.MarkDelivered("act:u1:g1:t1", past); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	if err := m.MarkDelivered("act:u1:g1:t1", past); err != nil {
		t.Fatalf("mark delivered twice: %v", err)
	}
	rows, _ := m.ListRetry(time.Now().UTC().Format(time.RFC3339Nano), 100)
	if len(rows) != 0 {
		t.Fatalf("delivered 行仍出现在重投扫描: %+v", rows)
	}

	// 终态行不再被 MarkFailed 触碰
	if err := m.MarkFailed("act:u1:g1:t1", 3, "boom", past); err != nil {
		t.Fatalf("mark failed on delivered: %v", err)
	}
	if err := m.MarkDelivered("act:u1:g1:t1", past); err != nil {
		t.Fatalf("mark delivered after failed: %v", err)
	}
	_ = m.Enqueue("act:u1:g1:t2", "u1", "g1", "t2", `{"a":2}`, past)
	if err := m.MarkFailed("act:u1:g1:t2", 2, "boom", future); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	rows, _ = m.ListRetry(time.Now().UTC().Format(time.RFC3339Nano), 100)
	if len(rows) != 0 {
		t.Fatalf("退避窗口内的失败行仍被扫描: %+v", rows)
	}
	rows, _ = m.ListRetry(future, 100)
	if len(rows) != 1 || rows[0].Attempts != 2 || rows[0].LastError != "boom" {
		t.Fatalf("rows = %+v, want attempts=2 last_error=boom", rows)
	}

	// 重试耗尽置 dropped 留痕（不再进重投扫描）
	if err := m.MarkDropped("act:u1:g1:t2", 10, "exhausted"); err != nil {
		t.Fatalf("mark dropped: %v", err)
	}
	rows, _ = m.ListRetry(time.Now().UTC().Format(time.RFC3339Nano), 100)
	if len(rows) != 0 {
		t.Fatalf("dropped 行仍出现在重投扫描: %+v", rows)
	}
}

func TestActivityOutbox_Prune(t *testing.T) {
	db := openTestDB(t)
	m := NewActivityOutboxModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create outbox tables: %v", err)
	}
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	recent := time.Now().UTC().Format(time.RFC3339Nano)
	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano)

	_ = m.Enqueue("act:u1:g1:old", "u1", "g1", "old", `{"a":1}`, old)
	_ = m.Enqueue("act:u1:g1:recent", "u1", "g1", "recent", `{"a":2}`, old)
	_ = m.Enqueue("act:u1:g1:dropped", "u1", "g1", "dropped", `{"a":3}`, future)
	// 把 dropped 行的 created_at 拨回 48 小时前（Enqueue 恒为当前时刻，
	// Prune 的 dropped 分支按 created_at 判定）
	if _, err := db.Exec(`UPDATE player_activity_outbox SET created_at = ? WHERE act_key = ?`, old, "act:u1:g1:dropped"); err != nil {
		t.Fatalf("rewind dropped created_at: %v", err)
	}

	// 旧 delivered 清理；新 delivered 与 pending 不清理；旧 dropped 清理
	_ = m.MarkDelivered("act:u1:g1:old", old)
	_ = m.MarkDelivered("act:u1:g1:recent", recent)
	_ = m.MarkDropped("act:u1:g1:dropped", 10, "exhausted")

	n, err := m.Prune(cutoff)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 2 {
		t.Fatalf("pruned = %d, want 2（旧 delivered + 旧 dropped）", n)
	}
	_ = m.Enqueue("act:u1:g1:pend", "u1", "g1", "pend", `{"a":4}`, old)
	rows, _ := m.ListRetry(time.Now().UTC().Format(time.RFC3339Nano), 100)
	if len(rows) != 1 || rows[0].ActKey != "act:u1:g1:pend" {
		t.Fatalf("rows = %+v, want 仅 pending 行留存", rows)
	}
}
