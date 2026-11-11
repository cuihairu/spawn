package delivery

// GP-203 发货接缝：Courier teller 的 Deliver 是进程内 Go 钩子（冻结仓网关
// 未接线 = nil Deliver 沙箱成功），生产接线（Courier 提案 / GP-005 联合
// 沙箱）经签名 webhook 打到本分发器。指令形状对齐 teller.Delivery
// （OrderID/AccountID/SkuID/ProductID/Form/AmountCents/Currency）。
//
// 幂等（payment.md v1.1 安全边界 3）：order_id 唯一账本，已投递直接成功，
// 同订单不二次发货；游戏服侧按契约同样按订单幂等（互为兜底）。
// 失败：账本记 failed 供补单/定时恢复重投；订单在 teller 停 PAID 可对账，
// 本服务不复制订单状态机。

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tappi/tappi/services/payments/model"
)

// Instruction 发货指令（webhook 体；对齐 teller.Delivery 字段语义）。
type Instruction struct {
	OrderId     string `json:"orderId"`
	AccountId   string `json:"accountId"`
	SkuId       string `json:"skuId"`
	ProductId   string `json:"productId"`
	Form        string `json:"form"`
	AmountCents int64  `json:"amountCents"`
	Currency    string `json:"currency"`
}

// 游戏服发货请求头：X-Game-Delivery-Signature =
// hex(HMAC-SHA256(endpointSecret, rawBody))；游戏服侧验签先于解析。
const gameSignatureHeader = "X-Game-Delivery-Signature"

// locksEntry 同订单进程内串行化条目：mu 串行化同订单分发；refs 为并发持有
// 者计数（受 Dispatcher.locksMu 保护），归零即从 map 删除（P1-2：防
// sync.Map 无限增长——原实现只存不删，每订单一个 Mutex 永久驻留）。
type locksEntry struct {
	mu   sync.Mutex
	refs int
}

// Dispatcher 发货分发器：核验归属/金额 → 调游戏服发货接口 → 账本记账。
type Dispatcher struct {
	db          *model.DeliveryModel
	products    *model.ProductModel
	hc          *http.Client
	backoff     time.Duration
	onDelivered func(orderID string) // 投递成功回调（订单投影 DELIVERED；nil 安全）
	locks       sync.Map             // order id -> *locksEntry
	locksMu     sync.Mutex           // 保护 entry 生命周期（创建/refs/删除），不持业务锁
}

// NewDispatcher 构造分发器；baseBackoff 为失败重投基础间隔（恢复面按
// attempts 指数退避，封顶 6 倍）；onDelivered 为投递成功钩子（订单只读
// 投影，nil 安全）。
func NewDispatcher(db *model.DeliveryModel, products *model.ProductModel, timeoutMs int64, baseBackoff time.Duration, onDelivered func(orderID string)) *Dispatcher {
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	if baseBackoff <= 0 {
		baseBackoff = time.Minute
	}
	return &Dispatcher{
		db:          db,
		products:    products,
		hc:          &http.Client{Timeout: time.Millisecond * time.Duration(timeoutMs)},
		backoff:     baseBackoff,
		onDelivered: onDelivered,
	}
}

// acquire 取（或建）订单串行化条目并递增引用计数；调用方负责 mu.Lock 后
// 配对 release（mu.Unlock + 计数递减/按需删除）。
func (d *Dispatcher) acquire(orderID string) *locksEntry {
	d.locksMu.Lock()
	defer d.locksMu.Unlock()
	v, ok := d.locks.Load(orderID)
	if !ok {
		e := &locksEntry{}
		d.locks.Store(orderID, e)
		v = e
	}
	e := v.(*locksEntry)
	e.refs++
	return e
}

// release 递减引用计数；归零删除 map 条目（无并发持有者，删除安全）。
func (d *Dispatcher) release(orderID string, e *locksEntry) {
	d.locksMu.Lock()
	defer d.locksMu.Unlock()
	e.refs--
	if e.refs == 0 {
		d.locks.Delete(orderID)
	}
}

