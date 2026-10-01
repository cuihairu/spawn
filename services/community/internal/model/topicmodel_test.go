package model

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/tappi/tappi/services/community/internal/types"

	_ "github.com/mattn/go-sqlite3"
)

// newTopicModel 打开临时文件 SQLite（连接池由模型钳制为 1）并建表；
// seed=true 时写入内嵌种子。
func newTopicModel(t *testing.T, seed bool) *TopicModel {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "community.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	m := NewTopicModel(db)
	if err := m.CreateTopicsTable(); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if seed {
		if err := m.SeedIfEmpty(); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return m
}

// --- 种子与初始化 ---

func TestTopicSeedIfEmpty_PopulatesAndSkipsNonEmpty(t *testing.T) {
	m := newTopicModel(t, true)

	if _, total := m.List("", false, 50, 0); total != 2 {
		t.Fatalf("seeded total = %d, want 2", total)
	}
	got, err := m.Get(1)
	if err != nil || got.Name != "《黑神话：悟空》" || !got.IsOfficial {
		t.Fatalf("Get(1) = %+v, %v; want seeded official topic", got, err)
	}
	created, err := m.Create(&types.CreateTopicReq{Name: "new"})
	if err != nil || created.Id != 3 {
		t.Fatalf("created id = %d, %v; want 3", created.Id, err)
	}

	// 非空表再跑 SeedIfEmpty 必须是 no-op
	if err := m.SeedIfEmpty(); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if _, total := m.List("", false, 50, 0); total != 3 {
		t.Fatalf("total after re-seed = %d, want 3", total)
	}
}

func TestTopicSeed_SkipsNilAndNonPositiveID(t *testing.T) {
	m := newTopicModel(t, false)
	if err := m.Seed([]*types.Topic{
		nil,
		{Name: "no id"},
		{Id: 9, Name: "t9", Description: "d"},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := m.Get(9); err != nil {
		t.Fatalf("Get(9): %v", err)
	}
	if _, total := m.List("", false, 10, 0); total != 1 {
		t.Fatalf("total = %d, want 1 (nil/zero-id skipped)", total)
	}
}

func TestTopicModelGet_NotFound(t *testing.T) {
	m := newTopicModel(t, true)

	if _, err := m.Get(999); !errors.Is(err, ErrTopicNotFound) {
		t.Fatalf("err = %v, want ErrTopicNotFound", err)
	}
	// 无负缓存：随后写入同 id 应立即可见
	if err := m.Seed([]*types.Topic{{Id: 999, Name: "late"}}); err != nil {
		t.Fatalf("late seed: %v", err)
	}
	if _, err := m.Get(999); err != nil {
		t.Fatalf("Get(999) after late insert: %v", err)
	}
}

func TestTopicModel_CacheHitServesAfterDBClose(t *testing.T) {
	m := newTopicModel(t, true)

	if _, err := m.Get(1); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	if err := m.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	if got, err := m.Get(1); err != nil || got.Name != "《黑神话：悟空》" {
		t.Fatalf("cached topic = %+v, err=%v", got, err)
	}
	if _, err := m.Get(2); err == nil || errors.Is(err, ErrTopicNotFound) {
		t.Fatalf("err = %v, want non-sentinel db error", err)
	}
}

// --- Create / List ---

func TestTopicModelCreate_TrimsAndDefaults(t *testing.T) {
	m := newTopicModel(t, true)

	created, err := m.Create(&types.CreateTopicReq{Name: "  Indie  ", Description: "roguelike", Icon: " i ", CoverImage: " c "})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Name != "Indie" || created.Description != "roguelike" || created.Icon != "i" || created.CoverImage != "c" {
		t.Fatalf("trim not applied: %+v", created)
	}
	if created.IsOfficial || created.PostCount != 0 || created.FollowerCount != 0 {
		t.Fatalf("defaults not applied: %+v", created)
	}
	if _, err := time.Parse(time.RFC3339, created.CreatedAt); err != nil {
		t.Fatalf("CreatedAt %q not RFC3339: %v", created.CreatedAt, err)
	}
}

func TestTopicModel_ListKeywordOfficialAndPagination(t *testing.T) {
	m := newTopicModel(t, true)

	if _, err := m.Create(&types.CreateTopicReq{Name: "  Indie  ", Description: "roguelike"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	official, totalOfficial := m.List("", true, 50, 0)
	if totalOfficial <= 0 || len(official) == 0 {
		t.Fatalf("expected official topics to be non-empty, total=%d", totalOfficial)
	}
	for _, tpc := range official {
		if !tpc.IsOfficial {
			t.Fatalf("expected only official topics, got %#v", tpc)
		}
	}

	filtered, totalFiltered := m.List("开放", false, 50, 0)
	if totalFiltered <= 0 || len(filtered) == 0 {
		t.Fatalf("expected keyword match to return results")
	}

	// 大小写不敏感（名称与描述都参与匹配）
	if _, total := m.List("INDIE", false, 50, 0); total != 1 {
		t.Fatalf("case-insensitive keyword total = %d, want 1", total)
	}
	if _, total := m.List("ROGUELIKE", false, 50, 0); total != 1 {
		t.Fatalf("description keyword total = %d, want 1", total)
	}
	if _, total := m.List("  ", false, 50, 0); total != 3 {
		t.Fatalf("blank keyword total = %d, want 3 (no filter)", total)
	}

	page1, totalAll := m.List("", false, 1, 0)
	page2, _ := m.List("", false, 1, 1)
	if totalAll < 2 {
		t.Fatalf("expected >=2 topics, got %d", totalAll)
	}
	if len(page1) != 1 || len(page2) != 1 {
		t.Fatalf("expected both pages to have 1 item, got %d and %d", len(page1), len(page2))
	}
	if page1[0].Id == page2[0].Id {
		t.Fatalf("expected pagination to return different topics")
	}
}

func TestTopicModel_ListPaginationClamps(t *testing.T) {
	m := newTopicModel(t, true)

	if _, total := m.List("", false, -1, -5); total < 1 {
		t.Fatalf("List with negative paging total = %d, want seeds", total)
	}
	if got, _ := m.List("", false, 1, 99999); len(got) != 0 {
		t.Fatalf("List past end = %+v, want empty", got)
	}
	// limit<=0 回落 20（种子 2 条全量返回）
	if got, total := m.List("", false, 0, 0); total != 2 || len(got) != 2 {
		t.Fatalf("List limit=0 = %d items (total %d), want 2", len(got), total)
	}
}

func TestTopicModel_ListCorruptRowReturnsZero(t *testing.T) {
	m := newTopicModel(t, false)

	// 数字列塞文本 → 行扫描失败 → List 全量降级 nil/0
	if _, err := m.db.Exec(`INSERT INTO topics (id, name, description, icon, cover_image,
		is_official, post_count, follower_count, created_at, updated_at)
		VALUES (9, 'dirty', '', '', '', 0, 'abc', 0, 'x', 'y')`); err != nil {
		t.Fatalf("hand insert: %v", err)
	}
	if got, total := m.List("", false, 10, 0); got != nil || total != 0 {
		t.Fatalf("List with corrupt numeric column = %v/%d, want nil/0", got, total)
	}
}

func TestTopicModel_ListClosedDBReturnsZero(t *testing.T) {
	m := newTopicModel(t, true)
	if err := m.db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if got, total := m.List("", false, 10, 0); got != nil || total != 0 {
		t.Fatalf("List on closed db = %v/%d, want nil/0", got, total)
	}
}

// --- 计数增减 ---

func TestTopicModel_IncrementCountsClampsAtZero(t *testing.T) {
	m := newTopicModel(t, true)

	created, err := m.Create(&types.CreateTopicReq{Name: "count-test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := m.IncrementPostCount(created.Id, 3); err != nil {
		t.Fatalf("IncrementPostCount(+3): %v", err)
	}
	if err := m.IncrementFollowerCount(created.Id, 2); err != nil {
		t.Fatalf("IncrementFollowerCount(+2): %v", err)
	}

	if err := m.IncrementPostCount(created.Id, -99); err != nil {
		t.Fatalf("IncrementPostCount(-99): %v", err)
	}
	if err := m.IncrementFollowerCount(created.Id, -99); err != nil {
		t.Fatalf("IncrementFollowerCount(-99): %v", err)
	}

	got, err := m.Get(created.Id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PostCount != 0 || got.FollowerCount != 0 {
		t.Fatalf("expected counts to be clamped at zero, got post=%d follower=%d", got.PostCount, got.FollowerCount)
	}
}

func TestTopicModel_IncrementMissingTopic(t *testing.T) {
	m := newTopicModel(t, true)

	if err := m.IncrementPostCount(99999, 1); !errors.Is(err, ErrTopicNotFound) {
		t.Fatalf("IncrementPostCount = %v, want ErrTopicNotFound", err)
	}
	if err := m.IncrementFollowerCount(99999, 1); !errors.Is(err, ErrTopicNotFound) {
		t.Fatalf("IncrementFollowerCount = %v, want ErrTopicNotFound", err)
	}
}

// --- 关库错误传播与写失败注入 ---

func TestTopicModel_ClosedDBErrorPaths(t *testing.T) {
	m := newTopicModel(t, true)
	if err := m.db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if _, err := m.Get(1); err == nil || errors.Is(err, ErrTopicNotFound) {
		t.Fatalf("Get on closed db = %v, want non-sentinel error", err)
	}
	if _, err := m.Create(&types.CreateTopicReq{Name: "x"}); err == nil {
		t.Fatal("Create on closed db must error")
	}
	if err := m.IncrementPostCount(1, 1); err == nil {
		t.Fatal("IncrementPostCount on closed db must error")
	}
	if err := m.IncrementFollowerCount(1, 1); err == nil {
		t.Fatal("IncrementFollowerCount on closed db must error")
	}
	if err := m.SeedIfEmpty(); err == nil {
		t.Fatal("SeedIfEmpty on closed db must error")
	}
	if err := m.CreateTopicsTable(); err == nil {
		t.Fatal("CreateTopicsTable on closed db must error")
	}
}

func TestTopicModel_UpdateWriteFailurePropagates(t *testing.T) {
	m := newTopicModel(t, true)

	// 触发器令 UPDATE 中止：读路径不受影响，覆盖计数更新的写失败传播分支
	if _, err := m.db.Exec(`CREATE TRIGGER block_topic_update BEFORE UPDATE ON topics
		BEGIN SELECT RAISE(ABORT, 'blocked'); END;`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	if err := m.IncrementPostCount(1, 1); err == nil {
		t.Fatal("increment under abort trigger must error")
	}
}
