package openscan

// scanner.go 补漏分支：isDue 纯函数全臂、NewRunner 默认周期、Run 的 ctx 取消
// 与 ticker 分支、OpenOnce 的错误臂（ListScheduledServers/OpenServerIfDue/
// GetGameServer 失败、排期非法跳过、条件更新未命中跳过）。到点自动开服的
// 主路径已由 handler 集成测试覆盖，此处补错误臂与纯函数。

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/tappi/tappi/services/game-catalog/internal/config"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/model"
)

const testJWTSecret = "test-secret-for-openscan-only"

// failOpenStore 选择性注入 AdminRepository 错误，其余委托真实实现。
type failOpenStore struct {
	model.AdminStore
	listErr error
	openErr error
	openOK  *bool // 非 nil 时覆盖 OpenServerIfDue 返回
	getErr  error
}

func (s *failOpenStore) ListScheduledServers(limit int) ([]*model.GameServer, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.AdminStore.ListScheduledServers(limit)
}

func (s *failOpenStore) OpenServerIfDue(id int64, currentStatus, plannedOpenAt string) (bool, error) {
	if s.openErr != nil {
		return false, s.openErr
	}
	if s.openOK != nil {
		return *s.openOK, nil
	}
	return s.AdminStore.OpenServerIfDue(id, currentStatus, plannedOpenAt)
}

func (s *failOpenStore) GetGameServer(id int64) (*model.GameServer, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.AdminStore.GetGameServer(id)
}

func newScanSvcCtx(t *testing.T) *svc.ServiceContext {
	t.Helper()
	var c config.Config
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "scan.db")
	c.Auth.JWTSecret = testJWTSecret
	c.Auth.AdminUserIds = "1"
	// Notify.Url 空 = no-op 发布器（NotifyServerOpen 提前返回，不触达上游）。
	return svc.NewServiceContext(c)
}

