package logic

// GP-401 收藏游戏（F05）：收藏/取消幂等，我的收藏回读游戏现状；收藏人群
// 同时是上线通知受众（game_launch，AdminLogic 发布/过审上架过渡时扇出）。

import (
	"context"
	"errors"
	"net/http"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type FavoriteLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewFavoriteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FavoriteLogic {
	return &FavoriteLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *FavoriteLogic) userID() int64 {
	id, _ := l.ctx.Value("user_id").(int64)
	return id
}

// Favorite 收藏游戏（幂等：重复收藏 created=false）。仅已上架游戏可收藏
// （与详情同口径，不泄露未上架游戏的存在性）。
func (l *FavoriteLogic) Favorite(req *types.IdPathRequest) (*types.FavoriteGameResponse, error) {
	userID := l.userID()
	if userID <= 0 {
		return nil, newAPIError(http.StatusUnauthorized, "未登录")
	}
	g, err := l.svcCtx.GameRepository.Get(req.Id)
	if err != nil {
		if errors.Is(err, model.ErrGameNotFound) {
			return nil, newAPIError(http.StatusNotFound, "游戏不存在")
		}
		l.Errorf("query game failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询游戏失败")
	}
	if g.Status != model.GameStatusApproved {
		return nil, newAPIError(http.StatusNotFound, "游戏不存在")
	}
	_, created, err := l.svcCtx.AdminRepository.AddFavorite(req.Id, userID)
	if err != nil {
		l.Errorf("add favorite game %s user %d failed: %v", req.Id, userID, err)
		return nil, newAPIError(http.StatusInternalServerError, "收藏失败")
	}
	return &types.FavoriteGameResponse{GameId: req.Id, Created: created}, nil
}

// Unfavorite 取消收藏（未收藏也成功，幂等）。
func (l *FavoriteLogic) Unfavorite(req *types.IdPathRequest) error {
	userID := l.userID()
	if userID <= 0 {
		return newAPIError(http.StatusUnauthorized, "未登录")
	}
	if err := l.svcCtx.AdminRepository.RemoveFavorite(req.Id, userID); err != nil {
		l.Errorf("remove favorite game %s user %d failed: %v", req.Id, userID, err)
		return newAPIError(http.StatusInternalServerError, "取消收藏失败")
	}
	return nil
}

// MyFavorites 我的收藏（新在前；游戏已物理删除的收藏行跳过不阻断）。
func (l *FavoriteLogic) MyFavorites() (*types.MyFavoritesResponse, error) {
	userID := l.userID()
	if userID <= 0 {
		return nil, newAPIError(http.StatusUnauthorized, "未登录")
	}
	rows, err := l.svcCtx.AdminRepository.ListMyFavorites(userID, 100)
	if err != nil {
		l.Errorf("list favorites for user %d failed: %v", userID, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询收藏失败")
	}
	items := make([]types.FavoriteItemView, 0, len(rows))
	for _, f := range rows {
		g, err := l.svcCtx.GameRepository.Get(f.GameId)
		if err != nil {
			continue // 游戏已删除：跳过（收藏行保留）
		}
		items = append(items, types.FavoriteItemView{
			Game: toTypesGame(g), FavoritedAt: f.CreatedAt,
		})
	}
	return &types.MyFavoritesResponse{Items: items}, nil
}