// Dispatch 投递一笔发货指令。成功返回 nil（账本 delivered）；失败返回错误
// （账本 failed，调用方据此向 teller Deliver 钩子返回非 nil 使订单停 PAID）。
func (d *Dispatcher) Dispatch(ctx context.Context, in Instruction) error {
	if strings.TrimSpace(in.OrderId) == "" {
		return errors.New("发货指令缺少订单号")
	}
	entry := d.acquire(in.OrderId)
	entry.mu.Lock()
	defer func() {
		entry.mu.Unlock()
		d.release(in.OrderId, entry)
	}()

	row, err := d.db.Get(in.OrderId)
	if errors.Is(err, model.ErrDeliveryOrderNotFound) {
		// 非本服务下单或账本缺失（如账本损毁）：以指令补建，充值目标待补单回填
		row = &model.DeliveryOrder{
			OrderId: in.OrderId, SkuId: in.SkuId, ProductId: in.ProductId,
			Form: in.Form, AmountCents: in.AmountCents, Currency: in.Currency,
		}
		if createErr := d.db.CreateTarget(row); createErr != nil && !errors.Is(createErr, model.ErrDeliveryOrderExists) {
			return fmt.Errorf("建发货账本失败: %w", createErr)
		}
		if row, err = d.db.Get(in.OrderId); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// 幂等：已投递订单直接成功，不二次发货
	if row.Status == model.DeliveryStatusDelivered {
		return nil
	}

	product, err := d.products.Get(in.SkuId)
	if err != nil {
		return d.fail(row, fmt.Errorf("商品不存在: %s", in.SkuId))
	}
	// 商户归属/金额核验在发货面复核（F15）：账本归属与商品配置漂移即拒绝
	if row.GameId != "" && row.GameId != product.GameId {
		return d.fail(row, fmt.Errorf("订单归属 %s 与商品归属 %s 不一致", row.GameId, product.GameId))
	}
	if in.ProductId != product.ProductId || in.AmountCents != product.AmountCents || in.Currency != product.Currency {
		return d.fail(row, fmt.Errorf("发货指令与商品配置不一致: sku=%s product=%s/%s amount=%d%s want %s/%d%s",
			in.SkuId, in.ProductId, in.Form, in.AmountCents, in.Currency,
			product.ProductId, product.AmountCents, product.Currency))
	}
	if row.ServerId <= 0 {
		return d.fail(row, errors.New("缺少充值目标（区服）"))
	}
	ep, err := d.db.GetEndpoint(product.GameId)
	if err != nil || !ep.Enabled || ep.Secret == "" {
		return d.fail(row, fmt.Errorf("游戏 %s 未配置可用的发货接口", product.GameId))
	}

	payload, err := json.Marshal(map[string]any{
		"orderId":     in.OrderId,
		"accountId":   in.AccountId,
		"serverId":    row.ServerId,
		"roleId":      row.RoleId,
		"productId":   product.ProductId,
		"skuId":       in.SkuId,
		"form":        in.Form,
		"amountCents": in.AmountCents,
		"currency":    in.Currency,
	})
	if err != nil {
		return d.fail(row, err)
	}
	mac := hmac.New(sha256.New, []byte(ep.Secret))
	mac.Write(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return d.fail(row, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(gameSignatureHeader, hex.EncodeToString(mac.Sum(nil)))
	resp, err := d.hc.Do(req)
	if err != nil {
		return d.fail(row, fmt.Errorf("游戏服不可达: %w", err))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return d.fail(row, fmt.Errorf("游戏服发货接口返回 %d: %s", resp.StatusCode, truncate(string(raw), 256)))
	}
	if err := d.db.MarkDelivered(in.OrderId); err != nil {
		return fmt.Errorf("记发货成功失败: %w", err)
	}
	if d.onDelivered != nil {
		d.onDelivered(in.OrderId)
	}
	return nil
}

// fail 记账失败并按指数退避（封顶 6 倍）定下次重投时间，原样返回错误。
func (d *Dispatcher) fail(row *model.DeliveryOrder, cause error) error {
	mult := row.Attempts + 1
	if mult > 6 {
		mult = 6
	}
	next := time.Now().UTC().Add(d.backoff * time.Duration(mult)).Format(time.RFC3339Nano)
	if err := d.db.MarkFailed(row.OrderId, truncate(cause.Error(), 500), next); err != nil {
		return fmt.Errorf("记发货失败: %v（原因: %w）", err, cause)
	}
	return cause
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
