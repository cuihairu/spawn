package logic

import (
	"context"
	"errors"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/notify"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"
)

// failingOpsStore GP-103 管理面 logic 错误分支注入：内嵌真实仓储，仅按字段
// 覆写指定方法；未覆写方法委托真实仓储，种子数据照常可用。与
// userreviewlogic_test.go 的 failingAdminStore 同款模式、不同覆写面。
type failingOpsStore struct {
	model.AdminStore
	getPartnerErr         error
	setRoleErr            error
	createPartnerErr      error
	listPartnersErr       error
	setPartnerStatusErr   error
	listReviewsErr        error
	getReviewErr          error
	decideReviewErr       error
	listVersionsErr       error
	getVersionErr         error
	publishVersionErr     error
	revokeVersionErr      error
	listAuditErr          error
	getRoleWithPartnerErr error
	listFavUsersErr       error
	listResvUsersErr      error
	writeAuditErr         error
	assignErr             error
}

var _ model.AdminStore = (*failingOpsStore)(nil)

func (f *failingOpsStore) GetPartner(id int64) (*model.Partner, error) {
	if f.getPartnerErr != nil {
		return nil, f.getPartnerErr
	}
	return f.AdminStore.GetPartner(id)
}

func (f *failingOpsStore) SetRole(userId int64, role string, partnerId int64) error {
	if f.setRoleErr != nil {
		return f.setRoleErr
	}
	return f.AdminStore.SetRole(userId, role, partnerId)
}

func (f *failingOpsStore) CreatePartner(name, contactEmail string) (*model.Partner, error) {
	if f.createPartnerErr != nil {
		return nil, f.createPartnerErr
	}
	return f.AdminStore.CreatePartner(name, contactEmail)
}

func (f *failingOpsStore) ListPartners() ([]*model.Partner, error) {
	if f.listPartnersErr != nil {
		return nil, f.listPartnersErr
	}
	return f.AdminStore.ListPartners()
}

func (f *failingOpsStore) SetPartnerStatus(id int64, status string) (*model.Partner, error) {
	if f.setPartnerStatusErr != nil {
		return nil, f.setPartnerStatusErr
	}
	return f.AdminStore.SetPartnerStatus(id, status)
}

func (f *failingOpsStore) ListReviews(status string, limit, offset int) ([]*model.GameReview, int, error) {
	if f.listReviewsErr != nil {
		return nil, 0, f.listReviewsErr
	}
	return f.AdminStore.ListReviews(status, limit, offset)
}

func (f *failingOpsStore) GetReview(id int64) (*model.GameReview, error) {
	if f.getReviewErr != nil {
		return nil, f.getReviewErr
	}
	return f.AdminStore.GetReview(id)
}

func (f *failingOpsStore) DecideReview(id, reviewerId int64, decision, reason string) (*model.GameReview, error) {
	if f.decideReviewErr != nil {
		return nil, f.decideReviewErr
	}
	return f.AdminStore.DecideReview(id, reviewerId, decision, reason)
}

func (f *failingOpsStore) ListVersionsByGame(gameId string) ([]*model.GameVersion, error) {
	if f.listVersionsErr != nil {
		return nil, f.listVersionsErr
	}
	return f.AdminStore.ListVersionsByGame(gameId)
}

func (f *failingOpsStore) GetVersion(id string) (*model.GameVersion, error) {
	if f.getVersionErr != nil {
		return nil, f.getVersionErr
	}
	return f.AdminStore.GetVersion(id)
}

func (f *failingOpsStore) PublishVersion(id string) (*model.GameVersion, error) {
	if f.publishVersionErr != nil {
		return nil, f.publishVersionErr
	}
	return f.AdminStore.PublishVersion(id)
}

func (f *failingOpsStore) RevokeVersion(id string) (*model.GameVersion, error) {
	if f.revokeVersionErr != nil {
		return nil, f.revokeVersionErr
	}
	return f.AdminStore.RevokeVersion(id)
}

func (f *failingOpsStore) ListAuditLogs(resourceType string, limit, offset int) ([]*model.AuditLog, int, error) {
	if f.listAuditErr != nil {
		return nil, 0, f.listAuditErr
	}
	return f.AdminStore.ListAuditLogs(resourceType, limit, offset)
}

func (f *failingOpsStore) GetRoleWithPartner(userId int64) (string, int64, error) {
	if f.getRoleWithPartnerErr != nil {
		return "", 0, f.getRoleWithPartnerErr
	}
	return f.AdminStore.GetRoleWithPartner(userId)
}

