package logic

// GP-303 回流判定源 S2S 受理：courier-adapter 上报账号×游戏活跃事实
//（游戏会话签发成功），服务端校验后落表。X-Internal-Token 守卫在 handler
// 层（与 /internal/games/:id/activities 同款 fail-closed）。

import (
	"net/http"
	"strings"
	"time"

	"github.com/tappi/tappi/services/promotions/internal/svc"
	"github.com/tappi/tappi/services/promotions/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// RecordPlayerActivity 校验并落一条活跃事实。
func RecordPlayerActivity(svcCtx *svc.ServiceContext, l logx.Logger, req *types.InternalPlayerActivityRequest) error {
	accountID := strings.TrimSpace(req.AccountId)
	gameID := strings.TrimSpace(req.GameId)
	if accountID == "" {
		return newAPIError(http.StatusBadRequest, "账号标识不能为空")
	}
	if gameID == "" {
		return newAPIError(http.StatusBadRequest, "游戏归属不能为空")
	}
	activeAt := strings.TrimSpace(req.ActiveAt)
	if activeAt == "" {
		activeAt = time.Now().UTC().Format(time.RFC3339Nano)
	} else if t, err := time.Parse(time.RFC3339, activeAt); err != nil {
		return newAPIError(http.StatusBadRequest, "活跃时刻格式须为 RFC3339")
	} else {
		// 规范为 UTC RFC3339Nano（存储定长同格式，字符串比较即时间比较）
		activeAt = t.UTC().Format(time.RFC3339Nano)
	}
	if err := svcCtx.ActivityPlayer.RecordActivity(accountID, gameID, activeAt); err != nil {
		l.Errorf("record player activity account=%s game=%s failed: %v", accountID, gameID, err)
		return newAPIError(http.StatusInternalServerError, "记录活跃事实失败")
	}
	return nil
}
