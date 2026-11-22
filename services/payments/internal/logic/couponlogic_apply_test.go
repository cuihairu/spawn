package logic

// couponlogic.go ApplyCoupons 分支：空/无效 ids、券不存在、查询失败、未上架、
// 不在有效期、跨游戏、不适用商品、未达门槛、未领取、已占用/已用、不可叠加、
// 折让封顶、发完（used>=total_count）与成功锁券+核销关联。

import (
	"testing"

	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/model"
)

func applyLogic(t *testing.T) (*CouponLogic, *svc.ServiceContext) {
	t.Helper()
	sc := couponTestSvc(t, "platform_admin")
	return NewCouponLogic(wbCtx(), sc), sc
}

func TestApplyCoupons_EmptyAndInvalid(t *testing.T) {
	l, _ := applyLogic(t)
	if total, applied, err := l.ApplyCoupons("o", "game-a", "sku-a", 7, 100, nil); err != nil || total != 0 || applied != nil {
		t.Fatalf("empty = (%d,%v,%v)", total, applied, err)
	}
	if total, _, err := l.ApplyCoupons("o", "game-a", "sku-a", 7, 100, []int64{0, -1}); err != nil || total != 0 {
		t.Fatalf("invalid ids = (%d,%v)", total, err)
	}
}

func TestApplyCoupons_PrecheckArms(t *testing.T) {
	l, sc := applyLogic(t)
	uid := int64(7)

	// 不存在 → 400
	if _, _, err := l.ApplyCoupons("o", "game-a", "sku-a", uid, 100, []int64{99999}); err == nil {
		t.Fatal("not found = nil error, want 400")
	}
	// 查询失败（闭库）→ 500
	if _, _, err := NewCouponLogic(wbCtx(), &svc.ServiceContext{CouponDB: model.NewCouponModel(mustClosedDB(t))}).
		ApplyCoupons("o", "game-a", "sku-a", uid, 100, []int64{1}); err == nil {
		t.Fatal("closed db = nil error, want 500")
	}
	// 未上架（草稿）→ 409
	draft := mustCreateCoupon(t, sc.CouponDB, "AP-DRAFT", model.CouponStatusDraft, nil)
	if _, _, err := l.ApplyCoupons("o", "game-a", "sku-a", uid, 100, []int64{draft.Id}); err == nil {
		t.Fatal("draft = nil error, want 409")
	}
	// 不在有效期 → 409
	future := mustCreateCoupon(t, sc.CouponDB, "AP-FUT", model.CouponStatusPublished, func(c *model.Coupon) {
		c.StartAt, c.EndAt = "2031-01-01T00:00:00Z", "2032-01-01T00:00:00Z"
	})
	if _, _, err := l.ApplyCoupons("o", "game-a", "sku-a", uid, 100, []int64{future.Id}); err == nil {
		t.Fatal("future = nil error, want 409")
	}
	// 跨游戏 → 400
	otherGame := mustCreateCoupon(t, sc.CouponDB, "AP-GAME", model.CouponStatusPublished, func(c *model.Coupon) { c.GameId = "game-b" })
	if _, _, err := l.ApplyCoupons("o", "game-a", "sku-a", uid, 100, []int64{otherGame.Id}); err == nil {
		t.Fatal("game mismatch = nil error, want 400")
	}
	// 不适用商品 → 400
	otherSku := mustCreateCoupon(t, sc.CouponDB, "AP-SKU", model.CouponStatusPublished, func(c *model.Coupon) { c.SkuId = "sku-b" })
	if _, _, err := l.ApplyCoupons("o", "game-a", "sku-a", uid, 100, []int64{otherSku.Id}); err == nil {
		t.Fatal("sku mismatch = nil error, want 400")
	}
	// 未达门槛 → 400
	threshold := mustCreateCoupon(t, sc.CouponDB, "AP-THR", model.CouponStatusPublished, func(c *model.Coupon) { c.ThresholdCents = 200 })
	if _, _, err := l.ApplyCoupons("o", "game-a", "sku-a", uid, 100, []int64{threshold.Id}); err == nil {
		t.Fatal("threshold = nil error, want 400")
	}
	// 未领取 → 400
	unclaimed := mustCreateCoupon(t, sc.CouponDB, "AP-NOCLAIM", model.CouponStatusPublished, nil)
	if _, _, err := l.ApplyCoupons("o", "game-a", "sku-a", uid, 100, []int64{unclaimed.Id}); err == nil {
		t.Fatal("unclaimed = nil error, want 400")
	}
	// 已占用（locked）→ 409
	held := mustCreateCoupon(t, sc.CouponDB, "AP-HELD", model.CouponStatusPublished, nil)
	uc, _, _ := sc.CouponDB.ClaimCoupon(&model.UserCoupon{CouponId: held.Id, UserId: uid, RequestId: "held"})
	sc.CouponDB.LockUserCoupon(uc.Id, "other-order")
	if _, _, err := l.ApplyCoupons("o", "game-a", "sku-a", uid, 100, []int64{held.Id}); err == nil {
		t.Fatal("held = nil error, want 409")
	}
}

