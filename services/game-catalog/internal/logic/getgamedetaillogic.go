package logic

import (
	"context"
	"errors"
	"net/http"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetGameDetailLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetGameDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetGameDetailLogic {
	return &GetGameDetailLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetGameDetailLogic) GetGameDetail(req *types.GetGameDetailRequest) (*types.GameDetailResponse, error) {
	game, err := l.svcCtx.GameRepository.Get(req.Id)
	if err != nil {
		if errors.Is(err, model.ErrGameNotFound) {
			return nil, newAPIError(http.StatusNotFound, "游戏不存在")
		}
		l.Errorf("query game failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询失败")
	}

	// 公开详情只露出已上架游戏；提审中/驳回/下架由运营或合作方后台查看。
	if game.Status != model.GameStatusApproved {
		return nil, newAPIError(http.StatusNotFound, "游戏不存在")
	}

	return &types.GameDetailResponse{
		Game: toTypesGame(game),
	}, nil
}

// ListServers 公开区服列表（GP-201 充值目标）：与详情同口径只对已上架
// 游戏开放；closed 区服不外露（下线区服的充值对账只在合作方后台可见）。
func (l *GetGameDetailLogic) ListServers(req *types.GetGameDetailRequest) (*types.ServerListResponse, error) {
	game, err := l.svcCtx.GameRepository.Get(req.Id)
	if err != nil {
		if errors.Is(err, model.ErrGameNotFound) {
			return nil, newAPIError(http.StatusNotFound, "游戏不存在")
		}
		l.Errorf("query game failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询失败")
	}
	if game.Status != model.GameStatusApproved {
		return nil, newAPIError(http.StatusNotFound, "游戏不存在")
	}
	servers, err := l.svcCtx.AdminRepository.ListGameServers(req.Id, false)
	if err != nil {
		l.Errorf("list game servers failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询区服失败")
	}
	items := make([]types.GameServer, 0, len(servers))
	for _, s := range servers {
		items = append(items, *toTypesServer(s))
	}
	return &types.ServerListResponse{Items: items}, nil
}
