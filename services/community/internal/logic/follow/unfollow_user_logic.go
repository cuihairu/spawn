// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package follow

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UnfollowUserLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUnfollowUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnfollowUserLogic {
	return &UnfollowUserLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UnfollowUserLogic) UnfollowUser(req *types.FollowUserReq) (resp *types.CommonResp, err error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.UserId <= 0 {
		return nil, httperr.BadRequest("user_id required")
	}
	if req.UserId == userId {
		return nil, httperr.BadRequest("cannot unfollow yourself")
	}

	_ = l.svcCtx.FollowRepo.UnfollowUser(userId, req.UserId)
	return &types.CommonResp{Code: 0, Message: "ok"}, nil
}
