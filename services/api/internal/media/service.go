package media

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/yisskqu/TwilightLog/services/api/internal/auth"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/storage"
)

const (
	uploadTTL       = 15 * time.Minute
	originalTTL     = 5 * time.Minute
	processingLimit = 10 * time.Minute
)

// ServiceConfig carries the media settings derived from the environment.
type ServiceConfig struct {
	Limits        Limits
	BatchLimit    int
	ProcessAsync  bool
	ProcessorName string
}

// Service owns the upload lifecycle and the image processing pipeline.
type Service struct {
	repo      *Repository
	backend   storage.Backend
	processor Processor
	cfg       ServiceConfig
	logger    *slog.Logger
	now       func() time.Time
}

func NewService(repo *Repository, backend storage.Backend, processor Processor, cfg ServiceConfig, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.BatchLimit <= 0 {
		cfg.BatchLimit = 20
	}
	return &Service{
		repo:      repo,
		backend:   backend,
		processor: processor,
		cfg:       cfg,
		logger:    logger,
		now:       time.Now,
	}
}

// PresignRequest is the client declaration for a new upload.
type PresignRequest struct {
	Purpose     string `json:"purpose"`
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	BatchID     string `json:"batchId"`
}

// PresignResult describes where and how the browser must upload the file.
type PresignResult struct {
	MediaID    string            `json:"mediaId"`
	Status     string            `json:"status"`
	UploadMode string            `json:"uploadMode"`
	UploadURL  string            `json:"uploadUrl,omitempty"`
	PartURLs   []string          `json:"partUrls,omitempty"`
	PartSize   int64             `json:"partSize,omitempty"`
	PartCount  int               `json:"partCount,omitempty"`
	Headers    map[string]string `json:"headers"`
	ExpiresIn  int64             `json:"expiresIn"`
}

func (s *Service) Presign(ctx context.Context, actor auth.User, req PresignRequest, idemKey string) (PresignResult, error) {
	idemKey = strings.TrimSpace(idemKey)
	if len(idemKey) > 128 {
		return PresignResult{}, fmt.Errorf("%w: Idempotency-Key 过长", ErrInvalidInput)
	}

	if idemKey != "" {
		existingID, found, err := s.repo.FindIdempotencyKey(ctx, actor.ID, idemKey)
		if err != nil {
			return PresignResult{}, err
		}
		if found {
			asset, err := s.repo.Get(ctx, existingID)
			if err != nil {
				return PresignResult{}, err
			}
			return s.presignResult(asset, 0), nil
		}
	}

	validated, err := ValidateUpload(UploadInput{
		Purpose:      req.Purpose,
		OriginalName: req.Filename,
		ContentType:  req.ContentType,
		Size:         req.Size,
		BatchID:      req.BatchID,
	}, s.cfg.Limits)
	if err != nil {
		return PresignResult{}, err
	}

	count, err := s.repo.CountBatch(ctx, actor.ID, strings.TrimSpace(req.BatchID))
	if err != nil {
		return PresignResult{}, err
	}
	if count >= s.cfg.BatchLimit {
		return PresignResult{}, fmt.Errorf("%w: 单次最多 %d 张", ErrBatchLimit, s.cfg.BatchLimit)
	}

	id := newID()
	storageKey := fmt.Sprintf("private/%s/%s/%s.%s",
		validated.Purpose, s.now().UTC().Format("2006/01"), id, validated.Extension)
	asset := Asset{
		ID:               id,
		OwnerID:          actor.ID,
		Purpose:          validated.Purpose,
		StorageKey:       storageKey,
		OriginalFilename: sanitizeFilename(req.Filename),
		MimeType:         validated.MimeType,
		FileSize:         req.Size,
		Status:           StatusUploading,
		BatchID:          strings.TrimSpace(req.BatchID),
		UploadMode:       validated.UploadMode,
		UploadID:         newID(),
		PartSize:         validated.PartSize,
		PartCount:        validated.PartCount,
		Variants:         map[string]VariantInfo{},
	}

	if err := s.repo.CreateWithIdempotency(ctx, asset, idemKey); err != nil {
		if isUniqueViolation(err) && idemKey != "" {
			existingID, found, lookupErr := s.repo.FindIdempotencyKey(ctx, actor.ID, idemKey)
			if lookupErr == nil && found {
				existing, getErr := s.repo.Get(ctx, existingID)
				if getErr == nil {
					return s.presignResult(existing, 0), nil
				}
			}
		}
		return PresignResult{}, err
	}

	return s.presignResult(asset, 0), nil
}

