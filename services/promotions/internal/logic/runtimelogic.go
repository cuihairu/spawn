package logic

// GP-301 活动快照运行时消费接口（玩家侧/游戏服侧读取此前未建——配置域
// 只进不出，运营发布后无读取面可消费）。运行时一律按**已发布版本快照**
// 读取（current_version ⋈ activity_versions，不读可变草稿行，改草稿不影
// 响运行中版本）；窗口按时刻判定（RFC3339 解析成时刻比较，跨时区偏移安
// 全——字符串比较在 "+08:00" 与 "Z" 混存时不可靠；解析失败或倒挂的快照
// 跳过不展示，fail-closed）；人群定向（audience.type 非 all）不出现在无
// 玩家上下文的列表里（定向触达走奖励评估通道，不靠展示面泄露）。

import (
	"time"

	"github.com/tappi/tappi/services/promotions/internal/svc"
	"github.com/tappi/tappi/services/promotions/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListRuntimeActivities 玩家侧活动列表：该游戏已发布、当前窗口内、人群
// 为全体（audience.type 空/all）的活动展示摘要。预算（budget_cents）为
// 财务口径，不下发玩家侧。
func ListRuntimeActivities(svcCtx *svc.ServiceContext, l logx.Logger, gameID string, now time.Time) []types.RuntimeActivity {
	published, err := svcCtx.ActivityDB.ListPublishedSnapshots(gameID)
	if err != nil {
		l.Errorf("list published snapshots game=%s failed: %v", gameID, err)
		return []types.RuntimeActivity{}
	}
	items := make([]types.RuntimeActivity, 0, len(published))
	for _, p := range published {
		cfg, err := parseSnapshot(p.Snapshot)
		if err != nil {
			l.Errorf("parse snapshot activity=%d: %v", p.ActivityId, err)
			continue
		}
		if !runtimeWindowCovers(cfg, now) {
			continue
		}
		aud := snapshotMap(cfg, "audience")
		if t, _ := aud["type"].(string); t != "" && t != "all" {
			continue
		}
		limits := snapshotMap(cfg, "limits")
		delete(limits, "budget_cents")
		items = append(items, types.RuntimeActivity{
			Id:         p.ActivityId,
			Version:    p.Version,
			Name:       snapshotString(cfg, "name"),
			StartAt:    snapshotString(cfg, "start_at"),
			EndAt:      snapshotString(cfg, "end_at"),
			Stackable:  snapshotBool(cfg, "stackable"),
			StackGroup: snapshotString(cfg, "stack_group"),
			Rewards:    snapshotMap(cfg, "rewards"),
			Limits:     limits,
		})
	}
	return items
}

// ListRuntimeSnapshots 游戏服侧活动快照：同窗口判定，返回快照原文（含
// 人群/触发明细的完整配置，供游戏服按自身玩家上下文消费）。
func ListRuntimeSnapshots(svcCtx *svc.ServiceContext, l logx.Logger, gameID string, now time.Time) []types.RuntimeSnapshot {
	published, err := svcCtx.ActivityDB.ListPublishedSnapshots(gameID)
	if err != nil {
		l.Errorf("list published snapshots game=%s failed: %v", gameID, err)
		return []types.RuntimeSnapshot{}
	}
	items := make([]types.RuntimeSnapshot, 0, len(published))
	for _, p := range published {
		cfg, err := parseSnapshot(p.Snapshot)
		if err != nil {
			l.Errorf("parse snapshot activity=%d: %v", p.ActivityId, err)
			continue
		}
		if !runtimeWindowCovers(cfg, now) {
			continue
		}
		items = append(items, types.RuntimeSnapshot{
			ActivityId: p.ActivityId,
			Version:    p.Version,
			Snapshot:   []byte(p.Snapshot),
		})
	}
	return items
}

// runtimeWindowCovers 窗口按时刻判定：start ≤ now ≤ end（含头含尾，与
// 评估通道 periodCovers 同口径，但比较在时刻域做，跨时区偏移安全）。
func runtimeWindowCovers(cfg map[string]any, now time.Time) bool {
	start, err := time.Parse(time.RFC3339, snapshotString(cfg, "start_at"))
	if err != nil {
		return false
	}
	end, err := time.Parse(time.RFC3339, snapshotString(cfg, "end_at"))
	if err != nil {
		return false
	}
	return !start.After(now) && !end.Before(now)
}
