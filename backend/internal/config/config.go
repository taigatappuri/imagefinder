package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv                 string
	HTTPAddr               string
	FrontendBaseURL        string
	DatabaseURL            string
	AutoMigrate            bool
	SessionSecret          string
	SessionTTL             time.Duration
	TokenEncryptionKey     string
	KMSKeyID               string
	AWSRegion              string
	GoogleClientID         string
	GoogleClientSecret     string
	GoogleRedirectURL      string
	GoogleOAuthScopes      []string
	GoogleOAuthAuthURL     string
	GoogleOAuthTokenURL    string
	GoogleUserInfoURL      string
	GooglePhotosAPIBase    string
	GooglePhotosPickerBase string
	GooglePhotosMode       string
	GooglePhotosPageSize   int
	GeminiMode             string
	GeminiAPIKey           string
	GeminiAPIEndpoint      string
	GeminiModel            string
	OpenAIMode             string
	OpenAIAPIKey           string
	OpenAIAPIEndpoint      string
	OpenAIModel            string
	EmbeddingDim           int
	MaxSearchLimit         int
	MaxQueryLength         int
	MaxLocationLength      int
	MaxEmbeddingTextLength int
	ExternalAPIRetryMax    int
	ExternalAPIRetryDelay  time.Duration
	JobPollInterval        time.Duration
	CookieSecure           bool
	AllowedCORSOrigins     []string
}

func Load() Config {
	return Config{
		AppEnv:                 getEnv("APP_ENV", "development"),
		HTTPAddr:               getEnv("HTTP_ADDR", ":8080"),
		FrontendBaseURL:        getEnv("FRONTEND_BASE_URL", "http://localhost:5173"),
		DatabaseURL:            getEnv("DATABASE_URL", "postgres://imagefinder:imagefinder@localhost:5432/imagefinder?sslmode=disable"),
		AutoMigrate:            getEnvBool("DB_AUTO_MIGRATE", true),
		SessionSecret:          getEnv("SESSION_SECRET", "dev-secret"),
		SessionTTL:             getEnvDuration("SESSION_TTL", 24*time.Hour*7),
		TokenEncryptionKey:     getEnv("TOKEN_ENCRYPTION_KEY", ""),
		KMSKeyID:               getEnv("KMS_KEY_ID", ""),
		AWSRegion:              getEnv("AWS_REGION", ""),
		GoogleClientID:         getEnv("GOOGLE_OAUTH_CLIENT_ID", ""),
		GoogleClientSecret:     getEnv("GOOGLE_OAUTH_CLIENT_SECRET", ""),
		GoogleRedirectURL:      getEnv("GOOGLE_OAUTH_REDIRECT_URL", "http://localhost:8080/auth/callback"),
		GoogleOAuthScopes:      splitEnv("GOOGLE_OAUTH_SCOPES", "https://www.googleapis.com/auth/photospicker.mediaitems.readonly openid email"),
		GoogleOAuthAuthURL:     getEnv("GOOGLE_OAUTH_AUTH_URL", "https://accounts.google.com/o/oauth2/v2/auth"),
		GoogleOAuthTokenURL:    getEnv("GOOGLE_OAUTH_TOKEN_URL", "https://oauth2.googleapis.com/token"),
		GoogleUserInfoURL:      getEnv("GOOGLE_USERINFO_URL", "https://www.googleapis.com/oauth2/v3/userinfo"),
		GooglePhotosAPIBase:    getEnv("GOOGLE_PHOTOS_API_BASE", "https://photoslibrary.googleapis.com/v1"),
		GooglePhotosPickerBase: getEnv("GOOGLE_PHOTOS_PICKER_BASE", "https://photospicker.googleapis.com/v1"),
		GooglePhotosMode:       getEnv("GOOGLE_PHOTOS_MODE", "picker"),
		GooglePhotosPageSize:   getEnvInt("GOOGLE_PHOTOS_PAGE_SIZE", 50),
		GeminiMode:             getEnv("GEMINI_MODE", "mock"),
		GeminiAPIKey:           getEnv("GEMINI_API_KEY", ""),
		GeminiAPIEndpoint:      getEnv("GEMINI_API_ENDPOINT", "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent"),
		GeminiModel:            getEnv("GEMINI_MODEL", "gemini-2.5-flash"),
		OpenAIMode:             getEnv("OPENAI_MODE", "mock"),
		OpenAIAPIKey:           getEnv("OPENAI_API_KEY", ""),
		OpenAIAPIEndpoint:      getEnv("OPENAI_API_ENDPOINT", "https://api.openai.com/v1/embeddings"),
		OpenAIModel:            getEnv("OPENAI_MODEL", "text-embedding-3-small"),
		EmbeddingDim:           getEnvInt("EMBEDDING_DIM", 1536),
		MaxSearchLimit:         getEnvInt("MAX_SEARCH_LIMIT", 30),
		MaxQueryLength:         getEnvInt("MAX_QUERY_LENGTH", 200),
		MaxLocationLength:      getEnvInt("MAX_LOCATION_LENGTH", 100),
		MaxEmbeddingTextLength: getEnvInt("MAX_EMBEDDING_TEXT_LENGTH", 2000),
		ExternalAPIRetryMax:    getEnvInt("EXTERNAL_API_RETRY_MAX", 3),
		ExternalAPIRetryDelay:  getEnvDuration("EXTERNAL_API_RETRY_BASE_DELAY", 500*time.Millisecond),
		JobPollInterval:        getEnvDuration("JOB_POLL_INTERVAL", 3*time.Second),
		CookieSecure:           getEnvBool("COOKIE_SECURE", false),
		AllowedCORSOrigins:     splitEnv("CORS_ALLOWED_ORIGINS", "http://localhost:5173"),
	}
}

func getEnv(key, def string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return def
	}
	return value
}

func splitEnv(key, def string) []string {
	value := getEnv(key, def)
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return []string{}
	}
	return parts
}

func getEnvInt(key string, def int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return def
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return def
	}
	return parsed
}

func getEnvBool(key string, def bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return def
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return def
	}
	return parsed
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return def
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return def
	}
	return parsed
}
