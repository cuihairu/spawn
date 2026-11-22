package logic

// 剩余可触达分支补漏：审计的 nil 仓储跳过与写失败容忍（create/update tags/
// promo）、validPromoCode 长度上限、Cancel 的 GetGameServer 通用失败、
// ListServers 的 game.Get 通用失败、Download 的扩展名回退与 userID 回退 0、
// PackageStore.Open 通用失败（ELOOP 符号链接环）。crypto/rand 失败臂与
// 2 GiB 超限上传臂（Save TooLarge → 413）属不可/不宜构造，见 coverage 台账。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/analytics"
	"github.com/tappi/tappi/services/game-catalog/internal/store"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"
)

// 审计：AdminRepository 为 nil（未装配）→ 跳过写审计，主操作不受影响。
func TestCreateGameLogic_AuditNilStore(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	svcCtx.AdminRepository = nil
	l := NewCreateGameLogic(context.Background(), svcCtx)
	resp, err := l.CreateGame(&types.CreateGameRequest{
		Title: "T", Description: "D", Genres: []string{"RPG"}, Platforms: []string{"PC"},
	})
	if err != nil || resp == nil {
		t.Fatalf("create without audit store: resp=%+v err=%v", resp, err)
	}
}

// 审计：WriteAudit 失败仅记日志，不阻断创建。
func TestCreateGameLogic_AuditFailedTolerated(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	svcCtx.AdminRepository = &failingAdminStore{
		AdminStore:    svcCtx.AdminRepository,
		writeAuditErr: errors.New("audit down"),
	}
	l := NewCreateGameLogic(adminCtx(1, "operator"), svcCtx)
	resp, err := l.CreateGame(&types.CreateGameRequest{
		Title: "T", Description: "D", Genres: []string{"RPG"}, Platforms: []string{"PC"},
	})
	if err != nil || resp == nil {
		t.Fatalf("audit failure tolerated: resp=%+v err=%v", resp, err)
	}
}

// 审计：标签更新时 AdminRepository 为 nil → 跳过，主操作仍成功。
func TestUpdateGameTagsLogic_AuditNilStore(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	svcCtx.AdminRepository = nil
	l := NewUpdateGameTagsLogic(reviewCtx(1), svcCtx)
	resp, err := l.UpdateGameTags(&types.UpdateGameTagsRequest{Id: "g-rpg-1", Tags: []string{"a"}})
	if err != nil || resp == nil {
		t.Fatalf("update tags without audit store: resp=%+v err=%v", resp, err)
	}
}

// 审计：promo 渠道创建时 WriteAudit 失败 → 容忍（渠道照常返回）。
func TestPromoLogic_AuditFailedTolerated(t *testing.T) {
	svcCtx, _ := newPromoCtx(t)
	svcCtx.AdminRepository = &failingPromoStore{
		AdminStore:    svcCtx.AdminRepository,
		writeAuditErr: errors.New("audit down"),
	}
	l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
	resp, err := l.CreatePromoChannel(&types.CreatePromoChannelRequest{Code: "kol-audit", Name: "x"})
	if err != nil || resp == nil {
		t.Fatalf("promo audit failure tolerated: resp=%+v err=%v", resp, err)
	}
}

// validPromoCode 上限：65 字符 code → 400（>64 被拒）。
func TestPromoLogic_ValidPromoCodeTooLong(t *testing.T) {
	svcCtx, _ := newPromoCtx(t)
	l := NewPromoLogic(adminCtx(1, "operator"), svcCtx)
	long := strings.Repeat("a", 65)
	_, err := l.CreatePromoChannel(&types.CreatePromoChannelRequest{Code: long, Name: "x"})
	requireAPIStatus(t, err, 400)
}

// Cancel 区服查询返回非哨兵错误 → 500（不同于 404 与 cancelErr 臂）。
func TestReservationLogic_CancelGetServerFailed500(t *testing.T) {
	svcCtx, _ := newReservationCtx(t)
	svcCtx.AdminRepository = &failingReservationStore{
		AdminStore:   svcCtx.AdminRepository,
		getServerErr: errors.New("server db down"),
	}
	l := NewReservationLogic(reviewCtx(7), svcCtx)
	err := l.Cancel(&types.ServerPathRequest{Id: "g-rpg-1", ServerId: 1})
	requireAPIStatus(t, err, 500)
}

