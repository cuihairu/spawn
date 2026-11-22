// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package post

import (
	"context"
	"fmt"
	"strings"

	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/moderation"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreatePostLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreatePostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreatePostLogic {
	return &CreatePostLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreatePostLogic) CreatePost(req *types.CreatePostReq) (resp *types.PostResp, err error) {
	userId, username, err := common.UserFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, httperr.BadRequest("request required")
	}
	if req.TopicId <= 0 {
		return nil, httperr.BadRequest("topic_id required")
	}
	if strings.TrimSpace(req.Title) == "" {
		return nil, httperr.BadRequest("title required")
	}
	if strings.TrimSpace(req.Content) == "" {
		return nil, httperr.BadRequest("content required")
	}
	if hit, ok := moderation.FirstHit(l.svcCtx.BlockedWords, req.Title+" "+req.Content); ok {
		return nil, httperr.BadRequest(fmt.Sprintf("content contains blocked word: %q", hit))
	}

	if _, err := l.svcCtx.TopicRepo.Get(req.TopicId); err != nil {
		return nil, err
	}

	// GP-307 官方身份：审核管理员发言 = 官方帖（author_official 落库，
	// 公告入口按此聚合；身份由服务端角色判定，客户端不可自报）。
	p, err := l.svcCtx.PostRepo.Create(req.TopicId, userId, username, req)
	if err != nil {
		return nil, err
	}
	if l.svcCtx.IsAdmin(userId) {
		if err := l.svcCtx.PostRepo.SetAuthorOfficial(p.Id, true); err != nil {
			l.Errorf("mark post %d official failed: %v", p.Id, err)
		}
		p.AuthorOfficial = true
	}
	_ = l.svcCtx.TopicRepo.IncrementPostCount(req.TopicId, 1)

	return &types.PostResp{Post: *p}, nil
}
