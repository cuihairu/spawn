package logic

// GP-404 合作方自助账单确认与结算差异处理（F12/F23）：
// 合作方（partner 角色 + game-catalog 绑定的 partner_id）只能看到/操作
// 本人名下（vendor_statements.partner_id 相等）的对账单——行级隔离，
// 不是本人单据一律 404（避免跨合作方枚举）。
//
// 权限隔离随功能验收：角色权威在 game-catalog /me（requirePartnerRole），
// 本服务不复制角色数据；导出走同一 partner 作用域。

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

// PartnerStatementLogic 合作方自助账单面。
type PartnerStatementLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPartnerStatementLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PartnerStatementLogic {
	return &PartnerStatementLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// List 本人名下对账单列表（vendor/status 可筛；不含别的合作方单据）。
func (l *PartnerStatementLogic) List(req *types.PartnerStatementListRequest, authHeader string) (*types.StatementListResponse, error) {
	userID, ip := actorFrom(l.ctx)
	partnerID, err := requirePartnerRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.StatementDB.ListStatements(strings.TrimSpace(req.Vendor), partnerID, strings.TrimSpace(req.Status), 50, 0)
	if err != nil {
		l.Errorf("partner list statements failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询对账单失败")
	}
	detail, _ := json.Marshal(map[string]any{"partner_id": partnerID, "vendor": req.Vendor, "status": req.Status})
	_ = l.svcCtx.ProjectionDB.LogAudit(&model.OrderAuditLog{
		ActorId: userID, ActorRole: "partner", Action: "partner_statement_list", Detail: string(detail), Ip: ip,
	})
	items := make([]types.StatementView, 0, len(rows))
	for _, r := range rows {
		items = append(items, toStatementView(r))
	}
	return &types.StatementListResponse{Items: items}, nil
}

// Get 本人名下对账单详情（跨合作方 id 一律 404）。
func (l *PartnerStatementLogic) Get(req *types.StatementPathRequest, authHeader string) (*types.StatementResponse, error) {
	partnerID, err := requirePartnerRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	st, err := l.svcCtx.StatementDB.GetForPartner(req.Id, partnerID)
	if errors.Is(err, model.ErrStatementNotFound) {
		return nil, newAPIError(http.StatusNotFound, "对账单不存在")
	}
	if err != nil {
		l.Errorf("partner get statement %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询对账单失败")
	}
	return &types.StatementResponse{Statement: toStatementView(st)}, nil
}

// Export 导出本人名下对账单 CSV（明细 + 汇总），作用域同 Get。
func (l *PartnerStatementLogic) Export(req *types.StatementPathRequest, authHeader string) (string, []byte, error) {
	userID, ip := actorFrom(l.ctx)
	partnerID, err := requirePartnerRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return "", nil, err
	}
	st, err := l.svcCtx.StatementDB.GetForPartner(req.Id, partnerID)
	if errors.Is(err, model.ErrStatementNotFound) {
		return "", nil, newAPIError(http.StatusNotFound, "对账单不存在")
	}
	if err != nil {
		l.Errorf("partner get statement %d failed: %v", req.Id, err)
		return "", nil, newAPIError(http.StatusInternalServerError, "查询对账单失败")
	}
	items, err := l.svcCtx.StatementDB.StatementLineItems(st.PartnerId, st.PeriodStart, st.PeriodEnd)
	if err != nil {
		l.Errorf("partner line items %d failed: %v", req.Id, err)
		return "", nil, newAPIError(http.StatusInternalServerError, "读取明细失败")
	}
	var b strings.Builder
	b.WriteString("order_id,game_id,account_id,sku_id,status,amount_cents,paid_at\n")
	for _, it := range items {
		fmt.Fprintf(&b, "%s,%s,%s,%s,%s,%d,%s\n", it.OrderId, it.GameId, it.AccountId, it.SkuId, it.Status, it.AmountCents, it.PaidAt)
	}
	fmt.Fprintf(&b, "summary,paid_total=%d,discount_total=%d,refund_total=%d,split_base=%d,split_amount=%d,settlement=%d,diff=%d,order_count=%d\n",
		st.PaidTotal, st.DiscountTotal, st.RefundTotal, st.SplitBase, st.SplitAmount, st.Settlement, st.Diff, st.OrderCount)
	detail, _ := json.Marshal(map[string]any{"id": req.Id, "partner_id": partnerID, "rows": len(items)})
	_ = l.svcCtx.ProjectionDB.LogAudit(&model.OrderAuditLog{
		ActorId: userID, ActorRole: "partner", Action: "partner_statement_export", Detail: string(detail), Ip: ip,
	})
	filename := fmt.Sprintf("partner-statement-%d-%d.csv", st.PartnerId, st.Id)
	return filename, []byte(b.String()), nil
}

