package model

// promomodel.go 补漏分支：关库下各查询/写入失败臂、列表脏行 Scan 失败、
// RecordPromoClick 空 now 兜底、MarkClickAttributed 与 PrunePromoClicks 的
// UPDATE/DELETE 失败臂（RAISE(ABORT) 触发器，覆盖「前置 SELECT 成功但写失败」）。
// LastInsertId/RowsAffected 失败臂、MySQL INSERT IGNORE 回退、ChannelStats
// 聚合 ErrNoRows（聚合恒返一行）属登记不可构造家族，见 coverage 台账。

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// newTestAdminModelDB 同 newTestAdminModel 但返回底层连接（供关库/触发器用例）。
func newTestAdminModelDB(t *testing.T) (*AdminModel, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/admin.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	m := NewAdminModel(db)
	if err := m.CreateAdminTables(); err != nil {
		t.Fatalf("create admin tables: %v", err)
	}
	return m, db
}

// 关库：所有直接 Exec/Query 失败臂（含 RecordPromoClick 的非方言错误出口）。
func TestPromoModel_ClosedDBErrors(t *testing.T) {
	m, db := newTestAdminModelDB(t)
	db.Close()

	if _, err := m.ListPromoChannels(10, 0); err == nil {
		t.Fatal("ListPromoChannels closed db = nil error, want error")
	}
	if _, err := m.UpdatePromoChannel(1, "n", 24); err == nil {
		t.Fatal("UpdatePromoChannel closed db = nil error, want error")
	}
	if _, err := m.SetPromoChannelStatus(1, PromoChannelActive); err == nil {
		t.Fatal("SetPromoChannelStatus closed db = nil error, want error")
	}
	if _, err := m.GetPromoChannel(1); err == nil {
		t.Fatal("GetPromoChannel closed db = nil error, want error")
	}
	if _, err := m.GetPromoChannelByCode("kol-li"); err == nil {
		t.Fatal("GetPromoChannelByCode closed db = nil error, want error")
	}
	if _, err := m.RecordPromoClick("c-x", &PromoChannel{Id: 1, Code: "kol-li"}, PromoClickClick, "g-1", nowUTC()); err == nil {
		t.Fatal("RecordPromoClick closed db = nil error, want error")
	}
	if _, err := m.GetPromoClick("c-x"); err == nil {
		t.Fatal("GetPromoClick closed db = nil error, want error")
	}
	if _, err := m.MarkClickAttributed("c-x", 5); err == nil {
		t.Fatal("MarkClickAttributed closed db = nil error, want error")
	}
	if _, err := m.ChannelStats("kol-li", "", ""); err == nil {
		t.Fatal("ChannelStats closed db = nil error, want error")
	}
	if _, err := m.PrunePromoClicks("2000-01-01T00:00:00Z", 0); err == nil {
		t.Fatal("PrunePromoClicks closed db (cutoff) = nil error, want error")
	}
	if _, err := m.PrunePromoClicks("", 5); err == nil {
		t.Fatal("PrunePromoClicks closed db (capacity) = nil error, want error")
	}
}

// 列表脏行 Scan 失败：破坏 INTEGER 主键列（非数字文本越过 SQLite 亲和）。
func TestPromoModel_ListScanDirty(t *testing.T) {
	m, db := newTestAdminModelDB(t)
	if _, err := m.CreatePromoChannel("kol-li", "李", 72); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.Exec(`UPDATE promo_channels SET attribution_window_hours = 'abc' WHERE id = 1`); err != nil {
		t.Fatalf("corrupt window: %v", err)
	}
	if _, err := m.ListPromoChannels(10, 0); err == nil {
		t.Fatal("ListPromoChannels dirty window = nil error, want scan error")
	}
}

// RecordPromoClick 空 now → 走 nowUTC 兜底（正常落库）。
func TestPromoModel_RecordClickEmptyNow(t *testing.T) {
	m := newTestAdminModel(t)
	ch, err := m.CreatePromoChannel("kol-li", "李", 72)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := m.RecordPromoClick("c-now", ch, PromoClickClick, "g-1", ""); err != nil {
		t.Fatalf("RecordPromoClick empty now: %v", err)
	}
	if _, err := m.GetPromoClick("c-now"); err != nil {
		t.Fatalf("click not found after empty now: %v", err)
	}
}

// MarkClickAttributed：Get 成功但 UPDATE 被触发器中止 → 报错不 panic。
func TestPromoModel_MarkAttributedUpdateFailed(t *testing.T) {
	m, db := newTestAdminModelDB(t)
	ch, err := m.CreatePromoChannel("kol-li", "李", 72)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := m.RecordPromoClick("c-a", ch, PromoClickClick, "g-1", nowUTC()); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_promo_update BEFORE UPDATE ON promo_clicks
		BEGIN SELECT RAISE(ABORT, 'forced promo update failure'); END`); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if _, err := m.MarkClickAttributed("c-a", 5); err == nil {
		t.Fatal("MarkClickAttributed update failed = nil error, want error")
	}
}

// PrunePromoClicks 容量裁剪 DELETE 失败（cutoff 空跳过时间清理，COUNT 成功
// 后 DELETE 被触发器中止）→ 报错不 panic。
func TestPromoModel_PruneCapacityDeleteFailed(t *testing.T) {
	m, db := newTestAdminModelDB(t)
	ch, err := m.CreatePromoChannel("kol-li", "李", 72)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := m.RecordPromoClick("p-1", ch, PromoClickClick, "g-1", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("record1: %v", err)
	}
	if _, err := m.RecordPromoClick("p-2", ch, PromoClickClick, "g-1", "2026-01-02T00:00:00Z"); err != nil {
		t.Fatalf("record2: %v", err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_promo_delete BEFORE DELETE ON promo_clicks
		BEGIN SELECT RAISE(ABORT, 'forced promo delete failure'); END`); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if _, err := m.PrunePromoClicks("", 1); err == nil {
		t.Fatal("PrunePromoClicks capacity delete failed = nil error, want error")
	}
}
