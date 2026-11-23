package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/tappi/tappi/services/payments/internal/config"
	"github.com/tappi/tappi/services/payments/internal/logic"
	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/utils"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

const (
	testJWTSecret     = "payments-integration-secret"
	testSandboxSecret = "payments-sandbox-test"
	testNotifyToken   = "payments-notify-test-token"
	// testCourierToken 模拟客户端经 GP-201 代登录/票据兑换取得的 Courier
	// scope-bound accessToken（X-Courier-Authorization 头携带，见 GP-202 缺口）。
	testCourierToken = "payments-courier-session-token"
	// testInternalToken courier-adapter 映射存根的 X-Internal-Token（GP-202
	// scope 解析：payments 下单/查询前经 /internal/games/:id/mapping 取
	// courier_game_id+env，绝不用 spawn 游戏 id 当 Courier scope）。
	testInternalToken = "payments-mapping-s2s-token"
	// testCourierGameEnv 映射存根返回的 courier 侧 scope（spawn game-a → 该值）。
	testCourierGameEnv = "courier-game-a|dev"
)

type testEnv struct {
	base string
	// svcCtx 供测试直接装配事实（如订单投影），避免为断言重复走下单链路。
	svcCtx *svc.ServiceContext
	sign   func(claims utils.JWTClaims) string
	// courierAmounts 供 Courier 存根按 skuId 返回金额（默认 1000）
	courierAmounts map[string]int64
	callbackSeen   int
	// courierAuthSeen 记录 Courier 存根在 /v1/payments/orders 收到的
	// Authorization 头（GP-202 断言：必须是 Courier 令牌，非平台 JWT）。
	courierAuthSeen []string
	// courierScopeSeen 记录 Courier 存根收到的 X-Courier-Game-Id/Env
	// （GP-202 scope 断言：必须是映射后的 courier_game_id+env，非 spawn id）。
	courierScopeSeen []string
	// analyticsSeen GP-402 经营分析事实投递记录（data-panel /internal/events
	// 存根收到的批次与令牌），analyticsBase 为空时投递客户端为 no-op。
	// analyticsDown 置位 = data-panel 存根返回 503（模拟下游短时不可达）。
	analyticsSeen []string
	analyticsTok  string
	analyticsBase string
	analyticsDown atomic.Bool
}

