// Package analytics 向 data-panel /internal/events 投递平台事实事件
// （GP-402/F24：order_paid 支付成功、refund_requested 退款请求）。
// 事件金额/订单一律取自订单投影（交易事实），客户端/外部渠道不可上报
// 金额。fire-and-forget：投递失败仅记日志（经营分析为容忍丢失的数据面，
// 可由对账查询发现缺口），绝不影响支付主流程；BaseURL 未配置 = no-op。
package analytics

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// 事件类型口径（与 data-panel platform_events 对齐）：
const (
	EventOrderPaid       = "order_paid"
	EventRefundRequested = "refund_requested"
)

// Event 单条事实事件（data-panel IngestAnalyticsReq.events 元素同构）。
type Event struct {
	EventId     string `json:"event_id"`
	EventType   string `json:"event_type"`
	UserId      int64  `json:"user_id,optional"`
	GameId      string `json:"game_id,optional"`
	PartnerId   int64  `json:"partner_id,optional"` // GP-404 合作方归属（数据面板行级隔离）
	ActivityId  int64  `json:"activity_id,optional"`
	OrderId     string `json:"order_id,optional"`
	AmountCents int64  `json:"amount_cents,optional"`
	OccurredAt  string `json:"occurred_at,optional"`
}

// Client 事实事件投递口（与 game-catalog notify.Publisher 同款契约：
// 不向调用方返回错误）。
type Client interface {
	Emit(e Event)
	// Enabled 供触发路径跳过事实拼装（跨服务拉投影等昂贵操作）。
	Enabled() bool
}

type client struct {
	url    string
	token  string
	client *http.Client
}

// NewClient 构建投递客户端；baseURL 空返回 no-op（数据面板未部署时零开销）。
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

// Emit 同步投递（3s 级超时）：失败仅记日志。同步是为了触发路径可验收
// （测试断言 data-panel 侧收到事实）。
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

// UserIDOf 平台账号字符串 → int64（投影 AccountId 为字符串形态；
// 非数字账号（渠道直连账号等）返回 0，事件侧按匿名处理不参与留存）。
func UserIDOf(accountId string) int64 {
	id, err := strconv.ParseInt(accountId, 10, 64)
	if err != nil || id < 0 {
		return 0
	}
	return id
}
