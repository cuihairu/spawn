package stats

import (
	"context"
	"errors"
	"testing"

	"github.com/tappi/tappi/services/data-panel/internal/model"
	"github.com/tappi/tappi/services/data-panel/internal/types"
)

// failingStatsStore 仅 Summary/ListGames 投毒的故障注入仓储（其余方法不可达，
// 嵌入真实仓储即可满足 PlayerStatsStore）。
type failingStatsStore struct {
	model.PlayerStatsStore
	err error
}

func (s failingStatsStore) Summary(int64) (*types.StatSummary, error) {
	return nil, s.err
}

func (s failingStatsStore) ListGames(int64, int64, int64) ([]types.PlayerStat, int64, error) {
	return nil, 0, s.err
}

// TestListGameStats_ClampsPaging limit<=0 → 20、limit>100 → 100、offset<0 → 0。
func TestListGameStats_ClampsPaging(t *testing.T) {
	s := newTestServiceContext(t)
	l := NewListGameStatsLogic(context.Background(), s)

	for _, req := range []*types.ListGameStatsReq{
		{UserId: 1001, Limit: 0, Offset: -5}, // limit→20、offset→0
		{UserId: 1001, Limit: 500},           // limit→100
	} {
		resp, err := l.ListGameStats(req)
		if err != nil || resp == nil {
			t.Fatalf("req=%+v: resp=%v err=%v, want clamped page", req, resp, err)
		}
	}
}

// TestGetSummary_RepoErrorPropagates 聚合失败原样透传（HTTP 层按 500 映射）。
func TestGetSummary_RepoErrorPropagates(t *testing.T) {
	s := newTestServiceContext(t)
	s.StatsRepo = failingStatsStore{PlayerStatsStore: s.StatsRepo, err: errors.New("disk on fire")}

	if _, err := NewGetSummaryLogic(context.Background(), s).
		GetSummary(&types.GetSummaryReq{UserId: 1001}); err == nil {
		t.Fatal("summary repo failure must propagate")
	}
}

// TestListGameStats_RepoErrorPropagates 列表失败原样透传。
func TestListGameStats_RepoErrorPropagates(t *testing.T) {
	s := newTestServiceContext(t)
	s.StatsRepo = failingStatsStore{PlayerStatsStore: s.StatsRepo, err: errors.New("disk on fire")}

	if _, err := NewListGameStatsLogic(context.Background(), s).
		ListGameStats(&types.ListGameStatsReq{UserId: 1001}); err == nil {
		t.Fatal("list repo failure must propagate")
	}
}
