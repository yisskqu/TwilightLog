package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	PrivatePrefix = "private/"
	PublicPrefix  = "public/"
	TempPrefix    = "tmp/"
)

// Local is a filesystem-backed storage adapter used for local development.
// Originals live under private/, display derivatives under public/.
type Local struct {
	root      string
	assetBase string
	apiBase   string
	signer    *Signer
}

func NewLocal(root, assetBase, apiBase, signingSecret string) (*Local, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("local storage root is required")
	}
	signer, err := NewSigner(signingSecret)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve storage root: %w", err)
	}
	for _, dir := range []string{"private", "public", "tmp"} {
		if err := os.MkdirAll(filepath.Join(abs, dir), 0o755); err != nil {
			return nil, fmt.Errorf("create storage directory %s: %w", dir, err)
		}
	}
	return &Local{
		root:      abs,
		assetBase: strings.TrimRight(assetBase, "/"),
		apiBase:   strings.TrimRight(apiBase, "/"),
		signer:    signer,
	}, nil
}

func (l *Local) Root() string { return l.root }

// PublicURL maps an object key to the public derivative URL.
// Public objects are served from <ASSET_BASE_URL>/<key without the public prefix>.
func (l *Local) PublicURL(key, variant string) string {
	key = strings.Trim(key, "/")
	if variant != "" && !strings.HasSuffix(key, ".webp") {
		key = key + "/" + variant + ".webp"
	}
	key = strings.TrimPrefix(key, PublicPrefix)
	return l.assetBase + "/" + key
}

// SignedURL builds a short-lived capability URL for a private object.
func (l *Local) SignedURL(key string, ttl time.Duration) (string, error) {
	clean, err := normalizeKey(key)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(clean, PrivatePrefix) {
		return "", fmt.Errorf("%w: only private objects can be signed", ErrInvalidKey)
	}
	expires := time.Now().Add(ttl)
	token := l.signer.Sign("get:"+clean, expires)
	query := url.Values{}
	query.Set("key", clean)
	query.Set("expires", token)
	return l.apiBase + "/api/v1/assets/signed?" + query.Encode(), nil
}

// PresignPut builds a capability URL for a direct PUT under private/.
func (l *Local) PresignPut(key, contentType string) (string, map[string]string, error) {
	clean, err := normalizeKey(key)
	if err != nil {
		return "", nil, err
	}
	if !strings.HasPrefix(clean, PrivatePrefix) {
		return "", nil, fmt.Errorf("%w: only private objects can be uploaded", ErrInvalidKey)
	}
	expires := time.Now().Add(15 * time.Minute)
	token := l.signer.Sign("put:"+clean, expires)
	query := url.Values{}
	query.Set("key", clean)
	query.Set("expires", token)
	headers := map[string]string{"Content-Type": contentType}
	return l.apiBase + "/api/v1/assets/upload?" + query.Encode(), headers, nil
}

// Purge is a no-op until a CDN is attached.
func (l *Local) Purge(key string) error { return nil }

func (l *Local) Put(ctx context.Context, key string, r io.Reader, limit int64) (int64, error) {
	clean, err := normalizeKey(key)
	if err != nil {
		return 0, err
	}
	path := l.path(clean)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, fmt.Errorf("create object directory: %w", err)
	}

	tmpPath := path + ".tmp-" + randomHex(6)
	tmp, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, fmt.Errorf("create temporary object: %w", err)
	}
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	reader := io.Reader(r)
	if limit > 0 {
		reader = io.LimitReader(r, limit+1)
	}
	written, err := io.Copy(tmp, reader)
	if err != nil {
		cleanup()
		return 0, fmt.Errorf("write object: %w", err)
	}
	if limit > 0 && written > limit {
		cleanup()
		return 0, ErrTooLarge
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return 0, fmt.Errorf("flush object: %w", err)
	}
	// Close before rename: Windows refuses to rename an open file.
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("close object: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("commit object: %w", err)
	}
	return written, nil
}

