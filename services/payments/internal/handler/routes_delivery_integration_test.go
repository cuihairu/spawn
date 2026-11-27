package handler

// GP-203 发货接缝集成测试：webhook 验签先于解析、按订单幂等、发货失败
// 断单（502）+ 补单（权限守卫 + 定时恢复）、区服归属校验、目标回填。
// 复用 routes_integration_test.go 的 do/decodeJWT/testJWTSecret。

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/tappi/tappi/services/payments/internal/config"
	"github.com/tappi/tappi/services/payments/internal/delivery"
	"github.com/tappi/tappi/services/payments/internal/logic"
	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/utils"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

const testDeliverySecret = "payments-delivery-test-secret"

type testDeliveryEnv struct {
	base       string
	dsn        string
	svcCtx     *svc.ServiceContext
	sign       func(claims utils.JWTClaims) string
	gameServer *deliveryGameStub
	promo      *promoStub
	notify     *notifyStub
	orderSeq   int64
	// catalogPartnerID game-catalog 归属存根返回的 partner（F15 归属核验）；
	// 0 = 默认 2。合作方自助（partner 5）用例在起服务后、建商品前覆盖。
	catalogPartnerID int64
	orderMu          sync.Mutex
	orders           []map[string]any
	// GP-204 退款执行器接线：courierAdminToken 非空则注入
	// COURIER_PAYMENTS_ADMIN_TOKEN（执行器非 nil）；refundAdminStatus/Code
	// 控制 Courier 管理面退款端点响应（0=默认成功）。
	courierAdminToken string
	refundAdminCalls  int
	refundAdminStatus int
	refundAdminCode   string
	refundAdminScope  map[string]string
	refundRejectOrder string
	// GP-204 断单对账重发（courier 管理面 /v1/payments/admin/orders/{id}/
	// redeliver）：计数 + scope 断言；Status/Code 非 0 = 模拟确定性拒绝
	//（409 PAYMENT_ORDER_STATE / 404 PAYMENT_ORDER_NOT_FOUND）。
	redeliverAdminCalls  int
	redeliverAdminStatus int
	redeliverAdminCode   string
	redeliverAdminScope  map[string]string
	// GP-202 部署面 SKU 登记（courier 管理面 /v1/payments/admin/skus）：计数
	// + scope + 载荷断言；FailSku 非空 = 该 skuId 按指定 Status/Code 拒绝
	//（其余成功，供部分失败断言）。
	skuAdminCalls   int
	skuAdminMu      sync.Mutex
	skuAdminScope   []map[string]string
	skuAdminBody    []map[string]any
	skuAdminFailSku string
	skuAdminStatus  int
	skuAdminCode    string
}

// notifyStub GP-404 站内通知存根（community /internal/notifications）：记录
// 原始体 + X-Internal-Token 供回流合作方断言。
type notifyStub struct {
	mu   sync.Mutex
	raw  [][]byte
	sigs []string
	srv  *httptest.Server
	// down 置位 = community 短时不可达（返回 503，模拟下游故障）。
	down atomic.Bool
}

func (n *notifyStub) Calls() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.raw)
}

func (n *notifyStub) At(i int) ([]byte, string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if i < 0 || i >= len(n.raw) {
		return nil, ""
	}
	return n.raw[i], n.sigs[i]
}

// promoStub GP-302 promotions 评估接口存根（记录原始体+签名供验签断言）。
type promoStub struct {
	mu   sync.Mutex
	raw  [][]byte
	sigs []string
	srv  *httptest.Server
}

func (p *promoStub) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.raw)
}

func (p *promoStub) Request(i int) ([]byte, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.raw[i], p.sigs[i]
}

// deliveryGameStub 游戏服发货接口存根（可翻转失败；记录调用/载荷/签名）。
type deliveryGameStub struct {
	mu     sync.Mutex
	calls  int
	last   []byte
	sig    string
	fail   atomic.Bool
	srv    *httptest.Server
	secret string
}

func (g *deliveryGameStub) Snapshot() (body []byte, sig string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.last, g.sig
}

func (g *deliveryGameStub) Calls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func newDeliveryServer(t *testing.T) *testDeliveryEnv {
	return newDeliveryServerCfg(t, nil)
}

