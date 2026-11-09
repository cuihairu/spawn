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
	svcCtx     *svc.ServiceContext
	sign       func(claims utils.JWTClaims) string
	gameServer *deliveryGameStub
	promo      *promoStub
	orderSeq   int64
	orderMu    sync.Mutex
	orders     []map[string]any
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
	t.Helper()
	httpx.SetErrorHandlerCtx(logic.ErrorHandler)

	env := &testDeliveryEnv{}

	// game-catalog 存根：/me 判定角色（user 1 = platform_admin）；
	// GET /games/:id/servers 公开区服（s1 开放 / s2 维护 / s99 不存在）。
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/me":
			role := "player"
			if claims, ok := decodeJWT(r.Header.Get("Authorization")); ok && claims.UserId == 1 {
				role = "platform_admin"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"role": role, "partner_id": 1})
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
		http.NotFound(w, r)
	}))
	t.Cleanup(courier.Close)

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
	c.GameCatalog.BaseURL = catalog.URL
	c.Channel.SandboxSecret = testSandboxSecret
	c.Delivery.Secret = testDeliverySecret
	c.Promotions.BaseURL = promo.srv.URL
	c.Promotions.Secret = "promotions-reward-test-secret"

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
