package model

import (
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	userModel := NewUserModel(db)
	if err := userModel.CreateUsersTable(); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	return db
}

func TestUserModel_Create(t *testing.T) {
	db := setupTestDB(t)
	m := NewUserModel(db)

	t.Run("success", func(t *testing.T) {
		user := &User{
			Username: "testuser",
			Email:    "test@example.com",
			Password: "hashedpassword",
			Nickname: sql.NullString{String: "Test User", Valid: true},
		}
		err := m.Create(user)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		if user.Id == 0 {
			t.Fatal("expected user.Id to be set")
		}
		if user.CreatedAt.IsZero() || user.UpdatedAt.IsZero() {
			t.Fatal("expected CreatedAt and UpdatedAt to be set")
		}
	})

	t.Run("duplicate username fails", func(t *testing.T) {
		user1 := &User{
			Username: "dupuser",
			Email:    "dup1@example.com",
			Password: "pass1",
		}
		if err := m.Create(user1); err != nil {
			t.Fatalf("Create first user failed: %v", err)
		}

		user2 := &User{
			Username: "dupuser",
			Email:    "dup2@example.com",
			Password: "pass2",
		}
		err := m.Create(user2)
		if err == nil {
			t.Fatal("expected error for duplicate username")
		}
	})

	t.Run("duplicate email fails", func(t *testing.T) {
		user1 := &User{
			Username: "user1",
			Email:    "same@example.com",
			Password: "pass1",
		}
		if err := m.Create(user1); err != nil {
			t.Fatalf("Create first user failed: %v", err)
		}

		user2 := &User{
			Username: "user2",
			Email:    "same@example.com",
			Password: "pass2",
		}
		err := m.Create(user2)
		if err == nil {
			t.Fatal("expected error for duplicate email")
		}
	})

	t.Run("nil nickname", func(t *testing.T) {
		user := &User{
			Username: "nonick",
			Email:    "nonick@example.com",
			Password: "pass",
			Nickname: sql.NullString{Valid: false},
		}
		err := m.Create(user)
		if err != nil {
			t.Fatalf("Create with nil nickname failed: %v", err)
		}
		if user.Id == 0 {
			t.Fatal("expected user.Id to be set")
		}
	})
}

func TestUserModel_FindByUsername(t *testing.T) {
	db := setupTestDB(t)
	m := NewUserModel(db)

	// Setup
	created := &User{
		Username: "findme",
		Email:    "findme@example.com",
		Password: "secret",
		Nickname: sql.NullString{String: "Finder", Valid: true},
	}
	if err := m.Create(created); err != nil {
		t.Fatalf("setup Create failed: %v", err)
	}

	t.Run("found", func(t *testing.T) {
		user, err := m.FindByUsername("findme")
		if err != nil {
			t.Fatalf("FindByUsername failed: %v", err)
		}
		if user == nil {
			t.Fatal("expected user, got nil")
		}
		if user.Username != "findme" {
			t.Fatalf("username mismatch: %s", user.Username)
		}
		if user.Email != "findme@example.com" {
			t.Fatalf("email mismatch: %s", user.Email)
		}
		if user.Password != "secret" {
			t.Fatalf("password mismatch: %s", user.Password)
		}
		if !user.Nickname.Valid || user.Nickname.String != "Finder" {
			t.Fatalf("nickname mismatch: %v", user.Nickname)
		}
		if user.Id != created.Id {
			t.Fatalf("id mismatch: got %d, want %d", user.Id, created.Id)
		}
	})

	t.Run("not found", func(t *testing.T) {
		user, err := m.FindByUsername("nonexistent")
		if err == nil {
			t.Fatal("expected error for non-existent user")
		}
		if user != nil {
			t.Fatal("expected nil user for non-existent")
		}
	})
}

func TestUserModel_FindByEmail(t *testing.T) {
	db := setupTestDB(t)
	m := NewUserModel(db)

	created := &User{
		Username: "emailuser",
		Email:    "emailuser@example.com",
		Password: "secret",
		Nickname: sql.NullString{String: "Email User", Valid: true},
	}
	if err := m.Create(created); err != nil {
		t.Fatalf("setup Create failed: %v", err)
	}

	t.Run("found", func(t *testing.T) {
		user, err := m.FindByEmail("emailuser@example.com")
		if err != nil {
			t.Fatalf("FindByEmail failed: %v", err)
		}
		if user == nil {
			t.Fatal("expected user, got nil")
		}
		if user.Email != "emailuser@example.com" {
			t.Fatalf("email mismatch: %s", user.Email)
		}
		if user.Username != "emailuser" {
			t.Fatalf("username mismatch: %s", user.Username)
		}
		if user.Id != created.Id {
			t.Fatalf("id mismatch: got %d, want %d", user.Id, created.Id)
		}
	})

	t.Run("not found", func(t *testing.T) {
		user, err := m.FindByEmail("nonexistent@example.com")
		if err == nil {
			t.Fatal("expected error for non-existent email")
		}
		if user != nil {
			t.Fatal("expected nil user for non-existent")
		}
	})
}

