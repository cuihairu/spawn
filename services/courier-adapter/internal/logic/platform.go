package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tappi/tappi/services/courier-adapter/internal/svc"
)

// 平台侧辅助：角色权威在 game-catalog（/me），用户邮箱在 user-service
// （仅本人可见，故透传本人令牌）。适配服务不复制角色/用户数据。

// platformClient 简单 JSON GET 客户端。
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

// catalogClient 按配置构造（无状态，直接 new）。
func catalogClient(svcCtx *svc.ServiceContext) *platformClient {
	return newPlatformClient(svcCtx.Config.GameCatalog.BaseURL, svcCtx.Config.GameCatalog.Timeout)
}

func userClient(svcCtx *svc.ServiceContext) *platformClient {
	return newPlatformClient(svcCtx.Config.UserService.BaseURL, svcCtx.Config.UserService.Timeout)
}

// platformRole 经 game-catalog GET /me 读取平台角色（单一事实源）。
// 返回 (role, partnerId, error)；上游不可达或未认证时 error 非空。
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

// fetchUserEmail 经 user-service 读取本人邮箱（透传本人令牌）。
// 失败/为空 → 空串（调用方落占位邮箱），不阻断绑定主流程。
func fetchUserEmail(ctx context.Context, svcCtx *svc.ServiceContext, authHeader string, userID int64) string {
	url := fmt.Sprintf("%s/users/%d", userClient(svcCtx).base, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := userClient(svcCtx).hc.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ""
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Email string `json:"email"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Code != 200 {
		return ""
	}
	return envelope.Data.Email
}

// placeholderEmail 无平台邮箱时的 Courier 占位（接入设计 §1.3）。
func placeholderEmail(userID int64) string {
	return fmt.Sprintf("spawn_%d@platform.invalid", userID)
}
