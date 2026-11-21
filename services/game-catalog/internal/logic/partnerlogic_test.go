package logic

import (
	"errors"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"
)

// failingPartnerStore 合作方自助 logic 错误分支注入：内嵌真实仓储，仅按字段
// 覆写指定方法；未覆写方法委托真实仓储。
type failingPartnerStore struct {
	model.AdminStore
	getRoleErr          error
	getOwnershipErr     error
	listGamesErr        error
	getVersionErr       error
	listReviewsErr      error
	submitReviewErr     error
	createVersionErr    error
	listVersionsErr     error
	createServerErr     error
	getServerErr        error
	updateServerErr     error
	listServersErr      error
	listUnnotifiedErr   error
	writeAuditErr       error
	updateVersionPkgErr error
	setVersionStatusErr error
}

var _ model.AdminStore = (*failingPartnerStore)(nil)

func (f *failingPartnerStore) GetRoleWithPartner(userId int64) (string, int64, error) {
	if f.getRoleErr != nil {
		return "", 0, f.getRoleErr
	}
	return f.AdminStore.GetRoleWithPartner(userId)
}

func (f *failingPartnerStore) GetOwnership(gameId string) (*model.GameOwnership, error) {
	if f.getOwnershipErr != nil {
		return nil, f.getOwnershipErr
	}
	return f.AdminStore.GetOwnership(gameId)
}

func (f *failingPartnerStore) ListGamesByPartner(partnerId int64) ([]string, error) {
	if f.listGamesErr != nil {
		return nil, f.listGamesErr
	}
	return f.AdminStore.ListGamesByPartner(partnerId)
}

func (f *failingPartnerStore) GetVersion(id string) (*model.GameVersion, error) {
	if f.getVersionErr != nil {
		return nil, f.getVersionErr
	}
	return f.AdminStore.GetVersion(id)
}

func (f *failingPartnerStore) ListReviews(status string, limit, offset int) ([]*model.GameReview, int, error) {
	if f.listReviewsErr != nil {
		return nil, 0, f.listReviewsErr
	}
	return f.AdminStore.ListReviews(status, limit, offset)
}

func (f *failingPartnerStore) SubmitReview(gameId, versionId string, submittedBy int64) (*model.GameReview, error) {
	if f.submitReviewErr != nil {
		return nil, f.submitReviewErr
	}
	return f.AdminStore.SubmitReview(gameId, versionId, submittedBy)
}

func (f *failingPartnerStore) CreateVersion(v *model.GameVersion) (*model.GameVersion, error) {
	if f.createVersionErr != nil {
		return nil, f.createVersionErr
	}
	return f.AdminStore.CreateVersion(v)
}

func (f *failingPartnerStore) ListVersionsByGame(gameId string) ([]*model.GameVersion, error) {
	if f.listVersionsErr != nil {
		return nil, f.listVersionsErr
	}
	return f.AdminStore.ListVersionsByGame(gameId)
}

func (f *failingPartnerStore) CreateGameServer(gameId string, s *model.GameServer) (*model.GameServer, error) {
	if f.createServerErr != nil {
		return nil, f.createServerErr
	}
	return f.AdminStore.CreateGameServer(gameId, s)
}

func (f *failingPartnerStore) GetGameServer(id int64) (*model.GameServer, error) {
	if f.getServerErr != nil {
		return nil, f.getServerErr
	}
	return f.AdminStore.GetGameServer(id)
}

func (f *failingPartnerStore) UpdateGameServer(s *model.GameServer) (*model.GameServer, error) {
	if f.updateServerErr != nil {
		return nil, f.updateServerErr
	}
	return f.AdminStore.UpdateGameServer(s)
}

func (f *failingPartnerStore) ListGameServers(gameId string, includeClosed bool) ([]*model.GameServer, error) {
	if f.listServersErr != nil {
		return nil, f.listServersErr
	}
	return f.AdminStore.ListGameServers(gameId, includeClosed)
}

func (f *failingPartnerStore) ListUnnotifiedReservations(serverId int64) ([]*model.ServerReservation, error) {
	if f.listUnnotifiedErr != nil {
		return nil, f.listUnnotifiedErr
	}
	return f.AdminStore.ListUnnotifiedReservations(serverId)
}

