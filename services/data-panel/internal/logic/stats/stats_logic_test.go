package stats

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/data-panel/internal/config"
	"github.com/tappi/tappi/services/data-panel/internal/httperr"
	"github.com/tappi/tappi/services/data-panel/internal/model"
	"github.com/tappi/tappi/services/data-panel/internal/svc"
	"github.com/tappi/tappi/services/data-panel/internal/types"
)

// newTestServiceContext 与其他服务 logic 包同款：真实服务上下文 + SQLite 临时库。
func newTestServiceContext(t *testing.T) *svc.ServiceContext {
	t.Helper()

	var c config.Config
	c.Auth.JWTSecret = "test-jwt-secret"
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "stats.db")

	return svc.NewServiceContext(c)
}

func authContext(userId int64, username string) context.Context {
	ctx := context.WithValue(context.Background(), "user_id", userId)
	return context.WithValue(ctx, "username", username)
}

func requireHTTPStatus(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want HTTP %d", want)
	}
	var appErr *httperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("err = %v (%T), want *httperr.Error with status %d", err, err, want)
	}
	if appErr.Status != want {
		t.Fatalf("status = %d, want %d (message %q)", appErr.Status, want, appErr.Message)
	}
}

func TestGetSummary(t *testing.T) {
	s := newTestServiceContext(t)

	// 无战绩用户 → 全零汇总（公开读，不 404）
	resp, err := NewGetSummaryLogic(context.Background(), s).GetSummary(&types.GetSummaryReq{UserId: 424242})
	if err != nil {
		t.Fatalf("empty summary: %v", err)
	}
	if resp.Summary.UserId != 424242 || resp.Summary.TotalMatches != 0 {
		t.Fatalf("empty summary mismatch: %+v", resp.Summary)
	}

	// 种子用户 1001：seed 里 elden-ring 42 场 + valorant 87 场 = 129
	resp, err = NewGetSummaryLogic(context.Background(), s).GetSummary(&types.GetSummaryReq{UserId: 1001})
	if err != nil {
		t.Fatalf("seeded summary: %v", err)
	}
	if resp.Summary.TotalMatches != 129 || resp.Summary.GameCount != 2 {
		t.Fatalf("seeded summary mismatch: %+v", resp.Summary)
	}

	// 非法参数 400
	_, err = NewGetSummaryLogic(context.Background(), s).GetSummary(&types.GetSummaryReq{UserId: 0})
	requireHTTPStatus(t, err, 400)
	_, err = NewGetSummaryLogic(context.Background(), s).GetSummary(nil)
	requireHTTPStatus(t, err, 400)
}

func TestListGameStats(t *testing.T) {
	s := newTestServiceContext(t)

	l := NewListGameStatsLogic(context.Background(), s)
	resp, err := l.ListGameStats(&types.ListGameStatsReq{UserId: 1002})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.Total != 1 || len(resp.Games) != 1 || resp.Games[0].GameId != "game-apex" {
		t.Fatalf("list mismatch: %+v", resp)
	}

	// 分页钳制：limit<=0 → 20、>100 → 100、offset<0 → 0
	page, err := l.ListGameStats(&types.ListGameStatsReq{UserId: 1001, Limit: 1, Offset: 1})
	if err != nil || len(page.Games) != 1 {
		t.Fatalf("paged: n=%d err=%v", len(page.Games), err)
	}
	_, err = l.ListGameStats(&types.ListGameStatsReq{UserId: 0})
	requireHTTPStatus(t, err, 400)
}

func TestGetGameStat(t *testing.T) {
	s := newTestServiceContext(t)

	resp, err := NewGetGameStatLogic(context.Background(), s).GetGameStat(&types.GetGameStatReq{
		UserId: 1001, GameId: "game-elden-ring",
	})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if resp.Stat.GameTitle != "Elden Ring" || resp.Stat.Matches != 42 || resp.Stat.Wins != 25 {
		t.Fatalf("stat mismatch: %+v", resp.Stat)
	}
	if resp.Stat.WinRate != 0.5952 || resp.Stat.Kd != 1.7222 {
		t.Fatalf("ratio mismatch: win_rate=%v kd=%v", resp.Stat.WinRate, resp.Stat.Kd)
	}

	// 未命中走模型哨兵错误（HTTP 层由 ErrorHandler 映射 404）
	_, err = NewGetGameStatLogic(context.Background(), s).GetGameStat(&types.GetGameStatReq{
		UserId: 1001, GameId: "game-nonexistent",
	})
	if !errors.Is(err, model.ErrStatNotFound) {
		t.Fatalf("missing stat err=%v, want ErrStatNotFound", err)
	}
	// 非法参数 400
	_, err = NewGetGameStatLogic(context.Background(), s).GetGameStat(&types.GetGameStatReq{})
	requireHTTPStatus(t, err, 400)
}

func TestRecordStat(t *testing.T) {
	s := newTestServiceContext(t)

	created, err := NewRecordStatLogic(authContext(3001, "玩家丙"), s).Record(&types.RecordStatReq{
		GameId: "game-valorant", GameTitle: "Valorant",
		Matches: 1, Wins: 1, Kills: 20, Deaths: 10, Score: 300, RankPoints: 25,
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if created.Code != 200 || created.Stat.Matches != 1 || created.Stat.WinRate != 1 {
		t.Fatalf("created mismatch: %+v", created.Stat)
	}

	// 增量累加 + 归属取 JWT user（不同用户各自成行）
	again, err := NewRecordStatLogic(authContext(3001, "玩家丙"), s).Record(&types.RecordStatReq{
		GameId: "game-valorant", Matches: 2, Kills: 15, Deaths: 12,
	})
	if err != nil {
		t.Fatalf("record again: %v", err)
	}
	if again.Stat.Matches != 3 || again.Stat.Wins != 1 || again.Stat.Kills != 35 {
		t.Fatalf("increment mismatch: %+v", again.Stat)
	}
	// 空标题不覆盖、played_at 缺省自动补当前时间
	if again.Stat.GameTitle != "Valorant" || again.Stat.LastPlayedAt == "" {
		t.Fatalf("merge fields mismatch: %+v", again.Stat)
	}

	// 校验：401（缺登录）/ 400（nil 请求、空 game_id、负 delta）
	_, err = NewRecordStatLogic(context.Background(), s).Record(&types.RecordStatReq{GameId: "x", Matches: 1})
	requireHTTPStatus(t, err, 401)
	_, err = NewRecordStatLogic(authContext(3001, "玩家丙"), s).Record(nil)
	requireHTTPStatus(t, err, 400)
	_, err = NewRecordStatLogic(authContext(3001, "玩家丙"), s).Record(&types.RecordStatReq{GameId: "   "})
	requireHTTPStatus(t, err, 400)
	_, err = NewRecordStatLogic(authContext(3001, "玩家丙"), s).Record(&types.RecordStatReq{
		GameId: "game-valorant", Kills: -1,
	})
	requireHTTPStatus(t, err, 400)
}
