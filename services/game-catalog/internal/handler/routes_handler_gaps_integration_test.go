package handler

// 补漏 handler 错误臂：合作方列表/版本、包体上传 multipart 边角、预约取消/
// 我的预约/预约人群、区服列表、受控下载（parse 臂 + 非 apk 内容类型 +
// 文件名回退与字符净化）。与 routes_parse_errors / routes_logic_errors /
// routes_store_errors 互补。

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/config"
	"github.com/tappi/tappi/services/game-catalog/utils"
)

// postMultipartText 以 multipart/form-data 仅发文本字段（无文件字段）。
func postMultipartText(t *testing.T, url, token string, fields map[string]string) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("write field %s: %v", k, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("multipart text %s: %v", url, err)
	}
	return resp.StatusCode, readAll(t, resp)
}

// setupPartnerGrant 建合作方、授权 user2 为 partner、指派 game-elden-ring 归属；
// 返回 user2 的令牌。
func setupPartnerGrant(t *testing.T, base string, sign func(claims utils.JWTClaims) string) string {
	t.Helper()
	admin := sign(claimsFor(1, "admin"))
	_, body := doAuthed(t, http.MethodPost, base+"/admin/partners", admin, `{"name":"缺口厂"}`)
	var partnerResp struct {
		Id int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &partnerResp)
	doAuthed(t, http.MethodPost, base+"/admin/roles", admin,
		`{"user_id":2,"role":"partner","partner_id":`+jsonNumber(partnerResp.Id)+`}`)
	doAuthed(t, http.MethodPost, base+"/admin/games/game-elden-ring/ownership", admin,
		`{"partner_id":`+jsonNumber(partnerResp.Id)+`}`)
	return sign(claimsFor(2, "carol"))
}

// publishPartnerVersion 建版本（draft）→ 传包 → 提审 → 过审 → 发布，返回版本 id。
func publishPartnerVersion(t *testing.T, base, user2, admin, versionJSON, fileName string, content []byte) string {
	t.Helper()
	_, body := doAuthed(t, http.MethodPost, base+"/partner/games/game-elden-ring/versions", user2, versionJSON)
	var verResp struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &verResp); err != nil || verResp.Id == "" {
		t.Fatalf("create version: body=%s err=%v", body, err)
	}
	if code, _ := postMultipart(t, base+"/partner/games/game-elden-ring/versions/"+verResp.Id+"/package",
		user2, "file", fileName, content); code != http.StatusOK {
		t.Fatalf("upload package = %d", code)
	}
	_, body = doAuthed(t, http.MethodPost, base+"/partner/games/game-elden-ring/submit", user2,
		`{"version_id":"`+verResp.Id+`"}`)
	var submitResp struct {
		Id int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &submitResp)
	if code, _ := doAuthed(t, http.MethodPost, base+"/admin/reviews/"+jsonNumber(submitResp.Id)+"/decision", admin,
		`{"decision":"approve"}`); code != http.StatusOK {
		t.Fatalf("approve = %d", code)
	}
	if code, _ := doAuthed(t, http.MethodPost, base+"/admin/games/game-elden-ring/versions/"+verResp.Id+"/publish", admin, ""); code != http.StatusOK {
		t.Fatalf("publish = %d", code)
	}
	return verResp.Id
}

// 合作方列表/版本 handler 错误臂。
func TestRoutes_PartnerHandlerGaps(t *testing.T) {
	t.Run("list games logic error 403", func(t *testing.T) {
		base, sign, svcCtx := newAuthedTestServerSvc(t, nil)
		user2 := setupPartnerGrant(t, base, sign)
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore: svcCtx.AdminRepository,
			getRoleErr: errors.New("role down"),
		}
		if code, _ := doAuthed(t, http.MethodGet, base+"/partner/games", user2, ""); code != http.StatusForbidden {
			t.Fatalf("list partner games = %d, want 403", code)
		}
	})
	t.Run("create version empty 400", func(t *testing.T) {
		base, sign := newAuthedTestServer(t)
		user2 := setupPartnerGrant(t, base, sign)
		if code, _ := doAuthed(t, http.MethodPost, base+"/partner/games/game-elden-ring/versions", user2,
			`{"version":" "}`); code != http.StatusBadRequest {
			t.Fatalf("create version empty = %d, want 400", code)
		}
	})
	t.Run("list versions parse 400", func(t *testing.T) {
		base, sign := newAuthedTestServer(t)
		user2 := setupPartnerGrant(t, base, sign)
		if code, _ := doAuthed(t, http.MethodGet, base+"/partner/games/game-elden-ring/versions", user2, `{`); code != http.StatusBadRequest {
			t.Fatalf("list versions parse = %d, want 400", code)
		}
	})
	t.Run("list versions unknown game 404", func(t *testing.T) {
		base, sign := newAuthedTestServer(t)
		user2 := setupPartnerGrant(t, base, sign)
		if code, _ := doAuthed(t, http.MethodGet, base+"/partner/games/g-ghost/versions", user2, ""); code != http.StatusNotFound {
			t.Fatalf("list versions unknown = %d, want 404", code)
		}
	})
}

