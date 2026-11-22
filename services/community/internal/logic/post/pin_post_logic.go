package post

// GP-307 官方公告运营位（F09）：置顶/取消置顶（仅审核管理员）与公告入口
// 聚合（author_official 帖子，置顶优先）。身份判定用 AdminSet（配置预计算）。

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PinPostLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPinPostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PinPostLogic {
	return &PinPostLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// PinPost 置顶/取消置顶（幂等；仅审核管理员）。
func (l *PinPostLogic) PinPost(req *types.PinPostReq) error {
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return err
	}
	if req == nil || req.Id <= 0 {
		return httperr.BadRequest("post id required")
	}
	if !l.svcCtx.IsAdmin(userId) {
		return httperr.Forbidden("admin permission required")
	}
	return l.svcCtx.PostRepo.SetPinned(req.Id, req.Pinned)
}

type ListAnnouncementsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListAnnouncementsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAnnouncementsLogic {
	return &ListAnnouncementsLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// ListAnnouncements 公告入口：官方帖（置顶优先，id 倒序），公开读。
// 空结果规整为空切片非 nil。
func (l *ListAnnouncementsLogic) ListAnnouncements(req *types.AnnouncementsReq) (*types.PostsResp, error) {
	limit := int64(20)
	if req != nil && req.Limit > 0 {
		limit = req.Limit
	}
	posts := l.svcCtx.PostRepo.Announcements(limit)
	if posts == nil {
		posts = []types.Post{}
	}
	return &types.PostsResp{Posts: posts, Total: int64(len(posts))}, nil
}
