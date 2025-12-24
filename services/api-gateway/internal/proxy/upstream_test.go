package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUpstream_ForwardsPathQueryAndHeaders(t *testing.T) {
	t.Parallel()

	var gotPath, gotQuery, gotAuth string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(upstreamServer.Close)

	up, err := NewUpstream(upstreamServer.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewUpstream: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/v1/posts?limit=10", nil)
	req.Header.Set("Authorization", "Bearer test")
	rr := httptest.NewRecorder()

	up.Handler()(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}
	if gotPath != "/api/v1/posts" {
		t.Fatalf("expected upstream path %q, got %q", "/api/v1/posts", gotPath)
	}
	if gotQuery != "limit=10" {
		t.Fatalf("expected upstream query %q, got %q", "limit=10", gotQuery)
	}
	if gotAuth != "Bearer test" {
		t.Fatalf("expected auth header to be forwarded, got %q", gotAuth)
	}
}
