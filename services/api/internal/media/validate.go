package media

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	multipartThreshold = 5 << 20
	multipartPartSize  = 5 << 20
	maxUploadParts     = 32
)

// UploadInput is the client declaration accepted by the presign endpoint.
type UploadInput struct {
	Purpose      string
	OriginalName string
	ContentType  string
	Size         int64
	BatchID      string
}

// ValidatedUpload is the normalized result of a presign declaration.
type ValidatedUpload struct {
	Purpose    string
	MimeType   string
	Extension  string
	UploadMode string
	PartSize   int64
	PartCount  int
}

// Limits carries the configured upload ceilings.
type Limits struct {
	MaxArticleBytes int64
	MaxPhotoBytes   int64
}

func ValidateUpload(in UploadInput, cfg Limits) (ValidatedUpload, error) {
	purpose := strings.TrimSpace(strings.ToLower(in.Purpose))
	if purpose != PurposeArticle && purpose != PurposePhoto {
		return ValidatedUpload{}, fmt.Errorf("%w: purpose 只能是 article 或 photo", ErrInvalidInput)
	}
	batchID := strings.TrimSpace(in.BatchID)
	if batchID == "" || len(batchID) > 100 {
		return ValidatedUpload{}, fmt.Errorf("%w: batchId 必填且不超过 100 个字符", ErrInvalidInput)
	}
	name := sanitizeFilename(in.OriginalName)
	if name == "" {
		return ValidatedUpload{}, fmt.Errorf("%w: 文件名不能为空", ErrInvalidInput)
	}

	contentType := normalizeContentType(in.ContentType)
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if ext == "" {
		return ValidatedUpload{}, fmt.Errorf("%w: 文件缺少扩展名", ErrInvalidInput)
	}
	declared, ok := extensionMime[ext]
	if !ok {
		return ValidatedUpload{}, fmt.Errorf("%w: 不支持的扩展名 .%s", ErrUnsupportedFormat, ext)
	}
	if contentType == "" {
		contentType = declared
	}
	if contentType != declared {
		return ValidatedUpload{}, fmt.Errorf("%w: 扩展名与 Content-Type 不一致", ErrInvalidInput)
	}
	if !allowedMime(purpose, contentType) {
		return ValidatedUpload{}, fmt.Errorf("%w: %s 不接受 %s", ErrUnsupportedFormat, purposeLabel(purpose), contentType)
	}

	if in.Size <= 0 {
		return ValidatedUpload{}, fmt.Errorf("%w: 文件大小无效", ErrInvalidInput)
	}
	limit := maxBytesFor(purpose, cfg)
	if in.Size > limit {
		return ValidatedUpload{}, fmt.Errorf("%w: 超过 %d MB 限制", ErrFileTooLarge, limit>>20)
	}

	mode := UploadModeSingle
	partCount := 0
	if in.Size > multipartThreshold && purpose == PurposePhoto {
		mode = UploadModeMultipart
		partCount = int(math.Ceil(float64(in.Size) / float64(multipartPartSize)))
		if partCount > maxUploadParts {
			return ValidatedUpload{}, fmt.Errorf("%w: 分片数量过多", ErrFileTooLarge)
		}
	}

	return ValidatedUpload{
		Purpose:    purpose,
		MimeType:   contentType,
		Extension:  canonicalExtension(contentType),
		UploadMode: mode,
		PartSize:   multipartPartSize,
		PartCount:  partCount,
	}, nil
}

// DetectFormat inspects magic bytes only; extensions are never trusted.
func DetectFormat(head []byte) (mimeType, extension string, ok bool) {
	switch {
	case len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF:
		return "image/jpeg", "jpg", true
	case len(head) >= 8 && string(head[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png", "png", true
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return "image/webp", "webp", true
	case len(head) >= 12 && string(head[4:8]) == "ftyp" && heicBrands[string(head[8:12])]:
		return "image/heic", "heic", true
	}
	return "", "", false
}

func normalizeContentType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if idx := strings.Index(value, ";"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	if value == "image/jpg" {
		value = "image/jpeg"
	}
	if value == "image/heif" {
		value = "image/heic"
	}
	return value
}

func allowedMime(purpose, mimeType string) bool {
	switch purpose {
	case PurposeArticle:
		return mimeType == "image/jpeg" || mimeType == "image/png" || mimeType == "image/webp"
	case PurposePhoto:
		return mimeType == "image/jpeg" || mimeType == "image/png" || mimeType == "image/heic"
	}
	return false
}

func maxBytesFor(purpose string, cfg Limits) int64 {
	if purpose == PurposeArticle {
		return cfg.MaxArticleBytes
	}
	return cfg.MaxPhotoBytes
}

func maxPixelsFor(purpose string) int {
	if purpose == PurposeArticle {
		return 30_000_000
	}
	return 60_000_000
}

func canonicalExtension(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/webp":
		return "webp"
	case "image/heic":
		return "heic"
	}
	return ""
}

func purposeLabel(purpose string) string {
	if purpose == PurposeArticle {
		return "文章图片"
	}
	return "摄影照片"
}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	if len([]rune(name)) > 255 {
		name = string([]rune(name)[:255])
	}
	if name == "." || name == ".." || name == "/" {
		return ""
	}
	return name
}

var extensionMime = map[string]string{
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"webp": "image/webp",
	"heic": "image/heic",
	"heif": "image/heic",
}

var heicBrands = map[string]bool{
	"heic": true,
	"heix": true,
	"hevc": true,
	"hevx": true,
	"heim": true,
	"heis": true,
	"hevm": true,
	"hevs": true,
	"mif1": true,
	"msf1": true,
	"heif": true,
}
