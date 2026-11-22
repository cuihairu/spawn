package logic

// giftpacklogic 补漏分支：validatePack 全校验臂、礼包管理面 CRUD 与状态
// 冲突（404/409/500，含角色 502/403）、marshal 不可序列值 400 臂、码导入
// （草稿可补码/空列表/查无）、码列表、领取记录列表、玩家侧 PlayerList/
// PlayerGet/MyClaims（未认证 401/未上架 404/区服过滤）、Claim 全链路
// （幂等重放/限领/全量上限/码领完/区服/人群/窗口外）、Redeem 验签链、
// packTargetHit/packAudienceHit 全臂、DROP 表注入的查询错误 500 臂。
//
// 登记不可达（不测）：Update/ImportCodes 落库错误臂（查询错误先命中同一
// 失败点）；ensureStock 的 CountAvailableCodes 查询错误臂；Claim 的
// IssueCode 通用错误臂（SELECT 仅返回 ErrNoRows 或 nil）与 BindClaimCode/
// GetCode 失败回退臂（同库同事务窗口无法选择性注入）；InsertClaim 冲突后
// 二次幂等分支（单线程无并发窗口）。

import (
	"net/http"
	"testing"

	"github.com/tappi/tappi/services/promotions/internal/types"
	"github.com/tappi/tappi/services/promotions/model"
)

func validPackCfg() types.GiftPackConfig {
	return types.GiftPackConfig{
		Name: "新手礼包", GameId: "g1", Timezone: "Asia/Shanghai",
		StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-12-31T00:00:00Z",
		ServerIds: []int64{1}, PerUserLimit: 1,
		Content: map[string]any{"items": []any{"gem"}},
	}
}

// seedPack 管理面建包 → 导码 → 发布，返回礼包 id。
func seedPack(t *testing.T, env *logicEnv, cfg types.GiftPackConfig, codes []string) int64 {
	t.Helper()
	env.withCatalog(t, "operator", http.StatusOK)
	l := NewGiftPackLogic(adminCtx(7, "1.2.3.4"), env.sc)
	const auth = "Bearer t"
	resp, err := l.Create(&types.CreateGiftPackRequest{GiftPackConfig: cfg}, auth)
	if err != nil {
		t.Fatalf("create pack: %v", err)
	}
	id := resp.GiftPack.Id
	if len(codes) > 0 {
		if _, err := l.ImportCodes(&types.ImportCodesRequest{Id: id, Codes: codes}, auth); err != nil {
			t.Fatalf("import codes: %v", err)
		}
	}
	if _, err := l.Publish(&types.GiftPackPathRequest{Id: id}, auth); err != nil {
		t.Fatalf("publish pack: %v", err)
	}
	return id
}

func TestValidatePack_Arms(t *testing.T) {
	if err := validatePack(&types.GiftPackConfig{}); err == nil {
		t.Fatal("empty = nil err")
	}
	cfg := validPackCfg()
	cfg.Name = " "
	if err := validatePack(&cfg); err == nil {
		t.Fatal("blank name = nil err")
	}
	cfg = validPackCfg()
	cfg.GameId = " "
	if err := validatePack(&cfg); err == nil {
		t.Fatal("blank game = nil err")
	}
	cfg = validPackCfg()
	cfg.Timezone = " "
	if err := validatePack(&cfg); err == nil {
		t.Fatal("blank timezone = nil err")
	}
	cfg = validPackCfg()
	cfg.StartAt, cfg.EndAt = "", "2026-12-31T00:00:00Z"
	if err := validatePack(&cfg); err == nil {
		t.Fatal("empty start = nil err")
	}
	cfg = validPackCfg()
	cfg.StartAt, cfg.EndAt = "2026-12-31T00:00:00Z", "2026-01-01T00:00:00Z"
	if err := validatePack(&cfg); err == nil {
		t.Fatal("start after end = nil err")
	}
	cfg = validPackCfg()
	cfg.PerUserLimit = 0
	if err := validatePack(&cfg); err == nil {
		t.Fatal("zero per user = nil err")
	}
	cfg = validPackCfg()
	cfg.TotalLimit = -1
	if err := validatePack(&cfg); err == nil {
		t.Fatal("negative total = nil err")
	}
	cfg = validPackCfg()
	cfg.ServerIds = []int64{0}
	if err := validatePack(&cfg); err == nil {
		t.Fatal("zero server = nil err")
	}
	cfg = validPackCfg()
	cfg.Content = nil
	if err := validatePack(&cfg); err == nil {
		t.Fatal("empty content = nil err")
	}
	validCfg := validPackCfg()
	if err := validatePack(&validCfg); err != nil {
		t.Fatalf("valid = %v", err)
	}
}

