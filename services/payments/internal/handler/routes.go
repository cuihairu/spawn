package handler

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/payments/internal/channel"
	"github.com/tappi/tappi/services/payments/internal/logic"
	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/internal/types"
	"github.com/tappi/tappi/services/payments/model"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// RegisterHandlers 路由装配：玩家支付面（平台 JWT，scope=path gameId）、
// 商品配置管理面（角色经 game-catalog /me）、渠道 S2S 回调（验签接缝）。
func RegisterHandlers(server *rest.Server, svcCtx *svc.ServiceContext) {
	auth := AuthMiddleware(svcCtx)

	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/healthz", Handler: HealthHandler()},
	})

	// 玩家面：下单/订单查询/我的订单/可购 SKU（Bearer 透传 Courier，归属在 Courier 侧）
	playerRoutes := []rest.Route{
		{Method: http.MethodPost, Path: "/games/:gameId/orders", Handler: CreateOrderHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/games/:gameId/orders/:orderId", Handler: GetOrderHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/games/:gameId/orders", Handler: ListMyOrdersHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/games/:gameId/skus", Handler: ListSkusHandler(svcCtx)},
		// GP-305 玩家侧：领取/我的券（平台 JWT）
		{Method: http.MethodPost, Path: "/coupons/:id/claim", Handler: ClaimCouponHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/coupons/my", Handler: MyCouponsHandler(svcCtx)},
	}
	server.AddRoutes(rest.WithMiddleware(auth, playerRoutes...))

	// 管理面：商品配置 + 发货接口/补单 + 订单投影检索/审计/退款适配（platform_admin/operator）
	adminRoutes := []rest.Route{
		{Method: http.MethodPost, Path: "/admin/products/:id", Handler: CreateProductHandler(svcCtx)},
		{Method: http.MethodPatch, Path: "/admin/products/:id/enabled", Handler: SetProductEnabledHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/games/:gameId/products", Handler: ListProductsHandler(svcCtx)},
		{Method: http.MethodPut, Path: "/admin/games/:gameId/delivery", Handler: SetDeliveryEndpointHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/games/:gameId/delivery", Handler: GetDeliveryEndpointHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/deliveries", Handler: ListDeliveriesHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/admin/deliveries/:orderId/redeliver", Handler: RedeliverHandler(svcCtx)},
		{Method: http.MethodPatch, Path: "/admin/deliveries/:orderId/target", Handler: BackfillTargetHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/orders", Handler: AdminListOrdersHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/orders/:orderId", Handler: AdminGetOrderHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/admin/orders/:orderId/refund", Handler: CreateRefundHandler(svcCtx)},
		// GP-204 断单对账重发（courier 管理面同步执行；courier 权威判定 PAID 态，
		// 已发货/已关闭 409 不二次发货；未接线 fail-closed 503 不伪造成功）
		{Method: http.MethodPost, Path: "/admin/orders/:orderId/redeliver", Handler: CourierRedeliverHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/refunds", Handler: ListRefundsHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/audit", Handler: ListAuditHandler(svcCtx)},
		// GP-205 厂商对账单：生成/列表/详情/结算单录入/确认/导出（先确认不自动打款）
		{Method: http.MethodPost, Path: "/admin/statements", Handler: GenerateStatementHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/statements", Handler: ListStatementsHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/statements/:id", Handler: GetStatementHandler(svcCtx)},
		{Method: http.MethodPut, Path: "/admin/statements/:id/settlement", Handler: UpdateSettlementHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/admin/statements/:id/confirm", Handler: ConfirmStatementHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/admin/statements/:id/resolve", Handler: ResolveDisputeHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/statements/:id/disputes", Handler: DisputeRoundsHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/statements/:id/export", Handler: ExportStatementHandler(svcCtx)},
		// GP-305 优惠券管理面：定义/状态机/列表/详情（platform_admin/operator）
		{Method: http.MethodPost, Path: "/admin/coupons", Handler: CreateCouponHandler(svcCtx)},
		{Method: http.MethodPut, Path: "/admin/coupons/:id", Handler: UpdateCouponHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/admin/coupons/:id/publish", Handler: PublishCouponHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/admin/coupons/:id/disable", Handler: DisableCouponHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/admin/coupons/:id/revise", Handler: ReviseCouponHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/coupons", Handler: ListCouponsHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/admin/coupons/:id", Handler: GetCouponHandler(svcCtx)},
	}
	server.AddRoutes(rest.WithMiddleware(auth, adminRoutes...))

	// 合作方自助面（GP-404 F12/F23）：账单确认与结算差异，角色 partner +
	// game-catalog 绑定的 partner_id 行级隔离（requirePartnerRole 在 logic 内
	// 权威判定，与 adminRoutes 共用同一 AuthMiddleware 解析令牌）。
	partnerRoutes := []rest.Route{
		{Method: http.MethodGet, Path: "/partner/statements", Handler: PartnerListStatementsHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/partner/statements/differences", Handler: PartnerStatementDifferencesHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/partner/statements/:id", Handler: PartnerGetStatementHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/partner/statements/:id/export", Handler: PartnerExportStatementHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/partner/statements/:id/confirm", Handler: PartnerConfirmStatementHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/partner/statements/:id/dispute", Handler: PartnerDisputeStatementHandler(svcCtx)},
	}
	server.AddRoutes(rest.WithMiddleware(auth, partnerRoutes...))

	// 渠道 S2S 回调：验签先于 body 解析（payment.md 安全边界 2），无 Bearer
	server.AddRoutes([]rest.Route{
		{Method: http.MethodPost, Path: "/callback/:channel", Handler: CallbackHandler(svcCtx)},
	})

	// 发货分发 webhook（GP-203）：teller Deliver 钩子的签名落地面，S2S
	// 专用，不进网关反代；验签先于 body 解析。
	server.AddRoutes([]rest.Route{
		{Method: http.MethodPost, Path: "/internal/deliveries", Handler: DispatchHandler(svcCtx)},
	})

	// S2S 订单事实查询（GP-307）：courier-adapter 售后工单 orderId 归属
	// 强校验，专用不进网关反代。
	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/internal/orders/:orderId", Handler: InternalGetOrderHandler(svcCtx)},
	})
}

// AuthMiddleware 平台 JWT 认证（user-service 签发，本服务仅校验；注入原始
// Authorization 头供角色 /me 查询透传）。
func AuthMiddleware(svcCtx *svc.ServiceContext) rest.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":401,"message":"缺少认证令牌"}`))
				return
			}
			token := strings.TrimSpace(authHeader[len("Bearer "):])
			if token == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":401,"message":"令牌不能为空"}`))
				return
			}
			claims, err := svcCtx.Auth.ParseToken(token)
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":401,"message":"无效的令牌"}`))
				return
			}
			ctx := context.WithValue(r.Context(), "user_id", claims.UserId)
			ctx = context.WithValue(ctx, "username", claims.Username)
			ctx = context.WithValue(ctx, "auth_header", authHeader)
			ctx = context.WithValue(ctx, "client_ip", clientIP(r))
			next(w, r.WithContext(ctx))
		}
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func authHeaderFrom(r *http.Request) string {
	if v, ok := r.Context().Value("auth_header").(string); ok {
		return v
	}
	return ""
}

