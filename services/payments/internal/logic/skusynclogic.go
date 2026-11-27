package logic

// GP-202 部署面 SKU 登记：payment_products（服务端定价事实）经 Courier 管理面
// v1.2 POST /v1/payments/admin/skus 登记/改价到 teller 价格表（幂等覆盖 =
// 改价，已建订单不受影响）。显式管理面动作而非商品创建钩子——价格表对齐
// 由运营在商品配置后主动推送，商品创建流程不隐式触网。管理面未接线
//（AdminToken 未配置）fail-closed 503 不伪造登记成功；scope 经 GP-002 映射
// 解析，绝不透传 spawn 游戏 id 当 Courier scope；登记载荷带同值 scope
//（契约 v1.3 SKU 隔离：SKU 只对本游戏 scope 可见可购）。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/tappi/tappi/services/payments/courier"
	"github.com/tappi/tappi/services/payments/internal/courierscope"
	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/internal/types"
	"github.com/tappi/tappi/services/payments/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type SkuSyncLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSkuSyncLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SkuSyncLogic {
	return &SkuSyncLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// SyncGame 把某游戏的全部商品（含 disabled——价格表是定价事实，可购门仍在
// 本侧 enabled 位）逐条登记到 teller 价格表；部分失败逐条留痕继续其余，
// 结果含 synced/failed 计数供运营复核重推。
func (l *SkuSyncLogic) SyncGame(req *types.GamePathRequest, authHeader string) (*types.SkuSyncResponse, error) {
	userID, ip := actorFrom(l.ctx)
	role, err := requireAdmin(l.ctx, l.svcCtx, authHeader)
	if err != nil {
		return nil, err
	}
	adminToken := l.svcCtx.Config.Courier.AdminToken
	if adminToken == "" {
		return nil, newAPIError(http.StatusServiceUnavailable, "Courier 管理面未接线（未配置管理凭证）")
	}
	gameID, env, err := l.resolveCourierScope(req.GameId)
	if err != nil {
		return nil, err
	}
	products, err := l.svcCtx.ProductDB.ListByGame(req.GameId, false)
	if err != nil {
		l.Errorf("list products for sku sync failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询商品失败")
	}
	items := make([]types.SkuSyncItem, 0, len(products))
	synced, failed := 0, 0
	for _, p := range products {
		_, err := l.svcCtx.Courier.AdminSku(gameID, env, adminToken, &courier.AdminSkuReq{
			SkuId:       p.Id,
			ProductId:   p.ProductId,
			Form:        p.Form,
			AmountCents: p.AmountCents,
			Currency:    p.Currency,
			// 契约 v1.3 SKU scope 隔离：登记带 scope，SKU 只对本游戏
			// scope 可见可购（旧网关忽略未知字段，登记仍全局）。
			GameID: gameID,
			Env:    env,
		})
		item := types.SkuSyncItem{SkuId: p.Id, Synced: err == nil}
		if err != nil {
			item.Error = skuSyncErrorText(err)
			l.Errorf("sku sync %s failed: %v", p.Id, err)
			failed++
		} else {
			synced++
		}
		items = append(items, item)
	}
	detail, _ := json.Marshal(map[string]any{
		"game_id": req.GameId, "synced": synced, "failed": failed,
	})
	_ = l.svcCtx.ProjectionDB.LogAudit(&model.OrderAuditLog{
		ActorId: userID, ActorRole: role, Action: "courier_sku_sync", Detail: string(detail), Ip: ip,
	})
	return &types.SkuSyncResponse{
		GameId: req.GameId, Items: items,
		Synced: int64(synced), Failed: int64(failed),
	}, nil
}

// resolveCourierScope spawn game id → Courier scope（GP-002 映射权威，与
// 订单/回调/管理面同款口径）。
func (l *SkuSyncLogic) resolveCourierScope(spawnGameID string) (string, string, error) {
	scope, err := l.svcCtx.ScopeRes.Resolve(spawnGameID)
	if err != nil {
		if errors.Is(err, courierscope.ErrMappingNotFound) {
			return "", "", newAPIError(http.StatusBadGateway, "该游戏未接入 Courier 支付（无映射）")
		}
		l.Errorf("resolve courier scope for game %s failed: %v", spawnGameID, err)
		return "", "", newAPIError(http.StatusBadGateway, "支付接线解析失败")
	}
	return scope.CourierGameId, scope.Env, nil
}

// skuSyncErrorText 逐条结果错误文本：Courier 契约错误码原样留痕（运营可
// 对码处理），其余一律泛化不透传内部细节。
func skuSyncErrorText(err error) string {
	var ae *courier.APIError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return "SYNC_FAILED"
}