func TestGiftPackAdmin_Arms(t *testing.T) {
	env := newLogicEnv(t, "")
	l := NewGiftPackLogic(adminCtx(7, "1.2.3.4"), env.sc)
	const auth = "Bearer t"

	// 角色 502/403 臂
	env.sc.Config.GameCatalog.BaseURL = "http://127.0.0.1:1"
	if _, err := l.Create(&types.CreateGiftPackRequest{GiftPackConfig: validPackCfg()}, auth); err == nil {
		t.Fatal("unreachable catalog = nil err")
	} else {
		mustAPIError(t, err, http.StatusBadGateway)
	}
	env.withCatalog(t, "player", http.StatusOK)
	if _, err := l.Create(&types.CreateGiftPackRequest{GiftPackConfig: validPackCfg()}, auth); err == nil {
		t.Fatal("player role = nil err")
	} else {
		mustAPIError(t, err, http.StatusForbidden)
	}
	env.withCatalog(t, "operator", http.StatusOK)

	// 校验失败 → 400
	bad := validPackCfg()
	bad.Name = ""
	if _, err := l.Create(&types.CreateGiftPackRequest{GiftPackConfig: bad}, auth); err == nil {
		t.Fatal("bad cfg = nil err")
	} else {
		mustAPIError(t, err, http.StatusBadRequest)
	}

	// 不可序列化值（chan）→ 400
	chanCfg := validPackCfg()
	chanCfg.Audience = map[string]any{"ch": make(chan int)}
	_, err := l.Create(&types.CreateGiftPackRequest{GiftPackConfig: chanCfg}, auth)
	mustAPIError(t, err, http.StatusBadRequest)
	chanCfg = validPackCfg()
	chanCfg.Content = map[string]any{"ch": make(chan int)}
	_, err = l.Create(&types.CreateGiftPackRequest{GiftPackConfig: chanCfg}, auth)
	mustAPIError(t, err, http.StatusBadRequest)

	// 建草稿
	resp, err := l.Create(&types.CreateGiftPackRequest{GiftPackConfig: validPackCfg()}, auth)
	if err != nil || resp.GiftPack.Status != "draft" || resp.GiftPack.Id == 0 {
		t.Fatalf("create = %+v/%v", resp, err)
	}
	id := resp.GiftPack.Id

	// 草稿更新
	upd := validPackCfg()
	upd.Name = "新手礼包·改"
	upd.PerUserLimit = 2
	if _, err := l.Update(&types.UpdateGiftPackRequest{Id: id, GiftPackConfig: upd}, auth); err != nil {
		t.Fatalf("update: %v", err)
	}
	// 更新校验失败 → 400；不可序列化值 → 400
	badUpd := validPackCfg()
	badUpd.Name = ""
	_, err = l.Update(&types.UpdateGiftPackRequest{Id: id, GiftPackConfig: badUpd}, auth)
	mustAPIError(t, err, http.StatusBadRequest)
	chanUpd := validPackCfg()
	chanUpd.Audience = map[string]any{"ch": make(chan int)}
	_, err = l.Update(&types.UpdateGiftPackRequest{Id: id, GiftPackConfig: chanUpd}, auth)
	mustAPIError(t, err, http.StatusBadRequest)
	chanUpd = validPackCfg()
	chanUpd.Content = map[string]any{"ch": make(chan int)}
	_, err = l.Update(&types.UpdateGiftPackRequest{Id: id, GiftPackConfig: chanUpd}, auth)
	mustAPIError(t, err, http.StatusBadRequest)
	// 更新查无 → 404
	_, err = l.Update(&types.UpdateGiftPackRequest{Id: 9999, GiftPackConfig: validPackCfg()}, auth)
	mustAPIError(t, err, http.StatusNotFound)
	// 草稿停用 → 409（requireStatus want=published）
	_, err = l.Disable(&types.GiftPackPathRequest{Id: id}, auth)
	mustAPIError(t, err, http.StatusConflict)

	// 发布 → published
	pub, err := l.Publish(&types.GiftPackPathRequest{Id: id}, auth)
	if err != nil || pub.GiftPack.Status != "published" {
		t.Fatalf("publish = %+v/%v", pub, err)
	}
	// 已发布更新 → 409
	_, err = l.Update(&types.UpdateGiftPackRequest{Id: id, GiftPackConfig: validPackCfg()}, auth)
	mustAPIError(t, err, http.StatusConflict)
	// 已发布再发布 → 409
	_, err = l.Publish(&types.GiftPackPathRequest{Id: id}, auth)
	mustAPIError(t, err, http.StatusConflict)
	// 停用 → disabled
	if _, err := l.Disable(&types.GiftPackPathRequest{Id: id}, auth); err != nil {
		t.Fatalf("disable: %v", err)
	}
	// 停用已停用 → 409
	_, err = l.Disable(&types.GiftPackPathRequest{Id: id}, auth)
	mustAPIError(t, err, http.StatusConflict)
	// 回草稿 → draft
	rev, err := l.Revise(&types.GiftPackPathRequest{Id: id}, auth)
	if err != nil || rev.GiftPack.Status != "draft" {
		t.Fatalf("revise = %+v/%v", rev, err)
	}
	// 草稿回草稿 → 409
	_, err = l.Revise(&types.GiftPackPathRequest{Id: id}, auth)
	mustAPIError(t, err, http.StatusConflict)
	// 回到草稿后补导码（草稿可补码）→ 发布
	if _, err := l.ImportCodes(&types.ImportCodesRequest{Id: id, Codes: []string{"P1", "P1"}}, auth); err != nil {
		t.Fatalf("import after revise: %v", err)
	}
	if _, err := l.Publish(&types.GiftPackPathRequest{Id: id}, auth); err != nil {
		t.Fatalf("publish after codes: %v", err)
	}

	// 查无 404 臂
	_, err = l.Publish(&types.GiftPackPathRequest{Id: 9999}, auth)
	mustAPIError(t, err, http.StatusNotFound)
	_, err = l.Disable(&types.GiftPackPathRequest{Id: 9999}, auth)
	mustAPIError(t, err, http.StatusNotFound)
	_, err = l.Revise(&types.GiftPackPathRequest{Id: 9999}, auth)
	mustAPIError(t, err, http.StatusNotFound)
	_, err = l.Get(&types.GiftPackPathRequest{Id: 9999}, auth)
	mustAPIError(t, err, http.StatusNotFound)

	// 列表（过滤）
	list, err := l.List(&types.GiftPackListRequest{GameId: "g1"}, auth)
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("list = %+v/%v", list, err)
	}
	if list, _ = l.List(&types.GiftPackListRequest{GameId: "nope"}, auth); len(list.Items) != 0 {
		t.Fatalf("list other = %+v", list.Items)
	}
	got, err := l.Get(&types.GiftPackPathRequest{Id: id}, auth)
	if err != nil || got.GiftPack.Name != "新手礼包·改" || got.GiftPack.PerUserLimit != 2 {
		t.Fatalf("get = %+v/%v", got, err)
	}

	// 码包发布前须有码 → 400（ensureStock）
	codeCfg := validPackCfg()
	codeCfg.CodeRequired = true
	codeResp, err := l.Create(&types.CreateGiftPackRequest{GiftPackConfig: codeCfg}, auth)
	if err != nil {
		t.Fatalf("create code pack: %v", err)
	}
	_, err = l.Publish(&types.GiftPackPathRequest{Id: codeResp.GiftPack.Id}, auth)
	mustAPIError(t, err, http.StatusBadRequest)

	// 导码：空列表 → 400；查无 → 404；正常（去重）
	codeId := codeResp.GiftPack.Id
	_, err = l.ImportCodes(&types.ImportCodesRequest{Id: codeId}, auth)
	mustAPIError(t, err, http.StatusBadRequest)
	_, err = l.ImportCodes(&types.ImportCodesRequest{Id: 9999, Codes: []string{"C"}}, auth)
	mustAPIError(t, err, http.StatusNotFound)
	imp, err := l.ImportCodes(&types.ImportCodesRequest{Id: codeId, Codes: []string{"C1", "C1", "  "}}, auth)
	if err != nil || imp.Imported != 1 || imp.Duplicate != 1 {
		t.Fatalf("import = %+v/%v", imp, err)
	}
	// 导码后可发布（ensureStock 命中）
	if _, err := l.Publish(&types.GiftPackPathRequest{Id: codeId}, auth); err != nil {
		t.Fatalf("publish code pack: %v", err)
	}
	// 停用后不可再导码 → 409
	if _, err := l.Disable(&types.GiftPackPathRequest{Id: codeId}, auth); err != nil {
		t.Fatalf("disable code pack: %v", err)
	}
	_, err = l.ImportCodes(&types.ImportCodesRequest{Id: codeId, Codes: []string{"C2"}}, auth)
	mustAPIError(t, err, http.StatusConflict)

	// 码列表（含状态筛）+ 查无 → 404
	codes, err := l.ListCodes(&types.CodeListRequest{Id: codeId}, auth)
	if err != nil || len(codes.Codes) != 1 {
		t.Fatalf("list codes = %+v/%v", codes, err)
	}
	if codes, _ = l.ListCodes(&types.CodeListRequest{Id: codeId, Status: "used"}, auth); len(codes.Codes) != 0 {
		t.Fatalf("list used codes = %+v", codes.Codes)
	}
	_, err = l.ListCodes(&types.CodeListRequest{Id: 9999}, auth)
	mustAPIError(t, err, http.StatusNotFound)

	// 领取记录列表（空）+ 管理面按包筛
	claims, err := l.ListClaims(&types.ClaimListAdminRequest{}, auth)
	if err != nil || len(claims.Claims) != 0 {
		t.Fatalf("list claims = %+v/%v", claims, err)
	}
	if claims, err = l.ListClaims(&types.ClaimListAdminRequest{PackId: 9999}, auth); err != nil || len(claims.Claims) != 0 {
		t.Fatalf("list claims by pack = %+v/%v", claims, err)
	}
}

