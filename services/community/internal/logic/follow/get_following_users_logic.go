// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package follow

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetFollowingUsersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetFollowingUsersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFollowingUsersLogic {
	return &GetFollowingUsersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetFollowingUsers 已关注用户 id 列表（升序）。community 不持有用户资料，
// 名字等信息由客户端向 user-service 取；空关注返回空数组而非 null。
func (l *GetFollowingUsersLogic) GetFollowingUsers() (resp *types.FollowingUsersResp, err error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	ids := l.svcCtx.FollowRepo.ListFollowingUserIds(userId)
	if ids == nil {
		ids = []int64{}
	}

	return &types.FollowingUsersResp{UserIds: ids}, nil
}
