package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ambatumove/core/internal/identity"
)

type requestWindow struct {
	started time.Time
	count   int
}
type partnerContextKey struct{}
type limiter struct {
	mu      sync.Mutex
	entries map[string]requestWindow
}

func newLimiter() *limiter { return &limiter{entries: make(map[string]requestWindow)} }
func (l *limiter) allow(key string, limit int, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if entry, ok := l.entries[key]; ok && now.Sub(entry.started) < time.Minute {
		if entry.count >= limit {
			return false
		}
		entry.count++
		l.entries[key] = entry
		return true
	}
	if len(l.entries) >= 4096 {
		for k, v := range l.entries {
			if now.Sub(v.started) >= time.Minute {
				delete(l.entries, k)
			}
		}
		if len(l.entries) >= 4096 {
			return false
		}
	}
	l.entries[key] = requestWindow{started: now, count: 1}
	return true
}

func rateLimitMiddleware(publicRPM, partnerRPM int, keys *limiter, trusted []string, auth *identity.Service, next http.Handler) http.Handler {
	trustedNetworks := make([]*net.IPNet, 0, len(trusted))
	for _, value := range trusted {
		if ip := net.ParseIP(value); ip != nil {
			bits := 128
			if ip.To4() != nil {
				bits = 32
				ip = ip.To4()
			}
			trustedNetworks = append(trustedNetworks, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		if _, block, err := net.ParseCIDR(value); err == nil {
			trustedNetworks = append(trustedNetworks, block)
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r, trustedNetworks)
		route := rateGroup(r.URL.Path)
		limit := publicRPM
		if strings.HasPrefix(route, "auth") {
			limit = 5
		}
		identityKey := ip
		if partner, ok := r.Context().Value(partnerContextKey{}).(identity.PartnerKey); ok && partner.ID != "" {
			digest := sha256.Sum256([]byte(partner.ID))
			identityKey = "partner:" + hex.EncodeToString(digest[:8])
			if partnerRPM > 0 {
				limit = partnerRPM
			}
		} else if auth != nil {
			if bearer := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")); bearer != "" {
				if principal, err := auth.Authenticate(r.Context(), bearer); err == nil && principal.UserID != "" {
					identityKey = "user:" + principal.UserID
				}
			}
		}
		key := identityKey + ":" + route
		if !keys.allow(key, limit, time.Now()) {
			w.Header().Set("Retry-After", "60")
			WriteError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "Request rate limit exceeded.", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request, trusted []*net.IPNet) string {
	peer := remoteIP(r.RemoteAddr)
	if peer == nil {
		return r.RemoteAddr
	}
	if !isTrusted(peer, trusted) {
		return peer.String()
	}
	forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if forwarded == "" {
		return peer.String()
	}
	parts := strings.Split(forwarded, ",")
	chain := make([]net.IP, 0, len(parts)+1)
	for _, part := range parts {
		ip := net.ParseIP(strings.TrimSpace(part))
		if ip == nil {
			return peer.String()
		}
		chain = append(chain, ip)
	}
	chain = append(chain, peer)
	for i := len(chain) - 1; i > 0; i-- {
		if !isTrusted(chain[i], trusted) {
			return chain[i].String()
		}
		if !isTrusted(chain[i-1], trusted) {
			return chain[i-1].String()
		}
	}
	return chain[0].String()
}

func remoteIP(remote string) net.IP {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	return net.ParseIP(strings.Trim(host, "[]"))
}
func isTrusted(ip net.IP, trusted []*net.IPNet) bool {
	for _, block := range trusted {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}
func rateGroup(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "v1" {
		return parts[2]
	}
	return "system"
}

func plannerLimitMiddleware(max int, next http.Handler) http.Handler {
	if max < 1 {
		max = 2
	}
	slots := make(chan struct{}, max)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isPlannerRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
			next.ServeHTTP(w, r)
		default:
			w.Header().Set("Retry-After", "1")
			WriteError(w, r, http.StatusServiceUnavailable, "PLANNER_BUSY", "Workout planning capacity is busy; retry shortly.", nil)
		}
	})
}

func plannerDeadlineMiddleware(timeout time.Duration, next http.Handler) http.Handler {
	if timeout <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isPlannerRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isPlannerRequest(r *http.Request) bool {
	path := r.URL.Path
	if r.Method != http.MethodPost && r.Method != http.MethodPatch {
		return false
	}
	return path == "/api/v1/workouts/generate" || path == "/api/v1/workouts/adapt-preview" || path == "/api/v1/programs/generate" ||
		strings.HasSuffix(path, "/alternatives") || strings.HasSuffix(path, "/adapt") || strings.HasSuffix(path, "/replace")
}

func partnerAccessMiddleware(service *identity.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get("X-API-Key")
		if raw == "" || service == nil {
			next.ServeHTTP(w, r)
			return
		}
		key, err := service.AuthenticatePartner(r.Context(), raw)
		if err != nil {
			WriteError(w, r, http.StatusUnauthorized, "INVALID_API_KEY", "The partner API key is invalid or revoked.", nil)
			return
		}
		required := partnerScope(r.URL.Path)
		if required != "" && !hasScope(key.Scopes, required) {
			WriteError(w, r, http.StatusForbidden, "INSUFFICIENT_SCOPE", "The partner API key does not grant this operation.", nil)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), partnerContextKey{}, key))
		next.ServeHTTP(w, r)
	})
}
func partnerScope(path string) string {
	switch {
	case strings.Contains(path, "/gyms") || strings.Contains(path, "/facilities"):
		return "gyms:read"
	case strings.Contains(path, "/exercises") || strings.Contains(path, "/equipment") || strings.Contains(path, "/muscle-groups") || strings.Contains(path, "/exercise-categories"):
		if strings.Contains(path, "/alternatives") {
			return "planner:preview"
		}
		return "catalog:read"
	case strings.Contains(path, "/calories"):
		return "calories:estimate"
	case strings.Contains(path, "/workouts") || strings.Contains(path, "/programs"):
		return "planner:preview"
	}
	return ""
}
func hasScope(scopes []string, want string) bool {
	for _, scope := range scopes {
		if scope == want {
			return true
		}
	}
	return false
}

func corsMiddleware(allowed []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		for _, value := range allowed {
			if origin != "" && origin == value {
				w.Header().Set("Access-Control-Allow-Origin", value)
				w.Header().Add("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, If-Match, X-API-Key")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
		}
		if r.Method == http.MethodOptions && origin != "" {
			WriteError(w, r, http.StatusForbidden, "CORS_ORIGIN_DENIED", "The request origin is not allowed.", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