// TestGiftPack_RoleArms 各管理方法的 requireAdminRole 非管理角色 403 臂。
func TestGiftPack_RoleArms(t *testing.T) {
	env := newLogicEnv(t, "")
	env.withCatalog(t, "operator", http.StatusOK)
	l := NewGiftPackLogic(adminCtx(7, "1.2.3.4"), env.sc)
	const auth = "Bearer t"
	resp, err := l.Create(&types.CreateGiftPackRequest{GiftPackConfig: validPackCfg()}, auth)
	if err != nil {
		t.Fatalf("seed create: %v", err)
	}
	id := resp.GiftPack.Id

	env.withCatalog(t, "player", http.StatusOK)
	calls := []struct {
		name string
		run  func() error
	}{
		{"update", func() error {
			_, e := l.Update(&types.UpdateGiftPackRequest{Id: id, GiftPackConfig: validPackCfg()}, auth)
			return e
		}},
		{"publish", func() error { _, e := l.Publish(&types.GiftPackPathRequest{Id: id}, auth); return e }},
		{"disable", func() error { _, e := l.Disable(&types.GiftPackPathRequest{Id: id}, auth); return e }},
		{"revise", func() error { _, e := l.Revise(&types.GiftPackPathRequest{Id: id}, auth); return e }},
		{"list", func() error { _, e := l.List(&types.GiftPackListRequest{}, auth); return e }},
		{"get", func() error { _, e := l.Get(&types.GiftPackPathRequest{Id: id}, auth); return e }},
		{"import", func() error {
			_, e := l.ImportCodes(&types.ImportCodesRequest{Id: id, Codes: []string{"C"}}, auth)
			return e
		}},
		{"listCodes", func() error { _, e := l.ListCodes(&types.CodeListRequest{Id: id}, auth); return e }},
		{"listClaims", func() error { _, e := l.ListClaims(&types.ClaimListAdminRequest{}, auth); return e }},
	}
	for _, c := range calls {
		if err := c.run(); err == nil {
			t.Fatalf("%s role = nil err", c.name)
		} else {
			mustAPIError(t, err, http.StatusForbidden)
		}
	}
}

