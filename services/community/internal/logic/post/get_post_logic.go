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

type GetPostLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetPostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostLogic {
	return &GetPostLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetPostLogic) GetPost(req *types.GetPostReq) (resp *types.PostResp, err error) {
	if req == nil || req.Id <= 0 {
		return nil, errors.New("id required")
	}

	_ = l.svcCtx.PostRepo.IncrementViews(req.Id)
	p, err := l.svcCtx.PostRepo.Get(req.Id)
	if err != nil {
		return nil, err
	}

	return &types.PostResp{Post: *p}, nil
}
