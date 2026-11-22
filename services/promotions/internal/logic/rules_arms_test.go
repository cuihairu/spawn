package logic

// rewardrules 补漏分支：三类奖励规格解析（倍率 mode/rate_bp 边界、首充
// 正整数、累充空档/非正/非升序）、toInt64 六类型、trigger/limits 校验、
// bonusQuantity 三臂、resolveStack 叠加裁决（独跑/组内互斥/无组独立/平票
// id 小者）、betterGroup 三臂、periodCovers 三窗、matchTarget 四失配臂与
// snapshot* 帮助函数、parseSnapshot 成败。

import (
	"encoding/json"
	"testing"

	"github.com/tappi/tappi/services/promotions/model"
)

func TestParseMultiplierSpec_Arms(t *testing.T) {
	// 空 rewards → nil（无倍率奖励）
	if spec, err := ParseMultiplierSpec(map[string]any{}); spec != nil || err != nil {
		t.Fatalf("empty = %v/%v", spec, err)
	}
	// 非 multiplier 类型 → nil
	if spec, err := ParseMultiplierSpec(map[string]any{"type": "other"}); spec != nil || err != nil {
		t.Fatalf("other type = %v/%v", spec, err)
	}
	// mode 非法
	if _, err := ParseMultiplierSpec(map[string]any{"type": "multiplier", "mode": "half", "rate_bp": 10000.0}); err == nil {
		t.Fatal("bad mode = nil err")
	}
	// rate_bp 非整数
	if _, err := ParseMultiplierSpec(map[string]any{"type": "multiplier", "mode": "total", "rate_bp": "15000"}); err == nil {
		t.Fatal("rate_bp string = nil err")
	}
	// total 模式 rate_bp < 10000
	if _, err := ParseMultiplierSpec(map[string]any{"type": "multiplier", "mode": "total", "rate_bp": 9999.0}); err == nil {
		t.Fatal("total rate 9999 = nil err")
	}
	// extra 模式 rate_bp < 0
	if _, err := ParseMultiplierSpec(map[string]any{"type": "multiplier", "mode": "extra", "rate_bp": -1.0}); err == nil {
		t.Fatal("extra rate -1 = nil err")
	}
	// 合法 total
	spec, err := ParseMultiplierSpec(map[string]any{"type": "multiplier", "mode": "total", "rate_bp": 15000.0})
	if err != nil || spec.Mode != ModeTotal || spec.RateBp != 15000 {
		t.Fatalf("total = %+v/%v", spec, err)
	}
	// 合法 extra
	spec, err = ParseMultiplierSpec(map[string]any{"type": "multiplier", "mode": "extra", "rate_bp": 5000.0})
	if err != nil || spec.Mode != ModeExtra || spec.RateBp != 5000 {
		t.Fatalf("extra = %+v/%v", spec, err)
	}
}

func TestToInt64_Arms(t *testing.T) {
	if n, ok := toInt64(3.0); !ok || n != 3 {
		t.Fatalf("float64 int = %d/%v", n, ok)
	}
	if _, ok := toInt64(3.5); ok {
		t.Fatal("float64 frac = true, want false")
	}
	if n, ok := toInt64(json.Number("42")); !ok || n != 42 {
		t.Fatalf("json.Number = %d/%v", n, ok)
	}
	if _, ok := toInt64(json.Number("x")); ok {
		t.Fatal("json.Number bad = true, want false")
	}
	if n, ok := toInt64(int64(7)); !ok || n != 7 {
		t.Fatalf("int64 = %d/%v", n, ok)
	}
	if n, ok := toInt64(int(8)); !ok || n != 8 {
		t.Fatalf("int = %d/%v", n, ok)
	}
	if _, ok := toInt64("9"); ok {
		t.Fatal("string = true, want false")
	}
}

func TestParseFirstPurchaseSpec_Arms(t *testing.T) {
	if spec, err := ParseFirstPurchaseSpec(map[string]any{"type": "other"}); spec != nil || err != nil {
		t.Fatalf("other type = %v/%v", spec, err)
	}
	if _, err := ParseFirstPurchaseSpec(map[string]any{"type": model.RuleTypeFirstPurchase, "extra_quantity": 0.0}); err == nil {
		t.Fatal("zero extra = nil err")
	}
	if _, err := ParseFirstPurchaseSpec(map[string]any{"type": model.RuleTypeFirstPurchase, "extra_quantity": "x"}); err == nil {
		t.Fatal("bad extra = nil err")
	}
	spec, err := ParseFirstPurchaseSpec(map[string]any{"type": model.RuleTypeFirstPurchase, "extra_quantity": 500.0})
	if err != nil || spec.ExtraQuantity != 500 {
		t.Fatalf("valid = %+v/%v", spec, err)
	}
}

