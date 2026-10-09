// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package post

import (
	"context"
	"fmt"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/moderation"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdatePostLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdatePostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdatePostLogic {
	return &UpdatePostLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdatePostLogic) UpdatePost(req *types.UpdatePostReq) (resp *types.PostResp, err error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.Id <= 0 {
		return nil, httperr.BadRequest("id required")
	}
	// 仅扫描本次提供的字段（未提供的字段保持原值，不参与过滤）。
	if hit, ok := moderation.FirstHit(l.svcCtx.BlockedWords, req.Title+" "+req.Content); ok {
		return nil, httperr.BadRequest(fmt.Sprintf("content contains blocked word: %q", hit))
	}

	p, err := l.svcCtx.PostRepo.Update(req.Id, userId, req)
	if err != nil {
		return nil, err
	}

	return &types.PostResp{Post: *p}, nil
}
