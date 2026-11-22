package logic

// deliverylogic.go 剩余补漏分支：Dispatch 的 fail-closed 403（分发密钥未配置）、
// ConfigureEndpoint 的空值/非法 scheme 400 与 Enabled 覆盖臂与 Upsert 失败 500、
// GetEndpoint/ListDeliveries/BackfillTarget 的角色守卫 403、endpointView 未配置
// 404、ListByGame 查询失败 500、Redeliver 的 404/500、BackfillTarget 的 400/
// 404/409（已发货目标不可改）与 SetTarget 写入失败 500。
//
// 落库失败两法同对账：闭库（首个 SELECT 即败）与 ?mode=ro 只读连接（Get 成功、
// SetTarget 写入失败）。已发货 409 经 CreateTarget+MarkDelivered 落行后触发。

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/internal/types"
	"github.com/tappi/tappi/services/payments/model"
)

// deliverySvcAt 只建发货账本表的上下文（目录存根返回 role/partner 1）。
func deliverySvcAt(t *testing.T, role, dbPath string) (*svc.ServiceContext, *model.DeliveryModel) {
	t.Helper()
	st := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"role":"` + role + `","partner_id":1}`))
	}))
	t.Cleanup(st.Close)
	sc := testSvc(st.URL)
	db := openDB(t, "file:"+dbPath)
	if err := model.NewDeliveryModel(db).CreateTables(); err != nil {
		t.Fatalf("delivery tables: %v", err)
	}
	m := model.NewDeliveryModel(db)
	sc.DeliveryDB = m
	return sc, m
}

func TestDelivery_DispatchFailClosed(t *testing.T) {
	// PAYMENTS_DELIVERY_SECRET 未配置 → fail-closed 403（不设旁路）
	sc := testSvc("http://127.0.0.1:1") // 零值 Config → Secret ""
	if err := NewDeliveryLogic(wbCtx(), sc).Dispatch([]byte(`{"order_id":"o1"}`), "sig"); err == nil {
		t.Fatal("dispatch no secret = nil error, want 403")
	}
}

func TestDelivery_ConfigureEndpointArms(t *testing.T) {
	sc, _ := deliverySvcAt(t, "platform_admin", filepath.Join(t.TempDir(), "dl-cfg.db"))
	l := NewDeliveryLogic(wbCtx(), sc)
	valid := &types.SetDeliveryEndpointRequest{GameId: "game-a", Endpoint: "https://gs.example/pay", Secret: "s3cr3t"}

	// 空 endpoint / 空 secret → 400
	if _, err := l.ConfigureEndpoint(&types.SetDeliveryEndpointRequest{GameId: "game-a", Secret: "s"}, "Bearer t"); err == nil {
		t.Fatal("empty endpoint = nil error, want 400")
	}
	if _, err := l.ConfigureEndpoint(&types.SetDeliveryEndpointRequest{GameId: "game-a", Endpoint: "https://gs.example/pay"}, "Bearer t"); err == nil {
		t.Fatal("empty secret = nil error, want 400")
	}
	// 非法 scheme → 400
	if _, err := l.ConfigureEndpoint(&types.SetDeliveryEndpointRequest{GameId: "game-a", Endpoint: "ftp://gs.example/pay", Secret: "s"}, "Bearer t"); err == nil {
		t.Fatal("ftp scheme = nil error, want 400")
	}
	// Enabled 覆盖臂（false）→ 成功且回显 enabled=false、secret_set=true
	off := false
	resp, err := l.ConfigureEndpoint(&types.SetDeliveryEndpointRequest{GameId: "game-a", Endpoint: "https://gs.example/pay", Secret: "s3cr3t", Enabled: &off}, "Bearer t")
	if err != nil {
		t.Fatalf("configure enabled=false = %v", err)
	}
	if resp.Endpoint.Enabled || !resp.Endpoint.SecretSet {
		t.Fatalf("enabled=%v secret_set=%v, want false/true", resp.Endpoint.Enabled, resp.Endpoint.SecretSet)
	}
	// UpsertEndpoint 失败 → 500
	sc.DeliveryDB = model.NewDeliveryModel(mustClosedDB(t))
	if _, err := l.ConfigureEndpoint(valid, "Bearer t"); err == nil {
		t.Fatal("upsert closed = nil error, want 500")
	}
}

func TestDelivery_EndpointViewNotFound(t *testing.T) {
	// GetEndpoint 守卫过、endpointView 查无配置 → 404
	sc, _ := deliverySvcAt(t, "platform_admin", filepath.Join(t.TempDir(), "dl-nf.db"))
	if _, err := NewDeliveryLogic(wbCtx(), sc).GetEndpoint(&types.DeliveryEndpointPathRequest{GameId: "game-a"}, "Bearer t"); err == nil {
		t.Fatal("endpoint missing = nil error, want 404")
	}
}

func TestDelivery_GuardArms(t *testing.T) {
	// 玩家角色 → requireAdminRole 403（GetEndpoint/ListDeliveries/BackfillTarget）
	sc, _ := deliverySvcAt(t, "player", filepath.Join(t.TempDir(), "dl-guard.db"))
	l := NewDeliveryLogic(wbCtx(), sc)
	if _, err := l.GetEndpoint(&types.DeliveryEndpointPathRequest{GameId: "game-a"}, "Bearer t"); err == nil {
		t.Fatal("player get endpoint = nil error, want 403")
	}
	if _, err := l.ListDeliveries(&types.DeliveryListRequest{}, "Bearer t"); err == nil {
		t.Fatal("player list = nil error, want 403")
	}
	if err := l.BackfillTarget(&types.BackfillTargetRequest{OrderId: "o1", ServerId: 1}, "Bearer t"); err == nil {
		t.Fatal("player backfill = nil error, want 403")
	}
}

func TestDelivery_ListQueryError(t *testing.T) {
	// ListByGame 首个查询即败 → 500
	sc, _ := deliverySvcAt(t, "platform_admin", filepath.Join(t.TempDir(), "dl-list.db"))
	sc.DeliveryDB = model.NewDeliveryModel(mustClosedDB(t))
	if _, err := NewDeliveryLogic(wbCtx(), sc).ListDeliveries(&types.DeliveryListRequest{}, "Bearer t"); err == nil {
		t.Fatal("list closed = nil error, want 500")
	}
}

func TestDelivery_RedeliverArms(t *testing.T) {
	// 查无投递记录 → 404
	sc, _ := deliverySvcAt(t, "platform_admin", filepath.Join(t.TempDir(), "dl-rd.db"))
	if err := NewDeliveryLogic(wbCtx(), sc).Redeliver(&types.DeliveryOrderPathRequest{OrderId: "nope"}, "Bearer t"); err == nil {
		t.Fatal("redeliver missing = nil error, want 404")
	}
	// 查询失败 → 500
	sc.DeliveryDB = model.NewDeliveryModel(mustClosedDB(t))
	if err := NewDeliveryLogic(wbCtx(), sc).Redeliver(&types.DeliveryOrderPathRequest{OrderId: "o1"}, "Bearer t"); err == nil {
		t.Fatal("redeliver closed = nil error, want 500")
	}
}

func TestDelivery_BackfillTargetArms(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "dl-bf.db")
	sc, m := deliverySvcAt(t, "platform_admin", dbPath)
	l := NewDeliveryLogic(wbCtx(), sc)

	// 区服非正 → 400（守卫过、未触库）
	if err := l.BackfillTarget(&types.BackfillTargetRequest{OrderId: "o1", ServerId: 0}, "Bearer t"); err == nil {
		t.Fatal("server_id 0 = nil error, want 400")
	}
	// 查无投递记录 → 404
	if err := l.BackfillTarget(&types.BackfillTargetRequest{OrderId: "nope", ServerId: 1}, "Bearer t"); err == nil {
		t.Fatal("backfill missing = nil error, want 404")
	}
	// 已发货 → 409（目标不可改）
	if err := m.CreateTarget(&model.DeliveryOrder{OrderId: "o-delivered", GameId: "game-a", ServerId: 1, RoleId: "r1", AccountId: "a1", SkuId: "sk1", ProductId: "p1", AmountCents: 1000, Currency: "CNY"}); err != nil {
		t.Fatalf("create delivered row: %v", err)
	}
	if err := m.MarkDelivered("o-delivered"); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	if err := l.BackfillTarget(&types.BackfillTargetRequest{OrderId: "o-delivered", ServerId: 2}, "Bearer t"); err == nil {
		t.Fatal("backfill delivered = nil error, want 409")
	}
	// 只读连接：Get ok（pending 行在），SetTarget 写入败 → 500
	if err := m.CreateTarget(&model.DeliveryOrder{OrderId: "o-pending", GameId: "game-a", ServerId: 1, RoleId: "r1", AccountId: "a1", SkuId: "sk1", ProductId: "p1", AmountCents: 1000, Currency: "CNY"}); err != nil {
		t.Fatalf("create pending row: %v", err)
	}
	ro := openDB(t, "file:"+dbPath+"?mode=ro")
	sc.DeliveryDB = model.NewDeliveryModel(ro)
	if err := l.BackfillTarget(&types.BackfillTargetRequest{OrderId: "o-pending", ServerId: 3, RoleId: "r9"}, "Bearer t"); err == nil {
		t.Fatal("backfill ro = nil error, want 500")
	}
}