func TestGiftPackPlayer_Arms(t *testing.T) {
	env := newLogicEnv(t, "")
	seedPack(t, env, validPackCfg(), nil)
	l := NewGiftPackLogic(adminCtx(7, "1.2.3.4"), env.sc)
	const auth = "Bearer t"

	// 未认证 → 401
	if _, err := l.PlayerList("g1", 1, ""); err == nil {
		t.Fatal("no auth list = nil err")
	} else {
		mustAPIError(t, err, http.StatusUnauthorized)
	}
	if _, err := l.PlayerGet(1, ""); err == nil {
		t.Fatal("no auth get = nil err")
	} else {
		mustAPIError(t, err, http.StatusUnauthorized)
	}
	if _, err := l.Claim(&types.ClaimGiftPackRequest{Id: 1}, ""); err == nil {
		t.Fatal("no auth claim = nil err")
	} else {
		mustAPIError(t, err, http.StatusUnauthorized)
	}
	if _, err := l.MyClaims(""); err == nil {
		t.Fatal("no auth myclaims = nil err")
	} else {
		mustAPIError(t, err, http.StatusUnauthorized)
	}

	// PlayerList：区服过滤 + 详情
	pk := seedPack(t, env, func() types.GiftPackConfig {
		cfg := validPackCfg()
		cfg.Name = "玩家礼包"
		return cfg
	}(), nil)
	if items, err := l.PlayerList("g1", 1, auth); err != nil || len(items.Items) == 0 {
		t.Fatalf("player list = %+v/%v", items, err)
	} else {
		if items.Items[0].AvailableCodes != 0 || items.Items[0].ClaimedCount != 0 {
			t.Fatalf("player list counters = %+v", items.Items[0])
		}
	}
	// 区服不在名单 → 过滤掉
	if items, _ := l.PlayerList("g1", 9, auth); len(items.Items) != 0 {
		t.Fatalf("player list other server = %+v", items.Items)
	}
	// 其他游戏 → 空
	if items, _ := l.PlayerList("nope", 1, auth); len(items.Items) != 0 {
		t.Fatalf("player list other game = %+v", items.Items)
	}
	// PlayerGet 命中 + 未上架 404 + 查无 404
	view, err := l.PlayerGet(pk, auth)
	if err != nil || view.Name != "玩家礼包" {
		t.Fatalf("player get = %+v/%v", view, err)
	}
	draft := validPackCfg()
	draft.Name = "草稿包"
	l2 := NewGiftPackLogic(adminCtx(7, "1.2.3.4"), env.sc)
	dr, err := l2.Create(&types.CreateGiftPackRequest{GiftPackConfig: draft}, auth)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	_, err = l.PlayerGet(dr.GiftPack.Id, auth) // 草稿 → 未上架 404
	mustAPIError(t, err, http.StatusNotFound)
	_, err = l.PlayerGet(9999, auth)
	mustAPIError(t, err, http.StatusNotFound)

	// MyClaims：空 + 有记录
	if cl, err := l.MyClaims(auth); err != nil || len(cl.Claims) != 0 {
		t.Fatalf("my claims empty = %+v/%v", cl, err)
	}
	req := &types.ClaimGiftPackRequest{Id: pk, ServerId: 1, RequestId: "r0"}
	if _, err := l.Claim(req, auth); err != nil {
		t.Fatalf("seed claim: %v", err)
	}
	if cl, err := l.MyClaims(auth); err != nil || len(cl.Claims) != 1 {
		t.Fatalf("my claims = %+v/%v", cl, err)
	}
}