func newPaymentsServer(t *testing.T) *testEnv {
	t.Helper()
	httpx.SetErrorHandlerCtx(logic.ErrorHandler)

	env := &testEnv{courierAmounts: map[string]int64{}}

	// game-catalog 存根：/me 解码 JWT 判定角色（user 1 → platform_admin）
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, partner := "player", int64(0)
		if claims, ok := decodeJWT(r.Header.Get("Authorization")); ok {
			if claims.UserId == 1 {
				role, partner = "platform_admin", 1
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"role": role, "partner_id": partner})
	}))
	t.Cleanup(catalog.Close)

	// courier-adapter 映射存根（GP-202 scope 解析）：/internal/games/:id/mapping
	// 校验 X-Internal-Token，返回映射后的 courier scope（spawn game-a →
	// courier-game-a + dev）。payments 下单/查询前调用它，绝不用 spawn id。
	mapping := newMappingStub(t)

	// Courier 存根：下单返回订单快照；回调校验 HMAC
	courier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/payments/orders":
			env.courierAuthSeen = append(env.courierAuthSeen, r.Header.Get("Authorization"))
			env.courierScopeSeen = append(env.courierScopeSeen,
				r.Header.Get("X-Courier-Game-Id")+"|"+r.Header.Get("X-Courier-Env"))
			var body struct {
				SkuId string `json:"skuId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			amt, ok := env.courierAmounts[body.SkuId]
			if !ok {
				amt = 1000
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"id": "order_" + body.SkuId, "status": "CREATED", "skuId": body.SkuId,
				"amountCents": amt, "currency": "CNY", "payToken": "pay_" + body.SkuId,
				"createdAt": "2026-11-09T12:00:00.000Z",
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/payments/callback":
			raw, _ := io.ReadAll(r.Body)
			mac := hmac.New(sha256.New, []byte(testSandboxSecret))
			mac.Write(raw)
			want := hex.EncodeToString(mac.Sum(nil))
			if r.Header.Get("X-Payment-Signature") != want {
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "PAYMENT_INVALID_SIGNATURE", "message": "bad"}})
				return
			}
			env.callbackSeen++
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(courier.Close)

	// data-panel 存根：/internal/events 记录事实事件批次（GP-402）；
	// analyticsDown 置位时返回 503（模拟下游短时不可达）。
	datapanel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/events" {
			http.NotFound(w, r)
			return
		}
		if env.analyticsDown.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		env.analyticsTok = r.Header.Get("X-Internal-Token")
		env.analyticsSeen = append(env.analyticsSeen, string(raw))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"accepted": 1})
	}))
	t.Cleanup(datapanel.Close)

	// 预占空闲端口
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	var c config.Config
	c.Port = port
	c.MySQL.DataSource = "file:" + t.TempDir() + "/payments.db"
	c.Auth.JWTSecret = testJWTSecret
	c.Courier.BaseURL = courier.URL
	c.GameCatalog.BaseURL = catalog.URL
	c.CourierAdapter.BaseURL = mapping.URL
	c.Internal.Token = testInternalToken
	c.Channel.SandboxSecret = testSandboxSecret
	c.Analytics.BaseURL = datapanel.URL
	c.Analytics.Token = "gp402-payments-token"
	env.analyticsBase = datapanel.URL

	svcCtx := svc.NewServiceContext(c)
	env.svcCtx = svcCtx
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
			t.Fatal("server not ready")
		}
		time.Sleep(20 * time.Millisecond)
	}

	env.base = base
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

// newMappingStub courier-adapter 映射存根（GP-202 scope 解析）：
// /internal/games/:id/mapping 校验 X-Internal-Token（testInternalToken），
// 返回映射后的 courier scope（spawn <id> → courier-<id> + dev）且 payment
// 能力开（GP-002 能力门）；id 前缀 nomap 返 404（无映射）、err500 返 500
// （映射服务故障）、nopay 返 200 但无能力位（未开通支付，下单须 403）。
// payments 下单/查询前调用它，绝不用 spawn id 当 Courier scope。
func newMappingStub(t *testing.T) *httptest.Server {
	t.Helper()
	mapping := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Token") != testInternalToken {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"code":403,"message":"forbidden"}`))
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/internal/games/"), "/")
		id := parts[0]
		if strings.HasPrefix(id, "nomap") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":404,"message":"游戏未接入 Courier"}`))
			return
		}
		if strings.HasPrefix(id, "err500") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"code":500,"message":"mapping unavailable"}`))
			return
		}
		if strings.HasPrefix(id, "nopay") {
			// 映射存在但 payment 能力未开（缺 capabilities，fail-closed false）。
			_ = json.NewEncoder(w).Encode(map[string]any{
				"courierGameId": "courier-" + id, "env": "dev",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"courierGameId": "courier-" + id, "env": "dev",
			"capabilities": map[string]any{"payment": true},
		})
	}))
	t.Cleanup(mapping.Close)
	return mapping
}

func decodeJWT(authHeader string) (*utils.JWTClaims, bool) {
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, false
	}
	parts := strings.Split(strings.TrimPrefix(authHeader, "Bearer "), ".")
	if len(parts) != 3 {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var claims utils.JWTClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, false
	}
	return &claims, true
}

// do 发请求并携带平台 JWT（Authorization）+ 客户端 Courier 会话令牌
// （X-Courier-Authorization，GP-202 客户端带 Courier token 决策）两个头。
// 玩家订单/查询面需后者（Courier 归属/验权），管理/券/账单面忽略它。
func do(t *testing.T, method, url, token, body string) (int, string) {
	t.Helper()
	return doWithCourier(t, method, url, token, "Bearer "+testCourierToken, body)
}

// doNoCourier 发请求仅带平台 JWT、不带 Courier 会话令牌——用于断言玩家
// 订单面缺失 Courier token 时 fail-closed 401（不静默回退平台 JWT）。
func doNoCourier(t *testing.T, method, url, token, body string) (int, string) {
	t.Helper()
	return doWithCourier(t, method, url, token, "", body)
}

func doWithCourier(t *testing.T, method, url, token, courierAuth, body string) (int, string) {
	t.Helper()
	var req *http.Request
	if body == "" {
		req, _ = http.NewRequest(method, url, nil)
	} else {
		req, _ = http.NewRequest(method, url, bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if courierAuth != "" {
		req.Header.Set("X-Courier-Authorization", courierAuth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func TestRoutes_PaymentsFlow(t *testing.T) {
	env := newPaymentsServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	// 1. admin 建商品（服务端定价 1000 CNY，游戏 game-a，商户 2）
	code, body := do(t, http.MethodPost, env.base+"/admin/products/sku-gem-100", admin,
		`{"product_id":"gem-100","form":"DIRECT_PURCHASE","amount_cents":1000,"currency":"CNY","game_id":"game-a","partner_id":2}`)
	if code != http.StatusOK {
		t.Fatalf("create product = %d body=%s", code, body)
	}

	// 2. player 在 game-a 下单 → 200（金额核验通过）
	env.courierAmounts["sku-gem-100"] = 1000
	code, body = do(t, http.MethodPost, env.base+"/games/game-a/orders", player, `{"sku_id":"sku-gem-100"}`)
	if code != http.StatusOK || !strings.Contains(body, "order_sku-gem-100") {
		t.Fatalf("create order = %d body=%s", code, body)
	}

	// 3. player 在 game-b 下单 game-a 的 SKU → 404（跨游戏越权）
	code, _ = do(t, http.MethodPost, env.base+"/games/game-b/orders", player, `{"sku_id":"sku-gem-100"}`)
	if code != http.StatusNotFound {
		t.Fatalf("cross-game order = %d, want 404", code)
	}

	// 4. Courier 返回金额漂移（配置 1000 vs Courier 999）→ 409
	env.courierAmounts["sku-gem-100"] = 999
	code, _ = do(t, http.MethodPost, env.base+"/games/game-a/orders", player, `{"sku_id":"sku-gem-100"}`)
	if code != http.StatusConflict {
		t.Fatalf("amount mismatch order = %d, want 409", code)
	}

	// 5. 玩家可购 SKU 列表（启用）
	code, body = do(t, http.MethodGet, env.base+"/games/game-a/skus", player, "")
	if code != http.StatusOK || !strings.Contains(body, "sku-gem-100") {
		t.Fatalf("list skus = %d body=%s", code, body)
	}

	// 6. 沙箱回调：有效 HMAC → 200 且转发到 Courier；无效 → 403
	cbBody := `{"orderId":"order_sku-gem-100","paidAt":"2026-11-09T12:01:00.000Z"}`
	mac := hmac.New(sha256.New, []byte(testSandboxSecret))
	mac.Write([]byte(cbBody))
	goodSig := hex.EncodeToString(mac.Sum(nil))
	code, _ = do(t, http.MethodPost, env.base+"/callback/sandbox", "", cbBody)
	if code != http.StatusForbidden {
		t.Fatalf("missing sig = %d, want 403", code)
	}
	code, _ = do(t, http.MethodPost, env.base+"/callback/sandbox", "", "")
	if code != http.StatusForbidden {
		t.Fatalf("empty body = %d, want 403", code)
	}
	req, _ := http.NewRequest(http.MethodPost, env.base+"/callback/sandbox", bytes.NewReader([]byte(cbBody)))
	req.Header.Set("X-Payment-Signature", goodSig)
	resp, _ := http.DefaultClient.Do(req)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid sandbox callback = %d, want 200", resp.StatusCode)
	}
	if env.callbackSeen != 1 {
		t.Fatalf("courier callback seen = %d, want 1", env.callbackSeen)
	}

	// 7. 真实渠道未接入 → 501（绝不拿沙箱 HMAC 冒充）；签名存在但不被接受
	req2, _ := http.NewRequest(http.MethodPost, env.base+"/callback/real", bytes.NewReader([]byte(cbBody)))
	req2.Header.Set("X-Payment-Signature", "native-channel-sig")
	resp2, _ := http.DefaultClient.Do(req2)
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotImplemented {
		t.Fatalf("real channel = %d, want 501", resp2.StatusCode)
	}

	// 8. 管理面角色守卫：player 建商品 → 403
	code, _ = do(t, http.MethodPost, env.base+"/admin/products/sku-x", player,
		`{"product_id":"x","form":"DIRECT_PURCHASE","amount_cents":1,"currency":"CNY","game_id":"game-a"}`)
	if code != http.StatusForbidden {
		t.Fatalf("player create product = %d, want 403", code)
	}
}
