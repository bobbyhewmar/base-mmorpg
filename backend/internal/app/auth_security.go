package app

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	passwordAlgorithmSHA256   = "sha256"
	passwordAlgorithmBcryptV1 = "bcrypt_v1"
	defaultAccessTokenTTL     = 2 * time.Hour
)

type RateLimitConfig struct {
	MaxAttempts int
	Window      time.Duration
}

type ServerConfig struct {
	AllowedOrigins       []string
	AccessTokenTTL       time.Duration
	AuthRateLimit        RateLimitConfig
	AttachRateLimit      RateLimitConfig
	InternalAuditEnabled bool
	InternalAuditToken   string
}

type fixedWindowRateLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	max     int
	entries map[string]rateLimitEntry
}

type rateLimitEntry struct {
	windowStartedAt time.Time
	count           int
}

func defaultServerConfig() ServerConfig {
	return ServerConfig{
		AccessTokenTTL: defaultAccessTokenTTL,
		AuthRateLimit: RateLimitConfig{
			MaxAttempts: 8,
			Window:      time.Minute,
		},
		AttachRateLimit: RateLimitConfig{
			MaxAttempts: 16,
			Window:      time.Minute,
		},
	}
}

func normalizeServerConfig(config ServerConfig) ServerConfig {
	defaults := defaultServerConfig()
	if config.AccessTokenTTL <= 0 {
		config.AccessTokenTTL = defaults.AccessTokenTTL
	}
	if config.AuthRateLimit.MaxAttempts <= 0 || config.AuthRateLimit.Window <= 0 {
		config.AuthRateLimit = defaults.AuthRateLimit
	}
	if config.AttachRateLimit.MaxAttempts <= 0 || config.AttachRateLimit.Window <= 0 {
		config.AttachRateLimit = defaults.AttachRateLimit
	}
	config.AllowedOrigins = normalizeAllowedOrigins(config.AllowedOrigins)
	return config
}

func newFixedWindowRateLimiter(config RateLimitConfig) *fixedWindowRateLimiter {
	if config.MaxAttempts <= 0 || config.Window <= 0 {
		return nil
	}
	return &fixedWindowRateLimiter{
		window:  config.Window,
		max:     config.MaxAttempts,
		entries: map[string]rateLimitEntry{},
	}
}

func (limiter *fixedWindowRateLimiter) Allow(key string, now time.Time) bool {
	if limiter == nil || key == "" {
		return true
	}

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	entry, exists := limiter.entries[key]
	if !exists || now.Sub(entry.windowStartedAt) >= limiter.window {
		limiter.entries[key] = rateLimitEntry{
			windowStartedAt: now,
			count:           1,
		}
		return true
	}
	if entry.count >= limiter.max {
		return false
	}
	entry.count++
	limiter.entries[key] = entry
	return true
}

func normalizeAllowedOrigins(origins []string) []string {
	if len(origins) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(origins))
	seen := map[string]struct{}{}
	for _, rawOrigin := range origins {
		trimmed := strings.TrimSpace(rawOrigin)
		if trimmed == "" {
			continue
		}
		parsed, err := url.Parse(trimmed)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			continue
		}
		origin := parsed.Scheme + "://" + parsed.Host
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		normalized = append(normalized, origin)
	}
	return normalized
}

func hashPasswordSHA256(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

func hashPasswordForStorage(password string) (string, string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", "", err
	}
	return string(hash), passwordAlgorithmBcryptV1, nil
}

func newCredentialRecord(accountID string, password string) (*CredentialRecord, error) {
	passwordHash, passwordAlgorithm, err := hashPasswordForStorage(password)
	if err != nil {
		return nil, err
	}
	return &CredentialRecord{
		AccountID:         accountID,
		PasswordHash:      passwordHash,
		PasswordAlgorithm: passwordAlgorithm,
	}, nil
}

func verifyPassword(password string, credential *CredentialRecord) (matched bool, needsUpgrade bool, err error) {
	if credential == nil {
		return false, false, errors.New("missing credential")
	}

	switch credential.PasswordAlgorithm {
	case passwordAlgorithmBcryptV1:
		err := bcrypt.CompareHashAndPassword([]byte(credential.PasswordHash), []byte(password))
		if err == nil {
			return true, false, nil
		}
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return false, false, nil
		}
		return false, false, err
	case passwordAlgorithmSHA256, "":
		expectedHash := hashPasswordSHA256(password)
		matched := subtle.ConstantTimeCompare([]byte(expectedHash), []byte(credential.PasswordHash)) == 1
		return matched, matched, nil
	default:
		return false, false, errors.New("unsupported password algorithm")
	}
}

func newAccountSession(accountID string, ttl time.Duration, now time.Time) *AccountSession {
	return &AccountSession{
		Token:     randomID("access"),
		AccountID: accountID,
		ExpiresAt: now.Add(ttl),
	}
}

func requestRemoteIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

func authRateLimitKeys(r *http.Request, login string) []string {
	keys := make([]string, 0, 2)
	if remoteIP := requestRemoteIP(r); remoteIP != "" {
		keys = append(keys, "ip:"+remoteIP)
	}
	login = strings.TrimSpace(strings.ToLower(login))
	if login != "" {
		keys = append(keys, "login:"+login)
	}
	return keys
}