// newDeliveryServerCfg 在服务构造前注入存根配置（如 Courier 管理面凭证
// 接线），before 为 nil = 默认配置（COURIER_PAYMENTS_ADMIN_TOKEN 空 →
// 退款执行器未接线 nil）。
func newDeliveryServerCfg(t *testing.T, before func(*testDeliveryEnv)) *testDeliveryEnv {
	t.Helper()
	httpx.SetErrorHandlerCtx(logic.ErrorHandler)

	env := &testDeliveryEnv{}
	if before != nil {
		before(env)
	}

	// game-catalog 存根：/me 判定角色（user 1 = platform_admin）；
	// GET /games/:id/servers 公开区服（s1 开放 / s2 维护 / s99 不存在）。
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/internal/games/") && strings.HasSuffix(r.URL.Path, "/ownership"):
			// F15 归属权威存根：game-a → partner 2/5 依测试声明（同一游戏可
			// 在不同的独立服务实例里绑定不同 partner，故按请求 URL 无法区分；
			// 这里统一返回 active 的 partner 2，合作方自助用例单独起服务时
			// 通过 env.catalogPartnerID 覆盖为 5）。
			if r.Header.Get("X-Internal-Token") != testInternalToken {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/internal/games/"), "/ownership")
			partner := int64(2)
			if env.catalogPartnerID != 0 {
				partner = env.catalogPartnerID
			}
			if strings.HasPrefix(id, "noowner") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"partner_id": partner, "status": "active"})
		case r.URL.Path == "/me":
			role, partnerID := "player", int64(1)
			if claims, ok := decodeJWT(r.Header.Get("Authorization")); ok {
				if claims.UserId == 1 {
					role = "platform_admin"
				}
				// GP-404：user 2 绑定合作方 5（合作方自助面的行级隔离口径）
				if claims.UserId == 2 {
					role, partnerID = "partner", 5
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"role": role, "partner_id": partnerID})
		case strings.HasPrefix(r.URL.Path, "/games/") && strings.HasSuffix(r.URL.Path, "/servers"):
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{
				{"id": 1, "status": "open"},
				{"id": 2, "status": "maintenance"},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(catalog.Close)

	// Courier 存根：唯一订单号 order_<sku>-<seq>；记录已建订单供列表查询
	courier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/payments/orders" {
			var body struct {
				SkuId string `json:"skuId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			seq := atomic.AddInt64(&env.orderSeq, 1)
			order := map[string]any{
				"id": fmt.Sprintf("order_%s-%d", body.SkuId, seq), "status": "CREATED",
				"skuId": body.SkuId, "amountCents": 1000, "currency": "CNY",
				"payToken": "pay_" + body.SkuId, "createdAt": "2026-11-09T12:00:00.000Z",
				"updatedAt": "2026-11-09T12:00:00.000Z",
			}
			env.orderMu.Lock()
			env.orders = append(env.orders, order)
			env.orderMu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": order})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/v1/payments/orders" {
			env.orderMu.Lock()
			items := make([]map[string]any, len(env.orders))
			copy(items, env.orders)
			env.orderMu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": items, "nextCursor": ""}})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v1/payments/callback" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"accepted": true}})
			return
		}
		// GP-204 管理面退款端点存根：默认成功登记（幂等既有不叠加）；可翻转
		// 状态码/错误码模拟确定性拒绝（如 409 PAYMENT_ORDER_STATE）。
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/payments/admin/orders/") &&
			strings.HasSuffix(r.URL.Path, "/refund") {
			orderID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/payments/admin/orders/"), "/refund")
			env.refundAdminCalls++
			env.refundAdminScope = map[string]string{
				"gameId": r.Header.Get("X-Courier-Game-Id"),
				"env":    r.Header.Get("X-Courier-Env"),
				"auth":   r.Header.Get("Authorization"),
			}
			// refundRejectOrder 非空时仅该订单按 status/code 拒绝（供批量扫
			// 描测试区分接受/拒绝两笔）。
			if env.refundRejectOrder == "" || orderID == env.refundRejectOrder {
				if env.refundAdminStatus != 0 {
					w.WriteHeader(env.refundAdminStatus)
					_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
						"code": env.refundAdminCode, "message": "courier 拒绝详情",
					}})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
					"orderId": orderID,
					"refund":  map[string]any{"action": "REFUND", "amountCents": 600, "at": "2026-11-23T00:00:00Z"},
				}})
				return
			}
			// 非目标订单（且 refundRejectOrder 已设）：按成功处理（批量扫描的
			// 接受笔）。
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"orderId": orderID,
				"refund":  map[string]any{"action": "REFUND", "amountCents": 600, "at": "2026-11-23T00:00:00Z"},
			}})
			return
		}
		// GP-202 部署面 SKU 登记端点存根：默认成功登记回显 skuId；FailSku 非空
		// 时该 skuId 按指定 Status/Code 拒绝（其余仍成功，供部分失败断言）。
		if r.Method == http.MethodPost && r.URL.Path == "/v1/payments/admin/skus" {
			var body struct {
				SkuId       string `json:"skuId"`
				ProductId   string `json:"productId"`
				Form        string `json:"form"`
				AmountCents int64  `json:"amountCents"`
				Currency    string `json:"currency"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			env.skuAdminMu.Lock()
			env.skuAdminCalls++
			env.skuAdminScope = append(env.skuAdminScope, map[string]string{
				"gameId": r.Header.Get("X-Courier-Game-Id"),
				"env":    r.Header.Get("X-Courier-Env"),
				"auth":   r.Header.Get("Authorization"),
			})
			env.skuAdminBody = append(env.skuAdminBody, map[string]any{
				"skuId": body.SkuId, "productId": body.ProductId, "form": body.Form,
				"amountCents": body.AmountCents, "currency": body.Currency,
			})
			failSku := env.skuAdminFailSku
			failStatus, failCode := env.skuAdminStatus, env.skuAdminCode
			env.skuAdminMu.Unlock()
			if failSku != "" && body.SkuId == failSku && failStatus != 0 {
				w.WriteHeader(failStatus)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
					"code": failCode, "message": "courier 拒绝详情",
				}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"sku": map[string]any{
					"id": body.SkuId, "amountCents": body.AmountCents, "currency": body.Currency,
				},
			}})
			return
		}
		// GP-204 断单对账重发端点存根：默认成功（redelivered=true, DELIVERED）；
		// 可翻转 Status/Code 模拟确定性拒绝（409 PAYMENT_ORDER_STATE 已发货、
		// 404 PAYMENT_ORDER_NOT_FOUND）。
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/payments/admin/orders/") &&
			strings.HasSuffix(r.URL.Path, "/redeliver") {
			orderID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/payments/admin/orders/"), "/redeliver")
			env.redeliverAdminCalls++
			env.redeliverAdminScope = map[string]string{
				"gameId": r.Header.Get("X-Courier-Game-Id"),
				"env":    r.Header.Get("X-Courier-Env"),
				"auth":   r.Header.Get("Authorization"),
			}
			if env.redeliverAdminStatus != 0 {
				w.WriteHeader(env.redeliverAdminStatus)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
					"code": env.redeliverAdminCode, "message": "courier 拒绝详情",
				}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"order": map[string]any{
					"id": orderID, "status": "DELIVERED", "amountCents": 1000, "currency": "CNY",
				},
				"redelivered": true,
			}})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(courier.Close)

	// GP-202 映射存根：spawn_game_id → courier scope（下单/查询前解析）。
	mapping := newMappingStub(t)

	// 游戏服发货接口存根
	game := &deliveryGameStub{secret: "gs-secret"}
	game.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		game.mu.Lock()
		game.last = raw
		game.sig = r.Header.Get("X-Game-Delivery-Signature")
		game.calls++
		game.mu.Unlock()
		if game.fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(game.srv.Close)
	env.gameServer = game

	// GP-302 promotions 评估接口存根（默认受理 200）
	promo := &promoStub{}
	promo.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		promo.mu.Lock()
		promo.raw = append(promo.raw, raw)
		promo.sigs = append(promo.sigs, r.Header.Get("X-Reward-Signature"))
		promo.mu.Unlock()
		_, _ = w.Write([]byte(`{"order_id":"x","replay":false,"grants":[],"skipped":[]}`))
	}))
	t.Cleanup(promo.srv.Close)
	env.promo = promo

	// GP-404 站内通知存根（community /internal/notifications）：记录原始体+令牌
	notify := &notifyStub{}
	notify.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if notify.down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		notify.mu.Lock()
		notify.raw = append(notify.raw, raw)
		notify.sigs = append(notify.sigs, r.Header.Get("X-Internal-Token"))
		notify.mu.Unlock()
		if r.Header.Get("X-Internal-Token") != testNotifyToken {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"created":1}`))
	}))
	t.Cleanup(notify.srv.Close)
	env.notify = notify

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	var c config.Config
	c.Port = port
	c.MySQL.DataSource = "file:" + t.TempDir() + "/delivery.db"
	c.Auth.JWTSecret = testJWTSecret
	c.Courier.BaseURL = courier.URL
	c.Courier.AdminToken = env.courierAdminToken // 空 = 执行器未接线（nil，保持 PENDING）
	c.GameCatalog.BaseURL = catalog.URL
	c.CourierAdapter.BaseURL = mapping.URL
	c.Internal.Token = testInternalToken
	c.Channel.SandboxSecret = testSandboxSecret
	c.Delivery.Secret = testDeliverySecret
	c.Promotions.BaseURL = promo.srv.URL
	c.Promotions.Secret = "promotions-reward-test-secret"
	c.Notify.BaseURL = notify.srv.URL
	c.Notify.Token = testNotifyToken

	svcCtx := svc.NewServiceContext(c)
	server := rest.MustNewServer(c.RestConf)
	RegisterHandlers(server, svcCtx)
	go server.Start()
	t.Cleanup(server.Stop)

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("delivery server not ready")
		}
		time.Sleep(20 * time.Millisecond)
	}

	env.base = base
	env.dsn = c.MySQL.DataSource
	env.svcCtx = svcCtx
	env.sign = func(claims utils.JWTClaims) string {
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := tok.SignedString([]byte(testJWTSecret))
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		return signed
	}
	return env
}

// deliverySig 计算发货 webhook 的 HMAC-SHA256 十六进制签名。
func deliverySig(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

// dispatchRaw 以自定义签名投递发货 webhook（带签名头）。
func dispatchRaw(t *testing.T, url, body, sig string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(body)))
	req.Header.Set("X-Delivery-Signature", sig)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func TestRoutes_DeliveryFlow(t *testing.T) {
	env := newDeliveryServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	// 1. 管理面配置游戏服发货接口；player 越权 403
	code, body := do(t, http.MethodPut, env.base+"/admin/games/game-a/delivery", admin,
		fmt.Sprintf(`{"endpoint":%q,"secret":"gs-secret"}`, env.gameServer.srv.URL))
	if code != http.StatusOK {
		t.Fatalf("configure endpoint = %d body=%s", code, body)
	}
	code, _ = do(t, http.MethodGet, env.base+"/admin/games/game-a/delivery", admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"secret_set":true`) || strings.Contains(body, "gs-secret") {
		t.Fatalf("get endpoint = %d body=%s (密钥不得回显)", code, body)
	}
	code, _ = do(t, http.MethodPut, env.base+"/admin/games/game-a/delivery", player, `{"endpoint":"http://x","secret":"y"}`)
	if code != http.StatusForbidden {
		t.Fatalf("player configure endpoint = %d, want 403", code)
	}

	// 2. 商品配置 + 带充值目标下单
	code, body = do(t, http.MethodPost, env.base+"/admin/products/sku-gem-100", admin,
		`{"product_id":"gem-100","form":"DIRECT_PURCHASE","amount_cents":1000,"currency":"CNY","game_id":"game-a","partner_id":2}`)
	if code != http.StatusOK {
		t.Fatalf("create product = %d body=%s", code, body)
	}
	code, body = do(t, http.MethodPost, env.base+"/games/game-a/orders", player,
		`{"sku_id":"sku-gem-100","server_id":1,"role_id":"role-9"}`)
	if code != http.StatusOK {
		t.Fatalf("create order = %d body=%s", code, body)
	}
	var orderResp struct {
		Order struct {
			Id string `json:"id"`
		} `json:"order"`
	}
	if err := json.Unmarshal([]byte(body), &orderResp); err != nil || orderResp.Order.Id == "" {
		t.Fatalf("parse order = %v body=%s", err, body)
	}
	orderID := orderResp.Order.Id

	// 3. 区服归属校验：维护区/不存在区 → 404
	code, _ = do(t, http.MethodPost, env.base+"/games/game-a/orders", player, `{"sku_id":"sku-gem-100","server_id":2}`)
	if code != http.StatusNotFound {
		t.Fatalf("maintenance server order = %d, want 404", code)
	}
	code, _ = do(t, http.MethodPost, env.base+"/games/game-a/orders", player, `{"sku_id":"sku-gem-100","server_id":99}`)
	if code != http.StatusNotFound {
		t.Fatalf("unknown server order = %d, want 404", code)
	}

	// 4. 发货 webhook：验签先于解析（缺签名/坏签名 403；好签名+坏体 400）
	dispatchURL := env.base + "/internal/deliveries"
	validBody := fmt.Sprintf(`{"orderId":%q,"accountId":"7","skuId":"sku-gem-100","productId":"gem-100","form":"DIRECT_PURCHASE","amountCents":1000,"currency":"CNY"}`, orderID)
	code, _ = do(t, http.MethodPost, dispatchURL, "", validBody)
	if code != http.StatusForbidden {
		t.Fatalf("missing sig = %d, want 403", code)
	}
	code, _ = dispatchRaw(t, dispatchURL, `{"not-json`, "deadbeef")
	if code != http.StatusForbidden {
		t.Fatalf("garbage+bad sig = %d, want 403 (验签先于解析)", code)
	}
	code, _ = dispatchRaw(t, dispatchURL, `{"not-json`, deliverySig(testDeliverySecret, `{"not-json`))
	if code != http.StatusBadRequest {
		t.Fatalf("garbage+valid sig = %d, want 400", code)
	}
	code, _ = dispatchRaw(t, dispatchURL, validBody, deliverySig(testDeliverySecret, validBody))
	if code != http.StatusOK {
		t.Fatalf("valid dispatch = %d", code)
	}
	gotBody, gotSig := env.gameServer.Snapshot()
	mac := hmac.New(sha256.New, []byte("gs-secret"))
	mac.Write(gotBody)
	if want := hex.EncodeToString(mac.Sum(nil)); gotSig != want {
		t.Fatalf("game signature = %q, want %q", gotSig, want)
	}
	for _, want := range []string{orderID, `"serverId":1`, `"roleId":"role-9"`, `"productId":"gem-100"`, `"amountCents":1000`} {
		if !strings.Contains(string(gotBody), want) {
			t.Fatalf("game payload missing %s: %s", want, gotBody)
		}
	}

	// 5. 重复回调（同订单再次投递）→ 200 幂等，游戏服只收到一次
	code, _ = dispatchRaw(t, dispatchURL, validBody, deliverySig(testDeliverySecret, validBody))
	if code != http.StatusOK {
		t.Fatalf("re-dispatch = %d, want 200", code)
	}
	if env.gameServer.Calls() != 1 {
		t.Fatalf("game calls = %d, want 1 (同订单不二次发货)", env.gameServer.Calls())
	}

	// 6. 未关联订单（无充值目标）→ 502，账本建 failed 待补单
	unknownBody := `{"orderId":"order_unknown","accountId":"7","skuId":"sku-gem-100","productId":"gem-100","form":"DIRECT_PURCHASE","amountCents":1000,"currency":"CNY"}`
	code, _ = dispatchRaw(t, dispatchURL, unknownBody, deliverySig(testDeliverySecret, unknownBody))
	if code != http.StatusBadGateway {
		t.Fatalf("unknown order dispatch = %d, want 502", code)
	}

	// 7. 断单补单：游戏服失败 → 502；player 补单 403；admin 补单 502（仍失败）
	env.gameServer.fail.Store(true)
	code, body = do(t, http.MethodPost, env.base+"/games/game-a/orders", player, `{"sku_id":"sku-gem-100","server_id":1}`)
	if code != http.StatusOK {
		t.Fatalf("create order2 = %d body=%s", code, body)
	}
	var o2 struct {
		Order struct {
			Id string `json:"id"`
		} `json:"order"`
	}
	_ = json.Unmarshal([]byte(body), &o2)
	order2 := o2.Order.Id
	body2 := fmt.Sprintf(`{"orderId":%q,"accountId":"7","skuId":"sku-gem-100","productId":"gem-100","form":"DIRECT_PURCHASE","amountCents":1000,"currency":"CNY"}`, order2)
	code, _ = dispatchRaw(t, dispatchURL, body2, deliverySig(testDeliverySecret, body2))
	if code != http.StatusBadGateway {
		t.Fatalf("dispatch while game down = %d, want 502", code)
	}
	code, _ = do(t, http.MethodPost, env.base+"/admin/deliveries/"+order2+"/redeliver", player, "")
	if code != http.StatusForbidden {
		t.Fatalf("player redeliver = %d, want 403", code)
	}
	code, _ = do(t, http.MethodPost, env.base+"/admin/deliveries/"+order2+"/redeliver", admin, "")
	if code != http.StatusBadGateway {
		t.Fatalf("admin redeliver while down = %d, want 502", code)
	}

	// 8. 游戏服恢复 → 补单成功；已发货再补单 409
	env.gameServer.fail.Store(false)
	code, _ = do(t, http.MethodPost, env.base+"/admin/deliveries/"+order2+"/redeliver", admin, "")
	if code != http.StatusOK {
		t.Fatalf("admin redeliver after recover = %d, want 200", code)
	}
	code, _ = do(t, http.MethodPost, env.base+"/admin/deliveries/"+order2+"/redeliver", admin, "")
	if code != http.StatusConflict {
		t.Fatalf("redeliver delivered order = %d, want 409", code)
	}
	code, body = do(t, http.MethodGet, env.base+"/admin/deliveries?game_id=game-a", admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"status":"delivered"`) || !strings.Contains(body, order2) {
		t.Fatalf("list deliveries = %d body=%s", code, body)
	}
	code, _ = do(t, http.MethodGet, env.base+"/admin/deliveries?game_id=game-a", player, "")
	if code != http.StatusForbidden {
		t.Fatalf("player list deliveries = %d, want 403", code)
	}

	// 9. 定时恢复：从未投递的订单（pending）在游戏服恢复后被扫出投递
	code, body = do(t, http.MethodPost, env.base+"/games/game-a/orders", player, `{"sku_id":"sku-gem-100","server_id":1}`)
	if code != http.StatusOK {
		t.Fatalf("create order3 = %d", code)
	}
	var o3 struct {
		Order struct {
			Id string `json:"id"`
		} `json:"order"`
	}
	_ = json.Unmarshal([]byte(body), &o3)
	rec := delivery.NewRecovery(env.svcCtx.DeliveryDB, env.svcCtx.Dispatcher, time.Second)
	attempted, delivered := rec.RecoverOnce(context.Background())
	if attempted == 0 || delivered == 0 {
		t.Fatalf("recover once = (%d, %d), want > 0", attempted, delivered)
	}
	// 已全部投递后再扫一轮：不重复投
	if _, delivered = rec.RecoverOnce(context.Background()); delivered != 0 {
		t.Fatalf("second recover delivered = %d, want 0", delivered)
	}

	// 10. 目标回填 + 补单：未关联订单回填区服后投递成功
	code, _ = do(t, http.MethodPatch, env.base+"/admin/deliveries/order_unknown/target", admin, `{"server_id":1,"role_id":"r-back"}`)
	if code != http.StatusOK {
		t.Fatalf("backfill target = %d, want 200", code)
	}
	code, _ = do(t, http.MethodPost, env.base+"/admin/deliveries/order_unknown/redeliver", admin, "")
	if code != http.StatusOK {
		t.Fatalf("redeliver backfilled = %d, want 200", code)
	}
}
