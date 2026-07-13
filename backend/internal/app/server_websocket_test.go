package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestGameplayWSAcceptsConfiguredFrontendOriginAndDeliversRegionContext(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "ws://localhost:5173/api/v1/gameplay/ws", store)
	httpServer := httptest.NewServer(server.withCORS(server.mux))
	defer httpServer.Close()

	character := &Character{
		ID:           "char_ws_origin",
		AccountID:    "acc_ws_origin",
		Name:         "WS Origin Hero",
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
		ID:              "sess_ws_origin",
		AccountID:       character.AccountID,
		CharacterID:     character.ID,
		AttachToken:     "attach_ws_origin",
		AttachExpiresAt: time.Now().Add(5 * time.Minute),
		Status:          sessionStatusPendingAttach,
	}
	if err := store.GameplaySessions.Create(context.Background(), session); err != nil {
		t.Fatalf("GameplaySessions.Create() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
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
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, attachPayload); err != nil {
		t.Fatalf("conn.Write() error = %v", err)
	}

	_, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("conn.Read() error = %v", err)
	}
	var message map[string]any
	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if message["kind"] != "region_context" {
		t.Fatalf("expected region_context, got %+v", message)
	}
}

func TestGameplayWSAcceptsDirectOriginWithoutPublicWSURL(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)
	httpServer := httptest.NewServer(server.withCORS(server.mux))
	defer httpServer.Close()

	character := &Character{
		ID:           "char_ws_direct",
		AccountID:    "acc_ws_direct",
		Name:         "WS Direct Hero",
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
		ID:              "sess_ws_direct",
		AccountID:       character.AccountID,
		CharacterID:     character.ID,
		AttachToken:     "attach_ws_direct",
		AttachExpiresAt: time.Now().Add(5 * time.Minute),
		Status:          sessionStatusPendingAttach,
	}
	if err := store.GameplaySessions.Create(context.Background(), session); err != nil {
		t.Fatalf("GameplaySessions.Create() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/v1/gameplay/ws"
	directOrigin := "http://" + strings.TrimPrefix(httpServer.URL, "http://")
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin": []string{directOrigin},
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
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, attachPayload); err != nil {
		t.Fatalf("conn.Write() error = %v", err)
	}

	_, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("conn.Read() error = %v", err)
	}
	var message map[string]any
	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if message["kind"] != "region_context" {
		t.Fatalf("expected region_context, got %+v", message)
	}
}

func TestGameplayWSRejectsIncoherentConfiguredOrigin(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "ws://example.test/v1/gameplay/ws", store)
	httpServer := httptest.NewServer(server.withCORS(server.mux))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/v1/gameplay/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin": []string{"http://localhost:5173"},
		},
	})
	if err == nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "expected handshake rejection")
		t.Fatal("expected websocket handshake to fail for incoherent configured origin")
	}
}

