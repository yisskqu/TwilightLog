package server

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/yisskqu/TwilightLog/services/api/internal/auth"
	"github.com/yisskqu/TwilightLog/services/api/internal/media"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/httpx"
	"github.com/yisskqu/TwilightLog/services/api/internal/platform/storage"
)

type mediaVariantResponse struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Size   int64  `json:"size"`
}

type mediaErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type mediaAssetResponse struct {
	ID               string                 `json:"id"`
	Purpose          string                 `json:"purpose"`
	Status           string                 `json:"status"`
	OriginalFilename string                 `json:"originalFilename"`
	MimeType         string                 `json:"mimeType"`
	FileSize         int64                  `json:"fileSize"`
	Width            int                    `json:"width"`
	Height           int                    `json:"height"`
	Blurhash         string                 `json:"blurhash"`
	Exif             media.ExifInfo         `json:"exif"`
	Variants         []mediaVariantResponse `json:"variants"`
	OriginalURL      string                 `json:"originalUrl,omitempty"`
	Error            *mediaErrorResponse    `json:"error,omitempty"`
	ProcessedAt      *time.Time             `json:"processedAt,omitempty"`
	CreatedAt        time.Time              `json:"createdAt"`
	UpdatedAt        time.Time              `json:"updatedAt"`
}

func presignHandler(svc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload media.PresignRequest
		if !decodeJSON(w, r, &payload) {
			return
		}
		actor, _ := auth.UserFromContext(r.Context())
		result, err := svc.Presign(r.Context(), actor, payload, r.Header.Get("Idempotency-Key"))
		if err != nil {
			writeMediaError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, result)
	}
}

func completeMediaHandler(svc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			MediaID string `json:"mediaId"`
		}
		if !decodeJSON(w, r, &payload) {
			return
		}
		if strings.TrimSpace(payload.MediaID) == "" {
			httpx.WriteError(w, r, http.StatusBadRequest, "validation_error", "mediaId 必填", nil)
			return
		}
		actor, _ := auth.UserFromContext(r.Context())
		asset, err := svc.Complete(r.Context(), actor, payload.MediaID)
		if err != nil {
			writeMediaError(w, r, err)
			return
		}
		status := http.StatusOK
		if asset.Status == media.StatusProcessing {
			status = http.StatusAccepted
		}
		httpx.WriteJSON(w, status, newMediaAssetResponse(svc, asset))
	}
}

func getMediaHandler(svc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := auth.UserFromContext(r.Context())
		asset, err := svc.Get(r.Context(), actor, chi.URLParam(r, "id"))
		if err != nil {
			writeMediaError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, newMediaAssetResponse(svc, asset))
	}
}

func retryMediaHandler(svc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := auth.UserFromContext(r.Context())
		asset, err := svc.Retry(r.Context(), actor, chi.URLParam(r, "id"))
		if err != nil {
			writeMediaError(w, r, err)
			return
		}
		status := http.StatusOK
		if asset.Status == media.StatusProcessing {
			status = http.StatusAccepted
		}
		httpx.WriteJSON(w, status, newMediaAssetResponse(svc, asset))
	}
}

func originalMediaHandler(svc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := auth.UserFromContext(r.Context())
		url, err := svc.OriginalURL(r.Context(), actor, chi.URLParam(r, "id"))
		if err != nil {
			writeMediaError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"url": url, "expiresIn": "300s"})
	}
}

