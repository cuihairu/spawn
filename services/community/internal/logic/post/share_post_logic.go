// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package post

import (
	"context"

	"community/internal/svc"
	"community/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SharePostLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSharePostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SharePostLogic {
	return &SharePostLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SharePostLogic) SharePost() (resp *types.CommonResp, err error) {
	// todo: add your logic here and delete this line

	return
}
