package logic

// couponlogic.go 白盒分支：validateCouponConfig 全校验臂、Claim 各臂
// （未登录/空幂等键/不存在/未上架/不在有效期/限领）、MyCoupons 各臂、
// couponDiscount 三种形态与封顶，以及 Redeem/Return 直通。

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/internal/types"
	"github.com/tappi/tappi/services/payments/model"

	_ "github.com/mattn/go-sqlite3"
)

func couponTestSvc(t *testing.T, role string) *svc.ServiceContext {
	t.Helper()
	st := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"role":"` + role + `","partner_id":1}`))
	}))
	t.Cleanup(st.Close)
	sc := testSvc(st.URL)
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "coupon.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := model.NewCouponModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("tables: %v", err)
	}
	sc.CouponDB = m
	return sc
}

func validCouponCfg() types.CouponConfig {
	return types.CouponConfig{
		Code: "C1", Name: "新手券", GameId: "game-a",
		DiscountType: model.CouponDiscountFixed, DiscountValue: 10,
		PerUserLimit: 5, TotalCount: 100,
		StartAt: "2020-01-01T00:00:00Z", EndAt: "2030-01-01T00:00:00Z",
	}
}

func mustCreateCoupon(t *testing.T, m *model.CouponModel, code, status string, tweak func(*model.Coupon)) *model.Coupon {
	t.Helper()
	c := &model.Coupon{
		Code: code, Name: "n", GameId: "game-a",
		DiscountType: model.CouponDiscountFixed, DiscountValue: 5,
		PerUserLimit: 2, TotalCount: 10,
		StartAt: "2020-01-01T00:00:00Z", EndAt: "2030-01-01T00:00:00Z",
		Status: status,
	}
	if tweak != nil {
		tweak(c)
	}
	if err := m.CreateCoupon(c); err != nil {
		t.Fatalf("create coupon: %v", err)
	}
	return c
}

func TestValidateCouponConfig_Arms(t *testing.T) {
	bad := []func(c *types.CouponConfig){
		func(c *types.CouponConfig) { c.Code = "" },
		func(c *types.CouponConfig) { c.Name = "" },
		func(c *types.CouponConfig) { c.GameId = "" },
		func(c *types.CouponConfig) { c.DiscountType = "bogus" },
		func(c *types.CouponConfig) { c.DiscountValue = 0 },
		func(c *types.CouponConfig) { c.DiscountType = model.CouponDiscountPercentBp; c.DiscountValue = 20000 },
		func(c *types.CouponConfig) { c.ThresholdCents = -1 },
		func(c *types.CouponConfig) { c.PerUserLimit = 0 },
		func(c *types.CouponConfig) { c.StartAt = "" },
		func(c *types.CouponConfig) { c.EndAt = "" },
		func(c *types.CouponConfig) { c.StartAt = "2031-01-01T00:00:00Z" },
	}
	for i, f := range bad {
		c := validCouponCfg()
		f(&c)
		if err := validateCouponConfig(&c); err == nil {
			t.Fatalf("case %d: invalid config accepted", i)
		}
	}
	valid := validCouponCfg()
	if err := validateCouponConfig(&valid); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestCouponLogic_Claim_Arms(t *testing.T) {
	sc := couponTestSvc(t, "platform_admin")
	if _, err := NewCouponLogic(context.Background(), sc).Claim(&types.ClaimCouponRequest{Id: 1, RequestId: "r"}); err == nil {
		t.Fatal("anon claim = nil error, want 401")
	}
	l := NewCouponLogic(wbCtx(), sc)

	coupon := mustCreateCoupon(t, sc.CouponDB, "CL", model.CouponStatusDraft, nil)

	// 空 request_id → 400
	if _, err := l.Claim(&types.ClaimCouponRequest{Id: coupon.Id, RequestId: " "}); err == nil {
		t.Fatal("empty request_id = nil error, want 400")
	}
	// 不存在 → 404
	if _, err := l.Claim(&types.ClaimCouponRequest{Id: 99999, RequestId: "r"}); err == nil {
		t.Fatal("not found = nil error, want 404")
	}
	// 草稿未上架 → 409
	if _, err := l.Claim(&types.ClaimCouponRequest{Id: coupon.Id, RequestId: "r"}); err == nil {
		t.Fatal("draft claim = nil error, want 409")
	}
	// 不在有效期 → 409
	if err := sc.CouponDB.SetCouponStatus(coupon.Id, model.CouponStatusPublished); err != nil {
		t.Fatalf("publish: %v", err)
	}
	future := mustCreateCoupon(t, sc.CouponDB, "FUT", model.CouponStatusPublished, func(c *model.Coupon) {
		c.StartAt, c.EndAt = "2031-01-01T00:00:00Z", "2032-01-01T00:00:00Z"
	})
	if _, err := l.Claim(&types.ClaimCouponRequest{Id: future.Id, RequestId: "r"}); err == nil {
		t.Fatal("future claim = nil error, want 409")
	}
	// 成功领取 + 幂等重放
	if _, err := l.Claim(&types.ClaimCouponRequest{Id: coupon.Id, RequestId: "r1"}); err != nil {
		t.Fatalf("claim = %v", err)
	}
	if _, err := l.Claim(&types.ClaimCouponRequest{Id: coupon.Id, RequestId: "r1"}); err != nil {
		t.Fatalf("idempotent replay = %v", err)
	}
	// 限领：per_user_limit=2，第 3 次新领 → 409
	limited := mustCreateCoupon(t, sc.CouponDB, "LIM", model.CouponStatusPublished, nil)
	l.Claim(&types.ClaimCouponRequest{Id: limited.Id, RequestId: "a"})
	l.Claim(&types.ClaimCouponRequest{Id: limited.Id, RequestId: "b"})
	if _, err := l.Claim(&types.ClaimCouponRequest{Id: limited.Id, RequestId: "c"}); err == nil {
		t.Fatal("over limit = nil error, want 409")
	}
}

func TestCouponLogic_MyCoupons_Arms(t *testing.T) {
	sc := couponTestSvc(t, "platform_admin")
	// 未登录 → 401
	if _, err := NewCouponLogic(context.Background(), sc).MyCoupons(""); err == nil {
		t.Fatal("anon my coupons = nil error, want 401")
	}
	// 查询失败 → 500
	closed := &svc.ServiceContext{CouponDB: model.NewCouponModel(mustClosedDB(t))}
	if _, err := NewCouponLogic(wbCtx(), closed).MyCoupons(""); err == nil {
		t.Fatal("my coupons db err = nil error, want 500")
	}
	// 成功：有一张已领（GetCoupon 成功路径）
	coupon := mustCreateCoupon(t, sc.CouponDB, "MY", model.CouponStatusPublished, nil)
	sc.CouponDB.ClaimCoupon(&model.UserCoupon{CouponId: coupon.Id, UserId: 7, RequestId: "m1"})
	resp, err := NewCouponLogic(wbCtx(), sc).MyCoupons("")
	if err != nil || len(resp.Items) != 1 {
		t.Fatalf("my coupons = %+v err=%v, want 1 item", resp, err)
	}
}

func TestCouponDiscount_Arms(t *testing.T) {
	fixed := &model.Coupon{DiscountType: model.CouponDiscountFixed, DiscountValue: 30}
	if d := couponDiscount(fixed, 100); d != 30 {
		t.Fatalf("fixed = %d, want 30", d)
	}
	pct := &model.Coupon{DiscountType: model.CouponDiscountPercentBp, DiscountValue: 2500}
	if d := couponDiscount(pct, 10000); d != 2500 {
		t.Fatalf("percent = %d, want 2500", d)
	}
	unknown := &model.Coupon{DiscountType: "weird", DiscountValue: 50}
	if d := couponDiscount(unknown, 100); d != 0 {
		t.Fatalf("unknown = %d, want 0", d)
	}
	over := &model.Coupon{DiscountType: model.CouponDiscountFixed, DiscountValue: 500}
	if d := couponDiscount(over, 100); d != 100 {
		t.Fatalf("clamp = %d, want 100", d)
	}
}

func TestCouponLogic_RedeemReturn_Arms(t *testing.T) {
	sc := couponTestSvc(t, "platform_admin")
	l := NewCouponLogic(wbCtx(), sc)
	if err := l.RedeemOrderCoupons("no-such-order"); err != nil {
		t.Fatalf("redeem no-op = %v", err)
	}
	if err := l.ReturnOrderCoupons("no-such-order"); err != nil {
		t.Fatalf("return no-op = %v", err)
	}
	// 闭库 → 返回错误
	closed := NewCouponLogic(wbCtx(), &svc.ServiceContext{CouponDB: model.NewCouponModel(mustClosedDB(t))})
	if err := closed.RedeemOrderCoupons("x"); err == nil {
		t.Fatal("redeem closed = nil error, want error")
	}
	if err := closed.ReturnOrderCoupons("x"); err == nil {
		t.Fatal("return closed = nil error, want error")
	}
}
