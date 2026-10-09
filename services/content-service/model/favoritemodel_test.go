package model

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// newFavoriteModel 建独立 SQLite 库 + guides/guide_favorites 两表，可选装样本攻略。
func newFavoriteModel(t *testing.T, withGuides bool) (*FavoriteModel, *GuideModel) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "content.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	guides := NewGuideModel(db)
	if err := guides.CreateGuidesTable(); err != nil {
		t.Fatalf("create guides table: %v", err)
	}
	if withGuides {
		seedGuides(t, guides, sampleGuideFixture()...)
	}

	favs := NewFavoriteModel(db)
	if err := favs.CreateFavoritesTable(); err != nil {
		t.Fatalf("create favorites table: %v", err)
	}
	return favs, guides
}

func TestFavoriteModel_AddExistsCount(t *testing.T) {
	favs, _ := newFavoriteModel(t, false)

	if exists, err := favs.Exists(1, 10); err != nil || exists {
		t.Fatalf("initial exists = %v, %v; want false", exists, err)
	}

	if err := favs.Add(1, 10); err != nil {
		t.Fatalf("add: %v", err)
	}
	// 幂等：重复收藏不报错、计数不重复
	if err := favs.Add(1, 10); err != nil {
		t.Fatalf("re-add: %v", err)
	}

	if exists, err := favs.Exists(1, 10); err != nil || !exists {
		t.Fatalf("exists after add = %v, %v; want true", exists, err)
	}
	count, err := favs.CountByGuide(10)
	if err != nil || count != 1 {
		t.Fatalf("count = %d, %v; want 1", count, err)
	}

	// 多用户独立
	if err := favs.Add(2, 10); err != nil {
		t.Fatalf("add user2: %v", err)
	}
	if exists, _ := favs.Exists(1, 10); !exists {
		t.Fatal("user1 favorite vanished after user2 add")
	}
	count, _ = favs.CountByGuide(10)
	if count != 2 {
		t.Fatalf("count = %d; want 2", count)
	}
	if exists, _ := favs.Exists(1, 11); exists {
		t.Fatal("unrelated guide should not be favorited")
	}
}

func TestFavoriteModel_RemoveIdempotent(t *testing.T) {
	favs, _ := newFavoriteModel(t, false)

	// 未收藏时取消同样成功（幂等）
	if err := favs.Remove(1, 10); err != nil {
		t.Fatalf("remove before add: %v", err)
	}

	if err := favs.Add(1, 10); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := favs.Remove(1, 10); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if exists, _ := favs.Exists(1, 10); exists {
		t.Fatal("favorite should be gone after remove")
	}
	if err := favs.Remove(1, 10); err != nil {
		t.Fatalf("remove twice: %v", err)
	}
}

func TestFavoriteModel_ListByUser(t *testing.T) {
	favs, _ := newFavoriteModel(t, true)

	// 用户 1 收藏 guide 1、3（样本攻略 id 1-3）
	if err := favs.Add(1, 1); err != nil {
		t.Fatalf("add 1: %v", err)
	}
	if err := favs.Add(1, 3); err != nil {
		t.Fatalf("add 3: %v", err)
	}
	// 用户 2 收藏 guide 2——不应出现在用户 1 的列表
	if err := favs.Add(2, 2); err != nil {
		t.Fatalf("add user2: %v", err)
	}

	guides, total, err := favs.ListByUser(1, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d; want 2", total)
	}
	// 收藏序新→旧；RFC3339 秒级精度下同秒并列按 guide_id 降序 → 3 在前
	if !equalInt64(guideIds(guides), []int64{3, 1}) {
		t.Fatalf("ids = %v; want [3 1]", guideIds(guides))
	}
	for _, g := range guides {
		if !g.IsPublished {
			t.Fatalf("guide %d should keep published flag", g.Id)
		}
		if len(g.Tags) == 0 && g.Id == 1 {
			t.Fatal("guide 1 tags should decode to non-empty")
		}
	}

	// 分页：limit 1 取第一条（收藏序新→旧，后收藏的 guide 3 在前）
	guides, total, err = favs.ListByUser(1, 1, 1)
	if err != nil {
		t.Fatalf("list page: %v", err)
	}
	if total != 2 || len(guides) != 1 {
		t.Fatalf("total = %d, len = %d; want 2, 1", total, len(guides))
	}

	// 用户 3 无收藏
	guides, total, err = favs.ListByUser(3, 10, 0)
	if err != nil || total != 0 || len(guides) != 0 {
		t.Fatalf("empty user: %v, %d, %d; want nil err, 0, 0", err, total, len(guides))
	}
}

func TestFavoriteModel_JoinDropsDeletedGuides(t *testing.T) {
	favs, _ := newFavoriteModel(t, true)

	if err := favs.Add(1, 1); err != nil {
		t.Fatalf("add: %v", err)
	}
	// 攻略被物理删除后收藏行残留，JOIN 应自然过滤掉
	// （GuideStore 无删除接口，测试直接清空 guides 表模拟物理删除）
	if _, err := favs.db.Exec(`DELETE FROM guides`); err != nil {
		t.Fatalf("purge guides: %v", err)
	}

	list, total, err := favs.ListByUser(1, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// total 计收藏行数（1），列表经 JOIN 不含已删攻略（0）
	if total != 1 || len(list) != 0 {
		t.Fatalf("total = %d, list = %d; want total 1, list 0", total, len(list))
	}
}
