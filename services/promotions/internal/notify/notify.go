// Package notify GP-306 活动消息触发客户端：活动发布时向游戏预约人群扇出
// 通知。受众来自 game-catalog /internal/games/:id/reservation-users（预约
// 体系权威在区服域），投递走 community /internal/notifications（站内信 +
// realtime-hub 远程推送在 community 侧）。任一 URL 为空时对应能力关闭：
// 未部署依赖服务的环境零开销、零行为差异。
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// Notification 单条业务通知请求（fan-out 由 community 侧完成）。
type Notification struct {
	UserIds   []int64 `json:"user_ids"`
	Type      string  `json:"type"`
	TargetId  int64   `json:"target_id"`
	Content   string  `json:"content"`
	ActorName string  `json:"actor_name,omitempty"`
}

// Client 活动通知客户端；零值安全（URL 空 = 能力关闭）。
type Client struct {
	communityURL string // community 基址；空 = 不投递
	catalogURL   string // game-catalog 基址；空 = 不拉受众
	token        string
	client       *http.Client
}

func New(communityURL, catalogURL, token string) *Client {
	return &Client{
		communityURL: communityURL,
		catalogURL:   catalogURL,
		token:        token,
		client:       &http.Client{Timeout: 3 * time.Second},
	}
}

// Enabled 是否配置了投递通道（调用方据此跳过昂贵的受众查询）。
func (c *Client) Enabled() bool { return c != nil && c.communityURL != "" }

// Publish 投递通知（3s 超时，失败仅记日志——通知绝不影响活动发布结果）。
func (c *Client) Publish(n Notification) {
	if !c.Enabled() {
		return
	}
	body, err := json.Marshal(n)
	if err != nil {
		logx.Errorf("notify %s: marshal: %v", n.Type, err)
		return
	}
	req, err := http.NewRequest(http.MethodPost, c.communityURL+"/internal/notifications", bytes.NewReader(body))
	if err != nil {
		logx.Errorf("notify %s: %v", n.Type, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		logx.Errorf("notify %s: %v", n.Type, err)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logx.Errorf("notify %s: community returned %d", n.Type, resp.StatusCode)
	}
}

// ReservationUsers 拉游戏维度去重预约人群（受众）；未配置 catalog 或查询
// 失败返回错误，由调用方决定降级（记日志跳过）。
func (c *Client) ReservationUsers(gameID string) ([]int64, error) {
	if c == nil || c.catalogURL == "" {
		return nil, fmt.Errorf("game-catalog not configured")
	}
	req, err := http.NewRequest(http.MethodGet,
		c.catalogURL+"/internal/games/"+gameID+"/reservation-users", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Token", c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog returned %d", resp.StatusCode)
	}
	var body struct {
		GameId  string  `json:"game_id"`
		UserIds []int64 `json:"user_ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.UserIds, nil
}