func TestParseCumulativeSpec_Arms(t *testing.T) {
	if spec, err := ParseCumulativeSpec(map[string]any{"type": "other"}); spec != nil || err != nil {
		t.Fatalf("other type = %v/%v", spec, err)
	}
	if _, err := ParseCumulativeSpec(map[string]any{"type": model.RuleTypeCumulative}); err == nil {
		t.Fatal("empty thresholds = nil err")
	}
	if _, err := ParseCumulativeSpec(map[string]any{"type": model.RuleTypeCumulative,
		"thresholds": []any{map[string]any{"cents": 0.0, "extra_quantity": 1.0}}}); err == nil {
		t.Fatal("zero cents = nil err")
	}
	if _, err := ParseCumulativeSpec(map[string]any{"type": model.RuleTypeCumulative,
		"thresholds": []any{map[string]any{"cents": 100.0, "extra_quantity": 0.0}}}); err == nil {
		t.Fatal("zero qty = nil err")
	}
	if _, err := ParseCumulativeSpec(map[string]any{"type": model.RuleTypeCumulative,
		"thresholds": []any{
			map[string]any{"cents": 200.0, "extra_quantity": 1.0},
			map[string]any{"cents": 100.0, "extra_quantity": 1.0},
		}}); err == nil {
		t.Fatal("descending = nil err")
	}
	spec, err := ParseCumulativeSpec(map[string]any{"type": model.RuleTypeCumulative,
		"thresholds": []any{
			map[string]any{"cents": 100.0, "extra_quantity": 10.0},
			map[string]any{"cents": 200.0, "extra_quantity": 20.0},
		}})
	if err != nil || len(spec.Thresholds) != 2 || spec.Thresholds[1].Cents != 200 {
		t.Fatalf("valid = %+v/%v", spec, err)
	}
}

func TestParseTrigger_Arms(t *testing.T) {
	sp, err := parseTrigger(map[string]any{})
	if err != nil || sp.Type != "order" || sp.MinAmountCents != 0 {
		t.Fatalf("empty = %+v/%v", sp, err)
	}
	sp, err = parseTrigger(map[string]any{"type": "order", "min_amount_cents": 100.0})
	if err != nil || sp.MinAmountCents != 100 {
		t.Fatalf("order = %+v/%v", sp, err)
	}
	if _, err := parseTrigger(map[string]any{"type": "refund"}); err == nil {
		t.Fatal("bad type = nil err")
	}
	if _, err := parseTrigger(map[string]any{"min_amount_cents": -1.0}); err == nil {
		t.Fatal("negative min = nil err")
	}
	if _, err := parseTrigger(map[string]any{"min_amount_cents": "x"}); err == nil {
		t.Fatal("string min = nil err")
	}
	// type 缺省（空串）走默认 order
	sp, err = parseTrigger(map[string]any{"type": "", "min_amount_cents": 50.0})
	if err != nil || sp.Type != "order" || sp.MinAmountCents != 50 {
		t.Fatalf("empty type = %+v/%v", sp, err)
	}
}

func TestParseLimits_Arms(t *testing.T) {
	if sp, err := parseLimits(map[string]any{}); err != nil || sp.PerUserCount != 0 {
		t.Fatalf("empty = %+v/%v", sp, err)
	}
	if _, err := parseLimits(map[string]any{"per_user_count": -1.0}); err == nil {
		t.Fatal("negative per_user = nil err")
	}
	if _, err := parseLimits(map[string]any{"global_count": "x"}); err == nil {
		t.Fatal("string global = nil err")
	}
	if _, err := parseLimits(map[string]any{"budget_cents": -5.0}); err == nil {
		t.Fatal("negative budget = nil err")
	}
	sp, err := parseLimits(map[string]any{"per_user_count": 3.0, "global_count": 10.0, "budget_cents": 500.0})
	if err != nil || sp.PerUserCount != 3 || sp.GlobalCount != 10 || sp.BudgetCents != 500 {
		t.Fatalf("valid = %+v/%v", sp, err)
	}
	// 非法 per_user_count（字符串）
	if _, err := parseLimits(map[string]any{"per_user_count": "3"}); err == nil {
		t.Fatal("string per_user = nil err")
	}
}

