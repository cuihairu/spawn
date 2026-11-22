package logic

import (
	"errors"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"
)

// failingReservationStore GP-306 预约 logic 错误分支注入：内嵌真实仓储，
// 仅按字段覆写指定方法。
type failingReservationStore struct {
	model.AdminStore
	getServerErr error
	reserveErr   error
	cancelErr    error
	listMineErr  error
	listMineRows []*model.ServerReservation
	resvUsersErr error
}

var _ model.AdminStore = (*failingReservationStore)(nil)

func (f *failingReservationStore) GetGameServer(id int64) (*model.GameServer, error) {
	if f.getServerErr != nil {
		return nil, f.getServerErr
	}
	return f.AdminStore.GetGameServer(id)
}

func (f *failingReservationStore) ReserveServer(r *model.ServerReservation) (*model.ServerReservation, bool, error) {
	if f.reserveErr != nil {
		return nil, false, f.reserveErr
	}
	return f.AdminStore.ReserveServer(r)
}

func (f *failingReservationStore) CancelReservation(serverId, userId int64) error {
	if f.cancelErr != nil {
		return f.cancelErr
	}
	return f.AdminStore.CancelReservation(serverId, userId)
}

func (f *failingReservationStore) ListMyReservations(userId int64, limit int) ([]*model.ServerReservation, error) {
	if f.listMineErr != nil {
		return nil, f.listMineErr
	}
	if f.listMineRows != nil {
		return f.listMineRows, nil
	}
	return f.AdminStore.ListMyReservations(userId, limit)
}

func (f *failingReservationStore) ListReservationUsersByGame(gameId string, limit int) ([]int64, error) {
	if f.resvUsersErr != nil {
		return nil, f.resvUsersErr
	}
	return f.AdminStore.ListReservationUsersByGame(gameId, limit)
}

// newReservationCtx 预约测试上下文：真实仓储 + 一个 closed 区服 S1（g-rpg-1）；
// 返回 svcCtx 与 S1 的 id。
func newReservationCtx(t *testing.T) (*svc.ServiceContext, int64) {
	t.Helper()
	svcCtx := newLogicSvcCtx(t)
	s, err := svcCtx.AdminRepository.CreateGameServer("g-rpg-1", &model.GameServer{
		Name: "S1", Status: model.ServerStatusClosed,
	})
	if err != nil {
		t.Fatalf("CreateGameServer: %v", err)
	}
	return svcCtx, s.Id
}

// 预约：未登录 → 401；区服不存在/跨游戏 → 404；查询失败 → 500；已开服 → 409；
// 落库失败 → 500。
func TestReservationLogic_ReserveErrors(t *testing.T) {
	t.Run("unauthorized 401", func(t *testing.T) {
		svcCtx, serverID := newReservationCtx(t)
		l := NewReservationLogic(reviewCtx(0), svcCtx)
		_, err := l.Reserve(&types.ServerPathRequest{Id: "g-rpg-1", ServerId: serverID})
		requireAPIStatus(t, err, 401)
	})
	t.Run("server not found 404", func(t *testing.T) {
		svcCtx, _ := newReservationCtx(t)
		svcCtx.AdminRepository = &failingReservationStore{
			AdminStore:   svcCtx.AdminRepository,
			getServerErr: model.ErrServerNotFound,
		}
		l := NewReservationLogic(reviewCtx(7), svcCtx)
		_, err := l.Reserve(&types.ServerPathRequest{Id: "g-rpg-1", ServerId: 999})
		requireAPIStatus(t, err, 404)
	})
	t.Run("cross game 404", func(t *testing.T) {
		svcCtx, serverID := newReservationCtx(t)
		l := NewReservationLogic(reviewCtx(7), svcCtx)
		_, err := l.Reserve(&types.ServerPathRequest{Id: "g-rpg-2", ServerId: serverID})
		requireAPIStatus(t, err, 404)
	})
	t.Run("query failed 500", func(t *testing.T) {
		svcCtx, _ := newReservationCtx(t)
		svcCtx.AdminRepository = &failingReservationStore{
			AdminStore:   svcCtx.AdminRepository,
			getServerErr: errors.New("query down"),
		}
		l := NewReservationLogic(reviewCtx(7), svcCtx)
		_, err := l.Reserve(&types.ServerPathRequest{Id: "g-rpg-1", ServerId: 999})
		requireAPIStatus(t, err, 500)
	})
	t.Run("already open 409", func(t *testing.T) {
		svcCtx, serverID := newReservationCtx(t)
		if _, err := svcCtx.AdminRepository.UpdateGameServer(&model.GameServer{Id: serverID, GameId: "g-rpg-1", Name: "S1", Status: model.ServerStatusOpen}); err != nil {
			t.Fatalf("UpdateGameServer: %v", err)
		}
		l := NewReservationLogic(reviewCtx(7), svcCtx)
		_, err := l.Reserve(&types.ServerPathRequest{Id: "g-rpg-1", ServerId: serverID})
		requireAPIStatus(t, err, 409)
	})
	t.Run("reserve failed 500", func(t *testing.T) {
		svcCtx, serverID := newReservationCtx(t)
		svcCtx.AdminRepository = &failingReservationStore{
			AdminStore: svcCtx.AdminRepository,
			reserveErr: errors.New("reserve down"),
		}
		l := NewReservationLogic(reviewCtx(7), svcCtx)
		_, err := l.Reserve(&types.ServerPathRequest{Id: "g-rpg-1", ServerId: serverID})
		requireAPIStatus(t, err, 500)
	})
	t.Run("success idempotent", func(t *testing.T) {
		svcCtx, serverID := newReservationCtx(t)
		l := NewReservationLogic(reviewCtx(7), svcCtx)
		resp, err := l.Reserve(&types.ServerPathRequest{Id: "g-rpg-1", ServerId: serverID})
		if err != nil || !resp.Created {
			t.Fatalf("resp=%+v err=%v", resp, err)
		}
	})
}

