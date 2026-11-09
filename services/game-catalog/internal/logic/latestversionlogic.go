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

type LatestVersionLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLatestVersionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LatestVersionLogic {
	return &LatestVersionLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Latest GP-105 公开读取游戏当前已发布版本元数据（受控下载入口）：
// 客户端先取 latest 再决定「平台下载（downloadable）」或「商店链接承接（F08）」。
// storage_url 是内部托管细节，公开视图一律置空。
func (l *LatestVersionLogic) Latest(gameId string) (*types.LatestVersionResponse, error) {
	if _, err := l.svcCtx.GameRepository.Get(gameId); err != nil {
		if errors.Is(err, model.ErrGameNotFound) {
			return nil, newAPIError(http.StatusNotFound, "游戏不存在")
		}
		l.Errorf("get game failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询游戏失败")
	}
	v, err := l.svcCtx.AdminRepository.GetLatestPublishedVersion(gameId)
	if errors.Is(err, model.ErrVersionNotFound) {
		return nil, newAPIError(http.StatusNotFound, "暂无已发布版本")
	}
	if err != nil {
		l.Errorf("get latest published version failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询版本失败")
	}
	out := toVersion(v)
	out.StorageUrl = ""
	return &types.LatestVersionResponse{Version: *out}, nil
}