func (f *failingOpsStore) ListFavoriteUsersByGame(gameId string, limit int) ([]int64, error) {
	if f.listFavUsersErr != nil {
		return nil, f.listFavUsersErr
	}
	return f.AdminStore.ListFavoriteUsersByGame(gameId, limit)
}

func (f *failingOpsStore) ListReservationUsersByGame(gameId string, limit int) ([]int64, error) {
	if f.listResvUsersErr != nil {
		return nil, f.listResvUsersErr
	}
	return f.AdminStore.ListReservationUsersByGame(gameId, limit)
}

func (f *failingOpsStore) WriteAudit(l *model.AuditLog) error {
	if f.writeAuditErr != nil {
		return f.writeAuditErr
	}
	return f.AdminStore.WriteAudit(l)
}

func (f *failingOpsStore) AssignOwnership(gameId string, partnerId, actorId int64) (*model.GameOwnership, error) {
	if f.assignErr != nil {
		return nil, f.assignErr
	}
	return f.AdminStore.AssignOwnership(gameId, partnerId, actorId)
}

// fakePublisher GP-306 通知口假实现：Enabled 可配，Publish 计数可断言。
type fakePublisher struct {
	enabled bool
	calls   int
}

func (f *fakePublisher) Publish(n notify.Notification) { f.calls++ }
func (f *fakePublisher) Enabled() bool                 { return f.enabled }

// newAdminLogicCtx 管理面 logic 测试上下文：真实 SQLite 仓储 + 一个合作方
// （id=1）+ 一条 g-rpg-1 的资料级提审（version 空）+ 一条 g-rpg-1 的版本；
// 返回提审 id 与版本 id。
func newAdminLogicCtx(t *testing.T) (*svc.ServiceContext, int64, string) {
	t.Helper()
	svcCtx := newLogicSvcCtx(t)
	admin := svcCtx.AdminRepository
	if _, err := admin.CreatePartner("Acme", "acme@x.com"); err != nil {
		t.Fatalf("CreatePartner: %v", err)
	}
	r, err := admin.SubmitReview("g-rpg-1", "", 8)
	if err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	v, err := admin.CreateVersion(&model.GameVersion{GameId: "g-rpg-1", Version: "1.0.0"})
	if err != nil {
		t.Fatalf("CreateVersion: %v", err)
	}
	return svcCtx, r.Id, v.Id
}

// adminCtx 带平台账号上下文的 context（audit/actor 取 ctx 值）。
func adminCtx(userID int64, role string) context.Context {
	ctx := context.WithValue(context.Background(), "user_id", userID)
	ctx = context.WithValue(ctx, "role", role)
	ctx = context.WithValue(ctx, "client_ip", "127.0.0.1")
	ctx = context.WithValue(ctx, "username", "admin")
	return ctx
}

// 授权角色：用户 id 非法 → 400；合作方绑定查不到 → 404；查询失败 → 500；
// 写入失败 → 500；成功（审计失败仅记日志）→ 200。
func TestAdminLogic_GrantRoleErrors(t *testing.T) {
	t.Run("user id invalid 400", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.GrantRole(&types.GrantRoleRequest{UserId: 0, Role: model.RolePlatformAdmin})
		requireAPIStatus(t, err, 400)
	})
	t.Run("partner not found 404", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.GrantRole(&types.GrantRoleRequest{UserId: 5, Role: model.RolePartner, PartnerId: 99})
		requireAPIStatus(t, err, 404)
	})
	t.Run("get partner failed 500", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:    svcCtx.AdminRepository,
			getPartnerErr: errors.New("partner down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.GrantRole(&types.GrantRoleRequest{UserId: 5, Role: model.RolePartner, PartnerId: 1})
		requireAPIStatus(t, err, 500)
	})
	t.Run("set role failed 500", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore: svcCtx.AdminRepository,
			setRoleErr: errors.New("role down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.GrantRole(&types.GrantRoleRequest{UserId: 5, Role: model.RolePartner, PartnerId: 1})
		requireAPIStatus(t, err, 500)
	})
	t.Run("audit failed tolerated", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:    svcCtx.AdminRepository,
			writeAuditErr: errors.New("audit down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		resp, err := l.GrantRole(&types.GrantRoleRequest{UserId: 5, Role: model.RolePartner, PartnerId: 1})
		if err != nil || resp == nil {
			t.Fatalf("audit failure should be tolerated: resp=%+v err=%v", resp, err)
		}
	})
}

