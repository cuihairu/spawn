package model

// userreviewmodel.go 补漏分支：查询/写入失败臂（关库）、UpsertUserRating 与
// CreateUserReview 的 UNIQUE 冲突回退臂（RAISE(ABORT, 'UNIQUE ...') 触发器）、
// revive 路径的 UPDATE/Scan 失败臂、投票事务的 INSERT/DELETE 失败臂
// （RAISE(ABORT) 触发器）、RowsAffected==0 的回读失败臂、脏行 Scan 失败、
// reviewVotable 的非 NoRows 错误臂。MySQL INSERT IGNORE 回退与「syntax/near」
// 分支、LastInsertId 失败臂、COUNT 成功后再查询失败臂、迁移/聚合空集分支
// 属登记不可构造家族，见 coverage 台账。

import (
	"errors"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// makeReview 造一条长评并返回其 id（无投票）。
func makeReview(t *testing.T, m *AdminModel, gameId string, userId int64) int64 {
	t.Helper()
	r, err := m.CreateUserReview(&UserReview{GameId: gameId, UserId: userId, Content: "hi"})
	if err != nil {
		t.Fatalf("CreateUserReview: %v", err)
	}
	return r.Id
}

// 关库：所有直接 Exec/Query/Begin 的失败臂，以及无前置 SELECT 的写失败臂。
func TestUserReviewModel_ClosedDBErrors(t *testing.T) {
	m, db := newTestAdminModelDB(t)
	db.Close()

	if err := m.UpsertUserRating("g-1", 7, 5); err == nil {
		t.Fatal("UpsertUserRating closed db = nil error, want error")
	}
	if _, err := m.GetRatingSummary("g-1"); err == nil {
		t.Fatal("GetRatingSummary closed db = nil error, want error")
	}
	if _, err := m.GetUserRating("g-1", 7); err == nil {
		t.Fatal("GetUserRating closed db = nil error, want error")
	}
	if _, err := m.CreateUserReview(&UserReview{GameId: "g-1", UserId: 7, Content: "c"}); err == nil {
		t.Fatal("CreateUserReview closed db = nil error, want error")
	}
	if _, err := m.GetUserReview(1); err == nil {
		t.Fatal("GetUserReview closed db = nil error, want error")
	}
	if _, _, err := m.ListUserReviewsByGame("g-1", 10, 0, false); err == nil {
		t.Fatal("ListUserReviewsByGame closed db = nil error, want error")
	}
	if _, err := m.VoteReview("g-1", 1, 7); err == nil {
		t.Fatal("VoteReview closed db = nil error, want error")
	}
	if _, err := m.UnvoteReview("g-1", 1, 7); err == nil {
		t.Fatal("UnvoteReview closed db = nil error, want error")
	}
	if err := reviewVotable(db, "g-1", 1); err == nil {
		t.Fatal("reviewVotable closed db = nil error, want error")
	}
	if _, err := m.UpdateUserReview(1, "c"); err == nil {
		t.Fatal("UpdateUserReview closed db = nil error, want error")
	}
	if err := m.DeleteUserReview(1); err == nil {
		t.Fatal("DeleteUserReview closed db = nil error, want error")
	}
	if _, err := m.RestoreUserReview(1); err == nil {
		t.Fatal("RestoreUserReview closed db = nil error, want error")
	}
	if _, err := m.UpsertDevReply(1, 7, "c"); err == nil {
		t.Fatal("UpsertDevReply closed db = nil error, want error")
	}
	if _, _, err := m.CreateReviewReport(&ReviewReport{ReviewId: 1, GameId: "g-1", ReporterId: 7, Reason: "spam"}); err == nil {
		t.Fatal("CreateReviewReport closed db = nil error, want error")
	}
	if _, err := m.getReviewReport(7, 1); err == nil {
		t.Fatal("getReviewReport closed db = nil error, want error")
	}
	if _, err := m.GetReviewReport(1); err == nil {
		t.Fatal("GetReviewReport closed db = nil error, want error")
	}
	if _, _, err := m.ListReviewReports("", 10, 0); err == nil {
		t.Fatal("ListReviewReports closed db = nil error, want error")
	}
	if _, err := m.DecideReviewReportIfPending(1, 7, ReviewReportResolved); err == nil {
		t.Fatal("DecideReviewReportIfPending closed db = nil error, want error")
	}
	if err := m.ResolvePendingReportsForReview(1, 7); err == nil {
		t.Fatal("ResolvePendingReportsForReview closed db = nil error, want error")
	}
	if _, err := m.ListPendingReportersByReview(1); err == nil {
		t.Fatal("ListPendingReportersByReview closed db = nil error, want error")
	}
	if _, _, err := m.AddFavorite("g-1", 7); err == nil {
		t.Fatal("AddFavorite closed db = nil error, want error")
	}
	if _, err := m.getFavorite("g-1", 7); err == nil {
		t.Fatal("getFavorite closed db = nil error, want error")
	}
	if err := m.RemoveFavorite("g-1", 7); err == nil {
		t.Fatal("RemoveFavorite closed db = nil error, want error")
	}
	if _, err := m.ListMyFavorites(7, 10); err == nil {
		t.Fatal("ListMyFavorites closed db = nil error, want error")
	}
	if _, err := m.ListFavoriteUsersByGame("g-1", 10); err == nil {
		t.Fatal("ListFavoriteUsersByGame closed db = nil error, want error")
	}
}

// UpsertUserRating UNIQUE 冲突回退臂：UPDATE 命中 0 行（首次评分）后 INSERT
// 被触发器以 UNIQUE 消息中止 → 命中 strings.Contains("UNIQUE") 分支 → 走
// 回退 UPDATE，源码把 err 重赋为回退 UPDATE 结果（成功即 nil）。此路径是并发
// 首评竞态（UPDATE 与 INSERT 之间他人已写入），断言函数不报错即可。
func TestUserReviewModel_UpsertRatingUniqueTrigger(t *testing.T) {
	m, db := newTestAdminModelDB(t)
	if _, err := db.Exec(`CREATE TRIGGER fail_rating_insert BEFORE INSERT ON game_ratings
		BEGIN SELECT RAISE(ABORT, 'UNIQUE constraint failed: game_ratings.rating'); END`); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if err := m.UpsertUserRating("g-1", 7, 5); err != nil {
		t.Fatalf("UpsertUserRating unique conflict = %v, want nil", err)
	}
}

// CreateUserReview 重复提评（未软删）→ 唯一键冲突且复活路径命中 0 行 → ErrUserReviewExists。
func TestUserReviewModel_DuplicateCreateExists(t *testing.T) {
	m := newTestAdminModel(t)
	makeReview(t, m, "g-1", 7)
	if _, err := m.CreateUserReview(&UserReview{GameId: "g-1", UserId: 7, Content: "again"}); !errors.Is(err, ErrUserReviewExists) {
		t.Fatalf("duplicate create = %v, want ErrUserReviewExists", err)
	}
}

// revive 路径 UPDATE 失败臂：软删行存在但 UPDATE 被触发器中止 → 复活失败 →
// 返回 nil → 调用方按 ErrUserReviewExists 上抛。
func TestUserReviewModel_ReviveUpdateFailed(t *testing.T) {
	m, db := newTestAdminModelDB(t)
	makeReview(t, m, "g-1", 7)
	if err := m.DeleteUserReview(1); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_review_update BEFORE UPDATE ON game_user_reviews
		BEGIN SELECT RAISE(ABORT, 'forced review update failure'); END`); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if _, err := m.CreateUserReview(&UserReview{GameId: "g-1", UserId: 7, Content: "again"}); !errors.Is(err, ErrUserReviewExists) {
		t.Fatalf("revive update failed = %v, want ErrUserReviewExists", err)
	}
}

// revive 路径回读 Scan 失败臂：软删行 dev_reply_by 列（INTEGER）被污染 →
// 回读 Scan 报错 → 复活失败 → ErrUserReviewExists。
func TestUserReviewModel_ReviveGetScanFailed(t *testing.T) {
	m, db := newTestAdminModelDB(t)
	makeReview(t, m, "g-1", 7)
	if err := m.DeleteUserReview(1); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if _, err := db.Exec(`UPDATE game_user_reviews SET dev_reply_by = 'abc' WHERE id = 1`); err != nil {
		t.Fatalf("corrupt dev_reply_by: %v", err)
	}
	if _, err := m.CreateUserReview(&UserReview{GameId: "g-1", UserId: 7, Content: "again"}); !errors.Is(err, ErrUserReviewExists) {
		t.Fatalf("revive scan failed = %v, want ErrUserReviewExists", err)
	}
}

// VoteReview INSERT 失败臂：入票被 BEFORE INSERT 触发器中止 → 主 INSERT 与
// MySQL 回退均失败 → 返回回退错误。
func TestUserReviewModel_VoteInsertFailed(t *testing.T) {
	m, db := newTestAdminModelDB(t)
	makeReview(t, m, "g-1", 7)
	if _, err := db.Exec(`CREATE TRIGGER fail_vote_insert BEFORE INSERT ON review_votes
		BEGIN SELECT RAISE(ABORT, 'forced vote insert failure'); END`); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if _, err := m.VoteReview("g-1", 1, 7); err == nil {
		t.Fatal("VoteReview insert failed = nil error, want error")
	}
}

// UnvoteReview DELETE 失败臂：取消投票的 DELETE 被触发器中止 → 报错不 panic。
func TestUserReviewModel_UnvoteDeleteFailed(t *testing.T) {
	m, db := newTestAdminModelDB(t)
	makeReview(t, m, "g-1", 7)
	if _, err := m.VoteReview("g-1", 1, 7); err != nil {
		t.Fatalf("seed vote: %v", err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_vote_delete BEFORE DELETE ON review_votes
		BEGIN SELECT RAISE(ABORT, 'forced vote delete failure'); END`); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if _, err := m.UnvoteReview("g-1", 1, 7); err == nil {
		t.Fatal("UnvoteReview delete failed = nil error, want error")
	}
}