// ListServers 的 game.Get 返回非哨兵错误 → 500（区别于 404 与 list 失败臂）。
func TestGetGameDetailLogic_ListServersGetGameFailed500(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	svcCtx.GameRepository = &failingGameStore{GameStore: svcCtx.GameRepository, getErr: errors.New("game db down")}
	l := NewGetGameDetailLogic(reviewCtx(7), svcCtx)
	_, err := l.ListServers(&types.GetGameDetailRequest{Id: "g-rpg-1"})
	requireAPIStatus(t, err, 500)
}

// fakeAnalytics 仅记录投递，驱动 emitDownload 的拼装路径。
type fakeAnalytics struct {
	events []analytics.Event
}

func (f *fakeAnalytics) Enabled() bool { return true }
func (f *fakeAnalytics) Emit(e analytics.Event) {
	f.events = append(f.events, e)
}

// publishVersion 建一条 g-rpg-1 已发布版本；storage_url 由回调按 id 计算
// （须在发布前设置，发布后 UpdateVersionPackage 会被锁）。
func publishVersion(t *testing.T, svcCtx *svc.ServiceContext, storageURL func(id string) string) string {
	t.Helper()
	admin := svcCtx.AdminRepository
	v, err := admin.CreateVersion(&model.GameVersion{GameId: "g-rpg-1", Version: "1.0.0"})
	if err != nil {
		t.Fatalf("CreateVersion: %v", err)
	}
	if _, err := admin.UpdateVersionPackage(v.Id, "sha256:x", 1, storageURL(v.Id)); err != nil {
		t.Fatalf("UpdateVersionPackage: %v", err)
	}
	if _, err := admin.SetVersionStatus(v.Id, model.VersionStatusApproved, ""); err != nil {
		t.Fatalf("SetVersionStatus: %v", err)
	}
	if _, err := admin.PublishVersion(v.Id); err != nil {
		t.Fatalf("PublishVersion: %v", err)
	}
	return v.Id
}

// 扩展名回退 + userID 回退：storage_url 无扩展名 → ext=".bin"；ctx 无
// user_id 且 analytics 启用 → 投递事件 UserId=0（不 panic）。
func TestDownloadVersionLogic_ExtFallbackAndUserIDZero(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	ps, err := store.NewLocalPackageStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalPackageStore: %v", err)
	}
	svcCtx.PackageStore = ps
	// storage_url 精确为 scheme+id（无扩展名）才命中 ext 回退。
	versionID := publishVersion(t, svcCtx, func(id string) string { return store.PackageURLScheme + id })
	if _, _, _, err := ps.Save(versionID, "pkg.bin", strings.NewReader("payload")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	fake := &fakeAnalytics{}
	svcCtx.AnalyticsSvc = fake

	l := NewDownloadVersionLogic(context.Background(), svcCtx) // 无 user_id
	res, err := l.Download("g-rpg-1", versionID, "kol-li")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer res.File.Close()
	if res.Ext != ".bin" {
		t.Fatalf("ext = %q, want .bin (fallback)", res.Ext)
	}
	if len(fake.events) != 1 || fake.events[0].UserId != 0 {
		t.Fatalf("emit events = %+v, want 1 event with UserId=0", fake.events)
	}
}

// PackageStore.Open 通用失败（非 ErrPackageNotFound）：符号链接环 → os.Open
// ELOOP → 500 读取包体失败。
func TestDownloadVersionLogic_OpenGenericError500(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	dir := t.TempDir()
	ps, err := store.NewLocalPackageStore(dir)
	if err != nil {
		t.Fatalf("NewLocalPackageStore: %v", err)
	}
	svcCtx.PackageStore = ps
	versionID := publishVersion(t, svcCtx, func(string) string { return store.PackageURLScheme + "x.apk" })
	// 符号链接自环：glob 命中 <id>.bin，os.Open 沿链接解析失败（ELOOP）。
	loop := filepath.Join(dir, versionID+".bin")
	if err := os.Symlink(loop, loop); err != nil {
		t.Fatalf("symlink loop: %v", err)
	}

	l := NewDownloadVersionLogic(reviewCtx(7), svcCtx)
	_, err = l.Download("g-rpg-1", versionID, "")
	requireAPIStatus(t, err, 500)
}
