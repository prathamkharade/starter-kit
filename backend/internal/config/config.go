// Package config loads all runtime configuration from environment variables
// (optionally from a .env file in development). Every variable is documented
// in backend/.env.example — keep the two in sync.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env         string // development | production | test
	AppName     string
	HTTPAddr    string
	PublicURL   string
	CORSOrigins []string
	TrustProxy  bool
	Proxies     []string

	DatabaseURL string
	DBMaxConns  int32

	JWTSecret  []byte
	JWTIssuer  string
	AccessTTL  time.Duration
	RefreshTTL time.Duration

	DataEncKey   []byte // AES-256 key for field-level encryption
	DataIndexKey []byte // HMAC key for blind indexes / OTP hashing

	WorkerEmbedded    bool
	WorkerConcurrency int

	EmailProvider string // console | smtp
	SMTPHost      string
	SMTPPort      int
	SMTPUser      string
	SMTPPass      string
	SMTPFrom      string
	SMTPTLS       string // starttls | tls | none

	SMSProvider         string // none | console | twilio | http
	TwilioSID           string
	TwilioToken         string
	TwilioFrom          string
	SMSHTTPURL          string
	SMSHTTPAuthHeader   string
	SMSHTTPBodyTemplate string

	S3Endpoint      string
	S3Region        string
	S3Bucket        string
	S3AccessKey     string
	S3SecretKey     string
	S3UseSSL        bool
	S3PublicBaseURL string

	MediaMaxBytes     int64
	MediaImageFormat  string // webp | avif | jpeg
	MediaImageQuality int
	MediaVideoCodec   string // h264 | hevc | av1
	MediaVideoCRF     int
	MediaVideoPreset  string
	MediaVideoMaxH    int
	MediaKeepOriginal bool

	TermsVersion   string
	PrivacyVersion string
	SignupViaOTP   bool
	DeletionGrace  time.Duration
	AuditRetention time.Duration

	AppMinVersion    string
	AppLatestVersion string
	Maintenance      bool
}

func (c *Config) IsProd() bool { return c.Env == "production" }

