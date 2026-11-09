package delivery

// GP-203 定时恢复：扫描发货账本中到期的 pending/failed 行重投。账本在
// SQLite/MySQL 持久化，进程重启后新实例继续扫（重启恢复），不依赖内存。
// 对齐 teller.Reconcile 口径：发货幂等，恢复重发不二次发货。

import (
	"context"
	"time"

	"github.com/tappi/tappi/services/payments/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// Recovery 断单恢复循环。
type Recovery struct {
	db       *model.DeliveryModel
	dispatch *Dispatcher
	interval time.Duration
}

// NewRecovery 构造恢复循环；interval <= 0 取 1 分钟。
func NewRecovery(db *model.DeliveryModel, dispatch *Dispatcher, interval time.Duration) *Recovery {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Recovery{db: db, dispatch: dispatch, interval: interval}
}

// RecoverOnce 扫一轮到期账本行并逐条重投；返回 (尝试数, 成功数)。
func (r *Recovery) RecoverOnce(ctx context.Context) (attempted, delivered int) {
	rows, err := r.db.ListRetry(nowUTC(), 100)
	if err != nil {
		logx.Errorf("delivery recovery scan failed: %v", err)
		return 0, 0
	}
	for _, row := range rows {
		attempted++
		err := r.dispatch.Dispatch(ctx, Instruction{
			OrderId:     row.OrderId,
			AccountId:   row.AccountId,
			SkuId:       row.SkuId,
			ProductId:   row.ProductId,
			Form:        row.Form,
			AmountCents: row.AmountCents,
			Currency:    row.Currency,
		})
		if err != nil {
			logx.Errorf("delivery recovery order=%s failed: %v", row.OrderId, err)
			continue
		}
		delivered++
	}
	return attempted, delivered
}

// Run 周期恢复循环，ctx 取消即退出（进程信号面）。
func (r *Recovery) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if attempted, delivered := r.RecoverOnce(ctx); attempted > 0 {
				logx.Infof("delivery recovery: attempted=%d delivered=%d", attempted, delivered)
			}
		}
	}
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
