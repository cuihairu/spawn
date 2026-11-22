// Package playeract GP-303 回流判定源上报：游戏会话签发成功 = 账号×游戏
// 活跃事实，向 promotions /internal/player-activity 上报（S2S，X-Internal-Token）。
// 上报先落本地 outbox 再同步首投，失败留 pending 由 Relay 重投——活跃事实
// 丢失会让玩家 previous_active 停在旧时刻，回流判定缺口被高估（误判回流、
// 错发方向），故按 outbox 口径持久化。promotions 侧活跃写入条件推进
// （仅更新更新时刻），重复上报不改变判定结果。
package playeract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tappi/tappi/services/courier-adapter/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// Report 活跃上报请求体（promotions /internal/player-activity 契约）。
type Report struct {
	AccountId string `json:"account_id"`
	GameId    string `json:"game_id"`
	ActiveAt  string `json:"active_at"`
}

// Reporter 活跃上报器（outbox + 同步首投）。
type Reporter struct {
	outbox *model.ActivityOutboxModel
	client *Client
}

// NewReporter 构造；baseURL 空 = 未接促销服务（返回 nil，上报 no-op，
// 游戏会话主流程不受影响）。
func NewReporter(baseURL, token string, timeoutMs int64, outbox *model.ActivityOutboxModel) *Reporter {
	c := NewClient(baseURL, token, timeoutMs)
	if c == nil {
		return nil
	}
	return &Reporter{outbox: outbox, client: c}
}

// Report 上报活跃事实：先落 outbox（幂等），再同步首投；成功置 delivered，
// 失败记 attempts 并按指数退避留 pending（Relay 续投）。会话主流程不等待
// 重投、不受上报成败影响。
func (r *Reporter) Report(ctx context.Context, accountID, gameID, activeAt string) {
	if r == nil {
		return
	}
	payload, err := json.Marshal(Report{AccountId: accountID, GameId: gameID, ActiveAt: activeAt})
	if err != nil {
		return
	}
	actKey := fmt.Sprintf("act:%s:%s:%s", accountID, gameID, activeAt)
	// 首投失败后的最早重投时刻（1 分钟，Relay 按 attempts 指数退避覆盖）
	next := time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
	if err := r.outbox.Enqueue(actKey, accountID, gameID, activeAt, string(payload), next); err != nil {
		logx.Errorf("player activity enqueue failed (user %s game %s): %v", accountID, gameID, err)
		return
	}
	if err := r.client.Report(ctx, payload); err != nil {
		// 同步首投失败：留 pending（attempts=1），Relay 定时续投
		if err2 := r.outbox.MarkFailed(actKey, 1, truncate(err.Error(), 500), next); err2 != nil {
			logx.Errorf("player activity mark failed (key %s): %v", actKey, err2)
		}
		return
	}
	if err := r.outbox.MarkDelivered(actKey, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		logx.Errorf("player activity mark delivered (key %s): %v", actKey, err)
	}
}

// Client promotions 活跃上报 HTTP 客户端。
type Client struct {
	baseURL string
	token   string
	hc      *http.Client
}

// NewClient 构造；baseURL 空 = 未接促销服务（返回 nil）。
func NewClient(baseURL, token string, timeoutMs int64) *Client {
	if baseURL == "" {
		return nil
	}
	if timeoutMs <= 0 {
		timeoutMs = 3000
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		hc:      &http.Client{Timeout: time.Millisecond * time.Duration(timeoutMs)},
	}
}

// Report 同步上报单条事实（2xx 视为受理；promotions 侧按条件推进幂等）。
func (c *Client) Report(ctx context.Context, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/internal/player-activity", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.token)
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("promotions 不可达: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("promotions 活跃上报返回 %d", resp.StatusCode)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
