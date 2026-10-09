package logic

import (
	"context"
	"net/http"

	"github.com/tappi/tappi/services/content-service/internal/svc"
	"github.com/tappi/tappi/services/content-service/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetFavoriteStatusLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetFavoriteStatusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFavoriteStatusLogic {
	return &GetFavoriteStatusLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetFavoriteStatus 查询当前用户对攻略的收藏状态。匿名访问只返回收藏总数
// （favorited=false），登录后（AuthMiddleware 可选令牌）才给出个人状态。
func (l *GetFavoriteStatusLogic) GetFavoriteStatus(req *types.FavoriteGuideRequest) (resp *types.FavoriteGuideResponse, err error) {
	count, err := l.svcCtx.FavoriteRepository.CountByGuide(req.Id)
	if err != nil {
		l.Logger.Errorf("count favorites failed: %v", err)
		return &types.FavoriteGuideResponse{
			Code:    http.StatusInternalServerError,
			Message: "查询收藏状态失败",
		}, nil
	}

	favorited := false
	userId, ok := l.ctx.Value("user_id").(int64)
	if ok && userId > 0 {
		exists, err := l.svcCtx.FavoriteRepository.Exists(userId, req.Id)
		if err != nil {
			l.Logger.Errorf("check favorite failed: %v", err)
			return &types.FavoriteGuideResponse{
				Code:    http.StatusInternalServerError,
				Message: "查询收藏状态失败",
			}, nil
		}
		favorited = exists
	}

	return &types.FavoriteGuideResponse{
		Code:      http.StatusOK,
		Message:   "获取成功",
		Favorited: favorited,
		Count:     count,
	}, nil
}
