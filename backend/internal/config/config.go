package config

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type Config struct {
	AppEnv                 string
	HTTPAddr               string
	DatabasePath           string
	LogLevel               string
	JWTSigningKey          []byte
	JWTIssuer              string
	JWTAudience            string
	PublicBaseURL          string
	CORSAllowedOrigins     []string
	PlannerMaxGenerationMS int
	PublicRateLimitRPM     int
	PartnerRateLimitRPM    int
	ReferenceCacheEnabled  bool
	GymCacheEnabled        bool
	SeedSyntheticGyms      bool
	TrustedProxyCIDRs      []string
}

type lookupEnv func(string) (string, bool)

func Load(getenv lookupEnv) (Config, error) {
	value := func(key, fallback string) string {
		if v, ok := getenv(key); ok {
			return strings.TrimSpace(v)
		}
		return fallback
	}
	cfg := Config{
		AppEnv:                value("APP_ENV", "development"),
		HTTPAddr:              value("HTTP_ADDR", ":8080"),
		DatabasePath:          value("DATABASE_PATH", "./data/ambatumove.sqlite"),
		LogLevel:              strings.ToLower(value("LOG_LEVEL", "info")),
		JWTIssuer:             value("JWT_ISSUER", "ambatumove"),
		JWTAudience:           value("JWT_AUDIENCE", "ambatumove-mobile"),
		PublicBaseURL:         value("PUBLIC_BASE_URL", ""),
		CORSAllowedOrigins:    splitCSV(value("CORS_ALLOWED_ORIGINS", "")),
		TrustedProxyCIDRs:     splitCSV(value("TRUSTED_PROXY_CIDRS", "")),
		ReferenceCacheEnabled: boolValue(value("REFERENCE_CACHE_ENABLED", "true")),
		GymCacheEnabled:       boolValue(value("GYM_CACHE_ENABLED", "true")),
		SeedSyntheticGyms:     boolValue(value("SEED_SYNTHETIC_GYMS", "false")),
	}
	var err error
	if cfg.PlannerMaxGenerationMS, err = intValue(value("PLANNER_MAX_GENERATION_MS", "1500"), "PLANNER_MAX_GENERATION_MS", 100, 10000); err != nil {
		return Config{}, err
	}
	if cfg.PublicRateLimitRPM, err = intValue(value("PUBLIC_RATE_LIMIT_RPM", "60"), "PUBLIC_RATE_LIMIT_RPM", 1, 600); err != nil {
		return Config{}, err
	}
	if cfg.PartnerRateLimitRPM, err = intValue(value("PARTNER_RATE_LIMIT_RPM", "120"), "PARTNER_RATE_LIMIT_RPM", 1, 1200); err != nil {
		return Config{}, err
	}
	if cfg.AppEnv != "development" && cfg.AppEnv != "test" && cfg.AppEnv != "production" {
		return Config{}, fmt.Errorf("APP_ENV must be development, test, or production")
	}
	if _, err := strconv.ParseBool(value("REFERENCE_CACHE_ENABLED", "true")); err != nil {
		return Config{}, fmt.Errorf("REFERENCE_CACHE_ENABLED must be true or false")
	}
	if _, err := strconv.ParseBool(value("GYM_CACHE_ENABLED", "true")); err != nil {
		return Config{}, fmt.Errorf("GYM_CACHE_ENABLED must be true or false")
	}
	if _, err := strconv.ParseBool(value("SEED_SYNTHETIC_GYMS", "false")); err != nil {
		return Config{}, fmt.Errorf("SEED_SYNTHETIC_GYMS must be true or false")
	}
	if cfg.AppEnv == "production" && cfg.SeedSyntheticGyms {
		return Config{}, fmt.Errorf("SEED_SYNTHETIC_GYMS is only allowed in development or test environments")
	}
	host, port, addrErr := net.SplitHostPort(cfg.HTTPAddr)
	portNumber, portErr := strconv.Atoi(port)
	if addrErr != nil || portErr != nil || portNumber < 1 || portNumber > 65535 || strings.ContainsAny(host, "\r\n") {
		return Config{}, fmt.Errorf("HTTP_ADDR must be a host:port address with port 1..65535")
	}
	for _, origin := range cfg.CORSAllowedOrigins {
		parsed, parseErr := url.Parse(origin)
		if parseErr != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || origin == "*" {
			return Config{}, fmt.Errorf("CORS_ALLOWED_ORIGINS entries must be explicit HTTP(S) origins without a path")
		}
	}
	for _, trusted := range cfg.TrustedProxyCIDRs {
		if net.ParseIP(trusted) != nil {
			continue
		}
		if _, _, parseErr := net.ParseCIDR(trusted); parseErr != nil {
			return Config{}, fmt.Errorf("TRUSTED_PROXY_CIDRS entries must be IP addresses or CIDRs")
		}
	}
	if cfg.DatabasePath == ":memory:" {
		return Config{}, fmt.Errorf("DATABASE_PATH must name a persistent database file")
	}
	if cfg.HTTPAddr == "" {
		return Config{}, fmt.Errorf("HTTP_ADDR must not be empty")
	}
	if cfg.DatabasePath == "" {
		return Config{}, fmt.Errorf("DATABASE_PATH must not be empty")
	}
	if cfg.LogLevel != "debug" && cfg.LogLevel != "info" && cfg.LogLevel != "warn" && cfg.LogLevel != "error" {
		return Config{}, fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error")
	}
	if cfg.AppEnv == "production" {
		encoded := value("JWT_SIGNING_KEY", "")
		key, decodeErr := base64.StdEncoding.DecodeString(encoded)
		if decodeErr != nil || len(key) < 32 {
			return Config{}, fmt.Errorf("JWT_SIGNING_KEY must be base64-encoded and contain at least 32 bytes in production")
		}
		cfg.JWTSigningKey = key
	} else if encoded := value("JWT_SIGNING_KEY", ""); encoded != "" {
		key, decodeErr := base64.StdEncoding.DecodeString(encoded)
		if decodeErr != nil || len(key) < 32 {
			return Config{}, fmt.Errorf("JWT_SIGNING_KEY must be valid base64 containing at least 32 bytes")
		}
		cfg.JWTSigningKey = key
	}
	return cfg, nil
}

func intValue(raw, name string, min, max int) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil || n < min || n > max {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, min, max)
	}
	return n, nil
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func boolValue(value string) bool {
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}
