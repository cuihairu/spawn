package logic

// GP-203 发货接缝：teller Deliver 钩子的签名 webhook 落地面 + 补单/恢复
// 管理面。验签先于解析（对齐 payment.md 安全边界 2 口径）；Secret 未配置
// 时分发 fail-closed（全部 403），不设旁路。

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/tappi/tappi/services/payments/internal/delivery"
	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/internal/types"
	"github.com/tappi/tappi/services/payments/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// DeliveryLogic 发货分发 webhook + 补单管理面。
type DeliveryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeliveryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeliveryLogic {
	return &DeliveryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Dispatch 处理发货分发 webhook（S2S，无 Bearer）：HMAC-SHA256 验签先于
// body 解析，签名缺失/不匹配一律 403。投递失败返回 502（调用方=teller
// Deliver 钩子据此返回错误，订单停 PAID 可对账）。
func (l *DeliveryLogic) Dispatch(rawBody []byte, signature string) error {
	secret := l.svcCtx.Config.Delivery.Secret
	if secret == "" {
		// fail-closed：未配置分发密钥即全部拒绝，不提供任何旁路
		l.Error("delivery dispatch rejected: PAYMENTS_DELIVERY_SECRET not configured")
		return newAPIError(http.StatusForbidden, "发货分发未配置")
	}
	if signature == "" {
		return newAPIError(http.StatusForbidden, "发货签名缺失")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(rawBody)
	if !hmac.Equal([]byte(signature), []byte(hex.EncodeToString(mac.Sum(nil)))) {
		return newAPIError(http.StatusForbidden, "发货签名无效")
	}
	// 验签通过后才解析
	var in delivery.Instruction
	if err := json.Unmarshal(rawBody, &in); err != nil || strings.TrimSpace(in.OrderId) == "" {
		return newAPIError(http.StatusBadRequest, "发货指令无效")
	}
	if err := l.svcCtx.Dispatcher.Dispatch(l.ctx, in); err != nil {
		l.Errorf("deliver order=%s failed: %v", in.OrderId, err)
		return newAPIError(http.StatusBadGateway, "发货失败："+err.Error())
	}
	return nil
}

// ConfigureEndpoint 配置游戏服发货接口（platform_admin/operator）。
func (l *DeliveryLogic) ConfigureEndpoint(req *types.SetDeliveryEndpointRequest, authHeader string) (*types.DeliveryEndpointResponse, error) {
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	endpoint := strings.TrimSpace(req.Endpoint)
	secret := strings.TrimSpace(req.Secret)
	if endpoint == "" || secret == "" {
		return nil, newAPIError(http.StatusBadRequest, "发货接口与密钥不能为空")
	}
	if u, err := url.Parse(endpoint); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, newAPIError(http.StatusBadRequest, "发货接口必须为 http(s) 地址")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if err := l.svcCtx.DeliveryDB.UpsertEndpoint(&model.DeliveryEndpoint{
		GameId: req.GameId, Endpoint: endpoint, Secret: secret, Enabled: enabled,
	}); err != nil {
		l.Errorf("upsert delivery endpoint failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "保存发货接口失败")
	}
	return l.endpointView(req.GameId)
}

// GetEndpoint 查看游戏服发货接口配置（密钥只回是否已设置）。
func (l *DeliveryLogic) GetEndpoint(req *types.DeliveryEndpointPathRequest, authHeader string) (*types.DeliveryEndpointResponse, error) {
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	return l.endpointView(req.GameId)
}

func (l *DeliveryLogic) endpointView(gameId string) (*types.DeliveryEndpointResponse, error) {
	ep, err := l.svcCtx.DeliveryDB.GetEndpoint(gameId)
	if errors.Is(err, model.ErrDeliveryEndpointNotFound) {
		return nil, newAPIError(http.StatusNotFound, "未配置发货接口")
	}
	if err != nil {
		l.Errorf("get delivery endpoint failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询发货接口失败")
	}
	return &types.DeliveryEndpointResponse{Endpoint: types.DeliveryEndpointView{
		GameId: ep.GameId, Endpoint: ep.Endpoint, SecretSet: ep.Secret != "", Enabled: ep.Enabled, UpdatedAt: ep.UpdatedAt,
	}}, nil
}

// ListDeliveries 管理面投递列表（可按游戏/状态筛）。
func (l *DeliveryLogic) ListDeliveries(req *types.DeliveryListRequest, authHeader string) (*types.DeliveryListResponse, error) {
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	rows, err := l.svcCtx.DeliveryDB.ListByGame(req.GameId, req.Status, 100)
	if err != nil {
		l.Errorf("list deliveries failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询投递列表失败")
	}
	items := make([]types.DeliveryOrderView, 0, len(rows))
	for _, row := range rows {
		items = append(items, toDeliveryView(row))
	}
	return &types.DeliveryListResponse{Items: items}, nil
}

// Redeliver 补单入口（platform_admin/operator）：对未发货订单立即重投。
// 已投递订单拒绝（幂等红线：不二次发货）。
func (l *DeliveryLogic) Redeliver(req *types.DeliveryOrderPathRequest, authHeader string) error {
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return err
	}
	row, err := l.svcCtx.DeliveryDB.Get(req.OrderId)
	if errors.Is(err, model.ErrDeliveryOrderNotFound) {
		return newAPIError(http.StatusNotFound, "投递记录不存在")
	}
	if err != nil {
		l.Errorf("get delivery %s failed: %v", req.OrderId, err)
		return newAPIError(http.StatusInternalServerError, "查询投递记录失败")
	}
	if row.Status == model.DeliveryStatusDelivered {
		return newAPIError(http.StatusConflict, "订单已发货，不重复发货")
	}
	return l.dispatchRow(row)
}

// BackfillTarget 补单回填充值目标（账本缺失目标/账本补建后的订单）。
func (l *DeliveryLogic) BackfillTarget(req *types.BackfillTargetRequest, authHeader string) error {
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return err
	}
	if req.ServerId <= 0 {
		return newAPIError(http.StatusBadRequest, "区服不能为空")
	}
	row, err := l.svcCtx.DeliveryDB.Get(req.OrderId)
	if errors.Is(err, model.ErrDeliveryOrderNotFound) {
		return newAPIError(http.StatusNotFound, "投递记录不存在")
	}
	if err != nil {
		l.Errorf("get delivery %s failed: %v", req.OrderId, err)
		return newAPIError(http.StatusInternalServerError, "查询投递记录失败")
	}
	if row.Status == model.DeliveryStatusDelivered {
		return newAPIError(http.StatusConflict, "订单已发货，目标不可改")
	}
	if err := l.svcCtx.DeliveryDB.SetTarget(req.OrderId, req.ServerId, req.RoleId); err != nil {
		l.Errorf("backfill target failed: %v", err)
		return newAPIError(http.StatusInternalServerError, "回填目标失败")
	}
	return nil
}

func (l *DeliveryLogic) dispatchRow(row *model.DeliveryOrder) error {
	if err := l.svcCtx.Dispatcher.Dispatch(l.ctx, delivery.Instruction{
		OrderId:     row.OrderId,
		AccountId:   row.AccountId,
		SkuId:       row.SkuId,
		ProductId:   row.ProductId,
		Form:        row.Form,
		AmountCents: row.AmountCents,
		Currency:    row.Currency,
	}); err != nil {
		l.Errorf("redeliver order=%s failed: %v", row.OrderId, err)
		return newAPIError(http.StatusBadGateway, "发货失败："+err.Error())
	}
	return nil
}

func toDeliveryView(o *model.DeliveryOrder) types.DeliveryOrderView {
	return types.DeliveryOrderView{
		OrderId: o.OrderId, GameId: o.GameId, ServerId: o.ServerId, RoleId: o.RoleId,
		AccountId: o.AccountId, SkuId: o.SkuId, ProductId: o.ProductId, Form: o.Form,
		AmountCents: o.AmountCents, Currency: o.Currency, Status: o.Status,
		Attempts: o.Attempts, LastError: o.LastError, NextRetryAt: o.NextRetryAt,
		DeliveredAt: o.DeliveredAt, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
	}
}