func (l *Local) Open(ctx context.Context, key string) (io.ReadSeekCloser, error) {
	clean, err := normalizeKey(key)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(l.path(clean))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open object: %w", err)
	}
	return file, nil
}

func (l *Local) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	clean, err := normalizeKey(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	info, err := os.Stat(l.path(clean))
	if errors.Is(err, fs.ErrNotExist) {
		return ObjectInfo{}, ErrNotFound
	}
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("stat object: %w", err)
	}
	return ObjectInfo{Size: info.Size(), ModTime: info.ModTime()}, nil
}

func (l *Local) Delete(ctx context.Context, key string) error {
	clean, err := normalizeKey(key)
	if err != nil {
		return err
	}
	err = os.Remove(l.path(clean))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (l *Local) DeletePrefix(ctx context.Context, prefix string) error {
	clean, err := normalizeKey(prefix)
	if err != nil {
		return err
	}
	path := l.path(clean)
	if err := os.RemoveAll(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete prefix: %w", err)
	}
	return nil
}

// CleanupTemp removes temporary upload directories older than the cutoff.
func (l *Local) CleanupTemp(ctx context.Context, before time.Time) (int, error) {
	dir := filepath.Join(l.root, "tmp")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("read temp directory: %w", err)
	}
	removed := 0
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(before) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return removed, fmt.Errorf("remove temp entry: %w", err)
		}
		removed++
	}
	return removed, nil
}

// PublicHandler serves only the public/ tree with immutable caching.
func (l *Local) PublicHandler() http.Handler {
	fileServer := http.FileServer(http.Dir(filepath.Join(l.root, "public")))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/private/") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.StripPrefix("/assets/", fileServer).ServeHTTP(w, r)
	})
}

// UploadURL returns a capability URL that the browser PUTs to directly.
func (l *Local) UploadURL(mediaID string, part int, ttl time.Duration) string {
	expires := time.Now().Add(ttl)
	scope := uploadScope(mediaID, part)
	token := l.signer.Sign(scope, expires)
	query := url.Values{}
	query.Set("token", token)
	path := l.apiBase + "/api/v1/media/upload/" + url.PathEscape(mediaID)
	if part > 0 {
		path += "/parts/" + strconvItoa(part)
	}
	return path + "?" + query.Encode()
}

func (l *Local) VerifyUploadToken(mediaID string, part int, token string, now time.Time) error {
	return l.signer.Verify(uploadScope(mediaID, part), token, now)
}

func uploadScope(mediaID string, part int) string {
	if part <= 0 {
		return "upload:" + mediaID
	}
	return "upload:" + mediaID + ":" + strconvItoa(part)
}

// VerifySignedKey checks a signed GET capability and returns the object key.
func (l *Local) VerifySignedKey(key, token string, now time.Time) error {
	clean, err := normalizeKey(key)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(clean, PrivatePrefix) {
		return ErrInvalidKey
	}
	return l.signer.Verify("get:"+clean, token, now)
}

// VerifyPutKey checks a signed PUT capability and returns the object key.
func (l *Local) VerifyPutKey(key, token string, now time.Time) error {
	clean, err := normalizeKey(key)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(clean, PrivatePrefix) {
		return ErrInvalidKey
	}
	return l.signer.Verify("put:"+clean, token, now)
}

// FilePath exposes the absolute path for serving with http.ServeContent.
func (l *Local) FilePath(key string) (string, error) {
	clean, err := normalizeKey(key)
	if err != nil {
		return "", err
	}
	return l.path(clean), nil
}

func (l *Local) path(clean string) string {
	return filepath.Join(l.root, filepath.FromSlash(clean))
}

func normalizeKey(key string) (string, error) {
	key = strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
	key = strings.TrimPrefix(key, "/")
	if key == "" {
		return "", ErrInvalidKey
	}
	for _, element := range strings.Split(key, "/") {
		if element == "" || element == "." || element == ".." {
			return "", ErrInvalidKey
		}
	}
	return key, nil
}

func strconvItoa(value int) string {
	return strconv.Itoa(value)
}
