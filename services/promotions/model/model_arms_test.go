package model

// activitymodel / rewardgrantmodel / giftpackmodel / db 补漏分支：各模型
// CRUD 主路径与错误臂（查无/限界钳制/幂等/条件更新/撤销/聚合/审计/发布
// 快照/码库存签发与回滚），以及 Open 的空 DSN/未知驱动/Ping 失败臂、
// CreateTables 双方言全失败臂（?mode=ro 只读库）、PublishSnapshot 事务内
// 两条 Exec 错误臂（只读库 / DROP activities）与 IssueCode 的 UPDATE 写
// 失败臂（只读库 SELECT 命中、UPDATE 写败）。
//
// 登记不可达（不测）：
// - 循环内 rows.Scan 类型错配臂（activitymodel 298/330/405/437；
//   giftpackmodel 315/337/397/585/617；rewardgrantmodel 222/290；
//   analyticsoutboxmodel 125；notifyoutboxmodel 126）：列均 NOT NULL 且
//   扫描目标同型，无错配输入。
// - INSERT OR IGNORE → INSERT IGNORE 语法回退臂（rewardgrantmodel 157；
//   giftpackmodel 359）：SQLite 下 OR IGNORE 恒合法，仅 MySQL 方言触发。
// - outbox CreateTables 的 MySQL-成功臂（analyticsoutboxmodel 85；
//   notifyoutboxmodel 86）：SQLite 方言失败后 MySQL DDL 在 SQLite 上也失败。
// - IssueCode 的 RowsAffected!=1 重试/耗尽臂（giftpackmodel 434/435/437）：
//   SetMaxOpenConns(1) 单连接串行，SELECT 后条件 UPDATE 恒中 1 行。
// - Create 的 LastInsertId 错误臂（activitymodel 205）、InsertGrant 的
//   RowsAffected 错误臂（rewardgrantmodel 172）、GetCode 非 ErrNoRows 通用
//   错误臂（giftpackmodel 496，闭库下 QueryRow 以 ErrNoRows 归返）。

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// newModelDB 打开临时文件 SQLite 并建全部业务表；返回连接与三模型。
func newModelDB(t *testing.T) (*sql.DB, *ActivityModel, *RewardGrantModel, *GiftPackModel) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "model.db")
	db, err := Open("file:" + path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	act := NewActivityModel(db)
	if err := act.CreateTables(); err != nil {
		t.Fatalf("create activity tables: %v", err)
	}
	rew := NewRewardGrantModel(db)
	if err := rew.CreateTables(); err != nil {
		t.Fatalf("create reward tables: %v", err)
	}
	gift := NewGiftPackModel(db)
	if err := gift.CreateTables(); err != nil {
		t.Fatalf("create gift pack tables: %v", err)
	}
	return db, act, rew, gift
}

// seedActivity 落一条草稿活动，回填 Id。
func seedActivity(t *testing.T, m *ActivityModel, name, gameID string) int64 {
	t.Helper()
	a := &Activity{
		Name: name, GameId: gameID, Status: ActivityStatusDraft,
		Timezone: "Asia/Shanghai", StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-12-31T00:00:00Z",
		Audience: "{}", TriggerSpec: "{}", LimitsSpec: "{}", RewardsSpec: "{}",
	}
	if err := m.Create(a); err != nil {
		t.Fatalf("seed activity: %v", err)
	}
	if a.Id == 0 {
		t.Fatal("seed activity id = 0")
	}
	return a.Id
}

// seedGrant 落一条奖励账本行（默认 granted）。
func seedGrant(t *testing.T, m *RewardGrantModel, orderID string, activityID int64, gameID, accountID string) int64 {
	t.Helper()
	g := &RewardGrant{
		OrderId: orderID, ActivityId: activityID, ActivityVersion: 1,
		GameId: gameID, AccountId: accountID, PaidCents: 1000, Currency: "CNY",
		BaseQuantity: 10, TotalQuantity: 10, RateMode: "total", RateBp: 1000,
		RuleType: RuleTypeMultiplier, Status: RewardStatusGranted,
		Snapshot: `{}`, GrantedAt: nowUTC(),
	}
	created, err := m.InsertGrant(g)
	if err != nil || !created {
		t.Fatalf("seed grant: created=%v err=%v", created, err)
	}
	// InsertGrant 不回填 Id：回读取主键
	stored, err := m.GetGrant(orderID, activityID)
	if err != nil {
		t.Fatalf("read back grant: %v", err)
	}
	g.Id = stored.Id
	return g.Id
}

