// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package moderation

import (
	"context"
	"net/http"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type HandleReportLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewHandleReportLogic(ctx context.Context, svcCtx *svc.ServiceContext) *HandleReportLogic {
	return &HandleReportLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// HandleReport 处置举报（仅管理员，仅 pending 可处置）：
//   - dismiss 驳回：只改状态，内容保留；
//   - resolve 处置：先删内容（post 软删 / comment 物理删除）再置 resolved，
//     删内容失败不落 resolved 状态。
//
// 状态流转走条件更新（UPDATE ... WHERE status='pending'）原子占位：
// 并发处置同一举报时只有一个管理员成功，后来者得到 409，
// 不会出现状态被覆盖或重复计数。
func (l *HandleReportLogic) HandleReport(req *types.HandleReportReq) (resp *types.CommonResp, err error) {
	if req == nil || req.Id <= 0 {
		return nil, httperr.BadRequest("id required")
	}
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if !l.svcCtx.IsAdmin(userId) {
		return nil, httperr.Forbidden("admin permission required")
	}

	report, err := l.svcCtx.ReportRepo.GetByID(req.Id)
	if err != nil {
		return nil, err
	}
	if report.Status != "pending" {
		return nil, alreadyHandledConflict()
	}

	switch req.Action {
	case "dismiss":
		ok, err := l.svcCtx.ReportRepo.UpdateStatusIfPending(report.Id, "dismissed", userId)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, alreadyHandledConflict()
		}
	case "resolve":
		if err := l.removeContent(report.TargetType, report.TargetId); err != nil {
			return nil, err
		}
		ok, err := l.svcCtx.ReportRepo.UpdateStatusIfPending(report.Id, "resolved", userId)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, alreadyHandledConflict()
		}
	default:
		return nil, httperr.BadRequest("action must be dismiss or resolve")
	}

	return &types.CommonResp{Code: 0, Message: "report handled"}, nil
}

// alreadyHandledConflict 举报已被处置（或并发处置竞争失败）→ 409。
func alreadyHandledConflict() error {
	return httperr.New(http.StatusConflict, http.StatusConflict, "report already handled")
}

// removeContent 按举报目标类型移除内容：post 软删（status='deleted'），
// comment 物理删除。其余类型当前不支持（举报入口只开放这两类）。
func (l *HandleReportLogic) removeContent(targetType string, targetId int64) error {
	switch targetType {
	case "post":
		return l.svcCtx.PostRepo.RemoveByModerator(targetId)
	case "comment":
		return l.svcCtx.CommentRepo.Delete(targetId)
	default:
		return httperr.BadRequest("unsupported report target type")
	}
}
