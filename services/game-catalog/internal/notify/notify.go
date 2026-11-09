// Package notify 向 community /internal/notifications 投递业务通知（GP-306：
// 开服提醒/版本通知）。community 侧扇出落库并经 realtime-hub 远程推送。
// Url 为空时为 no-op：community 未部署的环境零开销、零行为差异。
package notify

import (
	"bytes"
	"encoding/json"
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

// Publisher 通知投递口。实现不向调用方返回错误：通知失败仅记日志，
// 绝不影响主操作（与 community 落库→WS 推送同款契约）。
type Publisher interface {
	Publish(n Notification)
	// Enabled 供调用方跳过昂贵的受众查询（如跨服务拉预约人群）。
	Enabled() bool
}

type publisher struct {
	url    string // community 基址；空 = no-op
	token  string
	client *http.Client
}

// NewPublisher 构建发布器；url 空返回 no-op 实现。
func NewPublisher(url, token string) Publisher {
	if url == "" {
		return noop{}
	}
	return &publisher{
		url:    url + "/internal/notifications",
		token:  token,
		client: &http.Client{Timeout: 3 * time.Second},
	}
}

// Publish 同步投递（3s 超时）：失败仅记日志。同步是为了触发路径可验收
// （测试断言 community 侧收到扇出）；3s 上限保证不拖慢主操作。
func (p *publisher) Publish(n Notification) {
	body, err := json.Marshal(n)
	if err != nil {
		logx.Errorf("notify %s: marshal: %v", n.Type, err)
		return
	}
	req, err := http.NewRequest(http.MethodPost, p.url, bytes.NewReader(body))
	if err != nil {
		logx.Errorf("notify %s: %v", n.Type, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", p.token)
	resp, err := p.client.Do(req)
	if err != nil {
		logx.Errorf("notify %s: %v", n.Type, err)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logx.Errorf("notify %s: community returned %d", n.Type, resp.StatusCode)
	}
}

func (p *publisher) Enabled() bool { return true }

type noop struct{}

func (noop) Publish(Notification) {}
func (noop) Enabled() bool        { return false }