// 建合作方：名称空 → 400；落库失败 → 500。
func TestAdminLogic_CreatePartnerErrors(t *testing.T) {
	t.Run("empty name 400", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.CreatePartner(&types.CreatePartnerRequest{Name: "   "})
		requireAPIStatus(t, err, 400)
	})
	t.Run("create failed 500", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:       svcCtx.AdminRepository,
			createPartnerErr: errors.New("create down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.CreatePartner(&types.CreatePartnerRequest{Name: "Beta", ContactEmail: "b@x.com"})
		requireAPIStatus(t, err, 500)
	})
}

// 合作方列表查询失败 → 500。
func TestAdminLogic_ListPartnersFailed500(t *testing.T) {
	svcCtx, _, _ := newAdminLogicCtx(t)
	svcCtx.AdminRepository = &failingOpsStore{
		AdminStore:      svcCtx.AdminRepository,
		listPartnersErr: errors.New("list down"),
	}
	l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
	_, err := l.ListPartners()
	requireAPIStatus(t, err, 500)
}

// 合作方状态：查不到 → 404；查询失败 → 500；写入失败 → 400。
func TestAdminLogic_SetPartnerStatusErrors(t *testing.T) {
	t.Run("not found 404", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:    svcCtx.AdminRepository,
			getPartnerErr: model.ErrPartnerNotFound,
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.SetPartnerStatus(&types.SetPartnerStatusRequest{Id: 99, Status: "suspended"})
		requireAPIStatus(t, err, 404)
	})
	t.Run("get failed 500", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:    svcCtx.AdminRepository,
			getPartnerErr: errors.New("partner down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.SetPartnerStatus(&types.SetPartnerStatusRequest{Id: 1, Status: "suspended"})
		requireAPIStatus(t, err, 500)
	})
	t.Run("write failed 400", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:          svcCtx.AdminRepository,
			setPartnerStatusErr: errors.New("invalid status"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.SetPartnerStatus(&types.SetPartnerStatusRequest{Id: 1, Status: "bogus"})
		requireAPIStatus(t, err, 400)
	})
}

// 归属游戏：游戏查不到 → 404；合作方查不到 → 404；写入失败 → 500。
func TestAdminLogic_AssignOwnershipErrors(t *testing.T) {
	t.Run("game not found 404", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.GameRepository = &failingGameStore{
			GameStore: svcCtx.GameRepository,
			getErr:    model.ErrGameNotFound,
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.AssignOwnership(&types.AssignOwnershipRequest{Id: "g-ghost", PartnerId: 1})
		requireAPIStatus(t, err, 404)
	})
	t.Run("partner not found 404", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:    svcCtx.AdminRepository,
			getPartnerErr: model.ErrPartnerNotFound,
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.AssignOwnership(&types.AssignOwnershipRequest{Id: "g-rpg-1", PartnerId: 99})
		requireAPIStatus(t, err, 404)
	})
	t.Run("assign failed 500", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore: svcCtx.AdminRepository,
			assignErr:  errors.New("assign down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.AssignOwnership(&types.AssignOwnershipRequest{Id: "g-rpg-1", PartnerId: 1})
		requireAPIStatus(t, err, 500)
	})
}

// 审核队列查询失败 → 500。
func TestAdminLogic_ListReviewsFailed500(t *testing.T) {
	svcCtx, _, _ := newAdminLogicCtx(t)
	svcCtx.AdminRepository = &failingOpsStore{
		AdminStore:     svcCtx.AdminRepository,
		listReviewsErr: errors.New("queue down"),
	}
	l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
	_, err := l.ListReviews(&types.ListReviewsRequest{})
	requireAPIStatus(t, err, 500)
}

