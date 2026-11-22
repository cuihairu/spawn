package notify

// GP-306/401 业务通知定时重投（Relay）：扫描 notify_outbox 中到期的 pending
// 行重投 community。outbox 持久化在 SQLite/MySQL，进程重启后新实例继续扫
// （重启恢复）；重投安全性由 community 侧 UNIQUE(user_id, actor_id, type,
// target_id) 去重保证（已投用户 IGNORE，只补投缺失用户，不重复站内信）。
// 指数退避（封顶 6 分钟），重试耗尽置 dropped 留痕；已终态行按保留窗口清理。

import (
	"context"
	"time"

	"github.com/tappi/tappi/services/game-catalog/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// Relay outbox 重投循环。
type Relay struct {
	outbox      *model.NotifyOutboxModel
	client      *publisher // 具体客户端（post 低层投递）；noop 时 NewRelay 返回 nil
	maxAttempts int
	retention   time.Duration
	interval    time.Duration
}

// NewRelay 构造重投循环；outbox 空或客户端非启用态（noop）返回 nil（调用方
// 不启动扫描）。interval 为扫描周期。
func NewRelay(outbox *model.NotifyOutboxModel, c Publisher, interval time.Duration, maxAttempts int, retention time.Duration) *Relay {
	cc, ok := c.(*publisher)
	if !ok || outbox == nil {
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
	return &Relay{outbox: outbox, client: cc, maxAttempts: maxAttempts, retention: retention, interval: interval}
}

// RelayOnce 扫一轮到期 pending 行并逐条重投；返回 (尝试数, 成功数)。
func (r *Relay) RelayOnce(ctx context.Context) (attempted, delivered int) {
	if r == nil || r.outbox == nil {
		return 0, 0
	}
	rows, err := r.outbox.ListRetry(nowUTC(), 100)
	if err != nil {
		logx.Errorf("notify relay scan failed: %v", err)
		return 0, 0
	}
	for _, row := range rows {
		attempted++
		if err := r.client.post([]byte(row.Payload)); err != nil {
			logx.Errorf("notify relay event=%s type=%s: %v", row.NotifyKey, row.NotifyType, err)
			attempts := row.Attempts + 1
			if attempts >= r.maxAttempts {
				if err2 := r.outbox.MarkDropped(row.NotifyKey, attempts, truncate(err.Error(), 500)); err2 != nil {
					logx.Errorf("notify relay event=%s mark dropped: %v", row.NotifyKey, err2)
				}
			} else {
				mult := attempts
				if mult > 6 {
					mult = 6
				}
				next := time.Now().UTC().Add(time.Minute * time.Duration(mult)).Format(time.RFC3339Nano)
				if err2 := r.outbox.MarkFailed(row.NotifyKey, attempts, truncate(err.Error(), 500), next); err2 != nil {
					logx.Errorf("notify relay event=%s mark failed: %v", row.NotifyKey, err2)
				}
			}
			continue
		}
		delivered++
		if err := r.outbox.MarkDelivered(row.NotifyKey, nowUTC()); err != nil {
			logx.Errorf("notify relay event=%s mark delivered: %v", row.NotifyKey, err)
		}
	}
	return attempted, delivered
}

// Run 周期重投循环，ctx 取消即退出（进程信号面）；每轮顺带清理已终态行。
func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if attempted, delivered := r.RelayOnce(ctx); attempted > 0 {
				logx.Infof("notify relay: attempted=%d delivered=%d", attempted, delivered)
			}
			cutoff := time.Now().UTC().Add(-r.retention).Format(time.RFC3339)
			if n, err := r.outbox.Prune(cutoff); err != nil {
				logx.Errorf("notify relay prune: %v", err)
			} else if n > 0 {
				logx.Infof("notify relay pruned %d rows", n)
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
