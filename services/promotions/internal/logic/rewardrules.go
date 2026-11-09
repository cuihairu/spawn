package logic

// GP-302 倍率赠送规则语义（调研 §5：明确倍率指总量还是额外量，避免
// "十倍送"歧义）：
//   - mode=total：总倍率（含基础），rate_bp ≥ 10000；额外量 = 基础量 ×
//     (rate_bp - 10000) / 10000。如 15000 = 1.5 倍总量。
//   - mode=extra：额外倍率（纯加赠），rate_bp ≥ 0；额外量 = 基础量 ×
//     rate_bp / 10000。如 5000 = 加赠 50%。
// 基础量来自商品配置（payments 服务端商品 base_quantity，非客户端上报）；
// 赠量取整（向下取整）；赠币不是现金收入，不进分成基数（GP-205）。
//
// 叠加规则（确定性、可解释）：
//   - 任一命中活动 stackable=false：该活动独跑——同批命中里挑额外量最大者
//     （平票取 id 小者），其余全部排除；
//   - 否则同一 stack_group 内互斥：组内挑额外量最大者（平票 id 小），
//     不同组/无组可叠加求和。
// 限次：per_user_count > 0 时该活动对单用户发放条数达上限即跳过；
// global_count > 0 时该活动发放总条数达上限即跳过。budget_cents 运行时
// 扣减不在本项（GP-301 口径延续）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/promotions/model"
)

// MultiplierSpec 倍率奖励规格（rewards.type=multiplier）。
type MultiplierSpec struct {
	Mode   string `json:"mode"`    // total | extra
	RateBp int64  `json:"rate_bp"` // 基点；total 含基础（≥10000），extra 纯加赠（≥0）
}

const (
	ModeTotal = "total"
	ModeExtra = "extra"
)

// ParseMultiplierSpec 从 rewards 配置解析倍率规格；非 multiplier 类型返回
// nil（无倍率奖励）。不合法返回 API 错误（草稿校验与运行时共用同一口径）。
func ParseMultiplierSpec(rewards map[string]any) (*MultiplierSpec, error) {
	if len(rewards) == 0 {
		return nil, nil
	}
	if t, _ := rewards["type"].(string); t != "multiplier" {
		return nil, nil
	}
	mode, _ := rewards["mode"].(string)
	if mode != ModeTotal && mode != ModeExtra {
		return nil, newAPIError(http.StatusBadRequest, "倍率奖励 mode 必须为 total（总量）或 extra（额外）")
	}
	rateBp, ok := toInt64(rewards["rate_bp"])
	if !ok {
		return nil, newAPIError(http.StatusBadRequest, "倍率奖励 rate_bp 必须为整数基点")
	}
	if mode == ModeTotal && rateBp < 10000 {
		return nil, newAPIError(http.StatusBadRequest, "总倍率 rate_bp 必须 ≥ 10000（含基础）")
	}
	if mode == ModeExtra && rateBp < 0 {
		return nil, newAPIError(http.StatusBadRequest, "额外倍率 rate_bp 不能为负")
	}
	return &MultiplierSpec{Mode: mode, RateBp: rateBp}, nil
}

// toInt64 宽松转整数（JSON 解码后数值为 float64）。
func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		if n != float64(int64(n)) {
			return 0, false
		}
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	case int64:
		return n, true
	case int:
		return int64(n), true
	}
	return 0, false
}

// GP-303 首充/累充/回流规则（F19；不塞入基础商品倍率——与 multiplier 并列
// 的独立规则类型，可各自成活动或叠加）：
//   - first_purchase 首充：订单事实 first_purchase=true（payments 按投影判定：
//     同账号同游戏无更早 PAID/DELIVERED 订单）时发放一次；迟到事件双首充
//     由 promotions 账本兜底（该账号该活动 first_purchase 已发放即跳过）。
//   - cumulative_recharge 累充：累充口径=同账号同游戏 PAID/DELIVERED 订单
//     实付合计 − EXECUTED 退款合计（净额，payments 推送时计算）；thresholds
//     升序档位 [{cents, extra_quantity}]，累计净额首次达到档位即发放（幂等键
//     含档位序号）；退款后净额跌破已发档位 → 该档位发放置 revoked（评估
//     时重算，不追回已撤销前的消耗）。
//   - 回流：人群定向机制（audience.type + 评估请求 audience_type）GP-302
//     已就位，回流人群标签判定源（登录行为数据）未建，见 todo。

