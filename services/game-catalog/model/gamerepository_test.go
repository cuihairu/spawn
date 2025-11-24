package model

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleGames = `[
	{
		"id": "game-rpg-1",
		"title": "Sample RPG 1",
		"description": "RPG entry",
		"genres": ["RPG"],
		"platforms": ["PC"],
		"release_date": "2023-01-01",
		"developer": "Studio A",
		"publisher": "Studio A",
		"tags": ["Story", "Singleplayer"],
		"score": 9.0,
		"trending_score": 90,
		"cover_image": ""
	},
	{
		"id": "game-rpg-2",
		"title": "Sample RPG 2",
		"description": "RPG entry 2",
		"genres": ["RPG", "Action"],
		"platforms": ["PC", "PS5"],
		"release_date": "2024-01-01",
		"developer": "Studio B",
		"publisher": "Studio B",
		"tags": ["Co-op"],
		"score": 8.3,
		"trending_score": 70,
		"cover_image": ""
	},
	{
		"id": "game-fps-1",
		"title": "Sample FPS",
		"description": "Shooter entry",
		"genres": ["FPS"],
		"platforms": ["PC"],
		"release_date": "2022-06-01",
		"developer": "Studio C",
		"publisher": "Studio C",
		"tags": ["PVP"],
		"score": 8.8,
		"trending_score": 60,
		"cover_image": ""
	}
]`

func TestGameRepositoryListFilter(t *testing.T) {
	path := writeTempGameFile(t, sampleGames)
	repo, err := NewGameRepository(path)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}

	games, total := repo.List(GameFilter{Genre: "RPG", Limit: 10})
	if total != 2 {
		t.Fatalf("expected total 2, got %d", total)
	}
	if len(games) != 2 {
		t.Fatalf("expected 2 games, got %d", len(games))
	}

	if games[0].Id != "game-rpg-1" {
		t.Fatalf("expected sorted results by trending score, got %s first", games[0].Id)
	}
}

func TestGameRepositoryRecommendByUser(t *testing.T) {
	path := writeTempGameFile(t, sampleGames)
	repo, err := NewGameRepository(path)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}

	recsA := repo.Recommend("user-a", []string{"RPG"}, 2)
	recsB := repo.Recommend("user-b", []string{"RPG"}, 2)

	if len(recsA) == 0 || len(recsB) == 0 {
		t.Fatalf("expected recommendations for both users")
	}

	if recsA[0].Id == recsB[0].Id {
		t.Fatalf("expected user-specific ordering, got same head result %s", recsA[0].Id)
	}
}

func TestGameRepositoryRecommendFallback(t *testing.T) {
	path := writeTempGameFile(t, sampleGames)
	repo, err := NewGameRepository(path)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}

	recs := repo.Recommend("", []string{"Unknown"}, 5)
	if len(recs) == 0 {
		t.Fatalf("expected fallback recommendations when genre empty")
	}
}

func writeTempGameFile(t *testing.T, payload string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "games.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	return path
}
