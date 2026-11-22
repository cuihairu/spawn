package model

// DESIGN-001 策划视角游戏拆解库：每游戏一篇结构化拆解（核心循环/进度/经济/
// 战斗/养成/社交/付费/公平性/值得抄/踩坑），按品类归类聚合（同类横向对比），
// 讨论区独立评论表。与帖子/话题域分离：拆解是长文档型内容，编辑权在作者，
// 精选运营位在审核管理员。

import (
	"database/sql"
	"fmt"
	"time"
)

// DesignCategory 品类白名单：品类聚合页按此枚举出页，未知品类拒绝
// （避免自由文本品类把聚合页拆碎）。
var DesignCategories = []string{"MMORPG", "MOBA", "SLG", "卡牌", "放置"}

// ValidDesignCategory 品类校验（创建/更新时用）。
func ValidDesignCategory(c string) bool {
	for _, v := range DesignCategories {
		if v == c {
			return true
		}
	}
	return false
}

// DesignBreakdown 单篇游戏拆解。结构化字段一一对应拆解模板（每节纯文本，
// 客户端按标题分节渲染）；Sources 记录公开资料来源，不编造内部数值。
type DesignBreakdown struct {
	Id            int64  `json:"id"`
	GameTitle     string `json:"game_title"`
	Category      string `json:"category"`
	Summary       string `json:"summary"`
	CoreLoop      string `json:"core_loop"`
	Progression   string `json:"progression"`
	Economy       string `json:"economy"`
	Combat        string `json:"combat"`
	GrowthLines   string `json:"growth_lines"`
	Social        string `json:"social"`
	Monetization  string `json:"monetization"`
	Fairness      string `json:"fairness"`
	WorthStealing string `json:"worth_stealing"`
	Pitfalls      string `json:"pitfalls"`
	Sources       string `json:"sources"`
	AuthorId      int64  `json:"author_id"`
	AuthorName    string `json:"author_name"`
	Featured      bool   `json:"featured"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// DesignComment 拆解讨论区评论（单层，同帖子评论口径）。
type DesignComment struct {
	Id          int64  `json:"id"`
	BreakdownId int64  `json:"breakdown_id"`
	AuthorId    int64  `json:"author_id"`
	AuthorName  string `json:"author_name"`
	Content     string `json:"content"`
	CreatedAt   string `json:"created_at"`
}

// DesignCategoryRow 品类聚合行（品类页入口）。
type DesignCategoryRow struct {
	Category string `json:"category"`
	Count    int64  `json:"count"`
}

const designColumns = `id, game_title, category, summary, core_loop, progression, economy, combat,
	growth_lines, social, monetization, fairness, worth_stealing, pitfalls, sources,
	author_id, author_name, featured, created_at, updated_at`

// DesignStore 拆解域仓储接口。
type DesignStore interface {
	CreateBreakdownsTable() error
	SeedIfEmpty() error
	Create(b *DesignBreakdown) (int64, error)
	Update(id, authorId int64, b *DesignBreakdown) error
	Get(id int64) (*DesignBreakdown, error)
	List(category string, featured bool, limit, offset int64) ([]*DesignBreakdown, int64, error)
	Categories() ([]DesignCategoryRow, error)
	SetFeatured(id int64, featured bool) error
	AddComment(c *DesignComment) (int64, error)
	ListComments(breakdownId int64, limit, offset int64) ([]*DesignComment, error)
	DeleteComment(id, authorId int64) error
}

type DesignModel struct {
	db *sql.DB
}

// NewDesignModel 拆解域模型。
func NewDesignModel(db *sql.DB) *DesignModel {
	return &DesignModel{db: db}
}

// CreateBreakdownsTable 建拆解与讨论区两张表（双方言，幂等）。
func (m *DesignModel) CreateBreakdownsTable() error {
	for _, q := range []string{
		`
		CREATE TABLE IF NOT EXISTS design_breakdowns (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			game_title VARCHAR(64) NOT NULL,
			category VARCHAR(32) NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			core_loop TEXT NOT NULL DEFAULT '',
			progression TEXT NOT NULL DEFAULT '',
			economy TEXT NOT NULL DEFAULT '',
			combat TEXT NOT NULL DEFAULT '',
			growth_lines TEXT NOT NULL DEFAULT '',
			social TEXT NOT NULL DEFAULT '',
			monetization TEXT NOT NULL DEFAULT '',
			fairness TEXT NOT NULL DEFAULT '',
			worth_stealing TEXT NOT NULL DEFAULT '',
			pitfalls TEXT NOT NULL DEFAULT '',
			sources TEXT NOT NULL DEFAULT '',
			author_id INTEGER NOT NULL DEFAULT 0,
			author_name VARCHAR(64) NOT NULL DEFAULT '',
			featured INTEGER NOT NULL DEFAULT 0,
			created_at VARCHAR(32) NOT NULL,
			updated_at VARCHAR(32) NOT NULL
		)
		`,
		`
		CREATE TABLE IF NOT EXISTS design_breakdown_comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			breakdown_id INTEGER NOT NULL,
			author_id INTEGER NOT NULL DEFAULT 0,
			author_name VARCHAR(64) NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			created_at VARCHAR(32) NOT NULL
		)
		`,
		`CREATE INDEX IF NOT EXISTS idx_design_breakdowns_cat ON design_breakdowns (category, id)`,
		`CREATE INDEX IF NOT EXISTS idx_design_comments_bd ON design_breakdown_comments (breakdown_id, id)`,
	} {
		if _, err := m.db.Exec(q); err != nil {
			// MySQL 方言回落（AUTO_INCREMENT 主键 + ENGINE 声明）
			if !m.createMySQLDesignTables() {
				return fmt.Errorf("创建拆解表失败（尝试了 SQLite 和 MySQL 格式）: %w", err)
			}
			return nil
		}
	}
	return nil
}

// createMySQLDesignTables MySQL 侧建表（成功返回 true；任一失败返回 false 由
// 调用方保留 SQLite 错误）。
func (m *DesignModel) createMySQLDesignTables() bool {
	queries := []string{
		`
		CREATE TABLE IF NOT EXISTS design_breakdowns (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			game_title VARCHAR(64) NOT NULL,
			category VARCHAR(32) NOT NULL,
			summary TEXT,
			core_loop TEXT,
			progression TEXT,
			economy TEXT,
			combat TEXT,
			growth_lines TEXT,
			social TEXT,
			monetization TEXT,
			fairness TEXT,
			worth_stealing TEXT,
			pitfalls TEXT,
			sources TEXT,
			author_id BIGINT NOT NULL DEFAULT 0,
			author_name VARCHAR(64) NOT NULL DEFAULT '',
			featured TINYINT NOT NULL DEFAULT 0,
			created_at VARCHAR(32) NOT NULL,
			updated_at VARCHAR(32) NOT NULL,
			KEY idx_design_breakdowns_cat (category, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`,
		`
		CREATE TABLE IF NOT EXISTS design_breakdown_comments (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			breakdown_id BIGINT NOT NULL,
			author_id BIGINT NOT NULL DEFAULT 0,
			author_name VARCHAR(64) NOT NULL DEFAULT '',
			content TEXT,
			created_at VARCHAR(32) NOT NULL,
			KEY idx_design_comments_bd (breakdown_id, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`,
	}
	for _, q := range queries {
		if _, err := m.db.Exec(q); err != nil {
			return false
		}
	}
	return true
}

func designFields(b *DesignBreakdown) []any {
	return []any{b.GameTitle, b.Category, b.Summary, b.CoreLoop, b.Progression,
		b.Economy, b.Combat, b.GrowthLines, b.Social, b.Monetization,
		b.Fairness, b.WorthStealing, b.Pitfalls, b.Sources,
		b.AuthorId, b.AuthorName}
}

func scanDesignRow(row *sql.Row) (*DesignBreakdown, error) {
	b := &DesignBreakdown{}
	err := row.Scan(&b.Id, &b.GameTitle, &b.Category, &b.Summary, &b.CoreLoop,
		&b.Progression, &b.Economy, &b.Combat, &b.GrowthLines, &b.Social,
		&b.Monetization, &b.Fairness, &b.WorthStealing, &b.Pitfalls, &b.Sources,
		&b.AuthorId, &b.AuthorName, &b.Featured, &b.CreatedAt, &b.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrDesignNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询拆解失败: %w", err)
	}
	return b, nil
}

// Create 新建拆解（已登录用户即可发布；时间服务端落定）。
func (m *DesignModel) Create(b *DesignBreakdown) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	b.CreatedAt, b.UpdatedAt = now, now
	res, err := m.db.Exec(
		`INSERT INTO design_breakdowns (game_title, category, summary, core_loop, progression,
			economy, combat, growth_lines, social, monetization, fairness, worth_stealing,
			pitfalls, sources, author_id, author_name, featured, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		append(designFields(b), b.CreatedAt, b.UpdatedAt)...,
	)
	if err != nil {
		return 0, fmt.Errorf("写入拆解失败: %w", err)
	}
	return res.LastInsertId()
}

// Update 作者全量覆盖编辑（仅作者；时间刷新，featured 不经此路径）。
func (m *DesignModel) Update(id, authorId int64, b *DesignBreakdown) error {
	res, err := m.db.Exec(
		`UPDATE design_breakdowns SET game_title=?, category=?, summary=?, core_loop=?, progression=?,
		 economy=?, combat=?, growth_lines=?, social=?, monetization=?, fairness=?, worth_stealing=?,
		 pitfalls=?, sources=?, updated_at=? WHERE id=? AND author_id=?`,
		b.GameTitle, b.Category, b.Summary, b.CoreLoop, b.Progression,
		b.Economy, b.Combat, b.GrowthLines, b.Social, b.Monetization,
		b.Fairness, b.WorthStealing, b.Pitfalls, b.Sources,
		time.Now().UTC().Format(time.RFC3339), id, authorId,
	)
	if err != nil {
		return fmt.Errorf("更新拆解失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrDesignNotFound // 不区分「不存在」与「非作者」，避免泄露可编辑性
	}
	return nil
}

// Get 单篇详情（公开读，含未精选）。
func (m *DesignModel) Get(id int64) (*DesignBreakdown, error) {
	row := m.db.QueryRow(`SELECT `+designColumns+` FROM design_breakdowns WHERE id = ?`, id)
	return scanDesignRow(row)
}

// List 品类聚合页/精选列表（category 空 = 全品类；featured=true 只看精选）。
// 返回按 updated_at 倒序 + total。
func (m *DesignModel) List(category string, featured bool, limit, offset int64) ([]*DesignBreakdown, int64, error) {
	where := ` WHERE 1=1`
	args := []any{}
	if category != "" {
		where += ` AND category = ?`
		args = append(args, category)
	}
	if featured {
		where += ` AND featured = 1`
	}
	var total int64
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM design_breakdowns`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计拆解数失败: %w", err)
	}
	q := `SELECT ` + designColumns + ` FROM design_breakdowns` + where +
		` ORDER BY featured DESC, updated_at DESC, id DESC LIMIT ? OFFSET ?`
	rows, err := m.db.Query(q, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询拆解列表失败: %w", err)
	}
	defer rows.Close()
	out := []*DesignBreakdown{}
	for rows.Next() {
		b := &DesignBreakdown{}
		if err := rows.Scan(&b.Id, &b.GameTitle, &b.Category, &b.Summary, &b.CoreLoop,
			&b.Progression, &b.Economy, &b.Combat, &b.GrowthLines, &b.Social,
			&b.Monetization, &b.Fairness, &b.WorthStealing, &b.Pitfalls, &b.Sources,
			&b.AuthorId, &b.AuthorName, &b.Featured, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
}

// Categories 品类聚合行（品类页入口；按白名单顺序返回，含 0 行品类）。
func (m *DesignModel) Categories() ([]DesignCategoryRow, error) {
	rows, err := m.db.Query(
		`SELECT category, COUNT(*) FROM design_breakdowns GROUP BY category`)
	if err != nil {
		return nil, fmt.Errorf("聚合品类失败: %w", err)
	}
	defer rows.Close()
	counts := map[string]int64{}
	for rows.Next() {
		var c string
		var n int64
		if err := rows.Scan(&c, &n); err != nil {
			return nil, err
		}
		counts[c] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]DesignCategoryRow, 0, len(DesignCategories))
	for _, c := range DesignCategories {
		out = append(out, DesignCategoryRow{Category: c, Count: counts[c]})
	}
	return out, nil
}

// SetFeatured 精选运营位（仅审核管理员，幂等）。
func (m *DesignModel) SetFeatured(id int64, featured bool) error {
	res, err := m.db.Exec(`UPDATE design_breakdowns SET featured = ?, updated_at = ? WHERE id = ?`,
		featured, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("设置精选失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrDesignNotFound
	}
	return nil
}

// AddComment 讨论区发评论。
func (m *DesignModel) AddComment(c *DesignComment) (int64, error) {
	res, err := m.db.Exec(
		`INSERT INTO design_breakdown_comments (breakdown_id, author_id, author_name, content, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		c.BreakdownId, c.AuthorId, c.AuthorName, c.Content,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("写入拆解评论失败: %w", err)
	}
	return res.LastInsertId()
}

// ListComments 讨论区评论列表（id 升序，会话式阅读顺序）。
func (m *DesignModel) ListComments(breakdownId int64, limit, offset int64) ([]*DesignComment, error) {
	rows, err := m.db.Query(
		`SELECT id, breakdown_id, author_id, author_name, content, created_at
		 FROM design_breakdown_comments WHERE breakdown_id = ? ORDER BY id ASC LIMIT ? OFFSET ?`,
		breakdownId, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("查询拆解评论失败: %w", err)
	}
	defer rows.Close()
	out := []*DesignComment{}
	for rows.Next() {
		c := &DesignComment{}
		if err := rows.Scan(&c.Id, &c.BreakdownId, &c.AuthorId, &c.AuthorName, &c.Content, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteComment 删除自己的评论（仅作者本人；他人按不存在处理）。
func (m *DesignModel) DeleteComment(id, authorId int64) error {
	res, err := m.db.Exec(
		`DELETE FROM design_breakdown_comments WHERE id = ? AND author_id = ?`, id, authorId)
	if err != nil {
		return fmt.Errorf("删除拆解评论失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrDesignCommentNotFound
	}
	return nil
}
