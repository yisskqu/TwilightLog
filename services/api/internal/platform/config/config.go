package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv               string
	HTTPAddr             string
	DatabaseURL          string
	RunMigrations        bool
	ShutdownTimeout      time.Duration
	PublicOrigin         string
	AdminOrigin          string
	CORSOrigins          []string
	JWTSecret            string
	AccessTokenTTL       time.Duration
	RefreshTokenTTL      time.Duration
	RefreshTokenTTLShort time.Duration
	CookieSecure         bool
	CookieDomain         string

	StorageDriver      string
	LocalStorageRoot   string
	AssetBaseURL       string
	APIBaseURL         string
	MediaSigningSecret string
	MediaProcessor     string
	MediaProcessAsync  bool
	MediaBatchLimit    int
	MaxArticleBytes    int64
	MaxPhotoBytes      int64
}

func Load() (Config, error) {
	// Local development convenience. In containers the environment is already set.
	for _, path := range []string{".env", "../../.env"} {
		_ = godotenv.Load(path)
	}

	publicOrigin := env("PUBLIC_ORIGIN", "http://localhost:4321")
	adminOrigin := env("ADMIN_ORIGIN", "http://localhost:5173")
	storageDriver := strings.ToLower(env("STORAGE_DRIVER", "local"))
	jwtSecret := strings.TrimSpace(os.Getenv("JWT_SECRET"))

	cfg := Config{
		AppEnv:               env("APP_ENV", "development"),
		HTTPAddr:             env("HTTP_ADDR", ":8080"),
		DatabaseURL:          strings.TrimSpace(os.Getenv("DATABASE_URL")),
		RunMigrations:        boolEnv("RUN_MIGRATIONS", true),
		ShutdownTimeout:      durationEnv("SHUTDOWN_TIMEOUT", 10*time.Second),
		PublicOrigin:         publicOrigin,
		AdminOrigin:          adminOrigin,
		CORSOrigins:          csvEnv("CORS_ORIGINS", publicOrigin+","+adminOrigin),
		JWTSecret:            jwtSecret,
		AccessTokenTTL:       durationEnv("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:      durationEnv("REFRESH_TOKEN_TTL", 720*time.Hour),
		RefreshTokenTTLShort: durationEnv("REFRESH_TOKEN_TTL_SHORT", 24*time.Hour),
		CookieSecure:         boolEnv("COOKIE_SECURE", false),
		CookieDomain:         strings.TrimSpace(os.Getenv("COOKIE_DOMAIN")),

		StorageDriver:      storageDriver,
		LocalStorageRoot:   env("LOCAL_STORAGE_ROOT", "var/uploads"),
		AssetBaseURL:       strings.TrimRight(env("ASSET_BASE_URL", "http://localhost:8080/assets"), "/"),
		APIBaseURL:         strings.TrimRight(env("API_BASE_URL", "http://localhost:8080"), "/"),
		MediaSigningSecret: env("MEDIA_SIGNING_SECRET", jwtSecret),
		MediaProcessor:     strings.ToLower(env("MEDIA_PROCESSOR", defaultProcessor(storageDriver))),
		MediaProcessAsync:  boolEnv("MEDIA_PROCESS_ASYNC", true),
		MediaBatchLimit:    intEnv("MEDIA_BATCH_LIMIT", 20),
		MaxArticleBytes:    int64Env("MEDIA_MAX_ARTICLE_BYTES", 5<<20),
		MaxPhotoBytes:      int64Env("MEDIA_MAX_PHOTO_BYTES", 50<<20),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.StorageDriver != "local" {
		return Config{}, fmt.Errorf("STORAGE_DRIVER %q is not implemented yet; use local", cfg.StorageDriver)
	}
	if cfg.AppEnv == "production" && cfg.MediaProcessor == "local" {
		return Config{}, errors.New("MEDIA_PROCESSOR=local is not allowed in production; use the cloud image function")
	}
	if cfg.MediaBatchLimit <= 0 {
		cfg.MediaBatchLimit = 20
	}

	return cfg, nil
}

func defaultProcessor(storageDriver string) string {
	if storageDriver == "local" {
		return "local"
	}
	return "fc"
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func boolEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func intEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func int64Env(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func csvEnv(key, fallback string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		value = fallback
	}
	var result []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}
