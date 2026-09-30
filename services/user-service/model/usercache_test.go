package model

import (
	"testing"
	"time"
)

// createAlice 建一个可复用的测试用户。
func createAlice(t *testing.T, m *UserModel) *User {
	t.Helper()
	u := &User{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "hashed",
	}
	u.Nickname.String = "Alice"
	u.Nickname.Valid = true
	if err := m.Create(u); err != nil {
		t.Fatalf("create alice: %v", err)
	}
	return u
}

// TestUserModelCacheHitSurvivesClosedDB 三个键读热后关闭数据库：
// 命中缓存仍能返回用户，未缓存键穿透到已关闭连接报错（证明读的是缓存而非库）。
func TestUserModelCacheHitSurvivesClosedDB(t *testing.T) {
	db := setupTestDB(t)
	m := NewUserModel(db)
	u := createAlice(t, m)

	if _, err := m.FindOne(u.Id); err != nil {
		t.Fatalf("FindOne: %v", err)
	}
	if _, err := m.FindByUsername("alice"); err != nil {
		t.Fatalf("FindByUsername: %v", err)
	}
	if _, err := m.FindByEmail("alice@example.com"); err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	got, err := m.FindOne(u.Id)
	if err != nil || got.Email != "alice@example.com" {
		t.Fatalf("FindOne cached = %+v, %v", got, err)
	}
	if got, err = m.FindByUsername("alice"); err != nil || got.Username != "alice" {
		t.Fatalf("FindByUsername cached = %+v, %v", got, err)
	}
	if got, err = m.FindByEmail("alice@example.com"); err != nil || got.Id != u.Id {
		t.Fatalf("FindByEmail cached = %+v, %v", got, err)
	}

	// 未缓存键 → 穿透 → 关库错误（同时证明负结果未被缓存）
	if _, err = m.FindOne(987654); err == nil {
		t.Fatal("uncached key must hit the (closed) database and fail")
	}
}

// TestUserModelCacheInvalidatesOnUpdate 更新后三条路径必须立即读到新值：
// id 键直删；username/email 旧键靠 Update 内读行内旧值失效
// （调用方只带 Id+Email 的部分结构，username 键内容不可从参数推导）。
func TestUserModelCacheInvalidatesOnUpdate(t *testing.T) {
	db := setupTestDB(t)
	m := NewUserModel(db)
	u := createAlice(t, m)

	// 预热三键（旧 email）
	if _, err := m.FindOne(u.Id); err != nil {
		t.Fatalf("warm FindOne: %v", err)
	}
	if _, err := m.FindByUsername("alice"); err != nil {
		t.Fatalf("warm FindByUsername: %v", err)
	}
	if _, err := m.FindByEmail("alice@example.com"); err != nil {
		t.Fatalf("warm FindByEmail: %v", err)
	}

	// 模拟 updateuserinfo logic 的部分结构（Username 为空）
	upd := &User{Id: u.Id, Email: "alice@new.com"}
	upd.Nickname.String = "Alicia"
	upd.Nickname.Valid = true
	if err := m.Update(upd); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := m.FindOne(u.Id)
	if err != nil || got.Email != "alice@new.com" {
		t.Fatalf("FindOne after update = %+v, %v; want new email", got, err)
	}
	// 登录路径：username 键必须失效重查，不能返回旧 email
	if got, err = m.FindByUsername("alice"); err != nil || got.Email != "alice@new.com" {
		t.Fatalf("FindByUsername after update = %+v, %v; want new email", got, err)
	}
	// 旧 email 键失效 → 数据库真值（无该行）
	if _, err = m.FindByEmail("alice@example.com"); err == nil {
		t.Fatal("old email must be gone after update")
	}
	// 新 email 键可正常建立
	if got, err = m.FindByEmail("alice@new.com"); err != nil || got.Id != u.Id {
		t.Fatalf("FindByEmail new = %+v, %v", got, err)
	}
}

// TestUserModelCacheNoNegativeCaching 未找到的结果不缓存：
// 查询空 id 失败后插入该行，再查必须命中（负缓存会让这里永远失败）。
func TestUserModelCacheNoNegativeCaching(t *testing.T) {
	db := setupTestDB(t)
	m := NewUserModel(db)

	if _, err := m.FindOne(9999); err == nil {
		t.Fatal("missing id must fail")
	}
	now := time.Now()
	if _, err := db.Exec(`INSERT INTO users (id, username, email, password, nickname, created_at, updated_at)
		VALUES (9999, 'phantom', 'phantom@example.com', 'hashed', NULL, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := m.FindOne(9999)
	if err != nil || got.Username != "phantom" {
		t.Fatalf("FindOne after insert = %+v, %v; want fresh row", got, err)
	}
}

// TestUserModelCacheReturnsCopies 读缓存返回值副本：
// 调用方改动返回值不得污染缓存。
func TestUserModelCacheReturnsCopies(t *testing.T) {
	db := setupTestDB(t)
	m := NewUserModel(db)
	u := createAlice(t, m)

	first, err := m.FindOne(u.Id)
	if err != nil {
		t.Fatalf("FindOne: %v", err)
	}
	first.Email = "mutated@example.com"
	first.Nickname.String = "hacked"

	second, err := m.FindOne(u.Id)
	if err != nil {
		t.Fatalf("FindOne again: %v", err)
	}
	if second.Email != "alice@example.com" || second.Nickname.String != "Alice" {
		t.Fatalf("caller mutation leaked into cache: %+v", second)
	}
}
