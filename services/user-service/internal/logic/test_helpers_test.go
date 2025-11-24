package logic

import (
	"database/sql"
	"testing"
	"time"

	"github.com/tappi/tappi/services/user-service/model"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	userModel := model.NewUserModel(db)
	if err := userModel.CreateUsersTable(); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	return db
}

func insertTestUser(t *testing.T, db *sql.DB, username, email, password, nickname string) int64 {
	t.Helper()
	now := time.Now()
	res, err := db.Exec(`INSERT INTO users (username, email, password, nickname, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		username, email, password, nickname, now, now)
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("failed to fetch id: %v", err)
	}
	return id
}
