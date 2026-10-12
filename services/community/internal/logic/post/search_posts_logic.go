// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package post

import (
	"context"

	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SearchPostsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSearchPostsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SearchPostsLogic {
	return &SearchPostsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// SearchPosts 公开端点：帖子关键词搜索。q 由仓储归一（空白归一为空结果集），
// 这里只做分页钳制（与 GetPosts 同款护栏）。
func (l *SearchPostsLogic) SearchPosts(req *types.SearchPostsReq) (resp *types.PostsResp, err error) {
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

	posts, total := l.svcCtx.PostRepo.SearchPosts(req.Q, limit, offset)
	if posts == nil {
		posts = []types.Post{}
	}
	return &types.PostsResp{Posts: posts, Total: total}, nil
}
