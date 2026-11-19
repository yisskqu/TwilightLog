package media

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/yisskqu/TwilightLog/services/api/internal/platform/storage"
)

func newTestProcessor(t *testing.T) (*LocalProcessor, storage.Backend) {
	t.Helper()
	backend, err := storage.NewLocal(t.TempDir(), "http://localhost:8080/assets", "http://localhost:8080", "test-signing-secret-0123456789")
	if err != nil {
		t.Fatalf("create local storage: %v", err)
	}
	return NewLocalProcessor(backend, nil), backend
}

func putFixture(t *testing.T, backend storage.Backend, key, fixture string) {
	t.Helper()
	file, err := os.Open(testdataPath(fixture))
	if err != nil {
		t.Fatalf("open fixture %s: %v", fixture, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}
	if _, err := backend.Put(context.Background(), key, file, info.Size()+1); err != nil {
		t.Fatalf("store fixture: %v", err)
	}
}

func readObject(t *testing.T, backend storage.Backend, key string) []byte {
	t.Helper()
	file, err := backend.Open(context.Background(), key)
	if err != nil {
		t.Fatalf("open object %s: %v", key, err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read object %s: %v", key, err)
	}
	return data
}

func TestProcessJPEGProducesVariants(t *testing.T) {
	processor, backend := newTestProcessor(t)
	key := "private/photo/2026/11/test.jpg"
	putFixture(t, backend, key, "normal.jpg")

	result, err := processor.Process(context.Background(), Asset{
		ID:         "test-media",
		Purpose:    PurposePhoto,
		StorageKey: key,
		MimeType:   "image/jpeg",
	})
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	if result.Width != 1600 || result.Height != 1067 {
		t.Fatalf("expected 1600x1067, got %dx%d", result.Width, result.Height)
	}
	if result.Blurhash == "" {
		t.Fatal("blurhash is empty")
	}
	for _, name := range []string{"w480", "w960", "w1440"} {
		if _, ok := result.Variants[name]; !ok {
			t.Fatalf("missing variant %s", name)
		}
	}
	if _, ok := result.Variants["w2048"]; ok {
		t.Fatal("w2048 must not upscale a 1600px image")
	}

	data := readObject(t, backend, result.Variants["w960"].Key)
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		t.Fatal("derivative is not a WebP file")
	}
	for _, marker := range []string{"Exif", "GPS", "XMP", "ICC_PROFILE", "Photoshop"} {
		if bytes.Contains(data, []byte(marker)) {
			t.Fatalf("derivative still contains metadata marker %q", marker)
		}
	}
	if _, err := backend.Stat(context.Background(), result.MasterKey); err != nil {
		t.Fatalf("master missing: %v", err)
	}
}

func TestProcessAppliesOrientation(t *testing.T) {
	processor, backend := newTestProcessor(t)
	key := "private/photo/2026/11/oriented.jpg"
	putFixture(t, backend, key, "orientation-6.jpg")

	result, err := processor.Process(context.Background(), Asset{
		ID:         "oriented",
		Purpose:    PurposePhoto,
		StorageKey: key,
		MimeType:   "image/jpeg",
	})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if result.Width != 1067 || result.Height != 1600 {
		t.Fatalf("orientation not applied, got %dx%d", result.Width, result.Height)
	}
	if result.Exif.Orientation != 6 {
		t.Fatalf("expected exif orientation 6, got %d", result.Exif.Orientation)
	}
}

func TestProcessStripsGPS(t *testing.T) {
	processor, backend := newTestProcessor(t)
	key := "private/photo/2026/11/gps.jpg"
	putFixture(t, backend, key, "gps.jpg")

	result, err := processor.Process(context.Background(), Asset{
		ID:         "gps",
		Purpose:    PurposePhoto,
		StorageKey: key,
		MimeType:   "image/jpeg",
	})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	data := readObject(t, backend, result.Variants["w960"].Key)
	for _, marker := range []string{"GPS", "Exif", "XMP"} {
		if bytes.Contains(data, []byte(marker)) {
			t.Fatalf("public derivative still contains %q", marker)
		}
	}
}

func TestProcessRejectsFakeExtension(t *testing.T) {
	processor, backend := newTestProcessor(t)
	key := "private/article/2026/11/fake.jpg"
	putFixture(t, backend, key, "fake-extension.jpg")

	_, err := processor.Process(context.Background(), Asset{
		ID:         "fake",
		Purpose:    PurposeArticle,
		StorageKey: key,
		MimeType:   "image/jpeg",
	})
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("expected ErrUnsupportedFormat, got %v", err)
	}
}

func TestProcessHEIC(t *testing.T) {
	processor, backend := newTestProcessor(t)
	key := "private/photo/2026/11/sample.heic"
	putFixture(t, backend, key, "sample.heic")

	result, err := processor.Process(context.Background(), Asset{
		ID:         "heic",
		Purpose:    PurposePhoto,
		StorageKey: key,
		MimeType:   "image/heic",
	})
	if err != nil {
		t.Fatalf("process HEIC: %v", err)
	}
	if result.Width <= 0 || result.Height <= 0 {
		t.Fatalf("invalid HEIC dimensions %dx%d", result.Width, result.Height)
	}
	if len(result.Variants) == 0 {
		t.Fatal("HEIC produced no variants")
	}
	data := readObject(t, backend, result.Variants["w480"].Key)
	if len(data) < 12 || string(data[:4]) != "RIFF" {
		t.Fatal("HEIC derivative is not a WebP file")
	}
}
