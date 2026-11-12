package model

// GP-401 游戏评价（F04）与收藏（F05）：玩家评分、长评、评价举报、开发者
// 回复、收藏游戏。表建在 CreateAdminTables（与区服/预约同批），CRUD 在本
// 文件（servermodel.go 同款：方法挂 *AdminModel，归属校验在 logic 层）。
// 命名避开既有提审域（game_reviews/GameReview/ListReviews 为审核队列，
// 玩家长评一律 UserReview 前缀）。

import (
	"database/sql"
	"errors"
	"strings"
)

// 评价举报状态（community reports 同款三态：未决/驳回/已处置）。
const (
	ReviewReportPending   = "pending"
	ReviewReportDismissed = "dismissed"
	ReviewReportResolved  = "resolved"
)

// 哨兵 ErrUserReviewNotFound / ErrUserReviewExists / ErrReviewReportNotFound
// 与其余域哨兵同放 adminmodel.go。

// RatingSummary 评分聚合（1-5 星均值 + 人数；无评分 count=0 average=0）。
type RatingSummary struct {
	Average float64 `json:"average"`
	Count   int64   `json:"count"`
}

// UserReview 玩家长评（一游戏一评 UNIQUE(game_id,user_id)）；开发者回复
// 单条直接挂列（官方回复覆盖式更新，不建楼）。
type UserReview struct {
	Id         int64  `json:"id"`
	GameId     string `json:"game_id"`
	UserId     int64  `json:"user_id"`
	Rating     int    `json:"rating"` // 作者当前评分（0=未评分，列表 LEFT JOIN 评分表）
	Content    string `json:"content"`
	DevReply   string `json:"dev_reply"`
	DevReplyBy int64  `json:"dev_reply_by"`
	DevReplyAt string `json:"dev_reply_at"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// ReviewReport 长评举报（UNIQUE(reporter_id,review_id) 一人一评一报）。
type ReviewReport struct {
	Id         int64  `json:"id"`
	ReviewId   int64  `json:"review_id"`
	GameId     string `json:"game_id"`
	ReporterId int64  `json:"reporter_id"`
	Reason     string `json:"reason"`
	Status     string `json:"status"`
	HandledBy  int64  `json:"handled_by"`
	CreatedAt  string `json:"created_at"`
	HandledAt  string `json:"handled_at"`
}

// GameFavorite 收藏游戏（复合主键幂等）。
type GameFavorite struct {
	GameId    string `json:"game_id"`
	UserId    int64  `json:"user_id"`
	CreatedAt string `json:"created_at"`
}

// ---- 玩家评分（F04） ----

// UpsertUserRating 写/覆盖评分（1-5 由 logic 校验）。UPDATE 命中即覆盖，
// 未命中 INSERT，撞主键（并发首评）退回 UPDATE，双方言无差异。
func (m *AdminModel) UpsertUserRating(gameId string, userId int64, rating int) error {
	now := nowUTC()
	res, err := m.db.Exec(
		`UPDATE game_ratings SET rating = ?, updated_at = ? WHERE game_id = ? AND user_id = ?`,
		rating, now, gameId, userId,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return nil
	}
	if _, err := m.db.Exec(
		`INSERT INTO game_ratings (game_id, user_id, rating, updated_at) VALUES (?, ?, ?, ?)`,
		gameId, userId, rating, now,
	); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			_, err = m.db.Exec(
				`UPDATE game_ratings SET rating = ?, updated_at = ? WHERE game_id = ? AND user_id = ?`,
				rating, now, gameId, userId,
			)
		}
		return err
	}
	return nil
}

// GetRatingSummary 评分聚合（AVG 空集 NULL → 0）。
func (m *AdminModel) GetRatingSummary(gameId string) (*RatingSummary, error) {
	var count int64
	var avg sql.NullFloat64
	err := m.db.QueryRow(
		`SELECT COUNT(*), AVG(rating) FROM game_ratings WHERE game_id = ?`, gameId,
	).Scan(&count, &avg)
	if err != nil {
		return nil, err
	}
	s := &RatingSummary{Count: count}
	if avg.Valid {
		s.Average = avg.Float64
	}
	return s, nil
}

// GetUserRating 单人当前评分（未评分 0, nil）。
func (m *AdminModel) GetUserRating(gameId string, userId int64) (int, error) {
	var rating int
	err := m.db.QueryRow(
		`SELECT rating FROM game_ratings WHERE game_id = ? AND user_id = ?`, gameId, userId,
	).Scan(&rating)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return rating, nil
}

// ---- 玩家长评（F04） ----

const userReviewColumns = "id, game_id, user_id, content, dev_reply, dev_reply_by, dev_reply_at, created_at, updated_at"

// scanUserReview 附加尾列 rating（列表 JOIN 评分表 COALESCE 0，单查不 JOIN）。
func scanUserReview(r *UserReview) []any {
	return []any{&r.Id, &r.GameId, &r.UserId, &r.Content, &r.DevReply, &r.DevReplyBy, &r.DevReplyAt, &r.CreatedAt, &r.UpdatedAt}
}

func scanUserReviewWithRating(r *UserReview) []any {
	return append(scanUserReview(r), &r.Rating)
}

// CreateUserReview 发长评（一游戏一评，重复 409 语义由哨兵上抛）。
func (m *AdminModel) CreateUserReview(r *UserReview) (*UserReview, error) {
	now := nowUTC()
	res, err := m.db.Exec(
		`INSERT INTO game_user_reviews (game_id, user_id, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		r.GameId, r.UserId, r.Content, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, ErrUserReviewExists
		}
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	r.Id = id
	r.CreatedAt = now
	r.UpdatedAt = now
	return r, nil
}

