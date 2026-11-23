package logic

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/courierdownload"
	"github.com/tappi/tappi/services/game-catalog/internal/store"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/model"
)

// seedPublishedVersion 建一条 g-rpg-1 的已发布版本并装好真实包体存储。
func seedPublishedVersion(t *testing.T, svcCtx *svc.ServiceContext, storageURL string) string {
	t.Helper()
	admin := svcCtx.AdminRepository
	v, err := admin.CreateVersion(&model.GameVersion{
		GameId: "g-rpg-1", Version: "1.0.0", VersionCode: 1,
		StorageUrl: storageURL,
	})
	if err != nil {
		t.Fatalf("CreateVersion: %v", err)
	}
	if _, err := admin.SetVersionStatus(v.Id, model.VersionStatusApproved, ""); err != nil {
		t.Fatalf("SetVersionStatus: %v", err)
	}
	if _, err := admin.PublishVersion(v.Id); err != nil {
		t.Fatalf("PublishVersion: %v", err)
	}
	return v.Id
}

// Download 受控下载错误分支：未配置存储 → 500；版本不存在/未发布/跨游戏 → 404；
// 查询失败 → 500；包体未托管平台 → 409；文件缺失 → 500。
func TestDownloadVersionLogic_DownloadErrors(t *testing.T) {
	t.Run("store nil 500", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t) // PackageStore 默认 nil
		l := NewDownloadVersionLogic(reviewCtx(7), svcCtx)
		if _, err := l.Download("g-rpg-1", "ver-x", ""); err != nil {
			requireAPIStatus(t, err, 500)
		} else {
			t.Fatal("want 500")
		}
	})
	t.Run("version not found 404", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		ps, err := store.NewLocalPackageStore(t.TempDir())
		if err != nil {
			t.Fatalf("NewLocalPackageStore: %v", err)
		}
		svcCtx.PackageStore = ps
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:    svcCtx.AdminRepository,
			getVersionErr: model.ErrVersionNotFound,
		}
		l := NewDownloadVersionLogic(reviewCtx(7), svcCtx)
		if _, err := l.Download("g-rpg-1", "ver-x", ""); err != nil {
			requireAPIStatus(t, err, 404)
		} else {
			t.Fatal("want 404")
		}
	})
	t.Run("version query failed 500", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		ps, _ := store.NewLocalPackageStore(t.TempDir())
		svcCtx.PackageStore = ps
		svcCtx.AdminRepository = &failingOpsStore{
			AdminStore:    svcCtx.AdminRepository,
			getVersionErr: errors.New("query down"),
		}
		l := NewDownloadVersionLogic(reviewCtx(7), svcCtx)
		if _, err := l.Download("g-rpg-1", "ver-x", ""); err != nil {
			requireAPIStatus(t, err, 500)
		} else {
			t.Fatal("want 500")
		}
	})
	t.Run("cross game 404", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		ps, _ := store.NewLocalPackageStore(t.TempDir())
		svcCtx.PackageStore = ps
		versionID := seedPublishedVersion(t, svcCtx, store.PackageURLScheme+"ver-x.apk")
		l := NewDownloadVersionLogic(reviewCtx(7), svcCtx)
		if _, err := l.Download("g-rpg-2", versionID, ""); err != nil {
			requireAPIStatus(t, err, 404)
		} else {
			t.Fatal("want 404")
		}
	})
	t.Run("not published 404", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		ps, _ := store.NewLocalPackageStore(t.TempDir())
		svcCtx.PackageStore = ps
		v, err := svcCtx.AdminRepository.CreateVersion(&model.GameVersion{GameId: "g-rpg-1", Version: "1.0.0"})
		if err != nil {
			t.Fatalf("CreateVersion: %v", err)
		}
		l := NewDownloadVersionLogic(reviewCtx(7), svcCtx)
		if _, err := l.Download("g-rpg-1", v.Id, ""); err != nil {
			requireAPIStatus(t, err, 404)
		} else {
			t.Fatal("want 404")
		}
	})
	t.Run("external storage 409", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		ps, _ := store.NewLocalPackageStore(t.TempDir())
		svcCtx.PackageStore = ps
		versionID := seedPublishedVersion(t, svcCtx, "https://cdn.example.com/x.apk")
		l := NewDownloadVersionLogic(reviewCtx(7), svcCtx)
		if _, err := l.Download("g-rpg-1", versionID, ""); err != nil {
			requireAPIStatus(t, err, 409)
		} else {
			t.Fatal("want 409")
		}
	})
	t.Run("package file missing 500", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		ps, _ := store.NewLocalPackageStore(t.TempDir())
		svcCtx.PackageStore = ps
		versionID := seedPublishedVersion(t, svcCtx, store.PackageURLScheme+"ver-x.apk")
		l := NewDownloadVersionLogic(reviewCtx(7), svcCtx)
		if _, err := l.Download("g-rpg-1", versionID, ""); err != nil {
			requireAPIStatus(t, err, 500)
		} else {
			t.Fatal("want 500")
		}
	})
}