func (f *failingPartnerStore) WriteAudit(l *model.AuditLog) error {
	if f.writeAuditErr != nil {
		return f.writeAuditErr
	}
	return f.AdminStore.WriteAudit(l)
}

func (f *failingPartnerStore) SetVersionStatus(id, status, reason string) (*model.GameVersion, error) {
	if f.setVersionStatusErr != nil {
		return nil, f.setVersionStatusErr
	}
	return f.AdminStore.SetVersionStatus(id, status, reason)
}

func (f *failingPartnerStore) UpdateVersionPackage(id, hash string, sizeBytes int64, storageUrl string) (*model.GameVersion, error) {
	if f.updateVersionPkgErr != nil {
		return nil, f.updateVersionPkgErr
	}
	return f.AdminStore.UpdateVersionPackage(id, hash, sizeBytes, storageUrl)
}

// newPartnerCtx 合作方自助测试上下文：账号 8 绑定合作方 5、g-rpg-1 归属 5 且
// 已上架，另建一个 closed 区服 S1；返回 svcCtx 与 S1 的 id。
func newPartnerCtx(t *testing.T) (*svc.ServiceContext, int64) {
	t.Helper()
	svcCtx := newLogicSvcCtx(t)
	if _, err := svcCtx.GameRepository.SetStatus("g-rpg-1", model.GameStatusApproved, ""); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	admin := svcCtx.AdminRepository
	if err := admin.SetRole(8, model.RolePartner, 5); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	if _, err := admin.AssignOwnership("g-rpg-1", 5, 1); err != nil {
		t.Fatalf("AssignOwnership: %v", err)
	}
	s, err := admin.CreateGameServer("g-rpg-1", &model.GameServer{
		Name: "S1", Status: model.ServerStatusClosed,
	})
	if err != nil {
		t.Fatalf("CreateGameServer: %v", err)
	}
	return svcCtx, s.Id
}

// 未绑定合作方（角色非 partner / partnerID=0）→ 403。
func TestPartnerLogic_UnboundForbidden(t *testing.T) {
	svcCtx := newLogicSvcCtx(t) // 无角色绑定
	l := NewPartnerLogic(reviewCtx(8), svcCtx)
	if _, err := l.ListPartnerGames(); err != nil {
		requireAPIStatus(t, err, 403)
	} else {
		t.Fatal("want 403")
	}
	if _, err := l.Submit(&types.PartnerSubmitRequest{Id: "g-rpg-1"}); err != nil {
		requireAPIStatus(t, err, 403)
	} else {
		t.Fatal("want 403")
	}
	if _, err := l.CreateVersion(&types.CreateVersionRequest{Id: "g-rpg-1", Version: "2.0.0"}); err != nil {
		requireAPIStatus(t, err, 403)
	} else {
		t.Fatal("want 403")
	}
	if _, err := l.ListVersions("g-rpg-1"); err != nil {
		requireAPIStatus(t, err, 403)
	} else {
		t.Fatal("want 403")
	}
	if _, err := l.CreateServer(&types.CreateServerRequest{Id: "g-rpg-1", Name: "S2"}); err != nil {
		requireAPIStatus(t, err, 403)
	} else {
		t.Fatal("want 403")
	}
	if _, err := l.UpdateServer(&types.UpdateServerRequest{Id: "g-rpg-1", ServerId: 1}); err != nil {
		requireAPIStatus(t, err, 403)
	} else {
		t.Fatal("want 403")
	}
	if _, err := l.ListPartnerServers("g-rpg-1"); err != nil {
		requireAPIStatus(t, err, 403)
	} else {
		t.Fatal("want 403")
	}
}