// FirstPurchaseSpec 首充奖励规格。
type FirstPurchaseSpec struct {
	ExtraQuantity int64 `json:"extra_quantity"` // 首充加赠数量
}

// ParseFirstPurchaseSpec 解析 first_purchase 规格。
func ParseFirstPurchaseSpec(rewards map[string]any) (*FirstPurchaseSpec, error) {
	if t, _ := rewards["type"].(string); t != model.RuleTypeFirstPurchase {
		return nil, nil
	}
	q, ok := toInt64(rewards["extra_quantity"])
	if !ok || q <= 0 {
		return nil, newAPIError(http.StatusBadRequest, "首充奖励 extra_quantity 必须为正整数")
	}
	return &FirstPurchaseSpec{ExtraQuantity: q}, nil
}

// CumulativeThreshold 累充档位。
type CumulativeThreshold struct {
	Cents         int64 `json:"cents"`          // 累计净额门槛（最小货币单位）
	ExtraQuantity int64 `json:"extra_quantity"` // 达档加赠数量
}

// CumulativeSpec 累充奖励规格（档位升序）。
type CumulativeSpec struct {
	Thresholds []CumulativeThreshold `json:"thresholds"`
}

// ParseCumulativeSpec 解析 cumulative_recharge 规格（档位必须升序且为正）。
func ParseCumulativeSpec(rewards map[string]any) (*CumulativeSpec, error) {
	if t, _ := rewards["type"].(string); t != model.RuleTypeCumulative {
		return nil, nil
	}
	raw, _ := rewards["thresholds"].([]any)
	if len(raw) == 0 {
		return nil, newAPIError(http.StatusBadRequest, "累充奖励 thresholds 不能为空")
	}
	var prev int64 = -1
	spec := &CumulativeSpec{Thresholds: make([]CumulativeThreshold, 0, len(raw))}
	for i, item := range raw {
		m, _ := item.(map[string]any)
		cents, ok1 := toInt64(m["cents"])
		qty, ok2 := toInt64(m["extra_quantity"])
		if !ok1 || !ok2 || cents <= 0 || qty <= 0 {
			return nil, newAPIError(http.StatusBadRequest, "累充档位 cents/extra_quantity 必须为正整数")
		}
		if cents <= prev {
			return nil, newAPIError(http.StatusBadRequest, "累充档位必须按 cents 升序且不重复")
		}
		prev = cents
		spec.Thresholds = append(spec.Thresholds, CumulativeThreshold{Cents: cents, ExtraQuantity: qty})
		_ = i
	}
	return spec, nil
}

// triggerSpec 触发条件（type=order + min_amount_cents）。
type triggerSpec struct {
	Type           string `json:"type"`
	MinAmountCents int64  `json:"min_amount_cents"`
}

func parseTrigger(trigger map[string]any) (triggerSpec, error) {
	spec := triggerSpec{Type: "order"}
	if len(trigger) == 0 {
		return spec, nil
	}
	if t, _ := trigger["type"].(string); t != "" {
		if t != "order" {
			return spec, newAPIError(http.StatusBadRequest, "触发条件 type 仅支持 order")
		}
		spec.Type = t
	}
	if v, ok := trigger["min_amount_cents"]; ok {
		n, ok := toInt64(v)
		if !ok || n < 0 {
			return spec, newAPIError(http.StatusBadRequest, "触发条件 min_amount_cents 必须为非负整数")
		}
		spec.MinAmountCents = n
	}
	return spec, nil
}