// seedDownloadSuccess 装好真实包体文件 + 已发布版本，使 Download 成功返回。
func seedDownloadSuccess(t *testing.T, svcCtx *svc.ServiceContext) string {
	t.Helper()
	versionID := seedPublishedVersion(t, svcCtx, store.PackageURLScheme+"placeholder.apk")
	if _, _, _, err := svcCtx.PackageStore.Save(versionID, "game.apk", bytes.NewReader([]byte("apk-data"))); err != nil {
		t.Fatalf("save package: %v", err)
	}
	return versionID
}

// downloadGateStub courier-adapter 映射存根：download 能力由 status/body 控制。
func downloadGateStub(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Token") != "gate-tok" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(stub.Close)
	return stub
}

// GP-002 download 能力门：nil 客户端（未接线）no-op 放行；接线后 download
// false/未映射 → 403，查询失败 → 502。
func TestDownloadVersionLogic_DownloadGate(t *testing.T) {
	t.Run("unconfigured client no-op", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		ps, _ := store.NewLocalPackageStore(t.TempDir())
		svcCtx.PackageStore = ps
		// CourierDownload 保持 nil（COURIER_ADAPTER_URL 未配置）→ 不门控
		versionID := seedDownloadSuccess(t, svcCtx)
		l := NewDownloadVersionLogic(reviewCtx(7), svcCtx)
		res, err := l.Download("g-rpg-1", versionID, "")
		if err != nil || res == nil || res.Version.Id != versionID {
			t.Fatalf("unconfigured no-op = %+v err=%v, want success", res, err)
		}
	})
	t.Run("download enabled proceeds", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		ps, _ := store.NewLocalPackageStore(t.TempDir())
		svcCtx.PackageStore = ps
		svcCtx.CourierDownload = courierdownload.NewClient(
			downloadGateStub(t, http.StatusOK, `{"capabilities":{"download":true}}`).URL,
			"gate-tok", 0)
		versionID := seedDownloadSuccess(t, svcCtx)
		l := NewDownloadVersionLogic(reviewCtx(7), svcCtx)
		if _, err := l.Download("g-rpg-1", versionID, ""); err != nil {
			t.Fatalf("download enabled = %v, want success", err)
		}
	})
	t.Run("download disabled 403", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		ps, _ := store.NewLocalPackageStore(t.TempDir())
		svcCtx.PackageStore = ps
		svcCtx.CourierDownload = courierdownload.NewClient(
			downloadGateStub(t, http.StatusOK, `{"capabilities":{"download":false}}`).URL,
			"gate-tok", 0)
		l := NewDownloadVersionLogic(reviewCtx(7), svcCtx)
		if _, err := l.Download("g-rpg-1", "any", ""); err != nil {
			requireAPIStatus(t, err, 403)
		} else {
			t.Fatal("want 403")
		}
	})
	t.Run("unmapped 404 treated disabled", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		ps, _ := store.NewLocalPackageStore(t.TempDir())
		svcCtx.PackageStore = ps
		svcCtx.CourierDownload = courierdownload.NewClient(
			downloadGateStub(t, http.StatusNotFound, `{"code":404,"message":"游戏未接入 Courier"}`).URL,
			"gate-tok", 0)
		l := NewDownloadVersionLogic(reviewCtx(7), svcCtx)
		if _, err := l.Download("g-rpg-1", "any", ""); err != nil {
			requireAPIStatus(t, err, 403)
		} else {
			t.Fatal("want 403（未映射=未开通）")
		}
	})
	t.Run("lookup failure 502", func(t *testing.T) {
		svcCtx := newLogicSvcCtx(t)
		ps, _ := store.NewLocalPackageStore(t.TempDir())
		svcCtx.PackageStore = ps
		svcCtx.CourierDownload = courierdownload.NewClient(
			downloadGateStub(t, http.StatusInternalServerError, `boom`).URL,
			"gate-tok", 0)
		l := NewDownloadVersionLogic(reviewCtx(7), svcCtx)
		if _, err := l.Download("g-rpg-1", "any", ""); err != nil {
			requireAPIStatus(t, err, 502)
		} else {
			t.Fatal("want 502（fail-closed）")
		}
	})
}
