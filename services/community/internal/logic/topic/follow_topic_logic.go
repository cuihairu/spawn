// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package topic

import (
	"context"
	"errors"

	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type FollowTopicLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewFollowTopicLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FollowTopicLogic {
	return &FollowTopicLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *FollowTopicLogic) FollowTopic(req *types.FollowTopicReq) (resp *types.CommonResp, err error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.TopicId <= 0 {
		return nil, errors.New("topic_id required")
	}
	if _, err := l.svcCtx.TopicRepo.Get(req.TopicId); err != nil {
		return nil, err
	}

	if created := l.svcCtx.FollowRepo.FollowTopic(userId, req.TopicId); created {
		_ = l.svcCtx.TopicRepo.IncrementFollowerCount(req.TopicId, 1)
	}

	return &types.CommonResp{Code: 0, Message: "ok"}, nil
}