// courierAuthFrom 提取客户端携带的 Courier 会话令牌（GP-202 决策：客户端带
// Courier token）。客户端经 GP-201 代登录/票据兑换取得 scope-bound Courier
// accessToken 后，把它放在 X-Courier-Authorization 头随下单/订单请求传入；
// 本服务原样透传给 Courier，绝不透传平台 JWT——平台 token 无 scope、Courier
// RequireAuth 只认自签 opaque token，透传平台 JWT 必 502「auth: invalid
// credentials」，违反接入设计 §1 红线「不假设 JWT 互通。Spawn 只代登录」。
// 值形如 "Bearer <courier accessToken>"；裸 token 自动补前缀（宽容），缺失
// 返回空串由逻辑层 fail-closed 拒绝。
func courierAuthFrom(r *http.Request) string {
	v := strings.TrimSpace(r.Header.Get("X-Courier-Authorization"))
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(v), "bearer ") {
		return "Bearer " + v
	}
	return v
}

func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

func CreateOrderHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CreateOrderRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewOrderLogic(r.Context(), svcCtx).CreateOrder(&req, courierAuthFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func GetOrderHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.OrderQueryRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewOrderLogic(r.Context(), svcCtx).GetOrder(&req, courierAuthFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ListSkusHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GamePathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewOrderLogic(r.Context(), svcCtx).ListSkus(req.GameId)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func CreateProductHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CreateProductRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewProductLogic(r.Context(), svcCtx).Create(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func SetProductEnabledHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.SetProductEnabledRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewProductLogic(r.Context(), svcCtx).SetEnabled(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ListProductsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GamePathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewProductLogic(r.Context(), svcCtx).ListByGame(req.GameId, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// CallbackHandler 渠道 S2S 回调：原生验签接缝（sandbox HMAC / real 占位）。
// 验签先于解析，签名缺失/不匹配一律 403（payment.md 安全边界 2）。
func CallbackHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 只解析 path 变量：httpx.Parse 会在 Content-Type: application/json 时
		// 预读并消费 r.Body，使验签取到的原始字节为空 → 真实渠道回调（带
		// JSON content type）一律 403「回调签名无效」。
		var req types.ChannelPathRequest
		if err := httpx.ParsePath(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		rawBody, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		signature, meta := channelNativeSignature(r)
		if signature == "" && req.Channel != channel.AlipayName {
			// 支付宝异步通知的签名是表单字段 sign（适配器内解析 rawBody），
			// 其余渠道必须携带签名头（sandbox X-Payment-Signature / 微信
			// Wechatpay-Signature）。
			httpx.ErrorCtx(r.Context(), w, logic.NewAPIError(http.StatusForbidden, "回调签名缺失"))
			return
		}
		if err := logic.NewCallbackLogic(r.Context(), svcCtx).Handle(req.Channel, rawBody, signature, meta); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, struct{}{})
	}
}

// channelNativeSignature 提取渠道原生签名与验签元数据（GP-202）。
// 微信 v3 走 Wechatpay-Signature/-Timestamp/-Nonce/-Serial 头（适配器需
// platform-serial 选平台证书）；支付宝签名是表单字段（适配器内解析 body，
// 此处返回空 signature 由调用方按渠道放行）；其余渠道沿用 X-Payment-* 头。
func channelNativeSignature(r *http.Request) (string, map[string]string) {
	meta := map[string]string{
		"timestamp": r.Header.Get("X-Payment-Timestamp"),
		"nonce":     r.Header.Get("X-Payment-Nonce"),
	}
	if wechatSig := r.Header.Get("Wechatpay-Signature"); wechatSig != "" {
		meta["platform-serial"] = r.Header.Get("Wechatpay-Serial")
		if ts := r.Header.Get("Wechatpay-Timestamp"); ts != "" {
			meta["timestamp"] = ts
		}
		if n := r.Header.Get("Wechatpay-Nonce"); n != "" {
			meta["nonce"] = n
		}
		return wechatSig, meta
	}
	return r.Header.Get("X-Payment-Signature"), meta
}

// DispatchHandler 发货分发 webhook（GP-203）：验签先于 body 解析，签名
// 缺失/不匹配一律 403；投递失败 502（teller Deliver 钩子据此停单 PAID）。
func DispatchHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawBody, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		if err := logic.NewDeliveryLogic(r.Context(), svcCtx).Dispatch(rawBody, r.Header.Get("X-Delivery-Signature")); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, struct{}{})
	}
}

