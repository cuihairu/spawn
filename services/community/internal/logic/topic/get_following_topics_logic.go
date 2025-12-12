// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package topic

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetFollowingTopicsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetFollowingTopicsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFollowingTopicsLogic {
	return &GetFollowingTopicsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetFollowingTopicsLogic) GetFollowingTopics() (resp *types.FollowingResp, err error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	ids := l.svcCtx.FollowRepo.ListFollowingTopicIds(userId)
	topics := make([]types.Topic, 0, len(ids))
	for _, id := range ids {
		t, err := l.svcCtx.TopicRepo.Get(id)
		if err != nil {
			// topic may be removed; ignore
			continue
		}
		topics = append(topics, *t)
	}

	return &types.FollowingResp{Topics: topics}, nil
}
