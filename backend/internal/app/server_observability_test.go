package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type failingListCharactersRepo struct {
	CharacterRepository
}

func (repo failingListCharactersRepo) ListByAccountID(ctx context.Context, accountID string) ([]Character, error) {
	return nil, errors.New("forced character listing failure")
}

func TestMetricsEndpointTracksHTTPRequestsAndErrors(t *testing.T) {
	server := NewServer(":0", "", newMemoryStore())
	httpServer := httptest.NewServer(server.handler())
	defer httpServer.Close()

	response, err := httpServer.Client().Get(httpServer.URL + "/healthz")
	if err != nil {
		t.Fatalf("healthz request error = %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected healthz 200, got %d", response.StatusCode)
	}
	_ = response.Body.Close()

	request, err := http.NewRequest(http.MethodGet, httpServer.URL+"/healthz", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	request.Header.Set("Origin", "http://localhost:5173")
	response, err = httpServer.Client().Do(request)
	if err != nil {
		t.Fatalf("cross-origin healthz request error = %v", err)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("expected healthz 403 for unauthorized origin, got %d", response.StatusCode)
	}
	_ = response.Body.Close()

	metricsBody := fetchMetricsBody(t, httpServer.Client(), httpServer.URL)
	assertMetricLine(t, metricsBody, `l2bg_http_requests_total{method="GET",path="/healthz",status_code="200"} 1`)
	assertMetricLine(t, metricsBody, `l2bg_http_requests_total{method="GET",path="/healthz",status_code="403"} 1`)
	assertMetricLine(t, metricsBody, `l2bg_http_errors_total{method="GET",path="/healthz",status_code="403"} 1`)
	assertMetricLine(t, metricsBody, `l2bg_http_request_duration_seconds_count{method="GET",path="/healthz",status_code="200"} 1`)
}

func TestMetricsEndpointTracksGameplayAttachAndCommandSignals(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "ws://localhost:5173/api/v1/gameplay/ws", store)
	httpServer := httptest.NewServer(server.handler())
	defer httpServer.Close()

	character := &Character{
		ID:           "char_metrics_ws",
		AccountID:    "acc_metrics_ws",
		Name:         "Metrics Hero",
		Race:         "Human",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
		IsEnterable:  true,
	}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}
	session := &Session{
		ID:              "sess_metrics_ws",
		AccountID:       character.AccountID,
		CharacterID:     character.ID,
		AttachToken:     "attach_metrics_ws",
		AttachExpiresAt: time.Now().Add(5 * time.Minute),
		Status:          sessionStatusPendingAttach,
	}
	if err := store.GameplaySessions.Create(context.Background(), session); err != nil {
		t.Fatalf("GameplaySessions.Create() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/v1/gameplay/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin": []string{"http://localhost:5173"},
		},
	})
	if err != nil {
		t.Fatalf("websocket.Dial() error = %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test complete")

	attachPayload, err := json.Marshal(map[string]any{
		"kind":         "attach_session",
		"session_id":   session.ID,
		"attach_token": session.AttachToken,
	})
	if err != nil {
		t.Fatalf("json.Marshal(attach) error = %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, attachPayload); err != nil {
		t.Fatalf("attach conn.Write() error = %v", err)
	}

	message := readWSJSON(t, ctx, conn)
	if message["kind"] != "region_context" {
		t.Fatalf("expected region_context after attach, got %+v", message)
	}

	movePayload, err := json.Marshal(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_metrics_move",
		CommandSeq:      1,
		Type:            "move_intent",
		Payload:         []byte(`{"point":{"x":12,"z":4}}`),
	})
	if err != nil {
		t.Fatalf("json.Marshal(move) error = %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, movePayload); err != nil {
		t.Fatalf("move conn.Write() error = %v", err)
	}

	var ackMessage map[string]any
	var deltaMessage map[string]any
	for range 4 {
		message := readWSJSON(t, ctx, conn)
		switch message["kind"] {
		case "ack":
			ackMessage = message
		case "delta":
			if commandID, _ := message["applies_to_command_id"].(string); commandID == "cmd_metrics_move" {
				deltaMessage = message
			}
		}
		if ackMessage != nil && deltaMessage != nil {
			break
		}
	}
	if ackMessage == nil {
		t.Fatal("expected ack after move_intent")
	}
	if deltaMessage == nil {
		t.Fatal("expected authoritative move delta after move_intent")
	}

	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"broken_json"`)); err != nil {
		t.Fatalf("invalid envelope conn.Write() error = %v", err)
	}
	rejectMessage := readWSJSON(t, ctx, conn)
	if rejectMessage["kind"] != "reject" {
		t.Fatalf("expected reject for invalid envelope, got %+v", rejectMessage)
	}

	waitForMetricLine(t, httpServer.Client(), httpServer.URL, `l2bg_gameplay_commands_total{command_type="move_intent",result="applied"} 1`)
	waitForMetricLine(t, httpServer.Client(), httpServer.URL, `l2bg_gameplay_command_duration_seconds_count{command_type="move_intent",result="applied"} 1`)

	metricsBody := fetchMetricsBody(t, httpServer.Client(), httpServer.URL)
	assertMetricLine(t, metricsBody, `l2bg_websocket_connections_active 1`)
	assertMetricLine(t, metricsBody, `l2bg_attached_sessions_active 1`)
	assertMetricLine(t, metricsBody, `l2bg_region_occupancy{region_id="dawn_plaza"} 1`)
	assertMetricLine(t, metricsBody, `l2bg_ws_attach_attempts_total{reason_code="none",result="accepted"} 1`)
	assertMetricLine(t, metricsBody, `l2bg_gameplay_outbound_messages_total{kind="region_context"} 1`)
	assertMetricLine(t, metricsBody, `l2bg_gameplay_outbound_messages_total{kind="ack"} 1`)
	assertMetricLine(t, metricsBody, `l2bg_gameplay_outbound_messages_total{kind="delta"} 1`)
	assertMetricLine(t, metricsBody, `l2bg_gameplay_outbound_messages_total{kind="reject"} 1`)
	assertMetricLine(t, metricsBody, `l2bg_gameplay_rejects_total{reason_code="protocol.invalid_envelope"} 1`)

	if err := conn.Close(websocket.StatusNormalClosure, "metrics disconnect"); err != nil {
		t.Fatalf("conn.Close() error = %v", err)
	}

	waitForMetricLine(t, httpServer.Client(), httpServer.URL, `l2bg_websocket_connections_active 0`)
	waitForMetricLine(t, httpServer.Client(), httpServer.URL, `l2bg_attached_sessions_active 0`)
	waitForMetricLine(t, httpServer.Client(), httpServer.URL, `l2bg_region_occupancy{region_id="dawn_plaza"} 0`)
}

func TestMetricsEndpointCountsPersistenceErrors(t *testing.T) {
	store := newMemoryStore()
	account := &Account{
		ID:          "acc_metrics_db",
		Login:       "metrics.db@test",
		DisplayName: "Metrics DB",
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
		Token:     "access_metrics_db",
		AccountID: account.ID,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("AccountSessions.Create() error = %v", err)
	}
	store.Characters = failingListCharactersRepo{CharacterRepository: store.Characters}

	server := NewServer(":0", "", store)
	httpServer := httptest.NewServer(server.handler())
	defer httpServer.Close()

	request, err := http.NewRequest(http.MethodGet, httpServer.URL+"/v1/characters", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	request.Header.Set("Authorization", "Bearer access_metrics_db")
	response, err := httpServer.Client().Do(request)
	if err != nil {
		t.Fatalf("characters request error = %v", err)
	}
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected characters request to fail with 500, got %d", response.StatusCode)
	}
	_ = response.Body.Close()

	metricsBody := fetchMetricsBody(t, httpServer.Client(), httpServer.URL)
	assertMetricLine(t, metricsBody, `l2bg_db_errors_total{operation="characters.list_by_account",store_mode="memory"} 1`)
}

func fetchMetricsBody(t *testing.T, client *http.Client, baseURL string) string {
	t.Helper()

	response, err := client.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatalf("metrics request error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d", response.StatusCode)
	}

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("io.ReadAll(metrics) error = %v", err)
	}
	return string(payload)
}

func assertMetricLine(t *testing.T, metricsBody string, expectedLine string) {
	t.Helper()
	if !strings.Contains(metricsBody, expectedLine) {
		t.Fatalf("expected metrics to contain %q\nmetrics:\n%s", expectedLine, metricsBody)
	}
}

func waitForMetricLine(t *testing.T, client *http.Client, baseURL string, expectedLine string) {
	t.Helper()

	deadline := time.Now().Add(750 * time.Millisecond)
	for time.Now().Before(deadline) {
		metricsBody := fetchMetricsBody(t, client, baseURL)
		if strings.Contains(metricsBody, expectedLine) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	metricsBody := fetchMetricsBody(t, client, baseURL)
	t.Fatalf("expected metrics to contain %q after waiting\nmetrics:\n%s", expectedLine, metricsBody)
}

func readWSJSON(t *testing.T, ctx context.Context, conn *websocket.Conn) map[string]any {
	t.Helper()

	_, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("conn.Read() error = %v", err)
	}
	var message map[string]any
	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return message
}
