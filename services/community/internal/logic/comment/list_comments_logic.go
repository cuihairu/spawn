package comment

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListCommentsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListCommentsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListCommentsLogic {
	return &ListCommentsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListComments 帖子评论列表（公开，与帖子详情同一匿名语义）：id 升序、
// offset 分页钳制（offset<0→0、limit<=0→50、limit>200→200，与帖子列表同口径）。
func (l *ListCommentsLogic) ListComments(req *types.ListCommentsReq) (resp *types.CommentsResp, err error) {
	if req == nil || req.Id <= 0 {
		return nil, httperr.BadRequest("post id required")
	}
	if _, err := l.svcCtx.PostRepo.Get(req.Id); err != nil {
		return nil, err
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	comments, err := l.svcCtx.CommentRepo.ListByPost(req.Id, limit, offset)
	if err != nil {
		return nil, err
	}
	total, err := l.svcCtx.CommentRepo.CountByPost(req.Id)
	if err != nil {
		return nil, err
	}
	return &types.CommentsResp{Comments: comments, Total: total}, nil
}
