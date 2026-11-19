package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestETagMatches(t *testing.T) {
	etag := `"abc123"`
	cases := map[string]bool{
		"":                  false,
		`"other"`:           false,
		etag:                true,
		`W/"abc123"`:        true,
		`"one", "abc123"`:   true,
		`"one", W/"abc123"`: true,
		`*`:                 true,
	}
	for header, want := range cases {
		if got := ETagMatches(header, etag); got != want {
			t.Fatalf("ETagMatches(%q) = %v, want %v", header, got, want)
		}
	}
}

func TestWriteJSONETagRepliesNotModified(t *testing.T) {
	payload := map[string]string{"status": "ok"}

	first := httptest.NewRecorder()
	WriteJSONETag(first, httptest.NewRequest(http.MethodGet, "/api/v1/article-columns", nil), payload)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d", first.Code)
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag header")
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/article-columns", nil)
	request.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	WriteJSONETag(second, request, payload)
	if second.Code != http.StatusNotModified {
		t.Fatalf("second status = %d, want 304", second.Code)
	}
	if second.Body.Len() != 0 {
		t.Fatalf("304 body should be empty, got %q", second.Body.String())
	}
}

func TestWriteJSONETagChangesWithBody(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/photos", nil)
	first := httptest.NewRecorder()
	WriteJSONETag(first, request, map[string]string{"title": "一"})
	second := httptest.NewRecorder()
	WriteJSONETag(second, request, map[string]string{"title": "二"})
	if first.Header().Get("ETag") == second.Header().Get("ETag") {
		t.Fatal("different bodies must produce different ETags")
	}
}