func TestApplyCoupons_StackAndClampAndSoldOut(t *testing.T) {
	l, sc := applyLogic(t)
	uid := int64(7)

	// 不可叠加 + 另一张 → 400
	nonStack := mustCreateCoupon(t, sc.CouponDB, "AP-NS", model.CouponStatusPublished, func(c *model.Coupon) {
		c.Stackable = false
		c.DiscountValue = 10
	})
	stack := mustCreateCoupon(t, sc.CouponDB, "AP-ST", model.CouponStatusPublished, func(c *model.Coupon) { c.DiscountValue = 10 })
	sc.CouponDB.ClaimCoupon(&model.UserCoupon{CouponId: nonStack.Id, UserId: uid, RequestId: "ns"})
	sc.CouponDB.ClaimCoupon(&model.UserCoupon{CouponId: stack.Id, UserId: uid, RequestId: "st"})
	if _, _, err := l.ApplyCoupons("o", "game-a", "sku-a", uid, 100, []int64{nonStack.Id, stack.Id}); err == nil {
		t.Fatal("non-stackable = nil error, want 400")
	}

	// 折让封顶：两张各 80，listPrice 100 → total=100
	bigA := mustCreateCoupon(t, sc.CouponDB, "AP-BA", model.CouponStatusPublished, func(c *model.Coupon) {
		c.Stackable = true
		c.DiscountValue = 80
	})
	bigB := mustCreateCoupon(t, sc.CouponDB, "AP-BB", model.CouponStatusPublished, func(c *model.Coupon) {
		c.Stackable = true
		c.DiscountValue = 80
	})
	sc.CouponDB.ClaimCoupon(&model.UserCoupon{CouponId: bigA.Id, UserId: uid, RequestId: "ba"})
	sc.CouponDB.ClaimCoupon(&model.UserCoupon{CouponId: bigB.Id, UserId: uid, RequestId: "bb"})
	total, applied, err := l.ApplyCoupons("o", "game-a", "sku-a", uid, 100, []int64{bigA.Id, bigB.Id})
	if err != nil || total != 100 || len(applied) != 2 {
		t.Fatalf("clamp = (%d,%v,%v), want (100,2,nil)", total, applied, err)
	}

	// 发完：total_count=1，一次使用后再领，Apply → 409
	sold := mustCreateCoupon(t, sc.CouponDB, "AP-SOLD", model.CouponStatusPublished, func(c *model.Coupon) { c.TotalCount = 1 })
	uc1, _, _ := sc.CouponDB.ClaimCoupon(&model.UserCoupon{CouponId: sold.Id, UserId: uid, RequestId: "s1"})
	sc.CouponDB.LockUserCoupon(uc1.Id, "order-sold")
	sc.CouponDB.MarkCouponUsedByOrder("order-sold") // used=1
	sc.CouponDB.ClaimCoupon(&model.UserCoupon{CouponId: sold.Id, UserId: uid, RequestId: "s2"})
	if _, _, err := l.ApplyCoupons("o", "game-a", "sku-a", uid, 100, []int64{sold.Id}); err == nil {
		t.Fatal("sold out = nil error, want 409")
	}
}

func TestApplyCoupons_Success(t *testing.T) {
	l, sc := applyLogic(t)
	uid := int64(7)
	cp := mustCreateCoupon(t, sc.CouponDB, "AP-OK", model.CouponStatusPublished, func(c *model.Coupon) { c.DiscountValue = 25 })
	sc.CouponDB.ClaimCoupon(&model.UserCoupon{CouponId: cp.Id, UserId: uid, RequestId: "ok"})

	total, applied, err := l.ApplyCoupons("order-ok", "game-a", "sku-a", uid, 100, []int64{cp.Id})
	if err != nil || total != 25 || len(applied) != 1 || applied[0].DiscountCents != 25 {
		t.Fatalf("success = (%d,%v,%v), want (25,1,nil)", total, applied, err)
	}
}