func (s *Service) presignResult(asset Asset, expiresIn int64) PresignResult {
	if expiresIn == 0 {
		expiresIn = int64(uploadTTL.Seconds())
	}
	result := PresignResult{
		MediaID:    asset.ID,
		Status:     asset.Status,
		UploadMode: asset.UploadMode,
		PartSize:   asset.PartSize,
		PartCount:  asset.PartCount,
		Headers:    map[string]string{"Content-Type": asset.MimeType},
		ExpiresIn:  expiresIn,
	}
	switch asset.UploadMode {
	case UploadModeMultipart:
		result.PartURLs = make([]string, 0, asset.PartCount)
		for part := 1; part <= asset.PartCount; part++ {
			result.PartURLs = append(result.PartURLs, s.backend.UploadURL(asset.ID, part, uploadTTL))
		}
	default:
		result.UploadURL = s.backend.UploadURL(asset.ID, 0, uploadTTL)
	}
	return result
}

// AcceptUpload writes one direct upload body. part is 0 for single uploads.
func (s *Service) AcceptUpload(ctx context.Context, mediaID string, part int, token string, body io.Reader, declaredLength int64) error {
	if err := s.backend.VerifyUploadToken(mediaID, part, token, s.now()); err != nil {
		return err
	}
	asset, err := s.repo.Get(ctx, mediaID)
	if err != nil {
		return err
	}
	if asset.Status != StatusUploading {
		return fmt.Errorf("%w: 当前状态不接受上传", ErrConflict)
	}

	limit := maxBytesFor(asset.Purpose, s.cfg.Limits)
	if declaredLength > limit {
		return fmt.Errorf("%w: 超过 %d MB 限制", ErrFileTooLarge, limit>>20)
	}

	if part == 0 {
		if asset.UploadMode != UploadModeSingle {
			return fmt.Errorf("%w: 该文件需要分片上传", ErrConflict)
		}
		size, err := s.backend.Put(ctx, asset.StorageKey, body, limit)
		if err != nil {
			return err
		}
		return s.repo.SetUploadedSize(ctx, mediaID, size)
	}

	if asset.UploadMode != UploadModeMultipart {
		return fmt.Errorf("%w: 该文件不支持分片上传", ErrConflict)
	}
	if part < 1 || part > asset.PartCount {
		return fmt.Errorf("%w: 分片序号越界", ErrInvalidInput)
	}
	partKey := fmt.Sprintf("tmp/%s/parts/%05d", mediaID, part)
	partLimit := asset.PartSize + 64*1024
	_, err = s.backend.Put(ctx, partKey, body, partLimit)
	return err
}

// Complete finalizes an upload and starts processing. It is safe to call twice.
func (s *Service) Complete(ctx context.Context, actor auth.User, mediaID string) (Asset, error) {
	asset, err := s.repo.Get(ctx, mediaID)
	if err != nil {
		return Asset{}, err
	}
	if err := authorize(actor, asset); err != nil {
		return Asset{}, err
	}

	switch asset.Status {
	case StatusReady, StatusProcessing:
		return asset, nil
	case StatusFailed:
		return Asset{}, fmt.Errorf("%w: 处理失败，请使用重试接口", ErrConflict)
	}

	size, err := s.ensureUploaded(ctx, asset)
	if err != nil {
		return Asset{}, err
	}
	if err := s.repo.SetUploadedSize(ctx, asset.ID, size); err != nil {
		return Asset{}, err
	}

	marked, err := s.repo.MarkProcessing(ctx, asset.ID)
	if err != nil {
		return Asset{}, err
	}
	if !marked {
		return s.repo.Get(ctx, mediaID)
	}

	if s.cfg.ProcessAsync {
		s.startProcessing(mediaID)
		asset.Status = StatusProcessing
		return asset, nil
	}

	s.process(ctx, mediaID)
	return s.repo.Get(ctx, mediaID)
}

// Retry starts processing again for a failed asset.
func (s *Service) Retry(ctx context.Context, actor auth.User, mediaID string) (Asset, error) {
	asset, err := s.repo.Get(ctx, mediaID)
	if err != nil {
		return Asset{}, err
	}
	if err := authorize(actor, asset); err != nil {
		return Asset{}, err
	}
	if asset.Status != StatusFailed {
		return Asset{}, fmt.Errorf("%w: 只有失败的处理可以重试", ErrConflict)
	}
	marked, err := s.repo.MarkProcessing(ctx, asset.ID)
	if err != nil {
		return Asset{}, err
	}
	if !marked {
		return s.repo.Get(ctx, mediaID)
	}
	if s.cfg.ProcessAsync {
		s.startProcessing(mediaID)
		asset.Status = StatusProcessing
		return asset, nil
	}
	s.process(ctx, mediaID)
	return s.repo.Get(ctx, mediaID)
}

