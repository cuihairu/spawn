// Package courier Courier 网关客户端（identity 域，契约 Frozen：
// scope v1 / auth v1.1）。只做 header 注入（X-Courier-Game-Id /
// X-Courier-Env），scope 一律不进 URL/payload；不 fork 契约、不重签 token。
package courier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	hc      *http.Client
}

func NewClient(baseURL string, timeout time.Duration) (*Client, error) {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		return nil, fmt.Errorf("courier gateway base url is empty")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{baseURL: base, hc: &http.Client{Timeout: timeout}}, nil
}

// Session Courier 会话（sessionDTO 口径，auth.md Frozen v1.1）。
type Session struct {
	AccountID        string
	AccountType      string
	AccountStatus    string
	AccessToken      string
	AccessExpiresAt  string
	RefreshToken     string
	RefreshExpiresAt string
}

// DeleteResult 注销受理（v1.1：cancelToken 仅 PENDING，deletedAt 仅 DELETED）。
type DeleteResult struct {
	Status        string
	CancelToken   string
	PendingDelete bool
	DeletedAt     string
}

// APIError Courier 错误信封（{error:{code,message,retryable},traceID}）。
type APIError struct {
	HTTPStatus int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

func NewAPIError(status int, code, message string) *APIError {
	return &APIError{HTTPStatus: status, Code: code, Message: message}
}

// Register 邮箱+密码注册并登录（自动建号，接入设计 §1.3 步骤 3）。
func (c *Client) Register(ctx context.Context, gameID, env, email, password string) (*Session, error) {
	return c.sessionCall(ctx, gameID, env, "register", map[string]string{
		"email": email, "password": password,
	})
}

// Login 邮箱+密码代登录，scope = 该 game_id+env（接入设计 §1.3 步骤 4）。
func (c *Client) Login(ctx context.Context, gameID, env, email, password string) (*Session, error) {
	return c.sessionCall(ctx, gameID, env, "login", map[string]string{
		"email": email, "password": password,
	})
}

// Logout 吊销当前 scope 会话（auth.md POST /logout；幂等）。
func (c *Client) Logout(ctx context.Context, gameID, env, accessToken string) error {
	_, err := c.call(ctx, gameID, env, http.MethodPost, "logout", accessToken, nil)
	return err
}

// DeleteAccount 申请注销（Bearer；EMAIL 宽限期 / GUEST 立即）。
func (c *Client) DeleteAccount(ctx context.Context, gameID, env, accessToken string) (*DeleteResult, error) {
	data, err := c.call(ctx, gameID, env, http.MethodPost, "account/delete", accessToken, map[string]any{})
	if err != nil {
		return nil, err
	}
	res := &DeleteResult{}
	_ = json.Unmarshal(data, res) // 字段宽松：缺失即零值（契约字段可选）
	return res, nil
}

// CancelDelete 宽限期内撤销注销（匿名 + cancelToken；v1.1）。
func (c *Client) CancelDelete(ctx context.Context, gameID, env, cancelToken string) error {
	_, err := c.call(ctx, gameID, env, http.MethodPost, "account/delete/cancel", "",
		map[string]string{"cancelToken": cancelToken})
	return err
}

func (c *Client) sessionCall(ctx context.Context, gameID, env, action string, payload map[string]string) (*Session, error) {
	data, err := c.call(ctx, gameID, env, http.MethodPost, action, "", payload)
	if err != nil {
		return nil, err
	}
	var dto struct {
		Account struct {
			ID     string `json:"id"`
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"account"`
		AccessToken      string `json:"accessToken"`
		AccessExpiresAt  string `json:"accessExpiresAt"`
		RefreshToken     string `json:"refreshToken"`
		RefreshExpiresAt string `json:"refreshExpiresAt"`
	}
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, fmt.Errorf("decode %s session: %w", action, err)
	}
	if dto.AccessToken == "" || dto.Account.ID == "" {
		return nil, fmt.Errorf("%s response missing accessToken/accountId", action)
	}
	return &Session{
		AccountID:        dto.Account.ID,
		AccountType:      dto.Account.Type,
		AccountStatus:    dto.Account.Status,
		AccessToken:      dto.AccessToken,
		AccessExpiresAt:  dto.AccessExpiresAt,
		RefreshToken:     dto.RefreshToken,
		RefreshExpiresAt: dto.RefreshExpiresAt,
	}, nil
}

// call 发请求并解信封。scope 只经 header；body 恒不含 game_id/env。
func (c *Client) call(ctx context.Context, gameID, env, method, action, accessToken string, payload any) (json.RawMessage, error) {
	return c.callPath(ctx, gameID, env, method, "/v1/identity/"+action, accessToken, payload)
}

// callPath 同 call，但路径整条由调用方给（support 域 /v1/support/* 用）。
func (c *Client) callPath(ctx context.Context, gameID, env, method, path, accessToken string, payload any) (json.RawMessage, error) {
	var body io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(buf)
	} else if method != http.MethodGet {
		body = bytes.NewReader([]byte("{}"))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Courier-Game-Id", gameID)
	req.Header.Set("X-Courier-Env", env)
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("courier %s: %w", path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read courier %s response: %w", path, err)
	}
	if resp.StatusCode >= 400 {
		var fail struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &fail)
		return nil, &APIError{
			HTTPStatus: resp.StatusCode,
			Code:       fail.Error.Code,
			Message:    fail.Error.Message,
		}
	}
	var ok struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &ok); err != nil {
		return nil, fmt.Errorf("decode courier %s envelope: %w", path, err)
	}
	return ok.Data, nil
}

// --- GP-307 售后工单（support 域，契约 Frozen v1.1 只读适配）---

// SupportTicket 工单 wire 形状（契约数据模型；context 原样回显）。
type SupportTicket struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Status    string         `json:"status"`
	Category  string         `json:"category"`
	Context   map[string]any `json:"context,omitempty"`
	CreatedAt string         `json:"createdAt"`
	UpdatedAt string         `json:"updatedAt"`
}

// SupportMessage 工单消息（senderType PLAYER/AGENT/SYSTEM）。
type SupportMessage struct {
	SenderType string `json:"senderType"`
	Body       string `json:"body"`
	CreatedAt  string `json:"createdAt"`
}

// SupportTicketDetail 工单详情：处理记录 = messages（createdAt 升序）。
type SupportTicketDetail struct {
	Ticket   SupportTicket    `json:"ticket"`
	Messages []SupportMessage `json:"messages"`
}

// SupportPage 工单列表页（updatedAt 倒序，cursor 契约原样透传）。
type SupportTicketPage struct {
	Items      []SupportTicket `json:"items"`
	NextCursor string          `json:"nextCursor"`
}

// SupportFAQ FAQ 条目。
type SupportFAQ struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Answer   string   `json:"answer"`
	Keywords []string `json:"keywords"`
}

// SupportTicketInput 提单入参（context 由 Spawn 侧构建后传入）。
type SupportTicketInput struct {
	Title    string         `json:"title"`
	Body     string         `json:"body"`
	Category string         `json:"category,omitempty"`
	Context  map[string]any `json:"context,omitempty"`
}

// CreateTicket 提单（POST /v1/support/tickets；Bearer = 该 scope 会话）。
func (c *Client) CreateTicket(ctx context.Context, gameID, env, accessToken string, in SupportTicketInput) (*SupportTicket, error) {
	data, err := c.callPath(ctx, gameID, env, http.MethodPost, "/v1/support/tickets", accessToken, in)
	if err != nil {
		return nil, err
	}
	t := &SupportTicket{}
	if err := json.Unmarshal(data, t); err != nil {
		return nil, fmt.Errorf("decode support ticket: %w", err)
	}
	return t, nil
}

// ListTickets 我的工单列表（GET /v1/support/tickets；limit/cursor 透传）。
func (c *Client) ListTickets(ctx context.Context, gameID, env, accessToken string, limit, cursor string) (*SupportTicketPage, error) {
	q := url.Values{}
	if limit != "" {
		q.Set("limit", limit)
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	path := "/v1/support/tickets"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	data, err := c.callPath(ctx, gameID, env, http.MethodGet, path, accessToken, nil)
	if err != nil {
		return nil, err
	}
	page := &SupportTicketPage{Items: []SupportTicket{}}
	if err := json.Unmarshal(data, page); err != nil {
		return nil, fmt.Errorf("decode support ticket page: %w", err)
	}
	return page, nil
}

// TicketDetail 工单详情（处理记录 = messages 升序）。
func (c *Client) TicketDetail(ctx context.Context, gameID, env, accessToken, ticketID string) (*SupportTicketDetail, error) {
	data, err := c.callPath(ctx, gameID, env, http.MethodGet, "/v1/support/tickets/"+url.PathEscape(ticketID), accessToken, nil)
	if err != nil {
		return nil, err
	}
	d := &SupportTicketDetail{}
	if err := json.Unmarshal(data, d); err != nil {
		return nil, fmt.Errorf("decode support ticket detail: %w", err)
	}
	return d, nil
}

// AppendTicketMessage 追加工单消息（POST .../messages；CLOSED → 409）。
func (c *Client) AppendTicketMessage(ctx context.Context, gameID, env, accessToken, ticketID, body string) (*SupportMessage, error) {
	data, err := c.callPath(ctx, gameID, env, http.MethodPost,
		"/v1/support/tickets/"+url.PathEscape(ticketID)+"/messages", accessToken, map[string]string{"body": body})
	if err != nil {
		return nil, err
	}
	m := &SupportMessage{}
	if err := json.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("decode support message: %w", err)
	}
	return m, nil
}

// SearchFAQ FAQ 检索（keyword 可空 = 全量；limit 透传）。
func (c *Client) SearchFAQ(ctx context.Context, gameID, env, accessToken, keyword, limit string) ([]SupportFAQ, error) {
	q := url.Values{}
	if keyword != "" {
		q.Set("keyword", keyword)
	}
	if limit != "" {
		q.Set("limit", limit)
	}
	path := "/v1/support/faq"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	data, err := c.callPath(ctx, gameID, env, http.MethodGet, path, accessToken, nil)
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []SupportFAQ `json:"items"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("decode support faq: %w", err)
	}
	if page.Items == nil {
		page.Items = []SupportFAQ{}
	}
	return page.Items, nil
}
