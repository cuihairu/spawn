package comment

import (
	"context"
	"strings"
	"time"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateCommentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateCommentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateCommentLogic {
	return &CreateCommentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CreateComment 发表帖子评论（登录）：正文去空白后必填；parent_id 给出时
// 必须指向同一帖子下的既有评论，被回复者昵称由服务端冗余落库。
// 计数语义与点赞一致：comment_count 单调累加，无删除回退。
func (l *CreateCommentLogic) CreateComment(req *types.CreateCommentReq) (resp *types.CommentResp, err error) {
	if req == nil || req.Id <= 0 {
		return nil, httperr.BadRequest("post id required")
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, httperr.BadRequest("content required")
	}

	userId, username, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	post, err := l.svcCtx.PostRepo.Get(req.Id)
	if err != nil {
		return nil, err
	}

	replyTo := ""
	parentId := req.ParentId
	if parentId > 0 {
		parent, err := l.svcCtx.CommentRepo.GetByID(parentId)
		if err != nil {
			return nil, err
		}
		if parent.PostId != post.Id {
			return nil, httperr.BadRequest("parent comment belongs to another post")
		}
		replyTo = parent.AuthorName
	}

	now := time.Now().UTC().Format(time.RFC3339)
	comment := &types.Comment{
		PostId:            post.Id,
		AuthorId:          userId,
		AuthorName:        username,
		Content:           content,
		ParentId:          parentId,
		ReplyToAuthorName: replyTo,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	id, err := l.svcCtx.CommentRepo.Create(comment)
	if err != nil {
		return nil, err
	}
	comment.Id = id

	if _, err := l.svcCtx.PostRepo.IncrementComments(post.Id); err != nil {
		// 计数自增失败不回滚评论本体（读侧以 comments 表为准可重算），记日志即可。
		l.Errorf("increment comment count for post %d: %v", post.Id, err)
	}

	return &types.CommentResp{Comment: comment}, nil
}