// createServer 直接建一条 closed 区服（game_id 无需真实存在，SQLite 无 FK 强制）。
func createServer(t *testing.T, repo model.AdminStore, gameID, name, status, planned string) int64 {
	t.Helper()
	s, err := repo.CreateGameServer(gameID, &model.GameServer{
		Name: name, Region: "华东", Status: status, PlannedOpenAt: planned,
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	return s.Id
}

// isDue 全臂：空串非到期、非法值报错、过去到点、未来未到点。
func TestIsDue(t *testing.T) {
	if due, err := isDue("", time.Now()); err != nil || due {
		t.Fatalf("isDue('') = %v/%v, want false/nil", due, err)
	}
	if _, err := isDue("not-a-time", time.Now()); err == nil {
		t.Fatal("isDue(bad) = nil error, want parse error")
	}
	if due, _ := isDue("2000-01-01T00:00:00Z", time.Now()); !due {
		t.Fatal("isDue(past) = false, want true")
	}
	if due, _ := isDue("2999-01-01T00:00:00Z", time.Now()); due {
		t.Fatal("isDue(future) = true, want false")
	}
}

// NewRunner 非法周期回退 60 秒。
func TestNewRunner_DefaultInterval(t *testing.T) {
	r := NewRunner(nil, 0)
	if r.interval != 60*time.Second {
		t.Fatalf("interval = %v, want 60s", r.interval)
	}
	r = NewRunner(nil, time.Second)
	if r.interval != time.Second {
		t.Fatalf("interval = %v, want 1s", r.interval)
	}
}

// Run 已取消的 ctx 立即退出（未到第一个 tick）。
func TestRun_StopsOnCanceledCtx(t *testing.T) {
	r := &Runner{svcCtx: nil, interval: time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not exit on canceled ctx")
	}
}

// Run ticker 分支：真实扫描到点开服后 ctx 取消即退出。
func TestRun_TicksAndStops(t *testing.T) {
	svcCtx := newScanSvcCtx(t)
	createServer(t, svcCtx.AdminRepository, "game-t", "S-due", model.ServerStatusClosed, "2000-01-01T00:00:00Z")
	r := NewRunner(svcCtx, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()
	time.Sleep(40 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
	s, err := svcCtx.AdminRepository.GetGameServer(1)
	if err != nil || s.Status != model.ServerStatusOpen {
		t.Fatalf("server status = %v err=%v, want open", s.Status, err)
	}
}

// OpenOnce 主路径：到点 closed → open + 审计；未到点不动；空列表为 0。
func TestOpenOnce_MainPath(t *testing.T) {
	svcCtx := newScanSvcCtx(t)
	dueID := createServer(t, svcCtx.AdminRepository, "game-a", "A", model.ServerStatusClosed, "2000-01-01T00:00:00Z")
	createServer(t, svcCtx.AdminRepository, "game-a", "B", model.ServerStatusClosed, "2999-01-01T00:00:00Z")
	r := NewRunner(svcCtx, time.Second)
	if n := r.OpenOnce(context.Background()); n != 1 {
		t.Fatalf("opened=%d, want 1", n)
	}
	s, err := svcCtx.AdminRepository.GetGameServer(dueID)
	if err != nil || s.Status != model.ServerStatusOpen {
		t.Fatalf("due server = %s err=%v, want open", s.Status, err)
	}
	if n := r.OpenOnce(context.Background()); n != 0 {
		t.Fatalf("rescan opened=%d, want 0", n)
	}
}

// OpenOnce ListScheduledServers 失败 → 记日志返回 0。
func TestOpenOnce_ListError(t *testing.T) {
	svcCtx := newScanSvcCtx(t)
	svcCtx.AdminRepository = &failOpenStore{AdminStore: svcCtx.AdminRepository, listErr: errors.New("list down")}
	if n := NewRunner(svcCtx, time.Second).OpenOnce(context.Background()); n != 0 {
		t.Fatalf("opened=%d, want 0", n)
	}
}

// OpenOnce 排期值非法 → 跳过该服（留待人工），不 panic。
func TestOpenOnce_BadScheduleSkipped(t *testing.T) {
	svcCtx := newScanSvcCtx(t)
	createServer(t, svcCtx.AdminRepository, "game-b", "Bad", model.ServerStatusClosed, "not-a-time")
	if n := NewRunner(svcCtx, time.Second).OpenOnce(context.Background()); n != 0 {
		t.Fatalf("opened=%d, want 0", n)
	}
}

// OpenOnce OpenServerIfDue 失败 → 记日志跳过。
func TestOpenOnce_OpenError(t *testing.T) {
	svcCtx := newScanSvcCtx(t)
	createServer(t, svcCtx.AdminRepository, "game-c", "C", model.ServerStatusClosed, "2000-01-01T00:00:00Z")
	svcCtx.AdminRepository = &failOpenStore{AdminStore: svcCtx.AdminRepository, openErr: errors.New("open down")}
	if n := NewRunner(svcCtx, time.Second).OpenOnce(context.Background()); n != 0 {
		t.Fatalf("opened=%d, want 0", n)
	}
}

// OpenOnce 条件更新未命中（已被手动开服/改排期）→ 跳过不重发。
func TestOpenOnce_OpenNotOK(t *testing.T) {
	svcCtx := newScanSvcCtx(t)
	createServer(t, svcCtx.AdminRepository, "game-d", "D", model.ServerStatusClosed, "2000-01-01T00:00:00Z")
	no := false
	svcCtx.AdminRepository = &failOpenStore{AdminStore: svcCtx.AdminRepository, openOK: &no}
	if n := NewRunner(svcCtx, time.Second).OpenOnce(context.Background()); n != 0 {
		t.Fatalf("opened=%d, want 0", n)
	}
}

// OpenOnce 置 open 后重载失败 → 跳过（不通知不审计），不 panic。
func TestOpenOnce_ReloadError(t *testing.T) {
	svcCtx := newScanSvcCtx(t)
	createServer(t, svcCtx.AdminRepository, "game-e", "E", model.ServerStatusClosed, "2000-01-01T00:00:00Z")
	svcCtx.AdminRepository = &failOpenStore{AdminStore: svcCtx.AdminRepository, getErr: errors.New("reload down")}
	if n := NewRunner(svcCtx, time.Second).OpenOnce(context.Background()); n != 0 {
		t.Fatalf("opened=%d, want 0", n)
	}
}
