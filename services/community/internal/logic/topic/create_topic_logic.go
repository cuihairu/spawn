// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package topic

import (
	"context"

	"community/internal/svc"
	"community/internal/types"

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
	// todo: add your logic here and delete this line

	return
}
