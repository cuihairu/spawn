package stats

import (
	"context"

	"github.com/tappi/tappi/services/data-panel/internal/httperr"
	"github.com/tappi/tappi/services/data-panel/internal/svc"
	"github.com/tappi/tappi/services/data-panel/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetSummaryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetSummaryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetSummaryLogic {
	return &GetSummaryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetSummary 玩家跨游戏汇总（公开读；无战绩返回全零汇总）。
func (l *GetSummaryLogic) GetSummary(req *types.GetSummaryReq) (*types.SummaryResp, error) {
	if req == nil || req.UserId <= 0 {
		return nil, httperr.BadRequest("invalid user id")
	}

	summary, err := l.svcCtx.StatsRepo.Summary(req.UserId)
	if err != nil {
		return nil, err
	}
	return &types.SummaryResp{Summary: *summary}, nil
}
