package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tappi/tappi/services/payments/internal/svc"
)

// 平台侧辅助：角色权威在 game-catalog（/me），适配服务不复制角色数据。

type platformClient struct {
	base string
	hc   *http.Client
}

func newPlatformClient(base string, timeoutMs int64) *platformClient {
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	return &platformClient{base: strings.TrimRight(base, "/"), hc: &http.Client{Timeout: time.Millisecond * time.Duration(timeoutMs)}}
}

func catalogClient(svcCtx *svc.ServiceContext) *platformClient {
	return newPlatformClient(svcCtx.Config.GameCatalog.BaseURL, svcCtx.Config.GameCatalog.Timeout)
}

// platformRole 经 game-catalog GET /me 读取平台角色（单一事实源）。
func platformRole(ctx context.Context, svcCtx *svc.ServiceContext, authHeader string) (string, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, catalogClient(svcCtx).base+"/me", nil)
	if err != nil {
		return "", 0, err
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := catalogClient(svcCtx).hc.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("game-catalog unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("game-catalog /me status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", 0, err
	}
	var me struct {
		Role      string `json:"role"`
		PartnerId int64  `json:"partnerId"`
	}
	if err := json.Unmarshal(raw, &me); err != nil {
		return "", 0, fmt.Errorf("decode /me: %w", err)
	}
	return me.Role, me.PartnerId, nil
}

// requireAdminRole 管理面角色守卫：platform_admin/operator 放行。
func requireAdminRole(ctx context.Context, svcCtx *svc.ServiceContext, authHeader string) error {
	role, _, err := platformRole(ctx, svcCtx, authHeader)
	if err != nil {
		return newAPIError(http.StatusBadGateway, "角色服务不可用")
	}
	if role != "platform_admin" && role != "operator" {
		return newAPIError(http.StatusForbidden, "无权限执行该操作")
	}
	return nil
}

// catalogServerOpen 校验充值目标：serverId 必须出现在 game-catalog 该游戏的
// 公开区服列表（公开列表只对已上架游戏开放且隐藏 closed，见 GP-201 口径），
// 且状态为 open（maintenance/closed 不可充值）。目录不可达时 fail-closed
// ——无法核验充值目标就不该让可发货订单成立。
func catalogServerOpen(ctx context.Context, svcCtx *svc.ServiceContext, gameID string, serverID int64) (bool, error) {
	c := catalogClient(svcCtx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/games/"+url.PathEscape(gameID)+"/servers", nil)
	if err != nil {
		return false, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return false, fmt.Errorf("game-catalog unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("game-catalog servers status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, err
	}
	var list struct {
		Items []struct {
			Id     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return false, fmt.Errorf("decode servers: %w", err)
	}
	for _, s := range list.Items {
		if s.Id == serverID {
			return s.Status == "open", nil
		}
	}
	return false, nil
}
