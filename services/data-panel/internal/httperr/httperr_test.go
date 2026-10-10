package httperr

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"

	"github.com/tappi/tappi/services/data-panel/internal/model"
)

func TestErrorHandler_AppError(t *testing.T) {
	t.Parallel()

	status, body := ErrorHandler(context.Background(), Unauthorized("nope"))
	if status != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, status)
	}
	resp, ok := body.(Response)
	if !ok {
		t.Fatalf("expected Response body, got %T", body)
	}
	if resp.Code != http.StatusUnauthorized || resp.Message != "nope" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestErrorHandler_ModelNotFound(t *testing.T) {
	t.Parallel()

	status, body := ErrorHandler(context.Background(), model.ErrStatNotFound)
	if status != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, status)
	}
	resp := body.(Response)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected code %d, got %d", http.StatusNotFound, resp.Code)
	}
}

func TestErrorHandler_PathErrorIsInternal(t *testing.T) {
	t.Parallel()

	status, body := ErrorHandler(context.Background(), &os.PathError{Op: "open", Path: "/nope", Err: os.ErrNotExist})
	if status != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, status)
	}
	resp := body.(Response)
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected code %d, got %d", http.StatusInternalServerError, resp.Code)
	}
}

func TestError_NilReceiver(t *testing.T) {
	t.Parallel()

	var e *Error
	if e.Error() != "" {
		t.Fatalf("nil receiver Error() = %q, want empty", e.Error())
	}
	if got := BadRequest("boom").Error(); got != "boom" {
		t.Fatalf("Error() = %q, want message", got)
	}
}

func TestConstructors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		err     *Error
		status  int
		code    int64
		message string
	}{
		{"bad request", BadRequest("bad input"), http.StatusBadRequest, http.StatusBadRequest, "bad input"},
		{"forbidden", Forbidden("denied"), http.StatusForbidden, http.StatusForbidden, "denied"},
		{"not found", NotFound("gone"), http.StatusNotFound, http.StatusNotFound, "gone"},
		{"internal fallback", Internal("   "), http.StatusInternalServerError, http.StatusInternalServerError, "internal server error"},
		{"internal passthrough", Internal("boom"), http.StatusInternalServerError, http.StatusInternalServerError, "boom"},
	}
	for _, tc := range cases {
		if tc.err.Status != tc.status || tc.err.Code != tc.code || tc.err.Message != tc.message {
			t.Fatalf("%s: got %+v, want status=%d code=%d message=%q",
				tc.name, tc.err, tc.status, tc.code, tc.message)
		}
	}
}

// TestErrorHandler_StringFallbacks 裸 error 的字符串启发式分支：
// "unauthorized" 全等 → 401；含 "permission denied" → 403；其余 → 400。
func TestErrorHandler_StringFallbacks(t *testing.T) {
	t.Parallel()

	status, body := ErrorHandler(context.Background(), errors.New("unauthorized"))
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401", status)
	}
	if resp := body.(Response); resp.Message != "unauthorized" {
		t.Fatalf("unauthorized message = %q", resp.Message)
	}

	// 非 model.ErrPermissionDenied 的普通 error，仅靠子串匹配
	status, body = ErrorHandler(context.Background(), errors.New("topic permission denied by owner"))
	if status != http.StatusForbidden {
		t.Fatalf("substring permission status = %d, want 403", status)
	}
	if resp := body.(Response); resp.Code != http.StatusForbidden {
		t.Fatalf("substring permission code = %d", resp.Code)
	}

	status, body = ErrorHandler(context.Background(), errors.New("weird input"))
	if status != http.StatusBadRequest {
		t.Fatalf("fallback status = %d, want 400", status)
	}
	if resp := body.(Response); resp.Message != "weird input" {
		t.Fatalf("fallback message = %q", resp.Message)
	}
}
