package logic

// GP-307 售后工单（F25）：复用/适配 Courier SupportProvider（croupier，
// 契约 Frozen v1.1 只读）。Spawn 作为接入方：
//   - 工单归属账号 = 平台用户 ↔ Courier 一对一绑定（GP-201 同款绑定/代登录）；
//   - context 关联事实由 Spawn 侧构建：spawn_user_id / courier_account_id /
//     game_id 自动附加且不可被业务覆盖（契约 Client Requirements 口径）；
//     serverId 经 game-catalog 公开区服表校验归属后入 context；orderId 基础
//     卫生检查后透传（Courier 契约 context 开集零解释）；
//   - 处理记录 = Courier 工单消息流（PLAYER/AGENT/SYSTEM，详情升序）+
//     Spawn 侧审计（创建/追加）。
// 坐席侧（回复/关单）走 Courier 管理面，不经本服务（契约边界）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/tappi/tappi/services/courier-adapter/courier"
	"github.com/tappi/tappi/services/courier-adapter/internal/svc"
	"github.com/tappi/tappi/services/courier-adapter/internal/types"
	"github.com/tappi/tappi/services/courier-adapter/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type SupportLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSupportLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SupportLogic {
	return &SupportLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// validContextKey 契约 context 键形校验：1-32 字符 [a-zA-Z0-9_.]。