// limitsSpec 限次（运行时消费 per_user_count/global_count；budget_cents
// 配置面校验，运行时扣减不在本项）。
type limitsSpec struct {
	PerUserCount int64
	GlobalCount  int64
}

func parseLimits(limits map[string]any) (limitsSpec, error) {
	var spec limitsSpec
	if len(limits) == 0 {
		return spec, nil
	}
	if v, ok := limits["per_user_count"]; ok {
		n, ok := toInt64(v)
		if !ok || n < 0 {
			return spec, newAPIError(http.StatusBadRequest, "限次 per_user_count 必须为非负整数")
		}
		spec.PerUserCount = n
	}
	if v, ok := limits["global_count"]; ok {
		n, ok := toInt64(v)
		if !ok || n < 0 {
			return spec, newAPIError(http.StatusBadRequest, "限次 global_count 必须为非负整数")
		}
		spec.GlobalCount = n
	}
	return spec, nil
}

// bonusQuantity 按语义计算额外赠送量（向下取整）。
func (s *MultiplierSpec) bonusQuantity(base int64) int64 {
	if base <= 0 {
		return 0
	}
	if s.Mode == ModeTotal {
		return base * (s.RateBp - 10000) / 10000
	}
	return base * s.RateBp / 10000
}

// candidate 运行时命中的活动候选（快照 + 已算出的规格与额外量）。
// GP-303 起 candidate 可为三类规则之一：multiplier（按倍率算额外量）、
// first_purchase（固定加赠，仅首充订单）、cumulative_recharge（每达一档
// 生成一候选，Milestone 为档位序号）。
type candidate struct {
	ActivityId    int64
	Version       int64
	Config        map[string]any
	RuleType      string             // model.RuleType*
	Spec          *MultiplierSpec    // multiplier 专用
	First         *FirstPurchaseSpec // 首充专用
	Cumulative    *CumulativeSpec    // 累充专用
	Milestone     int64              // 累充档位序号
	Trigger       triggerSpec
	Limits        limitsSpec
	Stackable     bool
	StackGroup    string
	BaseQuantity  int64
	ExtraQuantity int64
}

// bonusKey 排序键：额外量优先，大者优先；平票取 activity id 小者（确定性）。
func (c *candidate) bonusKey() (int64, int64) { return c.ExtraQuantity, -c.ActivityId }

// activityGroup 同一活动的候选聚合（累充一活动可产出多档位候选，叠加
// 裁决以活动为单位，活动胜出则其全部档位候选都发放）。
type activityGroup struct {
	activityId int64
	stackable  bool
	stackGroup string
	total      int64 // 活动合计额外量（叠加比较用）
	members    []*candidate
}

// resolveStack 叠加裁决（按活动聚合）：返回胜出活动的全部候选与排除明细
// （activity_id → 原因）。
func resolveStack(cands []*candidate) (winners []*candidate, excluded map[int64]string) {
	excluded = map[int64]string{}
	if len(cands) == 0 {
		return nil, excluded
	}
	byActivity := map[int64]*activityGroup{}
	var order []int64
	for _, c := range cands {
		g, ok := byActivity[c.ActivityId]
		if !ok {
			g = &activityGroup{activityId: c.ActivityId, stackable: c.Stackable,
				stackGroup: c.StackGroup}
			byActivity[c.ActivityId] = g
			order = append(order, c.ActivityId)
		}
		g.total += c.ExtraQuantity
		g.members = append(g.members, c)
	}
	groups := make([]*activityGroup, 0, len(order))
	for _, id := range order {
		groups = append(groups, byActivity[id])
	}
	// 规则一：任一 stackable=false → 独跑（合计额外量最大、平票 id 小）
	var solo *activityGroup
	for _, g := range groups {
		if g.stackable && !betterGroup(g, solo) {
			continue
		}
		if !g.stackable && (solo == nil || betterGroup(g, solo)) {
			solo = g
		}
	}
	if solo != nil {
		for _, g := range groups {
			if g.activityId == solo.activityId {
				continue
			}
			reason := "excluded-by-non-stackable"
			if !g.stackable {
				reason = "stack-group-conflict"
			}
			excluded[g.activityId] = reason
		}
		return solo.members, excluded
	}
	// 规则二：同 stack_group 互斥，组内挑合计额外量最大者；不同组/无组叠加。
	// 无组（stack_group 为空）的活动各自独立成组——"无组可叠加"意味着它们
	// 不与他人互斥，空串不可当作共享组键（否则两个无组活动会互相冲突）。
	byStackGroup := map[string][]*activityGroup{}
	var stackOrder []string
	for _, g := range groups {
		key := g.stackGroup
		if key == "" {
			key = fmt.Sprintf("__nogroup_%d", g.activityId)
		}
		if _, ok := byStackGroup[key]; !ok {
			stackOrder = append(stackOrder, key)
		}
		byStackGroup[key] = append(byStackGroup[key], g)
	}
	for _, sg := range stackOrder {
		members := byStackGroup[sg]
		best := members[0]
		for _, g := range members[1:] {
			if betterGroup(g, best) {
				best = g
			}
		}
		winners = append(winners, best.members...)
		for _, g := range members {
			if g.activityId != best.activityId {
				excluded[g.activityId] = "stack-group-conflict"
			}
		}
	}
	return winners, excluded
}

