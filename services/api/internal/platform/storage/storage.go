package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	// ErrNotFound is returned when an object does not exist.
	ErrNotFound = errors.New("storage object not found")
	// ErrTooLarge is returned when a stream exceeds the allowed size.
	ErrTooLarge = errors.New("storage object exceeds size limit")
	// ErrInvalidKey is returned for empty, absolute or traversing keys.
	ErrInvalidKey = errors.New("invalid storage key")
	// ErrInvalidSignature is returned for expired or tampered capability URLs.
	ErrInvalidSignature = errors.New("invalid or expired signature")
	// ErrUnsupported is returned by adapters that cannot serve an operation.
	ErrUnsupported = errors.New("storage operation not supported")
)

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	Size    int64
	ModTime time.Time
}

// AssetResolver is the single URL entry point for stored assets.
// Callers must never hard-code storage domains; every image URL goes through it.
type AssetResolver interface {
	PublicURL(key, variant string) string
	SignedURL(key string, ttl time.Duration) (string, error)
	PresignPut(key, contentType string) (string, map[string]string, error)
	Purge(key string) error
}

// Backend extends AssetResolver with the object operations the media pipeline needs.
// Local development uses the filesystem adapter; production will provide OSS.
type Backend interface {
	AssetResolver

	// Put writes r to key atomically and rejects streams larger than limit.
	Put(ctx context.Context, key string, r io.Reader, limit int64) (int64, error)
	Open(ctx context.Context, key string) (io.ReadSeekCloser, error)
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	Delete(ctx context.Context, key string) error
	DeletePrefix(ctx context.Context, prefix string) error
	CleanupTemp(ctx context.Context, before time.Time) (int, error)

	// UploadURL returns a capability URL the browser can PUT to directly.
	// part is 0 for a single upload and 1..n for multipart parts.
	UploadURL(mediaID string, part int, ttl time.Duration) string
	VerifyUploadToken(mediaID string, part int, token string, now time.Time) error
}
