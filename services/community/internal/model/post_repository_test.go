package model

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/tappi/tappi/services/community/internal/types"
)

func TestPostRepository_UpdateDeletePermissionAndVisibility(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "posts.json")
	repo, err := NewPostRepository(path)
	if err != nil {
		t.Fatalf("NewPostRepository: %v", err)
	}

	post, err := repo.Create(1, 42, "author", &types.CreatePostReq{
		TopicId: 99,
		Title:   "  title  ",
		Content: " content ",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if post.TopicId != 1 {
		t.Fatalf("expected topicId %d, got %d", 1, post.TopicId)
	}

	if _, err := repo.Update(post.Id, 7, &types.UpdatePostReq{Title: "nope"}); err != ErrPermissionDenied {
		t.Fatalf("expected ErrPermissionDenied, got %v", err)
	}
	if err := repo.Delete(post.Id, 7); err != ErrPermissionDenied {
		t.Fatalf("expected ErrPermissionDenied, got %v", err)
	}

	updated, err := repo.Update(post.Id, 42, &types.UpdatePostReq{
		Title:   " new title ",
		Content: " new content ",
		Tags:    []string{"a", "b"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Title != "new title" || updated.Content != "new content" {
		t.Fatalf("unexpected update result: %#v", updated)
	}
	if err := repo.Delete(post.Id, 42); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := repo.Get(post.Id); err != ErrPostNotFound {
		t.Fatalf("expected ErrPostNotFound after delete, got %v", err)
	}
	if _, err := repo.Like(post.Id); err != ErrPostNotFound {
		t.Fatalf("expected ErrPostNotFound for Like after delete, got %v", err)
	}
	if _, err := repo.Share(post.Id); err != ErrPostNotFound {
		t.Fatalf("expected ErrPostNotFound for Share after delete, got %v", err)
	}
}

func TestPostRepository_ListDefaultsFiltersAndPagination(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "posts.json")
	repo, err := NewPostRepository(path)
	if err != nil {
		t.Fatalf("NewPostRepository: %v", err)
	}

	created, err := repo.Create(2, 500, "tester", &types.CreatePostReq{
		Title:   "fresh",
		Content: "no engagement yet",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	posts, total := repo.List(PostListFilter{TopicId: 2, Limit: 100, Offset: 0})
	if total < 1 {
		t.Fatalf("expected total >= 1, got %d", total)
	}
	found := false
	for _, p := range posts {
		if p.Id == created.Id {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected to find created post in topic list")
	}

	hotOnly, _ := repo.List(PostListFilter{IsHot: true, Limit: 100})
	for _, p := range hotOnly {
		if p.Id == created.Id {
			t.Fatalf("expected freshly created post (score=0) to be excluded from IsHot list")
		}
	}

	paged, _ := repo.List(PostListFilter{Limit: 1, Offset: 0})
	if len(paged) != 1 {
		t.Fatalf("expected 1 post on first page, got %d", len(paged))
	}
}

func TestPostRepository_HotOrderingPrefersPinnedThenScore(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "posts.json")
	repo, err := NewPostRepository(path)
	if err != nil {
		t.Fatalf("NewPostRepository: %v", err)
	}

	now := time.Now().UTC()
	newer, err := repo.Create(1, 1, "u1", &types.CreatePostReq{Title: "new", Content: "x"})
	if err != nil {
		t.Fatalf("Create(newer): %v", err)
	}
	older, err := repo.Create(1, 2, "u2", &types.CreatePostReq{Title: "old", Content: "x"})
	if err != nil {
		t.Fatalf("Create(older): %v", err)
	}

	repo.mu.Lock()
	repo.index[newer.Id].CreatedAt = now.Add(-10 * time.Hour).Format(time.RFC3339)
	repo.index[newer.Id].LikeCount = 10
	repo.index[older.Id].CreatedAt = now.Add(-60 * time.Hour).Format(time.RFC3339)
	repo.index[older.Id].LikeCount = 10
	repo.mu.Unlock()

	list := repo.Hot(10)
	if len(list) == 0 {
		t.Fatalf("expected hot list to be non-empty")
	}

	var newerIndex, olderIndex int = -1, -1
	for i := range list {
		if list[i].Id == newer.Id {
			newerIndex = i
		}
		if list[i].Id == older.Id {
			olderIndex = i
		}
	}
	if newerIndex == -1 || olderIndex == -1 {
		t.Fatalf("expected both posts to be present in hot list")
	}
	if newerIndex > olderIndex {
		t.Fatalf("expected newer post to rank above older (same engagement), got newerIndex=%d olderIndex=%d", newerIndex, olderIndex)
	}

	repo.mu.Lock()
	repo.index[older.Id].IsPinned = true
	repo.mu.Unlock()

	list2 := repo.Hot(10)
	if len(list2) == 0 || list2[0].Id != older.Id {
		t.Fatalf("expected pinned post to rank first")
	}
	if !list2[0].IsHot {
		t.Fatalf("expected returned Hot posts to have IsHot=true")
	}
}