func TestGiftPackClaim_Arms(t *testing.T) {
	env := newLogicEnv(t, "")
	// 幂等 + 码签发：码包 2 码，限领 2
	cfg := validPackCfg()
	cfg.PerUserLimit = 2
	cfg.CodeRequired = true
	id := seedPack(t, env, cfg, []string{"C1", "C2"})
	l := NewGiftPackLogic(adminCtx(7, "1.2.3.4"), env.sc)
	const auth = "Bearer t"

	// 幂等键缺失 → 400
	_, err := l.Claim(&types.ClaimGiftPackRequest{Id: id, ServerId: 1}, auth)
	mustAPIError(t, err, http.StatusBadRequest)
	// 查无 → 404
	_, err = l.Claim(&types.ClaimGiftPackRequest{Id: 9999, RequestId: "x"}, auth)
	mustAPIError(t, err, http.StatusNotFound)

	// 首领：签发 C1
	first, err := l.Claim(&types.ClaimGiftPackRequest{Id: id, ServerId: 1, RoleId: "r1", RequestId: "r1"}, auth)
	if err != nil || first.Code == nil || first.Code.Code != "C1" || first.Claim.ClaimIndex != 1 {
		t.Fatalf("first claim = %+v/%v", first, err)
	}
	// 幂等重放：同请求键 → 返回既有领取（不新增、不重复发码）
	replay, err := l.Claim(&types.ClaimGiftPackRequest{Id: id, ServerId: 1, RequestId: "r1"}, auth)
	if err != nil || replay.Claim.Id != first.Claim.Id || replay.Claim.RequestId != "r1" {
		t.Fatalf("replay claim = %+v/%v", replay, err)
	}
	if replay.Code == nil || replay.Code.Code != "C1" {
		t.Fatalf("replay code = %+v", replay.Code)
	}

	// 次领：签发 C2（claim_index=2）
	second, err := l.Claim(&types.ClaimGiftPackRequest{Id: id, ServerId: 1, RequestId: "r2"}, auth)
	if err != nil || second.Code == nil || second.Code.Code != "C2" || second.Claim.ClaimIndex != 2 {
		t.Fatalf("second claim = %+v/%v", second, err)
	}

	// 限领：已达 per_user_limit=2 → 409
	_, err = l.Claim(&types.ClaimGiftPackRequest{Id: id, ServerId: 1, RequestId: "r3"}, auth)
	mustAPIError(t, err, http.StatusConflict)

	// 管理面领取记录可见两条
	env.withCatalog(t, "operator", http.StatusOK)
	admin := NewGiftPackLogic(adminCtx(7, "1.2.3.4"), env.sc)
	if cl, err := admin.ListClaims(&types.ClaimListAdminRequest{PackId: id}, auth); err != nil || len(cl.Claims) != 2 {
		t.Fatalf("admin claims = %+v/%v", cl, err)
	}
}

