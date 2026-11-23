package logic

// GP-205 厂商对账单：从只读投影聚合实付/优惠承担/退款冲减，算合同分成
// 基数与差异；人工确认 + CSV 导出，先确认不自动打款（F23）。
//
// 事实源仍为 Courier teller（设计 §3.1），本服务仅聚合事实副本；无全局
// 订单/结算 HTTP API（边界 6），对账单由本服务按商户维度生成快照。优惠
// 承担已接（GP-305 券折让账本 coupon_order_links ⋈ order_projection，1dab81a）；
// 退款冲减只计 EXECUTED（执行器已接线 6176ea1，Courier 退款端点部署激活
// 前退款停 PENDING，冲减为 0）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/payments/internal/notify"
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
	if existing != nil && existing.Status == model.StatementStatusDisputed {
		return nil, newAPIError(http.StatusConflict, "该周期对账单存在未处理异议，请先复核处理")
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
	// 优惠承担（GP-305 券折让：coupon_order_links 按已付订单聚合）
	discount, err := l.svcCtx.StatementDB.StatementDiscountAggregate(req.PartnerId, start, end)
	if err != nil {
		l.Errorf("discount aggregate failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "聚合优惠承担失败")
	}
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

// ResolveDispute 运营复核处理异议（GP-404）：清空异议标记并回草稿，
// 由运营复核后改结算单再确认。异议明细（dispute_by/reason）留痕在审计里。
func (l *StatementLogic) ResolveDispute(req *types.StatementPathRequest, authHeader string) (*types.StatementResponse, error) {
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
	if st.Status != model.StatementStatusDisputed {
		return nil, newAPIError(http.StatusConflict, "对账单不存在待处理异议")
	}
	if err := l.svcCtx.StatementDB.ClearDispute(req.Id); err != nil {
		l.Errorf("resolve dispute %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "处理异议失败")
	}
	// GP-404 异议版本化留痕：结案最新一轮未结异议（填复核人/时间）。无未结
	// 轮次（存量单据在留痕上线前提的异议）不视为失败。
	if ok, err := l.svcCtx.StatementDB.ResolveLatestDisputeRound(req.Id, userID); err != nil {
		l.Errorf("resolve dispute round statement %d failed: %v", req.Id, err)
	} else if !ok {
		l.Infof("resolve dispute statement %d: no open dispute round", req.Id)
	}
	detail, _ := json.Marshal(map[string]any{"id": req.Id, "dispute_by": st.DisputeBy, "reason": st.DisputeReason, "diff": st.Diff})
	_ = l.svcCtx.ProjectionDB.LogAudit(&model.OrderAuditLog{
		ActorId: userID, ActorRole: role, Action: "statement_dispute_resolved", Detail: string(detail), Ip: ip,
	})
	// GP-404 异议复核结果回流合作方（站内信）：告知异议提交人复核结论，可
	// 回后台重新确认。fire-and-forget（notify 契约不返回错误），community
	// 未部署时 no-op；不影响复核结果。
	if st.DisputeBy > 0 {
		l.svcCtx.NotifySvc.Publish(notify.Notification{
			UserIds: []int64{st.DisputeBy}, Type: "statement_dispute_resolved", TargetId: req.Id,
			Content: "您的对账单异议已复核，可前往合作方后台重新确认",
		})
	}
	got, _ := l.svcCtx.StatementDB.Get(req.Id)
	return &types.StatementResponse{Statement: toStatementView(got)}, nil
}

// DisputeRounds 异议轮次时间线（GP-404 版本化留痕）：按轮次升序返回全部
// 异议轮次（含未结案），供运营复核差异责任时回溯。仅运营/管理角色。
func (l *StatementLogic) DisputeRounds(req *types.StatementPathRequest, authHeader string) (*types.StatementDisputeRoundsResponse, error) {
	if _, err := requireAdmin(l.ctx, l.svcCtx, authHeader); err != nil {
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
	rounds, err := l.svcCtx.StatementDB.ListDisputeRounds(st.Id)
	if err != nil {
		l.Errorf("list dispute rounds %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询异议轮次失败")
	}
	items := make([]types.StatementDisputeRoundView, 0, len(rounds))
	for _, r := range rounds {
		items = append(items, types.StatementDisputeRoundView{
			Id: r.Id, StatementId: r.StatementId, Round: r.RoundNo,
			DisputeBy: r.DisputeBy, DisputeAt: r.DisputeAt,
			DisputeReason: r.DisputeReason, DiffAtDispute: r.DiffAtDispute,
			ResolvedBy: r.ResolvedBy, ResolvedAt: r.ResolvedAt,
		})
	}
	return &types.StatementDisputeRoundsResponse{
		StatementId: st.Id, Status: st.Status, Items: items,
	}, nil
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
		ConfirmedBy: s.ConfirmedBy, ConfirmedAt: s.ConfirmedAt,
		DisputeBy: s.DisputeBy, DisputeAt: s.DisputeAt, DisputeReason: s.DisputeReason,
		CreatedBy: s.CreatedBy,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}
