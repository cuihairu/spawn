// Package analytics 向 data-panel /internal/events 投递平台事实事件
// （GP-402/F24：register 注册、login 活跃）。契约与 game-catalog
// notify.Publisher 同款：fire-and-forget，投递失败仅记日志；BaseURL 未
// 配置 = no-op（数据面板未部署的环境零开销、零行为差异）。
package analytics

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// 事件类型口径（与 data-panel platform_events 对齐）。
const (
	EventRegister = "register"
	EventLogin    = "login"
)

// Event 单条事实事件（data-panel IngestAnalyticsReq.events 元素同构）。
type Event struct {
	EventId     string `json:"event_id"`
	EventType   string `json:"event_type"`
	UserId      int64  `json:"user_id,optional"`
	GameId      string `json:"game_id,optional"`
	ActivityId  int64  `json:"activity_id,optional"`
	OrderId     string `json:"order_id,optional"`
	AmountCents int64  `json:"amount_cents,optional"`
	// Channel GP-403 渠道标识（注册时由推广链接归因得到，空=自然流量）。
	Channel    string `json:"channel,optional"`
	OccurredAt string `json:"occurred_at,optional"`
}

// Client 事实事件投递口（不向调用方返回错误）。
type Client interface {
	Emit(e Event)
	// EmitAsync fire-and-forget：登录等热路径用，data-panel 慢/挂时不拖
	// 慢登录响应（超时只影响本 goroutine）。
	EmitAsync(e Event)
	// Enabled 供触发路径跳过事实拼装。
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

// EmitAsync 异步投递（登录热路径）：event 为值拷贝，调用方可继续复用字段。
func (c *client) EmitAsync(e Event) {
	go c.Emit(e)
}

func (c *client) Enabled() bool { return true }

type noop struct{}

func (noop) Emit(Event)      {}
func (noop) EmitAsync(Event) {}
func (noop) Enabled() bool   { return false }