func TestGameplayWSOriginPatternsRejectMalformedPublicWSURL(t *testing.T) {
	server := NewServer(":0", "://bad-url", newMemoryStore())
	request := httptest.NewRequest(http.MethodGet, "http://backend.test/v1/gameplay/ws", nil)
	request.Host = "backend.test"

	patterns, err := server.gameplayWSOriginPatterns(request)
	if err == nil {
		t.Fatalf("expected invalid publicWSURL error, got patterns=%v", patterns)
	}
	if !strings.Contains(err.Error(), "invalid public websocket URL") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGameplayWSOriginPatternsRejectMissingDirectHost(t *testing.T) {
	server := NewServer(":0", "", newMemoryStore())

	patterns, err := server.gameplayWSOriginPatterns(&http.Request{})
	if err == nil {
		t.Fatalf("expected missing host error, got patterns=%v", patterns)
	}
	if !strings.Contains(err.Error(), "missing request host") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGameplayWSClosesAttachedSessionOnDisconnect(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "ws://localhost:5173/api/v1/gameplay/ws", store)
	httpServer := httptest.NewServer(server.withCORS(server.mux))
	defer httpServer.Close()

	character := &Character{
		ID:           "char_ws_close",
		AccountID:    "acc_ws_close",
		Name:         "WS Close Hero",
		Race:         "Human",
		BaseClass:    "Fighter",
		Sex:          "Female",
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
		ID:              "sess_ws_close",
		AccountID:       character.AccountID,
		CharacterID:     character.ID,
		AttachToken:     "attach_ws_close",
		AttachExpiresAt: time.Now().Add(5 * time.Minute),
		Status:          sessionStatusPendingAttach,
	}
	if err := store.GameplaySessions.Create(context.Background(), session); err != nil {
		t.Fatalf("GameplaySessions.Create() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
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

	attachPayload, err := json.Marshal(map[string]any{
		"kind":         "attach_session",
		"session_id":   session.ID,
		"attach_token": session.AttachToken,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, attachPayload); err != nil {
		t.Fatalf("conn.Write() error = %v", err)
	}
	if _, _, err := conn.Read(ctx); err != nil {
		t.Fatalf("conn.Read() error = %v", err)
	}
	if err := conn.Close(websocket.StatusNormalClosure, "disconnect after attach"); err != nil {
		t.Fatalf("conn.Close() error = %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		persistedSession, err := store.GameplaySessions.GetByID(context.Background(), session.ID)
		if err != nil {
			t.Fatalf("GameplaySessions.GetByID() error = %v", err)
		}
		if persistedSession.Status == sessionStatusClosed {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	persistedSession, err := store.GameplaySessions.GetByID(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GameplaySessions.GetByID() error = %v", err)
	}
	t.Fatalf("expected attached session to become closed after disconnect, got %+v", persistedSession)
}

func TestServerFanOutsLootAppearToOtherAttachedSessionsInSameRegion(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	sourceCharacter := &Character{ID: "char_fanout_source", AccountID: "acc_source", Name: "Source", LastRegionID: "dawn_plaza"}
	observerCharacter := &Character{ID: "char_fanout_observer", AccountID: "acc_observer", Name: "Observer", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), sourceCharacter, initialCharacterItemSeed(sourceCharacter)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(source) error = %v", err)
	}
	if err := store.CreateCharacterWithItemSeed(context.Background(), observerCharacter, initialCharacterItemSeed(observerCharacter)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(observer) error = %v", err)
	}

	sourceRuntime := newAttachedRuntime("sess_fanout_source", sourceCharacter)
	observerRuntime := newAttachedRuntime("sess_fanout_observer", observerCharacter)
	moveRuntimeNearMob(sourceRuntime, "mob_1")
	entity := sourceRuntime.knownEntities["mob_1"]
	entity.State["hp"] = 10
	entity.State["alive"] = true
	sourceRuntime.knownEntities["mob_1"] = entity

	observerMessages := make([]map[string]any, 0)
	server.registerAttachedSession("sess_fanout_source", sourceRuntime, func(map[string]any) bool { return true })
	server.registerAttachedSession("sess_fanout_observer", observerRuntime, func(message map[string]any) bool {
		observerMessages = append(observerMessages, message)
		return true
	})
	defer server.unregisterAttachedSession("sess_fanout_source")
	defer server.unregisterAttachedSession("sess_fanout_observer")

	outbound := sourceRuntime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})
	server.fanOutLootVisibility("sess_fanout_source", sourceRuntime, outbound)

	if len(observerMessages) != 1 || observerMessages[0]["kind"] != "entity_appear" {
		t.Fatalf("expected observer to receive loot entity_appear, got %+v", observerMessages)
	}
	entityPayload, ok := observerMessages[0]["entity"].(runtimeEntity)
	if !ok {
		t.Fatalf("expected runtimeEntity payload, got %+v", observerMessages[0]["entity"])
	}
	if entityPayload.EntityType != "loot" {
		t.Fatalf("expected fan-out entity to be loot, got %+v", entityPayload)
	}
	if _, exists := observerRuntime.knownEntities[entityPayload.EntityID]; !exists {
		t.Fatalf("expected observer runtime to track fan-out loot entity")
	}
}

func TestServerFanOutsLootDisappearAfterWinningPickup(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	winnerCharacter := &Character{ID: "char_fanout_winner", AccountID: "acc_winner", Name: "Winner", LastRegionID: "dawn_plaza"}
	observerCharacter := &Character{ID: "char_fanout_other", AccountID: "acc_other", Name: "Other", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), winnerCharacter, initialCharacterItemSeed(winnerCharacter)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(winner) error = %v", err)
	}
	if err := store.CreateCharacterWithItemSeed(context.Background(), observerCharacter, initialCharacterItemSeed(observerCharacter)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(observer) error = %v", err)
	}

	winnerRuntime := newAttachedRuntime("sess_fanout_winner", winnerCharacter)
	observerRuntime := newAttachedRuntime("sess_fanout_other", observerCharacter)
	sharedLoot := runtimeEntity{
		EntityID:   "loot_shared",
		EntityType: "loot",
		TemplateID: "duskgold",
		Position:   runtimePoint{X: -8, Z: 0},
		State:      map[string]any{"quantity": 4},
	}
	winnerRuntime.position = sharedLoot.Position
	observerRuntime.position = sharedLoot.Position
	winnerRuntime.knownEntities[sharedLoot.EntityID] = sharedLoot
	observerRuntime.knownEntities[sharedLoot.EntityID] = sharedLoot

	observerMessages := make([]map[string]any, 0)
	server.registerAttachedSession("sess_fanout_winner", winnerRuntime, func(map[string]any) bool { return true })
	server.registerAttachedSession("sess_fanout_other", observerRuntime, func(message map[string]any) bool {
		observerMessages = append(observerMessages, message)
		return true
	})
	defer server.unregisterAttachedSession("sess_fanout_winner")
	defer server.unregisterAttachedSession("sess_fanout_other")

	outbound := winnerRuntime.processLootPickup(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_pickup",
		CommandSeq:      1,
		Type:            "pick_up_loot",
		Payload:         []byte(`{"loot_id":"loot_shared"}`),
	})
	server.fanOutLootVisibility("sess_fanout_winner", winnerRuntime, outbound)

	if len(observerMessages) != 1 || observerMessages[0]["kind"] != "entity_disappear" {
		t.Fatalf("expected observer to receive loot entity_disappear, got %+v", observerMessages)
	}
	if observerMessages[0]["entity_id"] != "loot_shared" {
		t.Fatalf("expected observer disappear for loot_shared, got %+v", observerMessages[0])
	}
	if _, exists := observerRuntime.knownEntities["loot_shared"]; exists {
		t.Fatalf("expected observer runtime loot to be removed after winner pickup")
	}
}

func TestServerFanOutsPlayerPresenceLifecycleAcrossSameRegion(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	sourceCharacter := &Character{
		ID:           "char_presence_source",
		AccountID:    "acc_presence_source",
		Name:         "Source",
		BaseClass:    "Fighter",
		Sex:          "Female",
		Level:        3,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	}
	observerCharacter := &Character{
		ID:           "char_presence_observer",
		AccountID:    "acc_presence_observer",
		Name:         "Observer",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        4,
		LastRegionID: "dawn_plaza",
		PositionX:    -4,
		PositionZ:    2,
	}
	if err := store.CreateCharacterWithItemSeed(context.Background(), sourceCharacter, initialCharacterItemSeed(sourceCharacter)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(source) error = %v", err)
	}
	if err := store.CreateCharacterWithItemSeed(context.Background(), observerCharacter, initialCharacterItemSeed(observerCharacter)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(observer) error = %v", err)
	}

	sourceRuntime := newAttachedRuntime("sess_presence_source", sourceCharacter)
	observerRuntime := newAttachedRuntime("sess_presence_observer", observerCharacter)

	sourceMessages := make([]map[string]any, 0)
	observerMessages := make([]map[string]any, 0)

	server.stageAttachedSession("sess_presence_source", sourceRuntime, func(message map[string]any) bool {
		sourceMessages = append(sourceMessages, message)
		return true
	})
	defer server.unregisterAttachedSession("sess_presence_source")
	if messages := server.activateAttachedSession("sess_presence_source"); len(messages) != 0 {
		t.Fatalf("expected first attached session to have no peer sync messages, got %+v", messages)
	}

	server.stageAttachedSession("sess_presence_observer", observerRuntime, func(message map[string]any) bool {
		observerMessages = append(observerMessages, message)
		return true
	})
	if _, exists := observerRuntime.knownEntities[sourceCharacter.ID]; !exists {
		t.Fatalf("expected staged observer runtime to seed source player into known-set")
	}
	if messages := server.activateAttachedSession("sess_presence_observer"); len(messages) != 0 {
		t.Fatalf("expected staged observer runtime to rely on seeded region context, got %+v", messages)
	}
	if _, exists := sourceRuntime.knownEntities[observerCharacter.ID]; !exists {
		t.Fatalf("expected source runtime to track observer player after activation")
	}
	if len(observerMessages) != 0 {
		t.Fatalf("expected observer send channel to stay quiet during activation seed path, got %+v", observerMessages)
	}
	if len(sourceMessages) != 1 || sourceMessages[0]["kind"] != "entity_appear" {
		t.Fatalf("expected source to receive one player entity_appear, got %+v", sourceMessages)
	}
	appearEntity, ok := sourceMessages[0]["entity"].(runtimeEntity)
	if !ok {
		t.Fatalf("expected player appear payload to stay typed as runtimeEntity, got %+v", sourceMessages[0]["entity"])
	}
	if appearEntity.EntityID != observerCharacter.ID || appearEntity.EntityType != "player" {
		t.Fatalf("expected source to receive observer player presence, got %+v", appearEntity)
	}

	observerRuntime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_move_presence",
		CommandSeq:      1,
		Type:            "move_intent",
		Payload:         []byte(`{"point":{"x":12,"z":-3}}`),
	})
	observerRuntime.collectTickMessages(time.Now().Add(3 * time.Second))
	server.fanOutPlayerState("sess_presence_observer", observerRuntime)

	if len(sourceMessages) != 2 || sourceMessages[1]["kind"] != "delta" {
		t.Fatalf("expected source to receive movement delta for observer, got %+v", sourceMessages)
	}
	entityPatches, ok := sourceMessages[1]["entities"].([]map[string]any)
	if !ok || len(entityPatches) != 1 {
		t.Fatalf("expected typed player entity patch slice, got %+v", sourceMessages[1]["entities"])
	}
	if entityPatches[0]["entity_id"] != observerCharacter.ID {
		t.Fatalf("expected observer entity_id in movement patch, got %+v", entityPatches[0])
	}
	position, ok := entityPatches[0]["position"].(runtimePoint)
	if !ok {
		t.Fatalf("expected runtimePoint in movement patch, got %+v", entityPatches[0]["position"])
	}
	if position != (runtimePoint{X: 12, Z: -3}) {
		t.Fatalf("expected movement patch to fan out authoritative observer position, got %+v", position)
	}
	knownObserver, exists := sourceRuntime.knownEntities[observerCharacter.ID]
	if !exists || knownObserver.Position != (runtimePoint{X: 12, Z: -3}) {
		t.Fatalf("expected source runtime known-set to reconcile observer position, got %+v exists=%v", knownObserver, exists)
	}

	server.unregisterAttachedSession("sess_presence_observer")

	if len(sourceMessages) != 3 || sourceMessages[2]["kind"] != "entity_disappear" {
		t.Fatalf("expected source to receive player entity_disappear, got %+v", sourceMessages)
	}
	if sourceMessages[2]["entity_id"] != observerCharacter.ID {
		t.Fatalf("expected observer player to disappear on disconnect, got %+v", sourceMessages[2])
	}
	if _, exists := sourceRuntime.knownEntities[observerCharacter.ID]; exists {
		t.Fatalf("expected source runtime to remove observer player after disconnect")
	}
}
