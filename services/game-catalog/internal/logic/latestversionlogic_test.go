package logic

import (
	"errors"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/model"
)

// failingLatestStore GP-105 最新版本 logic 错误分支注入。
type failingLatestStore struct {
	model.AdminStore
	getLatestErr error
}

var _ model.AdminStore = (*failingLatestStore)(nil)

func (f *failingLatestStore) GetLatestPublishedVersion(gameId string) (*model.GameVersion, error) {
	if f.getLatestErr != nil {
		return nil, f.getLatestErr
	}
	return f.AdminStore.GetLatestPublishedVersion(gameId)
}

// Latest：游戏查不到 → 404；查询失败 → 500；暂无已发布 → 404；查询版本失败 → 500；
// 成功置空 storage_url。
func TestLatestVersionLogic_LatestErrors(t *testing.T) {
	t.Run("game not found 404", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		svcCtx.GameRepository = &failingGameStore{
			GameStore: svcCtx.GameRepository,
			getErr:    model.ErrGameNotFound,
		}
		l := NewLatestVersionLogic(reviewCtx(7), svcCtx)
		if _, err := l.Latest("g-ghost"); err != nil {
			requireAPIStatus(t, err, 404)
		} else {
			t.Fatal("want 404")
		}
	})
	t.Run("query failed 500", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		svcCtx.GameRepository = &failingGameStore{
			GameStore: svcCtx.GameRepository,
			getErr:    errors.New("query down"),
		}
		l := NewLatestVersionLogic(reviewCtx(7), svcCtx)
		if _, err := l.Latest("g-rpg-1"); err != nil {
			requireAPIStatus(t, err, 500)
		} else {
			t.Fatal("want 500")
		}
	})
	t.Run("no published 404", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		svcCtx.AdminRepository = &failingLatestStore{
			AdminStore:   svcCtx.AdminRepository,
			getLatestErr: model.ErrVersionNotFound,
		}
		l := NewLatestVersionLogic(reviewCtx(7), svcCtx)
		if _, err := l.Latest("g-rpg-1"); err != nil {
			requireAPIStatus(t, err, 404)
		} else {
			t.Fatal("want 404")
		}
	})
	t.Run("version query failed 500", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		svcCtx.AdminRepository = &failingLatestStore{
			AdminStore:   svcCtx.AdminRepository,
			getLatestErr: errors.New("query down"),
		}
		l := NewLatestVersionLogic(reviewCtx(7), svcCtx)
		if _, err := l.Latest("g-rpg-1"); err != nil {
			requireAPIStatus(t, err, 500)
		} else {
			t.Fatal("want 500")
		}
	})
	t.Run("success hides storage url", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		admin := svcCtx.AdminRepository
		v, err := admin.CreateVersion(&model.GameVersion{GameId: "g-rpg-1", Version: "1.0.0", VersionCode: 1, StorageUrl: "pkg://ver-x.apk"})
		if err != nil {
			t.Fatalf("CreateVersion: %v", err)
		}
		if _, err := admin.SetVersionStatus(v.Id, model.VersionStatusApproved, ""); err != nil {
			t.Fatalf("SetVersionStatus: %v", err)
		}
		if _, err := admin.PublishVersion(v.Id); err != nil {
			t.Fatalf("PublishVersion: %v", err)
		}
		l := NewLatestVersionLogic(reviewCtx(7), svcCtx)
		resp, err := l.Latest("g-rpg-1")
		if err != nil || resp.Version.StorageUrl != "" {
			t.Fatalf("resp=%+v err=%v", resp, err)
		}
	})
}