func betterGroup(a, b *activityGroup) bool {
	if b == nil {
		return true
	}
	if a.total != b.total {
		return a.total > b.total
	}
	return a.activityId < b.activityId // id 小者优先
}

// parseSnapshot 解析版本快照 JSON（Publish 时固化的完整配置）。
func parseSnapshot(raw string) (map[string]any, error) {
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("解析版本快照: %w", err)
	}
	return cfg, nil
}

func snapshotString(cfg map[string]any, key string) string {
	s, _ := cfg[key].(string)
	return s
}

func snapshotInts(cfg map[string]any, key string) []int64 {
	raw, _ := cfg[key].([]any)
	out := make([]int64, 0, len(raw))
	for _, v := range raw {
		if n, ok := toInt64(v); ok {
			out = append(out, n)
		}
	}
	return out
}

func snapshotMap(cfg map[string]any, key string) map[string]any {
	m, _ := cfg[key].(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

func snapshotBool(cfg map[string]any, key string) bool {
	b, _ := cfg[key].(bool)
	return b
}

// periodCovers 活动起止时间覆盖判定（RFC3339 带时区；含头含尾，秒级字符串
// 比较与 GP-301 validate 同一口径——快照内为标准化 RFC3339）。
func periodCovers(cfg map[string]any, paidAt string) bool {
	start := snapshotString(cfg, "start_at")
	end := snapshotString(cfg, "end_at")
	if start != "" && paidAt < start {
		return false
	}
	if end != "" && paidAt > end {
		return false
	}
	return true
}

// matchTarget 区服/渠道/游戏/人群命中判定。人群：audience.type 为空或
// "all" 对所有人生效；定向人群（如 new_user/returning）要求评估请求携带
// 玩家人群标签且相等（缺省不带 = 不命中，fail-closed 不发放）。
func matchTarget(cfg map[string]any, gameID string, serverID int64, channel, playerSegment string) error {
	if snapshotString(cfg, "game_id") != gameID {
		return fmt.Errorf("game mismatch")
	}
	if ids := snapshotInts(cfg, "server_ids"); len(ids) > 0 {
		hit := false
		for _, id := range ids {
			if id == serverID {
				hit = true
				break
			}
		}
		if !hit {
			return fmt.Errorf("server not in range")
		}
	}
	if ch := snapshotString(cfg, "channel"); ch != "" && ch != channel {
		return fmt.Errorf("channel mismatch")
	}
	aud := snapshotMap(cfg, "audience")
	t, _ := aud["type"].(string)
	if t != "" && t != "all" && t != strings.TrimSpace(playerSegment) {
		return fmt.Errorf("audience mismatch")
	}
	return nil
}
