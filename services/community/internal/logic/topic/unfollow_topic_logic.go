// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package topic

import (
	"context"

	"community/internal/svc"
	"community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UnfollowTopicLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUnfollowTopicLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnfollowTopicLogic {
	return &UnfollowTopicLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UnfollowTopicLogic) UnfollowTopic(req *types.FollowTopicReq) (resp *types.CommonResp, err error) {
	// todo: add your logic here and delete this line

	return
}
