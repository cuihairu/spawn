package logic

// GP-205 厂商对账单：从只读投影聚合实付/优惠承担/退款冲减，算合同分成
// 基数与差异；人工确认 + CSV 导出，先确认不自动打款（F23）。
//
// 事实源仍为 Courier teller（设计 §3.1），本服务仅聚合事实副本；无全局
// 订单/结算 HTTP API（边界 6），对账单由本服务按商户维度生成快照。优惠
// 承担暂 0（优惠券子系统未接，GP-305）；退款冲减只计 EXECUTED（执行器待
// Courier 退款端点提案）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/internal/types"
	"github.com/tappi/tappi/services/payments/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// StatementLogic 厂商对账单管理面。
type StatementLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewStatementLogic(ctx context.Context, svcCtx *svc.ServiceContext) *StatementLogic {
	return &StatementLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Generate 生成对账单快照（同 key 重生成更新草稿；已确认 409）。
func (l *StatementLogic) Generate(req *types.GenerateStatementRequest, authHeader string) (*types.StatementResponse, error) {
	userID, ip := actorFrom(l.ctx)
	role, err := requireAdmin(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	vendor := strings.TrimSpace(req.Vendor)
	if vendor == "" {
		return nil, newAPIError(http.StatusBadRequest, "厂商标识不能为空")
	}
	if req.PartnerId <= 0 {
		return nil, newAPIError(http.StatusBadRequest, "商户归属必须为正整数")
	}
	if req.SplitRateBp < 0 || req.SplitRateBp > 10000 {
		return nil, newAPIError(http.StatusBadRequest, "分成比例基点需在 0~10000")
	}
	start, end := strings.TrimSpace(req.PeriodStart), strings.TrimSpace(req.PeriodEnd)
	if start == "" || end == "" || start > end {
		return nil, newAPIError(http.StatusBadRequest, "周期不合法")
	}
	existing, err := l.svcCtx.StatementDB.GetByKey(vendor, req.PartnerId, start, end)
	if err != nil && !errors.Is(err, model.ErrStatementNotFound) {
		l.Errorf("get statement key failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询对账单失败")
	}
	if existing != nil && existing.Status == model.StatementStatusConfirmed {
		return nil, newAPIError(http.StatusConflict, "该周期对账单已确认，不可重生成")
	}

	paid, err := l.svcCtx.StatementDB.StatementPaidAggregate(req.PartnerId, start, end)
	if err != nil {
		l.Errorf("paid aggregate failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "聚合实付失败")
	}
	refund, err := l.svcCtx.StatementDB.StatementRefundAggregate(req.PartnerId, start, end)
	if err != nil {
		l.Errorf("refund aggregate failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "聚合退款冲减失败")
	}
	// 优惠承担 = 0（优惠券子系统未接，GP-305 接入后自订单快照计）
	discount := int64(0)
	base := paid.Amount - discount - refund
	split := base * req.SplitRateBp / 10000
	settlement := int64(0)
	if existing != nil {
		settlement = existing.Settlement // 重生成保留已人工录入的结算单
	}
	st := &model.VendorStatement{
		Vendor: vendor, PartnerId: req.PartnerId, PeriodStart: start, PeriodEnd: end,
		PaidTotal: paid.Amount, DiscountTotal: discount, RefundTotal: refund,
		SplitBase: base, SplitRateBp: req.SplitRateBp, SplitAmount: split,
		Settlement: settlement, Diff: base - settlement, OrderCount: paid.Count,
		Status: model.StatementStatusDraft, CreatedBy: userID,
	}
	if err := l.svcCtx.StatementDB.UpsertStatement(st); err != nil {
		l.Errorf("upsert statement failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "保存对账单失败")
	}
	got, err := l.svcCtx.StatementDB.GetByKey(vendor, req.PartnerId, start, end)
	if err != nil {
		l.Errorf("reload statement failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "读取对账单失败")
	}
	detail, _ := json.Marshal(map[string]any{
		"vendor": vendor, "partner_id": req.PartnerId, "period": start + "~" + end,
		"paid": paid.Amount, "discount": discount, "refund": refund, "split_base": base,
		"split_rate_bp": req.SplitRateBp, "split_amount": split,
	})
	_ = l.svcCtx.ProjectionDB.LogAudit(&model.OrderAuditLog{
		ActorId: userID, ActorRole: role, Action: "statement_generate", Detail: string(detail), Ip: ip,
	})
	return &types.StatementResponse{Statement: toStatementView(got)}, nil
}

// List 对账单列表（vendor/partner/status 可筛）。
func (l *StatementLogic) List(req *types.StatementListRequest, authHeader string) (*types.StatementListResponse, error) {
	userID, ip := actorFrom(l.ctx)
	role, err := requireAdmin(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.StatementDB.ListStatements(req.Vendor, req.PartnerId, req.Status, 50, 0)
	if err != nil {
		l.Errorf("list statements failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询对账单失败")
	}
	detail, _ := json.Marshal(map[string]string{"vendor": req.Vendor, "status": req.Status})
	_ = l.svcCtx.ProjectionDB.LogAudit(&model.OrderAuditLog{
		ActorId: userID, ActorRole: role, Action: "statement_list", Detail: string(detail), Ip: ip,
	})
	items := make([]types.StatementView, 0, len(rows))
	for _, r := range rows {
		items = append(items, toStatementView(r))
	}
	return &types.StatementListResponse{Items: items}, nil
}

// Get 对账单详情。
func (l *StatementLogic) Get(req *types.StatementPathRequest, authHeader string) (*types.StatementResponse, error) {
	_, err := requireAdmin(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	st, err := l.svcCtx.StatementDB.Get(req.Id)
	if errors.Is(err, model.ErrStatementNotFound) {
		return nil, newAPIError(http.StatusNotFound, "对账单不存在")
	}
	if err != nil {
		l.Errorf("get statement %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询对账单失败")
	}
	return &types.StatementResponse{Statement: toStatementView(st)}, nil
}

// UpdateSettlement 人工录入渠道结算单金额并重算差异（确认后不可改）。
func (l *StatementLogic) UpdateSettlement(req *types.UpdateSettlementRequest, authHeader string) (*types.StatementResponse, error) {
	userID, ip := actorFrom(l.ctx)
	role, err := requireAdmin(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	if req.Settlement < 0 {
		return nil, newAPIError(http.StatusBadRequest, "结算单金额不能为负")
	}
	st, err := l.svcCtx.StatementDB.Get(req.Id)
	if errors.Is(err, model.ErrStatementNotFound) {
		return nil, newAPIError(http.StatusNotFound, "对账单不存在")
	}
	if err != nil {
		l.Errorf("get statement %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询对账单失败")
	}
	if st.Status == model.StatementStatusConfirmed {
		return nil, newAPIError(http.StatusConflict, "对账单已确认，不可改结算单")
	}
	if err := l.svcCtx.StatementDB.UpdateSettlement(req.Id, req.Settlement, st.SplitBase-req.Settlement); err != nil {
		l.Errorf("update settlement %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "更新结算单失败")
	}
	detail, _ := json.Marshal(map[string]any{"id": req.Id, "settlement": req.Settlement, "diff": st.SplitBase - req.Settlement})
	_ = l.svcCtx.ProjectionDB.LogAudit(&model.OrderAuditLog{
		ActorId: userID, ActorRole: role, Action: "statement_settlement", Detail: string(detail), Ip: ip,
	})
	got, _ := l.svcCtx.StatementDB.Get(req.Id)
	return &types.StatementResponse{Statement: toStatementView(got)}, nil
}

// Confirm 人工确认（先确认不自动打款）。
func (l *StatementLogic) Confirm(req *types.StatementPathRequest, authHeader string) (*types.StatementResponse, error) {
	userID, ip := actorFrom(l.ctx)
	role, err := requireAdmin(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	st, err := l.svcCtx.StatementDB.Get(req.Id)
	if errors.Is(err, model.ErrStatementNotFound) {
		return nil, newAPIError(http.StatusNotFound, "对账单不存在")
	}
	if err != nil {
		l.Errorf("get statement %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询对账单失败")
	}
	if st.Status == model.StatementStatusConfirmed {
		return nil, newAPIError(http.StatusConflict, "对账单已确认")
	}
	if err := l.svcCtx.StatementDB.ConfirmStatement(req.Id, userID); err != nil {
		l.Errorf("confirm statement %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "确认对账单失败")
	}
	detail, _ := json.Marshal(map[string]any{"id": req.Id, "split_amount": st.SplitAmount, "diff": st.Diff})
	_ = l.svcCtx.ProjectionDB.LogAudit(&model.OrderAuditLog{
		ActorId: userID, ActorRole: role, Action: "statement_confirm", Detail: string(detail), Ip: ip,
	})
	got, _ := l.svcCtx.StatementDB.Get(req.Id)
	return &types.StatementResponse{Statement: toStatementView(got)}, nil
}

// Export 导出对账单 CSV（明细 + 汇总）；返回文件名与内容。
func (l *StatementLogic) Export(req *types.StatementPathRequest, authHeader string) (string, []byte, error) {
	userID, ip := actorFrom(l.ctx)
	role, err := requireAdmin(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return "", nil, err
	}
	st, err := l.svcCtx.StatementDB.Get(req.Id)
	if errors.Is(err, model.ErrStatementNotFound) {
		return "", nil, newAPIError(http.StatusNotFound, "对账单不存在")
	}
	if err != nil {
		l.Errorf("get statement %d failed: %v", req.Id, err)
		return "", nil, newAPIError(http.StatusInternalServerError, "查询对账单失败")
	}
	items, err := l.svcCtx.StatementDB.StatementLineItems(st.PartnerId, st.PeriodStart, st.PeriodEnd)
	if err != nil {
		l.Errorf("line items %d failed: %v", req.Id, err)
		return "", nil, newAPIError(http.StatusInternalServerError, "读取明细失败")
	}
	var b strings.Builder
	b.WriteString("order_id,game_id,account_id,sku_id,status,amount_cents,paid_at\n")
	for _, it := range items {
		fmt.Fprintf(&b, "%s,%s,%s,%s,%s,%d,%s\n", it.OrderId, it.GameId, it.AccountId, it.SkuId, it.Status, it.AmountCents, it.PaidAt)
	}
	fmt.Fprintf(&b, "summary,paid_total=%d,discount_total=%d,refund_total=%d,split_base=%d,split_amount=%d,settlement=%d,diff=%d,order_count=%d\n",
		st.PaidTotal, st.DiscountTotal, st.RefundTotal, st.SplitBase, st.SplitAmount, st.Settlement, st.Diff, st.OrderCount)
	detail, _ := json.Marshal(map[string]any{"id": req.Id, "rows": len(items)})
	_ = l.svcCtx.ProjectionDB.LogAudit(&model.OrderAuditLog{
		ActorId: userID, ActorRole: role, Action: "statement_export", Detail: string(detail), Ip: ip,
	})
	filename := fmt.Sprintf("statement-%d-%d.csv", st.PartnerId, st.Id)
	return filename, []byte(b.String()), nil
}

func toStatementView(s *model.VendorStatement) types.StatementView {
	return types.StatementView{
		Id: s.Id, Vendor: s.Vendor, PartnerId: s.PartnerId, PeriodStart: s.PeriodStart, PeriodEnd: s.PeriodEnd,
		PaidTotal: s.PaidTotal, DiscountTotal: s.DiscountTotal, RefundTotal: s.RefundTotal,
		SplitBase: s.SplitBase, SplitRateBp: s.SplitRateBp, SplitAmount: s.SplitAmount,
		Settlement: s.Settlement, Diff: s.Diff, OrderCount: s.OrderCount, Status: s.Status,
		ConfirmedBy: s.ConfirmedBy, ConfirmedAt: s.ConfirmedAt, CreatedBy: s.CreatedBy,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}
