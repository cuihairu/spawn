package logic

import (
	"errors"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"
)

// UpdateGameTags：标签数超 20 → 400；单标签超长 → 400；更新失败（哨兵/通用）
// → 404/500；审计写入失败仅记日志不阻断。
func TestUpdateGameTagsLogic_Errors(t *testing.T) {
	t.Run("too many tags 400", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		tags := make([]string, 21)
		for i := range tags {
			tags[i] = "t"
		}
		l := NewUpdateGameTagsLogic(reviewCtx(1), svcCtx)
		_, err := l.UpdateGameTags(&types.UpdateGameTagsRequest{Id: "g-rpg-1", Tags: tags})
		requireAPIStatus(t, err, 400)
	})
	t.Run("long tag 400", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		long := make([]rune, 33)
		for i := range long {
			long[i] = 'a'
		}
		l := NewUpdateGameTagsLogic(reviewCtx(1), svcCtx)
		_, err := l.UpdateGameTags(&types.UpdateGameTagsRequest{Id: "g-rpg-1", Tags: []string{string(long)}})
		requireAPIStatus(t, err, 400)
	})
	t.Run("game not found 404", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		svcCtx.GameRepository = &failingGameStore{
			GameStore:  svcCtx.GameRepository,
			setTagsErr: model.ErrGameNotFound,
		}
		l := NewUpdateGameTagsLogic(reviewCtx(1), svcCtx)
		_, err := l.UpdateGameTags(&types.UpdateGameTagsRequest{Id: "g-ghost", Tags: []string{"a"}})
		requireAPIStatus(t, err, 404)
	})
	t.Run("update failed 500", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		svcCtx.GameRepository = &failingGameStore{
			GameStore:  svcCtx.GameRepository,
			setTagsErr: errors.New("update down"),
		}
		l := NewUpdateGameTagsLogic(reviewCtx(1), svcCtx)
		_, err := l.UpdateGameTags(&types.UpdateGameTagsRequest{Id: "g-rpg-1", Tags: []string{"a"}})
		requireAPIStatus(t, err, 500)
	})
	t.Run("audit failed tolerated", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:    svcCtx.AdminRepository,
			writeAuditErr: errors.New("audit down"),
		}
		l := NewUpdateGameTagsLogic(reviewCtx(1), svcCtx)
		resp, err := l.UpdateGameTags(&types.UpdateGameTagsRequest{Id: "g-rpg-1", Tags: []string{"new-tag"}})
		if err != nil || resp == nil {
			t.Fatalf("audit failure tolerated: resp=%+v err=%v", resp, err)
		}
	})
}