// seedPack 落一条草稿礼包，回填 Id。
func seedPack(t *testing.T, m *GiftPackModel, name, gameID string) int64 {
	t.Helper()
	p := &GiftPack{
		Name: name, GameId: gameID, Status: GiftPackStatusDraft, Timezone: "Asia/Shanghai",
		StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-12-31T00:00:00Z",
		Audience: "{}", PerUserLimit: 1, Content: "{}",
	}
	if err := m.Create(p); err != nil {
		t.Fatalf("seed pack: %v", err)
	}
	return p.Id
}

func TestModelOpen_Arms(t *testing.T) {
	// 空 DSN → 错误
	if _, err := Open(""); err == nil {
		t.Fatal("open empty dsn = nil err")
	}
	// 非 file: 前缀 → MySQL 驱动分支（驱动未注册 → 错误）
	if _, err := Open("user:pass@tcp(127.0.0.1:3306)/db"); err == nil {
		t.Fatal("open mysql dsn = nil err")
	}
	// 垃圾字节文件 → Ping 失败
	garbage := filepath.Join(t.TempDir(), "garbage.db")
	if err := os.WriteFile(garbage, []byte("not a sqlite database"), 0o644); err != nil {
		t.Fatalf("garbage: %v", err)
	}
	if _, err := Open("file:" + garbage); err == nil {
		t.Fatal("open garbage = nil err")
	}
	// 正常文件 → 成功
	if _, err := Open("file:" + filepath.Join(t.TempDir(), "ok.db")); err != nil {
		t.Fatalf("open ok = %v", err)
	}
}

// TestOutboxModel_Arms 两 outbox 模型的 CreateTables 成功臂与 ListRetry
// limit 钳制臂（模型测试二进制未建这两表，单独补测）。
func TestOutboxModel_Arms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.db")
	db, err := Open("file:" + path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	an := NewAnalyticsOutboxModel(db)
	if err := an.CreateTables(); err != nil {
		t.Fatalf("analytics outbox create: %v", err)
	}
	if _, err := an.ListRetry(nowUTC(), 0); err != nil { // limit<=0 → 100
		t.Fatalf("analytics list retry clamp: %v", err)
	}
	if _, err := an.ListRetry(nowUTC(), 9999); err != nil { // limit>500 → 100
		t.Fatalf("analytics list retry high clamp: %v", err)
	}

	nt := NewNotifyOutboxModel(db)
	if err := nt.CreateTables(); err != nil {
		t.Fatalf("notify outbox create: %v", err)
	}
	if _, err := nt.ListRetry(nowUTC(), 0); err != nil {
		t.Fatalf("notify list retry clamp: %v", err)
	}
	if _, err := nt.ListRetry(nowUTC(), 9999); err != nil {
		t.Fatalf("notify list retry high clamp: %v", err)
	}

	// 闭库 → ListRetry 的 Query err 臂
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := an.ListRetry(nowUTC(), 10); err == nil {
		t.Fatal("closed analytics ListRetry = nil err")
	}
	if _, err := nt.ListRetry(nowUTC(), 10); err == nil {
		t.Fatal("closed notify ListRetry = nil err")
	}
}

