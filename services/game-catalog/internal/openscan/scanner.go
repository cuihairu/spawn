// Package openscan GP-306 开服排期到点自动开服。
//
// 合作方 PATCH 手动置 open 之外，排期（planned_open_at）到点的区服由本扫描
// 自动置 open 并触发预约人群通知——排期不再只是展示字段。判定按**时刻**
// 解析 RFC3339（planned_open_at 带时区偏移，字符串比较不可靠），置 open
// 走条件 UPDATE（状态仍为 closed 且排期未被改动才生效），故与手动 PATCH
// 并发时以先到者为准、不会双开或双发提醒（notified_at 幂等）。
package openscan

import (
	"context"
	"time"

	"github.com/tappi/tappi/services/game-catalog/internal/logic"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// Runner 到点扫描循环。
type Runner struct {
	svcCtx   *svc.ServiceContext
	interval time.Duration
}

// NewRunner 构造扫描循环；interval <= 0 取 60 秒。
func NewRunner(svcCtx *svc.ServiceContext, interval time.Duration) *Runner {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &Runner{svcCtx: svcCtx, interval: interval}
}

// OpenOnce 扫一轮已排期未开服的区服，将到点者置 open 并触发预约提醒；
// 返回本轮开服的区服数。单轮错误仅记日志（下一轮重试，不退出循环）。
func (r *Runner) OpenOnce(ctx context.Context) int {
	l := logx.WithContext(ctx)
	servers, err := r.svcCtx.AdminRepository.ListScheduledServers(200)
	if err != nil {
		l.Errorf("list scheduled servers failed: %v", err)
		return 0
	}
	now := time.Now()
	opened := 0
	for _, s := range servers {
		due, err := isDue(s.PlannedOpenAt, now)
		if err != nil {
			// 排期值非法（合作方写入畸形时间）：不自动开服，留待人工处理
			l.Errorf("parse planned_open_at server=%d value=%q failed: %v", s.Id, s.PlannedOpenAt, err)
			continue
		}
		if !due {
			continue
		}
		ok, err := r.svcCtx.AdminRepository.OpenServerIfDue(s.Id, s.Status, s.PlannedOpenAt)
		if err != nil {
			l.Errorf("open server %d failed: %v", s.Id, err)
			continue
		}
		if !ok {
			continue // 已被手动开服/改排期/下线：跳过
		}
		refreshed, err := r.svcCtx.AdminRepository.GetGameServer(s.Id)
		if err != nil {
			l.Errorf("reload server %d failed: %v", s.Id, err)
			continue
		}
		refreshed.Status = model.ServerStatusOpen
		// 与手动 PATCH 同一提醒路径（notified_at 保证只发一次）
		logic.NotifyServerOpen(l, r.svcCtx, refreshed)
		logic.AuditServerAutoOpen(l, r.svcCtx, s.Id)
		opened++
	}
	if opened > 0 {
		l.Infof("openscan: opened=%d due servers", opened)
	}
	return opened
}

// Run 周期扫描，ctx 取消即退出（进程信号面，与发货恢复同款）。
func (r *Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.OpenOnce(ctx)
		}
	}
}

// isDue 判定排期是否已到点（RFC3339 解析，跨时区偏移按时刻比较）。
func isDue(plannedOpenAt string, now time.Time) (bool, error) {
	if plannedOpenAt == "" {
		return false, nil
	}
	at, err := time.Parse(time.RFC3339, plannedOpenAt)
	if err != nil {
		return false, err
	}
	return !at.After(now), nil
}
