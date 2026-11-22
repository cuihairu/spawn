package logic

// activitylogic 补漏分支：validate 全校验臂（空名/空游戏/空时区/空起止/
// 起晚于止/非正区服/负预算/未知奖励类型/三类规格非法）、marshalJSON 含
// 不可序列化值的 400 臂、Create/Update/Publish/Disable/Revise/List/Get/
// Versions/ListAudit 主路径与状态冲突（404/409/500）、角色 502/403 臂、
// notifyActivityPublished 三降级臂（未启用/拉受众失败/空受众/成功扇出）。
//
// 登记不可达（不测）：marshalInts 的 json.Marshal 失败臂（[]int64 恒可序
// 列化）；Publish 快照 json.Marshal 失败臂（map 恒可序列化）。

import (
	"net/http"
	"testing"

	"github.com/tappi/tappi/services/promotions/internal/types"
)

func validActivityCfg() types.ActivityConfig {
	return types.ActivityConfig{
		Name: "夏日充值", GameId: "g1", Timezone: "Asia/Shanghai",
		StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-12-31T00:00:00Z",
		Trigger: map[string]any{"type": "order", "min_amount_cents": 1.0},
		Limits:  map[string]any{},
		Rewards: map[string]any{"type": "multiplier", "mode": "total", "rate_bp": 15000.0},
	}
}

func TestValidate_Arms(t *testing.T) {
	// 全空配置逐臂
	if err := validate(&types.ActivityConfig{}); err == nil {
		t.Fatal("empty cfg = nil err")
	}
	cfg := validActivityCfg()
	cfg.Name = " "
	if err := validate(&cfg); err == nil {
		t.Fatal("blank name = nil err")
	}
	cfg = validActivityCfg()
	cfg.GameId = " "
	if err := validate(&cfg); err == nil {
		t.Fatal("blank game = nil err")
	}
	cfg = validActivityCfg()
	cfg.Timezone = " "
	if err := validate(&cfg); err == nil {
		t.Fatal("blank timezone = nil err")
	}
	cfg = validActivityCfg()
	cfg.StartAt, cfg.EndAt = "", "2026-12-31T00:00:00Z"
	if err := validate(&cfg); err == nil {
		t.Fatal("empty start = nil err")
	}
	cfg = validActivityCfg()
	cfg.StartAt, cfg.EndAt = "2026-12-31T00:00:00Z", "2026-01-01T00:00:00Z"
	if err := validate(&cfg); err == nil {
		t.Fatal("start after end = nil err")
	}
	cfg = validActivityCfg()
	cfg.ServerIds = []int64{0}
	if err := validate(&cfg); err == nil {
		t.Fatal("zero server = nil err")
	}
	cfg = validActivityCfg()
	cfg.Limits = map[string]any{"budget_cents": -1.0}
	if err := validate(&cfg); err == nil {
		t.Fatal("negative budget = nil err")
	}
	cfg = validActivityCfg()
	cfg.Rewards = map[string]any{"type": "unknown"}
	if err := validate(&cfg); err == nil {
		t.Fatal("unknown rewards = nil err")
	}
	cfg = validActivityCfg()
	cfg.Rewards = map[string]any{"type": "multiplier", "mode": "total", "rate_bp": 1.0}
	if err := validate(&cfg); err == nil {
		t.Fatal("bad multiplier = nil err")
	}
	cfg = validActivityCfg()
	cfg.Rewards = map[string]any{"type": "first_purchase", "extra_quantity": 0.0}
	if err := validate(&cfg); err == nil {
		t.Fatal("bad first purchase = nil err")
	}
	cfg = validActivityCfg()
	cfg.Rewards = map[string]any{"type": "cumulative_recharge"}
	if err := validate(&cfg); err == nil {
		t.Fatal("bad cumulative = nil err")
	}
	// 非数值 budget（非 float64 → 不校验，放行）
	cfg = validActivityCfg()
	cfg.Limits = map[string]any{"budget_cents": "unlimited"}
	if err := validate(&cfg); err != nil {
		t.Fatalf("string budget = %v", err)
	}
	// 空奖励（len==0 跳过规格校验）
	cfg = validActivityCfg()
	cfg.Rewards = nil
	if err := validate(&cfg); err != nil {
		t.Fatalf("no rewards = %v", err)
	}
	// 合法
	validCfg := validActivityCfg()
	if err := validate(&validCfg); err != nil {
		t.Fatalf("valid = %v", err)
	}
}

