package logic

import (
	"errors"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"
)

// 游戏详情：查不到 → 404；未上架 → 404；评分聚合失败 → 500。
func TestGetGameDetailLogic_Errors(t *testing.T) {
	t.Run("not found 404", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		svcCtx.GameRepository = &failingGameStore{
			GameStore: svcCtx.GameRepository,
			getErr:    model.ErrGameNotFound,
		}
		l := NewGetGameDetailLogic(reviewCtx(7), svcCtx)
		if _, err := l.GetGameDetail(&types.GetGameDetailRequest{Id: "g-ghost"}); err != nil {
			requireAPIStatus(t, err, 404)
		} else {
			t.Fatal("want 404")
		}
	})
	t.Run("not approved 404", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		if _, err := svcCtx.GameRepository.SetStatus("g-act-1", model.GameStatusOffline, ""); err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
		l := NewGetGameDetailLogic(reviewCtx(7), svcCtx)
		if _, err := l.GetGameDetail(&types.GetGameDetailRequest{Id: "g-act-1"}); err != nil {
			requireAPIStatus(t, err, 404)
		} else {
			t.Fatal("want 404")
		}
	})
	t.Run("rating summary failed 500", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		if _, err := svcCtx.GameRepository.SetStatus("g-rpg-1", model.GameStatusApproved, ""); err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:          svcCtx.AdminRepository,
			getRatingSummaryErr: errors.New("summary down"),
		}
		l := NewGetGameDetailLogic(reviewCtx(7), svcCtx)
		if _, err := l.GetGameDetail(&types.GetGameDetailRequest{Id: "g-rpg-1"}); err != nil {
			requireAPIStatus(t, err, 500)
		} else {
			t.Fatal("want 500")
		}
	})
}

// 公开区服列表：未上架 → 404；查询失败 → 500。
func TestGetGameDetailLogic_ListServersErrors(t *testing.T) {
	t.Run("not approved 404", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		if _, err := svcCtx.GameRepository.SetStatus("g-act-1", model.GameStatusOffline, ""); err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
		l := NewGetGameDetailLogic(reviewCtx(7), svcCtx)
		if _, err := l.ListServers(&types.GetGameDetailRequest{Id: "g-act-1"}); err != nil {
			requireAPIStatus(t, err, 404)
		} else {
			t.Fatal("want 404")
		}
	})
	t.Run("list failed 500", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		if _, err := svcCtx.GameRepository.SetStatus("g-rpg-1", model.GameStatusApproved, ""); err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:     svcCtx.AdminRepository,
			listServersErr: errors.New("list down"),
		}
		l := NewGetGameDetailLogic(reviewCtx(7), svcCtx)
		if _, err := l.ListServers(&types.GetGameDetailRequest{Id: "g-rpg-1"}); err != nil {
			requireAPIStatus(t, err, 500)
		} else {
			t.Fatal("want 500")
		}
	})
}
