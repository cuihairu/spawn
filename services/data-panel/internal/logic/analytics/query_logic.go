package analytics

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tappi/tappi/services/data-panel/internal/httperr"
	"github.com/tappi/tappi/services/data-panel/internal/model"
	"github.com/tappi/tappi/services/data-panel/internal/svc"
	"github.com/tappi/tappi/services/data-panel/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// OverviewLogic 期间按事件类型聚合（漏斗/总览共用底座）。
type OverviewLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewOverviewLogic(ctx context.Context, svcCtx *svc.ServiceContext) *OverviewLogic {
	return &OverviewLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *OverviewLogic) Overview(req *types.AnalyticsOverviewReq) (*types.AnalyticsOverviewResp, error) {
	if err := requireAnalyticsAdmin(l.svcCtx, l.ctx); err != nil {
		return nil, err
	}
	from, to, err := normalizeBounds(req.From, req.To)
	if err != nil {
		return nil, err
	}
	var typeList []string
	if s := strings.TrimSpace(req.Types); s != "" {
		for _, t := range strings.Split(s, ",") {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			if !model.ValidEventType(t) {
				return nil, httperr.BadRequest("unknown event_type: " + t)
			}
			typeList = append(typeList, t)
		}
	}
	rows, err := l.svcCtx.AnalyticsSvc.Overview(typeList, from, to, req.GameId)
	if err != nil {
		return nil, err
	}
	items := make([]types.AnalyticsTypeCount, 0, len(rows))
	for _, r := range rows {
		items = append(items, types.AnalyticsTypeCount{
			EventType: r.EventType, Events: r.Events, Users: r.Users, AmountCents: r.Amount,
		})
	}
	return &types.AnalyticsOverviewResp{From: from, To: to, GameId: req.GameId, Items: items}, nil
}

// FunnelLogic 固定漏斗：注册→活跃→下载→付费（按去重人数算转化率）。
type FunnelLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewFunnelLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FunnelLogic {
	return &FunnelLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

var funnelStages = []string{model.EventRegister, model.EventLogin, model.EventDownload, model.EventOrderPaid}

func (l *FunnelLogic) Funnel(req *types.AnalyticsFunnelReq) (*types.AnalyticsFunnelResp, error) {
	if err := requireAnalyticsAdmin(l.svcCtx, l.ctx); err != nil {
		return nil, err
	}
	from, to, err := normalizeBounds(req.From, req.To)
	if err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.AnalyticsSvc.Overview(funnelStages, from, to, req.GameId)
	if err != nil {
		return nil, err
	}
	byType := make(map[string]model.TypeCounts, len(rows))
	for _, r := range rows {
		byType[r.EventType] = r
	}
	stages := make([]types.AnalyticsFunnelStage, 0, len(funnelStages))
	var base int64
	for i, st := range funnelStages {
		row := byType[st]
		overall, step := 0.0, 0.0
		if base > 0 {
			overall = float64(row.Users) / float64(base)
		}
		if i == 0 {
			if row.Users > 0 || row.Events > 0 {
				step = 1
			}
			base = row.Users
		} else if prev := stages[i-1].Users; prev > 0 {
			step = float64(row.Users) / float64(prev)
		}
		stages = append(stages, types.AnalyticsFunnelStage{
			Stage: st, Users: row.Users, Events: row.Events,
			OverallConv: round4(overall), StepConv: round4(step),
		})
	}
	return &types.AnalyticsFunnelResp{From: from, To: to, GameId: req.GameId, Stages: stages}, nil
}

// RetentionLogic 留存：拉 (日期, user) 去重对，date 算术与分桶在 Go 侧
// 完成（SQL 保持方言中立）。口径：cohort = 窗口内首次活跃日；第 N 日留存
// = 该 cohort 在首活跃日+N 仍活跃人数 / cohort 人数。
type RetentionLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRetentionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RetentionLogic {
	return &RetentionLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

const (
	retentionDefaultWindow = 7
	retentionMaxWindow     = 30
)

func (l *RetentionLogic) Retention(req *types.AnalyticsRetentionReq) (*types.AnalyticsRetentionResp, error) {
	if err := requireAnalyticsAdmin(l.svcCtx, l.ctx); err != nil {
		return nil, err
	}
	eventType := req.EventType
	if eventType == "" {
		eventType = model.EventLogin
	}
	if !model.ValidEventType(eventType) {
		return nil, httperr.BadRequest("unknown event_type: " + eventType)
	}
	window := req.Window
	if window == 0 {
		window = retentionDefaultWindow
	}
	if window < 0 || window > retentionMaxWindow {
		return nil, httperr.BadRequest("window must be in [0,30]")
	}
	from, to, err := normalizeBounds(req.From, req.To)
	if err != nil {
		return nil, err
	}

	pairs, err := l.svcCtx.AnalyticsSvc.DailyActiveUsers(eventType, from, to)
	if err != nil {
		return nil, err
	}

	// user → 活跃日集合
	type userDays map[string]bool
	actives := make(map[int64]userDays)
	for _, p := range pairs {
		set, ok := actives[p.UserId]
		if !ok {
			set = userDays{}
			actives[p.UserId] = set
		}
		set[p.Day] = true
	}

	// cohort 聚合：首活跃日 → size + 各偏移活跃人数
	type cohortAgg struct {
		size    int64
		actives map[int]int64
	}
	cohorts := make(map[string]*cohortAgg)
	for _, days := range actives {
		daysList := make([]string, 0, len(days))
		for d := range days {
			daysList = append(daysList, d)
		}
		sort.Strings(daysList)
		first := daysList[0]
		agg, ok := cohorts[first]
		if !ok {
			agg = &cohortAgg{actives: map[int]int64{}}
			cohorts[first] = agg
		}
		agg.size++
		firstT, _ := time.Parse("2006-01-02", first)
		for _, d := range daysList {
			dT, _ := time.Parse("2006-01-02", d)
			offset := int(dT.Sub(firstT).Hours() / 24)
			if offset < 0 || offset > int(window) {
				continue
			}
			agg.actives[offset]++
		}
	}

	keys := make([]string, 0, len(cohorts))
	for k := range cohorts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]types.AnalyticsCohortRow, 0, len(keys))
	for _, day := range keys {
		agg := cohorts[day]
		ret := make(map[string]types.AnalyticsRetentionCell, window+1)
		for off := 0; off <= int(window); off++ {
			users := agg.actives[off]
			r := 0.0
			if agg.size > 0 {
				r = float64(users) / float64(agg.size)
			}
			ret[strconv.Itoa(off)] = types.AnalyticsRetentionCell{Retention: round4(r), Users: users}
		}
		out = append(out, types.AnalyticsCohortRow{
			Day: day, Size: agg.size, Retention: ret,
		})
	}
	return &types.AnalyticsRetentionResp{
		EventType: eventType, From: from, To: to, Window: window, Cohorts: out,
	}, nil
}

