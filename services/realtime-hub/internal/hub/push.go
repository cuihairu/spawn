package hub

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// Notifier 离线推送出口（厂商推送桥的接缝）：目标用户本实例零投递时
// 回调，由桥接服务转厂商协议（APNs/FCM 等）。多实例（Relay 模式）下
// 本地零投递 ≠ 离线，分布式推送决策交部署面，暂不启用。
type Notifier interface {
	NotifyOffline(userIds []int64, frame []byte)
}

// PushEnvelope webhook 请求体：frame 内嵌原始推送帧（type + payload），
// 桥接服务自选厂商协议与限流策略。
type PushEnvelope struct {
	UserIds []int64         `json:"user_ids"`
	Frame   json.RawMessage `json:"frame"`
}

// WebhookNotifier 通用 webhook 实现：fire-and-forget POST，失败仅记日志，
// 永不影响投递主链路。
type WebhookNotifier struct {
	url    string
	token  string
	client *http.Client
}

// NewWebhookNotifier token 空则不带 X-Push-Token 头。
func NewWebhookNotifier(url, token string) *WebhookNotifier {
	return &WebhookNotifier{url: url, token: token, client: &http.Client{Timeout: 3 * time.Second}}
}

// NotifyOffline 异步 POST 一次（批量 user_ids 合并单请求）。
func (n *WebhookNotifier) NotifyOffline(userIds []int64, frame []byte) {
	body, err := json.Marshal(PushEnvelope{UserIds: userIds, Frame: frame})
	if err != nil {
		log.Printf("push: marshal envelope: %v", err)
		return
	}
	go func() {
		req, err := http.NewRequest(http.MethodPost, n.url, bytes.NewReader(body))
		if err != nil {
			log.Printf("push: new request: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if n.token != "" {
			req.Header.Set("X-Push-Token", n.token)
		}
		resp, err := n.client.Do(req)
		if err != nil {
			log.Printf("push: webhook %s: %v", n.url, err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= http.StatusMultipleChoices {
			log.Printf("push: webhook status %d", resp.StatusCode)
		}
	}()
}
