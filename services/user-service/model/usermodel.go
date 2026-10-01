package model

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/tappi/tappi/services/user-service/internal/cache"
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

// 缓存参数：进程内 TTL+LRU（设计决策见 docs/development-guide.md
// 「数据库集成 → 设计决策」）。60s TTL 同时兜底任何失效遗漏的最终一致性。
const (
	defaultUserCacheTTL = 60 * time.Second
	defaultUserCacheMax = 4096
)

// UserModel 用户数据访问层。
// FindOne/FindByUsername/FindByEmail 读进程内缓存；Create/Update 走失效点。
// 不做负缓存（未找到/错误不写入），CheckXxxExists 的 COUNT 恒直查数据库。
type UserModel struct {
	db    *sql.DB
	cache *cache.Cache[string, User]
}

// UserStore 用户仓储接口：ServiceContext 面向接口依赖，测试可注入故障实现。
type UserStore interface {
	Create(user *User) error
	FindOne(id int64) (*User, error)
	FindByUsername(username string) (*User, error)
	FindByEmail(email string) (*User, error)
	Update(user *User) error
	CheckUsernameExists(username string) (bool, error)
	CheckEmailExists(email string) (bool, error)
}

var _ UserStore = (*UserModel)(nil)

// NewUserModel 创建用户模型（缓存默认开启）
func NewUserModel(db *sql.DB) *UserModel {
	return &UserModel{
		db:    db,
		cache: cache.New[string, User](defaultUserCacheTTL, defaultUserCacheMax),
	}
}

// 缓存键：三种键指向同一份 User 值副本（值语义，读写均无别名共享）。
func userKeyID(id int64) string       { return "id:" + strconv.FormatInt(id, 10) }
func userKeyUsername(u string) string { return "username:" + u }
func userKeyEmail(e string) string    { return "email:" + e }

// invalidateKeys 删除指定键组合（写路径失效点）。
func (m *UserModel) invalidateKeys(keys ...string) {
	for _, k := range keys {
		m.cache.Delete(k)
	}
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
	// 失效新行的键（负缓存不存在，通常无键可删；防御同名重建路径）
	m.invalidateKeys(userKeyID(id), userKeyUsername(user.Username), userKeyEmail(user.Email))
	return nil
}

// FindByUsername 根据用户名查找用户（读缓存）
func (m *UserModel) FindByUsername(username string) (*User, error) {
	key := userKeyUsername(username)
	if cached, ok := m.cache.Get(key); ok {
		return &cached, nil
	}

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

	m.cache.Set(key, *user)
	return user, nil
}

// FindByEmail 根据邮箱查找用户（读缓存）
func (m *UserModel) FindByEmail(email string) (*User, error) {
	key := userKeyEmail(email)
	if cached, ok := m.cache.Get(key); ok {
		return &cached, nil
	}

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

	m.cache.Set(key, *user)
	return user, nil
}

// FindOne 根据ID查找用户（读缓存）
func (m *UserModel) FindOne(id int64) (*User, error) {
	key := userKeyID(id)
	if cached, ok := m.cache.Get(key); ok {
		return &cached, nil
	}

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

	m.cache.Set(key, *user)
	return user, nil
}

// Update 更新用户信息（email/nickname 可变，username 不变）。
// 失效前先读取行内旧 username/email（绕过缓存取数据库真值）：
// 调用方常只带 Id+Email 构造部分结构，旧键必须从行内容推导。
func (m *UserModel) Update(user *User) error {
	var oldUsername, oldEmail string
	_ = m.db.QueryRow(`SELECT username, email FROM users WHERE id = ?`, user.Id).
		Scan(&oldUsername, &oldEmail) // 读不到不影响后续 UPDATE 的错误语义

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

	m.invalidateKeys(
		userKeyID(user.Id),
		userKeyUsername(oldUsername),
		userKeyEmail(oldEmail),
		userKeyEmail(user.Email), // 防 A→B→A 翻转后的旧映射残留
	)
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