func TestActivityLifecycle_Arms(t *testing.T) {
	env := newLogicEnv(t, "")
	env.withCatalog(t, "operator", http.StatusOK)
	l := NewActivityLogic(adminCtx(7, "1.2.3.4"), env.sc)
	const auth = "Bearer t"

	// 角色不可达 → 502；角色不足 → 403
	env.sc.Config.GameCatalog.BaseURL = "http://127.0.0.1:1"
	_, err := l.Create(&types.CreateActivityRequest{ActivityConfig: validActivityCfg()}, auth)
	mustAPIError(t, err, http.StatusBadGateway)
	env.withCatalog(t, "player", http.StatusOK)
	_, err = l.Create(&types.CreateActivityRequest{ActivityConfig: validActivityCfg()}, auth)
	mustAPIError(t, err, http.StatusForbidden)
	env.withCatalog(t, "operator", http.StatusOK)

	// 校验失败 → 400
	bad := validActivityCfg()
	bad.Name = ""
	_, err = l.Create(&types.CreateActivityRequest{ActivityConfig: bad}, auth)
	mustAPIError(t, err, http.StatusBadRequest)

	// 人群配置不可序列化（chan 值）→ 400
	chanCfg := validActivityCfg()
	chanCfg.Audience = map[string]any{"ch": make(chan int)}
	_, err = l.Create(&types.CreateActivityRequest{ActivityConfig: chanCfg}, auth)
	mustAPIError(t, err, http.StatusBadRequest)
	chanCfg = validActivityCfg()
	chanCfg.Trigger = map[string]any{"ch": make(chan int)}
	_, err = l.Create(&types.CreateActivityRequest{ActivityConfig: chanCfg}, auth)
	mustAPIError(t, err, http.StatusBadRequest)
	chanCfg = validActivityCfg()
	chanCfg.Limits = map[string]any{"ch": make(chan int)}
	_, err = l.Create(&types.CreateActivityRequest{ActivityConfig: chanCfg}, auth)
	mustAPIError(t, err, http.StatusBadRequest)
	chanCfg = validActivityCfg()
	chanCfg.Rewards = map[string]any{"type": "multiplier", "mode": "total", "rate_bp": 15000.0, "ch": make(chan int)}
	_, err = l.Create(&types.CreateActivityRequest{ActivityConfig: chanCfg}, auth)
	mustAPIError(t, err, http.StatusBadRequest)

	// 建草稿
	resp, err := l.Create(&types.CreateActivityRequest{ActivityConfig: validActivityCfg()}, auth)
	if err != nil || resp.Activity.Status != "draft" || resp.Activity.Id == 0 {
		t.Fatalf("create = %+v/%v", resp, err)
	}
	id := resp.Activity.Id

	// 草稿更新（含区服列表）
	upd := validActivityCfg()
	upd.ServerIds = []int64{1, 2}
	upd.Name = "夏日充值·改"
	if _, err := l.Update(&types.UpdateActivityRequest{Id: id, ActivityConfig: upd}, auth); err != nil {
		t.Fatalf("update: %v", err)
	}
	// 更新查无 → 404
	_, err = l.Update(&types.UpdateActivityRequest{Id: 9999, ActivityConfig: validActivityCfg()}, auth)
	mustAPIError(t, err, http.StatusNotFound)

	// 发布 → published
	pub, err := l.Publish(&types.ActivityPathRequest{Id: id}, auth)
	if err != nil || pub.Activity.Status != "published" || pub.Activity.CurrentVersion != 1 {
		t.Fatalf("publish = %+v/%v", pub, err)
	}
	// 已发布再发 → 409
	_, err = l.Publish(&types.ActivityPathRequest{Id: id}, auth)
	mustAPIError(t, err, http.StatusConflict)
	// 发布查无 → 404
	_, err = l.Publish(&types.ActivityPathRequest{Id: 9999}, auth)
	mustAPIError(t, err, http.StatusNotFound)

	// 已发布不可编辑 → 409
	_, err = l.Update(&types.UpdateActivityRequest{Id: id, ActivityConfig: validActivityCfg()}, auth)
	mustAPIError(t, err, http.StatusConflict)

	// 版本列表 + 详情 + 审计
	vers, err := l.Versions(&types.ActivityPathRequest{Id: id}, auth)
	if err != nil || len(vers.Items) != 1 || vers.Items[0].Version != 1 {
		t.Fatalf("versions = %+v/%v", vers, err)
	}
	got, err := l.Get(&types.ActivityPathRequest{Id: id}, auth)
	if err != nil || got.Activity.Name != "夏日充值·改" {
		t.Fatalf("get = %+v/%v", got, err)
	}
	_, err = l.Get(&types.ActivityPathRequest{Id: 9999}, auth)
	mustAPIError(t, err, http.StatusNotFound)
	audits, err := l.ListAudit(id, auth)
	if err != nil || len(audits.Items) == 0 {
		t.Fatalf("list audit = %+v/%v", audits, err)
	}

	// 停用（published → disabled）；草稿停用 → 409
	if _, err := l.Disable(&types.ActivityPathRequest{Id: id}, auth); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, err := l.Disable(&types.ActivityPathRequest{Id: id}, auth); err == nil {
		t.Fatal("disable disabled = nil err")
	}
	// 回草稿（disabled → draft）；草稿回草稿 → 409
	if _, err := l.Revise(&types.ActivityPathRequest{Id: id}, auth); err != nil {
		t.Fatalf("revise: %v", err)
	}
	_, err = l.Revise(&types.ActivityPathRequest{Id: id}, auth)
	mustAPIError(t, err, http.StatusConflict)
	_, err = l.Revise(&types.ActivityPathRequest{Id: 9999}, auth)
	mustAPIError(t, err, http.StatusNotFound)
	_, err = l.Disable(&types.ActivityPathRequest{Id: 9999}, auth)
	mustAPIError(t, err, http.StatusNotFound)

	// 列表（过滤 + 空结果）
	list, err := l.List(&types.ActivityListRequest{GameId: "g1"}, auth)
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("list = %+v/%v", list, err)
	}
	if list, _ := l.List(&types.ActivityListRequest{GameId: "nope"}, auth); len(list.Items) != 0 {
		t.Fatalf("list other = %+v", list.Items)
	}
	// 版本列表查无 → 404
	_, err = l.Versions(&types.ActivityPathRequest{Id: 9999}, auth)
	mustAPIError(t, err, http.StatusNotFound)
}

