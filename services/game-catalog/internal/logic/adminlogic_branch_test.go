package logic

// adminlogic 补漏分支：toJSON 边界、AssignOwnership 查询失败 500、
// DecideReview 状态同步失败各臂、上线/版本通知空受众臂、RevokeVersion
// 版本校验失败传播、Me 空 ctx 角色回退、clampPage 边界。

import (
	"errors"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"
)

// toJSON：nil 入参 / 不可序列化入参 → ""。
func TestAdminLogic_ToJSONEdges(t *testing.T) {
	if got := toJSON(nil); got != "" {
		t.Fatalf("toJSON(nil) = %q, want empty", got)
	}
	if got := toJSON(func() {}); got != "" {
		t.Fatalf("toJSON(func) = %q, want empty", got)
	}
	if got := toJSON(map[string]int{"a": 1}); got == "" {
		t.Fatal("toJSON(map) should not be empty")
	}
}

// AssignOwnership：游戏/合作方查询通用失败 → 500。
func TestAdminLogic_AssignOwnershipQueryFailed(t *testing.T) {
	t.Run("game query failed", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.GameRepository = &failingGameStore{
			GameStore: svcCtx.GameRepository,
			getErr:    errors.New("game db down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.AssignOwnership(&types.AssignOwnershipRequest{Id: "g-rpg-1", PartnerId: 1})
		requireAPIStatus(t, err, 500)
	})
	t.Run("partner query failed", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:    svcCtx.AdminRepository,
			getPartnerErr: errors.New("partner db down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.AssignOwnership(&types.AssignOwnershipRequest{Id: "g-rpg-1", PartnerId: 1})
		requireAPIStatus(t, err, 500)
	})
}

// DecideReview 审结后状态同步失败 → 500（过审前查游戏/写游戏状态、
// 驳回写游戏状态、驳回版本写版本状态四条臂）。
func TestAdminLogic_DecideReviewStatusSyncFailed(t *testing.T) {
	t.Run("approve game query failed", func(t *testing.T) {
		svcCtx, reviewID, _ := newAdminLogicCtx(t)
		svcCtx.GameRepository = &failingGameStore{
			GameStore: svcCtx.GameRepository,
			getErr:    errors.New("game db down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.DecideReview(&types.DecideReviewRequest{Id: reviewID, Decision: model.ReviewApprove})
		requireAPIStatus(t, err, 500)
	})
	t.Run("approve game set status failed", func(t *testing.T) {
		svcCtx, reviewID, _ := newAdminLogicCtx(t)
		svcCtx.GameRepository = &failingGameStore{
			GameStore:    svcCtx.GameRepository,
			setStatusErr: errors.New("set status down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.DecideReview(&types.DecideReviewRequest{Id: reviewID, Decision: model.ReviewApprove})
		requireAPIStatus(t, err, 500)
	})
	t.Run("reject game set status failed", func(t *testing.T) {
		svcCtx, reviewID, _ := newAdminLogicCtx(t)
		svcCtx.GameRepository = &failingGameStore{
			GameStore:    svcCtx.GameRepository,
			setStatusErr: errors.New("set status down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.DecideReview(&types.DecideReviewRequest{Id: reviewID, Decision: model.ReviewReject, Reason: "不合规"})
		requireAPIStatus(t, err, 500)
	})
	t.Run("reject version set status failed", func(t *testing.T) {
		svcCtx, _, versionID := newAdminLogicCtx(t)
		admin := svcCtx.AdminRepository
		r, err := admin.SubmitReview("g-rpg-1", versionID, 9)
		if err != nil {
			t.Fatalf("SubmitReview version: %v", err)
		}
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:          svcCtx.AdminRepository,
			setVersionStatusErr: errors.New("version status down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err = l.DecideReview(&types.DecideReviewRequest{Id: r.Id, Decision: model.ReviewReject, Reason: "回退"})
		requireAPIStatus(t, err, 500)
	})
}

// notifyGameLaunch/notifyGameVersion：nil 守卫与空受众（启用通知但无
// 收藏/预约人群）→ 不扇出、不报错。
func TestAdminLogic_NotifyEmptyAudience(t *testing.T) {
	svcCtx, _, _ := newAdminLogicCtx(t)
	svcCtx.Notify = &fakePublisher{enabled: true}
	l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)

	l.notifyGameLaunch(nil, nil) // nil 守卫

	before := &model.Game{Id: "g-rpg-1", Title: "RPG", Status: model.GameStatusDraft}
	after := &model.Game{Id: "g-rpg-1", Title: "RPG", Status: model.GameStatusApproved}
	l.notifyGameLaunch(before, after)                                    // 上架过渡但无收藏人群
	l.notifyGameVersion("g-rpg-1", &model.GameVersion{Version: "1.0.0"}) // 无预约人群

	if p := svcCtx.Notify.(*fakePublisher); p.calls != 0 {
		t.Fatalf("publish calls = %d, want 0 (empty audience)", p.calls)
	}
}

// RevokeVersion：版本不存在 → versionOfGame 404 原样传播。
func TestAdminLogic_RevokeVersionNotFound(t *testing.T) {
	svcCtx, _, _ := newAdminLogicCtx(t)
	l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
	_, err := l.RevokeVersion("g-rpg-1", "no-such-version")
	requireAPIStatus(t, err, 404)
}

// Me：ctx 未带角色 → 回退库中角色，响应仍成功。
func TestAdminLogic_MeRoleFallback(t *testing.T) {
	svcCtx, _, _ := newAdminLogicCtx(t)
	l := NewAdminLogic(adminCtx(1, ""), svcCtx)
	resp, err := l.Me()
	if err != nil || resp == nil {
		t.Fatalf("Me with empty ctx role: resp=%+v err=%v", resp, err)
	}
	if resp.Role == "" {
		t.Fatal("Me role should fall back to db role")
	}
}

// clampPage：limit 超上限截断；offset 负数归零。
func TestClampPage(t *testing.T) {
	if l, o := clampPage(150, 0, 100); l != 100 || o != 0 {
		t.Fatalf("clampPage(150,0,100) = (%d,%d), want (100,0)", l, o)
	}
	if l, o := clampPage(20, -5, 100); l != 20 || o != 0 {
		t.Fatalf("clampPage(20,-5,100) = (%d,%d), want (20,0)", l, o)
	}
}