// 包体上传 multipart 边角：无文件字段（EOF break + 排空 + 缺文件 400）与
// 解析错误（NextPart 读失败 → 400）。
func TestRoutes_UploadPackageGaps(t *testing.T) {
	t.Run("no file field 400", func(t *testing.T) {
		base, sign := newAuthedTestServer(t)
		user2 := setupPartnerGrant(t, base, sign)
		code, body := postMultipartText(t, base+"/partner/games/game-elden-ring/versions/ver-x/package",
			user2, map[string]string{"note": "hello"})
		if code != http.StatusBadRequest || !strings.Contains(body, "缺少文件字段") {
			t.Fatalf("no file field = %d body=%s, want 400 缺少文件字段", code, body)
		}
	})
	t.Run("next part error 400", func(t *testing.T) {
		base, sign := newAuthedTestServer(t)
		user2 := setupPartnerGrant(t, base, sign)
		// 边界合法但 part 头缺冒号 → mime 解析失败（非 EOF）→ NextPart 报错。
		var body bytes.Buffer
		body.WriteString("--xyz\r\n")
		body.WriteString("not-a-valid-header\r\n")
		body.WriteString("\r\ndata\r\n--xyz--\r\n")
		req, _ := http.NewRequest(http.MethodPost,
			base+"/partner/games/game-elden-ring/versions/ver-x/package", &body)
		req.Header.Set("Content-Type", "multipart/form-data; boundary=xyz")
		req.Header.Set("Authorization", "Bearer "+user2)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("malformed multipart: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("next part error = %d, want 400", resp.StatusCode)
		}
	})
}