func TestGiftPackClaim_RejectArms(t *testing.T) {
	env := newLogicEnv(t, "")
	env.withCatalog(t, "operator", http.StatusOK)
	l := NewGiftPackLogic(adminCtx(7, "1.2.3.4"), env.sc)
	const auth = "Bearer t"

	// 未上架（草稿）→ 409
	draftCfg := validPackCfg()
	draftCfg.Name = "草稿"
	l2 := NewGiftPackLogic(adminCtx(7, "1.2.3.4"), env.sc)
	dr, err := l2.Create(&types.CreateGiftPackRequest{GiftPackConfig: draftCfg}, auth)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	_, err = l.Claim(&types.ClaimGiftPackRequest{Id: dr.GiftPack.Id, RequestId: "r"}, auth)
	mustAPIError(t, err, http.StatusConflict)

	// 窗口外（过期包）→ 409
	past := validPackCfg()
	past.Name = "过期"
	past.StartAt, past.EndAt = "2020-01-01T00:00:00Z", "2020-12-31T00:00:00Z"
	pastId := seedPack(t, env, past, nil)
	_, err = l.Claim(&types.ClaimGiftPackRequest{Id: pastId, RequestId: "r"}, auth)
	mustAPIError(t, err, http.StatusConflict)

	// 区服不在范围 → 403
	id := seedPack(t, env, validPackCfg(), nil)
	_, err = l.Claim(&types.ClaimGiftPackRequest{Id: id, ServerId: 9, RequestId: "r"}, auth)
	mustAPIError(t, err, http.StatusForbidden)

	// 人群不匹配 → 403
	audCfg := validPackCfg()
	audCfg.Name = "定向"
	audCfg.Audience = map[string]any{"type": "new_user"}
	audId := seedPack(t, env, audCfg, nil)
	_, err = l.Claim(&types.ClaimGiftPackRequest{Id: audId, ServerId: 1, RequestId: "r"}, auth)
	mustAPIError(t, err, http.StatusForbidden)
	// 人群匹配 → 成功
	if cl, err := l.Claim(&types.ClaimGiftPackRequest{Id: audId, ServerId: 1, AudienceType: "new_user", RequestId: "aud"}, auth); err != nil {
		t.Fatalf("audience hit claim: %v", err)
	} else if cl.Claim.Id == 0 {
		t.Fatalf("audience claim = %+v", cl)
	}
}

func TestGiftPackClaim_TotalAndExhaustArms(t *testing.T) {
	// 全量上限：total_limit=1，同账号第二把（换请求键）→ 礼包已领完
	env := newLogicEnv(t, "")
	cfg := validPackCfg()
	cfg.PerUserLimit = 2
	cfg.TotalLimit = 1
	id := seedPack(t, env, cfg, nil)
	l := NewGiftPackLogic(adminCtx(7, "1.2.3.4"), env.sc)
	const auth = "Bearer t"
	if _, err := l.Claim(&types.ClaimGiftPackRequest{Id: id, ServerId: 1, RequestId: "t1"}, auth); err != nil {
		t.Fatalf("total claim 1: %v", err)
	}
	_, err := l.Claim(&types.ClaimGiftPackRequest{Id: id, ServerId: 1, RequestId: "t2"}, auth)
	mustAPIError(t, err, http.StatusConflict)

	// 码领完：码包 1 码限领 2 → 第二把签发失败回滚领取并报领完
	env2 := newLogicEnv(t, "")
	codeCfg := validPackCfg()
	codeCfg.PerUserLimit = 2
	codeCfg.CodeRequired = true
	codeId := seedPack(t, env2, codeCfg, []string{"X1"})
	l2 := NewGiftPackLogic(adminCtx(7, "1.2.3.4"), env2.sc)
	if c1, err := l2.Claim(&types.ClaimGiftPackRequest{Id: codeId, ServerId: 1, RequestId: "c1"}, auth); err != nil || c1.Code == nil {
		t.Fatalf("code claim 1 = %+v/%v", c1, err)
	}
	_, err = l2.Claim(&types.ClaimGiftPackRequest{Id: codeId, ServerId: 1, RequestId: "c2"}, auth)
	mustAPIError(t, err, http.StatusConflict)
	// 领取已回滚：限领消费仍为 1
	if cl, err := l2.MyClaims(auth); err != nil || len(cl.Claims) != 1 {
		t.Fatalf("after rollback claims = %+v/%v", cl, err)
	}
}