// ownGame：归属查不到 → 404；其他合作方 → 403；查询失败 → 500。
func TestPartnerLogic_OwnGameErrors(t *testing.T) {
	t.Run("ownership missing 404", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:      svcCtx.AdminRepository,
			getOwnershipErr: model.ErrOwnershipMissing,
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.ListVersions("g-rpg-1")
		requireAPIStatus(t, err, 404)
	})
	t.Run("not mine 403", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		// g-rpg-2 归属其它合作方：改归属到 9
		if _, err := svcCtx.AdminRepository.AssignOwnership("g-rpg-2", 9, 1); err != nil {
			t.Fatalf("AssignOwnership: %v", err)
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.ListVersions("g-rpg-2")
		requireAPIStatus(t, err, 403)
	})
	t.Run("query failed 500", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:      svcCtx.AdminRepository,
			getOwnershipErr: errors.New("query down"),
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.ListVersions("g-rpg-1")
		requireAPIStatus(t, err, 500)
	})
}

// 合作方游戏列表：查询失败 → 500；成功聚合。
func TestPartnerLogic_ListPartnerGamesErrors(t *testing.T) {
	t.Run("list failed 500", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:   svcCtx.AdminRepository,
			listGamesErr: errors.New("list down"),
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.ListPartnerGames()
		requireAPIStatus(t, err, 500)
	})
	t.Run("success aggregates", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		resp, err := l.ListPartnerGames()
		if err != nil {
			t.Fatalf("ListPartnerGames: %v", err)
		}
		if len(resp.Games) != 1 || resp.Games[0].Id != "g-rpg-1" {
			t.Fatalf("games = %+v, want g-rpg-1", resp.Games)
		}
	})
}

// 提审：版本不存在 → 404；版本不属于该游戏 → 400；未决队列查询失败 → 500；
// 已有未决提审 → 409；登记失败 → 500；版本提审状态更新失败 → 500。
func TestPartnerLogic_SubmitErrors(t *testing.T) {
	t.Run("empty game id 400", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.Submit(&types.PartnerSubmitRequest{Id: "  "})
		requireAPIStatus(t, err, 400)
	})
	t.Run("version not found 404", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:    svcCtx.AdminRepository,
			getVersionErr: model.ErrVersionNotFound,
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.Submit(&types.PartnerSubmitRequest{Id: "g-rpg-1", VersionId: "ver-x"})
		requireAPIStatus(t, err, 404)
	})
	t.Run("version not belong 400", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		v, err := svcCtx.AdminRepository.CreateVersion(&model.GameVersion{GameId: "g-rpg-2", Version: "9.9.9"})
		if err != nil {
			t.Fatalf("CreateVersion: %v", err)
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err = l.Submit(&types.PartnerSubmitRequest{Id: "g-rpg-1", VersionId: v.Id})
		requireAPIStatus(t, err, 400)
	})
	t.Run("pending list failed 500", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:     svcCtx.AdminRepository,
			listReviewsErr: errors.New("queue down"),
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.Submit(&types.PartnerSubmitRequest{Id: "g-rpg-1"})
		requireAPIStatus(t, err, 500)
	})
	t.Run("duplicate pending 409", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		if _, err := svcCtx.AdminRepository.SubmitReview("g-rpg-1", "", 8); err != nil {
			t.Fatalf("SubmitReview: %v", err)
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.Submit(&types.PartnerSubmitRequest{Id: "g-rpg-1"})
		requireAPIStatus(t, err, 409)
	})
	t.Run("submit failed 500", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:      svcCtx.AdminRepository,
			submitReviewErr: errors.New("submit down"),
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.Submit(&types.PartnerSubmitRequest{Id: "g-rpg-1"})
		requireAPIStatus(t, err, 500)
	})
	t.Run("set game pending failed 500", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		// SubmitReview 成功（委托真实），但 SetStatus 失败：用只读游戏仓储注入
		svcCtx.GameRepository = &failingGameStore{
			GameStore:    svcCtx.GameRepository,
			setStatusErr: errors.New("status down"),
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.Submit(&types.PartnerSubmitRequest{Id: "g-rpg-1"})
		requireAPIStatus(t, err, 500)
	})
	t.Run("set version pending failed 500", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		v, err := svcCtx.AdminRepository.CreateVersion(&model.GameVersion{GameId: "g-rpg-1", Version: "3.0.0"})
		if err != nil {
			t.Fatalf("CreateVersion: %v", err)
		}
		// SetVersionStatus 失败需注入：扩展覆写
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:          svcCtx.AdminRepository,
			setVersionStatusErr: errors.New("status down"),
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err = l.Submit(&types.PartnerSubmitRequest{Id: "g-rpg-1", VersionId: v.Id})
		requireAPIStatus(t, err, 500)
	})
}

