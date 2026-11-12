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
	"net/http"
	"strconv"
	"time"

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
	OccurredAt  string `json:"occurred_at,optional"`
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

// UserIDOf 平台账号字符串 → int64（非数字账号返回 0，事件侧按匿名处理
// 不参与留存，但仍计入活动效果笔数与 GMV）。
func UserIDOf(accountId string) int64 {
	id, err := strconv.ParseInt(accountId, 10, 64)
	if err != nil || id < 0 {
		return 0
	}
	return id
}
