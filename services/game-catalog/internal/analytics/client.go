// Package analytics 向 data-panel /internal/events 投递平台事实事件
// （GP-402/F24：download 包体下载）。与 notify.Publisher 同款契约：
// fire-and-forget，投递失败仅记日志，绝不影响主操作；BaseURL 未配置 =
// no-op（数据面板未部署的环境零开销、零行为差异）。
package analytics

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// EventDownload 包体下载事实（漏斗阶段：注册→活跃→下载→付费）。
const EventDownload = "download"

// Event 单条事实事件（data-panel IngestAnalyticsReq.events 元素同构）。
type Event struct {
	EventId     string `json:"event_id"`
	EventType   string `json:"event_type"`
	UserId      int64  `json:"user_id,optional"`
	GameId      string `json:"game_id,optional"`
	ActivityId  int64  `json:"activity_id,optional"`
	OrderId     string `json:"order_id,optional"`
	AmountCents int64  `json:"amount_cents,optional"`
	// Channel GP-403 渠道标识（单层，如 my-app.com 的渠道 code）。
	Channel    string `json:"channel,optional"`
	OccurredAt string `json:"occurred_at,optional"`
}

// Client 事实事件投递口（不向调用方返回错误）。
type Client interface {
	Emit(e Event)
	// Enabled 供触发路径跳过昂贵的事实拼装。
	Enabled() bool
}

type client struct {
	url    string
	token  string
	client *http.Client
}

// NewClient 构建投递客户端；baseURL 空返回 no-op。
func NewClient(baseURL, token string, timeoutMs int64) Client {
	if baseURL == "" {
		return noop{}
	}
	return &client{
		url:    baseURL + "/internal/events",
		token:  token,
		client: &http.Client{Timeout: time.Millisecond * time.Duration(timeoutMs)},
	}
}

// Emit 同步投递：失败仅记日志。
func (c *client) Emit(e Event) {
	body, err := json.Marshal(struct {
		Events []Event `json:"events"`
	}{Events: []Event{e}})
	if err != nil {
		logx.Errorf("analytics emit %s: marshal: %v", e.EventType, err)
		return
	}
	req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		logx.Errorf("analytics emit %s: %v", e.EventType, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		logx.Errorf("analytics emit %s: %v", e.EventType, err)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logx.Errorf("analytics emit %s: data-panel returned %d", e.EventType, resp.StatusCode)
	}
}

func (c *client) Enabled() bool { return true }

type noop struct{}

func (noop) Emit(Event)    {}
func (noop) Enabled() bool { return false }
