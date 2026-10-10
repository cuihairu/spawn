package common

import (
	"context"
	"testing"
)

// ctxWith 成对塞入 key/value，便于复用。
func ctxWith(pairs ...any) context.Context {
	ctx := context.Background()
	for i := 0; i+1 < len(pairs); i += 2 {
		ctx = context.WithValue(ctx, pairs[i], pairs[i+1])
	}
	return ctx
}

// TestUserFromContext_Unauthorized 无 user_id、0、负数、非 int64 → 401。
func TestUserFromContext_Unauthorized(t *testing.T) {
	for _, name := range []string{"missing", "zero", "negative", "wrong type"} {
		t.Run(name, func(t *testing.T) {
			var ctx context.Context
			switch name {
			case "zero":
				ctx = ctxWith("user_id", int64(0))
			case "negative":
				ctx = ctxWith("user_id", int64(-1))
			case "wrong type":
				ctx = ctxWith("user_id", "1")
			default:
				ctx = context.Background()
			}

			id, name, err := UserFromContext(ctx)
			if id != 0 || name != "" || err == nil {
				t.Fatalf("id=%d name=%q err=%v, want (0, \"\", unauthorized)", id, name, err)
			}
		})
	}
}

// TestUserFromContext_Valid 合法 user_id 放行；username 缺失或非 string 时留空。
func TestUserFromContext_Valid(t *testing.T) {
	id, username, err := UserFromContext(ctxWith("user_id", int64(7), "username", "alice"))
	if err != nil || id != 7 || username != "alice" {
		t.Fatalf("got (%d, %q, %v), want (7, \"alice\", nil)", id, username, err)
	}

	id, username, err = UserFromContext(ctxWith("user_id", int64(8)))
	if err != nil || id != 8 || username != "" {
		t.Fatalf("missing username got (%d, %q, %v), want (8, \"\", nil)", id, username, err)
	}

	id, username, err = UserFromContext(ctxWith("user_id", int64(9), "username", 42))
	if err != nil || id != 9 || username != "" {
		t.Fatalf("wrong username type got (%d, %q, %v), want (9, \"\", nil)", id, username, err)
	}
}
