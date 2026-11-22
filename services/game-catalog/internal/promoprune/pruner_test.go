package promoprune

import (
	"context"
	"database/sql"
	"errors"
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

// failPruneStore 注入 PrunePromoClicks 失败，其余委托真实实现。
type failPruneStore struct {
	model.AdminStore
	pruneErr error
}

func (s *failPruneStore) PrunePromoClicks(cutoff string, maxRows int64) (int64, error) {
	if s.pruneErr != nil {
		return 0, s.pruneErr
	}
	return s.AdminStore.PrunePromoClicks(cutoff, maxRows)
}

// NewPruner 非法周期回退 6 小时；retention/maxRows 参数原样保留。
func TestPruner_NewPrunerDefaults(t *testing.T) {
	ctx := newPrunerTestCtx(t)
	p := NewPruner(ctx, 0, 0, 0)
	if p.interval != 6*time.Hour {
		t.Fatalf("interval = %v, want 6h", p.interval)
	}
	p = NewPruner(ctx, time.Hour, 5, time.Minute)
	if p.retention != time.Hour || p.maxRows != 5 || p.interval != time.Minute {
		t.Fatalf("params = retention=%v maxRows=%d interval=%v", p.retention, p.maxRows, p.interval)
	}
}

// PruneOnce 关闭保留窗口（retention<=0）→ cutoff 空、仅按容量上限裁剪。
func TestPruner_PruneOnceRetentionDisabled(t *testing.T) {
	ctx := newPrunerTestCtx(t)
	ch, err := ctx.AdminRepository.CreatePromoChannel("k1", "n1", 72)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := ctx.AdminRepository.RecordPromoClick("c1", ch, model.PromoClickClick, "g-1", "2000-01-01T00:00:00Z"); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := ctx.AdminRepository.RecordPromoClick("c2", ch, model.PromoClickClick, "g-1", "2000-01-02T00:00:00Z"); err != nil {
		t.Fatalf("record: %v", err)
	}
	p := NewPruner(ctx, 0, 1, time.Hour) // 关时间清理，容量上限 1 → 裁 1 行
	if n := p.PruneOnce(context.Background()); n != 1 {
		t.Fatalf("pruned = %d, want 1", n)
	}
	if _, err := ctx.AdminRepository.GetPromoClick("c2"); err != nil {
		t.Fatalf("newest click gone: %v", err)
	}
}

// PruneOnce PrunePromoClicks 失败 → 记日志返回 0。
func TestPruner_PruneOnceError(t *testing.T) {
	ctx := newPrunerTestCtx(t)
	ctx.AdminRepository = &failPruneStore{AdminStore: ctx.AdminRepository, pruneErr: errors.New("prune down")}
	if n := NewPruner(ctx, time.Hour, 0, time.Hour).PruneOnce(context.Background()); n != 0 {
		t.Fatalf("pruned = %d, want 0", n)
	}
}

// PruneOnce 无过期行 → 返回 0（不删行）。
func TestPruner_PruneOnceZeroDeleted(t *testing.T) {
	ctx := newPrunerTestCtx(t)
	if n := NewPruner(ctx, time.Hour, 0, time.Hour).PruneOnce(context.Background()); n != 0 {
		t.Fatalf("pruned = %d, want 0", n)
	}
}

// Run ticker 分支：真实清理到点行后 ctx 取消即退出。
func TestPruner_RunTicksAndStops(t *testing.T) {
	ctx := newPrunerTestCtx(t)
	ch, err := ctx.AdminRepository.CreatePromoChannel("run1", "rn", 72)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := ctx.AdminRepository.RecordPromoClick("r1", ch, model.PromoClickClick, "g-1", "2000-01-01T00:00:00Z"); err != nil {
		t.Fatalf("record: %v", err)
	}
	p := NewPruner(ctx, 30*24*time.Hour, 0, time.Millisecond)
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		p.Run(runCtx)
		close(done)
	}()
	time.Sleep(40 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
	if _, err := ctx.AdminRepository.GetPromoClick("r1"); err != model.ErrPromoClickNotFound {
		t.Fatalf("old click should be pruned by Run: %v", err)
	}
}

// Run 已取消的 ctx 立即退出。
func TestPruner_RunStopsOnCanceledCtx(t *testing.T) {
	ctx := newPrunerTestCtx(t)
	p := NewPruner(ctx, time.Hour, 0, time.Millisecond)
	runCtx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		p.Run(runCtx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not exit on canceled ctx")
	}
}
