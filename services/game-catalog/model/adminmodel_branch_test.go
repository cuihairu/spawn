package model

// adminmodel.go 补漏分支：CreateTables 的 MySQL 回退与双方言皆败臂（关库）、
// 迁移函数 SHOW COLUMNS 枚举失败跳过臂（关库）与缺列 ALTER 臂（预建无列表）、
// 各查询/写入失败臂（关库）、守卫臂（空参/非法状态/nil 审计）、脏行 Scan 失败、
// DecideReview 的 RowsAffected==0 未决不存在臂、UpdateVersionPackage 的锁定臂
// 与 UPDATE 失败臂（RAISE(ABORT) 触发器）。MySQL SHOW COLUMNS 成功路径、
// LastInsertId/RowsAffected 失败臂、COUNT 成功后再查询失败臂（同库无法单独
// 使其失败）与迁移函数永不返回错误的 return err 臂属登记不可构造家族。

import (
	"database/sql"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// 关库下 CreateTables：首表 SQLite 建表失败 → MySQL 回退 → 再败 → 返回
// 「尝试了 SQLite 和 MySQL 格式」。
func TestAdminModel_CreateTablesBothDialectsFail(t *testing.T) {
	m, db := newTestAdminModelDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := m.CreateAdminTables(); err == nil ||
		!strings.Contains(err.Error(), "尝试了 SQLite 和 MySQL 格式") {
		t.Fatalf("CreateAdminTables on closed db = %v, want 尝试了 SQLite 和 MySQL 格式", err)
	}
}

// 迁移函数关库：PRAGMA 失败 → SHOW COLUMNS 枚举失败 → 跳过不阻断（return nil）。
func TestAdminModel_MigrateSHOWColumnsClosedDB(t *testing.T) {
	m, db := newTestAdminModelDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := m.migrateServerColumns(); err != nil {
		t.Fatalf("migrateServerColumns closed db = %v, want nil (skip)", err)
	}
	if err := m.migrateVersionColumns(); err != nil {
		t.Fatalf("migrateVersionColumns closed db = %v, want nil (skip)", err)
	}
	if err := m.migrateUserReviewColumns(); err != nil {
		t.Fatalf("migrateUserReviewColumns closed db = %v, want nil (skip)", err)
	}
}

// 迁移函数缺列 ALTER 臂：预建不含待补列的表 → ALTER ADD COLUMN 生效。
func TestAdminModel_MigrateAlterMissingColumns(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/mig.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	m := NewAdminModel(db)

	if _, err := db.Exec(`CREATE TABLE game_servers (id INTEGER PRIMARY KEY, name VARCHAR(64) NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("create game_servers: %v", err)
	}
	if err := m.migrateServerColumns(); err != nil {
		t.Fatalf("migrateServerColumns: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO game_servers (name) VALUES ('x')`); err != nil {
		t.Fatalf("insert after migrate: %v", err)
	}

	if _, err := db.Exec(`CREATE TABLE game_versions (id VARCHAR(64) PRIMARY KEY, game_id VARCHAR(64) NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("create game_versions: %v", err)
	}
	if err := m.migrateVersionColumns(); err != nil {
		t.Fatalf("migrateVersionColumns: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO game_versions (id, game_id) VALUES ('v-mig', 'g-mig')`); err != nil {
		t.Fatalf("insert version after migrate: %v", err)
	}

	if _, err := db.Exec(`CREATE TABLE game_user_reviews (id INTEGER PRIMARY KEY, game_id VARCHAR(64) NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("create game_user_reviews: %v", err)
	}
	if err := m.migrateUserReviewColumns(); err != nil {
		t.Fatalf("migrateUserReviewColumns: %v", err)
	}
}

// 守卫臂：空参 / 非法状态 / nil 审计。
func TestAdminModel_Guards(t *testing.T) {
	m := newTestAdminModel(t)

	if _, err := m.CreatePartner("   ", "e@x.com"); err == nil {
		t.Fatal("CreatePartner blank name = nil error, want error")
	}
	if _, err := m.SetPartnerStatus(1, "weird"); err == nil {
		t.Fatal("SetPartnerStatus invalid = nil error, want error")
	}
	if _, err := m.AssignOwnership("   ", 1, 1); err == nil {
		t.Fatal("AssignOwnership blank game = nil error, want error")
	}
	if _, err := m.DecideReview(1, 1, "maybe", ""); err == nil {
		t.Fatal("DecideReview invalid decision = nil error, want error")
	}
	if _, err := m.SetVersionStatus("v", "maybe", ""); err == nil {
		t.Fatal("SetVersionStatus invalid = nil error, want error")
	}
	if _, err := m.CreateVersion(&GameVersion{}); err == nil {
		t.Fatal("CreateVersion empty = nil error, want error")
	}
	if err := m.WriteAudit(nil); err == nil {
		t.Fatal("WriteAudit nil entry = nil error, want error")
	}
}

// 关库：所有无前置 SELECT 的查询/写入失败臂，以及先 SELECT 再失败返回的臂。
func TestAdminModel_ClosedDBErrors(t *testing.T) {
	m, db := newTestAdminModelDB(t)
	db.Close()

	if _, err := m.GetRole(7); err == nil {
		t.Fatal("GetRole closed db = nil error, want error")
	}
	if _, _, err := m.GetRoleWithPartner(7); err == nil {
		t.Fatal("GetRoleWithPartner closed db = nil error, want error")
	}
	if err := m.SetRole(7, RoleOperator, 0); err == nil {
		t.Fatal("SetRole closed db = nil error, want error")
	}
	if _, err := m.CreatePartner("n", "e"); err == nil {
		t.Fatal("CreatePartner closed db = nil error, want error")
	}
	if _, err := m.GetPartner(1); err == nil {
		t.Fatal("GetPartner closed db = nil error, want error")
	}
	if _, err := m.ListPartners(); err == nil {
		t.Fatal("ListPartners closed db = nil error, want error")
	}
	if _, err := m.SetPartnerStatus(1, PartnerActive); err == nil {
		t.Fatal("SetPartnerStatus closed db = nil error, want error")
	}
	if _, err := m.AssignOwnership("g-1", 1, 1); err == nil {
		t.Fatal("AssignOwnership closed db = nil error, want error")
	}
	if _, err := m.GetOwnership("g-1"); err == nil {
		t.Fatal("GetOwnership closed db = nil error, want error")
	}
	if _, err := m.ListGamesByPartner(1); err == nil {
		t.Fatal("ListGamesByPartner closed db = nil error, want error")
	}
	if _, err := m.SubmitReview("g-1", "", 1); err == nil {
		t.Fatal("SubmitReview closed db = nil error, want error")
	}
	if _, err := m.GetReview(1); err == nil {
		t.Fatal("GetReview closed db = nil error, want error")
	}
	if _, err := m.DecideReview(1, 1, ReviewApprove, ""); err == nil {
		t.Fatal("DecideReview closed db = nil error, want error")
	}
	if _, _, err := m.ListReviews("", 10, 0); err == nil {
		t.Fatal("ListReviews closed db = nil error, want error")
	}
	if _, err := m.CreateVersion(&GameVersion{GameId: "g-1", Version: "1.0.0"}); err == nil {
		t.Fatal("CreateVersion closed db = nil error, want error")
	}
	if _, err := m.GetVersion("v-x"); err == nil {
		t.Fatal("GetVersion closed db = nil error, want error")
	}
	if _, err := m.ListVersionsByGame("g-1"); err == nil {
		t.Fatal("ListVersionsByGame closed db = nil error, want error")
	}
	if _, err := m.SetVersionStatus("v-x", VersionStatusDraft, ""); err == nil {
		t.Fatal("SetVersionStatus closed db = nil error, want error")
	}
	if _, err := m.PublishVersion("v-x"); err == nil {
		t.Fatal("PublishVersion closed db = nil error, want error")
	}
	if _, err := m.RevokeVersion("v-x"); err == nil {
		t.Fatal("RevokeVersion closed db = nil error, want error")
	}
	if _, err := m.UpdateVersionPackage("v-x", "h", 1, "u"); err == nil {
		t.Fatal("UpdateVersionPackage closed db = nil error, want error")
	}
	if _, err := m.GetLatestPublishedVersion("g-1"); err == nil {
		t.Fatal("GetLatestPublishedVersion closed db = nil error, want error")
	}
	if err := m.WriteAudit(&AuditLog{ActorId: 1}); err == nil {
		t.Fatal("WriteAudit closed db = nil error, want error")
	}
	if _, _, err := m.ListAuditLogs("", 10, 0); err == nil {
		t.Fatal("ListAuditLogs closed db = nil error, want error")
	}
}

// 脏行 Scan 失败臂：破坏非主键 INTEGER 列（越过 SQLite 列亲和）。
func TestAdminModel_DirtyRowScans(t *testing.T) {
	m, db := newTestAdminModelDB(t)

	if _, err := m.SubmitReview("g-1", "v-1", 7); err != nil {
		t.Fatalf("submit review: %v", err)
	}
	if _, err := db.Exec(`UPDATE game_reviews SET submitted_by = 'abc' WHERE id = 1`); err != nil {
		t.Fatalf("corrupt submitted_by: %v", err)
	}
	if _, _, err := m.ListReviews("", 10, 0); err == nil {
		t.Fatal("ListReviews dirty submitted_by = nil error, want scan error")
	}
	if _, err := db.Exec(`UPDATE game_reviews SET submitted_by = 7 WHERE id = 1`); err != nil {
		t.Fatalf("restore submitted_by: %v", err)
	}

	if _, err := m.CreateVersion(&GameVersion{GameId: "g-1", Version: "1.0.0"}); err != nil {
		t.Fatalf("create version: %v", err)
	}
	if _, err := db.Exec(`UPDATE game_versions SET version_code = 'abc' WHERE id = (SELECT id FROM game_versions LIMIT 1)`); err != nil {
		t.Fatalf("corrupt version_code: %v", err)
	}
	if _, err := m.ListVersionsByGame("g-1"); err == nil {
		t.Fatal("ListVersionsByGame dirty version_code = nil error, want scan error")
	}
	if _, err := db.Exec(`UPDATE game_versions SET version_code = 0 WHERE id = (SELECT id FROM game_versions LIMIT 1)`); err != nil {
		t.Fatalf("restore version_code: %v", err)
	}

	if err := m.WriteAudit(&AuditLog{ActorId: 7}); err != nil {
		t.Fatalf("write audit: %v", err)
	}
	if _, err := db.Exec(`UPDATE audit_logs SET actor_id = 'abc' WHERE id = 1`); err != nil {
		t.Fatalf("corrupt actor_id: %v", err)
	}
	if _, _, err := m.ListAuditLogs("", 10, 0); err == nil {
		t.Fatal("ListAuditLogs dirty actor_id = nil error, want scan error")
	}
}

// 数据库中存了非法角色 → GetRoleWithPartner 回退 player 缺省（不报错）。
func TestAdminModel_InvalidRoleRow(t *testing.T) {
	m, db := newTestAdminModelDB(t)
	if _, err := db.Exec(`INSERT INTO platform_roles (user_id, role, partner_id, updated_at) VALUES (7, 'bogus', 0, ?)`, nowUTC()); err != nil {
		t.Fatalf("insert bogus role: %v", err)
	}
	role, partnerID, err := m.GetRoleWithPartner(7)
	if err != nil || role != RolePlayer || partnerID != 0 {
		t.Fatalf("GetRoleWithPartner bogus = %s/%d/%v, want player/0/nil", role, partnerID, err)
	}
}

// DecideReview 未决记录不存在：UPDATE 影响 0 行 → GetReview 报 ErrReviewNotFound
// → 原样返回（区别于「该提审已审结」）。
func TestAdminModel_DecideReviewNotFound(t *testing.T) {
	m := newTestAdminModel(t)
	_, err := m.DecideReview(9999, 1, ReviewApprove, "")
	if err == nil {
		t.Fatal("DecideReview on missing review = nil error, want error")
	}
}

// UpdateVersionPackage 已过审版本 → 包体锁定（ErrVersionPackageLocked）。
func TestAdminModel_UpdateVersionPackageLocked(t *testing.T) {
	m := newTestAdminModel(t)
	v, err := m.CreateVersion(&GameVersion{GameId: "g-1", Version: "1.0.0"})
	if err != nil {
		t.Fatalf("create version: %v", err)
	}
	if _, err := m.SetVersionStatus(v.Id, VersionStatusApproved, ""); err != nil {
		t.Fatalf("approve version: %v", err)
	}
	if _, err := m.UpdateVersionPackage(v.Id, "h", 1, "u"); err != ErrVersionPackageLocked {
		t.Fatalf("UpdateVersionPackage approved = %v, want ErrVersionPackageLocked", err)
	}
}

// UpdateVersionPackage 的 UPDATE 失败臂：Get 成功（draft）但 UPDATE 被触发器
// 中止 → 更新版本包体失败。
func TestAdminModel_UpdateVersionPackageExecFailed(t *testing.T) {
	m, db := newTestAdminModelDB(t)
	v, err := m.CreateVersion(&GameVersion{GameId: "g-1", Version: "1.0.0"})
	if err != nil {
		t.Fatalf("create version: %v", err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_ver_update BEFORE UPDATE ON game_versions
		BEGIN SELECT RAISE(ABORT, 'forced version update failure'); END`); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if _, err := m.UpdateVersionPackage(v.Id, "h", 1, "u"); err == nil ||
		!strings.Contains(err.Error(), "更新版本包体失败") {
		t.Fatalf("UpdateVersionPackage update failed = %v, want 更新版本包体失败", err)
	}
}
