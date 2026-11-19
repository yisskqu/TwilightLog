package media

import (
	"errors"
	"strings"
	"time"
)

const (
	PurposeArticle = "article"
	PurposePhoto   = "photo"

	StatusUploading  = "uploading"
	StatusProcessing = "processing"
	StatusReady      = "ready"
	StatusFailed     = "failed"

	UploadModeSingle    = "single"
	UploadModeMultipart = "multipart"
)

// DisplayVariants are the only sizes exposed to the public site.
var DisplayVariants = []string{"w480", "w960", "w1440", "w2048", "w2560"}

// VariantInfo records one generated display derivative.
type VariantInfo struct {
	Name   string `json:"name"`
	Key    string `json:"key"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Size   int64  `json:"size"`
}

// ExifInfo is the whitelisted metadata kept after cleaning.
// GPS, XMP, IPTC and MakerNotes are never copied here.
type ExifInfo struct {
	CameraMake   string `json:"cameraMake,omitempty"`
	CameraModel  string `json:"cameraModel,omitempty"`
	LensModel    string `json:"lensModel,omitempty"`
	FocalLength  string `json:"focalLength,omitempty"`
	Aperture     string `json:"aperture,omitempty"`
	ShutterSpeed string `json:"shutterSpeed,omitempty"`
	ISO          int    `json:"iso,omitempty"`
	TakenAt      string `json:"takenAt,omitempty"`
	TimeZone     string `json:"timeZone,omitempty"`
	ExposureBias string `json:"exposureBias,omitempty"`
	WhiteBalance string `json:"whiteBalance,omitempty"`
	Orientation  int    `json:"orientation,omitempty"`
	SourceWidth  int    `json:"sourceWidth,omitempty"`
	SourceHeight int    `json:"sourceHeight,omitempty"`
}

// Asset is one row of media_assets.
type Asset struct {
	ID               string
	OwnerID          string
	Purpose          string
	StorageKey       string
	OriginalFilename string
	MimeType         string
	FileSize         int64
	Width            int
	Height           int
	Blurhash         string
	Status           string
	BatchID          string
	UploadMode       string
	UploadID         string
	PartSize         int64
	PartCount        int
	MasterKey        string
	Exif             ExifInfo
	Variants         map[string]VariantInfo
	ErrorCode        string
	ErrorMessage     string
	ProcessedAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

var (
	ErrNotFound          = errors.New("media not found")
	ErrForbidden         = errors.New("media access forbidden")
	ErrInvalidInput      = errors.New("invalid media input")
	ErrBatchLimit        = errors.New("media batch limit exceeded")
	ErrUploadIncomplete  = errors.New("media upload is incomplete")
	ErrConflict          = errors.New("media state conflict")
	ErrUnsupportedFormat = errors.New("unsupported image format")
	ErrFileTooLarge      = errors.New("image file too large")
	ErrPixelLimit        = errors.New("image dimensions exceed limit")
	ErrProcessorDisabled = errors.New("media processor is disabled")
)

// Message strips the sentinel prefix so clients get a readable reason.
func Message(err error) string {
	for _, sentinel := range []error{
		ErrInvalidInput, ErrBatchLimit, ErrUnsupportedFormat, ErrPixelLimit,
		ErrUploadIncomplete, ErrConflict, ErrFileTooLarge,
	} {
		prefix := sentinel.Error() + ": "
		if strings.HasPrefix(err.Error(), prefix) {
			return strings.TrimPrefix(err.Error(), prefix)
		}
	}
	return err.Error()
}

// ProcessResult is produced by the image pipeline.
type ProcessResult struct {
	MimeType  string
	Width     int
	Height    int
	Blurhash  string
	Exif      ExifInfo
	MasterKey string
	Variants  map[string]VariantInfo
}
