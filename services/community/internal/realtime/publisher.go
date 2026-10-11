// Package realtime 向 realtime-hub /internal/publish 投递实时事件。
// Url 为空时为 no-op：未部署 realtime-hub 的环境零开销、零行为差异。
package realtime

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// Publisher 事件投递口。实现必须不阻塞、不向调用方返回错误：
// 投递失败仅记日志，绝不影响主操作（与通知落库同款契约）。
type Publisher interface {
	Publish(eventType string, userIds []int64, payload any)
}

type publisher struct {
	url    string // 完整发布端点；空 = no-op
	token  string
	client *http.Client
}

// NewPublisher 构建发布器；url 空返回 no-op 实现。
func NewPublisher(url, token string) Publisher {
	if url == "" {
		return noop{}
	}
	return &publisher{
		url:    url + "/internal/publish",
		token:  token,
		client: &http.Client{Timeout: 3 * time.Second},
	}
}

type publishBody struct {
	UserIds []int64    `json:"user_ids"`
	Event   publishEvt `json:"event"`
}

type publishEvt struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// Publish fire-and-forget：网络投递放独立 goroutine（3s 超时），
// 序列化失败同步返回（非法事件类型属调用方 bug，日志即暴露）。
func (p *publisher) Publish(eventType string, userIds []int64, payload any) {
	body, err := json.Marshal(publishBody{UserIds: userIds, Event: publishEvt{Type: eventType, Payload: payload}})
	if err != nil {
		logx.Errorf("realtime publish %s: marshal: %v", eventType, err)
		return
	}
	go func() {
		req, err := http.NewRequest(http.MethodPost, p.url, bytes.NewReader(body))
		if err != nil {
			logx.Errorf("realtime publish %s: %v", eventType, err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Internal-Token", p.token)
		resp, err := p.client.Do(req)
		if err != nil {
			logx.Errorf("realtime publish %s: %v", eventType, err)
			return
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			logx.Errorf("realtime publish %s: hub returned %d", eventType, resp.StatusCode)
		}
	}()
}

type noop struct{}

func (noop) Publish(string, []int64, any) {}
