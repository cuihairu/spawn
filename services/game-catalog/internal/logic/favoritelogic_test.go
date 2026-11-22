package logic

import (
	"errors"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"
)

// failingFavoriteStore GP-401 收藏 logic 错误分支注入：内嵌真实仓储，仅按
// 字段覆写指定方法。
type failingFavoriteStore struct {
	model.AdminStore
	addFavErr    error
	removeFavErr error
	listMineErr  error
	listMineRows []*model.GameFavorite
}

var _ model.AdminStore = (*failingFavoriteStore)(nil)

func (f *failingFavoriteStore) AddFavorite(gameId string, userId int64) (*model.GameFavorite, bool, error) {
	if f.addFavErr != nil {
		return nil, false, f.addFavErr
	}
	return f.AdminStore.AddFavorite(gameId, userId)
}

func (f *failingFavoriteStore) RemoveFavorite(gameId string, userId int64) error {
	if f.removeFavErr != nil {
		return f.removeFavErr
	}
	return f.AdminStore.RemoveFavorite(gameId, userId)
}

func (f *failingFavoriteStore) ListMyFavorites(userId int64, limit int) ([]*model.GameFavorite, error) {
	if f.listMineErr != nil {
		return nil, f.listMineErr
	}
	if f.listMineRows != nil {
		return f.listMineRows, nil
	}
	return f.AdminStore.ListMyFavorites(userId, limit)
}

// 收藏：未登录 → 401；游戏查不到 → 404；查询失败 → 500；未上架 → 404；
// 落库失败 → 500；成功幂等。
func TestFavoriteLogic_FavoriteErrors(t *testing.T) {
	t.Run("unauthorized 401", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		l := NewFavoriteLogic(reviewCtx(0), svcCtx)
		_, err := l.Favorite(&types.IdPathRequest{Id: "g-rpg-1"})
		requireAPIStatus(t, err, 401)
	})
	t.Run("game not found 404", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		svcCtx.GameRepository = &failingGameStore{
			GameStore: svcCtx.GameRepository,
			getErr:    model.ErrGameNotFound,
		}
		l := NewFavoriteLogic(reviewCtx(7), svcCtx)
		_, err := l.Favorite(&types.IdPathRequest{Id: "g-ghost"})
		requireAPIStatus(t, err, 404)
	})
	t.Run("query failed 500", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		svcCtx.GameRepository = &failingGameStore{
			GameStore: svcCtx.GameRepository,
			getErr:    errors.New("query down"),
		}
		l := NewFavoriteLogic(reviewCtx(7), svcCtx)
		_, err := l.Favorite(&types.IdPathRequest{Id: "g-rpg-1"})
		requireAPIStatus(t, err, 500)
	})
	t.Run("not approved 404", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		if _, err := svcCtx.GameRepository.SetStatus("g-act-1", model.GameStatusOffline, ""); err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
		l := NewFavoriteLogic(reviewCtx(7), svcCtx)
		_, err := l.Favorite(&types.IdPathRequest{Id: "g-act-1"})
		requireAPIStatus(t, err, 404)
	})
	t.Run("add failed 500", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		if _, err := svcCtx.GameRepository.SetStatus("g-rpg-1", model.GameStatusApproved, ""); err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
		svcCtx.AdminRepository = &failingFavoriteStore{
			AdminStore: svcCtx.AdminRepository,
			addFavErr:  errors.New("fav down"),
		}
		l := NewFavoriteLogic(reviewCtx(7), svcCtx)
		_, err := l.Favorite(&types.IdPathRequest{Id: "g-rpg-1"})
		requireAPIStatus(t, err, 500)
	})
	t.Run("success", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		if _, err := svcCtx.GameRepository.SetStatus("g-rpg-1", model.GameStatusApproved, ""); err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
		l := NewFavoriteLogic(reviewCtx(7), svcCtx)
		resp, err := l.Favorite(&types.IdPathRequest{Id: "g-rpg-1"})
		if err != nil || !resp.Created {
			t.Fatalf("resp=%+v err=%v", resp, err)
		}
	})
}

// 取消收藏：未登录 → 401；取消失败 → 500。
func TestFavoriteLogic_UnfavoriteErrors(t *testing.T) {
	t.Run("unauthorized 401", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		l := NewFavoriteLogic(reviewCtx(0), svcCtx)
		if err := l.Unfavorite(&types.IdPathRequest{Id: "g-rpg-1"}); err != nil {
			requireAPIStatus(t, err, 401)
		} else {
			t.Fatal("want 401")
		}
	})
	t.Run("remove failed 500", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		svcCtx.AdminRepository = &failingFavoriteStore{
			AdminStore:   svcCtx.AdminRepository,
			removeFavErr: errors.New("remove down"),
		}
		l := NewFavoriteLogic(reviewCtx(7), svcCtx)
		if err := l.Unfavorite(&types.IdPathRequest{Id: "g-rpg-1"}); err != nil {
			requireAPIStatus(t, err, 500)
		} else {
			t.Fatal("want 500")
		}
	})
	t.Run("success", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		l := NewFavoriteLogic(reviewCtx(7), svcCtx)
		if err := l.Unfavorite(&types.IdPathRequest{Id: "g-rpg-1"}); err != nil {
			t.Fatalf("unfavorite: %v", err)
		}
	})
}

// 我的收藏：未登录 → 401；查询失败 → 500；游戏已删的收藏行跳过不阻断。
func TestFavoriteLogic_MyFavoritesErrors(t *testing.T) {
	t.Run("unauthorized 401", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		l := NewFavoriteLogic(reviewCtx(0), svcCtx)
		if _, err := l.MyFavorites(); err != nil {
			requireAPIStatus(t, err, 401)
		} else {
			t.Fatal("want 401")
		}
	})
	t.Run("list failed 500", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		svcCtx.AdminRepository = &failingFavoriteStore{
			AdminStore:  svcCtx.AdminRepository,
			listMineErr: errors.New("list down"),
		}
		l := NewFavoriteLogic(reviewCtx(7), svcCtx)
		if _, err := l.MyFavorites(); err != nil {
			requireAPIStatus(t, err, 500)
		} else {
			t.Fatal("want 500")
		}
	})
	t.Run("deleted game row skipped", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		svcCtx.AdminRepository = &failingFavoriteStore{
			AdminStore:   svcCtx.AdminRepository,
			listMineRows: []*model.GameFavorite{{GameId: "g-ghost", UserId: 7}},
		}
		l := NewFavoriteLogic(reviewCtx(7), svcCtx)
		resp, err := l.MyFavorites()
		if err != nil {
			t.Fatalf("want success: %v", err)
		}
		if len(resp.Items) != 0 {
			t.Fatalf("items = %+v, want empty", resp.Items)
		}
	})
}
