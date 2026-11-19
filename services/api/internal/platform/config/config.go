package config

import (
	"errors"
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
}

func Load() (Config, error) {
	// Local development convenience. In containers the environment is already set.
	for _, path := range []string{".env", "../../.env"} {
		_ = godotenv.Load(path)
	}

	publicOrigin := env("PUBLIC_ORIGIN", "http://localhost:4321")
	adminOrigin := env("ADMIN_ORIGIN", "http://localhost:5173")

	cfg := Config{
		AppEnv:               env("APP_ENV", "development"),
		HTTPAddr:             env("HTTP_ADDR", ":8080"),
		DatabaseURL:          strings.TrimSpace(os.Getenv("DATABASE_URL")),
		RunMigrations:        boolEnv("RUN_MIGRATIONS", true),
		ShutdownTimeout:      durationEnv("SHUTDOWN_TIMEOUT", 10*time.Second),
		PublicOrigin:         publicOrigin,
		AdminOrigin:          adminOrigin,
		CORSOrigins:          csvEnv("CORS_ORIGINS", publicOrigin+","+adminOrigin),
		JWTSecret:            strings.TrimSpace(os.Getenv("JWT_SECRET")),
		AccessTokenTTL:       durationEnv("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:      durationEnv("REFRESH_TOKEN_TTL", 720*time.Hour),
		RefreshTokenTTLShort: durationEnv("REFRESH_TOKEN_TTL_SHORT", 24*time.Hour),
		CookieSecure:         boolEnv("COOKIE_SECURE", false),
		CookieDomain:         strings.TrimSpace(os.Getenv("COOKIE_DOMAIN")),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	return cfg, nil
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
