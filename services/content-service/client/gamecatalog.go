package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// 默认熔断与重试参数。
const (
	defaultMaxAttempts = 3                      // 单次调用最多尝试次数（含首次）
	defaultBaseDelay   = 100 * time.Millisecond // 重试基础退避
	defaultMaxDelay    = time.Second            // 重试退避上限
	defaultMaxFailures = 3                      // 连续失败熔断阈值
	defaultCooldown    = 5 * time.Second        // 熔断开启后的冷却时长
)

// ErrBreakerOpen 熔断器处于开启状态时快速失败返回的错误。
var ErrBreakerOpen = errors.New("熔断已开启，快速失败")

// GameCatalogClient 游戏目录服务客户端
type GameCatalogClient struct {
	baseURL    string
	httpClient *http.Client
	retry      retryConfig
	breaker    *Breaker
	sleep      func(time.Duration)
}

// GameInfo 游戏基本信息
type GameInfo struct {
	Id         string   `json:"id"`
	Title      string   `json:"title"`
	CoverImage string   `json:"cover_image"`
	Genres     []string `json:"genres"`
	Platforms  []string `json:"platforms"`
}

// responsePayload 兼容两种返回格式：
//   - game-catalog 实际返回：{"game": {...}}（GET /games/:id）
//   - 早期文档约定的旧格式：{"code": 200, "message": "...", "data": {...}}
//
// Code/Data/Game 均用指针以区分字段缺失与零值。
type responsePayload struct {
	Code    *int      `json:"code"`
	Message string    `json:"message"`
	Data    *GameInfo `json:"data"`
	Game    *GameInfo `json:"game"`
}

// NewGameCatalogClient 创建游戏目录服务客户端（使用默认熔断与重试参数）。
func NewGameCatalogClient(baseURL string, timeout time.Duration) *GameCatalogClient {
	return newGameCatalogClient(baseURL, timeout, defaultClientOptions())
}

// newGameCatalogClient 创建客户端，允许测试注入重试参数、熔断器与休眠函数。
func newGameCatalogClient(baseURL string, timeout time.Duration, opts clientOptions) *GameCatalogClient {
	if opts.retry.maxAttempts < 1 {
		opts.retry.maxAttempts = 1
	}
	if opts.retry.baseDelay < 0 {
		opts.retry.baseDelay = 0
	}
	if opts.retry.maxDelay < 0 {
		opts.retry.maxDelay = 0
	}
	if opts.sleep == nil {
		opts.sleep = time.Sleep
	}
	if opts.breaker == nil {
		opts.breaker = NewBreaker(defaultMaxFailures, defaultCooldown)
	}

	return &GameCatalogClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		retry:   opts.retry,
		breaker: opts.breaker,
		sleep:   opts.sleep,
	}
}

// GetGameById 根据游戏ID获取游戏信息。
// 瞬时错误（网络错误、5xx、429、业务错误码）按指数退避重试；
// 熔断开启时快速失败。失败降级（调用方用 gameId 作标题）的语义保持不变。
func (c *GameCatalogClient) GetGameById(ctx context.Context, gameId string) (*GameInfo, error) {
	// 调用方上下文已不可用：直接失败，不触发重试或熔断。
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 1; attempt <= c.retry.maxAttempts; attempt++ {
		// 熔断检查：open 时快速失败；half-open 时仅放行探测请求。
		if !c.breaker.Allow() {
			return nil, ErrBreakerOpen
		}

		game, err := c.getGameByIdOnce(ctx, gameId)
		if err == nil {
			c.breaker.Success()
			return game, nil
		}

		lastErr = err
		reportBreaker(c.breaker, err)

		if attempt == c.retry.maxAttempts || !isRetryable(err) {
			return nil, lastErr
		}

		// 退避期间上下文失效则放弃后续重试。
		delay := c.retry.backoffDelay(attempt)
		if delay > 0 {
			c.sleep(delay)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

// getGameByIdOnce 发起单次请求并按错误类别返回。
func (c *GameCatalogClient) getGameByIdOnce(ctx context.Context, gameId string) (*GameInfo, error) {
	url := fmt.Sprintf("%s/games/%s", c.baseURL, gameId)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, &callError{kind: kindCanceled, msg: fmt.Sprintf("创建请求失败: %v", err)}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// 调用方上下文已取消/超时：不算服务故障。
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, &callError{kind: kindCanceled, msg: fmt.Sprintf("请求失败: %v", err)}
		}
		return nil, &callError{kind: kindTransient, msg: fmt.Sprintf("请求失败: %v", err)}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		// 读取响应体失败通常是连接中断，属于瞬时错误。
		return nil, &callError{kind: kindTransient, msg: fmt.Sprintf("读取响应失败: %v", err)}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &callError{
			kind: statusKind(resp.StatusCode),
			msg:  fmt.Sprintf("请求失败，状态码: %d, 响应: %s", resp.StatusCode, string(body)),
		}
	}

	var payload responsePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, &callError{kind: kindContract, msg: fmt.Sprintf("解析响应失败: %v", err)}
	}

	if payload.Code != nil && *payload.Code != http.StatusOK {
		return nil, &callError{kind: kindTransient, msg: fmt.Sprintf("获取游戏信息失败: %s", payload.Message)}
	}

	if payload.Game != nil {
		return payload.Game, nil
	}
	if payload.Data != nil {
		return payload.Data, nil
	}

	return nil, &callError{kind: kindContract, msg: fmt.Sprintf("响应格式无法识别: %s", string(body))}
}