// uploadHandler accepts a direct PUT for a single file or one multipart part.
func uploadHandler(svc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		part := 0
		if raw := chi.URLParam(r, "part"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				httpx.WriteError(w, r, http.StatusBadRequest, "validation_error", "分片序号无效", nil)
				return
			}
			part = parsed
		}
		if err := svc.AcceptUpload(r.Context(), chi.URLParam(r, "id"), part, r.URL.Query().Get("token"), r.Body, r.ContentLength); err != nil {
			writeMediaError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// signedAssetHandler serves a private original through a signed capability URL.
func signedAssetHandler(svc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path, err := svc.SignedFilePath(r.URL.Query().Get("key"), r.URL.Query().Get("expires"), time.Now())
		if err != nil {
			writeMediaError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Robots-Tag", "noindex")
		w.Header().Set("Content-Disposition", "inline")
		http.ServeFile(w, r, path)
	}
}

// signedPutHandler accepts a generic capability PUT created by PresignPut.
func signedPutHandler(svc *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if err := svc.AcceptSignedPut(r.Context(), query.Get("key"), query.Get("expires"), r.Body, r.ContentLength); err != nil {
			writeMediaError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func newMediaAssetResponse(svc *media.Service, asset media.Asset) mediaAssetResponse {
	response := mediaAssetResponse{
		ID:               asset.ID,
		Purpose:          asset.Purpose,
		Status:           asset.Status,
		OriginalFilename: asset.OriginalFilename,
		MimeType:         asset.MimeType,
		FileSize:         asset.FileSize,
		Width:            asset.Width,
		Height:           asset.Height,
		Blurhash:         asset.Blurhash,
		Exif:             asset.Exif,
		Variants:         make([]mediaVariantResponse, 0, len(asset.Variants)),
		ProcessedAt:      asset.ProcessedAt,
		CreatedAt:        asset.CreatedAt,
		UpdatedAt:        asset.UpdatedAt,
	}
	for _, name := range media.DisplayVariants {
		variant, ok := asset.Variants[name]
		if !ok {
			continue
		}
		response.Variants = append(response.Variants, mediaVariantResponse{
			Name:   name,
			URL:    svc.VariantURL(variant.Key),
			Width:  variant.Width,
			Height: variant.Height,
			Size:   variant.Size,
		})
	}
	if asset.Status == media.StatusReady {
		if url, err := svc.OriginalURLFor(asset); err == nil {
			response.OriginalURL = url
		}
	}
	if asset.Status == media.StatusFailed {
		response.Error = &mediaErrorResponse{Code: asset.ErrorCode, Message: asset.ErrorMessage}
	}
	return response
}

func writeMediaError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, storage.ErrInvalidSignature):
		httpx.WriteError(w, r, http.StatusForbidden, "invalid_signature", "链接已过期或签名无效", nil)
	case errors.Is(err, storage.ErrInvalidKey):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_key", "存储路径不合法", nil)
	case errors.Is(err, storage.ErrTooLarge), errors.Is(err, media.ErrFileTooLarge):
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "file_too_large", "文件超过大小限制", nil)
	case errors.Is(err, media.ErrBatchLimit):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "batch_limit_exceeded", media.Message(err), nil)
	case errors.Is(err, media.ErrUnsupportedFormat):
		httpx.WriteError(w, r, http.StatusUnsupportedMediaType, "unsupported_format", media.Message(err), nil)
	case errors.Is(err, media.ErrPixelLimit):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "pixel_limit_exceeded", media.Message(err), nil)
	case errors.Is(err, media.ErrUploadIncomplete):
		httpx.WriteError(w, r, http.StatusConflict, "upload_incomplete", media.Message(err), nil)
	case errors.Is(err, media.ErrConflict):
		httpx.WriteError(w, r, http.StatusConflict, "conflict", media.Message(err), nil)
	case errors.Is(err, media.ErrInvalidInput):
		httpx.WriteError(w, r, http.StatusBadRequest, "validation_error", media.Message(err), nil)
	case errors.Is(err, media.ErrNotFound), errors.Is(err, storage.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "媒体不存在", nil)
	case errors.Is(err, media.ErrForbidden):
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "无权访问", nil)
	case errors.Is(err, storage.ErrUnsupported):
		httpx.WriteError(w, r, http.StatusNotImplemented, "not_implemented", "当前存储适配器不支持该操作", nil)
	default:
		slog.ErrorContext(r.Context(), "media handler failed", "error", err, "path", r.URL.Path)
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "服务器内部错误", nil)
	}
}
