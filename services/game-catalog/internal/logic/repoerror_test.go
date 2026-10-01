package logic

import (
	"context"
	"errors"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"
)

// failingGameStore 内嵌真实仓储，仅覆写 Get/Create 注入错误，
// 触达 logic 层「查询/创建失败 → 500」分支。
type failingGameStore struct {
	model.GameStore
	getErr    error
	createErr error
}

func (f *failingGameStore) Get(id string) (*model.Game, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.GameStore.Get(id)
}

func (f *failingGameStore) Create(game *model.Game) (*model.Game, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.GameStore.Create(game)
}

// requireAPIStatus 断言 logic 返回 *apiError 且状态码匹配。
func requireAPIStatus(t *testing.T, err error, wantCode int) {
	t.Helper()
	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v (%T), want *apiError", err, err)
	}
	if apiErr.code != wantCode {
		t.Fatalf("apiError code = %d, want %d (message %q)", apiErr.code, wantCode, apiErr.message)
	}
}

// TestGetGameDetail_QueryFailed500 仓储返回非哨兵错误 → 500 查询失败。
func TestGetGameDetail_QueryFailed500(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	svcCtx.GameRepository = &failingGameStore{GameStore: svcCtx.GameRepository, getErr: errors.New("repository on fire")}

	_, err := NewGetGameDetailLogic(context.Background(), svcCtx).
		GetGameDetail(&types.GetGameDetailRequest{Id: "g-rpg-1"})
	requireAPIStatus(t, err, 500)
}

// TestCreateGame_CreateFailed500 仓储创建失败 → 500 创建失败
// （校验通过后的落库错误分支）。
func TestCreateGame_CreateFailed500(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	svcCtx.GameRepository = &failingGameStore{GameStore: svcCtx.GameRepository, createErr: errors.New("repository on fire")}

	_, err := NewCreateGameLogic(context.Background(), svcCtx).CreateGame(&types.CreateGameRequest{
		Title:       "T",
		Description: "D",
		Genres:      []string{"RPG"},
		Platforms:   []string{"PC"},
	})
	requireAPIStatus(t, err, 500)
}