// UpdateUserReview：UPDATE 影响 0 行（不存在）→ 回读校验存在性 → 报错。
func TestUserReviewModel_UpdateReviewNotFound(t *testing.T) {
	m := newTestAdminModel(t)
	if _, err := m.UpdateUserReview(9999, "c"); err == nil {
		t.Fatal("UpdateUserReview missing = nil error, want error")
	}
}

// 脏行 Scan 失败臂：破坏非主键 INTEGER 列（越过 SQLite 列亲和），列表仍能
// 命中该行则 Scan 报错。
func TestUserReviewModel_CorruptScans(t *testing.T) {
	m, db := newTestAdminModelDB(t)

	makeReview(t, m, "g-1", 7)
	if _, err := db.Exec(`UPDATE game_user_reviews SET user_id = 'abc' WHERE id = 1`); err != nil {
		t.Fatalf("corrupt user_id: %v", err)
	}
	if _, _, err := m.ListUserReviewsByGame("g-1", 10, 0, false); err == nil {
		t.Fatal("ListUserReviewsByGame dirty user_id = nil error, want scan error")
	}
	if _, err := db.Exec(`UPDATE game_user_reviews SET user_id = 7 WHERE id = 1`); err != nil {
		t.Fatalf("restore user_id: %v", err)
	}

	if _, _, err := m.CreateReviewReport(&ReviewReport{ReviewId: 1, GameId: "g-1", ReporterId: 7, Reason: "spam"}); err != nil {
		t.Fatalf("create report: %v", err)
	}
	if _, err := db.Exec(`UPDATE review_reports SET reporter_id = 'abc' WHERE id = 1`); err != nil {
		t.Fatalf("corrupt reporter_id: %v", err)
	}
	if _, _, err := m.ListReviewReports("", 10, 0); err == nil {
		t.Fatal("ListReviewReports dirty reporter_id = nil error, want scan error")
	}
	if _, err := m.ListPendingReportersByReview(1); err == nil {
		t.Fatal("ListPendingReportersByReview dirty reporter_id = nil error, want scan error")
	}
}
