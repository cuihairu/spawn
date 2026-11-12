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
)

type testEnv struct {
	base string
	// svcCtx 供测试直接装配事实（如订单投影），避免为断言重复走下单链路。
	svcCtx *svc.ServiceContext
	sign   func(claims utils.JWTClaims) string
	// courierAmounts 供 Courier 存根按 skuId 返回金额（默认 1000）
	courierAmounts map[string]int64
	callbackSeen   int
	// analyticsSeen GP-402 经营分析事实投递记录（data-panel /internal/events
	// 存根收到的批次与令牌），analyticsBase 为空时投递客户端为 no-op。
	analyticsSeen []string
	analyticsTok  string
	analyticsBase string
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

	// Courier 存根：下单返回订单快照；回调校验 HMAC
	courier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/payments/orders":
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

	// data-panel 存根：/internal/events 记录事实事件批次（GP-402）
	datapanel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/events" {
			http.NotFound(w, r)
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

func do(t *testing.T, method, url, token, body string) (int, string) {
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
