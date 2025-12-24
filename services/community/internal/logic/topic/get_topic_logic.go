// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package topic

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetTopicLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetTopicLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetTopicLogic {
	return &GetTopicLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetTopicLogic) GetTopic(req *types.GetTopicReq) (resp *types.TopicResp, err error) {
	if req == nil || req.Id <= 0 {
		return nil, httperr.BadRequest("id required")
	}
	t, err := l.svcCtx.TopicRepo.Get(req.Id)
	if err != nil {
		return nil, err
	}
	return &types.TopicResp{Topic: *t}, nil
}