func TestUserModel_FindOne(t *testing.T) {
	db := setupTestDB(t)
	m := NewUserModel(db)

	created := &User{
		Username: "iduser",
		Email:    "iduser@example.com",
		Password: "secret",
		Nickname: sql.NullString{String: "ID User", Valid: true},
	}
	if err := m.Create(created); err != nil {
		t.Fatalf("setup Create failed: %v", err)
	}

	t.Run("found", func(t *testing.T) {
		user, err := m.FindOne(created.Id)
		if err != nil {
			t.Fatalf("FindOne failed: %v", err)
		}
		if user == nil {
			t.Fatal("expected user, got nil")
		}
		if user.Id != created.Id {
			t.Fatalf("id mismatch: got %d, want %d", user.Id, created.Id)
		}
		if user.Username != "iduser" {
			t.Fatalf("username mismatch: %s", user.Username)
		}
	})

	t.Run("not found", func(t *testing.T) {
		user, err := m.FindOne(999999)
		if err == nil {
			t.Fatal("expected error for non-existent id")
		}
		if user != nil {
			t.Fatal("expected nil user for non-existent")
		}
	})
}

func TestUserModel_Update(t *testing.T) {
	db := setupTestDB(t)
	m := NewUserModel(db)

	created := &User{
		Username: "updateuser",
		Email:    "old@example.com",
		Password: "secret",
		Nickname: sql.NullString{String: "Old Nick", Valid: true},
	}
	if err := m.Create(created); err != nil {
		t.Fatalf("setup Create failed: %v", err)
	}
	originalUpdatedAt := created.UpdatedAt
	time.Sleep(10 * time.Millisecond) // ensure time moves forward

	t.Run("update email and nickname", func(t *testing.T) {
		created.Email = "new@example.com"
		created.Nickname = sql.NullString{String: "New Nick", Valid: true}
		err := m.Update(created)
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		// Verify by fetching
		user, err := m.FindOne(created.Id)
		if err != nil {
			t.Fatalf("FindOne after update failed: %v", err)
		}
		if user.Email != "new@example.com" {
			t.Fatalf("email not updated: %s", user.Email)
		}
		if !user.Nickname.Valid || user.Nickname.String != "New Nick" {
			t.Fatalf("nickname not updated: %v", user.Nickname)
		}
		if user.UpdatedAt.Equal(originalUpdatedAt) || user.UpdatedAt.Before(originalUpdatedAt) {
			t.Fatalf("UpdatedAt not updated: %v", user.UpdatedAt)
		}
		// Username and password should not change
		if user.Username != "updateuser" {
			t.Fatalf("username changed unexpectedly: %s", user.Username)
		}
		if user.Password != "secret" {
			t.Fatalf("password changed unexpectedly: %s", user.Password)
		}
	})

	t.Run("clear nickname", func(t *testing.T) {
		created.Nickname = sql.NullString{Valid: false}
		err := m.Update(created)
		if err != nil {
			t.Fatalf("Update clear nickname failed: %v", err)
		}
		user, err := m.FindOne(created.Id)
		if err != nil {
			t.Fatalf("FindOne after clear nickname failed: %v", err)
		}
		if user.Nickname.Valid {
			t.Fatalf("expected nickname to be cleared, got: %v", user.Nickname)
		}
	})

	t.Run("update non-existent user", func(t *testing.T) {
		nonExistent := &User{
			Id:       999999,
			Email:    "ghost@example.com",
			Nickname: sql.NullString{String: "Ghost", Valid: true},
		}
		err := m.Update(nonExistent)
		// Update on non-existent should not error (0 rows affected is not an error in SQL)
		if err != nil {
			t.Fatalf("Update non-existent returned error: %v", err)
		}
	})
}

func TestUserModel_CheckUsernameExists(t *testing.T) {
	db := setupTestDB(t)
	m := NewUserModel(db)

	created := &User{
		Username: "checkuser",
		Email:    "check@example.com",
		Password: "secret",
	}
	if err := m.Create(created); err != nil {
		t.Fatalf("setup Create failed: %v", err)
	}

	t.Run("exists", func(t *testing.T) {
		exists, err := m.CheckUsernameExists("checkuser")
		if err != nil {
			t.Fatalf("CheckUsernameExists failed: %v", err)
		}
		if !exists {
			t.Fatal("expected username to exist")
		}
	})

	t.Run("not exists", func(t *testing.T) {
		exists, err := m.CheckUsernameExists("nonexistent")
		if err != nil {
			t.Fatalf("CheckUsernameExists failed: %v", err)
		}
		if exists {
			t.Fatal("expected username to not exist")
		}
	})
}

