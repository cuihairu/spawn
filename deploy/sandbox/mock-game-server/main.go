// Command mock-game-server 是 Spawn 发货接缝（GP-203）的参考游戏服实现：
// 沙箱走查此前用「宿主 python3 现写几十行 POST 恒 200」的临时桩（README
// 口径），不验签、不幂等——与生产契约只差一个 200。本实现把契约固化成
// 可运行的参考：真实游戏服团队按本文件逐条对照接入即可。
//
// 契约（payments services/payments/internal/delivery/dispatcher.go 出站面）：
//   - POST {endpoint}（per-game 在 payments PUT /admin/games/:gameId/delivery
//     配置的 URL + secret），Content-Type: application/json；
//   - X-Game-Delivery-Signature = hex(HMAC-SHA256(secret, rawBody))，
//     **验签先于解析**（payment.md 安全边界 2 同款纪律）；
//   - 体：{"orderId","accountId","serverId","roleId","productId","skuId",
//     "form","amountCents","currency"}；
//   - 2xx = 发货成功（账本转 delivered）；非 2xx/不可达 = 失败，payments
//     按指数退避重投——故本服务对**重复回调按 orderId 幂等**（互为兜底，
//     幂等重放返 200 不重复发放，绝不 500 触发无意义重投）。
//
// 环境变量：
//
//	MOCK_GAME_ADDR    监听地址（缺省 :9020）
//	MOCK_GAME_SECRET  发货验签密钥（与 payments 发货接口配置同值；
//	                  **空 = fail-closed 恒 403**，启动期大声告警）
//
// 附加面（仅沙箱走查用，非契约）：GET /granted 回已发放订单列表。
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
)

const maxBodyBytes = 1 << 20

// granted 幂等账本：已发放的订单号（生产游戏服应落库而非内存）。
var granted struct {
	mu     sync.Mutex
	orders map[string]bool
}

func init() {
	granted.orders = make(map[string]bool)
}

func secret() string {
	return os.Getenv("MOCK_GAME_SECRET")
}

// verifySignature 验签先于解析：缺头/非 hex/不匹配一律 false。
func verifySignature(raw []byte, sig string, key string) bool {
	if sig == "" || key == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(raw)
	want, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	return hmac.Equal(want, mac.Sum(nil))
}

func handleDeliver(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// 验签先于解析（契约红线）：失败恒 403，不回读 body 内容。
	key := secret()
	if key == "" {
		log.Println("deliver rejected: MOCK_GAME_SECRET not configured (fail-closed)")
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if !verifySignature(raw, r.Header.Get("X-Game-Delivery-Signature"), key) {
		log.Printf("deliver rejected: bad signature (order payload %d bytes)", len(raw))
		w.WriteHeader(http.StatusForbidden)
		return
	}
	var body struct {
		OrderId   string `json:"orderId"`
		AccountId string `json:"accountId"`
		ServerId  int64  `json:"serverId"`
		RoleId    string `json:"roleId"`
		SkuId     string `json:"skuId"`
	}
	if err := json.Unmarshal(raw, &body); err != nil || body.OrderId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	granted.mu.Lock()
	duplicate := granted.orders[body.OrderId]
	granted.orders[body.OrderId] = true
	granted.mu.Unlock()
	if duplicate {
		// 幂等重放：已发放不重复发放，仍 200（防止触发 payments 无意义重投）。
		log.Printf("order %s replay: already granted, idempotent 200", body.OrderId)
	} else {
		log.Printf("order %s granted (account=%s server=%d role=%s sku=%s)",
			body.OrderId, body.AccountId, body.ServerId, body.RoleId, body.SkuId)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": true, "orderId": body.OrderId, "duplicate": duplicate,
	})
}

func handleGranted(w http.ResponseWriter, r *http.Request) {
	granted.mu.Lock()
	defer granted.mu.Unlock()
	ids := make([]string, 0, len(granted.orders))
	for id := range granted.orders {
		ids = append(ids, id)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"count": len(ids), "orderIds": ids})
}

func main() {
	addr := os.Getenv("MOCK_GAME_ADDR")
	if addr == "" {
		addr = ":9020"
	}
	if secret() == "" {
		log.Println("WARN: MOCK_GAME_SECRET empty — every delivery will be rejected 403 (fail-closed). " +
			"Set it to the same value as the secret configured in payments PUT /admin/games/:gameId/delivery.")
	}
	http.HandleFunc("/deliver", handleDeliver)
	http.HandleFunc("/granted", handleGranted)
	log.Printf("mock game server listening on %s (POST /deliver, GET /granted)", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
