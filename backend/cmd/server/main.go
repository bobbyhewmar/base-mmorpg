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
	internalAuditEnabled := boolEnv("L2BG_INTERNAL_AUDIT_ENABLED")
	internalAuditToken := strings.TrimSpace(os.Getenv("L2BG_INTERNAL_AUDIT_TOKEN"))
	if internalAuditEnabled && internalAuditToken == "" {
		log.Fatal("L2BG_INTERNAL_AUDIT_ENABLED requires L2BG_INTERNAL_AUDIT_TOKEN")
	}
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
		AllowedOrigins:       allowedOrigins,
		AccessTokenTTL:       accessTokenTTL,
		AuthRateLimit:        authRateLimit,
		AttachRateLimit:      attachRateLimit,
		InternalAuditEnabled: internalAuditEnabled,
		InternalAuditToken:   internalAuditToken,
	})
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
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
