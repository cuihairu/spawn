package logic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/payments/courier"
	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/internal/types"
	"github.com/tappi/tappi/services/payments/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// OrderLogic 订单接入：事实源在 Courier（teller），本服务不复制状态机，
// 只做下单转发 + 金额/币种/商户归属核验 + 回调验签接缝。
type OrderLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *OrderLogic {
	return &OrderLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func envOr(env string) string {
	if strings.TrimSpace(env) == "" {
		return "prod"
	}
	return env
}

// courierError 映射 Courier 契约错误为语义化状态码。
func courierError(err error) error {
	var apiErr *courier.APIError
	if e, ok := err.(*courier.APIError); ok {
		apiErr = e
	}
	switch {
	case apiErr == nil:
		return newAPIError(http.StatusBadGateway, "支付服务暂不可用")
	case apiErr.IsCode("PAYMENT_SKU_NOT_FOUND"):
		return newAPIError(http.StatusNotFound, "商品不存在或不可购")
	case apiErr.IsCode("PAYMENT_ORDER_NOT_FOUND"):
		return newAPIError(http.StatusNotFound, "订单不存在")
	case apiErr.IsCode("PAYMENT_RISK_REJECTED"):
		return newAPIError(http.StatusForbidden, "风控拒绝")
	case apiErr.IsCode("PAYMENT_INVALID_SIGNATURE"):
		return newAPIError(http.StatusForbidden, "回调签名无效")
	case apiErr.IsCode("PAYMENT_ORDER_STATE"):
		return newAPIError(http.StatusConflict, "订单状态不允许该操作")
	default:
		return newAPIError(http.StatusBadGateway, "支付服务错误："+apiErr.Message)
	}
}

// CreateOrder 玩家下单：scope=path gameId，先核验商品归属（商品必须属于该
// 游戏且启用——跨游戏越权 404，不泄露存在性），再转发 Courier 下单，最后
// 核验返回订单的金额/币种与服务端配置一致（防配置漂移/客户端报价）。
// GP-203：带充值目标时核验区服归属与开放状态，并把 订单→区服/角色/商品
// 关联落发货账本（投递事实，非订单状态机）。
func (l *OrderLogic) CreateOrder(req *types.CreateOrderRequest, bearer string) (*types.OrderResponse, error) {
	userID, _ := l.ctx.Value("user_id").(int64)
	if userID <= 0 {
		return nil, newAPIError(http.StatusUnauthorized, "未登录")
	}
	product, err := l.svcCtx.ProductDB.Get(req.SkuId)
	if err == model.ErrProductNotFound {
		return nil, newAPIError(http.StatusNotFound, "商品不存在")
	}
	if err != nil {
		l.Errorf("get product failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询商品失败")
	}
	if !product.Enabled || product.GameId != req.GameId {
		return nil, newAPIError(http.StatusNotFound, "商品不存在或不可购")
	}
	// 充值目标核验：区服必须属于该游戏且开放（GP-201 区服目录为权威）
	if req.ServerId > 0 {
		open, err := catalogServerOpen(l.ctx, l.svcCtx, product.GameId, req.ServerId)
		if err != nil {
			l.Errorf("check server %d of game %s failed: %v", req.ServerId, product.GameId, err)
			return nil, newAPIError(http.StatusBadGateway, "区服服务不可用")
		}
		if !open {
			return nil, newAPIError(http.StatusNotFound, "区服不存在或未开放")
		}
	}
	env := envOr(req.Env)
	order, err := l.svcCtx.Courier.CreateOrder(product.GameId, env, bearer, req.SkuId)
	if err != nil {
		return nil, courierError(err)
	}
	// 金额/币种核验：服务端配置为准，Courier 返回不符 → 拒绝（防漂移）
	if order.AmountCents != product.AmountCents || order.Currency != product.Currency {
		l.Errorf("order amount/currency mismatch: sku=%s configured=%d%s courier=%d%s",
			req.SkuId, product.AmountCents, product.Currency, order.AmountCents, order.Currency)
		return nil, newAPIError(http.StatusConflict, "订单金额与商品配置不一致")
	}
	// GP-305 券报价：客户端仅提交 coupon_ids，金额/门槛/叠加全服务端计算。
	// 锁券失败回滚（券被并发占用），订单在 Courier 侧为未付 CREATED（孤儿
	// 可弃），玩家重试即可；不把折让下传 Courier（Frozen 只读，见 todo）。
	discountCents, applied, err := NewCouponLogic(l.ctx, l.svcCtx).ApplyCoupons(
		order.Id, product.GameId, req.SkuId, userID, product.AmountCents, req.CouponIds)
	if err != nil {
		return nil, err
	}
	// 发货账本：订单 → 充值目标/商品快照（投递事实；teller Deliver 钩子的
	// webhook 面凭它定位区服）。落账失败即拒绝下单——没有目标的订单必然
	// 断单，宁可让玩家重试也不造断单。
	target := &model.DeliveryOrder{
		OrderId:     order.Id,
		GameId:      product.GameId,
		ServerId:    req.ServerId,
		RoleId:      req.RoleId,
		AccountId:   fmt.Sprintf("%d", userID),
		SkuId:       req.SkuId,
		ProductId:   product.ProductId,
		Form:        product.Form,
		AmountCents: product.AmountCents,
		Currency:    product.Currency,
	}
	if err := l.svcCtx.DeliveryDB.CreateTarget(target); err != nil && !errors.Is(err, model.ErrDeliveryOrderExists) {
		l.Errorf("record delivery target failed: order=%s err=%v", order.Id, err)
		// P0 修复：下单后半程失败回滚锁券（无券订单空跑），释放失败仅记
		// 日志不掩盖原错误，残留锁由超时释放扫描兜底。
		l.releaseOrderCoupons(order.Id, req.CouponIds)
		return nil, newAPIError(http.StatusInternalServerError, "落发货目标失败，请重试")
	}
	// 订单只读投影（事实副本）：CREATED 快照；状态后续由回调（PAID）与
	// 投递面（DELIVERED）上报，本服务不推进状态机。DiscountCents 为优惠承担
	//（GP-305，供 GP-205 分成基数冲减）。
	if err := l.svcCtx.ProjectionDB.UpsertFromOrder(&model.OrderProjection{
		OrderId: order.Id, GameId: product.GameId, PartnerId: product.PartnerId,
		AccountId: fmt.Sprintf("%d", userID), BaseQuantity: product.BaseQuantity,
		SkuId: req.SkuId, ProductId: product.ProductId, Form: product.Form,
		AmountCents: product.AmountCents, DiscountCents: discountCents,
		Currency: product.Currency, Status: order.Status,
	}); err != nil {
		l.Errorf("project order failed: order=%s err=%v", order.Id, err)
		// P0 修复：同 CreateTarget 失败口径，回滚锁券再拒绝。
		l.releaseOrderCoupons(order.Id, req.CouponIds)
		return nil, newAPIError(http.StatusInternalServerError, "投影订单失败，请重试")
	}
	resp := &types.OrderResponse{Order: toTypesOrder(order)}
	if discountCents > 0 {
		resp.DiscountCents = discountCents
		resp.PayAmountCents = product.AmountCents - discountCents
		resp.Coupons = applied
	}
	return resp, nil
}

// releaseOrderCoupons 下单失败回滚锁券：把锁到 orderID 的券释放回 claimed
// （P0 修复）。有 coupon_ids 才可能有锁（无券下单空跑）；释放失败仅记日志
// 不掩盖原错误，残留锁由超时释放扫描兜底。
func (l *OrderLogic) releaseOrderCoupons(orderID string, couponIDs []int64) {
	if len(couponIDs) == 0 {
		return
	}
	if err := l.svcCtx.CouponDB.ReturnCouponsByOrder(orderID); err != nil {
		l.Errorf("release locked coupons for failed order=%s err=%v", orderID, err)
	}
}

// GetOrder 玩家查询订单（scope=path gameId；Courier 归属校验：非本人 404）。
func (l *OrderLogic) GetOrder(req *types.OrderQueryRequest, bearer string) (*types.OrderResponse, error) {
	userID, _ := l.ctx.Value("user_id").(int64)
	if userID <= 0 {
		return nil, newAPIError(http.StatusUnauthorized, "未登录")
	}
	order, err := l.svcCtx.Courier.GetOrder(req.GameId, envOr(req.Env), bearer, req.OrderId)
	if err != nil {
		return nil, courierError(err)
	}
	return &types.OrderResponse{Order: toTypesOrder(order)}, nil
}

// ListMyOrders 玩家我的订单（Courier 只读投影，updatedAt 降序）。
func (l *OrderLogic) ListMyOrders(gameID, env, bearer string) (*types.OrderListResponse, error) {
	userID, _ := l.ctx.Value("user_id").(int64)
	if userID <= 0 {
		return nil, newAPIError(http.StatusUnauthorized, "未登录")
	}
	orders, err := l.svcCtx.Courier.ListOrders(gameID, envOr(env), bearer)
	if err != nil {
		return nil, courierError(err)
	}
	items := make([]types.Order, 0, len(orders))
	for _, o := range orders {
		items = append(items, toTypesOrder(&o))
	}
	return &types.OrderListResponse{Items: items, NextCursor: ""}, nil
}

// ListSkus 玩家可购 SKU 列表（scope=path gameId，仅启用商品）。
func (l *OrderLogic) ListSkus(gameID string) (*types.ProductListResponse, error) {
	userID, _ := l.ctx.Value("user_id").(int64)
	if userID <= 0 {
		return nil, newAPIError(http.StatusUnauthorized, "未登录")
	}
	products, err := l.svcCtx.ProductDB.ListByGame(gameID, true)
	if err != nil {
		l.Errorf("list enabled products failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询商品失败")
	}
	items := make([]types.Product, 0, len(products))
	for _, p := range products {
		items = append(items, *toTypesProduct(p))
	}
	return &types.ProductListResponse{Items: items}, nil
}

func toTypesOrder(o *courier.Order) types.Order {
	return types.Order{
		Id: o.Id, Status: o.Status, SkuId: o.SkuId, ProductId: o.ProductId,
		Form: o.Form, AmountCents: o.AmountCents, Currency: o.Currency,
		PayToken: o.PayToken, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
		PaidAt: o.PaidAt, DeliveredAt: o.DeliveredAt,
	}
}