// 建版本：版本号空 → 400；创建失败 → 500。
func TestPartnerLogic_CreateVersionErrors(t *testing.T) {
	t.Run("empty version 400", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.CreateVersion(&types.CreateVersionRequest{Id: "g-rpg-1", Version: " "})
		requireAPIStatus(t, err, 400)
	})
	t.Run("create failed 500", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:       svcCtx.AdminRepository,
			createVersionErr: errors.New("create down"),
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.CreateVersion(&types.CreateVersionRequest{Id: "g-rpg-1", Version: "2.0.0"})
		requireAPIStatus(t, err, 500)
	})
}

// 版本列表查询失败 → 500。
func TestPartnerLogic_ListVersionsFailed500(t *testing.T) {
	svcCtx, _ := newPartnerCtx(t)
	svcCtx.AdminRepository = &failingPartnerStore{
		AdminStore:      svcCtx.AdminRepository,
		listVersionsErr: errors.New("list down"),
	}
	l := NewPartnerLogic(reviewCtx(8), svcCtx)
	_, err := l.ListVersions("g-rpg-1")
	requireAPIStatus(t, err, 500)
}

// 建区服：名称空 → 400；非法状态 → 400；创建失败 → 500。
func TestPartnerLogic_CreateServerErrors(t *testing.T) {
	t.Run("empty name 400", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.CreateServer(&types.CreateServerRequest{Id: "g-rpg-1", Name: " "})
		requireAPIStatus(t, err, 400)
	})
	t.Run("invalid status 400", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.CreateServer(&types.CreateServerRequest{Id: "g-rpg-1", Name: "S2", Status: "bogus"})
		requireAPIStatus(t, err, 400)
	})
	t.Run("create failed 500", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:      svcCtx.AdminRepository,
			createServerErr: errors.New("create down"),
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.CreateServer(&types.CreateServerRequest{Id: "g-rpg-1", Name: "S2"})
		requireAPIStatus(t, err, 500)
	})
}

// 更新区服：查不到 → 404；非法状态 → 400；更新失败（哨兵/通用）→ 404/500。
func TestPartnerLogic_UpdateServerErrors(t *testing.T) {
	t.Run("not found 404", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:   svcCtx.AdminRepository,
			getServerErr: model.ErrServerNotFound,
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.UpdateServer(&types.UpdateServerRequest{Id: "g-rpg-1", ServerId: 999})
		requireAPIStatus(t, err, 404)
	})
	t.Run("invalid status 400", func(t *testing.T) {
		svcCtx, serverID := newPartnerCtx(t)
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.UpdateServer(&types.UpdateServerRequest{Id: "g-rpg-1", ServerId: serverID, Status: "bogus"})
		requireAPIStatus(t, err, 400)
	})
	t.Run("update not found 404", func(t *testing.T) {
		svcCtx, serverID := newPartnerCtx(t)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:      svcCtx.AdminRepository,
			updateServerErr: model.ErrServerNotFound,
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.UpdateServer(&types.UpdateServerRequest{Id: "g-rpg-1", ServerId: serverID, Status: "open"})
		requireAPIStatus(t, err, 404)
	})
	t.Run("update failed 500", func(t *testing.T) {
		svcCtx, serverID := newPartnerCtx(t)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:      svcCtx.AdminRepository,
			updateServerErr: errors.New("update down"),
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.UpdateServer(&types.UpdateServerRequest{Id: "g-rpg-1", ServerId: serverID, Status: "open"})
		requireAPIStatus(t, err, 500)
	})
}

// 区服列表查询失败 → 500。
func TestPartnerLogic_ListPartnerServersFailed500(t *testing.T) {
	svcCtx, _ := newPartnerCtx(t)
	svcCtx.AdminRepository = &failingPartnerStore{
		AdminStore:     svcCtx.AdminRepository,
		listServersErr: errors.New("list down"),
	}
	l := NewPartnerLogic(reviewCtx(8), svcCtx)
	_, err := l.ListPartnerServers("g-rpg-1")
	requireAPIStatus(t, err, 500)
}