func TestActivityLogic_ClosedDBArms(t *testing.T) {
	env := newLogicEnv(t, "")
	env.withCatalog(t, "operator", http.StatusOK)
	l := NewActivityLogic(adminCtx(7, "1.2.3.4"), env.sc)
	const auth = "Bearer t"

	// 先建一条草稿再闭库：各方法首查即失败 → 500
	resp, err := l.Create(&types.CreateActivityRequest{ActivityConfig: validActivityCfg()}, auth)
	if err != nil {
		t.Fatalf("seed create: %v", err)
	}
	id := resp.Activity.Id
	if err := env.db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := l.Create(&types.CreateActivityRequest{ActivityConfig: validActivityCfg()}, auth); err == nil {
		t.Fatal("closed create = nil err")
	}
	if _, err := l.Update(&types.UpdateActivityRequest{Id: id, ActivityConfig: validActivityCfg()}, auth); err == nil {
		t.Fatal("closed update = nil err")
	}
	if _, err := l.Publish(&types.ActivityPathRequest{Id: id}, auth); err == nil {
		t.Fatal("closed publish = nil err")
	}
	if _, err := l.Disable(&types.ActivityPathRequest{Id: id}, auth); err == nil {
		t.Fatal("closed disable = nil err")
	}
	if _, err := l.Revise(&types.ActivityPathRequest{Id: id}, auth); err == nil {
		t.Fatal("closed revise = nil err")
	}
	if _, err := l.List(&types.ActivityListRequest{}, auth); err == nil {
		t.Fatal("closed list = nil err")
	}
	if _, err := l.Get(&types.ActivityPathRequest{Id: id}, auth); err == nil {
		t.Fatal("closed get = nil err")
	}
	if _, err := l.Versions(&types.ActivityPathRequest{Id: id}, auth); err == nil {
		t.Fatal("closed versions = nil err")
	}
	if _, err := l.ListAudit(id, auth); err == nil {
		t.Fatal("closed list audit = nil err")
	}
}