// InternalGetOrderHandler S2S 订单事实查询（GP-307）：courier-adapter 售后
// 工单校验 orderId 归属（game_id/account_id）。X-Internal-Token 先于解析
// 严格比对，未配置恒 403（fail-closed）；404 不泄露更多（订单不存在与
// 不属于本人同一口径由调用方判定）。
func InternalGetOrderHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Internal-Token")
		if svcCtx.Config.Internal.Token == "" || token != svcCtx.Config.Internal.Token {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"internal order query disabled"}`))
			return
		}
		var req types.OrderPathRequest
		if err := httpx.ParsePath(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		order, err := svcCtx.ProjectionDB.Get(req.OrderId)
		if errors.Is(err, model.ErrOrderProjectionNotFound) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"order not found"}`))
			return
		}
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, logic.NewAPIError(http.StatusInternalServerError, "查询订单失败"))
			return
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]any{"order": order})
	}
}

func SetDeliveryEndpointHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.SetDeliveryEndpointRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewDeliveryLogic(r.Context(), svcCtx).ConfigureEndpoint(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func GetDeliveryEndpointHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DeliveryEndpointPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewDeliveryLogic(r.Context(), svcCtx).GetEndpoint(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ListDeliveriesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DeliveryListRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewDeliveryLogic(r.Context(), svcCtx).ListDeliveries(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func RedeliverHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DeliveryOrderPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		if err := logic.NewDeliveryLogic(r.Context(), svcCtx).Redeliver(&req, authHeaderFrom(r)); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, struct{}{})
	}
}

func BackfillTargetHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.BackfillTargetRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		if err := logic.NewDeliveryLogic(r.Context(), svcCtx).BackfillTarget(&req, authHeaderFrom(r)); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, struct{}{})
	}
}

func ListMyOrdersHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.OrderListRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewOrderLogic(r.Context(), svcCtx).ListMyOrders(req.GameId, req.Env, courierAuthFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func AdminListOrdersHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AdminOrderQueryRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewAdminOrderLogic(r.Context(), svcCtx).ListOrders(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func AdminGetOrderHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.OrderPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewAdminOrderLogic(r.Context(), svcCtx).GetOrder(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func CreateRefundHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CreateRefundRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewAdminOrderLogic(r.Context(), svcCtx).CreateRefund(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// CourierRedeliverHandler GP-204 断单对账重发管理适配（courier 管理面同步执行）。
func CourierRedeliverHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.OrderPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewAdminOrderLogic(r.Context(), svcCtx).CourierRedeliver(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ListRefundsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.RefundListRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewAdminOrderLogic(r.Context(), svcCtx).ListRefunds(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ListAuditHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AuditListRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewAdminOrderLogic(r.Context(), svcCtx).ListAudit(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func GenerateStatementHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GenerateStatementRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewStatementLogic(r.Context(), svcCtx).Generate(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ListStatementsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.StatementListRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewStatementLogic(r.Context(), svcCtx).List(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func GetStatementHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.StatementPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewStatementLogic(r.Context(), svcCtx).Get(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func UpdateSettlementHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.UpdateSettlementRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewStatementLogic(r.Context(), svcCtx).UpdateSettlement(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

func ConfirmStatementHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.StatementPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewStatementLogic(r.Context(), svcCtx).Confirm(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// ExportStatementHandler 导出对账单 CSV（text/csv 下载；内容型响应，非 JSON）。
func ExportStatementHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.StatementPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		filename, data, err := logic.NewStatementLogic(r.Context(), svcCtx).Export(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

// ResolveDisputeHandler 运营复核处理合作方异议（GP-404 结算差异处理）。
func ResolveDisputeHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.StatementPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewStatementLogic(r.Context(), svcCtx).ResolveDispute(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// DisputeRoundsHandler 异议轮次时间线（管理面，GP-404 版本化留痕）。
func DisputeRoundsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.StatementPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewStatementLogic(r.Context(), svcCtx).DisputeRounds(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// ---- GP-404 合作方自助账单面（角色 partner + partner_id 行级隔离） ----

// PartnerListStatementsHandler 合作方账单列表（本人名下）。
func PartnerListStatementsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.PartnerStatementListRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewPartnerStatementLogic(r.Context(), svcCtx).List(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// PartnerGetStatementHandler 合作方账单详情。
func PartnerGetStatementHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.StatementPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewPartnerStatementLogic(r.Context(), svcCtx).Get(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// PartnerExportStatementHandler 合作方账单 CSV 导出（作用域同详情）。
func PartnerExportStatementHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.StatementPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		filename, data, err := logic.NewPartnerStatementLogic(r.Context(), svcCtx).Export(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

// PartnerConfirmStatementHandler 合作方账单确认（先确认不自动打款）。
func PartnerConfirmStatementHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.StatementPathRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewPartnerStatementLogic(r.Context(), svcCtx).Confirm(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// PartnerDisputeStatementHandler 合作方对结算差异提异议。
func PartnerDisputeStatementHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DisputeStatementRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewPartnerStatementLogic(r.Context(), svcCtx).Dispute(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// PartnerStatementDifferencesHandler 合作方结算差异清单（diff ≠ 0）。
func PartnerStatementDifferencesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.StatementListRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		resp, err := logic.NewPartnerStatementLogic(r.Context(), svcCtx).Differences(&req, authHeaderFrom(r))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