func (s *Service) Get(ctx context.Context, actor auth.User, mediaID string) (Asset, error) {
	asset, err := s.repo.Get(ctx, mediaID)
	if err != nil {
		return Asset{}, err
	}
	if err := authorize(actor, asset); err != nil {
		return Asset{}, err
	}
	return asset, nil
}

// VariantURL returns the public URL for a generated derivative.
func (s *Service) VariantURL(key string) string {
	return s.backend.PublicURL(key, "")
}

// OriginalURL returns a short-lived signed URL for the private original.
func (s *Service) OriginalURL(ctx context.Context, actor auth.User, mediaID string) (string, error) {
	asset, err := s.Get(ctx, actor, mediaID)
	if err != nil {
		return "", err
	}
	return s.OriginalURLFor(asset)
}

// OriginalURLFor signs the original without re-checking ownership.
func (s *Service) OriginalURLFor(asset Asset) (string, error) {
	if asset.StorageKey == "" {
		return "", ErrNotFound
	}
	return s.backend.SignedURL(asset.StorageKey, originalTTL)
}

func (s *Service) ensureUploaded(ctx context.Context, asset Asset) (int64, error) {
	if asset.UploadMode == UploadModeMultipart {
		return s.assembleMultipart(ctx, asset)
	}
	info, err := s.backend.Stat(ctx, asset.StorageKey)
	if errors.Is(err, storage.ErrNotFound) {
		return 0, ErrUploadIncomplete
	}
	if err != nil {
		return 0, err
	}
	if info.Size <= 0 {
		return 0, ErrUploadIncomplete
	}
	return info.Size, nil
}

func (s *Service) assembleMultipart(ctx context.Context, asset Asset) (int64, error) {
	if asset.PartCount <= 0 {
		return 0, ErrUploadIncomplete
	}
	readers := make([]io.Reader, 0, asset.PartCount)
	closers := make([]io.Closer, 0, asset.PartCount)
	defer func() {
		for _, closer := range closers {
			_ = closer.Close()
		}
	}()

	for part := 1; part <= asset.PartCount; part++ {
		partKey := fmt.Sprintf("tmp/%s/parts/%05d", asset.ID, part)
		file, err := s.backend.Open(ctx, partKey)
		if errors.Is(err, storage.ErrNotFound) {
			return 0, fmt.Errorf("%w: 缺少第 %d 片", ErrUploadIncomplete, part)
		}
		if err != nil {
			return 0, err
		}
		readers = append(readers, file)
		closers = append(closers, file)
	}

	limit := maxBytesFor(asset.Purpose, s.cfg.Limits)
	size, err := s.backend.Put(ctx, asset.StorageKey, io.MultiReader(readers...), limit)
	// Close part files before deleting the temp directory (Windows locks open files).
	for _, closer := range closers {
		_ = closer.Close()
	}
	closers = nil
	if err != nil {
		return 0, err
	}
	if err := s.backend.DeletePrefix(ctx, "tmp/"+asset.ID); err != nil {
		s.logger.Warn("cleanup multipart parts", "media_id", asset.ID, "error", err)
	}
	return size, nil
}

func (s *Service) startProcessing(mediaID string) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("media processing panic", "media_id", mediaID, "error", recovered)
				_ = s.repo.MarkFailed(context.Background(), mediaID, "processing_panic", "图片处理异常，请重试")
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), processingLimit)
		defer cancel()
		s.process(ctx, mediaID)
	}()
}

func (s *Service) process(ctx context.Context, mediaID string) {
	asset, err := s.repo.Get(ctx, mediaID)
	if err != nil {
		s.logger.Error("load media for processing", "media_id", mediaID, "error", err)
		return
	}
	result, err := s.processor.Process(ctx, asset)
	if err != nil {
		code, message := classifyProcessingError(err)
		s.logger.Error("process media", "media_id", mediaID, "error", err)
		if markErr := s.repo.MarkFailed(ctx, mediaID, code, message); markErr != nil {
			s.logger.Error("mark media failed", "media_id", mediaID, "error", markErr)
		}
		_ = s.backend.DeletePrefix(ctx, "tmp/"+mediaID)
		return
	}
	if err := s.repo.MarkReady(ctx, mediaID, result.MimeType, result); err != nil {
		s.logger.Error("mark media ready", "media_id", mediaID, "error", err)
		_ = s.repo.MarkFailed(ctx, mediaID, "persist_failed", "处理结果保存失败，请重试")
		return
	}
	_ = s.backend.DeletePrefix(ctx, "tmp/"+mediaID)
	s.logger.Info("media processed", "media_id", mediaID, "width", result.Width, "height", result.Height)
}

