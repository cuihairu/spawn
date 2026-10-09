// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package moderation

import (
	"context"
	"time"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ReportPostLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewReportPostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReportPostLogic {
	return &ReportPostLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ReportPost 举报帖子（登录用户，reason 可选）：目标必须存在；
// 同一用户对同一目标重复举报由唯一键去重（幂等，不报错）。
func (l *ReportPostLogic) ReportPost(req *types.ReportPostReq) (resp *types.CommonResp, err error) {
	if req == nil || req.Id <= 0 {
		return nil, httperr.BadRequest("id required")
	}
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if _, err := l.svcCtx.PostRepo.Get(req.Id); err != nil {
		return nil, err
	}

	report := &types.Report{
		ReporterId: userId,
		TargetType: "post",
		TargetId:   req.Id,
		Reason:     req.Reason,
		Status:     "pending",
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	if err := l.svcCtx.ReportRepo.Create(report); err != nil {
		return nil, err
	}

	return &types.CommonResp{Code: 0, Message: "report submitted"}, nil
}