// 预约 handler 错误臂：取消 parse/404、我的预约存储失败、预约人群 parse/存储失败。
func TestRoutes_ReservationHandlerGaps(t *testing.T) {
	t.Run("cancel parse 400", func(t *testing.T) {
		base, sign := newAuthedTestServer(t)
		player := sign(claimsFor(7, "bob"))
		if code, _ := doAuthed(t, http.MethodDelete, base+"/games/game-elden-ring/servers/1/reserve", player, `{`); code != http.StatusBadRequest {
			t.Fatalf("cancel parse = %d, want 400", code)
		}
	})
	t.Run("cancel unknown server 404", func(t *testing.T) {
		base, sign := newAuthedTestServer(t)
		player := sign(claimsFor(7, "bob"))
		if code, _ := doAuthed(t, http.MethodDelete, base+"/games/g-ghost/servers/999/reserve", player, ""); code != http.StatusNotFound {
			t.Fatalf("cancel unknown = %d, want 404", code)
		}
	})
	t.Run("my reservations store 500", func(t *testing.T) {
		base, sign, svcCtx := newAuthedTestServerSvc(t, nil)
		player := sign(claimsFor(7, "bob"))
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:      svcCtx.AdminRepository,
			listMyReservErr: errors.New("list down"),
		}
		if code, _ := doAuthed(t, http.MethodGet, base+"/games/game-elden-ring/reservations/me", player, ""); code != http.StatusInternalServerError {
			t.Fatalf("my reservations = %d, want 500", code)
		}
	})
	t.Run("internal users parse 400", func(t *testing.T) {
		base, _ := newAuthedTestServerConfig(t, func(c *config.Config) { c.Internal.Token = "s2s-token" })
		req, _ := http.NewRequest(http.MethodGet, base+"/internal/games/game-elden-ring/reservation-users",
			strings.NewReader(`{`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Internal-Token", "s2s-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("internal users parse: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("internal users parse = %d, want 400", resp.StatusCode)
		}
	})
	t.Run("internal users store 500", func(t *testing.T) {
		base, _, svcCtx := newAuthedTestServerSvc(t, func(c *config.Config) { c.Internal.Token = "s2s-token" })
		svcCtx.AdminRepository = &failingAdminStore{
			AdminStore:      svcCtx.AdminRepository,
			listResUsersErr: errors.New("list down"),
		}
		req, _ := http.NewRequest(http.MethodGet, base+"/internal/games/game-elden-ring/reservation-users", nil)
		req.Header.Set("X-Internal-Token", "s2s-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("internal users store: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("internal users store = %d, want 500", resp.StatusCode)
		}
	})
}

// 区服列表 handler 错误臂：合作方列表 parse、公开列表 parse/未知游戏 404。
func TestRoutes_ServerHandlerGaps(t *testing.T) {
	t.Run("partner list servers parse 400", func(t *testing.T) {
		base, sign := newAuthedTestServer(t)
		user2 := setupPartnerGrant(t, base, sign)
		if code, _ := doAuthed(t, http.MethodGet, base+"/partner/games/game-elden-ring/servers", user2, `{`); code != http.StatusBadRequest {
			t.Fatalf("partner servers parse = %d, want 400", code)
		}
	})
	t.Run("public list servers parse 400", func(t *testing.T) {
		base, _ := newAuthedTestServer(t)
		if code, _ := doAuthed(t, http.MethodGet, base+"/games/game-elden-ring/servers", "", `{`); code != http.StatusBadRequest {
			t.Fatalf("public servers parse = %d, want 400", code)
		}
	})
	t.Run("public list servers unknown game 404", func(t *testing.T) {
		base, _ := newAuthedTestServer(t)
		if code, _ := doAuthed(t, http.MethodGet, base+"/games/g-ghost/servers", "", ""); code != http.StatusNotFound {
			t.Fatalf("public servers unknown = %d, want 404", code)
		}
	})
}

// 受控下载 handler 边角：parse 400、非 apk 内容类型、空包名回退、文件名净化。
func TestRoutes_DownloadHandlerGaps(t *testing.T) {
	t.Run("download parse 400", func(t *testing.T) {
		base, sign := newAuthedTestServer(t)
		player := sign(claimsFor(7, "bob"))
		if code, _ := doAuthed(t, http.MethodGet, base+"/games/game-elden-ring/versions/v1/download", player, `{`); code != http.StatusBadRequest {
			t.Fatalf("download parse = %d, want 400", code)
		}
	})
	t.Run("non apk content type and filename sanitize", func(t *testing.T) {
		base, sign := newAuthedTestServer(t)
		admin := sign(claimsFor(1, "admin"))
		user2 := setupPartnerGrant(t, base, sign)
		player := sign(claimsFor(7, "bob"))

		// 非 apk 扩展名 → 默认 octet-stream
		v1 := publishPartnerVersion(t, base, user2, admin,
			`{"version":"2.0.0","package_name":"com.test.bin"}`, "pkg.bin", []byte("BIN1"))
		// 空包名 → attachmentName 回退 "package"
		v2 := publishPartnerVersion(t, base, user2, admin,
			`{"version":"3.0.0"}`, "pkg.bin", []byte("BIN2"))
		// 引号/反斜杠/斜杠 → 净化替换为 '-'
		v3 := publishPartnerVersion(t, base, user2, admin,
			`{"version":"4.0.0","package_name":"a\"b\\c/d"}`, "pkg.bin", []byte("BIN3"))

		get := func(verID string) *http.Response {
			req, _ := http.NewRequest(http.MethodGet,
				base+"/games/game-elden-ring/versions/"+verID+"/download", nil)
			req.Header.Set("Authorization", "Bearer "+player)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("download %s: %v", verID, err)
			}
			readAll(t, resp)
			return resp
		}
		r1 := get(v1)
		if r1.StatusCode != http.StatusOK || r1.Header.Get("Content-Type") != "application/octet-stream" {
			t.Fatalf("non apk download = %d ct=%s, want 200 octet-stream", r1.StatusCode, r1.Header.Get("Content-Type"))
		}
		r2 := get(v2)
		if r2.StatusCode != http.StatusOK || !strings.HasPrefix(r2.Header.Get("Content-Disposition"), "attachment; filename=\"package-") {
			t.Fatalf("empty package name cd=%q, want package- prefix", r2.Header.Get("Content-Disposition"))
		}
		r3 := get(v3)
		if r3.StatusCode != http.StatusOK || !strings.Contains(r3.Header.Get("Content-Disposition"), "a-b-c-d-") {
			t.Fatalf("sanitized package name cd=%q, want a-b-c-d-", r3.Header.Get("Content-Disposition"))
		}
	})
}
