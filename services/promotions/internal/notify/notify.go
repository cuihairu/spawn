// Package notify GP-306 活动消息触发客户端：活动发布时向游戏预约人群扇出
// 通知。受众来自 game-catalog /internal/games/:id/reservation-users（预约
// 体系权威在区服域），投递走 community /internal/notifications（站内信 +
// realtime-hub 远程推送在 community 侧）。任一 URL 为空时对应能力关闭：
// 未部署依赖服务的环境零开销、零行为差异。
//
// 本地补偿（outbox）：先落队列再同步首投，失败留 pending 由 Relay 重投
// （同 game-catalog，重投安全由 community 侧 UNIQUE(user,actor,type,target)
// 去重保证）。
package notify

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tappi/tappi/services/promotions/model"

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
	// outbox 非空 = 投递失败先落 outbox 待 Relay 重投（本地补偿）。重投安全
	// 性由 community 侧 UNIQUE(user_id, actor_id, type, target_id) 去重保证。
	outbox *model.NotifyOutboxModel
}

func New(communityURL, catalogURL, token string, outbox *model.NotifyOutboxModel) *Client {
	return &Client{
		communityURL: communityURL,
		catalogURL:   catalogURL,
		token:        token,
		client:       &http.Client{Timeout: 3 * time.Second},
		outbox:       outbox,
	}
}

// Enabled 是否配置了投递通道（调用方据此跳过昂贵的受众查询）。
func (c *Client) Enabled() bool { return c != nil && c.communityURL != "" }

// Publish 投递通知（3s 超时，失败仅记日志——通知绝不影响活动发布结果）。
// 先落 outbox（幂等，写失败仅记日志仍尝试直投），再同步首投；成功标记
// delivered（Relay 不再重投），失败留 pending 待 Relay 重投。
func (c *Client) Publish(n Notification) {
	if !c.Enabled() {
		return
	}
	body, err := json.Marshal(n)
	if err != nil {
		logx.Errorf("notify %s: marshal: %v", n.Type, err)
		return
	}
	key := notifyKey(n.Type)
	if c.outbox != nil {
		if err := c.outbox.Enqueue(key, n.Type, string(body), nowUTC()); err != nil {
			logx.Errorf("notify %s: enqueue outbox: %v", n.Type, err)
		}
	}
	if err := c.post(body); err != nil {
		logx.Errorf("notify %s: %v", n.Type, err)
		return
	}
	if c.outbox != nil {
		if err := c.outbox.MarkDelivered(key, nowUTC()); err != nil {
			logx.Errorf("notify %s: mark delivered: %v", n.Type, err)
		}
	}
}

// post 低层投递（2xx 视为受理；community 侧按 (user,actor,type,target) 去重）。
func (c *Client) post(body []byte) error {
	req, err := http.NewRequest(http.MethodPost, c.communityURL+"/internal/notifications", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("community returned %d", resp.StatusCode)
	}
	return nil
}

// notifyKey 生成 outbox 行唯一键（随机事实 ID，与 game-catalog 通知同款：通知
// 可重复触发，每次 Publish 为独立逻辑事件，重放依赖 community 侧去重而非派生键）。
func notifyKey(notifyType string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "notify:" + notifyType + ":" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return "notify:" + hex.EncodeToString(buf)
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
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
