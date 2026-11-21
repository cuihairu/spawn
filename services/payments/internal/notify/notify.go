// Package notify 向 community /internal/notifications 投递站内通知
// （GP-404 结算差异异议复核结果回流合作方）。与 game-catalog notify 同款
// 契约：不向调用方返回错误，投递失败仅记日志；BaseURL 未配置 = no-op。
package notify

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// Notification 单条业务通知（fan-out 由 community 侧完成）。
type Notification struct {
	UserIds   []int64 `json:"user_ids"`
	Type      string  `json:"type"`
	TargetId  int64   `json:"target_id"`
	Content   string  `json:"content"`
	ActorName string  `json:"actor_name,omitempty"`
}

// Client 通知投递口。
type Client interface {
	Publish(n Notification)
	// Enabled 供触发路径跳过昂贵受众查询。
	Enabled() bool
}

type client struct {
	url    string
	token  string
	client *http.Client
}

// NewClient 构建投递客户端；baseURL 空返回 no-op（community 未部署零开销）。
func NewClient(baseURL, token string, timeoutMs int64) Client {
	if baseURL == "" {
		return noop{}
	}
	return &client{
		url:    baseURL + "/internal/notifications",
		token:  token,
		client: &http.Client{Timeout: time.Millisecond * time.Duration(timeoutMs)},
	}
}

// Publish 同步投递（超时级）：失败仅记日志，绝不影响主流程。
func (c *client) Publish(n Notification) {
	body, err := json.Marshal(n)
	if err != nil {
		logx.Errorf("notify %s: marshal: %v", n.Type, err)
		return
	}
	req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(body))
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

func (c *client) Enabled() bool { return true }

type noop struct{}

func (noop) Publish(Notification) {}
func (noop) Enabled() bool        { return false }
