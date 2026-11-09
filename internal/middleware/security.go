package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/abundo/ergotime-server/internal/config"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// SecurityHeaders adds headers that mitigate XSS, clickjacking, MIME sniffing, etc.
// Suitable when running behind a reverse HTTPS proxy (TLS is terminated upstream).
func SecurityHeaders(cfg *config.Config) echo.MiddlewareFunc {
	csp := cfg.Security.CSP
	if csp == "" {
		csp = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
			// HSTS: safe when always served via HTTPS reverse proxy
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			// Avoid caching authenticated API responses by default
			if strings.HasPrefix(c.Path(), "/api/") {
				h.Set("Cache-Control", "no-store")
			}
			// CSP primarily for HTML; apply globally for defense in depth
			h.Set("Content-Security-Policy", csp)
			return next(c)
		}
	}
}

// CORS returns Echo CORS middleware configured for same-origin SPA + optional extra origins.
func CORS(cfg *config.Config) echo.MiddlewareFunc {
	origins := cfg.CORS.AllowedOrigins
	if len(origins) == 0 {
		// Same-origin only: echo will not reflect arbitrary Origin
		origins = []string{}
	}

	return middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: origins,
		AllowMethods: []string{
			http.MethodGet, http.MethodHead, http.MethodPut,
			http.MethodPatch, http.MethodPost, http.MethodDelete, http.MethodOptions,
		},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
			"X-Requested-With",
		},
		AllowCredentials: cfg.CORS.AllowCredentials,
		MaxAge:           600,
	})
}

// RateLimiter is a simple in-memory sliding window limiter per key (IP).
type RateLimiter struct {
	mu        sync.Mutex
	hits      map[string][]time.Time
	limit     int
	window    time.Duration
	lastPrune time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit <= 0 {
		limit = 10
	}
	return &RateLimiter{
		hits:   make(map[string][]time.Time),
		limit:  limit,
		window: window,
	}
}

func (r *RateLimiter) Allow(key string) bool {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	if now.Sub(r.lastPrune) > r.window {
		for k, times := range r.hits {
			filtered := filterRecent(times, now, r.window)
			if len(filtered) == 0 {
				delete(r.hits, k)
			} else {
				r.hits[k] = filtered
			}
		}
		r.lastPrune = now
	}

	times := filterRecent(r.hits[key], now, r.window)
	if len(times) >= r.limit {
		r.hits[key] = times
		return false
	}
	r.hits[key] = append(times, now)
	return true
}

func filterRecent(times []time.Time, now time.Time, window time.Duration) []time.Time {
	out := times[:0]
	cutoff := now.Add(-window)
	for _, t := range times {
		if t.After(cutoff) {
			out = append(out, t)
		}
	}
	return out
}

// LoginRateLimit middleware applies per-IP rate limiting to login.
func LoginRateLimit(limiter *RateLimiter) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ip := c.RealIP()
			if !limiter.Allow(ip) {
				return echo.NewHTTPError(http.StatusTooManyRequests, "too many login attempts, try again later")
			}
			return next(c)
		}
	}
}
