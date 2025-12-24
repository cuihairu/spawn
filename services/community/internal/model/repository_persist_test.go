package model

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"
)

func TestTopicRepository_CreatesSeedFileAndPersistsCreate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "topics.json")

	repo, err := NewTopicRepository(path)
	if err != nil {
		t.Fatalf("NewTopicRepository: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected topics file to be created, stat error: %v", err)
	}

	created, err := repo.Create(&types.CreateTopicReq{Name: "foo"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Id <= 0 {
		t.Fatalf("expected created.Id > 0, got %d", created.Id)
	}

	reloaded, err := NewTopicRepository(path)
	if err != nil {
		t.Fatalf("NewTopicRepository(reload): %v", err)
	}
	got, err := reloaded.Get(created.Id)
	if err != nil {
		t.Fatalf("Get(reload): %v", err)
	}
	if got.Name != "foo" {
		t.Fatalf("expected name %q, got %q", "foo", got.Name)
	}
}

func TestPostRepository_CreatesSeedFileAndPersistsCreate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "posts.json")

	repo, err := NewPostRepository(path)
	if err != nil {
		t.Fatalf("NewPostRepository: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected posts file to be created, stat error: %v", err)
	}

	created, err := repo.Create(1, 123, "tester", &types.CreatePostReq{
		TopicId: 1,
		Title:   "hello",
		Content: "world",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Id <= 0 {
		t.Fatalf("expected created.Id > 0, got %d", created.Id)
	}

	reloaded, err := NewPostRepository(path)
	if err != nil {
		t.Fatalf("NewPostRepository(reload): %v", err)
	}
	got, err := reloaded.Get(created.Id)
	if err != nil {
		t.Fatalf("Get(reload): %v", err)
	}
	if got.Title != "hello" {
		t.Fatalf("expected title %q, got %q", "hello", got.Title)
	}
}

func TestFollowRepository_PersistsTopicAndUserFollows(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "follows.json")

	repo, err := NewFollowRepository(path)
	if err != nil {
		t.Fatalf("NewFollowRepository: %v", err)
	}

	if created := repo.FollowTopic(1, 2); !created {
		t.Fatalf("expected FollowTopic to create relation")
	}
	if created := repo.FollowUser(1, 10); !created {
		t.Fatalf("expected FollowUser to create relation")
	}

	var st followState
	if err := readJSONFile(path, &st); err != nil {
		t.Fatalf("readJSONFile: %v", err)
	}
	if got := st.UserTopics[1]; !reflect.DeepEqual(got, []int64{2}) {
		t.Fatalf("expected user_topics[1]=[2], got %#v", got)
	}
	if got := st.UserUsers[1]; !reflect.DeepEqual(got, []int64{10}) {
		t.Fatalf("expected user_users[1]=[10], got %#v", got)
	}
}
