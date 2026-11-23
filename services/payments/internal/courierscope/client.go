package courierscope

// spawn_game_id → courier scope 解析客户端（GP-202 集成缺口修复）：
// payments 调 Courier 的 X-Courier-Game-Id / X-Courier-Env 头必须是映射后的
// courier_game_id + env（与客户端经 GP-201 取得的 Courier 令牌绑定 scope 一致），
// 绝不能把 spawn 游戏 id 当 Courier scope 发——活走查实测发 spawn id 必
// 502「auth: token scope mismatch」。映射唯一权威在 courier-adapter 的
// courier_game_mapping（接入设计 §2.1），本客户端经其 S2S 端点
// GET /internal/games/:id/mapping 解析（X-Internal-Token，fail-closed）。

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ErrMappingNotFound 游戏未配置 Courier 映射（未接入支付接线）。
var ErrMappingNotFound = errors.New("courier mapping not found")

// Scope 解析结果：调 Courier 时的 X-Courier-Game-Id / X-Courier-Env 值，以及
// GP-002 能力位（login/payment/download/support，消费方 fail-closed 门）。
type Scope struct {
	CourierGameId string
	Env           string
	Login         bool
	Payment       bool
	Download      bool
	Support       bool
}

// Client courier-adapter 内部映射端点客户端。
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: timeout},
	}
}

type mappingResponse struct {
	CourierGameId string `json:"courierGameId"`
	Env           string `json:"env"`
	Capabilities  struct {
		Login    bool `json:"login"`
		Payment  bool `json:"payment"`
		Download bool `json:"download"`
		Support  bool `json:"support"`
	} `json:"capabilities"`
}

// Resolve 按 spawn_game_id 解析 courier scope。404 = 游戏未映射。
func (c *Client) Resolve(spawnGameID string) (*Scope, error) {
	req, err := http.NewRequest(http.MethodGet,
		c.baseURL+"/internal/games/"+url.PathEscape(spawnGameID)+"/mapping", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Token", c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrMappingNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("courier-adapter mapping: HTTP %d: %s", resp.StatusCode, string(raw))
	}
	var m mappingResponse
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("courier-adapter mapping 响应非 JSON: %w", err)
	}
	if m.CourierGameId == "" || m.Env == "" {
		return nil, fmt.Errorf("courier-adapter mapping 响应缺 courierGameId/env: %s", string(raw))
	}
	return &Scope{
		CourierGameId: m.CourierGameId,
		Env:           m.Env,
		Login:         m.Capabilities.Login,
		Payment:       m.Capabilities.Payment,
		Download:      m.Capabilities.Download,
		Support:       m.Capabilities.Support,
	}, nil
}
