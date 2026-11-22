package logic

// couponlogic.go 剩余补漏分支：Create/Update/Publish/Disable/Revise/Claim 的
// 落库失败 500（闭库=首个查询即败；?mode=ro=读成功写失败），MyCoupons 的
// 券已删（GetCoupon 失败→continue）臂，以及 ApplyCoupons 的 GetClaimedUserCoupon
// 失败 500（删 user_coupons 表）与 LinkOrderCoupon 失败 best-effort（仅日志）
// 臂。

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/internal/types"
	"github.com/tappi/tappi/services/payments/model"
)

// couponSvcAt 暴露 dbPath（可开第二连接删表）的优惠券上下文。
func couponSvcAt(t *testing.T, role, dbPath string) (*svc.ServiceContext, *model.CouponModel) {
	t.Helper()
	st := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"role":"` + role + `","partner_id":1}`))
	}))
	t.Cleanup(st.Close)
	sc := testSvc(st.URL)
	db := openDB(t, "file:"+dbPath)
	m := model.NewCouponModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("coupon tables: %v", err)
	}
	sc.CouponDB = m
	return sc, m
}

func TestCouponLogic_WriteErrorArms(t *testing.T) {
	// Create：首个查询即败 → 500
	closed := couponTestSvc(t, "platform_admin")
	closed.CouponDB = model.NewCouponModel(mustClosedDB(t))
	if _, err := NewCouponLogic(wbCtx(), closed).Create(&types.CreateCouponRequest{CouponConfig: validCouponCfg()}, "Bearer t"); err == nil {
		t.Fatal("create closed = nil error, want 500")
	}
	// Claim：GetCoupon 首个查询即败 → 500
	if _, err := NewCouponLogic(wbCtx(), closed).Claim(&types.ClaimCouponRequest{Id: 1, RequestId: "r"}); err == nil {
		t.Fatal("claim closed = nil error, want 500")
	}
}

func TestCouponLogic_UpdatePublishDisableReviseWriteErrors(t *testing.T) {
	// Update：草稿在，只读连接读成功写失败 → 500
	dbPath := filepath.Join(t.TempDir(), "cp-wr.db")
	sc, m := couponSvcAt(t, "platform_admin", dbPath)
	draft := mustCreateCoupon(t, m, "WR-D", model.CouponStatusDraft, nil)
	ro := openDB(t, "file:"+dbPath+"?mode=ro")
	sc.CouponDB = model.NewCouponModel(ro)
	if _, err := NewCouponLogic(wbCtx(), sc).Update(&types.UpdateCouponRequest{Id: draft.Id, CouponConfig: validCouponCfg()}, "Bearer t"); err == nil {
		t.Fatal("update ro = nil error, want 500")
	}
	if _, err := NewCouponLogic(wbCtx(), sc).Publish(&types.CouponPathRequest{Id: draft.Id}, "Bearer t"); err == nil {
		t.Fatal("publish ro = nil error, want 500")
	}
	// Disable/Revise：published 在，只读连接读成功写失败 → 500
	dbPath2 := filepath.Join(t.TempDir(), "cp-wr2.db")
	sc2, m2 := couponSvcAt(t, "platform_admin", dbPath2)
	pub := mustCreateCoupon(t, m2, "WR-P", model.CouponStatusPublished, nil)
	ro2 := openDB(t, "file:"+dbPath2+"?mode=ro")
	sc2.CouponDB = model.NewCouponModel(ro2)
	if _, err := NewCouponLogic(wbCtx(), sc2).Disable(&types.CouponPathRequest{Id: pub.Id}, "Bearer t"); err == nil {
		t.Fatal("disable ro = nil error, want 500")
	}
	if _, err := NewCouponLogic(wbCtx(), sc2).Revise(&types.CouponPathRequest{Id: pub.Id}, "Bearer t"); err == nil {
		t.Fatal("revise ro = nil error, want 500")
	}
	// Claim：published 在，只读连接 ClaimCoupon 写入败 → 500
	dbPath3 := filepath.Join(t.TempDir(), "cp-wr3.db")
	sc3, m3 := couponSvcAt(t, "platform_admin", dbPath3)
	pub3 := mustCreateCoupon(t, m3, "WR-C", model.CouponStatusPublished, nil)
	ro3 := openDB(t, "file:"+dbPath3+"?mode=ro")
	sc3.CouponDB = model.NewCouponModel(ro3)
	if _, err := NewCouponLogic(wbCtx(), sc3).Claim(&types.ClaimCouponRequest{Id: pub3.Id, RequestId: "r"}); err == nil {
		t.Fatal("claim ro = nil error, want 500")
	}
}

func TestCouponLogic_MyCouponsMissingCouponArm(t *testing.T) {
	// 已领券的券定义被删：GetCoupon 失败 → continue（不中断列表）
	dbPath := filepath.Join(t.TempDir(), "cp-mc.db")
	sc, m := couponSvcAt(t, "platform_admin", dbPath)
	c := mustCreateCoupon(t, m, "MC", model.CouponStatusPublished, nil)
	if _, _, err := m.ClaimCoupon(&model.UserCoupon{CouponId: c.Id, UserId: 7, RequestId: "mc1"}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	db := openDB(t, "file:"+dbPath)
	if _, err := db.Exec(`DROP TABLE coupons`); err != nil {
		t.Fatalf("drop coupons: %v", err)
	}
	resp, err := NewCouponLogic(wbCtx(), sc).MyCoupons("")
	if err != nil {
		t.Fatalf("my coupons = %v, want nil", err)
	}
	if len(resp.Items) != 0 {
		t.Fatalf("items = %d, want 0 (missing coupon skipped)", len(resp.Items))
	}
}

func TestApplyCoupons_GetClaimedError(t *testing.T) {
	// GetCoupon ok（published 在），user_coupons 被删 → GetClaimedUserCoupon 败 → 500
	dbPath := filepath.Join(t.TempDir(), "cp-gc.db")
	sc, m := couponSvcAt(t, "platform_admin", dbPath)
	c := mustCreateCoupon(t, m, "GC", model.CouponStatusPublished, nil)
	db := openDB(t, "file:"+dbPath)
	if _, err := db.Exec(`DROP TABLE user_coupons`); err != nil {
		t.Fatalf("drop user_coupons: %v", err)
	}
	if _, _, err := NewCouponLogic(wbCtx(), sc).ApplyCoupons("o", "game-a", "sku-a", 7, 1000, []int64{c.Id}); err == nil {
		t.Fatal("get claimed closed = nil error, want 500")
	}
}

func TestApplyCoupons_LinkErrorBestEffort(t *testing.T) {
	// coupon_order_links 被删：锁券 ok（user_coupons 在），LinkOrderCoupon 败 →
	// 仅记日志不阻断（best-effort），仍返回已应用券与折让
	dbPath := filepath.Join(t.TempDir(), "cp-lk.db")
	sc, m := couponSvcAt(t, "platform_admin", dbPath)
	c := mustCreateCoupon(t, m, "LK", model.CouponStatusPublished, nil)
	if _, _, err := m.ClaimCoupon(&model.UserCoupon{CouponId: c.Id, UserId: 7, RequestId: "lk1"}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	db := openDB(t, "file:"+dbPath)
	if _, err := db.Exec(`DROP TABLE coupon_order_links`); err != nil {
		t.Fatalf("drop links: %v", err)
	}
	total, applied, err := NewCouponLogic(wbCtx(), sc).ApplyCoupons("o", "game-a", "sku-a", 7, 1000, []int64{c.Id})
	if err != nil {
		t.Fatalf("apply = %v, want best-effort nil", err)
	}
	if len(applied) != 1 || total != 5 {
		t.Fatalf("applied=%d total=%d, want 1/5", len(applied), total)
	}
}