// GetUserReview 按主键查（不带评分 JOIN；作者/归属校验用轻查询）。
func (m *AdminModel) GetUserReview(id int64) (*UserReview, error) {
	var r UserReview
	err := m.db.QueryRow(
		`SELECT `+userReviewColumns+` FROM game_user_reviews WHERE id = ?`, id,
	).Scan(scanUserReview(&r)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserReviewNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListUserReviewsByGame 公开长评列表（新在前，带作者评分 LEFT JOIN）。
func (m *AdminModel) ListUserReviewsByGame(gameId string, limit, offset int) ([]*UserReview, int, error) {
	var total int
	if err := m.db.QueryRow(
		`SELECT COUNT(*) FROM game_user_reviews WHERE game_id = ?`, gameId,
	).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := m.db.Query(
		`SELECT r.id, r.game_id, r.user_id, r.content, r.dev_reply, r.dev_reply_by, r.dev_reply_at, r.created_at, r.updated_at, COALESCE(g.rating, 0)
		 FROM game_user_reviews r LEFT JOIN game_ratings g ON g.game_id = r.game_id AND g.user_id = r.user_id
		 WHERE r.game_id = ? ORDER BY r.id DESC LIMIT ? OFFSET ?`,
		gameId, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]*UserReview, 0, 8)
	for rows.Next() {
		var r UserReview
		if err := rows.Scan(scanUserReviewWithRating(&r)...); err != nil {
			return nil, 0, err
		}
		out = append(out, &r)
	}
	return out, total, rows.Err()
}

// UpdateUserReview 作者改评（content 覆盖，updated_at 前移）。
func (m *AdminModel) UpdateUserReview(id int64, content string) (*UserReview, error) {
	now := nowUTC()
	res, err := m.db.Exec(
		`UPDATE game_user_reviews SET content = ?, updated_at = ? WHERE id = ?`,
		content, now, id,
	)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// MySQL RowsAffected 只计变更行：内容未变也视为更新成功，回读校验存在性
		if _, err := m.GetUserReview(id); err != nil {
			return nil, err
		}
	}
	return m.GetUserReview(id)
}

