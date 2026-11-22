package playeract

// Relay GP-303 活跃上报 outbox 定时重投：扫描到期的 pending 行向 promotions
// 重投（/internal/player-activity）。outbox 持久化在 SQLite/MySQL，进程重启
// 后新实例继续扫（重启恢复）。指数退避（封顶 6 分钟），重试耗尽置 dropped
// 留痕；已终态行按保留窗口清理。重投安全由 promotions 侧条件推进保证
//（仅更新更新时刻，重复上报不改变判定）。

import (
	"context"
	"time"

	"github.com/tappi/tappi/services/courier-adapter/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// Relay 活跃上报 outbox 重投循环。
type Relay struct {
	outbox      *model.ActivityOutboxModel
	client      *Client
	maxAttempts int
	retention   time.Duration
	interval    time.Duration
}

// NewRelay 构造重投循环；outbox 空或客户端 nil 返回 nil（调用方不启动扫描）。
func NewRelay(outbox *model.ActivityOutboxModel, client *Client, interval time.Duration, maxAttempts int, retention time.Duration) *Relay {
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
	rows, err := r.outbox.ListRetry(nowRFC3339(), 100)
	if err != nil {
		logx.Errorf("player activity relay scan failed: %v", err)
		return 0, 0
	}
	for _, row := range rows {
		attempted++
		if err := r.client.Report(ctx, []byte(row.Payload)); err != nil {
			logx.Errorf("player activity relay key=%s: %v", row.ActKey, err)
			attempts := row.Attempts + 1
			if attempts >= r.maxAttempts {
				if err2 := r.outbox.MarkDropped(row.ActKey, attempts, truncate(err.Error(), 500)); err2 != nil {
					logx.Errorf("player activity relay key=%s mark dropped: %v", row.ActKey, err2)
				}
			} else {
				mult := attempts
				if mult > 6 {
					mult = 6
				}
				next := time.Now().UTC().Add(time.Minute * time.Duration(mult)).Format(time.RFC3339Nano)
				if err2 := r.outbox.MarkFailed(row.ActKey, attempts, truncate(err.Error(), 500), next); err2 != nil {
					logx.Errorf("player activity relay key=%s mark failed: %v", row.ActKey, err2)
				}
			}
			continue
		}
		delivered++
		if err := r.outbox.MarkDelivered(row.ActKey, nowRFC3339()); err != nil {
			logx.Errorf("player activity relay key=%s mark delivered: %v", row.ActKey, err)
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
				logx.Infof("player activity relay: attempted=%d delivered=%d", attempted, delivered)
			}
			cutoff := time.Now().UTC().Add(-r.retention).Format(time.RFC3339)
			if n, err := r.outbox.Prune(cutoff); err != nil {
				logx.Errorf("player activity relay prune: %v", err)
			} else if n > 0 {
				logx.Infof("player activity relay pruned %d rows", n)
			}
		}
	}
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
