package logic

import (
	"errors"
	"net/http"
	"testing"

	"github.com/tappi/tappi/services/content-service/internal/types"
	"github.com/tappi/tappi/services/content-service/model"
)

// failingFavoriteStore 按方法投毒的收藏仓储（未投毒方法回落真实实现）。
type failingFavoriteStore struct {
	model.FavoriteStore
	addErr, removeErr, existsErr, countErr, listErr error
}

func (s failingFavoriteStore) Add(userId, guideId int64) error {
	return s.addErr
}

func (s failingFavoriteStore) Remove(userId, guideId int64) error {
	return s.removeErr
}

func (s failingFavoriteStore) Exists(userId, guideId int64) (bool, error) {
	if s.existsErr != nil {
		return false, s.existsErr
	}
	return s.FavoriteStore.Exists(userId, guideId)
}

func (s failingFavoriteStore) CountByGuide(guideId int64) (int64, error) {
	if s.countErr != nil {
		return 0, s.countErr
	}
	return s.FavoriteStore.CountByGuide(guideId)
}

func (s failingFavoriteStore) ListByUser(userId int64, limit, offset int) ([]*model.Guide, int, error) {
	return nil, 0, s.listErr
}

// failingGuideStore 仅 Get 投毒的攻略仓储（其余方法回落真实实现）。
type failingGuideStore struct {
	model.GuideStore
	getErr error
}

func (s failingGuideStore) Get(id int64) (*model.Guide, error) {
	return nil, s.getErr
}

// --- 计数失败降级为 0（收藏/取消收藏） ---

func TestFavoriteGuide_CountFailureDegradesToZero(t *testing.T) {
	svcCtx := newFavoriteSvcCtx(t)
	svcCtx.FavoriteRepository = failingFavoriteStore{
		FavoriteStore: svcCtx.FavoriteRepository, countErr: errors.New("disk on fire"),
	}

	resp, err := NewFavoriteGuideLogic(userCtx(2, "u2"), svcCtx).
		FavoriteGuide(&types.FavoriteGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusOK {
		t.Fatalf("resp=%+v err=%v, want 200", resp, err)
	}
	if resp.Count != 0 {
		t.Fatalf("count on failure = %d, want degraded 0", resp.Count)
	}
}

func TestUnfavoriteGuide_CountFailureDegradesToZero(t *testing.T) {
	svcCtx := newFavoriteSvcCtx(t)
	svcCtx.FavoriteRepository = failingFavoriteStore{
		FavoriteStore: svcCtx.FavoriteRepository, countErr: errors.New("disk on fire"),
	}

	resp, err := NewUnfavoriteGuideLogic(userCtx(2, "u2"), svcCtx).
		UnfavoriteGuide(&types.FavoriteGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusOK || resp.Favorited || resp.Count != 0 {
		t.Fatalf("resp=%+v err=%v, want 200 unfavorited count=0", resp, err)
	}
}

// --- 写操作失败 → 500 ---

func TestFavoriteGuide_AddFailureIsInternal(t *testing.T) {
	svcCtx := newFavoriteSvcCtx(t)
	svcCtx.FavoriteRepository = failingFavoriteStore{
		FavoriteStore: svcCtx.FavoriteRepository, addErr: errors.New("disk on fire"),
	}

	resp, err := NewFavoriteGuideLogic(userCtx(2, "u2"), svcCtx).
		FavoriteGuide(&types.FavoriteGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusInternalServerError || resp.Message != "收藏失败" {
		t.Fatalf("resp=%+v err=%v, want 500 收藏失败", resp, err)
	}
}

func TestUnfavoriteGuide_RemoveFailureIsInternal(t *testing.T) {
	svcCtx := newFavoriteSvcCtx(t)
	svcCtx.FavoriteRepository = failingFavoriteStore{
		FavoriteStore: svcCtx.FavoriteRepository, removeErr: errors.New("disk on fire"),
	}

	resp, err := NewUnfavoriteGuideLogic(userCtx(2, "u2"), svcCtx).
		UnfavoriteGuide(&types.FavoriteGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusInternalServerError || resp.Message != "取消收藏失败" {
		t.Fatalf("resp=%+v err=%v, want 500 取消收藏失败", resp, err)
	}
}

// --- 读攻略失败 → 500（区别于不存在 404） ---

func TestFavoriteGuide_GuideGetFailureIsInternal(t *testing.T) {
	svcCtx := newFavoriteSvcCtx(t)
	svcCtx.GuideRepository = failingGuideStore{GuideStore: svcCtx.GuideRepository, getErr: errors.New("db gone")}

	resp, err := NewFavoriteGuideLogic(userCtx(2, "u2"), svcCtx).
		FavoriteGuide(&types.FavoriteGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusInternalServerError || resp.Message != "收藏失败" {
		t.Fatalf("resp=%+v err=%v, want 500 收藏失败", resp, err)
	}
}

func TestUnfavoriteGuide_GuideGetFailureIsInternal(t *testing.T) {
	svcCtx := newFavoriteSvcCtx(t)
	svcCtx.GuideRepository = failingGuideStore{GuideStore: svcCtx.GuideRepository, getErr: errors.New("db gone")}

	resp, err := NewUnfavoriteGuideLogic(userCtx(2, "u2"), svcCtx).
		UnfavoriteGuide(&types.FavoriteGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusInternalServerError || resp.Message != "取消收藏失败" {
		t.Fatalf("resp=%+v err=%v, want 500 取消收藏失败", resp, err)
	}
}

// --- 状态查询失败 → 500 ---

func TestGetFavoriteStatus_CountFailureIsInternal(t *testing.T) {
	svcCtx := newFavoriteSvcCtx(t)
	svcCtx.FavoriteRepository = failingFavoriteStore{
		FavoriteStore: svcCtx.FavoriteRepository, countErr: errors.New("disk on fire"),
	}

	resp, err := NewGetFavoriteStatusLogic(userCtx(2, "u2"), svcCtx).
		GetFavoriteStatus(&types.FavoriteGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusInternalServerError || resp.Message != "查询收藏状态失败" {
		t.Fatalf("resp=%+v err=%v, want 500 查询收藏状态失败", resp, err)
	}
}

func TestGetFavoriteStatus_ExistsFailureIsInternal(t *testing.T) {
	svcCtx := newFavoriteSvcCtx(t)
	svcCtx.FavoriteRepository = failingFavoriteStore{
		FavoriteStore: svcCtx.FavoriteRepository, existsErr: errors.New("disk on fire"),
	}

	resp, err := NewGetFavoriteStatusLogic(userCtx(2, "u2"), svcCtx).
		GetFavoriteStatus(&types.FavoriteGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusInternalServerError || resp.Message != "查询收藏状态失败" {
		t.Fatalf("resp=%+v err=%v, want 500 查询收藏状态失败", resp, err)
	}
}

// --- 收藏列表失败 → 500 ---

func TestListFavorites_ListFailureIsInternal(t *testing.T) {
	svcCtx := newFavoriteSvcCtx(t)
	svcCtx.FavoriteRepository = failingFavoriteStore{
		FavoriteStore: svcCtx.FavoriteRepository, listErr: errors.New("disk on fire"),
	}

	resp, err := NewListFavoritesLogic(userCtx(2, "u2"), svcCtx).
		ListFavorites(&types.ListFavoritesRequest{})
	if err != nil || resp.Code != http.StatusInternalServerError || resp.Message != "获取收藏列表失败" {
		t.Fatalf("resp=%+v err=%v, want 500 获取收藏列表失败", resp, err)
	}
}
