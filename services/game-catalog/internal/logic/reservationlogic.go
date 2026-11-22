package logic

// GP-306 开服计划与预约（F21）：玩家对未开服区服预约（幂等），开服时经
// community 收到站内通知 + realtime-hub 远程推送（提醒只发一次）；游戏维度
// 去重预约人群同时作为版本/活动通知受众（S2S 供 promotions 拉取）。

import (
	"context"
	"errors"
	"net/http"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type ReservationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewReservationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReservationLogic {
	return &ReservationLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Reserve 预约开服（幂等：重复预约返回既有，created=false）。
// 区服须属于该游戏且未开服（已开服无需预约 409）。
func (l *ReservationLogic) Reserve(req *types.ServerPathRequest) (*types.ReserveServerResponse, error) {
	userID, _ := l.ctx.Value("user_id").(int64)
	if userID <= 0 {
		return nil, newAPIError(http.StatusUnauthorized, "未登录")
	}
	s, err := l.svcCtx.AdminRepository.GetGameServer(req.ServerId)
	if errors.Is(err, model.ErrServerNotFound) || (err == nil && s.GameId != req.Id) {
		return nil, newAPIError(http.StatusNotFound, "区服不存在")
	}
	if err != nil {
		l.Errorf("get server %d failed: %v", req.ServerId, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询区服失败")
	}
	if s.Status == model.ServerStatusOpen {
		return nil, newAPIError(http.StatusConflict, "区服已开服，无需预约")
	}
	_, created, err := l.svcCtx.AdminRepository.ReserveServer(&model.ServerReservation{
		GameId: req.Id, ServerId: req.ServerId, UserId: userID,
	})
	if err != nil {
		l.Errorf("reserve server %d for user %d failed: %v", req.ServerId, userID, err)
		return nil, newAPIError(http.StatusInternalServerError, "预约失败")
	}
	return &types.ReserveServerResponse{
		ServerId: req.ServerId, GameId: req.Id, Status: s.Status,
		PlannedOpenAt: s.PlannedOpenAt, Created: created,
	}, nil
}

// Cancel 取消预约（幂等：未预约也返回成功）。
func (l *ReservationLogic) Cancel(req *types.ServerPathRequest) error {
	userID, _ := l.ctx.Value("user_id").(int64)
	if userID <= 0 {
		return newAPIError(http.StatusUnauthorized, "未登录")
	}
	s, err := l.svcCtx.AdminRepository.GetGameServer(req.ServerId)
	if errors.Is(err, model.ErrServerNotFound) || (err == nil && s.GameId != req.Id) {
		return newAPIError(http.StatusNotFound, "区服不存在")
	}
	if err != nil {
		l.Errorf("get server %d failed: %v", req.ServerId, err)
		return newAPIError(http.StatusInternalServerError, "查询区服失败")
	}
	if err := l.svcCtx.AdminRepository.CancelReservation(req.ServerId, userID); err != nil {
		l.Errorf("cancel reservation server %d user %d failed: %v", req.ServerId, userID, err)
		return newAPIError(http.StatusInternalServerError, "取消预约失败")
	}
	return nil
}

// MyReservations 我的开服预约（含区服现状：名称/状态/开服排期/是否已提醒）。
func (l *ReservationLogic) MyReservations() (*types.MyReservationsResponse, error) {
	userID, _ := l.ctx.Value("user_id").(int64)
	if userID <= 0 {
		return nil, newAPIError(http.StatusUnauthorized, "未登录")
	}
	rows, err := l.svcCtx.AdminRepository.ListMyReservations(userID, 100)
	if err != nil {
		l.Errorf("list reservations for user %d failed: %v", userID, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询预约失败")
	}
	items := make([]types.MyReservationView, 0, len(rows))
	for _, r := range rows {
		s, err := l.svcCtx.AdminRepository.GetGameServer(r.ServerId)
		if err != nil {
			continue // 区服已删除：跳过（预约行保留供审计）
		}
		items = append(items, types.MyReservationView{
			GameId: r.GameId, ServerId: r.ServerId, ServerName: s.Name,
			ServerStatus: s.Status, PlannedOpenAt: s.PlannedOpenAt,
			NotifiedAt: r.NotifiedAt, CreatedAt: r.CreatedAt,
		})
	}
	return &types.MyReservationsResponse{Items: items}, nil
}

// ListReservationUsers S2S：游戏维度去重预约人群（版本/活动通知受众）。
func (l *ReservationLogic) ListReservationUsers(gameID string) (*types.InternalReservationUsersResponse, error) {
	users, err := l.svcCtx.AdminRepository.ListReservationUsersByGame(gameID, 10000)
	if err != nil {
		l.Errorf("list reservation users for game %s failed: %v", gameID, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询预约人群失败")
	}
	return &types.InternalReservationUsersResponse{GameId: gameID, UserIds: users}, nil
}
