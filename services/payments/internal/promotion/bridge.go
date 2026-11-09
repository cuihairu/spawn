package promotion

// GP-302 活动奖励评估桥：订单 PAID 后把订单事实（含服务端商品固化的
// 基础购买量）推送 promotions 评估倍率赠送。队列表 promotion_evaluations
// 持久化 + 指数退避重投（与发货恢复同款，进程重启续扫）；promotions 侧
// 按 (order_id, activity_id) 唯一幂等去重，at-least-once 安全。
//
// 签名：X-Reward-Signature = hex(HMAC-SHA256(secret, rawBody))，与
// promotions 侧验签（先于解析、未配置密钥 fail-closed）互为镜像。

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tappi/tappi/services/payments/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// Facts 推送给 promotions 的订单事实（基础量来自服务端商品配置，非客户端）。
type Facts struct {
	OrderId      string `json:"order_id"`
	GameId       string `json:"game_id"`
	ServerId     int64  `json:"server_id"`
	RoleId       string `json:"role_id"`
	AccountId    string `json:"account_id"`
	SkuId        string `json:"sku_id"`
	ProductId    string `json:"product_id"`
	Channel      string `json:"channel"`
	PaidCents    int64  `json:"paid_cents"`
	Currency     string `json:"currency"`
	PaidAt       string `json:"paid_at"`
	BaseQuantity int64  `json:"base_quantity"`
}

// Bridge 评估队列分发桥。
type Bridge struct {
	db      *model.PromotionEvalModel
	baseURL string
	secret  string
	hc      *http.Client
}

// NewBridge 构造；baseURL 为空由调用方决定不启动扫描（部署面未带促销
// 服务时订单面行为不变）。
func NewBridge(db *model.PromotionEvalModel, baseURL, secret string, timeoutMs int64) *Bridge {
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	return &Bridge{
		db:      db,
		baseURL: baseURL,
		secret:  secret,
		hc:      &http.Client{Timeout: time.Millisecond * time.Duration(timeoutMs)},
	}
}

// Enabled 是否已配置促销上游（决定入队与扫描）。
func (b *Bridge) Enabled() bool { return b != nil && b.baseURL != "" }

// Enqueue 订单事实入队（幂等；channel 为回调路由的渠道标识）。
func (b *Bridge) Enqueue(orderID, channel string, facts Facts) error {
	payload, err := json.Marshal(facts)
	if err != nil {
		return err
	}
	return b.db.Enqueue(orderID, channel, string(payload))
}

// DeliverOnce 推送单条队列行；成功记 done，失败按指数退避（封顶 6 倍）。
func (b *Bridge) DeliverOnce(ctx context.Context, row *model.PromotionEval) error {
	attempts := row.Attempts + 1
	if err := b.post(ctx, []byte(row.Payload)); err != nil {
		mult := attempts
		if mult > 6 {
			mult = 6
		}
		next := time.Now().UTC().Add(time.Minute * time.Duration(mult)).Format(time.RFC3339Nano)
		if err2 := b.db.MarkFailed(row.OrderId, attempts, truncate(err.Error(), 500), next); err2 != nil {
			return fmt.Errorf("记评估失败: %v（原因: %w）", err2, err)
		}
		return err
	}
	return b.db.MarkDone(row.OrderId)
}

// post 签名推送（2xx 视为受理；promotions 受理 ≠ 已发放，账本事实以其
// 响应为准且其内部幂等）。
func (b *Bridge) post(ctx context.Context, payload []byte) error {
	mac := hmac.New(sha256.New, []byte(b.secret))
	mac.Write(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		b.baseURL+"/internal/reward-evaluations", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Reward-Signature", hex.EncodeToString(mac.Sum(nil)))
	resp, err := b.hc.Do(req)
	if err != nil {
		return fmt.Errorf("promotions 不可达: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("promotions 评估接口返回 %d", resp.StatusCode)
	}
	return nil
}

// Scan 扫描循环（与发货恢复同款）：到期 pending/failed 行重投，ctx 取消
// 即退出。
func (b *Bridge) Scan(ctx context.Context, interval time.Duration) {
	if !b.Enabled() {
		return
	}
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rows, err := b.db.ListRetry(time.Now().UTC().Format(time.RFC3339), 100)
			if err != nil {
				logx.Errorf("promotion eval scan failed: %v", err)
				continue
			}
			for _, row := range rows {
				if err := b.DeliverOnce(ctx, row); err != nil {
					logx.Errorf("promotion eval order=%s failed: %v", row.OrderId, err)
				}
			}
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