// DeleteUserReview 物理删除（管理处置「移除评价」）。
func (m *AdminModel) DeleteUserReview(id int64) error {
	_, err := m.db.Exec(`DELETE FROM game_user_reviews WHERE id = ?`, id)
	return err
}

// UpsertDevReply 开发者回复（覆盖式：同评一条官方回复，可更新）。
func (m *AdminModel) UpsertDevReply(id, by int64, content string) (*UserReview, error) {
	now := nowUTC()
	if _, err := m.db.Exec(
		`UPDATE game_user_reviews SET dev_reply = ?, dev_reply_by = ?, dev_reply_at = ?, updated_at = ? WHERE id = ?`,
		content, by, now, now, id,
	); err != nil {
		return nil, err
	}
	return m.GetUserReview(id)
}

// ---- 评价举报（F04） ----

const reviewReportColumns = "id, review_id, game_id, reporter_id, reason, status, handled_by, created_at, handled_at"

func scanReviewReport(r *ReviewReport) []any {
	return []any{&r.Id, &r.ReviewId, &r.GameId, &r.ReporterId, &r.Reason, &r.Status, &r.HandledBy, &r.CreatedAt, &r.HandledAt}
}

// CreateReviewReport 举报（UNIQUE(reporter_id,review_id) 幂等：重复举报返回
// 既有记录与 created=false，与开服预约同款 INSERT OR IGNORE 语义）。
func (m *AdminModel) CreateReviewReport(r *ReviewReport) (*ReviewReport, bool, error) {
	now := nowUTC()
	res, err := m.db.Exec(
		`INSERT OR IGNORE INTO review_reports (review_id, game_id, reporter_id, reason, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		r.ReviewId, r.GameId, r.ReporterId, r.Reason, ReviewReportPending, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "syntax") || strings.Contains(err.Error(), "near") {
			res, err = m.db.Exec(
				`INSERT IGNORE INTO review_reports (review_id, game_id, reporter_id, reason, status, created_at)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				r.ReviewId, r.GameId, r.ReporterId, r.Reason, ReviewReportPending, now,
			)
		}
		if err != nil {
			return nil, false, err
		}
	}
	if n, _ := res.RowsAffected(); n == 1 {
		if id, err := res.LastInsertId(); err == nil && id > 0 {
			r.Id = id
		}
		r.Status = ReviewReportPending
		r.CreatedAt = now
		return r, true, nil
	}
	existing, err := m.getReviewReport(r.ReporterId, r.ReviewId)
	return existing, false, err
}

