package model

import (
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"
)

func TestTopicRepository_ListKeywordOfficialAndPagination(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "topics.json")
	repo, err := NewTopicRepository(path)
	if err != nil {
		t.Fatalf("NewTopicRepository: %v", err)
	}

	if _, err := repo.Create(&types.CreateTopicReq{Name: "  Indie  ", Description: "roguelike"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	official, totalOfficial := repo.List("", true, 50, 0)
	if totalOfficial <= 0 || len(official) == 0 {
		t.Fatalf("expected official topics to be non-empty, total=%d", totalOfficial)
	}
	for _, tpc := range official {
		if !tpc.IsOfficial {
			t.Fatalf("expected only official topics, got %#v", tpc)
		}
	}

	filtered, totalFiltered := repo.List("开放", false, 50, 0)
	if totalFiltered <= 0 || len(filtered) == 0 {
		t.Fatalf("expected keyword match to return results")
	}

	page1, totalAll := repo.List("", false, 1, 0)
	page2, _ := repo.List("", false, 1, 1)
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

func TestTopicRepository_IncrementCountsClampsAtZero(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "topics.json")
	repo, err := NewTopicRepository(path)
	if err != nil {
		t.Fatalf("NewTopicRepository: %v", err)
	}

	created, err := repo.Create(&types.CreateTopicReq{Name: "count-test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.IncrementPostCount(created.Id, 3); err != nil {
		t.Fatalf("IncrementPostCount(+3): %v", err)
	}
	if err := repo.IncrementFollowerCount(created.Id, 2); err != nil {
		t.Fatalf("IncrementFollowerCount(+2): %v", err)
	}

	if err := repo.IncrementPostCount(created.Id, -99); err != nil {
		t.Fatalf("IncrementPostCount(-99): %v", err)
	}
	if err := repo.IncrementFollowerCount(created.Id, -99); err != nil {
		t.Fatalf("IncrementFollowerCount(-99): %v", err)
	}

	got, err := repo.Get(created.Id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PostCount != 0 || got.FollowerCount != 0 {
		t.Fatalf("expected counts to be clamped at zero, got post=%d follower=%d", got.PostCount, got.FollowerCount)
	}
}
