package media

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testLimits() Limits {
	return Limits{MaxArticleBytes: 5 << 20, MaxPhotoBytes: 50 << 20}
}

func TestValidateUpload(t *testing.T) {
	tests := []struct {
		name    string
		input   UploadInput
		wantErr error
		mode    string
		parts   int
	}{
		{
			name:  "article jpg under limit",
			input: UploadInput{Purpose: "article", OriginalName: "a.jpg", ContentType: "image/jpeg", Size: 4 << 20, BatchID: "b1"},
			mode:  UploadModeSingle,
		},
		{
			name:    "article jpg over limit",
			input:   UploadInput{Purpose: "article", OriginalName: "a.jpg", ContentType: "image/jpeg", Size: 6 << 20, BatchID: "b1"},
			wantErr: ErrFileTooLarge,
		},
		{
			name:    "article rejects heic",
			input:   UploadInput{Purpose: "article", OriginalName: "a.heic", ContentType: "image/heic", Size: 3 << 20, BatchID: "b1"},
			wantErr: ErrUnsupportedFormat,
		},
		{
			name:    "photo rejects webp",
			input:   UploadInput{Purpose: "photo", OriginalName: "a.webp", ContentType: "image/webp", Size: 3 << 20, BatchID: "b1"},
			wantErr: ErrUnsupportedFormat,
		},
		{
			name:    "extension mismatch",
			input:   UploadInput{Purpose: "photo", OriginalName: "a.jpg", ContentType: "image/png", Size: 3 << 20, BatchID: "b1"},
			wantErr: ErrInvalidInput,
		},
		{
			name:    "batch id required",
			input:   UploadInput{Purpose: "photo", OriginalName: "a.jpg", ContentType: "image/jpeg", Size: 3 << 20},
			wantErr: ErrInvalidInput,
		},
		{
			name:  "large photo becomes multipart",
			input: UploadInput{Purpose: "photo", OriginalName: "big.heic", ContentType: "image/heic", Size: 42 << 20, BatchID: "b1"},
			mode:  UploadModeMultipart,
			parts: 9,
		},
		{
			name:  "photo at exact limit",
			input: UploadInput{Purpose: "photo", OriginalName: "big.heic", ContentType: "image/heic", Size: 50 << 20, BatchID: "b1"},
			mode:  UploadModeMultipart,
			parts: 10,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateUpload(tc.input, testLimits())
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.UploadMode != tc.mode {
				t.Fatalf("expected mode %s, got %s", tc.mode, got.UploadMode)
			}
			if tc.parts > 0 && got.PartCount != tc.parts {
				t.Fatalf("expected %d parts, got %d", tc.parts, got.PartCount)
			}
		})
	}
}

func TestDetectFormatFromFixtures(t *testing.T) {
	tests := []struct {
		file string
		mime string
	}{
		{"normal.jpg", "image/jpeg"},
		{"normal.png", "image/png"},
		{"fake-extension.jpg", "image/png"},
		{"sample.heic", "image/heic"},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			file, err := os.Open(testdataPath(tc.file))
			if err != nil {
				t.Fatalf("open fixture: %v", err)
			}
			defer file.Close()
			head := make([]byte, 32)
			n, _ := file.Read(head)
			mimeType, _, ok := DetectFormat(head[:n])
			if !ok {
				t.Fatal("format not detected")
			}
			if mimeType != tc.mime {
				t.Fatalf("expected %s, got %s", tc.mime, mimeType)
			}
		})
	}
}

func testdataPath(name string) string {
	return filepath.Join("..", "..", "..", "..", "testdata", "images", name)
}
