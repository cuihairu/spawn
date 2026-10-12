// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package message

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetDMThreadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetDMThreadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetDMThreadLogic {
	return &GetDMThreadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetDMThread 与某 peer 的往来消息（id 倒序 + total；分页钳制同帖子列表）。
func (l *GetDMThreadLogic) GetDMThread(req *types.DMThreadReq) (resp *types.DMThreadResp, err error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req.PeerId <= 0 {
		return nil, httperr.BadRequest("peer id is required")
	}
	limit, offset := req.Limit, req.Offset
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	list, total, err := l.svcCtx.MessageRepo.Thread(userId, req.PeerId, limit, offset)
	if err != nil {
		return nil, err
	}
	return &types.DMThreadResp{Messages: list, Total: total}, nil
}
