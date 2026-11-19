package media

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"math"
	"path"
	"strings"
	"time"

	"github.com/buckket/go-blurhash"
	"github.com/disintegration/imaging"
	"github.com/gen2brain/heic"
	"github.com/gen2brain/webp"
	"github.com/rwcarlsen/goexif/exif"

	"github.com/yisskqu/TwilightLog/services/api/internal/platform/storage"
)

const (
	masterLongEdge = 2560
	maxDimension   = 20000
)

// Processor turns a private original into a cleaned master and public derivatives.
type Processor interface {
	Process(ctx context.Context, asset Asset) (ProcessResult, error)
}

type LocalProcessor struct {
	storage storage.Backend
	logger  *slog.Logger
}

func NewLocalProcessor(backend storage.Backend, logger *slog.Logger) *LocalProcessor {
	if logger == nil {
		logger = slog.Default()
	}
	return &LocalProcessor{storage: backend, logger: logger}
}

func (p *LocalProcessor) Process(ctx context.Context, asset Asset) (ProcessResult, error) {
	file, err := p.storage.Open(ctx, asset.StorageKey)
	if err != nil {
		return ProcessResult{}, fmt.Errorf("open original: %w", err)
	}
	defer file.Close()

	head := make([]byte, 32)
	n, err := io.ReadFull(file, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return ProcessResult{}, fmt.Errorf("read original header: %w", err)
	}
	head = head[:n]

	mimeType, _, ok := DetectFormat(head)
	if !ok {
		return ProcessResult{}, fmt.Errorf("%w: 文件头不是支持的图片格式", ErrUnsupportedFormat)
	}
	if !allowedMime(asset.Purpose, mimeType) {
		return ProcessResult{}, fmt.Errorf("%w: %s 不接受 %s", ErrUnsupportedFormat, purposeLabel(asset.Purpose), mimeType)
	}
	if asset.MimeType != "" && asset.MimeType != mimeType {
		return ProcessResult{}, fmt.Errorf("%w: 声明类型 %s 与实际类型 %s 不一致", ErrUnsupportedFormat, asset.MimeType, mimeType)
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ProcessResult{}, fmt.Errorf("rewind original: %w", err)
	}
	exifInfo := extractExif(file, mimeType)

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ProcessResult{}, fmt.Errorf("rewind original: %w", err)
	}
	decoded, err := decodeImage(file, mimeType)
	if err != nil {
		return ProcessResult{}, fmt.Errorf("%w: 无法解码图片: %v", ErrUnsupportedFormat, err)
	}

	bounds := decoded.Bounds()
	sourceWidth := bounds.Dx()
	sourceHeight := bounds.Dy()
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return ProcessResult{}, fmt.Errorf("%w: 图片尺寸无效", ErrUnsupportedFormat)
	}
	if sourceWidth > maxDimension || sourceHeight > maxDimension {
		return ProcessResult{}, fmt.Errorf("%w: 单边不能超过 %d 像素", ErrPixelLimit, maxDimension)
	}
	if int64(sourceWidth)*int64(sourceHeight) > int64(maxPixelsFor(asset.Purpose)) {
		return ProcessResult{}, fmt.Errorf("%w: 像素总数超过限制", ErrPixelLimit)
	}

	oriented := orient(decoded, exifInfo.Orientation)
	nrgba := imaging.Clone(oriented)
	if exifInfo.Orientation == 0 {
		exifInfo.Orientation = 1
	}
	exifInfo.SourceWidth = sourceWidth
	exifInfo.SourceHeight = sourceHeight

	blurhashValue := ""
	if small := imaging.Fit(nrgba, 32, 32, imaging.Linear); small != nil {
		if encoded, err := blurhash.Encode(4, 3, small); err == nil {
			blurhashValue = encoded
		}
	}

	correctedWidth := nrgba.Bounds().Dx()
	correctedHeight := nrgba.Bounds().Dy()
	master := nrgba
	masterLong := maxInt(correctedWidth, correctedHeight)
	if masterLong > masterLongEdge {
		if correctedWidth >= correctedHeight {
			master = imaging.Resize(nrgba, masterLongEdge, 0, imaging.Lanczos)
		} else {
			master = imaging.Resize(nrgba, 0, masterLongEdge, imaging.Lanczos)
		}
		masterLong = maxInt(master.Bounds().Dx(), master.Bounds().Dy())
	}

	masterKey := strings.TrimSuffix(asset.StorageKey, path.Ext(asset.StorageKey)) + "-master.webp"
	if err := p.writeWebP(ctx, masterKey, master, 88); err != nil {
		return ProcessResult{}, fmt.Errorf("write master: %w", err)
	}

	variants := make(map[string]VariantInfo, len(DisplayVariants))
	for _, name := range DisplayVariants {
		target := variantSize(name)
		if target > masterLong {
			continue
		}
		rendered := master
		if target < masterLong {
			if master.Bounds().Dx() >= master.Bounds().Dy() {
				rendered = imaging.Resize(master, target, 0, imaging.Lanczos)
			} else {
				rendered = imaging.Resize(master, 0, target, imaging.Lanczos)
			}
		} else {
			rendered = imaging.Clone(master)
		}
		applyWatermark(rendered, maxInt(rendered.Bounds().Dx(), rendered.Bounds().Dy()))

		key := variantKey(asset.ID, name)
		size, err := p.writeWebPSize(ctx, key, rendered, variantQuality(name))
		if err != nil {
			return ProcessResult{}, fmt.Errorf("write %s variant: %w", name, err)
		}
		variants[name] = VariantInfo{
			Name:   name,
			Key:    key,
			Width:  rendered.Bounds().Dx(),
			Height: rendered.Bounds().Dy(),
			Size:   size,
		}
	}
	if len(variants) == 0 {
		rendered := imaging.Clone(master)
		applyWatermark(rendered, masterLong)
		key := variantKey(asset.ID, "w480")
		size, err := p.writeWebPSize(ctx, key, rendered, 75)
		if err != nil {
			return ProcessResult{}, fmt.Errorf("write fallback variant: %w", err)
		}
		variants["w480"] = VariantInfo{
			Name:   "w480",
			Key:    key,
			Width:  rendered.Bounds().Dx(),
			Height: rendered.Bounds().Dy(),
			Size:   size,
		}
	}

	return ProcessResult{
		MimeType:  mimeType,
		Width:     correctedWidth,
		Height:    correctedHeight,
		Blurhash:  blurhashValue,
		Exif:      exifInfo,
		MasterKey: masterKey,
		Variants:  variants,
	}, nil
}

