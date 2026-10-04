// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package post

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type LikePostLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLikePostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LikePostLogic {
	return &LikePostLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LikePostLogic) LikePost(req *types.LikePostReq) (resp *types.CommonResp, err error) {
	if req == nil || req.Id <= 0 {
		return nil, httperr.BadRequest("id required")
	}
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if _, err := l.svcCtx.PostRepo.Like(req.Id, userId); err != nil {
		return nil, err
	}
	return &types.CommonResp{Code: 0, Message: "ok"}, nil
}
