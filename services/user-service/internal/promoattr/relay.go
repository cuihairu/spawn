// Package promoattr GP-403 渠道归因 outbox 定时重投（Relay）：扫描 outbox 中
// 到期的 pending 行向 game-catalog 重投归因解析（/internal/promo/clicks/:id/
// resolve）。outbox 持久化在 SQLite/MySQL，进程重启后新实例继续扫（重启恢复），
// 不依赖内存；重投安全性由 game-catalog 侧判定锚定原始 registered_at、回写
// user_id 幂等（一点击一归因）保证。指数退避（封顶 6 分钟），重试耗尽置
// dropped 留痕；已终态行按保留窗口清理。
package promoattr

import (
	"context"
	"time"

	"github.com/tappi/tappi/services/user-service/internal/integration"
	"github.com/tappi/tappi/services/user-service/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// Relay 归因 outbox 重投循环。
type Relay struct {
	outbox      *model.PromoAttrOutboxModel
	client      *integration.GameCatalogClient
	maxAttempts int
	retention   time.Duration
	interval    time.Duration
}

// NewRelay 构造重投循环；outbox 空或客户端 nil 返回 nil（调用方不启动扫描）。
// interval 为扫描周期。
func NewRelay(outbox *model.PromoAttrOutboxModel, client *integration.GameCatalogClient, interval time.Duration, maxAttempts int, retention time.Duration) *Relay {
	if outbox == nil || client == nil {
		return nil
	}
	if interval <= 0 {
		interval = time.Minute
	}
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	if retention <= 0 {
		retention = 7 * 24 * time.Hour
	}
	return &Relay{outbox: outbox, client: client, maxAttempts: maxAttempts, retention: retention, interval: interval}
}

// RelayOnce 扫一轮到期 pending 行并逐条重投；返回 (尝试数, 成功数)。
func (r *Relay) RelayOnce(ctx context.Context) (attempted, delivered int) {
	if r == nil || r.outbox == nil {
		return 0, 0
	}
	rows, err := r.outbox.ListRetry(nowUTC(), 100)
	if err != nil {
		logx.Errorf("promo attr relay scan failed: %v", err)
		return 0, 0
	}
	for _, row := range rows {
		attempted++
		// 重投锚定原始 registered_at（窗口判定确定），回写 user_id 幂等。
		if _, err := r.client.ResolvePromoClick(ctx, row.ClickId, row.RegisteredAt, row.UserId); err != nil {
			logx.Errorf("promo attr relay click=%s: %v", row.ClickId, err)
			attempts := row.Attempts + 1
			if attempts >= r.maxAttempts {
				if err2 := r.outbox.MarkDropped(row.AttrKey, attempts, truncate(err.Error(), 500)); err2 != nil {
					logx.Errorf("promo attr relay click=%s mark dropped: %v", row.ClickId, err2)
				}
			} else {
				mult := attempts
				if mult > 6 {
					mult = 6
				}
				next := time.Now().UTC().Add(time.Minute * time.Duration(mult)).Format(time.RFC3339Nano)
				if err2 := r.outbox.MarkFailed(row.AttrKey, attempts, truncate(err.Error(), 500), next); err2 != nil {
					logx.Errorf("promo attr relay click=%s mark failed: %v", row.ClickId, err2)
				}
			}
			continue
		}
		delivered++
		if err := r.outbox.MarkDelivered(row.AttrKey, nowUTC()); err != nil {
			logx.Errorf("promo attr relay click=%s mark delivered: %v", row.ClickId, err)
		}
	}
	return attempted, delivered
}

// Run 周期重投循环，ctx 取消即退出（进程信号面）；每轮顺带清理已终态行。
func (r *Relay) Run(ctx context.Context) {
	if r == nil {
		return
	}
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if attempted, delivered := r.RelayOnce(ctx); attempted > 0 {
				logx.Infof("promo attr relay: attempted=%d delivered=%d", attempted, delivered)
			}
			cutoff := time.Now().UTC().Add(-r.retention).Format(time.RFC3339)
			if n, err := r.outbox.Prune(cutoff); err != nil {
				logx.Errorf("promo attr relay prune: %v", err)
			} else if n > 0 {
				logx.Infof("promo attr relay pruned %d rows", n)
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

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
