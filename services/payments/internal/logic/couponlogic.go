package logic

// GP-305 优惠券逻辑：管理面（券定义/状态机）+ 玩家侧（领取/我的券）+
// 下单报价（ApplyCoupons：门槛/折扣/叠加校验 + 占位锁 + 核销关联账本）。
// 金额一律服务端计算，客户端仅提交 sku_id + coupon_ids（不提交可信金额）。
//
// 券状态机：claimed → locked（下单占位，防并发双用）→ used（PAID 核销）→
// claimed（退款 EXECUTED 返还，见 MarkRefundExecuted 钩子）。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/internal/types"
	"github.com/tappi/tappi/services/payments/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// CouponLogic 优惠券逻辑。
type CouponLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCouponLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CouponLogic {
	return &CouponLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func validateCouponConfig(cfg *types.CouponConfig) error {
	if strings.TrimSpace(cfg.Code) == "" || strings.TrimSpace(cfg.Name) == "" {
		return newAPIError(http.StatusBadRequest, "券码/名称不能为空")
	}
	if strings.TrimSpace(cfg.GameId) == "" {
		return newAPIError(http.StatusBadRequest, "游戏归属不能为空")
	}
	if cfg.DiscountType != model.CouponDiscountFixed && cfg.DiscountType != model.CouponDiscountPercentBp {
		return newAPIError(http.StatusBadRequest, "折扣类型必须为 fixed 或 percent_bp")
	}
	if cfg.DiscountValue <= 0 {
		return newAPIError(http.StatusBadRequest, "折扣值必须为正")
	}
	if cfg.DiscountType == model.CouponDiscountPercentBp && cfg.DiscountValue > 10000 {
		return newAPIError(http.StatusBadRequest, "百分比折扣不能超过 100%")
	}
	if cfg.ThresholdCents < 0 {
		return newAPIError(http.StatusBadRequest, "门槛不能为负")
	}
	if cfg.PerUserLimit <= 0 {
		return newAPIError(http.StatusBadRequest, "per_user_limit 必须为正整数")
	}
	if cfg.StartAt == "" || cfg.EndAt == "" {
		return newAPIError(http.StatusBadRequest, "起止时间不能为空")
	}
	if cfg.StartAt > cfg.EndAt {
		return newAPIError(http.StatusBadRequest, "开始时间不能晚于结束时间")
	}
	return nil
}

func (l *CouponLogic) Create(req *types.CreateCouponRequest, authHeader string) (*types.CouponResponse, error) {
	userID, _ := actorFrom(l.ctx)
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	if err := validateCouponConfig(&req.CouponConfig); err != nil {
		return nil, err
	}
	c := &model.Coupon{
		Code: req.Code, Name: req.Name, GameId: req.GameId, SkuId: req.SkuId,
		ThresholdCents: req.ThresholdCents, DiscountType: req.DiscountType,
		DiscountValue: req.DiscountValue, Stackable: req.Stackable,
		PerUserLimit: req.PerUserLimit, TotalCount: req.TotalCount,
		StartAt: req.StartAt, EndAt: req.EndAt,
		Status: model.CouponStatusDraft, CreatedBy: userID,
	}
	if err := l.svcCtx.CouponDB.CreateCoupon(c); err != nil {
		l.Errorf("create coupon failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "创建优惠券失败")
	}
	got, _ := l.svcCtx.CouponDB.GetCoupon(c.Id)
	return &types.CouponResponse{Coupon: toCouponView(got, 0)}, nil
}

func (l *CouponLogic) Update(req *types.UpdateCouponRequest, authHeader string) (*types.CouponResponse, error) {
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	if err := validateCouponConfig(&req.CouponConfig); err != nil {
		return nil, err
	}
	existing, err := l.svcCtx.CouponDB.GetCoupon(req.Id)
	if errors.Is(err, model.ErrCouponNotFound) {
		return nil, newAPIError(http.StatusNotFound, "优惠券不存在")
	}
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询优惠券失败")
	}
	if existing.Status != model.CouponStatusDraft {
		return nil, newAPIError(http.StatusConflict, "仅草稿可编辑（已发布请先 disable 或 revise）")
	}
	c := &model.Coupon{
		Id: req.Id, Code: req.Code, Name: req.Name, GameId: req.GameId, SkuId: req.SkuId,
		ThresholdCents: req.ThresholdCents, DiscountType: req.DiscountType,
		DiscountValue: req.DiscountValue, Stackable: req.Stackable,
		PerUserLimit: req.PerUserLimit, TotalCount: req.TotalCount,
		StartAt: req.StartAt, EndAt: req.EndAt,
	}
	if err := l.svcCtx.CouponDB.UpdateCoupon(c); err != nil {
		l.Errorf("update coupon %d failed: %v", req.Id, err)
		return nil, newAPIError(http.StatusInternalServerError, "更新优惠券失败")
	}
	got, _ := l.svcCtx.CouponDB.GetCoupon(req.Id)
	return &types.CouponResponse{Coupon: toCouponView(got, 0)}, nil
}

func (l *CouponLogic) Publish(req *types.CouponPathRequest, authHeader string) (*types.CouponResponse, error) {
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	c, err := l.svcCtx.CouponDB.GetCoupon(req.Id)
	if errors.Is(err, model.ErrCouponNotFound) {
		return nil, newAPIError(http.StatusNotFound, "优惠券不存在")
	}
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询优惠券失败")
	}
	if c.Status != model.CouponStatusDraft {
		return nil, newAPIError(http.StatusConflict, "仅草稿可发布")
	}
	if err := l.svcCtx.CouponDB.SetCouponStatus(req.Id, model.CouponStatusPublished); err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "发布优惠券失败")
	}
	got, _ := l.svcCtx.CouponDB.GetCoupon(req.Id)
	used, _ := l.svcCtx.CouponDB.CountCouponUsed(req.Id)
	return &types.CouponResponse{Coupon: toCouponView(got, used)}, nil
}

