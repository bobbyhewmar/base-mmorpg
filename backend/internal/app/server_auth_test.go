package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestLoginMigratesLegacySHA256CredentialAndCreatesAccountSession(t *testing.T) {
	store := newMemoryStore()
	account := &Account{
		ID:          "acc_auth_migrate",
		Login:       "migrate@test",
		DisplayName: "Migrate",
		State:       accountStateActive,
	}
	credential := &CredentialRecord{
		AccountID:         account.ID,
		PasswordHash:      hashPasswordSHA256("hunter123"),
		PasswordAlgorithm: passwordAlgorithmSHA256,
	}
	if err := store.CreateAccountWithCredential(context.Background(), account, credential); err != nil {
		t.Fatalf("CreateAccountWithCredential() error = %v", err)
	}

	server := NewServer(":0", "", store)
	httpServer := httptest.NewServer(server.withCORS(server.mux))
	defer httpServer.Close()

	response := postJSON(t, httpServer.Client(), httpServer.URL+"/v1/auth/login", map[string]any{
		"login":    account.Login,
		"password": "hunter123",
	}, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", response.StatusCode)
	}
	payload := decodeBody[map[string]any](t, response)

	accessToken, ok := payload["access_token"].(string)
	if !ok || accessToken == "" {
		t.Fatalf("missing access_token in login payload: %+v", payload)
	}

	persistedCredential, err := store.Credentials.GetByAccountID(context.Background(), account.ID)
	if err != nil {
		t.Fatalf("Credentials.GetByAccountID() error = %v", err)
	}
	if persistedCredential.PasswordAlgorithm != passwordAlgorithmBcryptV1 {
		t.Fatalf("expected migrated password algorithm %q, got %+v", passwordAlgorithmBcryptV1, persistedCredential)
	}
	if persistedCredential.PasswordHash == hashPasswordSHA256("hunter123") {
		t.Fatalf("expected migrated credential hash to differ from legacy sha256 hash")
	}

	accountSession, err := store.AccountSessions.GetActiveByToken(context.Background(), accessToken, time.Now())
	if err != nil {
		t.Fatalf("AccountSessions.GetActiveByToken() error = %v", err)
	}
	if accountSession.AccountID != account.ID {
		t.Fatalf("unexpected account session = %+v", accountSession)
	}
}

