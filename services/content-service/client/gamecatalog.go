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

	"github.com/tappi/tappi/pkg/resilience"
	"github.com/tappi/tappi/services/content-service/internal/metrics"
)

// 默认熔断与重试参数（CLN-004：取值上移 pkg/resilience 默认值）。
const (
	defaultMaxAttempts = resilience.DefaultMaxAttempts // 单次调用最多尝试次数（含首次）
	defaultBaseDelay   = resilience.DefaultBaseDelay   // 重试基础退避
	defaultMaxDelay    = resilience.DefaultMaxDelay    // 重试退避上限
	defaultMaxFailures = resilience.DefaultMaxFailures // 连续失败熔断阈值
	defaultCooldown    = resilience.DefaultCooldown    // 熔断开启后的冷却时长
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
	metrics    *clientMetrics
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
	opts.retry = opts.retry.Normalize()
	if opts.sleep == nil {
		opts.sleep = time.Sleep
	}
	if opts.breaker == nil {
		opts.breaker = NewBreaker(defaultMaxFailures, defaultCooldown)
	}
	if opts.metrics == nil {
		opts.metrics = defaultMetrics
	}
	// 接入熔断状态指标（客户端 metrics + 服务内部 metrics）。
	attachBreakerMetrics(opts.breaker, opts.metrics)

	return &GameCatalogClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		retry:   opts.retry,
		breaker: opts.breaker,
		sleep:   opts.sleep,
		metrics: opts.metrics,
	}
}

// internalStatusOf 将调用结果映射为内部 metrics 的状态标签。
func internalStatusOf(r resilience.Result) string {
	switch r {
	case resilience.ResultSuccess:
		return "success"
	case resilience.ResultBreakerOpen:
		return "circuit_open"
	case resilience.ResultCanceled:
		return "canceled"
	case resilience.ResultTransient:
		return "transient_error"
	case resilience.ResultBadRequest:
		return "bad_request"
	case resilience.ResultContract:
		return "parse_error"
	default:
		return "unknown_error"
	}
}

// GetGameById 根据游戏ID获取游戏信息。
// 瞬时错误（网络错误、5xx、429、业务错误码）按指数退避重试；
// 熔断开启时快速失败。失败降级（调用方用 gameId 作标题）的语义保持不变。
func (c *GameCatalogClient) GetGameById(ctx context.Context, gameId string) (*GameInfo, error) {
	start := time.Now()
	defer func() {
		c.metrics.duration.Observe(time.Since(start).Seconds())
		metrics.GameCatalogClientCallDurationSeconds.WithLabelValues("GetGameById").Observe(time.Since(start).Seconds())
	}()

	game, err := resilience.Call(ctx, resilience.CallOptions{
		Retry:   c.retry,
		Breaker: c.breaker,
		Sleep:   c.sleep,
		OnResult: func(r resilience.Result) {
			c.metrics.requests.WithLabelValues(string(r)).Inc()
			metrics.GameCatalogClientCallsTotal.WithLabelValues("GetGameById", internalStatusOf(r)).Inc()
		},
		OnRetry: func() {
			c.metrics.retries.Inc()
		},
	}, func(ctx context.Context) (*GameInfo, error) {
		return c.getGameByIdOnce(ctx, gameId)
	})
	if errors.Is(err, resilience.ErrBreakerOpen) {
		return nil, ErrBreakerOpen
	}
	if err != nil {
		return nil, err
	}
	return game, nil
}

// getGameByIdOnce 发起单次请求并按错误类别返回。
func (c *GameCatalogClient) getGameByIdOnce(ctx context.Context, gameId string) (*GameInfo, error) {
	url := fmt.Sprintf("%s/games/%s", c.baseURL, gameId)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, resilience.NewError(kindCanceled, fmt.Sprintf("创建请求失败: %v", err))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// 调用方上下文已取消/超时：不算服务故障。
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, resilience.NewError(kindCanceled, fmt.Sprintf("请求失败: %v", err))
		}
		return nil, resilience.NewError(kindTransient, fmt.Sprintf("请求失败: %v", err))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		// 读取响应体失败通常是连接中断，属于瞬时错误。
		return nil, resilience.NewError(kindTransient, fmt.Sprintf("读取响应失败: %v", err))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resilience.NewError(
			statusKind(resp.StatusCode),
			fmt.Sprintf("请求失败，状态码: %d, 响应: %s", resp.StatusCode, string(body)),
		)
	}

	var payload responsePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, resilience.NewError(kindContract, fmt.Sprintf("解析响应失败: %v", err))
	}

	if payload.Code != nil && *payload.Code != http.StatusOK {
		return nil, resilience.NewError(kindTransient, fmt.Sprintf("获取游戏信息失败: %s", payload.Message))
	}

	if payload.Game != nil {
		return payload.Game, nil
	}
	if payload.Data != nil {
		return payload.Data, nil
	}

	return nil, resilience.NewError(kindContract, fmt.Sprintf("响应格式无法识别: %s", string(body)))
}
