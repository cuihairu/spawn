// Package analytics 向 data-panel /internal/events 投递平台事实事件
// （GP-402/F24：reward_granted 活动发奖）。event_id 由
// "reward_granted:<order>:<activity>" 派生，评估重放只算一次发奖；
// 金额取订单事实（PaidCents），故活动效果口径 = 该活动发放所对应订单
// 实付合计（活动带动 GMV）。fire-and-forget，投递失败仅记日志，
// 绝不影响发放主流程；BaseURL 未配置 = no-op。
package analytics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/tappi/tappi/services/promotions/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// EventRewardGranted 活动发奖事实（activity_id 为活动效果聚合键）。
const EventRewardGranted = "reward_granted"

// Event 单条事实事件（data-panel IngestAnalyticsReq.events 元素同构）。
type Event struct {
	EventId     string `json:"event_id"`
	EventType   string `json:"event_type"`
	UserId      int64  `json:"user_id,optional"`
	GameId      string `json:"game_id,optional"`
	ActivityId  int64  `json:"activity_id,optional"`
	OrderId     string `json:"order_id,optional"`
	AmountCents int64  `json:"amount_cents,optional"`
	// Currency GP-402 币种（ISO 4217，如 CNY/USD）：跨境多币种 GMV 按此分桶，
	// 空 = data-panel 侧按平台默认币种归并。
	Currency   string `json:"currency,optional"`
	OccurredAt string `json:"occurred_at,optional"`
}

// Client 事实事件投递口（不向调用方返回错误）。
type Client interface {
	Emit(e Event)
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

// UserIDOf 平台账号字符串 → int64（非数字账号返回 0，事件侧按匿名处理
// 不参与留存，但仍计入活动效果笔数与 GMV）。
func UserIDOf(accountId string) int64 {
	id, err := strconv.ParseInt(accountId, 10, 64)
	if err != nil || id < 0 {
		return 0
	}
	return id
}
