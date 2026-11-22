package logic

// partnerlogic 补漏分支：角色查询失败、归属残留跳过、Submit 版本查询失败、
// 审计写失败、UploadPackage 各失败臂、UpdateServer 通用查询失败与字段更新、
// NotifyServerOpen 关闭/列表失败/标记失败、自动开服审计写失败。

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zeromicro/go-zero/core/logx"

	"github.com/tappi/tappi/services/game-catalog/internal/store"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"
)

// errReader 一读即错的包体输入（触发 PackageStore.Save 通用失败臂）。
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

// mustCreateVersion 建一条 g-rpg-1 的 draft 版本供换包测试用。
func mustCreateVersion(t *testing.T, svcCtx *svc.ServiceContext) *model.GameVersion {
	t.Helper()
	v, err := svcCtx.AdminRepository.CreateVersion(&model.GameVersion{GameId: "g-rpg-1", Version: "1.0.0"})
	if err != nil {
		t.Fatalf("CreateVersion: %v", err)
	}
	return v
}

// partnerID：角色绑定查询失败 → 记日志并回退 0 → 403。
func TestPartnerLogic_RoleQueryFailed(t *testing.T) {
	svcCtx, _ := newPartnerCtx(t)
	svcCtx.AdminRepository = &failingPartnerStore{
		AdminStore: svcCtx.AdminRepository,
		getRoleErr: errors.New("role db down"),
	}
	l := NewPartnerLogic(reviewCtx(8), svcCtx)
	_, err := l.ListPartnerGames()
	requireAPIStatus(t, err, 403)
}

// ListPartnerGames：归属残留（游戏已物理删除）→ Get 失败 → continue 跳过不阻断。
func TestPartnerLogic_ListPartnerGamesSkipsStale(t *testing.T) {
	svcCtx, _ := newPartnerCtx(t)
	svcCtx.AdminRepository = &failingPartnerStore{
		AdminStore:        svcCtx.AdminRepository,
		listGamesOverride: []string{"g-rpg-1", "g-ghost"},
	}
	l := NewPartnerLogic(reviewCtx(8), svcCtx)
	resp, err := l.ListPartnerGames()
	if err != nil {
		t.Fatalf("ListPartnerGames: %v", err)
	}
	if len(resp.Games) != 1 || resp.Games[0].Id != "g-rpg-1" {
		t.Fatalf("games = %+v, want only g-rpg-1 (stale skipped)", resp.Games)
	}
}

// Submit：ownGame 查询失败 → 原样传播；版本查询通用失败 → 500。
func TestPartnerLogic_SubmitOwnGameAndVersionQuery(t *testing.T) {
	t.Run("own game query failed 500", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:      svcCtx.AdminRepository,
			getOwnershipErr: errors.New("ownership down"),
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.Submit(&types.PartnerSubmitRequest{Id: "g-rpg-1"})
		requireAPIStatus(t, err, 500)
	})
	t.Run("version query generic failed 500", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:    svcCtx.AdminRepository,
			getVersionErr: errors.New("version db down"),
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.Submit(&types.PartnerSubmitRequest{Id: "g-rpg-1", VersionId: "ver-x"})
		requireAPIStatus(t, err, 500)
	})
}

// audit 写审计失败仅记日志，提审/建版本仍成功。
func TestPartnerLogic_AuditWriteFailedTolerated(t *testing.T) {
	svcCtx, _ := newPartnerCtx(t)
	svcCtx.AdminRepository = &failingPartnerStore{
		AdminStore:    svcCtx.AdminRepository,
		writeAuditErr: errors.New("audit down"),
	}
	l := NewPartnerLogic(reviewCtx(8), svcCtx)
	review, err := l.Submit(&types.PartnerSubmitRequest{Id: "g-rpg-1"})
	if err != nil || review == nil {
		t.Fatalf("Submit with audit failure: resp=%+v err=%v", review, err)
	}
}

// CreateVersion：ownGame 查询失败 → 500。
func TestPartnerLogic_CreateVersionOwnGameFailed(t *testing.T) {
	svcCtx, _ := newPartnerCtx(t)
	svcCtx.AdminRepository = &failingPartnerStore{
		AdminStore:      svcCtx.AdminRepository,
		getOwnershipErr: errors.New("ownership down"),
	}
	l := NewPartnerLogic(reviewCtx(8), svcCtx)
	_, err := l.CreateVersion(&types.CreateVersionRequest{Id: "g-rpg-1", Version: "2.0.0"})
	requireAPIStatus(t, err, 500)
}

