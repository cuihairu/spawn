package promoprune

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/model"

	_ "github.com/mattn/go-sqlite3"
)

// newPrunerTestCtx 构造最小 svc 上下文（仅 AdminRepository），临时文件 SQLite 建表。
func newPrunerTestCtx(t *testing.T) *svc.ServiceContext {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "promo.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	am := model.NewAdminModel(db)
	if err := am.CreateAdminTables(); err != nil {
		t.Fatalf("tables: %v", err)
	}
	return &svc.ServiceContext{AdminRepository: am}
}

// TestPrunerPruneOnce 清理循环一轮：保留窗口前的旧点击删除，窗口内保留。
func TestPrunerPruneOnce(t *testing.T) {
	ctx := newPrunerTestCtx(t)
	ch, err := ctx.AdminRepository.CreatePromoChannel("kol-li", "主播李", 72)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	old := "2026-01-01T00:00:00Z"
	recent := "2026-11-12T10:00:00Z"
	if _, err := ctx.AdminRepository.RecordPromoClick("a", ch, model.PromoClickClick, "g-1", old); err != nil {
		t.Fatalf("record old: %v", err)
	}
	if _, err := ctx.AdminRepository.RecordPromoClick("b", ch, model.PromoClickClick, "g-1", recent); err != nil {
		t.Fatalf("record recent: %v", err)
	}

	p := NewPruner(ctx, 30*24*time.Hour, 0, time.Hour)
	n := p.PruneOnce(context.Background())
	if n != 1 {
		t.Fatalf("pruned = %d, want 1", n)
	}
	if _, err := ctx.AdminRepository.GetPromoClick("a"); err != model.ErrPromoClickNotFound {
		t.Fatalf("old click still present: %v", err)
	}
	if _, err := ctx.AdminRepository.GetPromoClick("b"); err != nil {
		t.Fatalf("recent click gone: %v", err)
	}
}
