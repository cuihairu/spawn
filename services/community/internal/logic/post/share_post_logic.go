// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package post

import (
	"context"
	"errors"

	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SharePostLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSharePostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SharePostLogic {
	return &SharePostLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SharePostLogic) SharePost(req *types.SharePostReq) (resp *types.CommonResp, err error) {
	if req == nil || req.Id <= 0 {
		return nil, errors.New("id required")
	}
	if _, err := l.svcCtx.PostRepo.Share(req.Id); err != nil {
		return nil, err
	}
	return &types.CommonResp{Code: 0, Message: "ok"}, nil
}
