package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"
)

func newTestLocal(t *testing.T) *Local {
	t.Helper()
	local, err := NewLocal(t.TempDir(), "http://localhost:8080/assets", "http://localhost:8080", "test-signing-secret-0123456789")
	if err != nil {
		t.Fatalf("new local storage: %v", err)
	}
	return local
}

func TestLocalPutOpenStatDelete(t *testing.T) {
	local := newTestLocal(t)
	ctx := context.Background()
	key := "private/photo/2026/11/a.jpg"
	payload := []byte("hello-image")

	size, err := local.Put(ctx, key, bytes.NewReader(payload), 1024)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if size != int64(len(payload)) {
		t.Fatalf("size %d != %d", size, len(payload))
	}
	info, err := local.Stat(ctx, key)
	if err != nil || info.Size != int64(len(payload)) {
		t.Fatalf("stat: %v size=%d", err, info.Size)
	}
	file, err := local.Open(ctx, key)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	data, _ := io.ReadAll(file)
	_ = file.Close()
	if string(data) != string(payload) {
		t.Fatalf("payload mismatch: %q", data)
	}
	if err := local.Delete(ctx, key); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := local.Stat(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestLocalRejectsOversizedAndTraversal(t *testing.T) {
	local := newTestLocal(t)
	ctx := context.Background()
	if _, err := local.Put(ctx, "private/x.jpg", bytes.NewReader(make([]byte, 32)), 16); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
	if _, err := local.Put(ctx, "../escape.jpg", bytes.NewReader([]byte("x")), 16); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey, got %v", err)
	}
}

func TestLocalPublicURL(t *testing.T) {
	local := newTestLocal(t)
	got := local.PublicURL("public/media/abc/w480.webp", "")
	want := "http://localhost:8080/assets/media/abc/w480.webp"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestLocalSignedURL(t *testing.T) {
	local := newTestLocal(t)
	key := "private/photo/2026/11/a.jpg"
	signed, err := local.SignedURL(key, time.Minute)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	token := parsed.Query().Get("expires")
	if token == "" {
		t.Fatal("missing token")
	}
	if err := local.VerifySignedKey(key, token, time.Now()); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := local.VerifySignedKey(key, token+"x", time.Now()); err == nil {
		t.Fatal("tampered token should fail")
	}
	if err := local.VerifySignedKey(key, token, time.Now().Add(time.Hour)); err == nil {
		t.Fatal("expired token should fail")
	}
	if _, err := local.SignedURL("public/media/abc/w480.webp", time.Minute); err == nil {
		t.Fatal("public keys must not be signed")
	}
}

func TestLocalUploadToken(t *testing.T) {
	local := newTestLocal(t)
	uploadURL := local.UploadURL("media-1", 2, time.Minute)
	parsed, err := url.Parse(uploadURL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.Contains(parsed.Path, "/parts/2") {
		t.Fatalf("unexpected path %s", parsed.Path)
	}
	token := parsed.Query().Get("token")
	if err := local.VerifyUploadToken("media-1", 2, token, time.Now()); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := local.VerifyUploadToken("media-1", 1, token, time.Now()); err == nil {
		t.Fatal("token must be bound to the part number")
	}
}
