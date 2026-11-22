package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tappi/tappi/pkg/resilience"
)

// GameCatalogClient BFF → game-catalog 客户端。CLN-004：读路径接入统一
// 韧性抽象（pkg/resilience 熔断 + 确定性指数退避重试），聚合策略
// （单上游失败降级该组）保持 logic 层语义不变。
type GameCatalogClient struct {
	baseURL    string
	httpClient *http.Client
	breaker    *resilience.Breaker
	sleep      func(time.Duration) // 退避休眠注入点（测试用）
}

func NewGameCatalogClient(baseURL string, timeout time.Duration) *GameCatalogClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &GameCatalogClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		breaker: resilience.NewBreaker(resilience.DefaultMaxFailures, resilience.DefaultCooldown),
	}
}

func (c *GameCatalogClient) GetFeatured(ctx context.Context, limit int64) (map[string]interface{}, error) {
	return resilience.Call(ctx, resilience.CallOptions{
		Retry:   resilience.DefaultRetryConfig(),
		Breaker: c.breaker,
		Sleep:   c.sleepOrTimeSleep(),
	}, func(ctx context.Context) (map[string]interface{}, error) {
		return c.getFeaturedOnce(ctx, limit)
	})
}

func (c *GameCatalogClient) sleepOrTimeSleep() func(time.Duration) {
	if c.sleep != nil {
		return c.sleep
	}
	return time.Sleep
}

// getFeaturedOnce 单次精选游戏查询（错误按韧性分类）。
func (c *GameCatalogClient) getFeaturedOnce(ctx context.Context, limit int64) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("%s/games/featured?limit=%d", c.baseURL, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, resilience.NewError(resilience.KindCanceled, fmt.Sprintf("create request failed: %v", err))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, resilience.NewError(resilience.KindCanceled, fmt.Sprintf("game catalog request failed: %v", err))
		}
		return nil, resilience.NewError(resilience.KindTransient, fmt.Sprintf("game catalog request failed: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, resilience.NewError(
			resilience.StatusKind(resp.StatusCode),
			fmt.Sprintf("game catalog status %d", resp.StatusCode),
		)
	}

	result := make(map[string]interface{})
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, resilience.WrapError(resilience.KindContract, err, fmt.Sprintf("game catalog decode: %v", err))
	}
	return result, nil
}
