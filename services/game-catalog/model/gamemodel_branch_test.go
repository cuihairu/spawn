package model

// gamemodel.go 补漏分支：screenshots 列解码失败（Get/all，其余三列已有覆盖）、
// SetTags/SetStatus 的空 id 与 nil 标签守卫、UPDATE 失败臂（RAISE(ABORT) 触发器，
// 覆盖「SELECT 成功但 UPDATE 失败」）、关库下 ensureGameColumns 的方言探测
// 失败臂（PRAGMA 与 SHOW COLUMNS 双双失败 → 返回探测错误）。MySQL SHOW
// COLUMNS 成功路径、PRAGMA Scan 失败、rows.Err 与内嵌种子 panic 属登记
// 不可构造家族，见 coverage 台账。

import (
	"errors"
	"strings"
	"testing"
)

// TestGameModel_CorruptScreenshots screenshots 列存非法 JSON → Get 报
// 解析错误、列表读静默降级（与 genres/platforms/tags 同出口，列不同）。
func TestGameModel_CorruptScreenshots(t *testing.T) {
	m := newTestGameModel(t, false)
	if _, err := m.db.Exec(
		`INSERT INTO games (` + gameColumns + `)
		 VALUES ('g-shot', 'Shot', 'd', '[]', '[]', '[]', '2024-01-01', 'dev', 'pub', 1.0, 1, 'c', '[]', '', '', '', '', '', '', 'approved', '')`); err != nil {
		t.Fatalf("insert row: %v", err)
	}
	if _, err := m.db.Exec(`UPDATE games SET screenshots = '{oops' WHERE id = 'g-shot'`); err != nil {
		t.Fatalf("corrupt screenshots: %v", err)
	}

	if _, err := m.Get("g-shot"); err == nil || !strings.Contains(err.Error(), "解析游戏数组字段失败") {
		t.Fatalf("Get corrupt screenshots = %v, want 解析游戏数组字段失败", err)
	}
	if games, total := m.List(GameFilter{Limit: 5}); games != nil || total != 0 {
		t.Fatalf("List corrupt screenshots = %d/%d, want nil/0", len(games), total)
	}
}

// TestGameModel_SetTagsGuards 空 id → ErrGameNotFound；nil 标签归一为空数组。
func TestGameModel_SetTagsGuards(t *testing.T) {
	m := newTestGameModel(t, true)

	if _, err := m.SetTags("   ", nil); !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("SetTags blank id = %v, want ErrGameNotFound", err)
	}
	g, err := m.SetTags("game-elden-ring", nil)
	if err != nil {
		t.Fatalf("SetTags nil tags: %v", err)
	}
	if g.Tags == nil || len(g.Tags) != 0 {
		t.Fatalf("nil tags normalized = %v, want empty slice", g.Tags)
	}
}

// TestGameModel_SetStatusGuards 空 id 与不存在 id → ErrGameNotFound。
func TestGameModel_SetStatusGuards(t *testing.T) {
	m := newTestGameModel(t, true)

	if _, err := m.SetStatus("   ", "offline", ""); !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("SetStatus blank id = %v, want ErrGameNotFound", err)
	}
	if _, err := m.SetStatus("g-ghost", "offline", ""); !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("SetStatus missing id = %v, want ErrGameNotFound", err)
	}
}

// failGamesUpdateTrigger 挂 BEFORE UPDATE 触发器：games 表一切 UPDATE 中止
// （用于制造「Get 成功但 UPDATE 失败」）。
func failGamesUpdateTrigger(t *testing.T, m *GameModel) {
	t.Helper()
	if _, err := m.db.Exec(`CREATE TRIGGER fail_game_update BEFORE UPDATE ON games
		BEGIN SELECT RAISE(ABORT, 'forced game update failure'); END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
}

// SetTags：Get 成功但 UPDATE 被触发器中止 → 包一层「更新游戏标签失败」。
func TestGameModel_SetTagsUpdateFailed(t *testing.T) {
	m := newTestGameModel(t, true)
	failGamesUpdateTrigger(t, m)
	if _, err := m.SetTags("game-elden-ring", []string{"x"}); err == nil ||
		!strings.Contains(err.Error(), "更新游戏标签失败") {
		t.Fatalf("SetTags update failed = %v, want 更新游戏标签失败", err)
	}
}

// SetStatus：Get 成功但 UPDATE 被触发器中止 → 包一层「更新游戏状态失败」。
func TestGameModel_SetStatusUpdateFailed(t *testing.T) {
	m := newTestGameModel(t, true)
	failGamesUpdateTrigger(t, m)
	if _, err := m.SetStatus("game-elden-ring", "offline", ""); err == nil ||
		!strings.Contains(err.Error(), "更新游戏状态失败") {
		t.Fatalf("SetStatus update failed = %v, want 更新游戏状态失败", err)
	}
}

// 关库下 ensureGameColumns：PRAGMA 与 SHOW COLUMNS 双双失败 → 原样返回
// 探测错误（CreateGamesTable 的建表失败不会走到这里，须直调）。
func TestGameModel_EnsureColumnsClosedDB(t *testing.T) {
	m := newTestGameModel(t, false)
	if err := m.db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := m.ensureGameColumns(); err == nil {
		t.Fatal("ensureGameColumns on closed db = nil error, want error")
	}
}