func TestBonusQuantity_Arms(t *testing.T) {
	if got := (&MultiplierSpec{Mode: ModeTotal, RateBp: 15000}).bonusQuantity(0); got != 0 {
		t.Fatalf("zero base = %d, want 0", got)
	}
	if got := (&MultiplierSpec{Mode: ModeTotal, RateBp: 15000}).bonusQuantity(-100); got != 0 {
		t.Fatalf("negative base = %d, want 0", got)
	}
	if got := (&MultiplierSpec{Mode: ModeTotal, RateBp: 15000}).bonusQuantity(10000); got != 5000 {
		t.Fatalf("total bonus = %d, want 5000", got)
	}
	if got := (&MultiplierSpec{Mode: ModeExtra, RateBp: 5000}).bonusQuantity(10000); got != 5000 {
		t.Fatalf("extra bonus = %d, want 5000", got)
	}
	if got := (&MultiplierSpec{Mode: ModeExtra, RateBp: 0}).bonusQuantity(10000); got != 0 {
		t.Fatalf("extra zero rate = %d, want 0", got)
	}
}

func TestBetterGroup_Arms(t *testing.T) {
	if !betterGroup(&activityGroup{activityId: 1, total: 10}, nil) {
		t.Fatal("better vs nil = false, want true")
	}
	if !betterGroup(&activityGroup{activityId: 1, total: 20}, &activityGroup{activityId: 2, total: 10}) {
		t.Fatal("higher total should win")
	}
	if betterGroup(&activityGroup{activityId: 1, total: 10}, &activityGroup{activityId: 2, total: 20}) {
		t.Fatal("lower total should lose")
	}
	if !betterGroup(&activityGroup{activityId: 1, total: 10}, &activityGroup{activityId: 2, total: 10}) {
		t.Fatal("same total, smaller id should win")
	}
	if betterGroup(&activityGroup{activityId: 2, total: 10}, &activityGroup{activityId: 1, total: 10}) {
		t.Fatal("same total, larger id should lose")
	}
}

func TestResolveStack_EmptyAndSoloArms(t *testing.T) {
	winners, excluded := resolveStack(nil)
	if winners != nil || len(excluded) != 0 {
		t.Fatalf("empty = %v/%v", winners, excluded)
	}
	// 单个非 stackable 活动：独跑（排除另一活动）
	a := &candidate{ActivityId: 1, ExtraQuantity: 5, Stackable: false}
	b := &candidate{ActivityId: 2, ExtraQuantity: 9, Stackable: false}
	winners, excluded = resolveStack([]*candidate{a, b})
	if len(winners) != 1 || winners[0].ActivityId != 2 {
		t.Fatalf("solo winners = %v (want id2, higher total)", winners)
	}
	if len(excluded) != 1 || excluded[1] != "stack-group-conflict" {
		t.Fatalf("solo excluded = %v", excluded)
	}
	// 命中里有一个 stackable=true、一个非 stackable → 非 stackable 独跑
	c := &candidate{ActivityId: 3, ExtraQuantity: 1, Stackable: true}
	winners, excluded = resolveStack([]*candidate{c, b})
	if len(winners) != 1 || winners[0].ActivityId != 2 {
		t.Fatalf("non-stackable solo = %v", winners)
	}
	if len(excluded) != 1 || excluded[3] != "excluded-by-non-stackable" {
		t.Fatalf("excluded reason = %v", excluded)
	}
}

func TestResolveStack_GroupConflictArms(t *testing.T) {
	// 同 stack_group 互斥，组内挑合计大者；不同组/无组叠加
	g1a := &candidate{ActivityId: 1, ExtraQuantity: 5, Stackable: true, StackGroup: "x"}
	g1b := &candidate{ActivityId: 2, ExtraQuantity: 9, Stackable: true, StackGroup: "x"}
	g2 := &candidate{ActivityId: 3, ExtraQuantity: 3, Stackable: true, StackGroup: "y"}
	winners, excluded := resolveStack([]*candidate{g1a, g1b, g2})
	if len(winners) != 2 {
		t.Fatalf("winners = %v, want id2+id3", winners)
	}
	if len(excluded) != 1 || excluded[1] != "stack-group-conflict" {
		t.Fatalf("excluded = %v", excluded)
	}
	// 无组活动各自独立（空串组键不互斥）
	n1 := &candidate{ActivityId: 4, ExtraQuantity: 2, Stackable: true}
	n2 := &candidate{ActivityId: 5, ExtraQuantity: 3, Stackable: true}
	winners, excluded = resolveStack([]*candidate{n1, n2})
	if len(winners) != 2 || len(excluded) != 0 {
		t.Fatalf("nogroup winners/excluded = %v/%v", winners, excluded)
	}
	// 累充多档同活动 → 聚合为一个活动组，活动胜出则全部档位候选胜出
	m1 := &candidate{ActivityId: 6, ExtraQuantity: 2, Stackable: true, StackGroup: "z", Milestone: 0}
	m2 := &candidate{ActivityId: 6, ExtraQuantity: 3, Stackable: true, StackGroup: "z", Milestone: 1}
	loser := &candidate{ActivityId: 7, ExtraQuantity: 1, Stackable: true, StackGroup: "z"}
	winners, excluded = resolveStack([]*candidate{m1, m2, loser})
	if len(winners) != 2 || excluded[7] != "stack-group-conflict" {
		t.Fatalf("cumulative group winners/excluded = %v/%v", winners, excluded)
	}
}

