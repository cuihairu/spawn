package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// postMultipart 以 multipart/form-data 上传文件字段（GP-105 包体上传）。
func postMultipart(t *testing.T, url, token, fieldName, fileName string, content []byte) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(fieldName, fileName)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload %s: %v", url, err)
	}
	return resp.StatusCode, readAll(t, resp)
}

// TestRoutes_PackageDownload 覆盖 GP-105 包体托管与受控下载闭环：
// 合作方上传（multipart）→ 提审/过审/发布 → 登录下载（Range 续传、sha256 头）→
// 撤回后 404；latest 公开元数据不泄露 storage_url。
func TestRoutes_PackageDownload(t *testing.T) {
	base, sign := newAuthedTestServer(t)
	admin := sign(claimsFor(1, "admin"))
	user2 := sign(claimsFor(2, "carol"))
	player := sign(claimsFor(7, "bob"))
	var code int

	// 建合作方、授权 user2、指派 game-elden-ring 归属
	_, body := doAuthed(t, http.MethodPost, base+"/admin/partners", admin, `{"name":"包体厂"}`)
	var partnerResp struct {
		Id int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &partnerResp)
	if code, _ := doAuthed(t, http.MethodPost, base+"/admin/roles", admin,
		`{"user_id":2,"role":"partner","partner_id":`+jsonNumber(partnerResp.Id)+`}`); code != http.StatusOK {
		t.Fatalf("grant partner = %d", code)
	}
	if code, _ := doAuthed(t, http.MethodPost, base+"/admin/games/game-elden-ring/ownership", admin,
		`{"partner_id":`+jsonNumber(partnerResp.Id)+`}`); code != http.StatusOK {
		t.Fatalf("assign ownership = %d", code)
	}

	// 建版本（draft）
	if code, body = doAuthed(t, http.MethodPost, base+"/partner/games/game-elden-ring/versions", user2,
		`{"version":"1.0.0","package_name":"com.ring.elden","version_code":10}`); code != http.StatusOK || !strings.Contains(body, `"status":"draft"`) {
		t.Fatalf("create version = %d body=%s", code, body)
	}
	var verResp struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &verResp); err != nil {
		t.Fatalf("decode version: %v (%s)", err, body)
	}

	// 无已发布版本时 latest → 404
	if code, _ := doAuthed(t, http.MethodGet, base+"/games/game-elden-ring/versions/latest", player, ""); code != http.StatusNotFound {
		t.Fatalf("latest before publish = %d, want 404", code)
	}

	// 下载未发布版本 → 404（不泄露存在性）
	if code, _ := doAuthed(t, http.MethodGet, base+"/games/game-elden-ring/versions/"+verResp.Id+"/download", player, ""); code != http.StatusNotFound {
		t.Fatalf("download draft = %d, want 404", code)
	}

	// 匿名下载 → 401（受控下载要求登录）
	resp, err := http.Get(base + "/games/game-elden-ring/versions/" + verResp.Id + "/download")
	if err != nil {
		t.Fatalf("anonymous download: %v", err)
	}
	if code := resp.StatusCode; code != http.StatusUnauthorized {
		readAll(t, resp)
		t.Fatalf("anonymous download = %d, want 401", code)
	}

	// 玩家上传包体 → 403（partner 角色守卫）
	payload := []byte("PK\x03\x04fake-apk-payload-0123456789")
	wantHash := sha256.Sum256(payload)
	wantHashHex := hex.EncodeToString(wantHash[:])
	if code, _ := postMultipart(t, base+"/partner/games/game-elden-ring/versions/"+verResp.Id+"/package",
		player, "file", "demo.apk", payload); code != http.StatusForbidden {
		t.Fatalf("player upload = %d, want 403", code)
	}

	// 合作方上传包体 → 200，服务端回填哈希/大小/托管地址
	code, body = postMultipart(t, base+"/partner/games/game-elden-ring/versions/"+verResp.Id+"/package",
		user2, "file", "demo.apk", payload)
	if code != http.StatusOK {
		t.Fatalf("partner upload = %d body=%s", code, body)
	}
	var upResp struct {
		StorageUrl string `json:"storage_url"`
		Hash       string `json:"hash"`
		SizeBytes  int64  `json:"size_bytes"`
	}
	if err := json.Unmarshal([]byte(body), &upResp); err != nil {
		t.Fatalf("decode upload resp: %v (%s)", err, body)
	}
	if !strings.HasPrefix(upResp.StorageUrl, "package://"+verResp.Id) {
		t.Fatalf("storage_url = %s, want package:// prefix", upResp.StorageUrl)
	}
	if upResp.Hash != "sha256:"+wantHashHex {
		t.Fatalf("hash = %s, want sha256:%s", upResp.Hash, wantHashHex)
	}
	if upResp.SizeBytes != int64(len(payload)) {
		t.Fatalf("size_bytes = %d, want %d", upResp.SizeBytes, len(payload))
	}

	// 提审 → 过审 → 发布
	if code, body = doAuthed(t, http.MethodPost, base+"/partner/games/game-elden-ring/submit", user2,
		`{"version_id":"`+verResp.Id+`"}`); code != http.StatusOK {
		t.Fatalf("submit version = %d body=%s", code, body)
	}
	var submitResp struct {
		Id int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &submitResp)
	if code, _ = doAuthed(t, http.MethodPost, base+"/admin/reviews/"+jsonNumber(submitResp.Id)+"/decision", admin,
		`{"decision":"approve"}`); code != http.StatusOK {
		t.Fatalf("approve = %d", code)
	}
	if code, _ = doAuthed(t, http.MethodPost, base+"/admin/games/game-elden-ring/versions/"+verResp.Id+"/publish", admin, ""); code != http.StatusOK {
		t.Fatalf("publish version = %d", code)
	}

	// latest 公开元数据：不含 storage_url，downloadable = true
	resp, err = http.Get(base + "/games/game-elden-ring/versions/latest")
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	body = readAll(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("latest = %d body=%s", resp.StatusCode, body)
	}
	var latestResp struct {
		Version struct {
			Id           string `json:"id"`
			StorageUrl   string `json:"storage_url"`
			Hash         string `json:"hash"`
			Downloadable bool   `json:"downloadable"`
		} `json:"version"`
	}
	if err := json.Unmarshal([]byte(body), &latestResp); err != nil {
		t.Fatalf("decode latest: %v (%s)", err, body)
	}
	if latestResp.Version.Id != verResp.Id || !latestResp.Version.Downloadable || latestResp.Version.Hash != "sha256:"+wantHashHex {
		t.Fatalf("latest version = %+v body=%s", latestResp.Version, body)
	}
	if latestResp.Version.StorageUrl != "" {
		t.Fatalf("latest leaked storage_url: %s", latestResp.Version.StorageUrl)
	}

	// 登录下载 → 200，字节一致 + sha256/大小/Accept-Ranges 头
	dlReq, _ := http.NewRequest(http.MethodGet, base+"/games/game-elden-ring/versions/"+verResp.Id+"/download", nil)
	dlReq.Header.Set("Authorization", "Bearer "+player)
	resp, err = http.DefaultClient.Do(dlReq)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	var got bytes.Buffer
	if _, err := got.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read download: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Equal(got.Bytes(), payload) {
		t.Fatalf("download status=%d len=%d, want 200 + %d bytes", resp.StatusCode, got.Len(), len(payload))
	}
	if resp.Header.Get("X-Package-Sha256") != wantHashHex {
		t.Fatalf("X-Package-Sha256 = %s, want %s", resp.Header.Get("X-Package-Sha256"), wantHashHex)
	}
	if resp.Header.Get("Content-Length") != strconv.Itoa(len(payload)) || resp.Header.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("download headers: len=%s ranges=%s", resp.Header.Get("Content-Length"), resp.Header.Get("Accept-Ranges"))
	}

	// Range 续传：bytes=0-3 → 206 + Content-Range
	rangeReq, _ := http.NewRequest(http.MethodGet, base+"/games/game-elden-ring/versions/"+verResp.Id+"/download", nil)
	rangeReq.Header.Set("Authorization", "Bearer "+player)
	rangeReq.Header.Set("Range", "bytes=0-3")
	resp, err = http.DefaultClient.Do(rangeReq)
	if err != nil {
		t.Fatalf("range download: %v", err)
	}
	var head bytes.Buffer
	if _, err := head.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read range: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent ||
		resp.Header.Get("Content-Range") != "bytes 0-3/"+strconv.Itoa(len(payload)) ||
		!bytes.Equal(head.Bytes(), payload[:4]) {
		t.Fatalf("range = %d range=%s len=%d, want 206 first 4 bytes", resp.StatusCode, resp.Header.Get("Content-Range"), head.Len())
	}

	// 越界 Range → 416
	badRangeReq, _ := http.NewRequest(http.MethodGet, base+"/games/game-elden-ring/versions/"+verResp.Id+"/download", nil)
	badRangeReq.Header.Set("Authorization", "Bearer "+player)
	badRangeReq.Header.Set("Range", "bytes=999999-")
	resp, err = http.DefaultClient.Do(badRangeReq)
	if err != nil {
		t.Fatalf("bad range download: %v", err)
	}
	readAll(t, resp)
	if resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("bad range = %d, want 416", resp.StatusCode)
	}

	// 已发布版本禁止换包（防审核后掉包）→ 409
	if code, _ := postMultipart(t, base+"/partner/games/game-elden-ring/versions/"+verResp.Id+"/package",
		user2, "file", "demo.apk", []byte("tampered")); code != http.StatusConflict {
		t.Fatalf("upload to published = %d, want 409", code)
	}

	// 上传到不存在的版本 → 404
	if code, _ := postMultipart(t, base+"/partner/games/game-elden-ring/versions/ver-no-such/package",
		user2, "file", "demo.apk", payload); code != http.StatusNotFound {
		t.Fatalf("upload missing version = %d, want 404", code)
	}

	// 审计包含包体上传动作
	if code, body = doAuthed(t, http.MethodGet, base+"/admin/audit", admin, ""); code != http.StatusOK || !strings.Contains(body, "version_package_upload") {
		t.Fatalf("audit list = %d body=%s", code, body)
	}

	// 撤回后下载 → 404
	if code, _ = doAuthed(t, http.MethodPost, base+"/admin/games/game-elden-ring/versions/"+verResp.Id+"/revoke", admin, ""); code != http.StatusOK {
		t.Fatalf("revoke = %d", code)
	}
	if code, _ = doAuthed(t, http.MethodGet, base+"/games/game-elden-ring/versions/"+verResp.Id+"/download", player, ""); code != http.StatusNotFound {
		t.Fatalf("download revoked = %d, want 404", code)
	}
}
