// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package follow

import (
	"context"
	"errors"

	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type FollowUserLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewFollowUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FollowUserLogic {
	return &FollowUserLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *FollowUserLogic) FollowUser(req *types.FollowUserReq) (resp *types.CommonResp, err error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.UserId <= 0 {
		return nil, errors.New("user_id required")
	}
	if req.UserId == userId {
		return nil, errors.New("cannot follow yourself")
	}

	_ = l.svcCtx.FollowRepo.FollowUser(userId, req.UserId)
	return &types.CommonResp{Code: 0, Message: "ok"}, nil
}