// Load reads .env (if present) then the process environment.
func Load() (*Config, error) {
	_ = godotenv.Load() // ignore missing file; real env always wins

	c := &Config{
		Env:         str("APP_ENV", "development"),
		AppName:     str("APP_NAME", "App"),
		HTTPAddr:    str("HTTP_ADDR", ":8080"),
		PublicURL:   str("PUBLIC_URL", "http://localhost:8080"),
		CORSOrigins: list("CORS_ORIGINS", "http://localhost:5000"),
		TrustProxy:  boolean("TRUST_PROXY", false),
		Proxies:     list("TRUSTED_PROXIES", "127.0.0.1,::1"),

		DatabaseURL: str("DATABASE_URL", "postgres://app:app@localhost:5432/app?sslmode=disable"),
		DBMaxConns:  int32(integer("DB_MAX_CONNS", 10)),

		JWTSecret:  []byte(str("JWT_SECRET", "")),
		JWTIssuer:  str("JWT_ISSUER", "app"),
		AccessTTL:  dur("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTTL: dur("REFRESH_TOKEN_TTL", 30*24*time.Hour),

		WorkerEmbedded:    boolean("WORKER_EMBEDDED", true),
		WorkerConcurrency: integer("WORKER_CONCURRENCY", 2),

		EmailProvider: str("EMAIL_PROVIDER", "console"),
		SMTPHost:      str("SMTP_HOST", "localhost"),
		SMTPPort:      integer("SMTP_PORT", 1025),
		SMTPUser:      str("SMTP_USER", ""),
		SMTPPass:      str("SMTP_PASS", ""),
		SMTPFrom:      str("SMTP_FROM", "App <no-reply@example.com>"),
		SMTPTLS:       str("SMTP_TLS", "none"),

		SMSProvider:         str("SMS_PROVIDER", "console"),
		TwilioSID:           str("TWILIO_ACCOUNT_SID", ""),
		TwilioToken:         str("TWILIO_AUTH_TOKEN", ""),
		TwilioFrom:          str("TWILIO_FROM", ""),
		SMSHTTPURL:          str("SMS_HTTP_URL", ""),
		SMSHTTPAuthHeader:   str("SMS_HTTP_AUTH_HEADER", ""),
		SMSHTTPBodyTemplate: str("SMS_HTTP_BODY", `{"to":"{{to}}","message":"{{message}}"}`),

		S3Endpoint:      str("S3_ENDPOINT", "localhost:9000"),
		S3Region:        str("S3_REGION", "auto"),
		S3Bucket:        str("S3_BUCKET", "app-media"),
		S3AccessKey:     str("S3_ACCESS_KEY", ""),
		S3SecretKey:     str("S3_SECRET_KEY", ""),
		S3UseSSL:        boolean("S3_USE_SSL", false),
		S3PublicBaseURL: str("S3_PUBLIC_BASE_URL", ""),

		MediaMaxBytes:     int64(integer("MEDIA_MAX_MB", 200)) << 20,
		MediaImageFormat:  str("MEDIA_IMAGE_FORMAT", "webp"),
		MediaImageQuality: integer("MEDIA_IMAGE_QUALITY", 72),
		MediaVideoCodec:   str("MEDIA_VIDEO_CODEC", "h264"),
		MediaVideoCRF:     integer("MEDIA_VIDEO_CRF", 28),
		MediaVideoPreset:  str("MEDIA_VIDEO_PRESET", "veryfast"),
		MediaVideoMaxH:    integer("MEDIA_VIDEO_MAX_HEIGHT", 720),
		MediaKeepOriginal: boolean("MEDIA_KEEP_ORIGINAL", false),

		TermsVersion:   str("TERMS_VERSION", "2026-01-01"),
		PrivacyVersion: str("PRIVACY_VERSION", "2026-01-01"),
		SignupViaOTP:   boolean("SIGNUP_VIA_OTP", true),
		DeletionGrace:  dur("ACCOUNT_DELETION_GRACE", 30*24*time.Hour),
		AuditRetention: dur("AUDIT_RETENTION", 365*24*time.Hour),

		AppMinVersion:    str("APP_MIN_VERSION", "1.0.0"),
		AppLatestVersion: str("APP_LATEST_VERSION", "1.0.0"),
		Maintenance:      boolean("MAINTENANCE_MODE", false),
	}

	var err error
	if c.DataEncKey, err = b64("DATA_ENC_KEY"); err != nil {
		return nil, err
	}
	if c.DataIndexKey, err = b64("DATA_INDEX_KEY"); err != nil {
		return nil, err
	}
	return c, c.validate()
}

// devMarker is embedded in the sample secrets of .env.example so that
// production refuses to boot with them.
const devMarker = "DEV-ONLY"

func (c *Config) validate() error {
	var errs []error
	if len(c.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET must be at least 32 bytes (run: task secrets)"))
	}
	if len(c.DataEncKey) != 32 || len(c.DataIndexKey) != 32 {
		errs = append(errs, errors.New("DATA_ENC_KEY and DATA_INDEX_KEY must be base64 of 32 bytes (run: task secrets)"))
	}
	if c.IsProd() {
		if strings.Contains(string(c.JWTSecret), devMarker) || strings.Contains(os.Getenv("DATA_ENC_KEY"), "REVWLU9OTFk") {
			errs = append(errs, errors.New("development secrets detected in production"))
		}
		if c.EmailProvider == "console" {
			errs = append(errs, errors.New("EMAIL_PROVIDER=console is not allowed in production"))
		}
		for _, o := range c.CORSOrigins {
			if o == "*" {
				errs = append(errs, errors.New("CORS_ORIGINS=* is not allowed in production"))
			}
		}
	}
	return errors.Join(errs...)
}

// ---- helpers ---------------------------------------------------------------

func str(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func integer(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func boolean(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func dur(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func list(k, def string) []string {
	var out []string
	for _, p := range strings.Split(str(k, def), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func b64(k string) ([]byte, error) {
	v := os.Getenv(k)
	if v == "" {
		return nil, nil
	}
	b, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid base64: %w", k, err)
	}
	return b, nil
}