func TestPeriodCovers_Arms(t *testing.T) {
	cfg := map[string]any{"start_at": "2026-01-01T00:00:00Z", "end_at": "2026-12-31T00:00:00Z"}
	if periodCovers(cfg, "2025-12-31T23:59:59Z") {
		t.Fatal("before start should not cover")
	}
	if periodCovers(cfg, "2027-01-01T00:00:00Z") {
		t.Fatal("after end should not cover")
	}
	if !periodCovers(cfg, "2026-06-01T00:00:00Z") {
		t.Fatal("within should cover")
	}
	if !periodCovers(cfg, "2026-01-01T00:00:00Z") || !periodCovers(cfg, "2026-12-31T00:00:00Z") {
		t.Fatal("boundary should cover")
	}
	if !periodCovers(map[string]any{}, "2026-06-01T00:00:00Z") {
		t.Fatal("empty window should cover")
	}
}

func TestMatchTarget_Arms(t *testing.T) {
	cfg := baseSnapshotMap()
	// 游戏不符
	if err := matchTarget(cfg, "g2", 1, "", ""); err == nil {
		t.Fatal("game mismatch = nil err")
	}
	// 区服不在名单
	if err := matchTarget(cfg, "g1", 9, "", ""); err == nil {
		t.Fatal("server mismatch = nil err")
	}
	// 渠道不符（快照限定 ios，请求带 android）
	cfgChan := baseSnapshotMap()
	cfgChan["channel"] = "ios"
	if err := matchTarget(cfgChan, "g1", 1, "android", ""); err == nil {
		t.Fatal("channel mismatch = nil err")
	}
	// 快照渠道为空 = 全部渠道，任何请求渠道都命中
	if err := matchTarget(cfg, "g1", 1, "ios", ""); err != nil {
		t.Fatalf("empty channel = %v", err)
	}
	// 人群定向不符（缺省标签 fail-closed）
	aud := map[string]any{"type": "new_user"}
	cfg2 := map[string]any{}
	for k, v := range cfg {
		cfg2[k] = v
	}
	cfg2["audience"] = aud
	if err := matchTarget(cfg2, "g1", 1, "", ""); err == nil {
		t.Fatal("audience mismatch = nil err")
	}
	// 命中
	if err := matchTarget(cfg, "g1", 1, "", "anything"); err != nil {
		t.Fatalf("hit = %v", err)
	}
	// 空 audience / all
	if err := matchTarget(map[string]any{"game_id": "g1"}, "g1", 1, "", ""); err != nil {
		t.Fatalf("empty audience = %v", err)
	}
}

func baseSnapshotMap() map[string]any {
	var m map[string]any
	if err := json.Unmarshal([]byte(baseSnapshot()), &m); err != nil {
		panic(err)
	}
	return m
}

func TestParseSnapshot_Arms(t *testing.T) {
	m, err := parseSnapshot(baseSnapshot())
	if err != nil || m["name"] != "夏日活动" {
		t.Fatalf("valid = %v/%v", m, err)
	}
	if _, err := parseSnapshot("not-json"); err == nil {
		t.Fatal("bad json = nil err")
	}
}

func TestSnapshotHelpers_Arms(t *testing.T) {
	cfg := map[string]any{
		"name": "n", "server_ids": []any{1.0, 2.0, "x"}, "audience": map[string]any{"type": "all"},
		"stackable": true, "limits": map[string]any{"budget_cents": 100},
	}
	if got := snapshotString(cfg, "name"); got != "n" {
		t.Fatalf("string = %q", got)
	}
	if got := snapshotString(cfg, "missing"); got != "" {
		t.Fatalf("missing string = %q", got)
	}
	if got := snapshotInts(cfg, "server_ids"); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("ints = %v", got)
	}
	if got := snapshotInts(cfg, "missing"); len(got) != 0 {
		t.Fatalf("missing ints = %v", got)
	}
	if got := snapshotMap(cfg, "audience"); got["type"] != "all" {
		t.Fatalf("map = %v", got)
	}
	if got := snapshotMap(cfg, "missing"); got == nil || len(got) != 0 {
		t.Fatalf("missing map = %v", got)
	}
	if !snapshotBool(cfg, "stackable") {
		t.Fatal("bool = false")
	}
	if snapshotBool(cfg, "missing") {
		t.Fatal("missing bool = true")
	}
}