func (l *CouponLogic) Disable(req *types.CouponPathRequest, authHeader string) (*types.CouponResponse, error) {
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	c, err := l.svcCtx.CouponDB.GetCoupon(req.Id)
	if errors.Is(err, model.ErrCouponNotFound) {
		return nil, newAPIError(http.StatusNotFound, "优惠券不存在")
	}
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询优惠券失败")
	}
	if c.Status != model.CouponStatusPublished {
		return nil, newAPIError(http.StatusConflict, "仅已发布优惠券可停用")
	}
	if err := l.svcCtx.CouponDB.SetCouponStatus(req.Id, model.CouponStatusDisabled); err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "停用优惠券失败")
	}
	got, _ := l.svcCtx.CouponDB.GetCoupon(req.Id)
	used, _ := l.svcCtx.CouponDB.CountCouponUsed(req.Id)
	return &types.CouponResponse{Coupon: toCouponView(got, used)}, nil
}

func (l *CouponLogic) Revise(req *types.CouponPathRequest, authHeader string) (*types.CouponResponse, error) {
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	c, err := l.svcCtx.CouponDB.GetCoupon(req.Id)
	if errors.Is(err, model.ErrCouponNotFound) {
		return nil, newAPIError(http.StatusNotFound, "优惠券不存在")
	}
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询优惠券失败")
	}
	if c.Status == model.CouponStatusDraft {
		return nil, newAPIError(http.StatusConflict, "已是草稿")
	}
	if err := l.svcCtx.CouponDB.SetCouponStatus(req.Id, model.CouponStatusDraft); err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "回草稿失败")
	}
	got, _ := l.svcCtx.CouponDB.GetCoupon(req.Id)
	used, _ := l.svcCtx.CouponDB.CountCouponUsed(req.Id)
	return &types.CouponResponse{Coupon: toCouponView(got, used)}, nil
}

