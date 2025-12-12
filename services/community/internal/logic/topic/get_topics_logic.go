// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package topic

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetTopicsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetTopicsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetTopicsLogic {
	return &GetTopicsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetTopicsLogic) GetTopics(req *types.GetTopicsReq) (resp *types.TopicsResp, err error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	topics, total := l.svcCtx.TopicRepo.List(req.Keyword, req.IsOfficial, limit, offset)
	return &types.TopicsResp{Topics: topics, Total: total}, nil
}
