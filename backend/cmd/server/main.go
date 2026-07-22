package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"l2-board-game-backend/internal/app"
)

func main() {
	addr := os.Getenv("L2BG_BACKEND_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	publicWSURL := os.Getenv("L2BG_PUBLIC_WS_URL")
	databaseURL := os.Getenv("L2BG_DATABASE_URL")
	allowedOrigins := splitEnvList(os.Getenv("L2BG_ALLOWED_ORIGINS"))
	accessTokenTTL := 2 * time.Hour
	if rawTTL := strings.TrimSpace(os.Getenv("L2BG_ACCESS_TOKEN_TTL")); rawTTL != "" {
		parsedTTL, err := time.ParseDuration(rawTTL)
		if err != nil {
			log.Fatalf("invalid L2BG_ACCESS_TOKEN_TTL: %v", err)
		}
		accessTokenTTL = parsedTTL
	}
	authRateLimit := rateLimitConfigFromEnv("L2BG_AUTH_RATE_LIMIT")
	attachRateLimit := rateLimitConfigFromEnv("L2BG_ATTACH_RATE_LIMIT")
	socialAuth := app.SocialAuthConfig{
		Google: app.SocialProviderConfig{
			ClientID:     strings.TrimSpace(os.Getenv("L2BG_AUTH_SOCIAL_GOOGLE_CLIENT_ID")),
			ClientSecret: strings.TrimSpace(os.Getenv("L2BG_AUTH_SOCIAL_GOOGLE_CLIENT_SECRET")),
			RedirectURL:  strings.TrimSpace(os.Getenv("L2BG_AUTH_SOCIAL_GOOGLE_REDIRECT_URL")),
		},
		Facebook: app.SocialProviderConfig{
			ClientID:     strings.TrimSpace(os.Getenv("L2BG_AUTH_SOCIAL_FACEBOOK_CLIENT_ID")),
			ClientSecret: strings.TrimSpace(os.Getenv("L2BG_AUTH_SOCIAL_FACEBOOK_CLIENT_SECRET")),
			RedirectURL:  strings.TrimSpace(os.Getenv("L2BG_AUTH_SOCIAL_FACEBOOK_REDIRECT_URL")),
		},
	}
	internalAuditEnabled := boolEnv("L2BG_INTERNAL_AUDIT_ENABLED")
	internalAuditToken := strings.TrimSpace(os.Getenv("L2BG_INTERNAL_AUDIT_TOKEN"))
	if internalAuditEnabled && internalAuditToken == "" {
		log.Fatal("L2BG_INTERNAL_AUDIT_ENABLED requires L2BG_INTERNAL_AUDIT_TOKEN")
	}
	serverInstanceID := strings.TrimSpace(os.Getenv("L2BG_SERVER_INSTANCE_ID"))
	if serverInstanceID == "" {
		hostname, err := os.Hostname()
		if err != nil || strings.TrimSpace(hostname) == "" {
			log.Fatal("L2BG_SERVER_INSTANCE_ID is required when hostname is unavailable")
		}
		serverInstanceID = strings.TrimSpace(hostname)
	}
	sessionLeaseDuration := durationEnv("L2BG_SESSION_LEASE_DURATION", 30*time.Second)
	sessionLeaseRenewInterval := durationEnv("L2BG_SESSION_LEASE_RENEW_INTERVAL", 10*time.Second)
	if sessionLeaseRenewInterval >= sessionLeaseDuration {
		log.Fatal("L2BG_SESSION_LEASE_RENEW_INTERVAL must be shorter than L2BG_SESSION_LEASE_DURATION")
	}
	sessionAttachTokenTTL := durationEnv("L2BG_SESSION_ATTACH_TOKEN_TTL", 5*time.Minute)
	gameplayEventPollInterval := durationEnv("L2BG_GAMEPLAY_EVENT_POLL_INTERVAL", 250*time.Millisecond)
	gameplayEventClaimLease := durationEnv("L2BG_GAMEPLAY_EVENT_CLAIM_LEASE", 5*time.Second)
	gameplayEventRetryDelay := durationEnv("L2BG_GAMEPLAY_EVENT_RETRY_DELAY", 500*time.Millisecond)
	gameplayEventRetention := durationEnv("L2BG_GAMEPLAY_EVENT_RETENTION", 24*time.Hour)
	gameplayEventCleanupInterval := durationEnv("L2BG_GAMEPLAY_EVENT_CLEANUP_INTERVAL", 10*time.Minute)
	gameplayEventBatchSize := positiveIntEnv("L2BG_GAMEPLAY_EVENT_BATCH_SIZE", 32)
	gameplayEventMaxRetries := positiveIntEnv("L2BG_GAMEPLAY_EVENT_MAX_RETRIES", 5)
	regionProjectionTTL := durationEnv("L2BG_REGION_PROJECTION_TTL", 6*time.Second)
	regionProjectionHeartbeat := durationEnv("L2BG_REGION_PROJECTION_HEARTBEAT", 2*time.Second)
	if regionProjectionHeartbeat >= regionProjectionTTL {
		log.Fatal("L2BG_REGION_PROJECTION_HEARTBEAT must be shorter than L2BG_REGION_PROJECTION_TTL")
	}
	regionProjectionQueueSize := positiveIntEnv("L2BG_REGION_PROJECTION_QUEUE_SIZE", 256)
	store, err := app.NewStore(databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	log.Printf("backend persistence mode: %s", store.Mode)
	if err := store.SanitizeGameplaySessionLifecycle(context.Background(), time.Now()); err != nil {
		log.Fatal(err)
	}

	server := app.NewServerWithConfig(addr, publicWSURL, store, app.ServerConfig{
		AllowedOrigins:               allowedOrigins,
		AccessTokenTTL:               accessTokenTTL,
		AuthRateLimit:                authRateLimit,
		AttachRateLimit:              attachRateLimit,
		SocialAuth:                   socialAuth,
		InternalAuditEnabled:         internalAuditEnabled,
		InternalAuditToken:           internalAuditToken,
		ServerInstanceID:             serverInstanceID,
		SessionLeaseDuration:         sessionLeaseDuration,
		SessionLeaseRenewInterval:    sessionLeaseRenewInterval,
		SessionAttachTokenTTL:        sessionAttachTokenTTL,
		GameplayEventPollInterval:    gameplayEventPollInterval,
		GameplayEventClaimLease:      gameplayEventClaimLease,
		GameplayEventRetryDelay:      gameplayEventRetryDelay,
		GameplayEventRetention:       gameplayEventRetention,
		GameplayEventCleanupInterval: gameplayEventCleanupInterval,
		GameplayEventBatchSize:       gameplayEventBatchSize,
		GameplayEventMaxRetries:      gameplayEventMaxRetries,
		RegionProjectionTTL:          regionProjectionTTL,
		RegionProjectionHeartbeat:    regionProjectionHeartbeat,
		RegionProjectionQueueSize:    regionProjectionQueueSize,
	})
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}

func positiveIntEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		log.Fatalf("invalid %s: %q", key, raw)
	}
	return value
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		log.Fatalf("invalid %s: %q", key, raw)
	}
	return value
}

func splitEnvList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		values = append(values, trimmed)
	}
	return values
}

func rateLimitConfigFromEnv(prefix string) app.RateLimitConfig {
	config := app.RateLimitConfig{}
	if strings.TrimSpace(prefix) == "" {
		return config
	}

	if rawAttempts := strings.TrimSpace(os.Getenv(prefix + "_MAX_ATTEMPTS")); rawAttempts != "" {
		attempts, err := strconv.Atoi(rawAttempts)
		if err != nil || attempts <= 0 {
			log.Fatalf("invalid %s_MAX_ATTEMPTS: %q", prefix, rawAttempts)
		}
		config.MaxAttempts = attempts
	}
	if rawWindow := strings.TrimSpace(os.Getenv(prefix + "_WINDOW")); rawWindow != "" {
		window, err := time.ParseDuration(rawWindow)
		if err != nil || window <= 0 {
			log.Fatalf("invalid %s_WINDOW: %q", prefix, rawWindow)
		}
		config.Window = window
	}

	return config
}

func boolEnv(key string) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
