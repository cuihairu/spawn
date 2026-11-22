package couponrec

// GP-305 超时释放扫描：下单锁券（locked）后若订单久未 PAID（玩家放弃支付、
// 进程重启丢上下文等），券会永久卡死在 locked。本扫描把 locked_at 早于 TTL
// 的锁券条件 UPDATE 回 claimed（locked_order_id/locked_at 清空）；已 PAID
// 核销的券为 used 态不被命中（条件 status=locked），故只释放孤儿锁。
// 对齐 delivery.Recovery 的 ticker 循环；释放为幂等条件 UPDATE，重启恢复
// 不重复释放。

import (
	"context"
	"time"

	"github.com/tappi/tappi/services/payments/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// Release 超时释放循环。
type Release struct {
	db       *model.CouponModel
	interval time.Duration
	ttl      time.Duration
}

// NewRelease 构造释放循环；ttl/interval <= 0 取默认（1h / 5min）。
func NewRelease(db *model.CouponModel, ttl, interval time.Duration) *Release {
	if ttl <= 0 {
		ttl = time.Hour
	}
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &Release{db: db, interval: interval, ttl: ttl}
}

// ReleaseOnce 扫一轮超时锁券释放；olderThan = now - TTL。返回释放数量与其
// 订单号（审计/对账日志用）。
func (r *Release) ReleaseOnce(ctx context.Context, now time.Time) (released int, orderIDs []string) {
	olderThan := now.Add(-r.ttl).Format(time.RFC3339)
	released, orderIDs, err := r.db.ReleaseExpiredLocked(olderThan)
	if err != nil {
		logx.Errorf("coupon release scan failed: %v", err)
		return 0, nil
	}
	return released, orderIDs
}

// Run 周期释放循环，ctx 取消即退出（进程信号面）。
func (r *Release) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if released, orders := r.ReleaseOnce(ctx, time.Now().UTC()); released > 0 {
				logx.Infof("coupon release: released=%d orders=%v", released, orders)
			}
		}
	}
}
