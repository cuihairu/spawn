package model

import (
	"testing"
)

// newClosedFavoriteModel 建表后立即关闭底层库，专打各方法的错误返回路径。
func newClosedFavoriteModel(t *testing.T) *FavoriteModel {
	t.Helper()
	favs, _ := newFavoriteModel(t, false)
	if err := favs.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	return favs
}

// TestCreateFavoritesTable_ClosedDBErrors 关闭库 → SQLite 建表失败 →
// MySQL 回落（ENGINE=InnoDB）同样失败 → 双方言连败返回错误。
// 注：MySQL 回落的「成功」分支在 SQLite 测试环境不可达——SQLite 方言建表
// 恒成功，回落仅在连败时触达，SQL 方言仅静态核对（community 侧同款口径）。
func TestCreateFavoritesTable_ClosedDBErrors(t *testing.T) {
	favs := newClosedFavoriteModel(t)
	if err := favs.CreateFavoritesTable(); err == nil {
		t.Fatal("CreateFavoritesTable on closed db must error")
	}
}

// TestFavoriteModel_Add_ClosedDBErrors 关闭库 → INSERT OR IGNORE 失败 →
// INSERT IGNORE 回落同样失败 → 双方言连败返回错误。MySQL 回落「成功」
// 分支在 SQLite 测试环境不可达（同 CreateFavoritesTable 注）。
func TestFavoriteModel_Add_ClosedDBErrors(t *testing.T) {
	favs := newClosedFavoriteModel(t)
	if err := favs.Add(1, 10); err == nil {
		t.Fatal("Add on closed db must error")
	}
}

func TestFavoriteModel_Remove_ClosedDBErrors(t *testing.T) {
	favs := newClosedFavoriteModel(t)
	if err := favs.Remove(1, 10); err == nil {
		t.Fatal("Remove on closed db must error")
	}
}

func TestFavoriteModel_Exists_ClosedDBErrors(t *testing.T) {
	favs := newClosedFavoriteModel(t)
	if _, err := favs.Exists(1, 10); err == nil {
		t.Fatal("Exists on closed db must error")
	}
}

func TestFavoriteModel_CountByGuide_ClosedDBErrors(t *testing.T) {
	favs := newClosedFavoriteModel(t)
	if _, err := favs.CountByGuide(10); err == nil {
		t.Fatal("CountByGuide on closed db must error")
	}
}

func TestFavoriteModel_ListByUser_ClosedDBErrors(t *testing.T) {
	favs := newClosedFavoriteModel(t)
	if _, _, err := favs.ListByUser(1, 10, 0); err == nil {
		t.Fatal("ListByUser on closed db must error")
	}
}

// TestFavoriteModel_ListByUser_BadTagsIsInternal 收藏行 JOIN 到的攻略 tags 列为
// 非法 JSON 时，ListByUser 走解码失败分支返回内部错误。
func TestFavoriteModel_ListByUser_BadTagsIsInternal(t *testing.T) {
	favs, _ := newFavoriteModel(t, true)
	if err := favs.Add(1, 1); err != nil {
		t.Fatalf("add favorite: %v", err)
	}
	if _, err := favs.db.Exec(`UPDATE guides SET tags = '{bad' WHERE id = 1`); err != nil {
		t.Fatalf("corrupt tags: %v", err)
	}

	if _, _, err := favs.ListByUser(1, 10, 0); err == nil {
		t.Fatal("ListByUser with corrupt tags must error")
	}
}
