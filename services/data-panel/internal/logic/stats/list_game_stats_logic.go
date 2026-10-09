package stats

import (
	"context"

	"github.com/tappi/tappi/services/data-panel/internal/httperr"
	"github.com/tappi/tappi/services/data-panel/internal/svc"
	"github.com/tappi/tappi/services/data-panel/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListGameStatsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListGameStatsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListGameStatsLogic {
	return &ListGameStatsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListGameStats 按用户列战绩明细（公开读，场次降序；分页钳制与帖子列表同款）。
func (l *ListGameStatsLogic) ListGameStats(req *types.ListGameStatsReq) (*types.GameStatsResp, error) {
	if req == nil || req.UserId <= 0 {
		return nil, httperr.BadRequest("invalid user id")
	}

	limit, offset := req.Limit, req.Offset
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	games, total, err := l.svcCtx.StatsRepo.ListGames(req.UserId, limit, offset)
	if err != nil {
		return nil, err
	}
	if games == nil {
		games = []types.PlayerStat{}
	}
	return &types.GameStatsResp{Games: games, Total: total}, nil
}