// UploadPackage 换包失败臂。
func TestPartnerLogic_UploadPackageErrors(t *testing.T) {
	newPkg := func(t *testing.T, svcCtx *svc.ServiceContext) *store.LocalPackageStore {
		t.Helper()
		ps, err := store.NewLocalPackageStore(t.TempDir())
		if err != nil {
			t.Fatalf("NewLocalPackageStore: %v", err)
		}
		svcCtx.PackageStore = ps
		return ps
	}

	t.Run("unbound 403", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t) // 未绑定合作方
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.UploadPackage("g-rpg-1", "ver-x", "p.bin", strings.NewReader("x"))
		requireAPIStatus(t, err, 403)
	})
	t.Run("own game failed 500", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:      svcCtx.AdminRepository,
			getOwnershipErr: errors.New("ownership down"),
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.UploadPackage("g-rpg-1", "ver-x", "p.bin", strings.NewReader("x"))
		requireAPIStatus(t, err, 500)
	})
	t.Run("package store nil 500", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t) // newLogicSvcCtx 未配置 PackageStore
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.UploadPackage("g-rpg-1", "ver-x", "p.bin", strings.NewReader("x"))
		requireAPIStatus(t, err, 500)
	})
	t.Run("get version generic failed 500", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		newPkg(t, svcCtx)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:    svcCtx.AdminRepository,
			getVersionErr: errors.New("version db down"),
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.UploadPackage("g-rpg-1", "ver-x", "p.bin", strings.NewReader("x"))
		requireAPIStatus(t, err, 500)
	})
	t.Run("save failed 500", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		newPkg(t, svcCtx)
		v := mustCreateVersion(t, svcCtx)
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.UploadPackage("g-rpg-1", v.Id, "p.bin", errReader{err: errors.New("disk full")})
		requireAPIStatus(t, err, 500)
	})
	t.Run("update locked 409", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		newPkg(t, svcCtx)
		v := mustCreateVersion(t, svcCtx)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:          svcCtx.AdminRepository,
			updateVersionPkgErr: model.ErrVersionPackageLocked,
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.UploadPackage("g-rpg-1", v.Id, "p.bin", strings.NewReader("hello"))
		requireAPIStatus(t, err, 409)
	})
	t.Run("update generic failed 500", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		newPkg(t, svcCtx)
		v := mustCreateVersion(t, svcCtx)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:          svcCtx.AdminRepository,
			updateVersionPkgErr: errors.New("update down"),
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.UploadPackage("g-rpg-1", v.Id, "p.bin", strings.NewReader("hello"))
		requireAPIStatus(t, err, 500)
	})
}

// UpdateServer：区服通用查询失败 → 500；成功路径全字段更新。
func TestPartnerLogic_UpdateServerQueryAndFields(t *testing.T) {
	t.Run("get generic failed 500", func(t *testing.T) {
		svcCtx, _ := newPartnerCtx(t)
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:   svcCtx.AdminRepository,
			getServerErr: errors.New("server db down"),
		}
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		_, err := l.UpdateServer(&types.UpdateServerRequest{Id: "g-rpg-1", ServerId: 999})
		requireAPIStatus(t, err, 500)
	})
	t.Run("all fields updated", func(t *testing.T) {
		svcCtx, serverID := newPartnerCtx(t)
		l := NewPartnerLogic(reviewCtx(8), svcCtx)
		resp, err := l.UpdateServer(&types.UpdateServerRequest{
			Id: "g-rpg-1", ServerId: serverID,
			Name: "S2", Region: "CN", Sort: 5, PlannedOpenAt: "2026-01-01T00:00:00Z",
		})
		if err != nil || resp == nil {
			t.Fatalf("UpdateServer all fields: resp=%+v err=%v", resp, err)
		}
		if resp.Name != "S2" || resp.Region != "CN" || resp.Sort != 5 || resp.PlannedOpenAt != "2026-01-01T00:00:00Z" {
			t.Fatalf("updated server = %+v, want name S2 region CN sort 5 planned 2026-01-01", resp)
		}
	})
}

// NotifyServerOpen 开服通知守卫与失败臂（关闭 / 列表失败 / 标记失败）。
func TestPartnerLogic_NotifyServerOpenBranches(t *testing.T) {
	notifyCtx := func(svcCtx *svc.ServiceContext) logx.Logger {
		return logx.WithContext(context.Background())
	}

	t.Run("disabled returns early", func(t *testing.T) {
		svcCtx, serverID := newPartnerCtx(t)
		svcCtx.Notify = &fakePublisher{enabled: false}
		s, err := svcCtx.AdminRepository.GetGameServer(serverID)
		if err != nil {
			t.Fatalf("GetGameServer: %v", err)
		}
		NotifyServerOpen(notifyCtx(svcCtx), svcCtx, s)
	})
	t.Run("list failed logs", func(t *testing.T) {
		svcCtx, serverID := newPartnerCtx(t)
		svcCtx.Notify = &fakePublisher{enabled: true}
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:        svcCtx.AdminRepository,
			listUnnotifiedErr: errors.New("list down"),
		}
		s, err := svcCtx.AdminRepository.GetGameServer(serverID)
		if err != nil {
			t.Fatalf("GetGameServer: %v", err)
		}
		NotifyServerOpen(notifyCtx(svcCtx), svcCtx, s)
	})
	t.Run("mark failed logs after publish", func(t *testing.T) {
		svcCtx, serverID := newPartnerCtx(t)
		svcCtx.Notify = &fakePublisher{enabled: true}
		if _, _, err := svcCtx.AdminRepository.ReserveServer(&model.ServerReservation{
			GameId: "g-rpg-1", ServerId: serverID, UserId: 7,
		}); err != nil {
			t.Fatalf("ReserveServer: %v", err)
		}
		svcCtx.AdminRepository = &failingPartnerStore{
			AdminStore:      svcCtx.AdminRepository,
			markNotifiedErr: errors.New("mark down"),
		}
		s, err := svcCtx.AdminRepository.GetGameServer(serverID)
		if err != nil {
			t.Fatalf("GetGameServer: %v", err)
		}
		NotifyServerOpen(notifyCtx(svcCtx), svcCtx, s)
		if p := svcCtx.Notify.(*fakePublisher); p.calls != 1 {
			t.Fatalf("publish calls = %d, want 1 (mark failure still publishes)", p.calls)
		}
	})
}

// AuditServerAutoOpen：写审计失败仅记日志。
func TestPartnerLogic_AuditServerAutoOpenWriteFailed(t *testing.T) {
	svcCtx, serverID := newPartnerCtx(t)
	svcCtx.AdminRepository = &failingPartnerStore{
		AdminStore:    svcCtx.AdminRepository,
		writeAuditErr: errors.New("audit down"),
	}
	AuditServerAutoOpen(logx.WithContext(context.Background()), svcCtx, serverID)
}