func TestExpiredAccountSessionRejectsAuthenticatedHTTPAccess(t *testing.T) {
	store := newMemoryStore()
	account := &Account{
		ID:          "acc_auth_expired",
		Login:       "expired@test",
		DisplayName: "Expired",
		State:       accountStateActive,
	}
	credential, err := newCredentialRecord(account.ID, "hunter123")
	if err != nil {
		t.Fatalf("newCredentialRecord() error = %v", err)
	}
	if err := store.CreateAccountWithCredential(context.Background(), account, credential); err != nil {
		t.Fatalf("CreateAccountWithCredential() error = %v", err)
	}
	if err := store.AccountSessions.Create(context.Background(), &AccountSession{
		Token:     "access_expired",
		AccountID: account.ID,
		ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("AccountSessions.Create() error = %v", err)
	}

	server := NewServer(":0", "", store)
	httpServer := httptest.NewServer(server.withCORS(server.mux))
	defer httpServer.Close()

	request, err := http.NewRequest(http.MethodGet, httpServer.URL+"/v1/characters", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	request.Header.Set("Authorization", "Bearer access_expired")
	response, err := httpServer.Client().Do(request)
	if err != nil {
		t.Fatalf("client.Do() error = %v", err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired access session, got %d", response.StatusCode)
	}
	apiErr := decodeBody[apiError](t, response)
	if apiErr.ReasonCode != "auth.not_authenticated" {
		t.Fatalf("unexpected error payload = %+v", apiErr)
	}
}

func TestRegisterRateLimitRejectsExcessAttempts(t *testing.T) {
	server := NewServerWithConfig(":0", "", newMemoryStore(), ServerConfig{
		AuthRateLimit: RateLimitConfig{
			MaxAttempts: 1,
			Window:      time.Hour,
		},
	})
	httpServer := httptest.NewServer(server.withCORS(server.mux))
	defer httpServer.Close()

	firstResponse := postJSON(t, httpServer.Client(), httpServer.URL+"/v1/auth/register", map[string]any{
		"login":        "first@test",
		"email":        "first@example.com",
		"password":     "hunter123",
		"display_name": "First",
	}, "")
	if firstResponse.StatusCode != http.StatusCreated {
		t.Fatalf("first register status = %d", firstResponse.StatusCode)
	}

	secondResponse := postJSON(t, httpServer.Client(), httpServer.URL+"/v1/auth/register", map[string]any{
		"login":        "second@test",
		"email":        "second@example.com",
		"password":     "hunter123",
		"display_name": "Second",
	}, "")
	if secondResponse.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected second register to be rate limited, got %d", secondResponse.StatusCode)
	}
	apiErr := decodeBody[apiError](t, secondResponse)
	if apiErr.ReasonCode != "auth.rate_limited" {
		t.Fatalf("unexpected error payload = %+v", apiErr)
	}
}

func TestRegisterRejectsInvalidEmail(t *testing.T) {
	server := NewServer(":0", "", newMemoryStore())
	httpServer := httptest.NewServer(server.withCORS(server.mux))
	defer httpServer.Close()

	response := postJSON(t, httpServer.Client(), httpServer.URL+"/v1/auth/register", map[string]any{
		"login":        "invalid.email@test",
		"email":        "not-an-email",
		"password":     "hunter123",
		"display_name": "Invalid Email",
	}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid email to be rejected, got %d", response.StatusCode)
	}
	apiErr := decodeBody[apiError](t, response)
	if apiErr.ReasonCode != "auth.invalid_email" {
		t.Fatalf("unexpected error payload = %+v", apiErr)
	}
}

func TestRegisterRejectsUnavailableEmail(t *testing.T) {
	server := NewServer(":0", "", newMemoryStore())
	httpServer := httptest.NewServer(server.withCORS(server.mux))
	defer httpServer.Close()

	firstResponse := postJSON(t, httpServer.Client(), httpServer.URL+"/v1/auth/register", map[string]any{
		"login":        "first.email@test",
		"email":        "shared@example.com",
		"password":     "hunter123",
		"display_name": "First Email",
	}, "")
	if firstResponse.StatusCode != http.StatusCreated {
		t.Fatalf("first register status = %d", firstResponse.StatusCode)
	}

	secondResponse := postJSON(t, httpServer.Client(), httpServer.URL+"/v1/auth/register", map[string]any{
		"login":        "second.email@test",
		"email":        "shared@example.com",
		"password":     "hunter123",
		"display_name": "Second Email",
	}, "")
	if secondResponse.StatusCode != http.StatusConflict {
		t.Fatalf("expected duplicate email to be rejected, got %d", secondResponse.StatusCode)
	}
	apiErr := decodeBody[apiError](t, secondResponse)
	if apiErr.ReasonCode != "auth.email_unavailable" {
		t.Fatalf("unexpected error payload = %+v", apiErr)
	}
}

func TestSocialBeginRejectsUnconfiguredProvider(t *testing.T) {
	server := NewServer(":0", "", newMemoryStore())
	httpServer := httptest.NewServer(server.withCORS(server.mux))
	defer httpServer.Close()

	response := postJSON(t, httpServer.Client(), httpServer.URL+"/v1/auth/social/google/begin", map[string]any{}, "")
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected unconfigured social provider to be unavailable, got %d", response.StatusCode)
	}
	apiErr := decodeBody[apiError](t, response)
	if apiErr.ReasonCode != "auth.social_not_configured" {
		t.Fatalf("unexpected error payload = %+v", apiErr)
	}
}

func TestSocialBeginReturnsAuthorizationURLWhenConfigured(t *testing.T) {
	server := NewServerWithConfig(":0", "", newMemoryStore(), ServerConfig{
		SocialAuth: SocialAuthConfig{
			Google: SocialProviderConfig{
				ClientID:    "google-client-id",
				RedirectURL: "https://frontend.example.test/auth/social/google/callback",
			},
		},
	})
	httpServer := httptest.NewServer(server.withCORS(server.mux))
	defer httpServer.Close()

	response := postJSON(t, httpServer.Client(), httpServer.URL+"/v1/auth/social/google/begin", map[string]any{}, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected configured social provider to return authorization URL, got %d", response.StatusCode)
	}
	payload := decodeBody[map[string]any](t, response)
	if payload["provider"] != "google" {
		t.Fatalf("unexpected provider payload = %+v", payload)
	}
	authURL, ok := payload["authorization_url"].(string)
	if !ok || !strings.Contains(authURL, "accounts.google.com") || !strings.Contains(authURL, "client_id=google-client-id") {
		t.Fatalf("unexpected authorization_url = %+v", payload)
	}
}

func TestGameplayWSRateLimitRejectsExcessiveAttachAttempts(t *testing.T) {
	server := NewServerWithConfig(":0", "", newMemoryStore(), ServerConfig{
		AttachRateLimit: RateLimitConfig{
			MaxAttempts: 1,
			Window:      time.Hour,
		},
	})
	httpServer := httptest.NewServer(server.withCORS(server.mux))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/v1/gameplay/ws"
	directOrigin := "http://" + strings.TrimPrefix(httpServer.URL, "http://")
	firstConn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin": []string{directOrigin},
		},
	})
	if err != nil {
		t.Fatalf("first websocket.Dial() error = %v", err)
	}
	_ = firstConn.Close(websocket.StatusNormalClosure, "rate-limit test")

	secondConn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin": []string{directOrigin},
		},
	})
	if err == nil {
		_ = secondConn.Close(websocket.StatusPolicyViolation, "expected rate-limit rejection")
		t.Fatal("expected second websocket dial to be rate limited")
	}
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	server := NewServerWithConfig(":0", "", newMemoryStore(), ServerConfig{
		AllowedOrigins: []string{"http://localhost:5173"},
	})
	httpServer := httptest.NewServer(server.withCORS(server.mux))
	defer httpServer.Close()

	request, err := http.NewRequest(http.MethodOptions, httpServer.URL+"/v1/auth/login", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	request.Header.Set("Origin", "http://localhost:5173")

	response, err := httpServer.Client().Do(request)
	if err != nil {
		t.Fatalf("client.Do() error = %v", err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for allowed preflight, got %d", response.StatusCode)
	}
	if response.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("unexpected allow origin header = %q", response.Header.Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSRejectsUnauthorizedOrigin(t *testing.T) {
	server := NewServerWithConfig(":0", "", newMemoryStore(), ServerConfig{
		AllowedOrigins: []string{"http://localhost:5173"},
	})
	httpServer := httptest.NewServer(server.withCORS(server.mux))
	defer httpServer.Close()

	request, err := http.NewRequest(http.MethodGet, httpServer.URL+"/healthz", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	request.Header.Set("Origin", "http://malicious.test")

	response, err := httpServer.Client().Do(request)
	if err != nil {
		t.Fatalf("client.Do() error = %v", err)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for unauthorized origin, got %d", response.StatusCode)
	}
}

func TestCORSWithoutConfigurationFailsClosedForCrossOriginHTTP(t *testing.T) {
	server := NewServer(":0", "", newMemoryStore())
	httpServer := httptest.NewServer(server.withCORS(server.mux))
	defer httpServer.Close()

	request, err := http.NewRequest(http.MethodGet, httpServer.URL+"/healthz", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	request.Header.Set("Origin", "http://localhost:5173")

	response, err := httpServer.Client().Do(request)
	if err != nil {
		t.Fatalf("client.Do() error = %v", err)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 when CORS is not configured, got %d", response.StatusCode)
	}
}
