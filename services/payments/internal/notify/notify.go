// Package notify 向 community /internal/notifications 投递站内通知
// （GP-404 结算差异异议复核结果回流合作方）。与 game-catalog notify 同款
// 契约：不向调用方返回错误，投递失败仅记日志；BaseURL 未配置 = no-op。
// 本地补偿（outbox）：先落队列再同步首投，失败留 pending 由 Relay 重投
// （同 game-catalog，重投安全由 community 侧 UNIQUE(user,actor,type,target)
// 去重保证）。
package notify

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tappi/tappi/services/payments/model"

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
	// outbox 非空 = 投递失败先落 outbox 待 Relay 重投（本地补偿）。重投安全
	// 性由 community 侧 UNIQUE(user_id, actor_id, type, target_id) 去重保证。
	outbox *model.NotifyOutboxModel
}

// NewClient 构建投递客户端；baseURL 空返回 no-op（community 未部署零开销）。
// outbox 非空时启用本地补偿：先落队列再同步首投，失败留 pending 由 Relay
// 重投（节奏与退避在 Relay 侧，队列首行立即可扫）。
func NewClient(baseURL, token string, timeoutMs int64, outbox *model.NotifyOutboxModel) Client {
	if baseURL == "" {
		return noop{}
	}
	return &client{
		url:    baseURL + "/internal/notifications",
		token:  token,
		client: &http.Client{Timeout: time.Millisecond * time.Duration(timeoutMs)},
		outbox: outbox,
	}
}

// Publish 同步投递（超时级）：先落 outbox（幂等，写失败仅记日志仍尝试直投），
// 再同步首投；成功标记 delivered（Relay 不再重投），失败留 pending 待 Relay
// 重投。同步是为了触发路径可验收；超时上限保证不拖慢主操作。
func (c *client) Publish(n Notification) {
	body, err := json.Marshal(n)
	if err != nil {
		logx.Errorf("notify %s: marshal: %v", n.Type, err)
		return
	}
	key := notifyKey(n.Type)
	if c.outbox != nil {
		if err := c.outbox.Enqueue(key, n.Type, string(body), nowUTC()); err != nil {
			logx.Errorf("notify %s: enqueue outbox: %v", n.Type, err)
		}
	}
	if err := c.post(body); err != nil {
		logx.Errorf("notify %s: %v", n.Type, err)
		return
	}
	if c.outbox != nil {
		if err := c.outbox.MarkDelivered(key, nowUTC()); err != nil {
			logx.Errorf("notify %s: mark delivered: %v", n.Type, err)
		}
	}
}

// post 低层投递（2xx 视为受理；community 侧按 (user,actor,type,target) 去重）。
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
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("community returned %d", resp.StatusCode)
	}
	return nil
}

// notifyKey 生成 outbox 行唯一键（随机事实 ID，与 game-catalog 通知同款：通知
// 可重复触发，每次 Publish 为独立逻辑事件，重放依赖 community 侧去重而非派生键）。
func notifyKey(notifyType string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "notify:" + notifyType + ":" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return "notify:" + hex.EncodeToString(buf)
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func (c *client) Enabled() bool { return true }

type noop struct{}

func (noop) Publish(Notification) {}
func (noop) Enabled() bool        { return false }
