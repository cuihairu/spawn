// Package promoprune GP-403 渠道点击归档：周期清理过期/超量 promo_clicks。
//
// 推广点击（/promo/resolve 登记点击、/promo/download-click 计数）每条落
// promo_clicks 且长期追加——点击记录只对归因窗口内的注册有意义，过期者保留
// 仅撑大报表扫描范围。本包按时间保留窗口 + 容量上限裁剪，与 outbox 清理同款
// 节奏（进程信号 ctx，单轮错误仅记日志下一轮重试）。
package promoprune

import (
	"context"
	"time"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

// Pruner 清理循环：按保留窗口截止时间与容量上限裁剪 promo_clicks。
type Pruner struct {
	svcCtx    *svc.ServiceContext
	retention time.Duration
	maxRows   int64
	interval  time.Duration
}

// NewPruner 构造清理循环；interval <= 0 取 6 小时（清理非实时诉求）。
// retention <= 0 关闭时间清理；maxRows <= 0 关闭容量裁剪。
func NewPruner(svcCtx *svc.ServiceContext, retention time.Duration, maxRows int64, interval time.Duration) *Pruner {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	return &Pruner{svcCtx: svcCtx, retention: retention, maxRows: maxRows, interval: interval}
}

// PruneOnce 执行一轮清理，返回删除行数。cutoff = now - retention（空则仅容量
// 裁剪）。单轮错误仅记日志（下一轮重试，不退出循环）。
func (p *Pruner) PruneOnce(ctx context.Context) int64 {
	l := logx.WithContext(ctx)
	var cutoff string
	if p.retention > 0 {
		cutoff = time.Now().UTC().Add(-p.retention).Format(time.RFC3339)
	}
	deleted, err := p.svcCtx.AdminRepository.PrunePromoClicks(cutoff, p.maxRows)
	if err != nil {
		l.Errorf("promoprune failed: %v", err)
		return 0
	}
	if deleted > 0 {
		l.Infof("promoprune: deleted=%d clicks", deleted)
	}
	return deleted
}

// Run 周期清理，ctx 取消即退出（进程信号面，与 openscan 同款）。
func (p *Pruner) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.PruneOnce(ctx)
		}
	}
}
