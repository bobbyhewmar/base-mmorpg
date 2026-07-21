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
	passwordAlgorithmSHA256               = "sha256"
	passwordAlgorithmBcryptV1             = "bcrypt_v1"
	defaultAccessTokenTTL                 = 2 * time.Hour
	defaultSessionLeaseDuration           = 30 * time.Second
	defaultSessionLeaseRenewInterval      = 10 * time.Second
	defaultSessionAttachTokenTTL          = 5 * time.Minute
	defaultGameplayEventPollInterval      = 250 * time.Millisecond
	defaultGameplayEventClaimLease        = 5 * time.Second
	defaultGameplayEventRetryDelay        = 500 * time.Millisecond
	defaultGameplayEventRetention         = 24 * time.Hour
	defaultGameplayEventCleanupInterval   = 10 * time.Minute
	defaultGameplayEventBatchSize         = 32
	defaultGameplayEventMaxRetries        = 5
	defaultRegionProjectionTTL            = 6 * time.Second
	defaultRegionProjectionHeartbeat      = 2 * time.Second
	defaultRegionProjectionQueueSize      = 256
	defaultRegionProjectionInterestRadius = 128.0
)

type RateLimitConfig struct {
	MaxAttempts int
	Window      time.Duration
}

type ServerConfig struct {
	AllowedOrigins                 []string
	AccessTokenTTL                 time.Duration
	AuthRateLimit                  RateLimitConfig
	AttachRateLimit                RateLimitConfig
	InternalAuditEnabled           bool
	InternalAuditToken             string
	ServerInstanceID               string
	SessionLeaseDuration           time.Duration
	SessionLeaseRenewInterval      time.Duration
	SessionAttachTokenTTL          time.Duration
	GameplayEventPollInterval      time.Duration
	GameplayEventClaimLease        time.Duration
	GameplayEventRetryDelay        time.Duration
	GameplayEventRetention         time.Duration
	GameplayEventCleanupInterval   time.Duration
	GameplayEventBatchSize         int
	GameplayEventMaxRetries        int
	RegionProjectionTTL            time.Duration
	RegionProjectionHeartbeat      time.Duration
	RegionProjectionQueueSize      int
	RegionProjectionInterestRadius float64
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
		SessionLeaseDuration:           defaultSessionLeaseDuration,
		SessionLeaseRenewInterval:      defaultSessionLeaseRenewInterval,
		SessionAttachTokenTTL:          defaultSessionAttachTokenTTL,
		GameplayEventPollInterval:      defaultGameplayEventPollInterval,
		GameplayEventClaimLease:        defaultGameplayEventClaimLease,
		GameplayEventRetryDelay:        defaultGameplayEventRetryDelay,
		GameplayEventRetention:         defaultGameplayEventRetention,
		GameplayEventCleanupInterval:   defaultGameplayEventCleanupInterval,
		GameplayEventBatchSize:         defaultGameplayEventBatchSize,
		GameplayEventMaxRetries:        defaultGameplayEventMaxRetries,
		RegionProjectionTTL:            defaultRegionProjectionTTL,
		RegionProjectionHeartbeat:      defaultRegionProjectionHeartbeat,
		RegionProjectionQueueSize:      defaultRegionProjectionQueueSize,
		RegionProjectionInterestRadius: defaultRegionProjectionInterestRadius,
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
	if config.SessionLeaseDuration <= 0 {
		config.SessionLeaseDuration = defaults.SessionLeaseDuration
	}
	if config.SessionLeaseRenewInterval <= 0 || config.SessionLeaseRenewInterval >= config.SessionLeaseDuration {
		config.SessionLeaseRenewInterval = config.SessionLeaseDuration / 3
		if config.SessionLeaseRenewInterval <= 0 {
			config.SessionLeaseRenewInterval = time.Millisecond
		}
	}
	if config.SessionAttachTokenTTL <= 0 {
		config.SessionAttachTokenTTL = defaults.SessionAttachTokenTTL
	}
	if config.GameplayEventPollInterval <= 0 {
		config.GameplayEventPollInterval = defaults.GameplayEventPollInterval
	}
	if config.GameplayEventClaimLease <= 0 {
		config.GameplayEventClaimLease = defaults.GameplayEventClaimLease
	}
	if config.GameplayEventRetryDelay <= 0 {
		config.GameplayEventRetryDelay = defaults.GameplayEventRetryDelay
	}
	if config.GameplayEventRetention <= 0 {
		config.GameplayEventRetention = defaults.GameplayEventRetention
	}
	if config.GameplayEventCleanupInterval <= 0 {
		config.GameplayEventCleanupInterval = defaults.GameplayEventCleanupInterval
	}
	if config.GameplayEventBatchSize <= 0 {
		config.GameplayEventBatchSize = defaults.GameplayEventBatchSize
	}
	if config.GameplayEventMaxRetries <= 0 {
		config.GameplayEventMaxRetries = defaults.GameplayEventMaxRetries
	}
	if config.RegionProjectionTTL <= 0 {
		config.RegionProjectionTTL = defaults.RegionProjectionTTL
	}
	if config.RegionProjectionHeartbeat <= 0 || config.RegionProjectionHeartbeat >= config.RegionProjectionTTL {
		config.RegionProjectionHeartbeat = config.RegionProjectionTTL / 3
		if config.RegionProjectionHeartbeat <= 0 {
			config.RegionProjectionHeartbeat = time.Millisecond
		}
	}
	if config.RegionProjectionQueueSize <= 0 {
		config.RegionProjectionQueueSize = defaults.RegionProjectionQueueSize
	}
	if config.RegionProjectionInterestRadius <= 0 {
		config.RegionProjectionInterestRadius = defaults.RegionProjectionInterestRadius
	}
	config.ServerInstanceID = strings.TrimSpace(config.ServerInstanceID)
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