func (l *CouponLogic) List(req *types.CouponListRequest, authHeader string) (*types.CouponListResponse, error) {
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.CouponDB.ListCoupons(req.GameId, req.Status, 100)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询优惠券失败")
	}
	items := make([]types.CouponView, 0, len(rows))
	for _, c := range rows {
		used, _ := l.svcCtx.CouponDB.CountCouponUsed(c.Id)
		items = append(items, toCouponView(c, used))
	}
	return &types.CouponListResponse{Items: items}, nil
}

func (l *CouponLogic) Get(req *types.CouponPathRequest, authHeader string) (*types.CouponResponse, error) {
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	c, err := l.svcCtx.CouponDB.GetCoupon(req.Id)
	if errors.Is(err, model.ErrCouponNotFound) {
		return nil, newAPIError(http.StatusNotFound, "优惠券不存在")
	}
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询优惠券失败")
	}
	used, _ := l.svcCtx.CouponDB.CountCouponUsed(req.Id)
	return &types.CouponResponse{Coupon: toCouponView(c, used)}, nil
}

// Claim 玩家领取（幂等：同 request_id 返回既有；per_user_limit 限领）。
func (l *CouponLogic) Claim(req *types.ClaimCouponRequest) (*types.ClaimCouponResponse, error) {
	userID, _ := actorFrom(l.ctx)
	if userID <= 0 {
		return nil, newAPIError(http.StatusUnauthorized, "未登录")
	}
	req.RequestId = strings.TrimSpace(req.RequestId)
	if req.RequestId == "" {
		return nil, newAPIError(http.StatusBadRequest, "request_id 幂等键不能为空")
	}
	c, err := l.svcCtx.CouponDB.GetCoupon(req.Id)
	if errors.Is(err, model.ErrCouponNotFound) {
		return nil, newAPIError(http.StatusNotFound, "优惠券不存在")
	}
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询优惠券失败")
	}
	if c.Status != model.CouponStatusPublished {
		return nil, newAPIError(http.StatusConflict, "优惠券未上架或已下架")
	}
	if l.now() < c.StartAt || l.now() > c.EndAt {
		return nil, newAPIError(http.StatusConflict, "优惠券不在有效期内")
	}
	uc, created, err := l.svcCtx.CouponDB.ClaimCoupon(&model.UserCoupon{
		CouponId: c.Id, UserId: userID, RequestId: req.RequestId,
	})
	if err != nil {
		l.Errorf("claim coupon %d user %d failed: %v", c.Id, userID, err)
		return nil, newAPIError(http.StatusInternalServerError, "领取优惠券失败")
	}
	// 仅新插入才校验限领（幂等重放返回既有，不重复计数）
	if created {
		if err := l.ensureClaimLimit(c, userID); err != nil {
			return nil, err
		}
	}
	return &types.ClaimCouponResponse{Coupon: toUserCouponView(uc, c)}, nil
}

func (l *CouponLogic) ensureClaimLimit(c *model.Coupon, userID int64) error {
	// 名额仅计 claimed（P0 修复）：locked 是下单占位瞬时态（超时释放回
	// claimed 仍受约束），锁券不新增持有，计入会让占位吃掉领取名额。
	n, err := l.svcCtx.CouponDB.CountUserCouponClaimed(c.Id, userID)
	if err != nil {
		return newAPIError(http.StatusInternalServerError, "限领查询失败")
	}
	if n > c.PerUserLimit {
		return newAPIError(http.StatusConflict, "已达领取上限")
	}
	return nil
}

