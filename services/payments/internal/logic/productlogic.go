package logic

import (
	"context"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/internal/types"
	"github.com/tappi/tappi/services/payments/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// ProductLogic 商品配置管理面（服务端定价 + 商户归属）。
// 角色权威经 game-catalog /me 判定（platform_admin/operator），
// 与 courier-adapter 管理面同口径。
type ProductLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewProductLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProductLogic {
	return &ProductLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func validForm(form string) bool {
	switch form {
	case model.FormDirectPurchase, model.FormAccountWallet,
		model.FormGameWallet, model.FormExternalPay:
		return true
	}
	return false
}

// Create 新建商品（服务端定价）：amount/currency 由服务端配置落库，
// 客户端不可改（payment.md 安全边界 1）。
func (l *ProductLogic) Create(req *types.CreateProductRequest, authHeader string) (*types.Product, error) {
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Id) == "" || strings.TrimSpace(req.ProductId) == "" {
		return nil, newAPIError(http.StatusBadRequest, "商品 id 与 product_id 不能为空")
	}
	if !validForm(req.Form) {
		return nil, newAPIError(http.StatusBadRequest, "非法商品形态")
	}
	if req.AmountCents <= 0 {
		return nil, newAPIError(http.StatusBadRequest, "金额必须为正")
	}
	if len(req.Currency) != 3 {
		return nil, newAPIError(http.StatusBadRequest, "币种须为 ISO 4217 三字母")
	}
	p, err := l.svcCtx.ProductDB.Create(&model.Product{
		Id: req.Id, ProductId: req.ProductId, Form: req.Form,
		AmountCents: req.AmountCents, Currency: strings.ToUpper(req.Currency),
		GameId: req.GameId, PartnerId: req.PartnerId,
	})
	if err == model.ErrProductExists {
		return nil, newAPIError(http.StatusConflict, "商品 id 已存在")
	}
	if err != nil {
		l.Errorf("create product failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "创建商品失败")
	}
	return toTypesProduct(p), nil
}

// SetEnabled 上/下架商品（disabled 不参与玩家可购列表）。
func (l *ProductLogic) SetEnabled(req *types.SetProductEnabledRequest, authHeader string) (*types.Product, error) {
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	p, err := l.svcCtx.ProductDB.SetEnabled(req.Id, req.Enabled)
	if err == model.ErrProductNotFound {
		return nil, newAPIError(http.StatusNotFound, "商品不存在")
	}
	if err != nil {
		l.Errorf("set product enabled failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "更新商品失败")
	}
	return toTypesProduct(p), nil
}

// ListByGame 列某游戏商品（管理面全量）。
func (l *ProductLogic) ListByGame(gameID, authHeader string) (*types.ProductListResponse, error) {
	if err := requireAdminRole(l.ctx, l.svcCtx, authHeader); err != nil {
		return nil, err
	}
	products, err := l.svcCtx.ProductDB.ListByGame(gameID, false)
	if err != nil {
		l.Errorf("list products failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询商品失败")
	}
	items := make([]types.Product, 0, len(products))
	for _, p := range products {
		items = append(items, *toTypesProduct(p))
	}
	return &types.ProductListResponse{Items: items}, nil
}

func toTypesProduct(p *model.Product) *types.Product {
	if p == nil {
		return &types.Product{}
	}
	return &types.Product{
		Id: p.Id, ProductId: p.ProductId, Form: p.Form,
		AmountCents: p.AmountCents, Currency: p.Currency,
		GameId: p.GameId, PartnerId: p.PartnerId, Enabled: p.Enabled,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}