func TestGiftPackRedeem_Arms(t *testing.T) {
	env := newLogicEnv(t, "")
	cfg := validPackCfg()
	cfg.CodeRequired = true
	id := seedPack(t, env, cfg, []string{"R1"})
	l := NewGiftPackLogic(adminCtx(7, "1.2.3.4"), env.sc)
	const auth = "Bearer t"

	// 领取拿码
	claim, err := l.Claim(&types.ClaimGiftPackRequest{Id: id, ServerId: 1, RequestId: "rd"}, auth)
	if err != nil || claim.Code == nil {
		t.Fatalf("claim for redeem = %+v/%v", claim, err)
	}
	codeId := claim.Code.Id

	// 未配置密钥 → 403 fail-closed
	if err := l.Redeem([]byte(`{"code_id":1}`), ""); err == nil {
		t.Fatal("no secret = nil err")
	} else {
		mustAPIError(t, err, http.StatusForbidden)
	}
	env.withRewardSecret(t, "sec")

	// 签名无效 → 403
	if err := l.Redeem([]byte(`{"code_id":1}`), "deadbeef"); err == nil {
		t.Fatal("bad sig = nil err")
	} else {
		mustAPIError(t, err, http.StatusForbidden)
	}
	// 解析失败 → 400
	body, sig := signReward(t, "sec", `not-json`)
	if err := l.Redeem(body, sig); err == nil {
		t.Fatal("bad json = nil err")
	} else {
		mustAPIError(t, err, http.StatusBadRequest)
	}
	// code_id 缺失 → 400
	body, sig = signReward(t, "sec", map[string]any{"code_id": 0.0})
	if err := l.Redeem(body, sig); err == nil {
		t.Fatal("zero code = nil err")
	} else {
		mustAPIError(t, err, http.StatusBadRequest)
	}
	// 码不存在 → 404
	body, sig = signReward(t, "sec", map[string]any{"code_id": 9999.0})
	if err := l.Redeem(body, sig); err == nil {
		t.Fatal("missing code = nil err")
	} else {
		mustAPIError(t, err, http.StatusNotFound)
	}
	// 核销成功（issued → used）
	body, sig = signReward(t, "sec", map[string]any{"code_id": float64(codeId), "pack_id": float64(id)})
	if err := l.Redeem(body, sig); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	// 重复核销：幂等成功（nil）
	if err := l.Redeem(body, sig); err != nil {
		t.Fatalf("replay redeem: %v", err)
	}
	// 码状态已 used
	codes, err := l.ListCodes(&types.CodeListRequest{Id: id}, auth)
	if err != nil || codes.Codes[0].Status != "used" {
		t.Fatalf("used codes = %+v/%v", codes, err)
	}
}

func TestGiftPackHelpers_Arms(t *testing.T) {
	// packTargetHit：空名单全放行 / 非法 JSON / 命中 / 未命中
	empty := &model.GiftPack{ServerIds: ""}
	if !packTargetHit(empty, 9) {
		t.Fatal("empty server ids should hit")
	}
	bad := &model.GiftPack{ServerIds: "{bad"}
	if packTargetHit(bad, 1) {
		t.Fatal("bad json should miss")
	}
	zero := &model.GiftPack{ServerIds: "[]"}
	if !packTargetHit(zero, 9) {
		t.Fatal("zero length should hit")
	}
	hit := &model.GiftPack{ServerIds: "[1,2]"}
	if !packTargetHit(hit, 1) || packTargetHit(hit, 9) {
		t.Fatal("list match wrong")
	}

	// packAudienceHit：非法/空 JSON 放行、all/空 type 放行、定向失配 403、匹配放行
	if err := packAudienceHit(&model.GiftPack{Audience: "{bad"}, ""); err != nil {
		t.Fatalf("bad audience = %v", err)
	}
	if err := packAudienceHit(&model.GiftPack{Audience: ""}, ""); err != nil {
		t.Fatalf("empty audience = %v", err)
	}
	if err := packAudienceHit(&model.GiftPack{Audience: `{"type":"all"}`}, ""); err != nil {
		t.Fatalf("all audience = %v", err)
	}
	if err := packAudienceHit(&model.GiftPack{Audience: `{"x":1}`}, ""); err != nil {
		t.Fatalf("no type audience = %v", err)
	}
	if err := packAudienceHit(&model.GiftPack{Audience: `{"type":"new_user"}`}, ""); err == nil {
		t.Fatal("audience mismatch = nil err")
	} else {
		mustAPIError(t, err, http.StatusForbidden)
	}
	if err := packAudienceHit(&model.GiftPack{Audience: `{"type":"new_user"}`}, " new_user "); err != nil {
		t.Fatalf("audience hit = %v", err)
	}

	// apiError.Error / bonusKey 直测（纯函数臂）
	if newAPIError(http.StatusTeapot, "msg").Error() != "msg" {
		t.Fatal("apiError text wrong")
	}
	if k1, k2 := (&candidate{ExtraQuantity: 5, ActivityId: 2}).bonusKey(); k1 != 5 || k2 != -2 {
		t.Fatalf("bonusKey = %d/%d", k1, k2)
	}
}

