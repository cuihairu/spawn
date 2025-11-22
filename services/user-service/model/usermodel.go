package model

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// User 用户模型
type User struct {
	Id        int64          `json:"id"`
	Username  string         `json:"username"`
	Email     string         `json:"email"`
	Password  string         `json:"-"` // 不返回密码
	Nickname  sql.NullString `json:"nickname"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// UserModel 用户数据访问层
type UserModel struct {
	db *sql.DB
}

// NewUserModel 创建用户模型
func NewUserModel(db *sql.DB) *UserModel {
	return &UserModel{db: db}
}

// Create 创建用户
func (m *UserModel) Create(user *User) error {
	query := `
		INSERT INTO users (username, email, password, nickname, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	result, err := m.db.Exec(query, user.Username, user.Email, user.Password, user.Nickname, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return fmt.Errorf("创建用户失败: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("获取用户ID失败: %w", err)
	}

	user.Id = id
	return nil
}

// FindByUsername 根据用户名查找用户
func (m *UserModel) FindByUsername(username string) (*User, error) {
	query := `
		SELECT id, username, email, password, nickname, created_at, updated_at
		FROM users WHERE username = ?
	`

	user := &User{}
	err := m.db.QueryRow(query, username).Scan(
		&user.Id, &user.Username, &user.Email, &user.Password,
		&user.Nickname, &user.CreatedAt, &user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, errors.New("用户不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	return user, nil
}

// FindByEmail 根据邮箱查找用户
func (m *UserModel) FindByEmail(email string) (*User, error) {
	query := `
		SELECT id, username, email, password, nickname, created_at, updated_at
		FROM users WHERE email = ?
	`

	user := &User{}
	err := m.db.QueryRow(query, email).Scan(
		&user.Id, &user.Username, &user.Email, &user.Password,
		&user.Nickname, &user.CreatedAt, &user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, errors.New("用户不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	return user, nil
}

// FindOne 根据ID查找用户
func (m *UserModel) FindOne(id int64) (*User, error) {
	query := `
		SELECT id, username, email, password, nickname, created_at, updated_at
		FROM users WHERE id = ?
	`

	user := &User{}
	err := m.db.QueryRow(query, id).Scan(
		&user.Id, &user.Username, &user.Email, &user.Password,
		&user.Nickname, &user.CreatedAt, &user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, errors.New("用户不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	return user, nil
}

// Update 更新用户信息
func (m *UserModel) Update(user *User) error {
	query := `
		UPDATE users
		SET email = ?, nickname = ?, updated_at = ?
		WHERE id = ?
	`

	user.UpdatedAt = time.Now()
	_, err := m.db.Exec(query, user.Email, user.Nickname, user.UpdatedAt, user.Id)
	if err != nil {
		return fmt.Errorf("更新用户失败: %w", err)
	}

	return nil
}

// CheckUsernameExists 检查用户名是否存在
func (m *UserModel) CheckUsernameExists(username string) (bool, error) {
	query := "SELECT COUNT(*) FROM users WHERE username = ?"
	var count int
	err := m.db.QueryRow(query, username).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("检查用户名失败: %w", err)
	}
	return count > 0, nil
}

// CheckEmailExists 检查邮箱是否存在
func (m *UserModel) CheckEmailExists(email string) (bool, error) {
	query := "SELECT COUNT(*) FROM users WHERE email = ?"
	var count int
	err := m.db.QueryRow(query, email).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("检查邮箱失败: %w", err)
	}
	return count > 0, nil
}

// CreateUsersTable 创建用户表（开发使用）
func (m *UserModel) CreateUsersTable() error {
	// 检查数据库类型 - 通过数据库连接字符串判断
	var query string
	// 由于 Driver() 方法不可用，我们改为通过环境变量或者配置来判断
	// 这里使用一个简单的方法：尝试创建 SQLite 表格式，如果失败再尝试 MySQL 格式

	// 先尝试 SQLite 格式（更简单）
	query = `
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL UNIQUE,
			password TEXT NOT NULL,
			nickname TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`

	_, err := m.db.Exec(query)
	if err != nil {
		// 如果 SQLite 格式失败，尝试 MySQL 格式
		query = `
			CREATE TABLE IF NOT EXISTS users (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				username VARCHAR(50) NOT NULL UNIQUE,
				email VARCHAR(100) NOT NULL UNIQUE,
				password VARCHAR(255) NOT NULL,
				nickname VARCHAR(100),
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
				INDEX idx_username (username),
				INDEX idx_email (email)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		_, err = m.db.Exec(query)
		if err != nil {
			return fmt.Errorf("创建用户表失败（尝试了 SQLite 和 MySQL 格式）: %w", err)
		}
	}

	return nil
}