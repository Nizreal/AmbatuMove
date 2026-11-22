package config

import (
	"strings"
	"testing"
)

func TestLoadUsesDevelopmentDefaults(t *testing.T) {
	got, err := Load(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.HTTPAddr != ":8080" || got.DatabasePath != "./data/ambatumove.sqlite" || got.LogLevel != "info" {
		t.Fatalf("unexpected defaults: %+v", got)
	}
	if got.SeedSyntheticGyms {
		t.Fatal("synthetic gyms must be disabled by default")
	}
}

func TestSyntheticGymSeedingRequiresExplicitNonProductionOptIn(t *testing.T) {
	for _, env := range []string{"development", "test"} {
		cfg, err := Load(func(key string) (string, bool) {
			if key == "APP_ENV" {
				return env, true
			}
			if key == "SEED_SYNTHETIC_GYMS" {
				return "true", true
			}
			return "", false
		})
		if err != nil || !cfg.SeedSyntheticGyms {
			t.Fatalf("explicit %s fixture opt-in config=%+v err=%v", env, cfg, err)
		}
	}
	_, err := Load(func(key string) (string, bool) {
		switch key {
		case "APP_ENV":
			return "production", true
		case "JWT_SIGNING_KEY":
			return "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", true
		case "SEED_SYNTHETIC_GYMS":
			return "true", true
		default:
			return "", false
		}
	})
	if err == nil || !strings.Contains(err.Error(), "SEED_SYNTHETIC_GYMS") {
		t.Fatalf("production synthetic seed config error=%v, want explicit prohibition", err)
	}
}

func TestLoadRejectsOutOfRangePlannerTimeout(t *testing.T) {
	_, err := Load(func(key string) (string, bool) {
		if key == "PLANNER_MAX_GENERATION_MS" {
			return "0", true
		}
		return "", false
	})
	if err == nil || !strings.Contains(err.Error(), "PLANNER_MAX_GENERATION_MS") {
		t.Fatalf("Load() error = %v, want validation error for planner timeout", err)
	}
}

func TestLoadRequiresStrongProductionJWTKey(t *testing.T) {
	_, err := Load(func(key string) (string, bool) {
		if key == "APP_ENV" {
			return "production", true
		}
		return "", false
	})
	if err == nil || !strings.Contains(err.Error(), "JWT_SIGNING_KEY") {
		t.Fatalf("Load() error = %v, want production JWT key validation", err)
	}
}

func TestLoadRejectsWildcardAndMalformedCorsOrigins(t *testing.T) {
	for _, origin := range []string{"*", "https://example.test/path", "ftp://example.test"} {
		_, err := Load(func(key string) (string, bool) {
			if key == "CORS_ALLOWED_ORIGINS" {
				return origin, true
			}
			return "", false
		})
		if err == nil || !strings.Contains(err.Error(), "CORS_ALLOWED_ORIGINS") {
			t.Fatalf("origin %q error=%v", origin, err)
		}
	}
}

func TestLoadValidatesTrustedProxyNetworks(t *testing.T) {
	for _, value := range []string{"10.0.0.1,2001:db8::1,172.29.240.0/24", ""} {
		cfg, err := Load(func(key string) (string, bool) {
			if key == "TRUSTED_PROXY_CIDRS" {
				return value, true
			}
			return "", false
		})
		if err != nil {
			t.Fatalf("valid proxy list %q: %v", value, err)
		}
		if value == "" && len(cfg.TrustedProxyCIDRs) != 0 {
			t.Fatalf("default trusted proxies = %v", cfg.TrustedProxyCIDRs)
		}
	}
	_, err := Load(func(key string) (string, bool) {
		if key == "TRUSTED_PROXY_CIDRS" {
			return "not-an-address", true
		}
		return "", false
	})
	if err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXY_CIDRS") {
		t.Fatalf("invalid proxy list error=%v", err)
	}
}
