// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package moderation

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListReportsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListReportsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListReportsLogic {
	return &ListReportsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListReports 审核举报队列（仅管理员）：status 可选过滤（空 = 全部），
// id 倒序分页，空结果规整为空切片非 nil。分页钳制与 GetPosts 同款。
func (l *ListReportsLogic) ListReports(req *types.ListReportsReq) (resp *types.ReportsResp, err error) {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if !l.svcCtx.IsAdmin(userId) {
		return nil, httperr.Forbidden("admin permission required")
	}
	if req == nil {
		req = &types.ListReportsReq{}
	}

	// defaults are already defined in api, but keep defensive guards here.
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	list, total, err := l.svcCtx.ReportRepo.ListByStatus(req.Status, limit, offset)
	if err != nil {
		return nil, err
	}

	return &types.ReportsResp{Reports: list, Total: total}, nil
}