func (p *LocalProcessor) writeWebP(ctx context.Context, key string, img image.Image, quality int) error {
	_, err := p.writeWebPSize(ctx, key, img, quality)
	return err
}

func (p *LocalProcessor) writeWebPSize(ctx context.Context, key string, img image.Image, quality int) (int64, error) {
	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, webp.Options{Quality: quality, Method: 4}); err != nil {
		return 0, err
	}
	return p.storage.Put(ctx, key, bytes.NewReader(buf.Bytes()), int64(buf.Len())+1)
}

func decodeImage(r io.Reader, mimeType string) (image.Image, error) {
	switch mimeType {
	case "image/jpeg":
		return jpeg.Decode(r)
	case "image/png":
		return png.Decode(r)
	case "image/webp":
		return webp.Decode(r)
	case "image/heic":
		return heic.Decode(r)
	default:
		return nil, fmt.Errorf("unsupported mime type %s", mimeType)
	}
}

func orient(img image.Image, orientation int) image.Image {
	switch orientation {
	case 2:
		return imaging.FlipH(img)
	case 3:
		return imaging.Rotate180(img)
	case 4:
		return imaging.FlipV(img)
	case 5:
		return imaging.Transpose(img)
	case 6:
		return imaging.Rotate270(img)
	case 7:
		return imaging.Transverse(img)
	case 8:
		return imaging.Rotate90(img)
	default:
		return img
	}
}

func variantSize(name string) int {
	switch name {
	case "w480":
		return 480
	case "w960":
		return 960
	case "w1440":
		return 1440
	case "w2048":
		return 2048
	case "w2560":
		return 2560
	default:
		return 0
	}
}

func variantQuality(name string) int {
	switch name {
	case "w480":
		return 75
	case "w960":
		return 78
	case "w1440", "w2048":
		return 80
	default:
		return 85
	}
}

func variantKey(mediaID, name string) string {
	return fmt.Sprintf("public/media/%s/%s.webp", mediaID, name)
}