// getReviewReport 按 (reporter_id, review_id) 查既有举报（幂等回读）。
func (m *AdminModel) getReviewReport(reporterId, reviewId int64) (*ReviewReport, error) {
	var r ReviewReport
	err := m.db.QueryRow(
		`SELECT `+reviewReportColumns+` FROM review_reports WHERE reporter_id = ? AND review_id = ?`,
		reporterId, reviewId,
	).Scan(scanReviewReport(&r)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrReviewReportNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// GetReviewReport 按主键查举报。
func (m *AdminModel) GetReviewReport(id int64) (*ReviewReport, error) {
	var r ReviewReport
	err := m.db.QueryRow(
		`SELECT `+reviewReportColumns+` FROM review_reports WHERE id = ?`, id,
	).Scan(scanReviewReport(&r)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrReviewReportNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListReviewReports 管理举报队列（status 空为全部，新在前）。
func (m *AdminModel) ListReviewReports(status string, limit, offset int) ([]*ReviewReport, int, error) {
	q := `SELECT COUNT(*) FROM review_reports`
	args := []any{}
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	var total int
	if err := m.db.QueryRow(q, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q = `SELECT ` + reviewReportColumns + ` FROM review_reports`
	if status != "" {
		q += ` WHERE status = ?`
	}
	q += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := m.db.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]*ReviewReport, 0, 8)
	for rows.Next() {
		var r ReviewReport
		if err := rows.Scan(scanReviewReport(&r)...); err != nil {
			return nil, 0, err
		}
		out = append(out, &r)
	}
	return out, total, rows.Err()
}

// DecideReviewReportIfPending 未决才可处置（TOCTOU 条件更新，返回命中与否）。
func (m *AdminModel) DecideReviewReportIfPending(id, handledBy int64, status string) (bool, error) {
	res, err := m.db.Exec(
		`UPDATE review_reports SET status = ?, handled_by = ?, handled_at = ? WHERE id = ? AND status = ?`,
		status, handledBy, nowUTC(), id, ReviewReportPending,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	// MySQL 只计变更行：pending→pending 不存在（状态必变），0 即非未决
	return n == 1, nil
}

// ResolvePendingReportsForReview 移除评价后同评其余未决举报批量结案
// （queue 不留指向已删评价的悬空未决项）。
func (m *AdminModel) ResolvePendingReportsForReview(reviewId, handledBy int64) error {
	_, err := m.db.Exec(
		`UPDATE review_reports SET status = ?, handled_by = ?, handled_at = ? WHERE review_id = ? AND status = ?`,
		ReviewReportResolved, handledBy, nowUTC(), reviewId, ReviewReportPending,
	)
	return err
}

// ---- 收藏游戏（F05） ----

// AddFavorite 收藏（幂等：重复收藏返回既有与 created=false）。
func (m *AdminModel) AddFavorite(gameId string, userId int64) (*GameFavorite, bool, error) {
	now := nowUTC()
	res, err := m.db.Exec(
		`INSERT OR IGNORE INTO game_favorites (game_id, user_id, created_at) VALUES (?, ?, ?)`,
		gameId, userId, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "syntax") || strings.Contains(err.Error(), "near") {
			res, err = m.db.Exec(
				`INSERT IGNORE INTO game_favorites (game_id, user_id, created_at) VALUES (?, ?, ?)`,
				gameId, userId, now,
			)
		}
		if err != nil {
			return nil, false, err
		}
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return &GameFavorite{GameId: gameId, UserId: userId, CreatedAt: now}, true, nil
	}
	existing, err := m.getFavorite(gameId, userId)
	return existing, false, err
}

func (m *AdminModel) getFavorite(gameId string, userId int64) (*GameFavorite, error) {
	var f GameFavorite
	err := m.db.QueryRow(
		`SELECT game_id, user_id, created_at FROM game_favorites WHERE game_id = ? AND user_id = ?`,
		gameId, userId,
	).Scan(&f.GameId, &f.UserId, &f.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// RemoveFavorite 取消收藏（未收藏也是成功，幂等）。
func (m *AdminModel) RemoveFavorite(gameId string, userId int64) error {
	_, err := m.db.Exec(`DELETE FROM game_favorites WHERE game_id = ? AND user_id = ?`, gameId, userId)
	return err
}

// ListMyFavorites 我的收藏（新在前）。
func (m *AdminModel) ListMyFavorites(userId int64, limit int) ([]*GameFavorite, error) {
	rows, err := m.db.Query(
		`SELECT game_id, user_id, created_at FROM game_favorites WHERE user_id = ? ORDER BY created_at DESC, game_id ASC LIMIT ?`,
		userId, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*GameFavorite, 0, 8)
	for rows.Next() {
		var f GameFavorite
		if err := rows.Scan(&f.GameId, &f.UserId, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &f)
	}
	return out, rows.Err()
}

// ListFavoriteUsersByGame 收藏人群（上线通知受众，去重自然成立）。
func (m *AdminModel) ListFavoriteUsersByGame(gameId string, limit int) ([]int64, error) {
	rows, err := m.db.Query(
		`SELECT user_id FROM game_favorites WHERE game_id = ? ORDER BY user_id LIMIT ?`,
		gameId, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]int64, 0, 16)
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		out = append(out, uid)
	}
	return out, rows.Err()
}