// CleanupStaleUploads removes abandoned uploads and old failed artifacts.
func (s *Service) CleanupStaleUploads(ctx context.Context) (int, error) {
	removed := 0
	stale, err := s.repo.ListStaleUploading(ctx, s.now().Add(-24*time.Hour), 200)
	if err != nil {
		return 0, err
	}
	for _, asset := range stale {
		if err := s.removeAsset(ctx, asset); err != nil {
			s.logger.Warn("cleanup stale upload", "media_id", asset.ID, "error", err)
			continue
		}
		removed++
	}

	failed, err := s.repo.ListFailed(ctx, s.now().Add(-7*24*time.Hour), 200)
	if err != nil {
		return removed, err
	}
	for _, asset := range failed {
		if err := s.removeAsset(ctx, asset); err != nil {
			s.logger.Warn("cleanup failed media", "media_id", asset.ID, "error", err)
			continue
		}
		removed++
	}

	tempRemoved, err := s.backend.CleanupTemp(ctx, s.now().Add(-24*time.Hour))
	if err != nil {
		return removed, err
	}
	return removed + tempRemoved, nil
}

func (s *Service) removeAsset(ctx context.Context, asset Asset) error {
	_ = s.backend.Delete(ctx, asset.StorageKey)
	if asset.MasterKey != "" {
		_ = s.backend.Delete(ctx, asset.MasterKey)
	}
	_ = s.backend.DeletePrefix(ctx, "public/media/"+asset.ID)
	_ = s.backend.DeletePrefix(ctx, "tmp/"+asset.ID)
	return s.repo.Delete(ctx, asset.ID)
}

// Purge removes one asset's original, master, derivatives and database row.
// It is used when trashed content is permanently deleted.
func (s *Service) Purge(ctx context.Context, mediaID string) error {
	asset, err := s.repo.Get(ctx, mediaID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.removeAsset(ctx, asset)
}

// SignedFilePath verifies a signed GET capability and returns a local file path.
func (s *Service) SignedFilePath(key, token string, now time.Time) (string, error) {
	local, ok := s.backend.(interface {
		VerifySignedKey(key, token string, now time.Time) error
		FilePath(key string) (string, error)
	})
	if !ok {
		return "", storage.ErrUnsupported
	}
	if err := local.VerifySignedKey(key, token, now); err != nil {
		return "", err
	}
	return local.FilePath(key)
}

// AcceptSignedPut handles the generic PresignPut capability URL.
func (s *Service) AcceptSignedPut(ctx context.Context, key, token string, body io.Reader, declaredLength int64) error {
	local, ok := s.backend.(interface {
		VerifyPutKey(key, token string, now time.Time) error
	})
	if !ok {
		return storage.ErrUnsupported
	}
	if err := local.VerifyPutKey(key, token, s.now()); err != nil {
		return err
	}
	limit := s.cfg.Limits.MaxPhotoBytes
	if declaredLength > limit {
		return fmt.Errorf("%w: 超过 %d MB 限制", ErrFileTooLarge, limit>>20)
	}
	_, err := s.backend.Put(ctx, key, body, limit)
	return err
}

func authorize(actor auth.User, asset Asset) error {
	if actor.Role == "super_admin" || actor.Role == "admin" {
		return nil
	}
	if asset.OwnerID == actor.ID {
		return nil
	}
	return ErrNotFound
}

func classifyProcessingError(err error) (string, string) {
	switch {
	case errors.Is(err, ErrUnsupportedFormat):
		return "unsupported_format", "文件不是支持的图片格式或已损坏"
	case errors.Is(err, ErrPixelLimit):
		return "pixel_limit_exceeded", "图片尺寸超过限制"
	case errors.Is(err, ErrFileTooLarge):
		return "file_too_large", "图片超过大小限制"
	case errors.Is(err, storage.ErrNotFound):
		return "original_missing", "原图不存在，请重新上传"
	default:
		return "processing_failed", "图片处理失败，请重试"
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func newID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}
