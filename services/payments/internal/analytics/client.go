// Package analytics 向 data-panel /internal/events 投递平台事实事件
// （GP-402/F24：order_paid 支付成功、refund_requested 退款请求）。
// 事件金额/订单一律取自订单投影（交易事实），客户端/外部渠道不可上报
// 金额。fire-and-forget：失败先落本地 outbox（at-least-once），由 Relay
// 定时重投；data-panel 侧 UNIQUE(event_id) 摄入幂等，重投不重复计事实。
// 经营分析绝不阻塞支付主流程；BaseURL 未配置 = no-op。
package analytics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/tappi/tappi/services/payments/model"

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
	// Currency GP-402 币种（ISO 4217，如 CNY/USD）：跨境多币种 GMV 按此分桶，
	// 空 = data-panel 侧按平台默认币种归并。
	Currency   string `json:"currency,optional"`
	OccurredAt string `json:"occurred_at,optional"`
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
	// outbox 非空 = 投递失败先落 outbox 待 Relay 重投（本地补偿）
	outbox *model.AnalyticsOutboxModel
}

// NewClient 构建投递客户端；baseURL 空返回 no-op（数据面板未部署时零开销）。
// outbox 非空时启用本地补偿：先落队列再同步首投，失败留 pending 由 Relay
// 重投（节奏与退避在 Relay 侧，队列首行立即可扫）。
func NewClient(baseURL, token string, timeoutMs int64, outbox *model.AnalyticsOutboxModel) Client {
	if baseURL == "" {
		return noop{}
	}
	return &client{
		url:    baseURL + "/internal/events",
		token:  token,
		client: &http.Client{Timeout: time.Millisecond * time.Duration(timeoutMs)},
		outbox: outbox,
	}
}

// Emit 同步投递（3s 级超时）：先落 outbox（幂等，写失败仅记日志仍尝试直投），
// 再同步首投；成功标记 delivered（Relay 不再重投），失败留 pending 待 Relay
// 重投。同步是为了触发路径可验收（测试断言 data-panel 侧收到事实）。
func (c *client) Emit(e Event) {
	body, err := json.Marshal(struct {
		Events []Event `json:"events"`
	}{Events: []Event{e}})
	if err != nil {
		logx.Errorf("analytics emit %s: marshal: %v", e.EventType, err)
		return
	}
	if c.outbox != nil {
		if err := c.outbox.Enqueue(e.EventId, e.EventType, string(body), nowUTC()); err != nil {
			logx.Errorf("analytics emit %s: enqueue outbox: %v", e.EventType, err)
		}
	}
	if err := c.post(body); err != nil {
		logx.Errorf("analytics emit %s: %v", e.EventType, err)
		return
	}
	if c.outbox != nil {
		if err := c.outbox.MarkDelivered(e.EventId, nowUTC()); err != nil {
			logx.Errorf("analytics emit %s: mark delivered: %v", e.EventType, err)
		}
	}
}

// post 低层投递（2xx 视为受理；data-panel 摄入端按 event_id 幂等）。
func (c *client) post(body []byte) error {
	req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("data-panel returned %d", resp.StatusCode)
	}
	return nil
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
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