func TestUserModel_CheckEmailExists(t *testing.T) {
	db := setupTestDB(t)
	m := NewUserModel(db)

	created := &User{
		Username: "emailcheck",
		Email:    "emailcheck@example.com",
		Password: "secret",
	}
	if err := m.Create(created); err != nil {
		t.Fatalf("setup Create failed: %v", err)
	}

	t.Run("exists", func(t *testing.T) {
		exists, err := m.CheckEmailExists("emailcheck@example.com")
		if err != nil {
			t.Fatalf("CheckEmailExists failed: %v", err)
		}
		if !exists {
			t.Fatal("expected email to exist")
		}
	})

	t.Run("not exists", func(t *testing.T) {
		exists, err := m.CheckEmailExists("nonexistent@example.com")
		if err != nil {
			t.Fatalf("CheckEmailExists failed: %v", err)
		}
		if exists {
			t.Fatal("expected email to not exist")
		}
	})
}

func TestUserModel_CreateUsersTable(t *testing.T) {
	t.Run("idempotent on same db", func(t *testing.T) {
		db := setupTestDB(t)
		m := NewUserModel(db)
		// Table already created in setupTestDB, calling again should not error
		if err := m.CreateUsersTable(); err != nil {
			t.Fatalf("CreateUsersTable idempotent call failed: %v", err)
		}
	})

	t.Run("creates table on fresh db", func(t *testing.T) {
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			t.Fatalf("failed to open sqlite: %v", err)
		}
		defer db.Close()

		m := NewUserModel(db)
		if err := m.CreateUsersTable(); err != nil {
			t.Fatalf("CreateUsersTable on fresh db failed: %v", err)
		}

		// Verify table exists by inserting
		user := &User{
			Username: "fresh",
			Email:    "fresh@example.com",
			Password: "pass",
		}
		if err := m.Create(user); err != nil {
			t.Fatalf("Create on fresh table failed: %v", err)
		}
		if user.Id == 0 {
			t.Fatal("expected id to be set on fresh table")
		}
	})
}

func TestUserModel_ConcurrentAccess(t *testing.T) {
	// Use shared cache URI so all connections see the same in-memory database
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	defer db.Close()

	// Limit to 1 connection to avoid issues with in-memory DB
	db.SetMaxOpenConns(1)

	m := NewUserModel(db)
	if err := m.CreateUsersTable(); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	// Create initial user
	user := &User{
		Username: "concurrent",
		Email:    "concurrent@example.com",
		Password: "pass",
	}
	if err := m.Create(user); err != nil {
		t.Fatalf("initial create failed: %v", err)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 100)

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, err := m.FindByUsername("concurrent")
				if err != nil {
					errChan <- err
					return
				}
				_, err = m.FindByEmail("concurrent@example.com")
				if err != nil {
					errChan <- err
					return
				}
				_, err = m.FindOne(user.Id)
				if err != nil {
					errChan <- err
					return
				}
				_, err = m.CheckUsernameExists("concurrent")
				if err != nil {
					errChan <- err
					return
				}
				_, err = m.CheckEmailExists("concurrent@example.com")
				if err != nil {
					errChan <- err
					return
				}
			}
		}()
	}

	// Concurrent writes (unique usernames/emails to avoid conflicts)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			u := &User{
				Username: "concurrentwriter" + string(rune('0'+idx)),
				Email:    "writer" + string(rune('0'+idx)) + "@example.com",
				Password: "pass",
				Nickname: sql.NullString{String: "Writer", Valid: true},
			}
			_ = m.Create(u)
		}(i)
	}

	wg.Wait()
	close(errChan)
	for err := range errChan {
		t.Errorf("concurrent access error: %v", err)
	}
}

// TestUserModel_ClosedDBErrors 已关闭连接上的查询/写入/建表：所有 *sql.DB
// 错误分支原样返回包装错误；CreateUsersTable 在 SQLite 语法失败后尝试
// MySQL 语法、再度失败时返回双重尝试错误。
func TestUserModel_ClosedDBErrors(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	m := NewUserModel(db)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if _, err := m.FindByUsername("alice"); err == nil || !strings.Contains(err.Error(), "查询用户失败") {
		t.Fatalf("FindByUsername closed-db err = %v", err)
	}
	if _, err := m.FindByEmail("alice@example.com"); err == nil || !strings.Contains(err.Error(), "查询用户失败") {
		t.Fatalf("FindByEmail closed-db err = %v", err)
	}
	if _, err := m.FindOne(1); err == nil || !strings.Contains(err.Error(), "查询用户失败") {
		t.Fatalf("FindOne closed-db err = %v", err)
	}
	if err := m.Update(&User{Id: 1, Email: "a@b.com"}); err == nil || !strings.Contains(err.Error(), "更新用户失败") {
		t.Fatalf("Update closed-db err = %v", err)
	}
	if _, err := m.CheckEmailExists("a@b.com"); err == nil || !strings.Contains(err.Error(), "检查邮箱失败") {
		t.Fatalf("CheckEmailExists closed-db err = %v", err)
	}
	if err := m.CreateUsersTable(); err == nil || !strings.Contains(err.Error(), "尝试了 SQLite 和 MySQL 格式") {
		t.Fatalf("CreateUsersTable double-failure err = %v", err)
	}
}
