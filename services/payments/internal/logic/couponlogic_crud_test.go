package logic

// couponlogic.go 管理面状态机分支：Create/Update/Publish/Disable/Revise/List/Get
// 的 成功 / 角色 403 / 404 不存在 / 409 状态冲突 / 500 落库失败 各臂。

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/internal/types"
	"github.com/tappi/tappi/services/payments/model"

	_ "github.com/mattn/go-sqlite3"
)

func closedCouponSvc(t *testing.T) *svc.ServiceContext {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return &svc.ServiceContext{CouponDB: model.NewCouponModel(db)}
}

func TestCouponLogic_CreateArms(t *testing.T) {
	admin := couponTestSvc(t, "platform_admin")
	player := couponTestSvc(t, "player")

	// 角色守卫：player → 403
	if _, err := NewCouponLogic(wbCtx(), player).Create(&types.CreateCouponRequest{CouponConfig: validCouponCfg()}, "Bearer t"); err == nil {
		t.Fatal("player create = nil error, want 403")
	}
	// 校验失败 → 400
	if _, err := NewCouponLogic(wbCtx(), admin).Create(&types.CreateCouponRequest{CouponConfig: validCouponCfg()}, "Bearer t"); err != nil {
		t.Fatalf("valid create = %v", err)
	}
	bad := validCouponCfg()
	bad.Code = ""
	if _, err := NewCouponLogic(wbCtx(), admin).Create(&types.CreateCouponRequest{CouponConfig: bad}, "Bearer t"); err == nil {
		t.Fatal("invalid create = nil error, want 400")
	}
	// 落库失败 → 500
	if _, err := NewCouponLogic(wbCtx(), closedCouponSvc(t)).Create(&types.CreateCouponRequest{CouponConfig: validCouponCfg()}, "Bearer t"); err == nil {
		t.Fatal("create closed db = nil error, want 500")
	}
}

func TestCouponLogic_UpdateArms(t *testing.T) {
	sc := couponTestSvc(t, "platform_admin")
	l := NewCouponLogic(wbCtx(), sc)
	cfg := validCouponCfg()
	cfg.Code = "UP"

	// 不存在 → 404
	if _, err := l.Update(&types.UpdateCouponRequest{Id: 99999, CouponConfig: cfg}, "Bearer t"); err == nil {
		t.Fatal("update not found = nil error, want 404")
	}
	// 校验失败 → 400
	draft := mustCreateCoupon(t, sc.CouponDB, "UP", model.CouponStatusDraft, nil)
	bad := cfg
	bad.GameId = ""
	if _, err := l.Update(&types.UpdateCouponRequest{Id: draft.Id, CouponConfig: bad}, "Bearer t"); err == nil {
		t.Fatal("update bad cfg = nil error, want 400")
	}
	// 草稿成功
	if _, err := l.Update(&types.UpdateCouponRequest{Id: draft.Id, CouponConfig: cfg}, "Bearer t"); err != nil {
		t.Fatalf("update draft = %v", err)
	}
	// 非草稿 → 409
	sc.CouponDB.SetCouponStatus(draft.Id, model.CouponStatusPublished)
	if _, err := l.Update(&types.UpdateCouponRequest{Id: draft.Id, CouponConfig: cfg}, "Bearer t"); err == nil {
		t.Fatal("update published = nil error, want 409")
	}
	// 查询失败 → 500
	if _, err := NewCouponLogic(wbCtx(), closedCouponSvc(t)).Update(&types.UpdateCouponRequest{Id: 1, CouponConfig: cfg}, "Bearer t"); err == nil {
		t.Fatal("update closed db = nil error, want 500")
	}
}

func TestCouponLogic_PublishDisableReviseArms(t *testing.T) {
	sc := couponTestSvc(t, "platform_admin")
	l := NewCouponLogic(wbCtx(), sc)

	// 不存在 → 404
	if _, err := l.Publish(&types.CouponPathRequest{Id: 99999}, "Bearer t"); err == nil {
		t.Fatal("publish not found = nil error, want 404")
	}
	if _, err := l.Disable(&types.CouponPathRequest{Id: 99999}, "Bearer t"); err == nil {
		t.Fatal("disable not found = nil error, want 404")
	}
	if _, err := l.Revise(&types.CouponPathRequest{Id: 99999}, "Bearer t"); err == nil {
		t.Fatal("revise not found = nil error, want 404")
	}

	// 发布成功 + 状态冲突
	draft := mustCreateCoupon(t, sc.CouponDB, "PS", model.CouponStatusDraft, nil)
	if _, err := l.Publish(&types.CouponPathRequest{Id: draft.Id}, "Bearer t"); err != nil {
		t.Fatalf("publish draft = %v", err)
	}
	if _, err := l.Publish(&types.CouponPathRequest{Id: draft.Id}, "Bearer t"); err == nil {
		t.Fatal("republish = nil error, want 409")
	}
	// 停用成功（published→disabled）+ 状态冲突
	if _, err := l.Disable(&types.CouponPathRequest{Id: draft.Id}, "Bearer t"); err != nil {
		t.Fatalf("disable published = %v", err)
	}
	if _, err := l.Disable(&types.CouponPathRequest{Id: draft.Id}, "Bearer t"); err == nil {
		t.Fatal("redisable = nil error, want 409")
	}
	// 回草稿成功（disabled→draft）+ 已是草稿冲突
	if _, err := l.Revise(&types.CouponPathRequest{Id: draft.Id}, "Bearer t"); err != nil {
		t.Fatalf("revise disabled = %v", err)
	}
	if _, err := l.Revise(&types.CouponPathRequest{Id: draft.Id}, "Bearer t"); err == nil {
		t.Fatal("revise draft = nil error, want 409")
	}
	// 落库失败 → 500
	closed := NewCouponLogic(wbCtx(), closedCouponSvc(t))
	if _, err := closed.Publish(&types.CouponPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("publish closed = nil error, want 500")
	}
	if _, err := closed.Disable(&types.CouponPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("disable closed = nil error, want 500")
	}
	if _, err := closed.Revise(&types.CouponPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("revise closed = nil error, want 500")
	}
}

func TestCouponLogic_ListGetArms(t *testing.T) {
	sc := couponTestSvc(t, "platform_admin")
	l := NewCouponLogic(wbCtx(), sc)

	// 成功（含 CountCouponUsed）
	mustCreateCoupon(t, sc.CouponDB, "LG", model.CouponStatusPublished, nil)
	if _, err := l.List(&types.CouponListRequest{GameId: "game-a"}, "Bearer t"); err != nil {
		t.Fatalf("list = %v", err)
	}
	// 查询失败 → 500
	if _, err := NewCouponLogic(wbCtx(), closedCouponSvc(t)).List(&types.CouponListRequest{}, "Bearer t"); err == nil {
		t.Fatal("list closed = nil error, want 500")
	}
	// Get 成功
	cp := mustCreateCoupon(t, sc.CouponDB, "G1", model.CouponStatusDraft, nil)
	if _, err := l.Get(&types.CouponPathRequest{Id: cp.Id}, "Bearer t"); err != nil {
		t.Fatalf("get = %v", err)
	}
	// Get 不存在 → 404
	if _, err := l.Get(&types.CouponPathRequest{Id: 99999}, "Bearer t"); err == nil {
		t.Fatal("get not found = nil error, want 404")
	}
	// Get 查询失败 → 500
	if _, err := NewCouponLogic(wbCtx(), closedCouponSvc(t)).Get(&types.CouponPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("get closed = nil error, want 500")
	}
}