// 审核决定：提审查不到 → 404；查询失败 → 500；已审结 → 409；更新失败 → 500。
func TestAdminLogic_DecideReviewErrors(t *testing.T) {
	t.Run("not found 404", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:   svcCtx.AdminRepository,
			getReviewErr: model.ErrReviewNotFound,
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.DecideReview(&types.DecideReviewRequest{Id: 99, Decision: model.ReviewApprove})
		requireAPIStatus(t, err, 404)
	})
	t.Run("query failed 500", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:   svcCtx.AdminRepository,
			getReviewErr: errors.New("query down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.DecideReview(&types.DecideReviewRequest{Id: 99, Decision: model.ReviewApprove})
		requireAPIStatus(t, err, 500)
	})
	t.Run("already decided 409", func(t *testing.T) {
		svcCtx, reviewID, _ := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:      svcCtx.AdminRepository,
			decideReviewErr: errors.New("该提审已审结"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.DecideReview(&types.DecideReviewRequest{Id: reviewID, Decision: model.ReviewApprove})
		requireAPIStatus(t, err, 409)
	})
	t.Run("update failed 500", func(t *testing.T) {
		svcCtx, reviewID, _ := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:      svcCtx.AdminRepository,
			decideReviewErr: errors.New("update down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.DecideReview(&types.DecideReviewRequest{Id: reviewID, Decision: model.ReviewApprove})
		requireAPIStatus(t, err, 500)
	})
	t.Run("invalid decision 400", func(t *testing.T) {
		svcCtx, reviewID, _ := newAdminLogicCtx(t)
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.DecideReview(&types.DecideReviewRequest{Id: reviewID, Decision: "bogus"})
		requireAPIStatus(t, err, 400)
	})
}

// 版本列表查询失败 → 500。
func TestAdminLogic_ListVersionsFailed500(t *testing.T) {
	svcCtx, _, _ := newAdminLogicCtx(t)
	svcCtx.AdminRepository = &failingOpsStore{
		AdminStore:      svcCtx.AdminRepository,
		listVersionsErr: errors.New("list down"),
	}
	l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
	_, err := l.ListVersions("g-rpg-1")
	requireAPIStatus(t, err, 500)
}

// 发布版本：版本查不到 → 404；查询失败 → 500；版本不属于该游戏 → 400；
// 未过审 → 400；低于已发布 → 409；更新失败 → 500；成功走版本通知。
func TestAdminLogic_PublishVersionErrors(t *testing.T) {
	t.Run("version not found 404", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:    svcCtx.AdminRepository,
			getVersionErr: model.ErrVersionNotFound,
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.PublishVersion("g-rpg-1", "ver-ghost")
		requireAPIStatus(t, err, 404)
	})
	t.Run("get failed 500", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:    svcCtx.AdminRepository,
			getVersionErr: errors.New("query down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.PublishVersion("g-rpg-1", "ver-ghost")
		requireAPIStatus(t, err, 500)
	})
	t.Run("wrong game 400", func(t *testing.T) {
		svcCtx, _, versionID := newAdminLogicCtx(t)
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.PublishVersion("g-rpg-2", versionID)
		requireAPIStatus(t, err, 400)
	})
	t.Run("not approved 400", func(t *testing.T) {
		svcCtx, _, versionID := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:        svcCtx.AdminRepository,
			publishVersionErr: errors.New("仅已过审版本可发布"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.PublishVersion("g-rpg-1", versionID)
		requireAPIStatus(t, err, 400)
	})
	t.Run("below current 409", func(t *testing.T) {
		svcCtx, _, versionID := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:        svcCtx.AdminRepository,
			publishVersionErr: errors.New("不能发布低于当前已发布版本的渠道包"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.PublishVersion("g-rpg-1", versionID)
		requireAPIStatus(t, err, 409)
	})
	t.Run("update failed 500", func(t *testing.T) {
		svcCtx, _, versionID := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:        svcCtx.AdminRepository,
			publishVersionErr: errors.New("publish down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.PublishVersion("g-rpg-1", versionID)
		requireAPIStatus(t, err, 500)
	})
	t.Run("success notifies version 200", func(t *testing.T) {
		svcCtx, _, versionID := newAdminLogicCtx(t)
		admin := svcCtx.AdminRepository
		if _, err := admin.SetVersionStatus(versionID, model.VersionStatusApproved, ""); err != nil {
			t.Fatalf("SetVersionStatus: %v", err)
		}
		pub := &fakePublisher{enabled: true}
		svcCtx.Notify = pub
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:       svcCtx.AdminRepository,
			listResvUsersErr: errors.New("resv down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		if _, err := l.PublishVersion("g-rpg-1", versionID); err != nil {
			t.Fatalf("publish success expected: %v", err)
		}
	})
}

// 撤回版本：未发布 → 400；更新失败 → 500。
func TestAdminLogic_RevokeVersionErrors(t *testing.T) {
	t.Run("not published 400", func(t *testing.T) {
		svcCtx, _, versionID := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:       svcCtx.AdminRepository,
			revokeVersionErr: errors.New("仅已发布版本可撤回"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.RevokeVersion("g-rpg-1", versionID)
		requireAPIStatus(t, err, 400)
	})
	t.Run("revoke failed 500", func(t *testing.T) {
		svcCtx, _, versionID := newAdminLogicCtx(t)
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:       svcCtx.AdminRepository,
			revokeVersionErr: errors.New("revoke down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.RevokeVersion("g-rpg-1", versionID)
		requireAPIStatus(t, err, 500)
	})
}

// 审计列表查询失败 → 500。
func TestAdminLogic_ListAuditLogsFailed500(t *testing.T) {
	svcCtx, _, _ := newAdminLogicCtx(t)
	svcCtx.AdminRepository = &failingOpsStore{
		AdminStore:   svcCtx.AdminRepository,
		listAuditErr: errors.New("audit down"),
	}
	l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
	_, err := l.ListAuditLogs(&types.ListAuditRequest{})
	requireAPIStatus(t, err, 500)
}

// Me：角色查询失败 → 500。
func TestAdminLogic_MeFailed500(t *testing.T) {
	svcCtx, _, _ := newAdminLogicCtx(t)
	svcCtx.AdminRepository = &failingOpsStore{
		AdminStore:            svcCtx.AdminRepository,
		getRoleWithPartnerErr: errors.New("role down"),
	}
	l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
	_, err := l.Me()
	requireAPIStatus(t, err, 500)
}

// 上架通知：上线过渡（draft→approved）时收藏人群查询失败仅记日志不阻断；
// 未配置通知口（Enabled=false）时跳过；通知成功扇出调用计数。
func TestAdminLogic_PublishGameNotify(t *testing.T) {
	t.Run("fav query failed tolerated", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		// 种子游戏默认 approved，先落 draft 才有 draft→approved 过渡触发通知
		if _, err := svcCtx.GameRepository.SetStatus("g-rpg-1", model.GameStatusDraft, ""); err != nil {
			t.Fatalf("SetStatus draft: %v", err)
		}
		svcCtx.Notify = &fakePublisher{enabled: true}
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:      svcCtx.AdminRepository,
			listFavUsersErr: errors.New("favs down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		if _, err := l.PublishGame("g-rpg-1"); err != nil {
			t.Fatalf("fav query failure should be tolerated: %v", err)
		}
	})
	t.Run("notify disabled skips", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		if _, err := svcCtx.GameRepository.SetStatus("g-rpg-1", model.GameStatusDraft, ""); err != nil {
			t.Fatalf("SetStatus draft: %v", err)
		}
		pub := &fakePublisher{enabled: false}
		svcCtx.Notify = pub
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		if _, err := l.PublishGame("g-rpg-1"); err != nil {
			t.Fatalf("publish success expected: %v", err)
		}
		if pub.calls != 0 {
			t.Fatalf("disabled publisher should not fan out, calls=%d", pub.calls)
		}
	})
	t.Run("enabled fans out", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		if _, err := svcCtx.GameRepository.SetStatus("g-rpg-1", model.GameStatusDraft, ""); err != nil {
			t.Fatalf("SetStatus draft: %v", err)
		}
		admin := svcCtx.AdminRepository
		if _, _, err := admin.AddFavorite("g-rpg-1", 1); err != nil {
			t.Fatalf("AddFavorite: %v", err)
		}
		pub := &fakePublisher{enabled: true}
		svcCtx.Notify = pub
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		if _, err := l.PublishGame("g-rpg-1"); err != nil {
			t.Fatalf("publish success expected: %v", err)
		}
		if pub.calls != 1 {
			t.Fatalf("publisher should fan out once, calls=%d", pub.calls)
		}
	})
}

// 上架失败：游戏查不到 → 404；查询失败 → 500；状态更新失败 → 500。
func TestAdminLogic_PublishGameErrors(t *testing.T) {
	t.Run("game not found 404", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.GameRepository = &failingGameStore{
			GameStore: svcCtx.GameRepository,
			getErr:    model.ErrGameNotFound,
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.PublishGame("g-ghost")
		requireAPIStatus(t, err, 404)
	})
	t.Run("query failed 500", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.GameRepository = &failingGameStore{
			GameStore: svcCtx.GameRepository,
			getErr:    errors.New("query down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.PublishGame("g-rpg-1")
		requireAPIStatus(t, err, 500)
	})
	t.Run("set status failed 500", func(t *testing.T) {
		svcCtx, _, _ := newAdminLogicCtx(t)
		svcCtx.GameRepository = &failingGameStore{
			GameStore:    svcCtx.GameRepository,
			setStatusErr: errors.New("status down"),
		}
		l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
		_, err := l.PublishGame("g-rpg-1")
		requireAPIStatus(t, err, 500)
	})
}

// 下架：正常过渡（approved→offline）成功。
func TestAdminLogic_UnpublishGameOK(t *testing.T) {
	svcCtx, _, _ := newAdminLogicCtx(t)
	if _, err := svcCtx.GameRepository.SetStatus("g-rpg-1", model.GameStatusApproved, ""); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	l := NewAdminLogic(adminCtx(1, "admin"), svcCtx)
	if _, err := l.UnpublishGame("g-rpg-1"); err != nil {
		t.Fatalf("unpublish success expected: %v", err)
	}
}
