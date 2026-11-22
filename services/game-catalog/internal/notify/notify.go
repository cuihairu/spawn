// Package notify 向 community /internal/notifications 投递业务通知（GP-306：
// 开服提醒/版本通知）。community 侧扇出落库并经 realtime-hub 远程推送。
// Url 为空时为 no-op：community 未部署的环境零开销、零行为差异。
package notify

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tappi/tappi/services/game-catalog/model"

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
	// outbox 非空 = 投递失败先落 outbox 待 Relay 重投（本地补偿）。重投安全
	// 性由 community 侧 UNIQUE(user_id, actor_id, type, target_id) 去重保证。
	outbox *model.NotifyOutboxModel
}

// NewPublisher 构建发布器；url 空返回 no-op 实现。outbox 非空时启用本地
// 补偿：先落队列再同步首投，失败留 pending 由 Relay 重投（节奏与退避在
// Relay 侧，队列首行立即可扫）。
func NewPublisher(url, token string, outbox *model.NotifyOutboxModel) Publisher {
	if url == "" {
		return noop{}
	}
	return &publisher{
		url:    url + "/internal/notifications",
		token:  token,
		client: &http.Client{Timeout: 3 * time.Second},
		outbox: outbox,
	}
}

// Publish 同步投递（3s 超时）：先落 outbox（幂等，写失败仅记日志仍尝试直投），
// 再同步首投；成功标记 delivered（Relay 不再重投），失败留 pending 待 Relay
// 重投。同步是为了触发路径可验收（测试断言 community 侧收到扇出）；3s 上限
// 保证不拖慢主操作。
func (p *publisher) Publish(n Notification) {
	body, err := json.Marshal(n)
	if err != nil {
		logx.Errorf("notify %s: marshal: %v", n.Type, err)
		return
	}
	key := notifyKey(n.Type)
	if p.outbox != nil {
		if err := p.outbox.Enqueue(key, n.Type, string(body), nowUTC()); err != nil {
			logx.Errorf("notify %s: enqueue outbox: %v", n.Type, err)
		}
	}
	if err := p.post(body); err != nil {
		logx.Errorf("notify %s: %v", n.Type, err)
		return
	}
	if p.outbox != nil {
		if err := p.outbox.MarkDelivered(key, nowUTC()); err != nil {
			logx.Errorf("notify %s: mark delivered: %v", n.Type, err)
		}
	}
}

// post 低层投递（2xx 视为受理；community 侧按 (user,actor,type,target) 去重）。
func (p *publisher) post(body []byte) error {
	req, err := http.NewRequest(http.MethodPost, p.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", p.token)
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("community returned %d", resp.StatusCode)
	}
	return nil
}

// notifyKey 生成 outbox 行唯一键（随机事实 ID，与 download 事实同款：通知可
// 重复触发，每次 Publish 为独立逻辑事件，重放依赖 community 侧去重而非派生键）。
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

func (p *publisher) Enabled() bool { return true }

type noop struct{}

func (noop) Publish(Notification) {}
func (noop) Enabled() bool        { return false }
