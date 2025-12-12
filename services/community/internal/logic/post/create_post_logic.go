// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package post

import (
	"context"
	"errors"
	"strings"

	"github.com/tappi/tappi/services/community/internal/logic/common"
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
		return nil, errors.New("request required")
	}
	if req.TopicId <= 0 {
		return nil, errors.New("topic_id required")
	}
	if strings.TrimSpace(req.Title) == "" {
		return nil, errors.New("title required")
	}
	if strings.TrimSpace(req.Content) == "" {
		return nil, errors.New("content required")
	}

	if _, err := l.svcCtx.TopicRepo.Get(req.TopicId); err != nil {
		return nil, err
	}

	p, err := l.svcCtx.PostRepo.Create(req.TopicId, userId, username, req)
	if err != nil {
		return nil, err
	}
	_ = l.svcCtx.TopicRepo.IncrementPostCount(req.TopicId, 1)

	return &types.PostResp{Post: *p}, nil
}
