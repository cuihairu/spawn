package model

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func newTestAdminModel(t *testing.T) *AdminModel {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/admin.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	m := NewAdminModel(db)
	if err := m.CreateAdminTables(); err != nil {
		t.Fatalf("create admin tables: %v", err)
	}
	return m
}

func TestAdminModel_Roles(t *testing.T) {
	m := newTestAdminModel(t)

	// 未绑定 → player
	role, err := m.GetRole(42)
	if err != nil || role != RolePlayer {
		t.Fatalf("default role = %q/%v, want player", role, err)
	}

	// 绑定 operator
	if err := m.SetRole(42, RoleOperator, 0); err != nil {
		t.Fatalf("set role: %v", err)
	}
	if role, _ = m.GetRole(42); role != RoleOperator {
		t.Fatalf("role after set = %q, want operator", role)
	}

	// partner 绑定合作方
	if err := m.SetRole(7, RolePartner, 99); err != nil {
		t.Fatalf("set partner role: %v", err)
	}
	role, partner, err := m.GetRoleWithPartner(7)
	if err != nil || role != RolePartner || partner != 99 {
		t.Fatalf("partner binding = %q/%d/%v, want partner/99", role, partner, err)
	}

	// 非 partner 角色重置 partner_id 为 0
	if err := m.SetRole(7, RoleOperator, 99); err != nil {
		t.Fatalf("reassign role: %v", err)
	}
	if _, partner, _ = m.GetRoleWithPartner(7); partner != 0 {
		t.Fatalf("non-partner partner_id = %d, want 0", partner)
	}

	// 非法角色拒绝
	if err := m.SetRole(1, "superuser", 0); err == nil {
		t.Fatal("invalid role must error")
	}
}

func TestAdminModel_Partners(t *testing.T) {
	m := newTestAdminModel(t)

	p, err := m.CreatePartner("星辉互娱", "star@example.com")
	if err != nil {
		t.Fatalf("create partner: %v", err)
	}
	if p.Id <= 0 || p.Status != PartnerActive {
		t.Fatalf("partner = %+v, want active", p)
	}

	got, err := m.GetPartner(p.Id)
	if err != nil || got.Name != "星辉互娱" {
		t.Fatalf("get partner = %+v/%v", got, err)
	}

	partners, err := m.ListPartners()
	if err != nil || len(partners) != 1 {
		t.Fatalf("list partners = %d/%v, want 1", len(partners), err)
	}

	suspended, err := m.SetPartnerStatus(p.Id, PartnerSuspended)
	if err != nil || suspended.Status != PartnerSuspended {
		t.Fatalf("suspend = %+v/%v", suspended, err)
	}

	if _, err := m.GetPartner(9999); err != ErrPartnerNotFound {
		t.Fatalf("missing partner err = %v, want ErrPartnerNotFound", err)
	}
}

func TestAdminModel_OwnershipAndReviewFlow(t *testing.T) {
	m := newTestAdminModel(t)

	o, err := m.AssignOwnership("game-1", 3, 5)
	if err != nil || o.PartnerId != 3 || o.Status != OwnershipActive {
		t.Fatalf("assign = %+v/%v", o, err)
	}
	// 重指派覆盖
	if o, _ = m.AssignOwnership("game-1", 4, 5); o.PartnerId != 4 {
		t.Fatalf("reassign partner = %d, want 4", o.PartnerId)
	}
	ids, err := m.ListGamesByPartner(4)
	if err != nil || len(ids) != 1 || ids[0] != "game-1" {
		t.Fatalf("list by partner = %v/%v", ids, err)
	}

	// 提审 → 审核
	review, err := m.SubmitReview("game-1", "", 3)
	if err != nil || review.Status != ReviewPending {
		t.Fatalf("submit = %+v/%v", review, err)
	}
	decided, err := m.DecideReview(review.Id, 1, ReviewApprove, "")
	if err != nil || decided.Status != ReviewApprove || decided.ReviewerId != 1 {
		t.Fatalf("decide = %+v/%v", decided, err)
	}
	// 重复审核拒绝
	if _, err := m.DecideReview(review.Id, 1, ReviewApprove, ""); err == nil {
		t.Fatal("re-decide pending must error")
	}

	reviews, total, err := m.ListReviews(ReviewApprove, 20, 0)
	if err != nil || total != 1 || len(reviews) != 1 {
		t.Fatalf("list reviews = %d/%d/%v", len(reviews), total, err)
	}
}

func TestAdminModel_VersionsAndAudit(t *testing.T) {
	m := newTestAdminModel(t)

	v, err := m.CreateVersion("game-2", "1.0.0", "首个版本")
	if err != nil || v.Status != VersionStatusDraft {
		t.Fatalf("create version = %+v/%v", v, err)
	}
	if got, _ := m.GetVersion(v.Id); got.Version != "1.0.0" {
		t.Fatalf("get version = %+v", got)
	}
	if vv, _ := m.SetVersionStatus(v.Id, VersionStatusApproved, ""); vv.Status != VersionStatusApproved {
		t.Fatalf("set version status = %+v", vv)
	}
	versions, err := m.ListVersionsByGame("game-2")
	if err != nil || len(versions) != 1 {
		t.Fatalf("list versions = %d/%v", len(versions), err)
	}

	if err := m.WriteAudit(&AuditLog{ActorId: 9, ActorRole: RoleOperator, Action: "game_publish", ResourceType: "game", ResourceId: "game-2"}); err != nil {
		t.Fatalf("write audit: %v", err)
	}
	logs, total, err := m.ListAuditLogs("game", 20, 0)
	if err != nil || total != 1 || len(logs) != 1 || logs[0].ActorId != 9 {
		t.Fatalf("list audit = %d/%d/%v", len(logs), total, err)
	}
	if logs, _, _ := m.ListAuditLogs("partner", 20, 0); len(logs) != 0 {
		t.Fatalf("filtered audit should be empty, got %d", len(logs))
	}
}
