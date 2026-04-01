package logic

import (
	"context"
	"errors"
	"net/http"

	"github.com/tappi/tappi/services/content-service/internal/svc"
	"github.com/tappi/tappi/services/content-service/internal/types"
	"github.com/tappi/tappi/services/content-service/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetGuideLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetGuideLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetGuideLogic {
	return &GetGuideLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetGuideLogic) GetGuide(req *types.GetGuideRequest) (resp *types.GetGuideResponse, err error) {
	guide, err := l.svcCtx.GuideRepository.Get(req.Id)
	if err != nil {
		if errors.Is(err, model.ErrGuideNotFound) {
			return &types.GetGuideResponse{
				Code:    http.StatusNotFound,
				Message: "攻略不存在",
			}, nil
		}
		l.Logger.Errorf("get guide failed: %v", err)
		return &types.GetGuideResponse{
			Code:    http.StatusInternalServerError,
			Message: "获取失败",
		}, nil
	}

	// 草稿只允许作者访问（匿名/其他用户视为不存在）
	if !guide.IsPublished {
		userId, _ := l.ctx.Value("user_id").(int64)
		if userId == 0 || userId != guide.AuthorId {
			return &types.GetGuideResponse{
				Code:    http.StatusNotFound,
				Message: "攻略不存在",
			}, nil
		}
	}

	// 增加浏览次数
	_ = l.svcCtx.GuideRepository.IncrementViews(req.Id)

	return &types.GetGuideResponse{
		Code:    http.StatusOK,
		Message: "获取成功",
		Data:    modelGuideToType(guide),
	}, nil
}