// MyCoupons 我的券（status 可筛）。
func (l *CouponLogic) MyCoupons(status string) (*types.MyCouponsResponse, error) {
	userID, _ := actorFrom(l.ctx)
	if userID <= 0 {
		return nil, newAPIError(http.StatusUnauthorized, "未登录")
	}
	rows, err := l.svcCtx.CouponDB.ListUserCoupons(userID, status, 100)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "查询我的券失败")
	}
	items := make([]types.UserCouponView, 0, len(rows))
	for _, uc := range rows {
		c, err := l.svcCtx.CouponDB.GetCoupon(uc.CouponId)
		if err != nil {
			continue
		}
		items = append(items, toUserCouponView(uc, c))
	}
	return &types.MyCouponsResponse{Items: items}, nil
}

// ApplyCoupons 下单报价：校验门槛/折扣/叠加/归属/占用，锁券并落核销关联账本。
// 返回总折让与已应用券列表。并发安全：条件 UPDATE（claimed→locked）保证同一
// 券只被一张订单占用；部分失败回滚已锁券。金额全服务端计算。
func (l *CouponLogic) ApplyCoupons(orderID, gameID, skuID string, userID int64, listPrice int64, couponIDs []int64) (int64, []types.CouponApplied, error) {
	if len(couponIDs) == 0 {
		return 0, nil, nil
	}
	// 去重保序
	seen := map[int64]bool{}
	ids := make([]int64, 0, len(couponIDs))
	for _, id := range couponIDs {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return 0, nil, nil
	}
	now := l.now()
	type item struct {
		coupon *model.Coupon
		uc     *model.UserCoupon
		disc   int64
	}
	items := make([]item, 0, len(ids))
	total := int64(0)
	// 预校验（不锁状态，先快照）
	for _, id := range ids {
		c, err := l.svcCtx.CouponDB.GetCoupon(id)
		if errors.Is(err, model.ErrCouponNotFound) {
			return 0, nil, newAPIError(http.StatusBadRequest, "优惠券不存在")
		}
		if err != nil {
			return 0, nil, newAPIError(http.StatusInternalServerError, "查询优惠券失败")
		}
		if c.Status != model.CouponStatusPublished {
			return 0, nil, newAPIError(http.StatusConflict, "优惠券未上架或已下架")
		}
		if now < c.StartAt || now > c.EndAt {
			return 0, nil, newAPIError(http.StatusConflict, "优惠券不在有效期内")
		}
		if c.GameId != gameID {
			return 0, nil, newAPIError(http.StatusBadRequest, "优惠券不属于该游戏")
		}
		if c.SkuId != "" && c.SkuId != skuID {
			return 0, nil, newAPIError(http.StatusBadRequest, "优惠券不适用于该商品")
		}
		if listPrice < c.ThresholdCents {
			return 0, nil, newAPIError(http.StatusBadRequest, "未达优惠券使用门槛")
		}
		// 按 (券, 用户, claimed) 取本人实例：他人持有的不可见（400 未领取，
		// 不泄露他人持有），本人持有但全部锁定/已用 → 409。
		uc, err := l.svcCtx.CouponDB.GetClaimedUserCoupon(id, userID)
		if errors.Is(err, model.ErrUserCouponNotFound) {
			if held, _ := l.svcCtx.CouponDB.CountUserCouponClaims(id, userID); held > 0 {
				return 0, nil, newAPIError(http.StatusConflict, "优惠券已被占用或已使用")
			}
			return 0, nil, newAPIError(http.StatusBadRequest, "优惠券未领取")
		}
		if err != nil {
			return 0, nil, newAPIError(http.StatusInternalServerError, "查询已领券失败")
		}
		disc := couponDiscount(c, listPrice)
		items = append(items, item{coupon: c, uc: uc, disc: disc})
		total += disc
	}
	if len(items) > 1 {
		for _, it := range items {
			if !it.coupon.Stackable {
				return 0, nil, newAPIError(http.StatusBadRequest, "不可叠加优惠券不能与其他券同用")
			}
		}
	}
	if total > listPrice {
		total = listPrice
	}
	// 使用预算：每券 used < total_count（total_count=0 不限）
	for _, it := range items {
		if it.coupon.TotalCount > 0 {
			used, err := l.svcCtx.CouponDB.CountCouponUsed(it.coupon.Id)
			if err != nil {
				return 0, nil, newAPIError(http.StatusInternalServerError, "查询券使用量失败")
			}
			if used >= it.coupon.TotalCount {
				return 0, nil, newAPIError(http.StatusConflict, "优惠券已发完")
			}
		}
	}
	// 锁券（并发安全）+ 落核销关联；失败回滚已锁
	locked := make([]int64, 0, len(items))
	for _, it := range items {
		if err := l.svcCtx.CouponDB.LockUserCoupon(it.uc.Id, orderID); err != nil {
			for _, id := range locked {
				_ = l.svcCtx.CouponDB.UnlockUserCoupon(id)
			}
			return 0, nil, newAPIError(http.StatusConflict, "优惠券已被其他订单占用，请重试")
		}
		locked = append(locked, it.uc.Id)
	}
	applied := make([]types.CouponApplied, 0, len(items))
	for i, it := range items {
		snap, _ := json.Marshal(map[string]any{
			"code": it.coupon.Code, "name": it.coupon.Name,
			"discount_type": it.coupon.DiscountType, "discount_value": it.coupon.DiscountValue,
			"threshold_cents": it.coupon.ThresholdCents, "list_price": listPrice,
			"discount_cents": it.disc,
		})
		if err := l.svcCtx.CouponDB.LinkOrderCoupon(&model.CouponOrderLink{
			OrderId: orderID, CouponId: it.coupon.Id, UserCouponId: it.uc.Id,
			DiscountCents: it.disc, Snapshot: string(snap),
		}); err != nil {
			l.Errorf("link coupon order=%s coupon=%d failed: %v", orderID, it.coupon.Id, err)
		}
		applied = append(applied, types.CouponApplied{
			CouponId: it.coupon.Id, Code: it.coupon.Code, DiscountCents: it.disc,
		})
		_ = i
	}
	return total, applied, nil
}