// Confirm 合作方账单确认（F23：先确认不自动打款）。仅草稿可确认；
// 已异议（disputed）的单据须由运营复核后确认，合作方不可自证异议成立。
func (l *PartnerStatementLogic) Confirm(req *types.StatementPathRequest, authHeader string) (*types.StatementResponse, error) {
	userID, ip := actorFrom(l.ctx)
	partnerID, err := requirePartnerRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	st, err := l.svcCtx.StatementDB.GetForPartner(req.Id, partnerID)
	if errors.Is(err, model.ErrStatementNotFound) {
		return nil, newAPIError(http.StatusNotFound, "对账单不存在")
	}
	if err != nil {
		l.Errorf("partner get statement %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询对账单失败")
	}
	switch st.Status {
	case model.StatementStatusConfirmed:
		return nil, newAPIError(http.StatusConflict, "对账单已确认")
	case model.StatementStatusDisputed:
		return nil, newAPIError(http.StatusConflict, "对账单存在未处理异议，请联系运营复核")
	}
	if err := l.svcCtx.StatementDB.ConfirmStatement(req.Id, userID); err != nil {
		l.Errorf("partner confirm statement %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "确认对账单失败")
	}
	detail, _ := json.Marshal(map[string]any{"id": req.Id, "partner_id": partnerID, "split_amount": st.SplitAmount, "diff": st.Diff})
	_ = l.svcCtx.ProjectionDB.LogAudit(&model.OrderAuditLog{
		ActorId: userID, ActorRole: "partner", Action: "partner_statement_confirm", Detail: string(detail), Ip: ip,
	})
	got, _ := l.svcCtx.StatementDB.GetForPartner(req.Id, partnerID)
	return &types.StatementResponse{Statement: toStatementView(got)}, nil
}

// Dispute 合作方对结算差异提出异议（GP-404 差异处理）：仅当存在差异
// （diff ≠ 0）时才可提，理由必填留痕；异议后单据转 disputed，须运营复核。
func (l *PartnerStatementLogic) Dispute(req *types.DisputeStatementRequest, authHeader string) (*types.StatementResponse, error) {
	userID, ip := actorFrom(l.ctx)
	partnerID, err := requirePartnerRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(req.Reason)
	if len(reason) < 2 {
		return nil, newAPIError(http.StatusBadRequest, "异议理由不能为空")
	}
	if len(reason) > 256 {
		return nil, newAPIError(http.StatusBadRequest, "异议理由过长")
	}
	st, err := l.svcCtx.StatementDB.GetForPartner(req.Id, partnerID)
	if errors.Is(err, model.ErrStatementNotFound) {
		return nil, newAPIError(http.StatusNotFound, "对账单不存在")
	}
	if err != nil {
		l.Errorf("partner get statement %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "查询对账单失败")
	}
	if st.Status == model.StatementStatusConfirmed {
		return nil, newAPIError(http.StatusConflict, "对账单已确认，不可再提异议")
	}
	if st.Diff == 0 {
		return nil, newAPIError(http.StatusConflict, "该周期无结算差异，无需提异议")
	}
	if err := l.svcCtx.StatementDB.DisputeStatement(req.Id, userID, reason); err != nil {
		l.Errorf("partner dispute statement %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "提异议失败")
	}
	detail, _ := json.Marshal(map[string]any{"id": req.Id, "partner_id": partnerID, "diff": st.Diff, "reason": reason})
	_ = l.svcCtx.ProjectionDB.LogAudit(&model.OrderAuditLog{
		ActorId: userID, ActorRole: "partner", Action: "partner_statement_dispute", Detail: string(detail), Ip: ip,
	})
	got, _ := l.svcCtx.StatementDB.GetForPartner(req.Id, partnerID)
	return &types.StatementResponse{Statement: toStatementView(got)}, nil
}

// Differences 结算差异清单（本人名下 diff ≠ 0 的单据，按差异绝对值降序）——
// 合作方侧自查入口，与运营侧 Get/List 同源同口径。
func (l *PartnerStatementLogic) Differences(req *types.StatementListRequest, authHeader string) (*types.StatementListResponse, error) {
	partnerID, err := requirePartnerRole(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.StatementDB.ListDifferences(partnerID, strings.TrimSpace(req.Vendor), 50)
	if err != nil {
		l.Errorf("partner list differences failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询结算差异失败")
	}
	items := make([]types.StatementView, 0, len(rows))
	for _, r := range rows {
		items = append(items, toStatementView(r))
	}
	return &types.StatementListResponse{Items: items}, nil
}
