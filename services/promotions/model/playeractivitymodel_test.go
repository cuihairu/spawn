package model

// GP-303 回流判定源模型测试：活跃事实条件推进——首见插入（previous 空）、
// 后到推进（previous=旧 last）、重复/乱序重放 no-op（不把 last 回退、不把
// previous 覆盖坏）、查询不存在。

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func paDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "pa.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := NewPlayerActivityModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	return db
}

func rfc(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func TestPlayerActivity_FirstInsertHasNoPrevious(t *testing.T) {
	db := paDB(t)
	m := NewPlayerActivityModel(db)
	now := rfc(time.Now())
	if err := m.RecordActivity("u1", "g1", now); err != nil {
		t.Fatalf("record first: %v", err)
	}
	row, err := m.GetPlayerActivity("u1", "g1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.LastActiveAt != now {
		t.Fatalf("last = %q want %q", row.LastActiveAt, now)
	}
	if row.PreviousActiveAt != "" {
		t.Fatalf("previous = %q want empty (首见无历史)", row.PreviousActiveAt)
	}
}

func TestPlayerActivity_AdvanceMovesPrevious(t *testing.T) {
	db := paDB(t)
	m := NewPlayerActivityModel(db)
	t1 := rfc(time.Now().Add(-40 * 24 * time.Hour))
	t2 := rfc(time.Now())
	if err := m.RecordActivity("u1", "g1", t1); err != nil {
		t.Fatalf("record t1: %v", err)
	}
	if err := m.RecordActivity("u1", "g1", t2); err != nil {
		t.Fatalf("record t2: %v", err)
	}
	row, err := m.GetPlayerActivity("u1", "g1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.LastActiveAt != t2 {
		t.Fatalf("last = %q want %q", row.LastActiveAt, t2)
	}
	if row.PreviousActiveAt != t1 {
		t.Fatalf("previous = %q want %q（后到推进 previous=旧 last）", row.PreviousActiveAt, t1)
	}
}

func TestPlayerActivity_ReplayIsNoop(t *testing.T) {
	db := paDB(t)
	m := NewPlayerActivityModel(db)
	t1 := rfc(time.Now().Add(-10 * 24 * time.Hour))
	t2 := rfc(time.Now())
	if err := m.RecordActivity("u1", "g1", t1); err != nil {
		t.Fatalf("record t1: %v", err)
	}
	if err := m.RecordActivity("u1", "g1", t2); err != nil {
		t.Fatalf("record t2: %v", err)
	}
	// 重放 t1（旧）与 t2（相同）都不得回退 last / 覆盖 previous
	if err := m.RecordActivity("u1", "g1", t1); err != nil {
		t.Fatalf("replay old: %v", err)
	}
	if err := m.RecordActivity("u1", "g1", t2); err != nil {
		t.Fatalf("replay same: %v", err)
	}
	row, err := m.GetPlayerActivity("u1", "g1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.LastActiveAt != t2 {
		t.Fatalf("last = %q want %q（重放不回退）", row.LastActiveAt, t2)
	}
	if row.PreviousActiveAt != t1 {
		t.Fatalf("previous = %q want %q（重放不覆盖）", row.PreviousActiveAt, t1)
	}
}

func TestPlayerActivity_NotFound(t *testing.T) {
	db := paDB(t)
	m := NewPlayerActivityModel(db)
	if _, err := m.GetPlayerActivity("nope", "g1"); err != ErrPlayerActivityNotFound {
		t.Fatalf("err = %v want ErrPlayerActivityNotFound", err)
	}
}