func extractExif(r io.Reader, mimeType string) ExifInfo {
	switch mimeType {
	case "image/jpeg":
		return extractJPEGExif(r)
	case "image/heic":
		return extractHEICExif(r)
	default:
		return ExifInfo{}
	}
}

func extractJPEGExif(r io.Reader) ExifInfo {
	info := ExifInfo{}
	data, err := exif.Decode(r)
	if err != nil {
		return info
	}
	info.CameraMake = exifString(data, exif.Make)
	info.CameraModel = exifString(data, exif.Model)
	info.LensModel = exifString(data, exif.LensModel)
	info.TimeZone = exifString(data, exif.FieldName("OffsetTimeOriginal"))
	info.Orientation = exifInt(data, exif.Orientation)
	info.ISO = exifInt(data, exif.ISOSpeedRatings)
	if value := exifFloat(data, exif.FocalLength); value > 0 {
		info.FocalLength = fmt.Sprintf("%.0fmm", value)
	}
	if value := exifFloat(data, exif.FNumber); value > 0 {
		info.Aperture = fmt.Sprintf("f/%.1f", value)
	}
	if value := exifFloat(data, exif.ExposureTime); value > 0 {
		info.ShutterSpeed = formatShutter(value)
	}
	if value, err := exifRational(data, exif.ExposureBiasValue); err == nil && value != 0 {
		info.ExposureBias = fmt.Sprintf("%+.1fEV", value)
	}
	switch exifInt(data, exif.WhiteBalance) {
	case 0:
		info.WhiteBalance = "auto"
	case 1:
		info.WhiteBalance = "manual"
	}
	info.SourceWidth = exifInt(data, exif.PixelXDimension)
	info.SourceHeight = exifInt(data, exif.PixelYDimension)
	if taken := exifString(data, exif.DateTimeOriginal); taken != "" {
		info.TakenAt = normalizeExifTime(taken, info.TimeZone)
	}
	return info
}

func extractHEICExif(r io.Reader) ExifInfo {
	info := ExifInfo{}
	data, err := heic.DecodeExif(r)
	if err != nil || data == nil {
		return info
	}
	// GPS fields are intentionally never read into the whitelist.
	info.CameraMake = strings.TrimSpace(data.Make)
	info.CameraModel = strings.TrimSpace(data.Model)
	info.Orientation = data.Orientation
	info.ISO = data.ISOSpeed
	info.SourceWidth = data.Width
	info.SourceHeight = data.Height
	if data.FocalLength > 0 {
		info.FocalLength = fmt.Sprintf("%.0fmm", data.FocalLength)
	}
	if data.FNumber > 0 {
		info.Aperture = fmt.Sprintf("f/%.1f", data.FNumber)
	}
	if data.ExposureTime > 0 {
		info.ShutterSpeed = formatShutter(data.ExposureTime)
	}
	if data.DateTimeOriginal != "" {
		info.TakenAt = normalizeExifTime(data.DateTimeOriginal, "")
	}
	return info
}

func exifString(data *exif.Exif, name exif.FieldName) string {
	tag, err := data.Get(name)
	if err != nil {
		return ""
	}
	value, err := tag.StringVal()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func exifInt(data *exif.Exif, name exif.FieldName) int {
	tag, err := data.Get(name)
	if err != nil {
		return 0
	}
	value, err := tag.Int(0)
	if err != nil {
		return 0
	}
	return value
}

func exifFloat(data *exif.Exif, name exif.FieldName) float64 {
	value, err := exifRational(data, name)
	if err != nil {
		return 0
	}
	return value
}

func exifRational(data *exif.Exif, name exif.FieldName) (float64, error) {
	tag, err := data.Get(name)
	if err != nil {
		return 0, err
	}
	return tag.Float(0)
}

func formatShutter(seconds float64) string {
	if seconds <= 0 {
		return ""
	}
	if seconds < 1 {
		return fmt.Sprintf("1/%ds", int(math.Round(1/seconds)))
	}
	return fmt.Sprintf("%.1fs", seconds)
}

func normalizeExifTime(value, zone string) string {
	value = strings.TrimSpace(value)
	layouts := []string{"2006:01:02 15:04:05", "2006-01-02 15:04:05", time.RFC3339}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed.Format(time.RFC3339)
		}
	}
	return value
}
