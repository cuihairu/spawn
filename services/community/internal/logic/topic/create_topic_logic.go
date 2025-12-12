// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package topic

import (
	"context"
	"errors"
	"strings"

	"github.com/tappi/tappi/services/community/internal/logic/common"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateTopicLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateTopicLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateTopicLogic {
	return &CreateTopicLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateTopicLogic) CreateTopic(req *types.CreateTopicReq) (resp *types.TopicResp, err error) {
	if _, _, err := common.UserFromContext(l.ctx); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, errors.New("request required")
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("name required")
	}

	t, err := l.svcCtx.TopicRepo.Create(req)
	if err != nil {
		return nil, err
	}
	return &types.TopicResp{Topic: *t}, nil
}