// TestPublishSnapshot_TxErrorArms PublishSnapshot 的 tx 内两条 Exec 错误臂：
// 只读库 Begin 成功、INSERT 失败；DROP activities 保 activity_versions →
// INSERT 成功、UPDATE 失败。
func TestPublishSnapshot_TxErrorArms(t *testing.T) {
	// 只读库：Begin ok，INSERT activity_versions 写失败
	_, act, _, _ := roModelDB(t)
	if err := act.PublishSnapshot(1, 1, `{}`, 0); err == nil {
		t.Fatal("readonly PublishSnapshot = nil err, want tx INSERT fail")
	}

	// DROP activities（activity_versions 尚在）：INSERT 成功、UPDATE 失败
	path := filepath.Join(t.TempDir(), "drop.db")
	db, err := Open("file:" + path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	a := NewActivityModel(db)
	if err := a.CreateTables(); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.Exec(`DROP TABLE activities`); err != nil {
		t.Fatalf("drop activities: %v", err)
	}
	if err := a.PublishSnapshot(1, 1, `{}`, 0); err == nil {
		t.Fatal("drop-activities PublishSnapshot = nil err, want tx UPDATE fail")
	}
}

// TestIssueCode_UpdateErrorArm 只读库：SELECT 命中 available 码，条件
// UPDATE 写失败 → UPDATE 错误臂。
func TestIssueCode_UpdateErrorArm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "issue-ro.db")
	w, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := w.Exec(`CREATE TABLE _seed (x INTEGER)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	gift := NewGiftPackModel(w)
	if err := gift.CreateTables(); err != nil {
		t.Fatalf("create: %v", err)
	}
	pid := seedPack(t, gift, "p", "g1")
	if _, _, err := gift.ImportCodes(pid, []string{"RO-CODE"}); err != nil {
		t.Fatalf("import: %v", err)
	}
	_ = w.Close()

	db, err := Open("file:" + path + "?mode=ro")
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, _, err := NewGiftPackModel(db).IssueCode(pid, "7", 0, 0); err == nil {
		t.Fatal("readonly IssueCode = nil err, want UPDATE fail")
	}
}

// roModelDB 只读库（含 _seed 页）：所有 CREATE 双方言均失败。
func roModelDB(t *testing.T) (*sql.DB, *ActivityModel, *RewardGrantModel, *GiftPackModel) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ro.db")
	w, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open writable: %v", err)
	}
	if _, err := w.Exec(`CREATE TABLE _seed (x INTEGER)`); err != nil {
		t.Fatalf("seed table: %v", err)
	}
	_ = w.Close()
	db, err := Open("file:" + path + "?mode=ro")
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, NewActivityModel(db), NewRewardGrantModel(db), NewGiftPackModel(db)
}

func TestCreateTables_BothDialectFailArms(t *testing.T) {
	_, act, rew, gift := roModelDB(t)
	if err := act.CreateTables(); err == nil || !strings.Contains(err.Error(), "创建活动表失败") {
		t.Fatalf("activity create tables = %v", err)
	}
	if err := rew.CreateTables(); err == nil || !strings.Contains(err.Error(), "创建奖励账本表失败") {
		t.Fatalf("reward create tables = %v", err)
	}
	if err := gift.CreateTables(); err == nil || !strings.Contains(err.Error(), "创建礼包表失败") {
		t.Fatalf("gift create tables = %v", err)
	}
}

func TestActivityModel_CrudArms(t *testing.T) {
	db, m, _, _ := newModelDB(t)
	id := seedActivity(t, m, "夏日充值", "g1")

	// Update（草稿配置）
	if err := m.Update(&Activity{Id: id, Name: "夏日充值·改", GameId: "g1", Timezone: "Asia/Shanghai"}); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Get 命中 + 查无
	got, err := m.Get(id)
	if err != nil || got.Name != "夏日充值·改" {
		t.Fatalf("get = %+v/%v", got, err)
	}
	if _, err := m.Get(9999); err != ErrActivityNotFound {
		t.Fatalf("get missing = %v, want ErrActivityNotFound", err)
	}

	// List（game/status 过滤 + limit 钳制）
	if rows, err := m.List("g1", "", 0, 0); err != nil || len(rows) != 1 {
		t.Fatalf("list = %d/%v, want 1", len(rows), err)
	}
	if rows, err := m.List("g1", ActivityStatusDraft, 500, -5); err != nil || len(rows) != 1 {
		t.Fatalf("list clamp = %d/%v", len(rows), err)
	}
	if rows, _ := m.List("g-other", "", 50, 0); len(rows) != 0 {
		t.Fatalf("list other game = %d, want 0", len(rows))
	}

	// PublishSnapshot（落版本 + 推进 current_version/status）
	if err := m.PublishSnapshot(id, 1, `{"v":1}`, 42); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got, _ := m.Get(id); got.Status != ActivityStatusPublished || got.CurrentVersion != 1 {
		t.Fatalf("after publish = %s/%d", got.Status, got.CurrentVersion)
	}

	// NextVersion 1→2；HasSnapshots true
	if v, err := m.NextVersion(id); err != nil || v != 2 {
		t.Fatalf("next version = %d/%v, want 2", v, err)
	}
	if has, err := m.HasSnapshots(id); err != nil || !has {
		t.Fatalf("has snapshots = %v/%v, want true", has, err)
	}
	if has, _ := m.HasSnapshots(9999); has {
		t.Fatal("has snapshots missing = true, want false")
	}

	// ListVersions + GetVersion 命中/查无
	vers, err := m.ListVersions(id)
	if err != nil || len(vers) != 1 || vers[0].Version != 1 {
		t.Fatalf("list versions = %d/%v", len(vers), err)
	}
	if v, err := m.GetVersion(id, 1); err != nil || v.Snapshot != `{"v":1}` {
		t.Fatalf("get version = %+v/%v", v, err)
	}
	if _, err := m.GetVersion(id, 99); err != ErrVersionNotFound {
		t.Fatalf("get version missing = %v", err)
	}

	// ListPublishedSnapshots（published + current_version 匹配才出；
	// 须在 SetStatus(disabled) 之前校验）
	pubs, err := m.ListPublishedSnapshots("g1")
	if err != nil || len(pubs) != 1 || pubs[0].ActivityId != id || pubs[0].Version != 1 {
		t.Fatalf("published snapshots = %d/%v", len(pubs), err)
	}
	// 未发布游戏无快照
	if pubs, _ := m.ListPublishedSnapshots("g-other"); len(pubs) != 0 {
		t.Fatalf("published other = %d, want 0", len(pubs))
	}

	// SetStatus
	if err := m.SetStatus(id, ActivityStatusDisabled); err != nil {
		t.Fatalf("set status: %v", err)
	}

	// LogAudit + ListAudit（activityId 过滤 / limit 钳制）
	if err := m.LogAudit(&ActivityAuditLog{ActivityId: id, ActorId: 42, ActorRole: "operator", Action: "activity_create", Detail: "d", Ip: "1.2.3.4"}); err != nil {
		t.Fatalf("log audit: %v", err)
	}
	audits, err := m.ListAudit(id, 0)
	if err != nil || len(audits) != 1 || audits[0].Action != "activity_create" {
		t.Fatalf("list audit = %d/%v", len(audits), err)
	}
	if audits, _ := m.ListAudit(0, 500); len(audits) != 1 {
		t.Fatalf("list audit all = %d, want 1", len(audits))
	}
	if audits, _ := m.ListAudit(9999, 50); len(audits) != 0 {
		t.Fatalf("list audit other = %d, want 0", len(audits))
	}

	// 闭库错误臂
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := m.List("g1", "", 50, 0); err == nil {
		t.Fatal("list closed = nil err")
	}
	if err := m.Update(&Activity{Id: id}); err == nil {
		t.Fatal("update closed = nil err")
	}
	if err := m.PublishSnapshot(id, 2, `{}`, 1); err == nil {
		t.Fatal("publish closed = nil err")
	}
}

func TestRewardGrantModel_Arms(t *testing.T) {
	db, act, m, _ := newModelDB(t)
	a1 := seedActivity(t, act, "act", "g1")
	id := seedGrant(t, m, "ord-1", a1, "g1", "77")
	seedGrant(t, m, "ord-2", a1, "g1", "77")

	// 幂等：同 (order,activity) 重插 → created=false
	dup := &RewardGrant{OrderId: "ord-1", ActivityId: a1, GameId: "g1", AccountId: "77", Status: RewardStatusGranted, GrantedAt: nowUTC()}
	if created, err := m.InsertGrant(dup); err != nil || created {
		t.Fatalf("dup insert = %v/%v, want false", created, err)
	}

	// GetGrant 命中/查无
	if g, err := m.GetGrant("ord-1", a1); err != nil || g.AccountId != "77" {
		t.Fatalf("get grant = %+v/%v", g, err)
	}
	if _, err := m.GetGrant("ord-nope", a1); err != ErrRewardGrantNotFound {
		t.Fatalf("get grant missing = %v", err)
	}

	// ListGrants（过滤组合 + 默认 100 上限）
	if rows, err := m.ListGrants(a1, "", "", ""); err != nil || len(rows) != 2 {
		t.Fatalf("list grants = %d/%v, want 2", len(rows), err)
	}
	if rows, err := m.ListGrants(a1, "ord-1", "g1", "77"); err != nil || len(rows) != 1 {
		t.Fatalf("list grants filtered = %d/%v, want 1", len(rows), err)
	}

	// 计数
	if n, err := m.CountGrantsByUser(a1, "77"); err != nil || n != 2 {
		t.Fatalf("count by user = %d/%v, want 2", n, err)
	}
	if n, err := m.CountGrantsByActivity(a1); err != nil || n != 2 {
		t.Fatalf("count by activity = %d/%v, want 2", n, err)
	}
	if n, err := m.CountGrantsByUserRule(a1, "77", RuleTypeMultiplier); err != nil || n != 2 {
		t.Fatalf("count by rule = %d/%v, want 2", n, err)
	}

	// SumGrantedPaidCents 去重订单聚合（两订单各 1000）
	if sum, err := m.SumGrantedPaidCents(a1); err != nil || sum != 2000 {
		t.Fatalf("sum paid = %d/%v, want 2000", sum, err)
	}

	// ListGrantedCumulativeMilestones（非 cumulative 规则 → 空）
	if rows, err := m.ListGrantedCumulativeMilestones(a1, "77"); err != nil || len(rows) != 0 {
		t.Fatalf("cumulative milestones = %d/%v, want 0", len(rows), err)
	}

	// MarkGrantRevoked → Sum 计入减少
	if err := m.MarkGrantRevoked(id); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if sum, _ := m.SumGrantedPaidCents(a1); sum != 1000 {
		t.Fatalf("sum after revoke = %d, want 1000", sum)
	}
	if n, _ := m.CountGrantsByActivity(a1); n != 1 {
		t.Fatalf("count after revoke = %d, want 1", n)
	}

	// 闭库错误臂
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := m.CountGrantsByUser(a1, "77"); err == nil {
		t.Fatal("count closed = nil err")
	}
	if _, err := m.ListGrantedCumulativeMilestones(a1, "77"); err == nil {
		t.Fatal("milestones closed = nil err")
	}
	if err := m.MarkGrantRevoked(id); err == nil {
		t.Fatal("revoke closed = nil err")
	}
}

// TestModel_ClosedDBSweep 闭库后逐一调用所有模型方法，兜住各方法首条
// Query/Exec 的错误臂（闭库使一切读写失败）。
func TestModel_ClosedDBSweep(t *testing.T) {
	db, act, rew, gift := newModelDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Activity
	_ = act.Create(&Activity{Name: "x", GameId: "g1", Timezone: "Asia/Shanghai"})
	if _, err := act.Get(1); err == nil {
		t.Fatal("closed activity Get = nil err")
	}
	if _, err := act.List("g1", "", 50, 0); err == nil {
		t.Fatal("closed activity List = nil err")
	}
	if _, err := act.NextVersion(1); err == nil {
		t.Fatal("closed activity NextVersion = nil err")
	}
	if _, err := act.ListVersions(1); err == nil {
		t.Fatal("closed activity ListVersions = nil err")
	}
	if _, err := act.GetVersion(1, 1); err == nil {
		t.Fatal("closed activity GetVersion = nil err")
	}
	if _, err := act.HasSnapshots(1); err == nil {
		t.Fatal("closed activity HasSnapshots = nil err")
	}
	if err := act.LogAudit(&ActivityAuditLog{ActivityId: 1}); err == nil {
		t.Fatal("closed activity LogAudit = nil err")
	}
	if _, err := act.ListAudit(1, 50); err == nil {
		t.Fatal("closed activity ListAudit = nil err")
	}
	if _, err := act.ListPublishedSnapshots("g1"); err == nil {
		t.Fatal("closed activity ListPublishedSnapshots = nil err")
	}
	if err := act.SetStatus(1, ActivityStatusDisabled); err == nil {
		t.Fatal("closed activity SetStatus = nil err")
	}

	// RewardGrant
	g := &RewardGrant{OrderId: "o", ActivityId: 1, GameId: "g1", AccountId: "7", Status: RewardStatusGranted, GrantedAt: nowUTC()}
	if _, err := rew.InsertGrant(g); err == nil {
		t.Fatal("closed reward InsertGrant = nil err")
	}
	if _, err := rew.GetGrant("o", 1); err == nil {
		t.Fatal("closed reward GetGrant = nil err")
	}
	if _, err := rew.ListGrants(1, "", "", ""); err == nil {
		t.Fatal("closed reward ListGrants = nil err")
	}

	// GiftPack
	_ = gift.Create(&GiftPack{Name: "x", GameId: "g1", Timezone: "Asia/Shanghai"})
	if _, err := gift.Get(1); err == nil {
		t.Fatal("closed gift Get = nil err")
	}
	if _, err := gift.List("g1", "", 50); err == nil {
		t.Fatal("closed gift List = nil err")
	}
	if _, err := gift.ListCodes(1, CodeStatusAvailable, 10); err == nil {
		t.Fatal("closed gift ListCodes = nil err")
	}
	if _, _, err := gift.IssueCode(1, "7", 0, 0); err == nil {
		t.Fatal("closed gift IssueCode = nil err")
	}
	if err := gift.ReleaseCode(1); err == nil {
		t.Fatal("closed gift ReleaseCode = nil err")
	}
	if err := gift.MarkCodeUsed(1); err == nil {
		t.Fatal("closed gift MarkCodeUsed = nil err")
	}
	if _, err := gift.GetCode(1); err == nil {
		t.Fatal("closed gift GetCode = nil err")
	}
	if err := gift.InsertClaim(&GiftPackClaim{PackId: 1, AccountId: "7", GameId: "g1", ClaimIndex: 1, RequestId: "r", Status: ClaimStatusClaimed}); err == nil {
		t.Fatal("closed gift InsertClaim = nil err")
	}
	if err := gift.InsertClaimCapped(&GiftPackClaim{PackId: 1, AccountId: "7", GameId: "g1", ClaimIndex: 1, RequestId: "r", Status: ClaimStatusClaimed}, 1); err == nil {
		t.Fatal("closed gift InsertClaimCapped = nil err")
	}
	if _, err := gift.GetClaim(1); err == nil {
		t.Fatal("closed gift GetClaim = nil err")
	}
	if _, err := gift.ListClaimsByAccount("7", 10); err == nil {
		t.Fatal("closed gift ListClaimsByAccount = nil err")
	}
	if _, err := gift.ListClaims(1, "7", 10); err == nil {
		t.Fatal("closed gift ListClaims = nil err")
	}
}

func TestGiftPackModel_Arms(t *testing.T) {
	db, _, _, m := newModelDB(t)
	pid := seedPack(t, m, "新手礼包", "g1")

	// Update / SetStatus（Update 覆盖全列，须带完整时间窗）
	if err := m.Update(&GiftPack{Id: pid, Name: "新手礼包·改", GameId: "g1", Timezone: "Asia/Shanghai",
		StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-12-31T00:00:00Z", Content: "{}"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := m.SetStatus(pid, GiftPackStatusPublished); err != nil {
		t.Fatalf("set status: %v", err)
	}

	// Get 命中/查无
	if p, err := m.Get(pid); err != nil || p.Name != "新手礼包·改" {
		t.Fatalf("get = %+v/%v", p, err)
	}
	if _, err := m.Get(9999); err != ErrGiftPackNotFound {
		t.Fatalf("get missing = %v", err)
	}

	// List（game/status 过滤 + limit 钳制）
	if rows, err := m.List("g1", GiftPackStatusPublished, 0); err != nil || len(rows) != 1 {
		t.Fatalf("list = %d/%v", len(rows), err)
	}
	if rows, _ := m.List("g1", GiftPackStatusPublished, 500); len(rows) != 1 {
		t.Fatalf("list clamp = %d, want 1", len(rows))
	}

	// ListPublishedByGame（published + 时间窗覆盖 now）
	if rows, err := m.ListPublishedByGame("g1", "2026-06-01T00:00:00Z"); err != nil || len(rows) != 1 {
		t.Fatalf("published = %d/%v", len(rows), err)
	}
	if rows, _ := m.ListPublishedByGame("g1", "2025-01-01T00:00:00Z"); len(rows) != 0 {
		t.Fatalf("published before start = %d, want 0", len(rows))
	}

	// ImportCodes（新增/重复/空白）
	ok, dup, err := m.ImportCodes(pid, []string{"CODE-A", "code-a ", "  ", "CODE-B", "CODE-A"})
	if err != nil || ok != 3 || dup != 1 {
		t.Fatalf("import codes = %d/%d/%v, want 3/1/nil", ok, dup, err)
	}

	// CountAvailableCodes
	if n, err := m.CountAvailableCodes(pid); err != nil || n != 3 {
		t.Fatalf("available = %d/%v, want 3", n, err)
	}

	// ListCodes（status 过滤）
	if rows, err := m.ListCodes(pid, CodeStatusAvailable, 0); err != nil || len(rows) != 3 {
		t.Fatalf("list codes = %d/%v", len(rows), err)
	}

	// IssueCode 成功 → 已签发；库存逐枚签尽 → ErrGiftPackCodeNotFound
	codeID, code, err := m.IssueCode(pid, "77", 0, 0)
	if err != nil || codeID == 0 || code == "" {
		t.Fatalf("issue code = %d/%s/%v", codeID, code, err)
	}
	if _, _, err := m.IssueCode(pid, "77", 0, 0); err != nil { // 第 2 枚
		t.Fatalf("issue 2nd = %v", err)
	}
	codeID2, _, err := m.IssueCode(pid, "77", 0, 0) // 第 3 枚（最后一枚）
	if err != nil {
		t.Fatalf("issue 3rd = %v", err)
	}
	if _, _, err := m.IssueCode(pid, "77", 0, 0); err != ErrGiftPackCodeNotFound {
		t.Fatalf("issue exhausted = %v, want ErrGiftPackCodeNotFound", err)
	}

	// GetCode 命中/查无
	if c, err := m.GetCode(codeID); err != nil || c.Status != CodeStatusIssued {
		t.Fatalf("get code = %+v/%v", c, err)
	}
	if _, err := m.GetCode(9999); err != ErrGiftPackCodeNotFound {
		t.Fatalf("get code missing = %v", err)
	}

	// ReleaseCode（issued → available）
	if err := m.ReleaseCode(codeID); err != nil {
		t.Fatalf("release code: %v", err)
	}
	if c, _ := m.GetCode(codeID); c.Status != CodeStatusAvailable {
		t.Fatalf("after release = %s, want available", c.Status)
	}

	// MarkCodeUsed：未签发（available）→ notfound；签发后再用 → used
	if err := m.MarkCodeUsed(codeID); err != ErrGiftPackCodeNotFound {
		t.Fatalf("mark used available = %v, want notfound", err)
	}
	codeID2, _, err = m.IssueCode(pid, "77", 0, 0)
	if err != nil {
		t.Fatalf("issue 2nd: %v", err)
	}
	if err := m.MarkCodeUsed(codeID2); err != nil {
		t.Fatalf("mark used = %v", err)
	}
	if c, _ := m.GetCode(codeID2); c.Status != CodeStatusUsed {
		t.Fatalf("after used = %s, want used", c.Status)
	}

	// Claims：InsertClaim + CountClaims + GetClaimByRequest（命中/查无）
	claim := &GiftPackClaim{PackId: pid, AccountId: "77", GameId: "g1", ServerId: 5,
		RoleId: "r", ClaimIndex: 1, RequestId: "req-1", Status: ClaimStatusClaimed}
	if err := m.InsertClaim(claim); err != nil || claim.Id == 0 {
		t.Fatalf("insert claim = %d/%v", claim.Id, err)
	}
	if n, err := m.CountClaims(pid, "77"); err != nil || n != 1 {
		t.Fatalf("count claims = %d/%v, want 1", n, err)
	}
	if c, err := m.GetClaimByRequest(pid, "77", "req-1"); err != nil || c.Id != claim.Id {
		t.Fatalf("get by request = %+v/%v", c, err)
	}
	if _, err := m.GetClaimByRequest(pid, "77", "req-nope"); err != ErrGiftPackClaimNotFound {
		t.Fatalf("get by request missing = %v", err)
	}

	// InsertClaimCapped：超限 → ErrGiftPackClaimTotalReached
	cap := &GiftPackClaim{PackId: pid, AccountId: "88", GameId: "g1", ClaimIndex: 1, RequestId: "cap-1", Status: ClaimStatusClaimed}
	if err := m.InsertClaimCapped(cap, 1); err == nil {
		// 已有 1 行（account 77），上限 1 → 再插应超限
		if err := m.InsertClaimCapped(cap, 1); err != ErrGiftPackClaimTotalReached {
			t.Fatalf("capped = %v, want ErrGiftPackClaimTotalReached", err)
		}
	}
	// 未超限 → 成功并回填 Id（LastInsertId 臂）
	capOK := &GiftPackClaim{PackId: pid, AccountId: "99", GameId: "g1", ClaimIndex: 5, RequestId: "cap-ok", Status: ClaimStatusClaimed}
	if err := m.InsertClaimCapped(capOK, 10); err != nil || capOK.Id == 0 {
		t.Fatalf("capped ok = id %d / %v, want id>0/nil", capOK.Id, err)
	}

	// GetClaim 命中/查无 + BindClaimCode + ListClaimsByAccount + ListClaims
	if c, err := m.GetClaim(claim.Id); err != nil || c.AccountId != "77" {
		t.Fatalf("get claim = %+v/%v", c, err)
	}
	if _, err := m.GetClaim(9999); err != ErrGiftPackClaimNotFound {
		t.Fatalf("get claim missing = %v", err)
	}
	if err := m.BindClaimCode(claim.Id, codeID); err != nil {
		t.Fatalf("bind claim code: %v", err)
	}
	if rows, err := m.ListClaimsByAccount("77", 0); err != nil || len(rows) != 1 {
		t.Fatalf("claims by account = %d/%v", len(rows), err)
	}
	if rows, err := m.ListClaims(pid, "77", 0); err != nil || len(rows) != 1 {
		t.Fatalf("list claims = %d/%v", len(rows), err)
	}
	if rows, _ := m.ListClaims(0, "77", 500); len(rows) != 1 {
		t.Fatalf("list claims all = %d, want 1", len(rows))
	}

	// ReleaseClaim（claimed → 删除）
	if err := m.ReleaseClaim(claim.Id); err != nil {
		t.Fatalf("release claim: %v", err)
	}
	if n, _ := m.CountClaims(pid, "77"); n != 0 {
		t.Fatalf("count after release = %d, want 0", n)
	}

	// 闭库错误臂
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := m.CountAvailableCodes(pid); err == nil {
		t.Fatal("count codes closed = nil err")
	}
	if _, err := m.ListPublishedByGame("g1", nowUTC()); err == nil {
		t.Fatal("published closed = nil err")
	}
	if _, _, err := m.ImportCodes(pid, []string{"X"}); err == nil {
		t.Fatal("import closed = nil err")
	}
}
