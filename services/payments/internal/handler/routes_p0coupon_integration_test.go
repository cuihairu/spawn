package handler

// P0-1 回归：优惠券锁券后 CreateTarget / UpsertFromOrder 失败必须回滚锁券
// （券不永久卡死 locked）；以及超时释放扫描把「locked 且久未 PAID」的券
// 释放回 claimed。制造失败用 DROP TABLE + CREATE VIEW（INSERT 落 view 必
// 败），与仓内既有失败注入范式一致。

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/internal/couponrec"
	"github.com/tappi/tappi/services/payments/utils"
)

// breakTable 把表替换成同名 VIEW（后续 INSERT 落 view 报错），模拟该表不可写。
func breakTable(t *testing.T, env *testDeliveryEnv, table string) {
	t.Helper()
	db, err := sql.Open("sqlite3", env.dsn)
	if err != nil {
		t.Fatalf("open dsn: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`DROP TABLE ` + table); err != nil {
		t.Fatalf("drop %s: %v", table, err)
	}
	if _, err := db.Exec(`CREATE VIEW ` + table + ` AS SELECT 1 AS id`); err != nil {
		t.Fatalf("create view %s: %v", table, err)
	}
}

// ageLockedAt 把券下所有 locked user_coupon 的 locked_at 拨到指定时刻
// （模拟「下单后久未 PAID」的孤儿锁）。
func ageLockedAt(t *testing.T, env *testDeliveryEnv, couponID int64, at string) {
	t.Helper()
	db, err := sql.Open("sqlite3", env.dsn)
	if err != nil {
		t.Fatalf("open dsn: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(
		`UPDATE user_coupons SET locked_at=? WHERE coupon_id=? AND status='locked'`, at, couponID); err != nil {
		t.Fatalf("age locked_at: %v", err)
	}
}

// gpSetupCouponEnv 公共前置：商品 + 游戏服发货端点 + 一张已发布券（阈值 500
// 直减 200 限领 3）。返回 (admin, player, couponID)。
func gpSetupCouponEnv(t *testing.T, env *testDeliveryEnv) (string, string, int64) {
	t.Helper()
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})
	code, body := do(t, http.MethodPost, env.base+"/admin/products/sku-gem-100", admin,
		`{"product_id":"gem-100","form":"DIRECT_PURCHASE","amount_cents":1000,"currency":"CNY","game_id":"game-a","partner_id":2}`)
	if code != http.StatusOK {
		t.Fatalf("create product = %d body=%s", code, body)
	}
	code, _ = do(t, http.MethodPut, env.base+"/admin/games/game-a/delivery", admin,
		fmt.Sprintf(`{"endpoint":%q,"secret":"gs-secret"}`, env.gameServer.srv.URL))
	if code != http.StatusOK {
		t.Fatalf("configure endpoint = %d", code)
	}
	id := gpCreateCoupon(t, env, admin, gpCouponBody("P0ROLLBACK", "P0回滚券", 500, 200, true, 3))
	gpPublishCoupon(t, env, admin, id)
	return admin, player, id
}

// TestRoutes_CouponLockRollbackOnCreateTargetFailure 下单后半程 CreateTarget
// 失败 → 500 且锁券回滚到 claimed（原 P0：只 return 不回滚 → 券卡死）。
func TestRoutes_CouponLockRollbackOnCreateTargetFailure(t *testing.T) {
	env := newDeliveryServer(t)
	_, player, id := gpSetupCouponEnv(t, env)
	claimOK(t, env, player, id, "p0-rt-a1")
	breakTable(t, env, "delivery_orders") // CreateTarget 落 view 必败

	code, body := gpOrderReq(t, env, player, []int64{id})
	if code != http.StatusInternalServerError {
		t.Fatalf("order with broken delivery_orders = %d body=%s, want 500", code, body)
	}
	assertCouponStatus(t, env, player, id, "claimed")
	assertCouponNotStatus(t, env, player, id, "locked")
}

// TestRoutes_CouponLockRollbackOnProjectionFailure UpsertFromOrder 失败 →
// 500 且锁券回滚（CreateTarget 已成功，后半程投影失败同样要回滚）。
func TestRoutes_CouponLockRollbackOnProjectionFailure(t *testing.T) {
	env := newDeliveryServer(t)
	_, player, id := gpSetupCouponEnv(t, env)
	claimOK(t, env, player, id, "p0-rt-b1")
	breakTable(t, env, "order_projection") // UpsertFromOrder 落 view 必败

	code, body := gpOrderReq(t, env, player, []int64{id})
	if code != http.StatusInternalServerError {
		t.Fatalf("order with broken order_projection = %d body=%s, want 500", code, body)
	}
	assertCouponStatus(t, env, player, id, "claimed")
	assertCouponNotStatus(t, env, player, id, "locked")
}

// TestRoutes_CouponReleaseScanner 超时释放：锁券拨旧 locked_at 后运行
// ReleaseOnce（默认 ttl=1h）→ 券回 claimed。
func TestRoutes_CouponReleaseScanner(t *testing.T) {
	env := newDeliveryServer(t)
	_, player, id := gpSetupCouponEnv(t, env)
	claimOK(t, env, player, id, "p0-rt-c1")

	code, body := gpOrderReq(t, env, player, []int64{id})
	if code != http.StatusOK {
		t.Fatalf("order = %d body=%s, want 200", code, body)
	}
	assertCouponStatus(t, env, player, id, "locked")

	ageLockedAt(t, env, id, "2020-01-01T00:00:00Z") // 早于 now-1h

	rel := couponrec.NewRelease(env.svcCtx.CouponDB, time.Hour, 5*time.Minute)
	released, orders := rel.ReleaseOnce(context.Background(), time.Now().UTC())
	if released != 1 || len(orders) != 1 {
		t.Fatalf("release once = %d orders=%v, want 1", released, orders)
	}
	assertCouponStatus(t, env, player, id, "claimed")
	assertCouponNotStatus(t, env, player, id, "locked")
}

// claimOK 领券并断言 200。
func claimOK(t *testing.T, env *testDeliveryEnv, player string, id int64, reqID string) {
	t.Helper()
	code, body := do(t, http.MethodPost, fmt.Sprintf("%s/coupons/%d/claim", env.base, id), player,
		fmt.Sprintf(`{"request_id":%q}`, reqID))
	if code != http.StatusOK {
		t.Fatalf("claim %d = %d body=%s", id, code, body)
	}
}

// assertCouponStatus 断言我的券在指定状态出现。
func assertCouponStatus(t *testing.T, env *testDeliveryEnv, player string, id int64, status string) {
	t.Helper()
	code, body := do(t, http.MethodGet, fmt.Sprintf("%s/coupons/my?status=%s", env.base, status), player, "")
	if code != http.StatusOK || !strings.Contains(body, fmt.Sprintf(`"coupon_id":%d`, id)) {
		t.Fatalf("my coupons status=%s = %d body=%s, want coupon %d present", status, code, body, id)
	}
}

// assertCouponNotStatus 断言我的券在指定状态不出现。
func assertCouponNotStatus(t *testing.T, env *testDeliveryEnv, player string, id int64, status string) {
	t.Helper()
	code, body := do(t, http.MethodGet, fmt.Sprintf("%s/coupons/my?status=%s", env.base, status), player, "")
	if code != http.StatusOK {
		t.Fatalf("my coupons status=%s = %d body=%s", status, code, body)
	}
	if strings.Contains(body, fmt.Sprintf(`"coupon_id":%d`, id)) {
		t.Fatalf("my coupons status=%s body=%s, want coupon %d absent", status, body, id)
	}
}