func validContextKey(k string) bool {
	if n := len(k); n < 1 || n > 32 {
		return false
	}
	for _, c := range k {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

// mapping 校验游戏接入 + 工单能力开通。
func (l *SupportLogic) mapping(spawnGameID string) (*model.GameMapping, error) {
	mapping, err := l.svcCtx.Mappings.Get(spawnGameID)
	if err != nil {
		if errors.Is(err, model.ErrMappingNotFound) {
			return nil, newAPIError(http.StatusNotFound, "游戏未接入 Courier")
		}
		return nil, newAPIError(http.StatusInternalServerError, "查询映射失败")
	}
	if !mapping.Capabilities.Support {
		return nil, newAPIError(http.StatusForbidden, "该游戏未开通客服工单")
	}
	return mapping, nil
}

// Create 提单：绑定/代登录 → 构建关联 context → Courier 提单 → 审计。
func (l *SupportLogic) Create(req *types.CreateSupportTicketRequest, authHeader string, userID int64) (*types.SupportTicketResponse, error) {
	mapping, err := l.mapping(req.Id)
	if err != nil {
		return nil, err
	}
	sess, err := l.session(mapping, authHeader, userID)
	if err != nil {
		return nil, err
	}
	ctxMap, err := l.buildContext(mapping, sess.AccountID, req, userID)
	if err != nil {
		return nil, err
	}
	t, err := l.svcCtx.Courier.CreateTicket(l.ctx, mapping.CourierGameId, mapping.Env, sess.AccessToken, courier.SupportTicketInput{
		Title:    req.Title,
		Body:     req.Body,
		Category: req.Category,
		Context:  ctxMap,
	})
	if err != nil {
		l.Errorf("courier create ticket failed (user %d game %s): %v", userID, req.Id, err)
		return nil, supportUpstreamError(err)
	}
	l.audit(userID, "support_ticket_create", t.ID,
		fmt.Sprintf(`{"spawnGameId":%q,"category":%q,"contextKeys":%d}`, req.Id, req.Category, len(ctxMap)))
	return &types.SupportTicketResponse{Ticket: *t}, nil
}

// List 我的工单列表（updatedAt 倒序，limit/cursor 契约透传）。
func (l *SupportLogic) List(req *types.SupportTicketListRequest, authHeader string, userID int64) (*types.SupportTicketListResponse, error) {
	mapping, err := l.mapping(req.Id)
	if err != nil {
		return nil, err
	}
	sess, err := l.session(mapping, authHeader, userID)
	if err != nil {
		return nil, err
	}
	page, err := l.svcCtx.Courier.ListTickets(l.ctx, mapping.CourierGameId, mapping.Env, sess.AccessToken, req.Limit, req.Cursor)
	if err != nil {
		l.Errorf("courier list tickets failed (user %d game %s): %v", userID, req.Id, err)
		return nil, supportUpstreamError(err)
	}
	return &types.SupportTicketListResponse{Items: page.Items, NextCursor: page.NextCursor}, nil
}

// Detail 工单详情：处理记录 = messages（createdAt 升序）。
func (l *SupportLogic) Detail(req *types.SupportTicketPathRequest, authHeader string, userID int64) (*types.SupportTicketDetailResponse, error) {
	mapping, err := l.mapping(req.Id)
	if err != nil {
		return nil, err
	}
	sess, err := l.session(mapping, authHeader, userID)
	if err != nil {
		return nil, err
	}
	d, err := l.svcCtx.Courier.TicketDetail(l.ctx, mapping.CourierGameId, mapping.Env, sess.AccessToken, req.TicketId)
	if err != nil {
		l.Errorf("courier ticket detail failed (user %d ticket %s): %v", userID, req.TicketId, err)
		return nil, supportUpstreamError(err)
	}
	return &types.SupportTicketDetailResponse{Ticket: d.Ticket, Messages: d.Messages}, nil
}

// Append 追加工单消息（CLOSED → 409 透传语义；玩家说话不改状态）。
func (l *SupportLogic) Append(req *types.AppendSupportTicketRequest, authHeader string, userID int64) (*types.SupportMessageResponse, error) {
	mapping, err := l.mapping(req.Id)
	if err != nil {
		return nil, err
	}
	sess, err := l.session(mapping, authHeader, userID)
	if err != nil {
		return nil, err
	}
	m, err := l.svcCtx.Courier.AppendTicketMessage(l.ctx, mapping.CourierGameId, mapping.Env, sess.AccessToken, req.TicketId, req.Body)
	if err != nil {
		l.Errorf("courier append message failed (user %d ticket %s): %v", userID, req.TicketId, err)
		return nil, supportUpstreamError(err)
	}
	l.audit(userID, "support_ticket_append", req.TicketId, fmt.Sprintf(`{"spawnGameId":%q}`, req.Id))
	return &types.SupportMessageResponse{Message: *m}, nil
}

// FAQ 检索（keyword 空 = 全量；空结果为正常空态非错误）。
func (l *SupportLogic) FAQ(req *types.SupportFaqRequest, authHeader string, userID int64) (*types.SupportFaqResponse, error) {
	mapping, err := l.mapping(req.Id)
	if err != nil {
		return nil, err
	}
	sess, err := l.session(mapping, authHeader, userID)
	if err != nil {
		return nil, err
	}
	items, err := l.svcCtx.Courier.SearchFAQ(l.ctx, mapping.CourierGameId, mapping.Env, sess.AccessToken, req.Keyword, req.Limit)
	if err != nil {
		l.Errorf("courier faq failed (user %d game %s): %v", userID, req.Id, err)
		return nil, supportUpstreamError(err)
	}
	return &types.SupportFaqResponse{Items: items}, nil
}

// session 绑定/代登录（GP-201 同款路径）。
func (l *SupportLogic) session(mapping *model.GameMapping, authHeader string, userID int64) (*courier.Session, error) {
	return NewSessionLogic(l.ctx, l.svcCtx).EnsureSession(mapping, authHeader, userID)
}

// buildContext 关联事实构建：业务 context 先铺底（键形/值型预检，越限即
// 400），Spawn 自动附加事实覆盖保留键（契约：自动附加键不可被业务覆盖）；
// serverId 经 game-catalog 区服表校验（不存在或不属于该游戏 → 400）；
// orderId 基础卫生检查后透传；总量 ≤10 键（契约上限）。
func (l *SupportLogic) buildContext(mapping *model.GameMapping, courierAccountID string, req *types.CreateSupportTicketRequest, userID int64) (map[string]any, error) {
	ctxMap := map[string]any{}
	for k, v := range req.Context {
		if !validContextKey(k) {
			return nil, newAPIError(http.StatusBadRequest, "context 键仅限 1-32 位 [a-zA-Z0-9_.]")
		}
		switch tv := v.(type) {
		case string:
			if utf8.RuneCountInString(tv) > 256 {
				return nil, newAPIError(http.StatusBadRequest, "context 字符串值 ≤256 字符")
			}
		case bool:
		case json.Number: // go-zero httpx.Parse 以 UseNumber 解码，数字落 json.Number
		case float64: // 兜底（直构场景）
		default:
			return nil, newAPIError(http.StatusBadRequest, "context 值须为 string/number/boolean")
		}
		ctxMap[k] = v
	}
	// 区服归属校验（game-catalog 公开区服表；上游不可达 fail-open 不写区服事实）
	if sid := strings.TrimSpace(req.ServerId); sid != "" {
		serverName, err := l.catalogServer(mapping.SpawnGameId, sid)
		if err != nil {
			return nil, err
		}
		ctxMap["server_id"] = sid
		ctxMap["server_name"] = serverName
	}
	if oid := strings.TrimSpace(req.OrderId); oid != "" {
		if utf8.RuneCountInString(oid) > 64 {
			return nil, newAPIError(http.StatusBadRequest, "orderId 过长")
		}
		ctxMap["order_id"] = oid
	}
	// 自动附加事实（覆盖保留键，业务不可抢占）
	ctxMap["spawn_user_id"] = userID
	ctxMap["courier_account_id"] = courierAccountID
	ctxMap["game_id"] = mapping.CourierGameId
	if len(ctxMap) > 10 {
		return nil, newAPIError(http.StatusBadRequest, "context 键总数须 ≤10（含自动附加事实）")
	}
	return ctxMap, nil
}

// catalogServer 查 game-catalog 公开区服表校验区服归属（返回区服名）。
// 上游不可达/查询失败 fail-open：不写区服事实也不阻断提单（区服校验为
// 增强事实，不作为提单前置硬依赖）；区服存在但不匹配 → 400。
func (l *SupportLogic) catalogServer(spawnGameID, serverID string) (string, error) {
	servers, err := fetchCatalogServers(l.ctx, l.svcCtx, spawnGameID)
	if err != nil {
		l.Errorf("list catalog servers for game %s failed: %v", spawnGameID, err)
		return "", nil
	}
	for _, s := range servers {
		if serverIDOf(s) == serverID {
			name, _ := s["name"].(string)
			return name, nil
		}
	}
	return "", newAPIError(http.StatusBadRequest, "区服不存在或不属于该游戏")
}

// serverIDOf 区服 id 归一（JSON 数字 float64 / 兼容字符串）。
func serverIDOf(s map[string]any) string {
	switch v := s["id"].(type) {
	case float64:
		return fmt.Sprintf("%d", int64(v))
	case string:
		return v
	}
	return ""
}

// fetchCatalogServers GET {game-catalog}/games/:id/servers（公开读）。
func fetchCatalogServers(ctx context.Context, svcCtx *svc.ServiceContext, spawnGameID string) ([]map[string]any, error) {
	pc := catalogClient(svcCtx)
	url := pc.base + "/games/" + spawnGameID + "/servers"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := pc.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("game-catalog unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("game-catalog servers status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	return body.Items, nil
}

// supportUpstreamError support 域错误映射：契约域错误码透传语义，其余归 502。
func supportUpstreamError(err error) error {
	var apiErr *courier.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case "SUPPORT_TICKET_NOT_FOUND":
			return newAPIError(http.StatusNotFound, "工单不存在")
		case "SUPPORT_TICKET_CLOSED":
			return newAPIError(http.StatusConflict, "工单已关闭，无法追加消息")
		case "COMMON_INVALID_ARGUMENT":
			return newAPIError(http.StatusBadRequest, "工单参数不合法（标题 1-120 字 / 正文 1-4000 字）")
		case "RATE_LIMITED":
			return newAPIError(http.StatusTooManyRequests, "提单过于频繁，请稍后再试")
		}
	}
	return courierUpstreamError("courier 工单服务不可用", err)
}

// audit 记录工单动作审计（处理留痕；失败仅记日志不阻断主流程）。
func (l *SupportLogic) audit(userID int64, action, resourceID, detail string) {
	if err := l.svcCtx.Audits.Insert(userID, action, "support_ticket", resourceID, detail, clientIP(l.ctx)); err != nil {
		l.Errorf("audit %s failed: %v", action, err)
	}
}
