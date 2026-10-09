package stats

import (
	"context"

	"github.com/tappi/tappi/services/data-panel/internal/httperr"
	"github.com/tappi/tappi/services/data-panel/internal/svc"
	"github.com/tappi/tappi/services/data-panel/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetGameStatLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetGameStatLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetGameStatLogic {
	return &GetGameStatLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetGameStat 单游戏战绩详情（公开读；未命中走模型哨兵 ErrStatNotFound，
// HTTP 层由 ErrorHandler 映射 404）。
func (l *GetGameStatLogic) GetGameStat(req *types.GetGameStatReq) (*types.PlayerStatResp, error) {
	if req == nil || req.UserId <= 0 || req.GameId == "" {
		return nil, httperr.BadRequest("invalid user id or game id")
	}

	stat, err := l.svcCtx.StatsRepo.GetGame(req.UserId, req.GameId)
	if err != nil {
		return nil, err
	}
	return &types.PlayerStatResp{Stat: *stat}, nil
}