// 取消预约：未登录 → 401；区服不存在 → 404；查询失败 → 500；取消失败 → 500。
func TestReservationLogic_CancelErrors(t *testing.T) {
	t.Run("unauthorized 401", func(t *testing.T) {
		svcCtx, serverID := newReservationCtx(t)
		l := NewReservationLogic(reviewCtx(0), svcCtx)
		if err := l.Cancel(&types.ServerPathRequest{Id: "g-rpg-1", ServerId: serverID}); err != nil {
			requireAPIStatus(t, err, 401)
		} else {
			t.Fatal("want 401")
		}
	})
	t.Run("server not found 404", func(t *testing.T) {
		svcCtx, _ := newReservationCtx(t)
		svcCtx.AdminRepository = &failingReservationStore{
			AdminStore:   svcCtx.AdminRepository,
			getServerErr: model.ErrServerNotFound,
		}
		l := NewReservationLogic(reviewCtx(7), svcCtx)
		if err := l.Cancel(&types.ServerPathRequest{Id: "g-rpg-1", ServerId: 999}); err != nil {
			requireAPIStatus(t, err, 404)
		} else {
			t.Fatal("want 404")
		}
	})
	t.Run("cancel failed 500", func(t *testing.T) {
		svcCtx, serverID := newReservationCtx(t)
		svcCtx.AdminRepository = &failingReservationStore{
			AdminStore: svcCtx.AdminRepository,
			cancelErr:  errors.New("cancel down"),
		}
		l := NewReservationLogic(reviewCtx(7), svcCtx)
		if err := l.Cancel(&types.ServerPathRequest{Id: "g-rpg-1", ServerId: serverID}); err != nil {
			requireAPIStatus(t, err, 500)
		} else {
			t.Fatal("want 500")
		}
	})
	t.Run("success", func(t *testing.T) {
		svcCtx, serverID := newReservationCtx(t)
		if _, _, err := svcCtx.AdminRepository.ReserveServer(&model.ServerReservation{GameId: "g-rpg-1", ServerId: serverID, UserId: 7}); err != nil {
			t.Fatalf("ReserveServer: %v", err)
		}
		l := NewReservationLogic(reviewCtx(7), svcCtx)
		if err := l.Cancel(&types.ServerPathRequest{Id: "g-rpg-1", ServerId: serverID}); err != nil {
			t.Fatalf("cancel: %v", err)
		}
	})
}

// 我的预约：未登录 → 401；查询失败 → 500；区服已删的预约行跳过不阻断。
func TestReservationLogic_MyReservationsErrors(t *testing.T) {
	t.Run("unauthorized 401", func(t *testing.T) {
		svcCtx, _ := newReservationCtx(t)
		l := NewReservationLogic(reviewCtx(0), svcCtx)
		if _, err := l.MyReservations(); err != nil {
			requireAPIStatus(t, err, 401)
		} else {
			t.Fatal("want 401")
		}
	})
	t.Run("list failed 500", func(t *testing.T) {
		svcCtx, _ := newReservationCtx(t)
		svcCtx.AdminRepository = &failingReservationStore{
			AdminStore:  svcCtx.AdminRepository,
			listMineErr: errors.New("list down"),
		}
		l := NewReservationLogic(reviewCtx(7), svcCtx)
		if _, err := l.MyReservations(); err != nil {
			requireAPIStatus(t, err, 500)
		} else {
			t.Fatal("want 500")
		}
	})
	t.Run("deleted server row skipped", func(t *testing.T) {
		svcCtx, serverID := newReservationCtx(t)
		// 列表返回一条指向已删区服（999）的预约 → MyReservations 应跳过该行
		svcCtx.AdminRepository = &failingReservationStore{
			AdminStore:   svcCtx.AdminRepository,
			listMineRows: []*model.ServerReservation{{GameId: "g-rpg-1", ServerId: 999, UserId: 7}},
		}
		_ = serverID
		l := NewReservationLogic(reviewCtx(7), svcCtx)
		resp, err := l.MyReservations()
		if err != nil {
			t.Fatalf("want success: %v", err)
		}
		if len(resp.Items) != 0 {
			t.Fatalf("items = %+v, want empty", resp.Items)
		}
	})
}

// S2S 预约人群：查询失败 → 500。
func TestReservationLogic_ListReservationUsersFailed500(t *testing.T) {
	svcCtx, _ := newReservationCtx(t)
	svcCtx.AdminRepository = &failingReservationStore{
		AdminStore:   svcCtx.AdminRepository,
		resvUsersErr: errors.New("resv down"),
	}
	l := NewReservationLogic(reviewCtx(1), svcCtx)
	if _, err := l.ListReservationUsers("g-rpg-1"); err != nil {
		requireAPIStatus(t, err, 500)
	} else {
		t.Fatal("want 500")
	}
}
