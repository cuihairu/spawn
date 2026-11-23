package courierdownload

// GP-002 映射 download 能力查询客户端：game-catalog 受控包体下载前经
// courier-adapter S2S GET /internal/games/:id/mapping（X-Internal-Token）
// 查询该游戏是否开通「平台包体下载」能力（能力按需开通，与联运标签解耦，
// 见 courier-integration-design §2.1）。未配置 BaseURL 时 NewClient 返回
// nil，调用方按 no-op 处理（不门控，保持既有下载行为）——部署面未接线
// 时不因新依赖把既有下载打断。

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ErrMappingNotFound 游戏未配置 Courier 映射（未接入）。
var ErrMappingNotFound = errors.New("courier mapping not found")

// Client courier-adapter 内部映射端点客户端。
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient baseURL 为空返回 nil（未接线 = 调用方 no-op）。
func NewClient(baseURL, token string, timeout time.Duration) *Client {
	if baseURL == "" {
		return nil
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: timeout},
	}
}

type mappingResponse struct {
	Capabilities struct {
		Download bool `json:"download"`
	} `json:"capabilities"`
}

// DownloadEnabled 查询游戏是否开通平台包体下载能力。404（未映射）= false
// 无错（未接入即未开通）；非 200 = 错误（调用方 fail-closed）。
func (c *Client) DownloadEnabled(spawnGameID string) (bool, error) {
	req, err := http.NewRequest(http.MethodGet,
		c.baseURL+"/internal/games/"+url.PathEscape(spawnGameID)+"/mapping", nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("X-Internal-Token", c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("courier-adapter mapping: HTTP %d: %s", resp.StatusCode, string(raw))
	}
	var m mappingResponse
	if err := json.Unmarshal(raw, &m); err != nil {
		return false, fmt.Errorf("courier-adapter mapping 响应非 JSON: %w", err)
	}
	return m.Capabilities.Download, nil
}
