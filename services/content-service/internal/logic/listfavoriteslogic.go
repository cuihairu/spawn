package logic

import (
	"context"
	"net/http"

	"github.com/tappi/tappi/services/content-service/internal/svc"
	"github.com/tappi/tappi/services/content-service/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListFavoritesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListFavoritesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListFavoritesLogic {
	return &ListFavoritesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListFavorites 列出当前登录用户收藏的攻略（按收藏时间新→旧）。
func (l *ListFavoritesLogic) ListFavorites(req *types.ListFavoritesRequest) (resp *types.ListGuidesResponse, err error) {
	userId, ok := l.ctx.Value("user_id").(int64)
	if !ok || userId <= 0 {
		return &types.ListGuidesResponse{
			Code:    http.StatusUnauthorized,
			Message: "请先登录",
		}, nil
	}

	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 {
		req.PageSize = 20
	}

	guides, total, err := l.svcCtx.FavoriteRepository.ListByUser(userId, req.PageSize, (req.Page-1)*req.PageSize)
	if err != nil {
		l.Logger.Errorf("list favorites failed: %v", err)
		return &types.ListGuidesResponse{
			Code:    http.StatusInternalServerError,
			Message: "获取收藏列表失败",
		}, nil
	}

	return &types.ListGuidesResponse{
		Code:    http.StatusOK,
		Message: "获取成功",
		Data:    modelGuidesToTypes(guides),
		Total:   total,
		Page:    req.Page,
	}, nil
}