func TestGiftPack_DBErrorArms(t *testing.T) {
	env := newLogicEnv(t, "")
	seedPack(t, env, validPackCfg(), nil)
	env.withCatalog(t, "operator", http.StatusOK)
	l := NewGiftPackLogic(adminCtx(7, "1.2.3.4"), env.sc)
	const auth = "Bearer t"

	// DROP 三表 → 各读路径真实错误 → 500（closed DB 只能到 ErrNoRows/404）
	for _, tbl := range []string{"gift_packs", "gift_pack_codes", "gift_pack_claims"} {
		if _, err := env.db.Exec(`DROP TABLE ` + tbl); err != nil {
			t.Fatalf("drop %s: %v", tbl, err)
		}
	}
	if _, err := l.Create(&types.CreateGiftPackRequest{GiftPackConfig: validPackCfg()}, auth); err == nil {
		t.Fatal("drop create = nil err")
	} else {
		mustAPIError(t, err, http.StatusInternalServerError)
	}
	if _, err := l.Update(&types.UpdateGiftPackRequest{Id: 1, GiftPackConfig: validPackCfg()}, auth); err == nil {
		t.Fatal("drop update = nil err")
	} else {
		mustAPIError(t, err, http.StatusInternalServerError)
	}
	if _, err := l.Publish(&types.GiftPackPathRequest{Id: 1}, auth); err == nil {
		t.Fatal("drop publish = nil err")
	} else {
		mustAPIError(t, err, http.StatusInternalServerError)
	}
	if _, err := l.Disable(&types.GiftPackPathRequest{Id: 1}, auth); err == nil {
		t.Fatal("drop disable = nil err")
	} else {
		mustAPIError(t, err, http.StatusInternalServerError)
	}
	if _, err := l.Revise(&types.GiftPackPathRequest{Id: 1}, auth); err == nil {
		t.Fatal("drop revise = nil err")
	} else {
		mustAPIError(t, err, http.StatusInternalServerError)
	}
	if _, err := l.List(&types.GiftPackListRequest{}, auth); err == nil {
		t.Fatal("drop list = nil err")
	} else {
		mustAPIError(t, err, http.StatusInternalServerError)
	}
	if _, err := l.Get(&types.GiftPackPathRequest{Id: 1}, auth); err == nil {
		t.Fatal("drop get = nil err")
	} else {
		mustAPIError(t, err, http.StatusInternalServerError)
	}
	if _, err := l.ImportCodes(&types.ImportCodesRequest{Id: 1, Codes: []string{"C"}}, auth); err == nil {
		t.Fatal("drop import = nil err")
	} else {
		mustAPIError(t, err, http.StatusInternalServerError)
	}
	if _, err := l.ListCodes(&types.CodeListRequest{Id: 1}, auth); err == nil {
		t.Fatal("drop list codes = nil err")
	} else {
		mustAPIError(t, err, http.StatusInternalServerError)
	}
	if _, err := l.ListClaims(&types.ClaimListAdminRequest{}, auth); err == nil {
		t.Fatal("drop list claims = nil err")
	} else {
		mustAPIError(t, err, http.StatusInternalServerError)
	}
	if _, err := l.PlayerList("g1", 1, auth); err == nil {
		t.Fatal("drop player list = nil err")
	} else {
		mustAPIError(t, err, http.StatusInternalServerError)
	}
	if _, err := l.PlayerGet(1, auth); err == nil {
		t.Fatal("drop player get = nil err")
	} else {
		mustAPIError(t, err, http.StatusInternalServerError)
	}
	if _, err := l.Claim(&types.ClaimGiftPackRequest{Id: 1, RequestId: "r"}, auth); err == nil {
		t.Fatal("drop claim = nil err")
	} else {
		mustAPIError(t, err, http.StatusInternalServerError)
	}
	if _, err := l.MyClaims(auth); err == nil {
		t.Fatal("drop my claims = nil err")
	} else {
		mustAPIError(t, err, http.StatusInternalServerError)
	}
	if err := l.Redeem([]byte(`{"code_id":1}`), ""); err == nil {
		t.Fatal("drop redeem = nil err")
	}
}

func TestGiftPack_ImportCodesErrorArms(t *testing.T) {
	env := newLogicEnv(t, "")
	seedPack(t, env, validPackCfg(), nil)
	env.withCatalog(t, "operator", http.StatusOK)
	l := NewGiftPackLogic(adminCtx(7, "1.2.3.4"), env.sc)
	// 只 DROP 码表：Get 成功后 ImportCodes 真实错误 → 500
	if _, err := env.db.Exec(`DROP TABLE gift_pack_codes`); err != nil {
		t.Fatalf("drop gift_pack_codes: %v", err)
	}
	if _, err := l.ImportCodes(&types.ImportCodesRequest{Id: 1, Codes: []string{"C"}}, "Bearer t"); err == nil {
		t.Fatal("drop import = nil err")
	} else {
		mustAPIError(t, err, http.StatusInternalServerError)
	}
}

func TestGiftPack_ClaimQueryErrorArms(t *testing.T) {
	env := newLogicEnv(t, "")
	id := seedPack(t, env, validPackCfg(), nil)
	// 只 DROP 领取表：Get 成功后 GetClaimByRequest 真实错误 → 500
	if _, err := env.db.Exec(`DROP TABLE gift_pack_claims`); err != nil {
		t.Fatalf("drop gift_pack_claims: %v", err)
	}
	l := NewGiftPackLogic(adminCtx(7, "1.2.3.4"), env.sc)
	_, err := l.Claim(&types.ClaimGiftPackRequest{Id: id, ServerId: 1, RequestId: "q"}, "Bearer t")
	mustAPIError(t, err, http.StatusInternalServerError)
}
