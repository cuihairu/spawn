package handler

// couponhandler.go 剩余分支：Update/Publish/Disable/Revise/List/Get/Claim/
// MyCoupons 的 httpx.Parse 失败臂（畸形 JSON → 400）、逻辑错误臂（CouponDB
// 关闭 → 500），以及 Update/Revise/Get 的成功臂与生命周期（draft→published→
// disabled→draft）。500 臂通过把 svcCtx.CouponDB 换成已关闭的 sqlite 制造
// 「数据库不可用」（CouponDB 仅请求期访问，请求间赋值无数据竞争）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/payments/model"
	"github.com/tappi/tappi/services/payments/utils"
)

func TestRoutes_CouponParseErrorArms(t *testing.T) {
	env := newPaymentsServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})

	endpoints := []struct {
		method, url string
	}{
		{http.MethodPost, "/admin/coupons"},
		{http.MethodPut, "/admin/coupons/1"},
		{http.MethodPost, "/admin/coupons/1/publish"},
		{http.MethodPost, "/admin/coupons/1/disable"},
		{http.MethodPost, "/admin/coupons/1/revise"},
		{http.MethodGet, "/admin/coupons"},
		{http.MethodGet, "/admin/coupons/1"},
		{http.MethodPost, "/coupons/1/claim"},
	}
	for _, ep := range endpoints {
		code, body := do(t, ep.method, env.base+ep.url, admin, `{`)
		if code != http.StatusBadRequest {
			t.Fatalf("%s %s parse err = %d body=%s, want 400", ep.method, ep.url, code, body)
		}
	}
}

func TestRoutes_CouponDBErrorArms(t *testing.T) {
	env := newPaymentsServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	env.svcCtx.CouponDB = model.NewCouponModel(closedDB(t))

	put := func(url, body string, want int) {
		t.Helper()
		code, b := do(t, http.MethodPut, env.base+url, admin, body)
		if code != want {
			t.Fatalf("PUT %s = %d body=%s, want %d", url, code, b, want)
		}
	}
	post := func(url, body string, want int) {
		t.Helper()
		code, b := do(t, http.MethodPost, env.base+url, admin, body)
		if code != want {
			t.Fatalf("POST %s = %d body=%s, want %d", url, code, b, want)
		}
	}
	get := func(url, token string, want int) {
		t.Helper()
		code, b := do(t, http.MethodGet, env.base+url, token, "")
		if code != want {
			t.Fatalf("GET %s = %d body=%s, want %d", url, code, b, want)
		}
	}

	cfg := gpCouponBody("ERR", "错误臂", 0, 100, true, 1)
	put("/admin/coupons/1", cfg, http.StatusInternalServerError)
	post("/admin/coupons/1/publish", "", http.StatusInternalServerError)
	post("/admin/coupons/1/disable", "", http.StatusInternalServerError)
	post("/admin/coupons/1/revise", "", http.StatusInternalServerError)
	get("/admin/coupons", admin, http.StatusInternalServerError)
	get("/admin/coupons/1", admin, http.StatusInternalServerError)
	get("/coupons/my", player, http.StatusInternalServerError)
}

func TestRoutes_CouponUpdateReviseGetArms(t *testing.T) {
	env := newPaymentsServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})

	// 建券 → draft
	code, body := do(t, http.MethodPost, env.base+"/admin/coupons", admin,
		gpCouponBody("LIFE", "生命周期券", 500, 100, true, 1))
	if code != http.StatusOK {
		t.Fatalf("create coupon = %d body=%s", code, body)
	}
	var created struct {
		Coupon struct {
			Id     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"coupon"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil || created.Coupon.Id == 0 {
		t.Fatalf("parse create: %v body=%s", err, body)
	}
	id := created.Coupon.Id
	path := fmt.Sprintf("/admin/coupons/%d", id)

	// Get（草稿）→ 200
	code, _ = do(t, http.MethodGet, env.base+path, admin, "")
	if code != http.StatusOK {
		t.Fatalf("get draft = %d", code)
	}
	// Update（草稿可改）→ 200
	code, _ = do(t, http.MethodPut, env.base+path, admin, gpCouponBody("LIFE-2", "生命周期券改", 500, 80, true, 1))
	if code != http.StatusOK {
		t.Fatalf("update draft = %d", code)
	}
	// Publish（draft→published）→ 200
	code, _ = do(t, http.MethodPost, env.base+path+"/publish", admin, "")
	if code != http.StatusOK {
		t.Fatalf("publish = %d", code)
	}
	// Disable（published→disabled）→ 200
	code, _ = do(t, http.MethodPost, env.base+path+"/disable", admin, "")
	if code != http.StatusOK {
		t.Fatalf("disable = %d", code)
	}
	// Revise（disabled→draft）→ 200
	code, _ = do(t, http.MethodPost, env.base+path+"/revise", admin, "")
	if code != http.StatusOK {
		t.Fatalf("revise = %d", code)
	}
	// Get（草稿视图）→ 200
	code, body = do(t, http.MethodGet, env.base+path, admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"status":"draft"`) {
		t.Fatalf("get revised = %d body=%s", code, body)
	}
}
