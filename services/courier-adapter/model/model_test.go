package model

import (
	"database/sql"
	"errors"
	"testing"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open("file:" + t.TempDir() + "/adapter-test.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := CreateTables(db); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	return db
}

func TestMappingUpsertGet(t *testing.T) {
	db := openTestDB(t)
	m := NewMappingModel(db)

	if _, err := m.Get("g1"); !errors.Is(err, ErrMappingNotFound) {
		t.Fatalf("get missing mapping err = %v, want ErrMappingNotFound", err)
	}

	g := &GameMapping{
		SpawnGameId:   "g1",
		CourierGameId: "cg1",
		Env:           "prod",
		PartnerId:     7,
		Capabilities:  Capabilities{Login: true, Download: true},
	}
	if err := m.Upsert(g); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := m.Get("g1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CourierGameId != "cg1" || got.Env != "prod" || got.PartnerId != 7 {
		t.Fatalf("mapping mismatch: %+v", got)
	}
	if !got.Capabilities.Login || got.Capabilities.Payment || !got.Capabilities.Download {
		t.Fatalf("capabilities mismatch: %+v", got.Capabilities)
	}

	// 更新覆盖（同 spawn_game_id）
	g2 := &GameMapping{SpawnGameId: "g1", CourierGameId: "cg9", Env: "dev", PartnerId: 8,
		Capabilities: Capabilities{Payment: true}}
	if err := m.Upsert(g2); err != nil {
		t.Fatalf("upsert v2: %v", err)
	}
	got, _ = m.Get("g1")
	if got.CourierGameId != "cg9" || got.Env != "dev" || !got.Capabilities.Payment || got.Capabilities.Login {
		t.Fatalf("mapping update mismatch: %+v", got)
	}
}

func TestBindingLifecycle(t *testing.T) {
	db := openTestDB(t)
	m := NewBindingModel(db)

	if _, err := m.GetByUser(1); !errors.Is(err, ErrBindingNotFound) {
		t.Fatalf("get missing binding err = %v", err)
	}

	enc, err := EncryptSecret("k1", "app-password-123")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	b := &Binding{SpawnUserID: 1, CourierAccountID: "acc_1", CourierEmail: "p@x.invalid", SecretEnc: enc}
	if err := m.Create(b); err != nil {
		t.Fatalf("create: %v", err)
	}
	if b.Status != BindingActive || b.BindMethod != "auto" || b.BoundAt == "" {
		t.Fatalf("create defaults missing: %+v", b)
	}

	got, err := m.GetByUser(1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CourierAccountID != "acc_1" {
		t.Fatalf("binding mismatch: %+v", got)
	}
	plain, err := DecryptSecret("k1", got.SecretEnc)
	if err != nil || plain != "app-password-123" {
		t.Fatalf("decrypt roundtrip: %q err=%v", plain, err)
	}
	if _, err := DecryptSecret("k2", got.SecretEnc); err == nil {
		t.Fatal("decrypt with wrong key should fail")
	}

	for _, step := range []struct {
		status      string
		wantUnbound bool
	}{
		{BindingPendingDelete, false},
		{BindingActive, false},
		{BindingUnbound, true},
	} {
		if err := m.UpdateStatus(1, step.status); err != nil {
			t.Fatalf("update %s: %v", step.status, err)
		}
		got, _ = m.GetByUser(1)
		if got.Status != step.status {
			t.Fatalf("status = %s, want %s", got.Status, step.status)
		}
		if step.wantUnbound && got.UnboundAt == "" {
			t.Fatal("unbound_at should be set on UNBOUND")
		}
	}

	if err := m.UpdateStatus(99, BindingActive); !errors.Is(err, ErrBindingNotFound) {
		t.Fatalf("update missing binding err = %v", err)
	}
}

func TestGenerateAppPassword(t *testing.T) {
	p1, err := GenerateAppPassword()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	p2, _ := GenerateAppPassword()
	if len(p1) < 8 || len(p1) > 128 {
		t.Fatalf("password length = %d, out of contract 8-128", len(p1))
	}
	if p1 == p2 {
		t.Fatal("passwords should differ")
	}
}

func TestAuditList(t *testing.T) {
	db := openTestDB(t)
	m := NewAuditModel(db)
	for i := 0; i < 3; i++ {
		if err := m.Insert(77, "courier_bind", "binding", "user:77", "{}", "127.0.0.1"); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	items, err := m.ListRecent(2)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].ID <= items[1].ID {
		t.Fatal("list should be newest first")
	}
	if items[0].Action != "courier_bind" || items[0].ActorID != 77 {
		t.Fatalf("entry mismatch: %+v", items[0])
	}
}