// RedeemOrderCoupons PAID 核销：订单关联的 locked 券 → used（幂等）。
func (l *CouponLogic) RedeemOrderCoupons(orderID string) error {
	return l.svcCtx.CouponDB.MarkCouponUsedByOrder(orderID)
}

// ReturnOrderCoupons 退款返还：订单关联的 used/locked 券 → claimed（幂等）。
func (l *CouponLogic) ReturnOrderCoupons(orderID string) error {
	return l.svcCtx.CouponDB.ReturnCouponsByOrder(orderID)
}

func (l *CouponLogic) now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func couponDiscount(c *model.Coupon, listPrice int64) int64 {
	var d int64
	switch c.DiscountType {
	case model.CouponDiscountFixed:
		d = c.DiscountValue
	case model.CouponDiscountPercentBp:
		d = listPrice * c.DiscountValue / 10000
	default:
		d = 0
	}
	if d > listPrice {
		return listPrice
	}
	return d
}

func toCouponView(c *model.Coupon, used int64) types.CouponView {
	return types.CouponView{
		Id: c.Id, Code: c.Code, Name: c.Name, GameId: c.GameId, SkuId: c.SkuId,
		ThresholdCents: c.ThresholdCents, DiscountType: c.DiscountType,
		DiscountValue: c.DiscountValue, Stackable: c.Stackable,
		PerUserLimit: c.PerUserLimit, TotalCount: c.TotalCount,
		StartAt: c.StartAt, EndAt: c.EndAt, Status: c.Status,
		UsedCount: used, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

func toUserCouponView(uc *model.UserCoupon, c *model.Coupon) types.UserCouponView {
	return types.UserCouponView{
		Id: uc.Id, CouponId: uc.CouponId, Code: c.Code, Name: c.Name,
		Status: uc.Status, ClaimedAt: uc.ClaimedAt, LockedOrderId: uc.LockedOrderId,
		UsedAt: uc.UsedAt,
	}
}
