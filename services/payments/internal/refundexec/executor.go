package refundexec

// 退款执行器（GP-204 管理适配）：扫描 refund_requests 中 PENDING 意图 → 调
// Courier 管理面退款端点（payment v1.2 /v1/payments/admin/orders/{id}/refund）
// → 按结果映射 EXECUTED / REJECTED；瞬态失败留 PENDING 下轮重试。
//
// 与订单状态机正交（payment.md「管理面」）：Courier 只登记冲减事实、不改订单
// status；EXECUTED/REJECTED 与金额冲减口径归本服务（接入方侧）。幂等由两侧
// 各自 UNIQUE(order, action) 兜底：Courier 同动作重复登记返回既有，本侧状态
// 转移条件 status='PENDING' 防并发覆盖已终态。
//
// 确定性 vs 瞬态：4xx 业务拒绝（订单不存在/未付款/金额超实付）置 REJECTED
// 不再重试；401 凭证未配、5xx、网络失败按瞬态留 PENDING——凭证补配后下轮起
// 执行，不因误配把退款永久拒绝。

import (
	"context"
	"errors"
	"time"

	"github.com/tappi/tappi/services/payments/courier"
	"github.com/tappi/tappi/services/payments/internal/courierscope"
	"github.com/tappi/tappi/services/payments/model"

	"github.com/zeromicro/go-zero/core/logx"
)

// 执行态（RefundResponse.Execution 取值；refund_requests.status 是持久态）。
const (
	Executed = "executed"
	Rejected = "rejected"
	Pending  = "pending"
)

// Executor PENDING 退款 → Courier 管理面执行循环。
type Executor struct {
	db         *model.OrderProjectionModel
	courier    *courier.Client
	scope      *courierscope.Client
	adminToken string
	interval   time.Duration
}

// NewExecutor 构造执行器；adminToken 空或任一依赖缺失 = 管理面未接线，返回
// nil（调用方不启动、CreateRefund 留 pending——fail-closed 不伪造成功，
// 凭证补配后重启即执行）。与 AnalyticsRelay/NotifyRelay nil=no-op 同约定。
func NewExecutor(db *model.OrderProjectionModel, c *courier.Client, scope *courierscope.Client, adminToken string, interval time.Duration) *Executor {
	if adminToken == "" || db == nil || c == nil || scope == nil {
		return nil
	}
	if interval <= 0 {
		interval = time.Minute
	}
	return &Executor{db: db, courier: c, scope: scope, adminToken: adminToken, interval: interval}
}

// ExecuteOne 执行单条 PENDING 退款意图；返回执行态。瞬态失败留 PENDING，
// 确定性拒绝置 REJECTED，接受登记置 EXECUTED。幂等：意图不存在/已终态 =
// 无操作（返回现态）。
func (e *Executor) ExecuteOne(orderId, action string) string {
	if e == nil {
		return Pending
	}
	rf, err := e.db.GetRefund(orderId, action)
	if err != nil {
		if errors.Is(err, model.ErrRefundRequestNotFound) {
			return Pending
		}
		logx.Errorf("refund exec get %s/%s: %v", orderId, action, err)
		return Pending
	}
	if rf.Status != model.RefundStatusPending {
		return statusOf(rf.Status) // 已终态幂等
	}
	// 退款额度的 Courier scope 按订单所属游戏解析（投影是事实副本，游戏归属
	// 取这里；绝不把 spawn 游戏 id 当 Courier scope 发）。
	row, err := e.db.Get(orderId)
	if err != nil {
		logx.Errorf("refund exec order %s: %v", orderId, err)
		return Pending // 投影暂不可读 → 留 PENDING 重试
	}
	sc, err := e.scope.Resolve(row.GameId)
	if err != nil {
		logx.Errorf("refund exec scope game=%s: %v", row.GameId, err)
		return Pending // 未映射/adapter 不可达 → 瞬态，留 PENDING
	}
	if _, err := e.courier.AdminRefund(sc.CourierGameId, sc.Env, e.adminToken, orderId, action, rf.AmountCents, rf.Reason); err != nil {
		if permanent(err) {
			note := rejectNote(err)
			if err2 := e.db.MarkRefundRejected(orderId, action, note); err2 != nil {
				logx.Errorf("refund exec mark rejected %s/%s: %v", orderId, action, err2)
			}
			logx.Errorf("refund exec rejected %s/%s: %s", orderId, action, note)
			return Rejected
		}
		logx.Errorf("refund exec transient %s/%s: %v", orderId, action, err)
		return Pending
	}
	if err := e.db.MarkRefundExecuted(orderId, action); err != nil {
		logx.Errorf("refund exec mark executed %s/%s: %v", orderId, action, err)
	}
	return Executed
}

// RunOnce 扫一轮 PENDING 意图并逐条执行；返回（尝试数, 接受数）。
func (e *Executor) RunOnce() (attempted, executed int) {
	if e == nil {
		return 0, 0
	}
	rows, err := e.db.ListRefunds(model.RefundStatusPending, 50)
	if err != nil {
		logx.Errorf("refund exec scan failed: %v", err)
		return 0, 0
	}
	for _, r := range rows {
		attempted++
		if e.ExecuteOne(r.OrderId, r.Action) == Executed {
			executed++
		}
	}
	return attempted, executed
}

// Run 周期执行循环，ctx 取消即退出（进程信号面，与发货恢复共用）。
func (e *Executor) Run(ctx context.Context) {
	if e == nil {
		return
	}
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if attempted, executed := e.RunOnce(); attempted > 0 {
				logx.Infof("refund exec: attempted=%d executed=%d", attempted, executed)
			}
		}
	}
}

// permanent 判断 Courier 管理面退款端点的**确定性**拒绝（重试无益）：
// COMMON_INVALID_ARGUMENT（400 金额口径）/ PAYMENT_ORDER_NOT_FOUND（404）/
// PAYMENT_ORDER_STATE（409 未付款）。其余（401 凭证未配、5xx、网络）按瞬态
// 留 PENDING —— 401 属配置未就绪，补配后可执行，不因误配永久拒绝。
func permanent(err error) bool {
	var ae *courier.APIError
	if !errors.As(err, &ae) {
		return false
	}
	switch ae.Code {
	case "COMMON_INVALID_ARGUMENT", "PAYMENT_ORDER_NOT_FOUND", "PAYMENT_ORDER_STATE":
		return true
	}
	return false
}

// rejectNote 提取 Courier 拒绝原因（错误码 + 消息）落 note 留痕。
func rejectNote(err error) string {
	var ae *courier.APIError
	if errors.As(err, &ae) {
		return "courier 拒绝: " + ae.Code + " " + ae.Message
	}
	return "courier 拒绝: " + err.Error()
}

// statusOf 把持久态映射为执行态返回（已终态幂等回传现态）。
func statusOf(status string) string {
	switch status {
	case model.RefundStatusExecuted:
		return Executed
	case model.RefundStatusRejected:
		return Rejected
	default:
		return Pending
	}
}
