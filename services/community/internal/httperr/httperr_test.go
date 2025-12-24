package httperr

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/tappi/tappi/services/community/internal/model"
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

	status, body := ErrorHandler(context.Background(), model.ErrPostNotFound)
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
