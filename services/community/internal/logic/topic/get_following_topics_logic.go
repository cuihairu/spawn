// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package topic

import (
	"context"

	"community/internal/svc"
	"community/internal/types"

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
	// todo: add your logic here and delete this line

	return
}
