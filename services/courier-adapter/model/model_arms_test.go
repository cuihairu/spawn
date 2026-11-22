package model

// model 剩余补漏分支：Open 的 MySQL DSN 臂与 ensureSQLiteDir 四臂（查询串
// 剥离、:memory:、dir 为 "/"、MkdirAll 失败连带 Open 返回失败）、CreateTables
// 失败臂（只读）、MappingModel 的 Get 非 NotFound 查询失败臂与坏 capabilities
// 反序列化臂、Upsert 写失败臂（只读）、AuditModel 的 limit<=0/超上限钳制、
// 查询失败与 Scan 失败臂、BindingModel 的 GetByCourierAccount、scanBinding
// 非查无错误、UpdateStatus 写失败臂（只读）与 DecryptSecret 的 base64 解码
// 失败/密文过短两臂。脏行一律写非过滤列。
//
// 登记不可达（不测）：EncryptSecret/DecryptSecret 的 aes.NewCipher/NewGCM/
// rand.Read 失败臂（32 字节派生密钥恒合法、GCM 构造恒成功、crypto/rand 不
// 可注入失败）、GenerateAppPassword 的 rand.Read 失败臂、encodeCapabilities
// 的 json.Marshal 失败臂（固定类型结构体）、Open 的 sql.Open 失败臂（驱动
// 注册后几乎不报错）。

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// closedModelDB 已关连接的库（首查即败）。
func closedModelDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}

// roModelDB 以 ?mode=ro 重开同一文件（先建空库保证文件存在且有页）。
func roModelDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ro.db")
	if db, err := Open("file:" + path); err == nil {
		_ = db.Ping()
		_ = db.Close()
	}
	db, err := Open("file:" + path + "?mode=ro")
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestOpen_DsnArms(t *testing.T) {
	// MySQL DSN → 走 mysql 驱动（惰性，不建连）
	db, err := Open("user:pass@tcp(127.0.0.1:1)/db")
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	_ = db.Close()

	// :memory: → ensureSQLiteDir 空转返回
	db, err = Open("file::memory:")
	if err != nil {
		t.Fatalf("open memory: %v", err)
	}
	_ = db.Close()

	// dir 为 "/" → ensureSQLiteDir 直接返回（不建目录、不落文件）
	db, err = Open("file:/x.db")
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	_ = db.Close()

	// 父路径是文件 → MkdirAll 失败 → Open 失败
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("blocker: %v", err)
	}
	if _, err := Open("file:" + blocker + "/sub/x.db"); err == nil || !strings.Contains(err.Error(), "create sqlite dir") {
		t.Fatalf("blocked dir open = %v, want create sqlite dir error", err)
	}
}

func TestCreateTables_ReadonlyFail(t *testing.T) {
	// 只读空库 → CREATE 失败
	if err := CreateTables(roModelDB(t)); err == nil {
		t.Fatal("create tables ro = nil, want error")
	}
}

func TestMappingModel_Arms(t *testing.T) {
	db := openTestDB(t)
	m := NewMappingModel(db)
	// 坏 capabilities → Get 的反序列化失败臂（decode 失败 + 返回 err）
	if _, err := db.Exec(`INSERT INTO courier_game_mapping
		(spawn_game_id, courier_game_id, env, partner_id, capabilities, created_at, updated_at)
		VALUES ('g-bad', 'cg', 'prod', 0, 'not-json', 't', 't')`); err != nil {
		t.Fatalf("insert corrupt caps: %v", err)
	}
	if _, err := m.Get("g-bad"); err == nil {
		t.Fatal("get corrupt caps = nil, want error")
	}
	// 空 capabilities 串 → decode 空值臂（全关，不打断读路径）
	if _, err := db.Exec(`INSERT INTO courier_game_mapping
		(spawn_game_id, courier_game_id, env, partner_id, capabilities, created_at, updated_at)
		VALUES ('g-empty', 'cg-empty', 'prod', 0, '', 't', 't')`); err != nil {
		t.Fatalf("insert empty caps: %v", err)
	}
	got, err := m.Get("g-empty")
	if err != nil || got.Capabilities != (Capabilities{}) {
		t.Fatalf("get empty caps = %+v/%v, want zero capabilities", got, err)
	}
	// 查询失败（闭库）→ 非 ErrNoRows 错误臂
	if _, err := NewMappingModel(closedModelDB(t)).Get("g1"); err == nil {
		t.Fatal("get closed = nil, want error")
	}
	// 写失败（只读）→ Upsert 的 UPDATE 错误臂
	if err := NewMappingModel(roModelDB(t)).Upsert(&GameMapping{SpawnGameId: "g1", CourierGameId: "cg", Env: "prod"}); err == nil {
		t.Fatal("upsert ro = nil, want error")
	}
}

func TestAuditModel_Arms(t *testing.T) {
	db := openTestDB(t)
	m := NewAuditModel(db)
	if err := m.Insert(7, "a", "r", "1", "{}", "ip"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	// limit<=0 → 默认 20；超上限 → 钳到 200
	if _, err := m.ListRecent(0); err != nil {
		t.Fatalf("list limit 0: %v", err)
	}
	if _, err := m.ListRecent(999); err != nil {
		t.Fatalf("list limit 999: %v", err)
	}
	// 查询失败（闭库）
	if _, err := NewAuditModel(closedModelDB(t)).ListRecent(5); err == nil {
		t.Fatal("list closed = nil, want error")
	}
	// Scan 失败：actor_id（非过滤列）写 text
	if _, err := db.Exec(`UPDATE adapter_audit_logs SET actor_id='corrupt'`); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := m.ListRecent(5); err == nil {
		t.Fatal("list scan = nil, want error")
	}
}

func TestBindingModel_Arms(t *testing.T) {
	db := openTestDB(t)
	m := NewBindingModel(db)
	enc, err := EncryptSecret("k", "pw")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if err := m.Create(&Binding{SpawnUserID: 1, CourierAccountID: "acc_1", SecretEnc: enc}); err != nil {
		t.Fatalf("create: %v", err)
	}
	// GetByCourierAccount 命中成功臂
	got, err := m.GetByCourierAccount("acc_1")
	if err != nil || got.SpawnUserID != 1 {
		t.Fatalf("by account = %v/%v, want user 1", got, err)
	}
	// scanBinding 非查无错误臂（闭库）
	if _, err := NewBindingModel(closedModelDB(t)).GetByUser(1); err == nil {
		t.Fatal("get closed = nil, want error")
	}
	// UpdateStatus 写失败臂（只读）
	if err := NewBindingModel(roModelDB(t)).UpdateStatus(1, BindingUnbound); err == nil {
		t.Fatal("update ro = nil, want error")
	}
	// DecryptSecret：base64 解码失败臂
	if _, err := DecryptSecret("k", "!!!not-base64"); err == nil {
		t.Fatal("decode bad base64 = nil, want error")
	}
	// DecryptSecret：密文过短臂（4 字节 < nonce 12）
	if _, err := DecryptSecret("k", "AAAA"); err == nil || !strings.Contains(err.Error(), "too short") {
		t.Fatalf("short ciphertext = %v, want too short", err)
	}
}