// ActivityEffectLogic 活动效果：发奖事件事实聚合。
type ActivityEffectLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewActivityEffectLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ActivityEffectLogic {
	return &ActivityEffectLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ActivityEffectLogic) Effect(req *types.AnalyticsActivityReq) (*types.AnalyticsActivityResp, error) {
	if err := requireAnalyticsAdmin(l.svcCtx, l.ctx); err != nil {
		return nil, err
	}
	if req.Id <= 0 {
		return nil, httperr.BadRequest("activity id is required")
	}
	row, err := l.svcCtx.AnalyticsSvc.ActivityEffect(req.Id)
	if err != nil {
		return nil, err
	}
	return &types.AnalyticsActivityResp{Activity: types.AnalyticsActivityEffect{
		ActivityId: row.ActivityId, Grants: row.Grants,
		Participants: row.Participants, RewardCents: row.RewardCents,
	}}, nil
}

// ReconciliationLogic 账单核对（F24）：按游戏聚合付费/退款事件笔数与
// 金额，与 payments 对账单核对（两端同源，事件侧为转发副本）。
type ReconciliationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewReconciliationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReconciliationLogic {
	return &ReconciliationLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ReconciliationLogic) Reconcile(req *types.AnalyticsReconciliationReq) (*types.AnalyticsReconciliationResp, error) {
	if err := requireAnalyticsAdmin(l.svcCtx, l.ctx); err != nil {
		return nil, err
	}
	from, to, err := normalizeBounds(req.From, req.To)
	if err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.AnalyticsSvc.Reconciliation(from, to)
	if err != nil {
		return nil, err
	}
	items := make([]types.AnalyticsReconciliationRow, 0, len(rows))
	for _, r := range rows {
		items = append(items, types.AnalyticsReconciliationRow(r))
	}
	return &types.AnalyticsReconciliationResp{From: from, To: to, Items: items}, nil
}

func round4(v float64) float64 {
	return float64(int64(v*10000+0.5)) / 10000
}

// ChannelEffectLogic 渠道效果（F22/GP-403）：按渠道标识聚合注册人数与
// 付费金额；channel 空串行即自然流量（未带渠道的注册）。
type ChannelEffectLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewChannelEffectLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ChannelEffectLogic {
	return &ChannelEffectLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ChannelEffectLogic) Query(req *types.AnalyticsChannelEffectReq) (*types.AnalyticsChannelEffectResp, error) {
	if err := requireAnalyticsAdmin(l.svcCtx, l.ctx); err != nil {
		return nil, err
	}
	from, to, err := normalizeBounds(req.From, req.To)
	if err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.AnalyticsSvc.ChannelEffect(from, to, req.GameId)
	if err != nil {
		return nil, err
	}
	items := make([]types.AnalyticsChannelEffectRow, 0, len(rows))
	for _, r := range rows {
		items = append(items, types.AnalyticsChannelEffectRow(r))
	}
	return &types.AnalyticsChannelEffectResp{From: from, To: to, Items: items}, nil
}
