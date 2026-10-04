package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Upstream struct {
	baseURL *url.URL
	client  *http.Client
}

func NewUpstream(baseURL string, timeout time.Duration) (*Upstream, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("upstream base url is empty")
	}
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse upstream base url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid upstream base url: %s", baseURL)
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Upstream{
		baseURL: u,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func (u *Upstream) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u.serveHTTP(w, r)
	}
}

// maxProxyBody 反代请求体上限：移动端请求都是小 JSON，缓冲后以定长
// Content-Length 转发（见 serveHTTP 内说明），超限直接 413。
const maxProxyBody = 1 << 20 // 1 MiB

func (u *Upstream) serveHTTP(w http.ResponseWriter, r *http.Request) {
	upstreamURL := *u.baseURL
	upstreamURL.Path = strings.TrimRight(u.baseURL.Path, "/") + r.URL.Path
	upstreamURL.RawQuery = r.URL.RawQuery

	// 缓冲请求体再转发：直接转发 r.Body 时长度未知，Go 会改用 chunked 编码，
	// 而 go-zero 上游对 chunked 请求体解析失败（user-service 实测
	// httpx.Parse 报「field is not set」，直连同样请求带 Content-Length 正常）。
	// 缓冲后走 *bytes.Reader → http 库自动携带定长 Content-Length。
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxProxyBody+1))
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "read request body")
		return
	}
	if len(bodyBytes) > maxProxyBody {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	var body io.Reader
	if len(bodyBytes) > 0 {
		body = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL.String(), body)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "bad gateway")
		return
	}
	req.Header = cloneHeader(r.Header)
	req.Header.Del("Content-Length") // 定长由 req.ContentLength 驱动，避免双写
	req.Host = u.baseURL.Host

	resp, err := u.client.Do(req)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "upstream unavailable")
		return
	}
	defer resp.Body.Close()

	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, fmt.Sprintf(`{"code":%d,"message":%q}`, status, message))
}

func cloneHeader(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, vv := range h {
		if isHopByHopHeader(k) {
			continue
		}
		copied := make([]string, len(vv))
		copy(copied, vv)
		out[k] = copied
	}
	return out
}

func copyHeader(dst, src http.Header) {
	for k := range dst {
		if isHopByHopHeader(k) {
			dst.Del(k)
		}
	}
	for k, vv := range src {
		if isHopByHopHeader(k) {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func isHopByHopHeader(k string) bool {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "connection", "proxy-connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
		"te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func (u *Upstream) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.baseURL.String()+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("upstream unhealthy: %d", resp.StatusCode)
	}
	return nil
}
