package handler

// routes.go 剩余补漏分支：各 handler 的逻辑错误臂（底层 DB/Courier 故障 →
// 500/502）、AuthMiddleware 的空令牌 401 臂、InternalGetOrder 的查询失败 500，
// 以及 GetStatementHandler 的成功臂。DB 臂通过把 svcCtx.*DB 换成已关闭 sqlite
// 制造「首个查询即败」；Partner 面把 game-catalog 指到返回 partner 角色的存根。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/courier"
	"github.com/tappi/tappi/services/payments/model"
	"github.com/tappi/tappi/services/payments/utils"
)

// partnerCatalog 返回 platform_admin（user 1）/ partner（user 7），使同一环境
// 内 admin 与 partner 路由都能通过角色守卫。
func partnerCatalog(t *testing.T) *httptest.Server {
	t.Helper()
	st := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, pid := "player", int64(0)
		if claims, ok := decodeJWT(r.Header.Get("Authorization")); ok {
			if claims.UserId == 1 {
				role, pid = "platform_admin", 1
			}
			if claims.UserId == 7 {
				role, pid = "partner", 2
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"role": role, "partner_id": pid})
	}))
	t.Cleanup(st.Close)
	return st
}

func TestRoutes_HandlerLogicErrorArms(t *testing.T) {
	env := newPaymentsServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	// 关闭底层连接：首个查询即败 → 500
	env.svcCtx.ProjectionDB = model.NewOrderProjectionModel(closedDB(t))
	env.svcCtx.DeliveryDB = model.NewDeliveryModel(closedDB(t))
	env.svcCtx.StatementDB = model.NewStatementModel(closedDB(t))
	env.svcCtx.Courier = courier.NewClient("http://127.0.0.1:1", time.Second)
	env.svcCtx.Config.Internal.Token = "internal-test-token"
	env.svcCtx.Config.GameCatalog.BaseURL = partnerCatalog(t).URL

	// 管理面
	get := func(url, token string, want int) {
		t.Helper()
		if code, body := do(t, http.MethodGet, env.base+url, token, ""); code != want {
			t.Fatalf("GET %s = %d body=%s, want %d", url, code, body, want)
		}
	}
	get("/admin/orders/o1", admin, http.StatusInternalServerError)
	get("/admin/refunds", admin, http.StatusInternalServerError)
	get("/admin/audit", admin, http.StatusInternalServerError)
	get("/admin/games/game-a/delivery", admin, http.StatusInternalServerError)
	get("/admin/statements/1", admin, http.StatusInternalServerError)
	if code, body := do(t, http.MethodPost, env.base+"/admin/statements/1/confirm", admin, ""); code != http.StatusInternalServerError {
		t.Fatalf("confirm = %d body=%s, want 500", code, body)
	}
	if code, body := do(t, http.MethodPatch, env.base+"/admin/deliveries/o1/target", admin, `{"server_id":1,"role_id":"r1"}`); code != http.StatusInternalServerError {
		t.Fatalf("backfill = %d body=%s, want 500", code, body)
	}

	// 玩家我的订单（Courier 不可达）→ 502
	get("/games/game-a/orders", player, http.StatusBadGateway)

	// 合作方自助面（角色 partner 已绑定）→ 500
	get("/partner/statements", player, http.StatusInternalServerError)
	get("/partner/statements/differences", player, http.StatusInternalServerError)

	// S2S 订单事实查询：令牌正确但底层查询失败 → 500
	req, _ := http.NewRequest(http.MethodGet, env.base+"/internal/orders/o1", nil)
	req.Header.Set("X-Internal-Token", "internal-test-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("internal get: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("internal get = %d, want 500", resp.StatusCode)
	}
}

func TestRoutes_AuthEmptyTokenArm(t *testing.T) {
	env := newPaymentsServer(t)
	// 原始请求：Authorization 恰好为 "Bearer "（trim 后为空）→ 401
	req, _ := http.NewRequest(http.MethodGet, env.base+"/games/game-a/skus", nil)
	req.Header.Set("Authorization", "Bearer ")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("empty token: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("empty token = %d, want 401", resp.StatusCode)
	}
}

func TestRoutes_GetStatementSuccessArm(t *testing.T) {
	env := newPaymentsServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})

	// 生成一张对账单（零订单也可生成），再查详情 → 200
	code, body := do(t, http.MethodPost, env.base+"/admin/statements", admin,
		`{"vendor":"sandbox","partner_id":2,"period_start":"2026-01-01","period_end":"2026-01-31","split_rate_bp":7000}`)
	if code != http.StatusOK {
		t.Fatalf("generate = %d body=%s", code, body)
	}
	code, body = do(t, http.MethodGet, env.base+"/admin/statements/1", admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"id":1`) {
		t.Fatalf("get statement = %d body=%s", code, body)
	}
}
