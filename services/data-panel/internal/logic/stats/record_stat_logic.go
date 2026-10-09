package stats

import (
	"context"
	"strings"

	"github.com/tappi/tappi/services/data-panel/internal/httperr"
	"github.com/tappi/tappi/services/data-panel/internal/logic/common"
	"github.com/tappi/tappi/services/data-panel/internal/svc"
	"github.com/tappi/tappi/services/data-panel/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type RecordStatLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRecordStatLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RecordStatLogic {
	return &RecordStatLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Record 战绩摄入（登录态）：归属恒取 JWT 的 user_id，body 不可指定。
// 增量语义：负数 delta 拒绝（单调累加契约），空 game_id 拒绝；
// played_at 缺省由模型层取当前 UTC 时间。
func (l *RecordStatLogic) Record(req *types.RecordStatReq) (*types.RecordedStatResp, error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, httperr.BadRequest("request body required")
	}

	req.GameId = strings.TrimSpace(req.GameId)
	if req.GameId == "" {
		return nil, httperr.BadRequest("game id is required")
	}

	deltas := []struct {
		name  string
		value int64
	}{
		{"matches", req.Matches},
		{"wins", req.Wins},
		{"kills", req.Kills},
		{"deaths", req.Deaths},
		{"assists", req.Assists},
		{"score", req.Score},
		{"rank_points", req.RankPoints},
	}
	for _, d := range deltas {
		if d.value < 0 {
			return nil, httperr.BadRequest(d.name + " must be non-negative")
		}
	}

	stat, err := l.svcCtx.StatsRepo.Record(userId, req)
	if err != nil {
		return nil, err
	}

	return &types.RecordedStatResp{
		Code:    200,
		Message: "ok",
		Stat:    *stat,
	}, nil
}