func TestNotifyActivityPublished_Arms(t *testing.T) {
	env := newLogicEnv(t, "")
	env.withCatalog(t, "operator", http.StatusOK)

	// community 投递桩（计数）+ catalog 预约人群桩
	var hits int
	community := communityStub(t, &hits)
	reserved := reservationStub(t, `{"game_id":"g1","user_ids":[7,8]}`)
	env.withNotify(t, community, reserved)
	l := NewActivityLogic(adminCtx(7, "1.2.3.4"), env.sc)
	const auth = "Bearer t"

	// 发布成功 → 受众拉取 + 投递
	if _, err := l.Create(&types.CreateActivityRequest{ActivityConfig: validActivityCfg()}, auth); err != nil {
		t.Fatalf("create: %v", err)
	}
	rows, _ := env.sc.ActivityDB.List("g1", "", 10, 0)
	if len(rows) == 0 {
		t.Fatal("no seeded activity")
	}
	if _, err := l.Publish(&types.ActivityPathRequest{Id: rows[0].Id}, auth); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if hits != 1 {
		t.Fatalf("notify hits = %d, want 1", hits)
	}

	// 受众拉取失败（catalog 不可达）→ 仅记日志，发布仍成功
	env.withNotify(t, community, "http://127.0.0.1:1")
	resp2, err := l.Create(&types.CreateActivityRequest{ActivityConfig: validActivityCfg()}, auth)
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}
	if _, err := l.Publish(&types.ActivityPathRequest{Id: resp2.Activity.Id}, auth); err != nil {
		t.Fatalf("publish 2 (catalog unreachable): %v", err)
	}

	// 空受众（0 人）→ 直接返回
	env.withNotify(t, community, reservationStub(t, `{"game_id":"g1","user_ids":[]}`))
	resp3, err := l.Create(&types.CreateActivityRequest{ActivityConfig: validActivityCfg()}, auth)
	if err != nil {
		t.Fatalf("create 3: %v", err)
	}
	if _, err := l.Publish(&types.ActivityPathRequest{Id: resp3.Activity.Id}, auth); err != nil {
		t.Fatalf("publish 3: %v", err)
	}

	// Notify 未启用（零值）→ 整体跳过
	env.sc.Notify = nil
	resp4, err := l.Create(&types.CreateActivityRequest{ActivityConfig: validActivityCfg()}, auth)
	if err != nil {
		t.Fatalf("create 4: %v", err)
	}
	if _, err := l.Publish(&types.ActivityPathRequest{Id: resp4.Activity.Id}, auth); err != nil {
		t.Fatalf("publish 4 (notify off): %v", err)
	}
}
