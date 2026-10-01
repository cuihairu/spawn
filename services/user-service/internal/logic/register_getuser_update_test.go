package logic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tappi/tappi/services/user-service/internal/integration"
	"github.com/tappi/tappi/services/user-service/internal/svc"
	"github.com/tappi/tappi/services/user-service/internal/types"
	"github.com/tappi/tappi/services/user-service/model"
	"github.com/tappi/tappi/services/user-service/utils"
)

func newLogicSvcCtx(t *testing.T) *svc.ServiceContext {
	t.Helper()
	db := setupTestDB(t)
	return &svc.ServiceContext{
		DB:        db,
		UserModel: model.NewUserModel(db),
		Auth:      utils.NewAuth("logic-test-secret", 24*time.Hour),
	}
}

func envelopeCode(t *testing.T, resp *types.ApiResponse) int {
	t.Helper()
	if resp == nil {
		t.Fatal("resp must not be nil")
	}
	return resp.Code
}

// --- RegisterLogic ---

func TestRegisterLogicSuccess(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	l := NewRegisterLogic(context.Background(), svcCtx)

	resp, err := l.Register(&types.RegisterRequest{
		Username: "carol",
		Email:    "carol@example.com",
		Password: "pw-carol",
		Nickname: "Carol",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if envelopeCode(t, resp) != 200 {
		t.Fatalf("code = %d body=%+v", resp.Code, resp)
	}
	data := resp.Data.(map[string]interface{})
	if data["user_id"].(int64) <= 0 {
		t.Fatalf("user_id = %v", data["user_id"])
	}

	// 落库校验：昵称、密码已哈希
	user, err := svcCtx.UserModel.FindByUsername("carol")
	if err != nil {
		t.Fatalf("FindByUsername: %v", err)
	}
	if user.Nickname.String != "Carol" || !user.Nickname.Valid {
		t.Fatalf("nickname = %+v", user.Nickname)
	}
	if user.Password == "pw-carol" {
		t.Fatal("password must be hashed, not stored in plain text")
	}
	if !utils.CheckPassword("pw-carol", user.Password) {
		t.Fatal("hashed password must verify")
	}
}

func TestRegisterLogicNicknameDefaultsToUsername(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	l := NewRegisterLogic(context.Background(), svcCtx)

	resp, err := l.Register(&types.RegisterRequest{
		Username: "dave",
		Email:    "dave@example.cn",
		Password: "pw-dave",
	})
	if err != nil || envelopeCode(t, resp) != 200 {
		t.Fatalf("register failed: err=%v resp=%+v", err, resp)
	}
	user, err := svcCtx.UserModel.FindByUsername("dave")
	if err != nil {
		t.Fatalf("FindByUsername: %v", err)
	}
	// .cn 邮箱也合法；昵称缺省回退用户名
	if user.Email != "dave@example.cn" || user.Nickname.String != "dave" {
		t.Fatalf("user = %+v", user)
	}
}

func TestRegisterLogicValidation(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	l := NewRegisterLogic(context.Background(), svcCtx)

	cases := []struct {
		name string
		req  types.RegisterRequest
	}{
		{"short username", types.RegisterRequest{Username: "ab", Email: "a@b.com", Password: "pw-long"}},
		{"long username", types.RegisterRequest{Username: string(make([]byte, 51)), Email: "a@b.com", Password: "pw-long"}},
		{"empty email", types.RegisterRequest{Username: "carol", Email: "", Password: "pw-long"}},
		{"bad email suffix", types.RegisterRequest{Username: "carol", Email: "a@b.org", Password: "pw-long"}},
		{"short password", types.RegisterRequest{Username: "carol", Email: "a@b.com", Password: "pw"}},
		{"long password", types.RegisterRequest{Username: "carol", Email: "a@b.com", Password: string(make([]byte, 51))}},
	}
	for _, tc := range cases {
		resp, err := l.Register(&tc.req)
		if err != nil {
			t.Fatalf("%s: expected envelope, got error %v", tc.name, err)
		}
		if code := envelopeCode(t, resp); code != 400 {
			t.Fatalf("%s: code = %d, want 400 (%s)", tc.name, code, resp.Message)
		}
	}
}

func TestRegisterLogicDuplicates(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	l := NewRegisterLogic(context.Background(), svcCtx)

	base := &types.RegisterRequest{Username: "carol", Email: "carol@example.com", Password: "pw-carol"}
	if resp, err := l.Register(base); err != nil || envelopeCode(t, resp) != 200 {
		t.Fatalf("seed register failed: err=%v resp=%+v", err, resp)
	}

	dupUser := *base
	dupUser.Email = "other@example.com"
	if resp, err := l.Register(&dupUser); err != nil {
		t.Fatalf("err = %v", err)
	} else if code := envelopeCode(t, resp); code != 409 {
		t.Fatalf("dup username code = %d (%s)", code, resp.Message)
	}

	dupEmail := *base
	dupEmail.Username = "dave"
	if resp, err := l.Register(&dupEmail); err != nil {
		t.Fatalf("err = %v", err)
	} else if code := envelopeCode(t, resp); code != 409 {
		t.Fatalf("dup email code = %d (%s)", code, resp.Message)
	}
}

// --- GetUserInfoLogic ---

func TestGetUserInfoLogic(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	id := insertTestUser(t, svcCtx.DB, "alice", "alice@example.com", "hashed", "Alice")
	l := NewGetUserInfoLogic(context.Background(), svcCtx)

	// id <= 0 → 400
	if resp, err := l.GetUserInfo(&types.GetUserInfoRequest{Id: 0}); err != nil {
		t.Fatalf("err = %v", err)
	} else if code := envelopeCode(t, resp); code != 400 {
		t.Fatalf("id=0 code = %d", code)
	}

	// 不存在 → 404
	if resp, err := l.GetUserInfo(&types.GetUserInfoRequest{Id: 999}); err != nil {
		t.Fatalf("err = %v", err)
	} else if code := envelopeCode(t, resp); code != 404 {
		t.Fatalf("missing code = %d", code)
	}

	// 成功：昵称有效
	resp, err := l.GetUserInfo(&types.GetUserInfoRequest{Id: id})
	if err != nil || envelopeCode(t, resp) != 200 {
		t.Fatalf("get: err=%v resp=%+v", err, resp)
	}
	info := resp.Data.(*types.UserInfoResponse)
	if info.Username != "alice" || info.Nickname != "Alice" || info.Email != "alice@example.com" {
		t.Fatalf("info = %+v", info)
	}
}

func TestGetUserInfoLogicNicknameFallback(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	// nickname 列存 NULL（空串扫描后 Valid=true，不走回退分支）
	now := time.Now()
	res, err := svcCtx.DB.Exec(`INSERT INTO users (username, email, password, nickname, created_at, updated_at) VALUES (?, ?, ?, NULL, ?, ?)`,
		"bob", "bob@example.com", "hashed", now, now)
	if err != nil {
		t.Fatalf("insert bob: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	l := NewGetUserInfoLogic(context.Background(), svcCtx)

	resp, err := l.GetUserInfo(&types.GetUserInfoRequest{Id: id})
	if err != nil || envelopeCode(t, resp) != 200 {
		t.Fatalf("get: err=%v resp=%+v", err, resp)
	}
	// 昵称为 NULL → 回退用户名
	if info := resp.Data.(*types.UserInfoResponse); info.Nickname != "bob" {
		t.Fatalf("nickname = %q, want fallback username", info.Nickname)
	}
}

// --- UpdateUserInfoLogic ---

func TestUpdateUserInfoLogic(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	id := insertTestUser(t, svcCtx.DB, "alice", "alice@example.com", "hashed", "Alice")
	l := NewUpdateUserInfoLogic(context.Background(), svcCtx)

	// id <= 0 → 400
	if resp, err := l.UpdateUserInfo(&types.UpdateUserInfoRequest{Id: -1}); err != nil {
		t.Fatalf("err = %v", err)
	} else if code := envelopeCode(t, resp); code != 400 {
		t.Fatalf("id=-1 code = %d", code)
	}

	// 坏邮箱 → 400
	if resp, err := l.UpdateUserInfo(&types.UpdateUserInfoRequest{Id: id, Email: "nope", Nickname: "x"}); err != nil {
		t.Fatalf("err = %v", err)
	} else if code := envelopeCode(t, resp); code != 400 {
		t.Fatalf("bad email code = %d", code)
	}

	// 不存在 → 404
	if resp, err := l.UpdateUserInfo(&types.UpdateUserInfoRequest{Id: 999, Email: "a@b.com", Nickname: "x"}); err != nil {
		t.Fatalf("err = %v", err)
	} else if code := envelopeCode(t, resp); code != 404 {
		t.Fatalf("missing code = %d", code)
	}

	// 成功更新昵称 + 邮箱
	resp, err := l.UpdateUserInfo(&types.UpdateUserInfoRequest{Id: id, Email: "alice@new.com", Nickname: "Alicia"})
	if err != nil || envelopeCode(t, resp) != 200 {
		t.Fatalf("update: err=%v resp=%+v", err, resp)
	}
	info := resp.Data.(*types.UserInfoResponse)
	if info.Nickname != "Alicia" || info.Email != "alice@new.com" {
		t.Fatalf("info = %+v", info)
	}

	// 空昵称 → 回退用户名
	resp, err = l.UpdateUserInfo(&types.UpdateUserInfoRequest{Id: id, Email: "alice@new.com", Nickname: ""})
	if err != nil || envelopeCode(t, resp) != 200 {
		t.Fatalf("update2: err=%v resp=%+v", err, resp)
	}
	if info := resp.Data.(*types.UserInfoResponse); info.Nickname != "alice" {
		t.Fatalf("nickname = %q, want username fallback", info.Nickname)
	}
}

// --- UserLogic（示例路由 /from/:name）---

func TestUserLogicGreets(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	l := NewUserLogic(context.Background(), svcCtx)

	resp, err := l.User(&types.Request{Name: "you"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if resp.Message != "Hello, you!" {
		t.Fatalf("message = %q", resp.Message)
	}
}

// --- LoginLogic 空字段分支（补齐既有 Login 测试未覆盖路径）---

func TestLoginLogicEmptyFields(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	l := NewLoginLogic(context.Background(), svcCtx)

	if _, err := l.Login(&types.LoginRequest{Username: "", Password: "x"}); err == nil || err.Error() != "用户名不能为空" {
		t.Fatalf("err = %v, want 用户名不能为空", err)
	}
	if _, err := l.Login(&types.LoginRequest{Username: "alice", Password: ""}); err == nil || err.Error() != "密码不能为空" {
		t.Fatalf("err = %v, want 密码不能为空", err)
	}
	// 用户不存在 → 统一报"用户名或密码错误"（不泄露存在性）
	if _, err := l.Login(&types.LoginRequest{Username: "ghost", Password: "whatever"}); err == nil || err.Error() != "用户名或密码错误" {
		t.Fatalf("err = %v, want 用户名或密码错误", err)
	}
}

func TestLoginLogicNullNicknameFallsBack(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	hashed, err := utils.HashPassword("pw-nn")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	now := time.Now()
	if _, err := svcCtx.DB.Exec(`INSERT INTO users (username, email, password, nickname, created_at, updated_at) VALUES (?, ?, ?, NULL, ?, ?)`,
		"nonick", "nonick@example.com", hashed, now, now); err != nil {
		t.Fatalf("seed: %v", err)
	}

	l := NewLoginLogic(context.Background(), svcCtx)
	resp, err := l.Login(&types.LoginRequest{Username: "nonick", Password: "pw-nn"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if resp.UserInfo.Nickname != "nonick" {
		t.Fatalf("nickname = %q, want username fallback", resp.UserInfo.Nickname)
	}
}

// TestRegisterLogicDBError 关库故障注入：Check*Exists 出错 → 500 信封。
// 其余 500 分支（Create/HashPassword/二次 FindOne）需调用中途故障，无可达路径。
func TestRegisterLogicDBError(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	if err := svcCtx.DB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	l := NewRegisterLogic(context.Background(), svcCtx)
	resp, err := l.Register(&types.RegisterRequest{
		Username: "carol", Email: "carol@example.com", Password: "pw-carol",
	})
	if err != nil {
		t.Fatalf("expected envelope, got error %v", err)
	}
	if resp.Code != 500 || resp.Message != "服务器内部错误" {
		t.Fatalf("resp = %+v", resp)
	}
}

// TestRegisterLogicHashPasswordError RegisterLogic 中的 HashPassword 错误分支不可达：
// ValidatePassword 限制密码 ≤50 字符，bcrypt 限制 72 字节，前置校验拦截了所有会导致
// HashPassword 失败的输入。该分支在 RegisterLogic 中属死代码，登记台账不硬造用例。
// （HashPassword 直接调用的错误路径由 utils.TestHashPassword_TooLong 覆盖。）

// --- GetUserRecommendations 用户不存在分支 ---

func TestGetUserRecommendationsUserMissing(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	l := NewGetUserRecommendationsLogic(context.Background(), svcCtx)

	// 空库：任何 id 都不存在 → 404 envelope（不触发上游调用）
	resp, err := l.GetUserRecommendations(&types.GetUserRecommendationsRequest{Id: 42, Limit: 5})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if resp.Code != 404 || resp.Message != "用户不存在" {
		t.Fatalf("resp = %+v", resp)
	}
}

// TestGetUserRecommendationsNullNickname NULL 昵称用户成功路径 → 昵称回退用户名。
func TestGetUserRecommendationsNullNickname(t *testing.T) {
	svcCtx := newLogicSvcCtx(t)
	now := time.Now()
	res, err := svcCtx.DB.Exec(`INSERT INTO users (username, email, password, nickname, created_at, updated_at) VALUES (?, ?, ?, NULL, ?, ?)`,
		"reco-nn", "reco-nn@example.com", "hashed", now, now)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}

	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"games":[{"id":"g-1","title":"Alpha RPG","cover_image":"a.png","genres":["RPG"],"platforms":["PC"],"score":9.2,"tags":["story"]}]}`)
	}))
	defer stub.Close()
	svcCtx.GameCatalogClient = integration.NewGameCatalogClient(stub.URL, 2*time.Second)

	l := NewGetUserRecommendationsLogic(context.Background(), svcCtx)
	resp, err := l.GetUserRecommendations(&types.GetUserRecommendationsRequest{Id: id, Limit: 5})
	if err != nil || resp.Code != 200 {
		t.Fatalf("reco: err=%v resp=%+v", err, resp)
	}
	data := resp.Data.(map[string]interface{})
	if data["user_nickname"] != "reco-nn" {
		t.Fatalf("user_nickname = %v, want username fallback", data["user_nickname"])
	}
	recs := data["recommendations"].([]types.GameRecommendation)
	if len(recs) != 1 || recs[0].Title != "Alpha RPG" {
		t.Fatalf("recommendations = %+v", recs)
	}
}

// --- GetUserRecommendations 辅助函数边界 ---

func TestParseGenres(t *testing.T) {
	if got := parseGenres(""); got != nil {
		t.Fatalf("empty genres = %v, want nil", got)
	}
	got := parseGenres(" RPG , SIM")
	if len(got) != 2 || got[0] != "RPG" || got[1] != "SIM" {
		t.Fatalf("parseGenres = %v", got)
	}
	// 空段（连续逗号）被跳过
	if got = parseGenres("RPG,,SIM,"); len(got) != 2 {
		t.Fatalf("parseGenres with empty segments = %v", got)
	}
}

func TestClampInt(t *testing.T) {
	if got := clampInt(0, 1, 20); got != 1 {
		t.Fatalf("clamp(0) = %d, want 1", got)
	}
	if got := clampInt(99, 1, 20); got != 20 {
		t.Fatalf("clamp(99) = %d, want 20", got)
	}
	if got := clampInt(5, 1, 20); got != 5 {
		t.Fatalf("clamp(5) = %d, want 5", got)
	}
}
