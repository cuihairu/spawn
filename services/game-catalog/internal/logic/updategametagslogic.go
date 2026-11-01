package logic

import (
	"context"
	"errors"
	"net/http"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateGameTagsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateGameTagsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateGameTagsLogic {
	return &UpdateGameTagsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// UpdateGameTags 整体覆盖游戏标签（全量替换，幂等）。标签为展示信息，仅作筛选/徽标，
// 不自动开通支付或下载能力（能力由 courier_game_mapping 配置表控制，见 M0 接入设计）。
func (l *UpdateGameTagsLogic) UpdateGameTags(req *types.UpdateGameTagsRequest) (*types.GameDetailResponse, error) {
	if len(req.Tags) > 20 {
		return nil, newAPIError(http.StatusBadRequest, "标签数量不能超过 20 个")
	}
	for _, tag := range req.Tags {
		if len([]rune(tag)) > 32 {
			return nil, newAPIError(http.StatusBadRequest, "单个标签不能超过 32 个字符")
		}
	}

	game, err := l.svcCtx.GameRepository.SetTags(req.Id, req.Tags)
	if err != nil {
		if errors.Is(err, model.ErrGameNotFound) {
			return nil, newAPIError(http.StatusNotFound, "游戏不存在")
		}
		l.Errorf("update game tags failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "更新失败")
	}

	return &types.GameDetailResponse{
		Game: toTypesGame(game),
	}, nil
}
