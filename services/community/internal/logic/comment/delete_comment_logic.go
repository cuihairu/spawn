package comment

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteCommentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteCommentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteCommentLogic {
	return &DeleteCommentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// DeleteComment 删除评论（仅作者本人）：路径为帖子子资源
// DELETE /api/v1/posts/:id/comments/:cid，cid 必须属于 id 帖子；
// 与删帖同款 permission denied 语义；comment_count 不回退（与点赞计数
// 同款单调累加契约，读侧以列表为准）。
func (l *DeleteCommentLogic) DeleteComment(req *types.DeleteCommentReq) (resp *types.CommonResp, err error) {
	if req == nil || req.CommentId <= 0 {
		return nil, httperr.BadRequest("comment id required")
	}
	userId, _, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	comment, err := l.svcCtx.CommentRepo.GetByID(req.CommentId)
	if err != nil {
		return nil, err
	}
	if req.Id > 0 && comment.PostId != req.Id {
		return nil, httperr.BadRequest("comment belongs to another post")
	}
	if comment.AuthorId != userId {
		return nil, httperr.Forbidden("permission denied")
	}

	if err := l.svcCtx.CommentRepo.Delete(req.CommentId); err != nil {
		return nil, err
	}
	return &types.CommonResp{Code: 0, Message: "ok"}, nil
}
