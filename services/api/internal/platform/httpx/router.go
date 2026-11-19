package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// Pinger reports whether a downstream dependency, normally the database, is reachable.
type Pinger interface {
	Ping(context.Context) error
}

// Options describes the shared HTTP router.
type Options struct {
	Pinger      Pinger
	Logger      *slog.Logger
	CORSOrigins []string
	// Register mounts additional routes under /api/v1.
	Register func(chi.Router)
}

type contextKey string

const requestIDKey contextKey = "request_id"

type errorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details"`
	RequestID string `json:"requestId"`
}

// NewRouter keeps the stage 1 signature for tests and simple usage.
func NewRouter(p Pinger, logger *slog.Logger) http.Handler {
	return NewRouterWithOptions(Options{Pinger: p, Logger: logger})
}

// NewRouterWithOptions builds the shared middleware chain and mounts optional routes.
func NewRouterWithOptions(opts Options) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	r := chi.NewRouter()
	r.Use(CORS(opts.CORSOrigins))
	r.Use(RequestID)
	r.Use(AccessLog(logger))
	r.Use(Recoverer(logger))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if opts.Pinger == nil || opts.Pinger.Ping(ctx) != nil {
			WriteError(w, r, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用", nil)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/capabilities", func(w http.ResponseWriter, r *http.Request) {
			WriteJSON(w, http.StatusOK, map[string]any{
				"modules":     []string{},
				"menus":       []string{},
				"permissions": []string{},
			})
		})
		if opts.Register != nil {
			opts.Register(r)
		}
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusNotFound, "not_found", "请求的资源不存在", nil)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", nil)
	})

	return r
}

// CORS allows credentialed requests from the configured browser origins only.
func CORS(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		origin = strings.TrimRight(strings.TrimSpace(origin), "/")
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if _, ok := allowed[origin]; ok && origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
				w.Header().Set("Access-Control-Max-Age", "600")
				w.Header().Add("Vary", "Origin")
			}
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func AccessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)
			logger.Info("http_request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", recorder.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", RequestIDFromContext(r.Context()),
			)
		})
	}
}

func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("panic",
						"error", recovered,
						"stack", string(debug.Stack()),
						"request_id", RequestIDFromContext(r.Context()),
					)
					WriteError(w, r, http.StatusInternalServerError, "internal_error", "服务器内部错误", nil)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string, details any) {
	WriteJSON(w, status, errorResponse{
		Code:      code,
		Message:   message,
		Details:   details,
		RequestID: RequestIDFromContext(r.Context()),
	})
}

func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func newRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(buf[:])
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
