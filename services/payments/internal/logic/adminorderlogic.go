package logic

// GP-204 运营订单管理面：只读投影检索 + 审计 + 退款/冲正管理适配。
//
// 投影是订单事实副本（接入设计 §3.1），状态来自 Courier 上报，本服务不
// 维护第二套可独立推进的状态机。Courier 无全局订单 HTTP 管理 API（调研
// §4 边界 6），运营检索/审计经本投影；不旁路契约。
//
// 退款/冲正：管理适配接缝——登记 PENDING 意图 + 状态映射（PENDING →
// 执行器 → EXECUTED/REJECTED）。**不假设 Courier 已有退款端点**：执行器
// 为 Courier 提案（provider 扩展或独立管理 Provider），接入前保持 PENDING
// 并附 pending-courier-refund-endpoint 说明，不伪造执行成功。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/internal/types"
	"github.com/tappi/tappi/services/payments/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// AdminOrderLogic 运营订单管理面。
type AdminOrderLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAdminOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AdminOrderLogic {
	return &AdminOrderLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func actorFrom(ctx context.Context) (userID int64, ip string) {
	if v, ok := ctx.Value("user_id").(int64); ok {
		userID = v
	}
	if v, ok := ctx.Value("client_ip").(string); ok {
		ip = v
	}
	return userID, ip
}

// ListOrders 运营订单检索（投影事实副本）；查询动作写审计。
func (l *AdminOrderLogic) ListOrders(req *types.AdminOrderQueryRequest, authHeader string) (*types.AdminOrderListResponse, error) {
	userID, ip := actorFrom(l.ctx)
	role, err := l.adminRole(authHeader)
	if err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.ProjectionDB.ListByQuery(req.GameId, req.Status, req.AccountId, int(req.Limit), int(req.Offset))
	if err != nil {
		l.Errorf("list orders failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询订单失败")
	}
	filter, _ := json.Marshal(map[string]string{
		"game_id": req.GameId, "status": req.Status, "account_id": req.AccountId,
	})
	_ = l.svcCtx.ProjectionDB.LogAudit(&model.OrderAuditLog{
		ActorId: userID, ActorRole: role, Action: "order_query", Detail: string(filter), Ip: ip,
	})
	items := make([]types.OrderProjectionView, 0, len(rows))
	for _, row := range rows {
		items = append(items, toProjectionView(row))
	}
	return &types.AdminOrderListResponse{Items: items}, nil
}

// GetOrder 运营订单详情（投影事实副本）。
func (l *AdminOrderLogic) GetOrder(req *types.OrderPathRequest, authHeader string) (*types.OrderProjectionView, error) {
	userID, ip := actorFrom(l.ctx)
	role, err := l.adminRole(authHeader)
	if err != nil {
		return nil, err
	}
	row, err := l.svcCtx.ProjectionDB.Get(req.OrderId)
	if errors.Is(err, model.ErrOrderProjectionNotFound) {
		return nil, newAPIError(http.StatusNotFound, "订单不存在")
	}
	if err != nil {
		l.Errorf("get order %s failed: %v", req.OrderId, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询订单失败")
	}
	_ = l.svcCtx.ProjectionDB.LogAudit(&model.OrderAuditLog{
		OrderId: req.OrderId, ActorId: userID, ActorRole: role, Action: "order_view", Ip: ip,
	})
	view := toProjectionView(row)
	return &view, nil
}

// CreateRefund 登记退款/冲正意图（管理适配接缝；不执行——待 Courier 端点）。
func (l *AdminOrderLogic) CreateRefund(req *types.CreateRefundRequest, authHeader string) (*types.RefundResponse, error) {
	userID, ip := actorFrom(l.ctx)
	role, err := l.adminRole(authHeader)
	if err != nil {
		return nil, err
	}
	action := strings.ToUpper(strings.TrimSpace(req.Action))
	if action == "" {
		action = model.RefundActionRefund
	}
	if action != model.RefundActionRefund && action != model.RefundActionCorrect {
		return nil, newAPIError(http.StatusBadRequest, "退款动作仅支持 REFUND/CORRECTION")
	}
	row, err := l.svcCtx.ProjectionDB.Get(req.OrderId)
	if errors.Is(err, model.ErrOrderProjectionNotFound) {
		return nil, newAPIError(http.StatusNotFound, "订单不存在")
	}
	if err != nil {
		l.Errorf("get order %s for refund failed: %v", req.OrderId, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询订单失败")
	}
	if row.Status != "PAID" && row.Status != "DELIVERED" && row.Status != "CLOSED" {
		return nil, newAPIError(http.StatusConflict, "订单未付款，不可退款")
	}
	amount := req.AmountCents
	if amount <= 0 {
		amount = row.AmountCents
	}
	if amount > row.AmountCents {
		return nil, newAPIError(http.StatusBadRequest, "退款金额不能超过实付")
	}
	refund := &model.RefundRequest{
		OrderId: req.OrderId, Action: action, AmountCents: amount,
		Reason: req.Reason, Status: model.RefundStatusPending,
		Note: "待 Courier 退款端点接入后执行", CreatedBy: userID,
	}
	if err := l.svcCtx.ProjectionDB.CreateRefund(refund); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "Duplicate") {
			return nil, newAPIError(http.StatusConflict, "该订单已登记过该退款动作")
		}
		l.Errorf("create refund %s failed: %v", req.OrderId, err)
		return nil, newAPIError(http.StatusInternalServerError, "登记退款失败")
	}
	detail, _ := json.Marshal(map[string]any{
		"action": action, "amount_cents": amount, "reason": req.Reason,
	})
	_ = l.svcCtx.ProjectionDB.LogAudit(&model.OrderAuditLog{
		OrderId: req.OrderId, ActorId: userID, ActorRole: role, Action: "refund_request", Detail: string(detail), Ip: ip,
	})
	got, _ := l.svcCtx.ProjectionDB.GetRefund(req.OrderId, action)
	return &types.RefundResponse{Refund: toRefundView(got), Execution: "pending-courier-refund-endpoint"}, nil
}

// ListRefunds 退款/冲正意图列表（状态可筛）。
func (l *AdminOrderLogic) ListRefunds(req *types.RefundListRequest, authHeader string) (*types.RefundListResponse, error) {
	userID, ip := actorFrom(l.ctx)
	role, err := l.adminRole(authHeader)
	if err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.ProjectionDB.ListRefunds(req.Status, 100)
	if err != nil {
		l.Errorf("list refunds failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询退款失败")
	}
	_ = l.svcCtx.ProjectionDB.LogAudit(&model.OrderAuditLog{
		ActorId: userID, ActorRole: role, Action: "refund_list", Detail: `{"status":"` + req.Status + `"}`, Ip: ip,
	})
	items := make([]types.RefundView, 0, len(rows))
	for _, row := range rows {
		items = append(items, toRefundView(row))
	}
	return &types.RefundListResponse{Items: items}, nil
}

// ListAudit 审计列表（orderId 可筛）。
func (l *AdminOrderLogic) ListAudit(req *types.AuditListRequest, authHeader string) (*types.AuditListResponse, error) {
	userID, ip := actorFrom(l.ctx)
	role, err := l.adminRole(authHeader)
	if err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.ProjectionDB.ListAudit(req.OrderId, 100)
	if err != nil {
		l.Errorf("list audit failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询审计失败")
	}
	_ = l.svcCtx.ProjectionDB.LogAudit(&model.OrderAuditLog{
		ActorId: userID, ActorRole: role, Action: "audit_list", Detail: `{"order_id":"` + req.OrderId + `"}`, Ip: ip,
	})
	items := make([]types.AuditView, 0, len(rows))
	for _, row := range rows {
		items = append(items, toAuditView(row))
	}
	return &types.AuditListResponse{Items: items}, nil
}

// adminRole 管理面角色守卫（platform_admin/operator），返回角色供审计。
func (l *AdminOrderLogic) adminRole(authHeader string) (string, error) {
	return requireAdmin(l.ctx, l.svcCtx, authHeader)
}

// requireAdmin 管理面角色守卫（platform_admin/operator），返回角色供审计。
// game-catalog /me 权威判定角色，不信任客户端声明。
func requireAdmin(ctx context.Context, svcCtx *svc.ServiceContext, authHeader string) (string, error) {
	role, _, err := platformRole(ctx, svcCtx, authHeader)
	if err != nil {
		return "", newAPIError(http.StatusBadGateway, "角色服务不可用")
	}
	if role != "platform_admin" && role != "operator" {
		return "", newAPIError(http.StatusForbidden, "无权限执行该操作")
	}
	return role, nil
}

func toProjectionView(p *model.OrderProjection) types.OrderProjectionView {
	return types.OrderProjectionView{
		OrderId: p.OrderId, GameId: p.GameId, PartnerId: p.PartnerId, AccountId: p.AccountId, SkuId: p.SkuId,
		ProductId: p.ProductId, Form: p.Form, AmountCents: p.AmountCents, Currency: p.Currency,
		Status: p.Status, PaidAt: p.PaidAt, DeliveredAt: p.DeliveredAt,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func toRefundView(r *model.RefundRequest) types.RefundView {
	return types.RefundView{
		Id: r.Id, OrderId: r.OrderId, Action: r.Action, AmountCents: r.AmountCents,
		Reason: r.Reason, Status: r.Status, Note: r.Note, CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func toAuditView(a *model.OrderAuditLog) types.AuditView {
	return types.AuditView{
		Id: a.Id, OrderId: a.OrderId, ActorId: a.ActorId, ActorRole: a.ActorRole,
		Action: a.Action, Detail: a.Detail, Ip: a.Ip, CreatedAt: a.CreatedAt,
	}
}
