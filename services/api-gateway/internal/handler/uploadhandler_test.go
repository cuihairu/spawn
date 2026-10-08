package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/tappi/tappi/services/api-gateway/internal/upload"
	"github.com/zeromicro/go-zero/rest/pathvar"
)

const testSecret = "test-upload-secret"

func newUploadCtx(t *testing.T) *svc.ServiceContext {
	t.Helper()
	return &svc.ServiceContext{
		UploadAuth:  upload.NewValidator(testSecret),
		UploadStore: &upload.Store{Dir: t.TempDir(), MaxBytes: 1 << 20},
	}
}

func uploadToken() string {
	claims := upload.UserClaims{
		UserId:   1,
		Username: "tester",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		panic(err)
	}
	return token
}

func uploadRequest(t *testing.T, token, filename, contentType string, body []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(body); err != nil {
		t.Fatal(err)
	}
	mw.Close()
	r := httptest.NewRequest(http.MethodPost, "/upload", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)

func TestUploadOK(t *testing.T) {
	ctx := newUploadCtx(t)
	r := uploadRequest(t, uploadToken(), "a.png", "image/png", pngBytes)
	w := httptest.NewRecorder()
	UploadHandler(ctx).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var resp UploadResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resp.Url, "/uploads/") || !strings.HasSuffix(resp.Url, ".png") {
		t.Fatalf("unexpected url %q", resp.Url)
	}
}

func TestUploadRejectsWithoutOrBadToken(t *testing.T) {
	ctx := newUploadCtx(t)
	for name, token := range map[string]string{"缺失": "", "无效": "bad-token"} {
		w := httptest.NewRecorder()
		UploadHandler(ctx).ServeHTTP(w, uploadRequest(t, token, "a.png", "image/png", pngBytes))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s: code=%d", name, w.Code)
		}
	}
}

func TestUploadRejectsNonImage(t *testing.T) {
	ctx := newUploadCtx(t)
	w := httptest.NewRecorder()
	UploadHandler(ctx).ServeHTTP(w, uploadRequest(t, uploadToken(), "a.txt", "text/plain", []byte("hello plain text here")))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestUploadRejectsOversize(t *testing.T) {
	ctx := newUploadCtx(t)
	big := append(pngBytes, bytes.Repeat([]byte{0}, 1<<20)...)
	w := httptest.NewRecorder()
	UploadHandler(ctx).ServeHTTP(w, uploadRequest(t, uploadToken(), "a.png", "image/png", big))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestUploadAcceptsJpeg(t *testing.T) {
	ctx := newUploadCtx(t)
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0}, bytes.Repeat([]byte{0}, 64)...)
	w := httptest.NewRecorder()
	UploadHandler(ctx).ServeHTTP(w, uploadRequest(t, uploadToken(), "a.jpg", "image/jpeg", jpeg))
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestUploadsFileHandlerServesAndGuards(t *testing.T) {
	ctx := newUploadCtx(t)

	// 先经上传入口落一张图。
	ur := uploadRequest(t, uploadToken(), "a.png", "image/png", pngBytes)
	uw := httptest.NewRecorder()
	UploadHandler(ctx).ServeHTTP(uw, ur)
	var resp UploadResponse
	if err := json.Unmarshal(uw.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	// 按返回 URL 回读。
	name := strings.TrimPrefix(resp.Url, "/uploads/")
	fr := httptest.NewRequest(http.MethodGet, resp.Url, nil)
	fr = pathvar.WithVars(fr, map[string]string{"file": name})
	fw := httptest.NewRecorder()
	UploadsFileHandler(ctx).ServeHTTP(fw, fr)
	if fw.Code != http.StatusOK {
		t.Fatalf("serve code=%d", fw.Code)
	}
	got, err := io.ReadAll(fw.Body)
	if err != nil || !bytes.Equal(got, pngBytes) {
		t.Fatalf("serve content mismatch: err=%v len=%d", err, len(got))
	}

	// 路径遍历与不存在的文件都 404。
	for _, p := range []string{"../etc/passwd", "ffffffffffffffffffffffffffffffff.txt", "0000000000000000000000000000000.png"} {
		br := httptest.NewRequest(http.MethodGet, "/uploads/"+p, nil)
		br = pathvar.WithVars(br, map[string]string{"file": p})
		bw := httptest.NewRecorder()
		UploadsFileHandler(ctx).ServeHTTP(bw, br)
		if bw.Code != http.StatusNotFound {
			t.Fatalf("guard %q: code=%d", p, bw.Code)
		}
	}

	// 落盘目录里确实只有这一个文件。
	entries, err := filepath.Glob(filepath.Join(ctx.UploadStore.Dir, "*"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("dir entries=%v err=%v", entries, err)
	}
}
