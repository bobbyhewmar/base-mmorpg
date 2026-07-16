package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

func TestWorldEnterReissuesActiveOwnedSessionForReconnect(t *testing.T) {
	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "ownership.reconnect@test")
	createResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Male",
		"hair_style": 0,
		"hair_color": "#6b4e37",
		"skin_type":  0,
		"name":       "Lease Reconnect",
	}, accessToken)
	if createResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", createResponse.StatusCode)
	}
	characterPayload := decodeBody[map[string]any](t, createResponse)
	createdCharacter, ok := characterPayload["character"].(map[string]any)
	if !ok {
		t.Fatalf("missing character payload: %+v", characterPayload)
	}
	characterID := createdCharacter["character_id"].(string)

	enter := func() map[string]any {
		response := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/world/enter", map[string]any{
			"character_id": characterID,
		}, accessToken)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("world enter status = %d", response.StatusCode)
		}
		return decodeBody[map[string]any](t, response)
	}
	firstEnter := enter()
	sessionID := firstEnter["session_id"].(string)
	attachToken := firstEnter["attach_token"].(string)
	ownedSession, _, err := env.server.attachSession(sessionID, attachToken)
	if err != nil {
		t.Fatalf("attachSession() error = %v", err)
	}
	defer env.server.closeAttachedSession(ownedSession.ID, ownedSession.FencingToken)

	secondEnter := enter()
	if secondEnter["session_id"] != sessionID || secondEnter["attach_token"] == attachToken {
		t.Fatalf("reconnect must reissue the active owned session: first=%+v second=%+v", firstEnter, secondEnter)
	}
	if secondEnter["self_state"] == nil {
		t.Fatalf("reconnect response lost authoritative hydration: %+v", secondEnter)
	}
	reconnectedSession, _, err := env.server.attachSession(sessionID, secondEnter["attach_token"].(string))
	if err != nil {
		t.Fatalf("attachSession(reconnect) error = %v", err)
	}
	defer env.server.closeAttachedSession(reconnectedSession.ID, reconnectedSession.FencingToken)
	if reconnectedSession.FencingToken <= ownedSession.FencingToken {
		t.Fatalf("reconnect did not advance the durable fence: before=%+v after=%+v", ownedSession, reconnectedSession)
	}
	if _, err := env.store.GameplaySessions.RenewOwnership(context.Background(), characterID, ownedSession.ID, env.server.config.ServerInstanceID, ownedSession.FencingToken, "dawn_plaza", time.Minute, 5*time.Minute); !errors.Is(err, errOwnershipStale) {
		t.Fatalf("previous socket fence remained valid after reconnect: %v", err)
	}
}

func TestPostgresSessionOwnershipSerializesTwoServerInstances(t *testing.T) {
	env := newPersistenceTestEnv(t)
	accountID, _ := registerAndLogin(t, env, "ownership.instances@test")
	character := &Character{
		ID:           "character_pg_ownership",
		AccountID:    accountID,
		Name:         "Ownership Hero",
		Race:         "Human",
		BaseClass:    "Fighter",
		Sex:          "Female",
		Level:        1,
		CurrentCP:    80,
		CurrentHP:    122,
		CurrentMP:    58,
		LastRegionID: "dawn_plaza",
		PositionX:    10,
		PositionZ:    10,
		IsEnterable:  true,
	}
	if err := env.store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}
	session := &Session{
		ID:              "session_pg_ownership",
		AccountID:       accountID,
		CharacterID:     character.ID,
		AttachToken:     "attach_pg_ownership",
		AttachExpiresAt: time.Now().Add(5 * time.Minute),
		Status:          sessionStatusPendingAttach,
	}
	if err := env.store.GameplaySessions.Create(context.Background(), session); err != nil {
		t.Fatalf("GameplaySessions.Create() error = %v", err)
	}

	secondStore, err := NewStore(os.Getenv("L2BG_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("NewStore(second instance) error = %v", err)
	}
	defer secondStore.Close()
	serverA := NewServerWithConfig(":0", "", env.store, ServerConfig{ServerInstanceID: "pg-instance-a"})
	serverB := NewServerWithConfig(":0", "", secondStore, ServerConfig{ServerInstanceID: "pg-instance-b"})

	start := make(chan struct{})
	type attachResult struct {
		instance string
		session  *Session
		err      error
	}
	results := make(chan attachResult, 2)
	var wait sync.WaitGroup
	for instance, server := range map[string]*Server{"pg-instance-a": serverA, "pg-instance-b": serverB} {
		wait.Add(1)
		go func(instance string, server *Server) {
			defer wait.Done()
			<-start
			attached, _, err := server.attachSession(session.ID, session.AttachToken)
			results <- attachResult{instance: instance, session: attached, err: err}
		}(instance, server)
	}
	close(start)
	wait.Wait()
	close(results)

	winners := 0
	losers := 0
	winnerInstance := ""
	var winnerSession *Session
	for result := range results {
		if result.err == nil {
			winners++
			winnerInstance = result.instance
			winnerSession = result.session
			continue
		}
		if result.err.Error() != "session.invalid_attach_token" {
			t.Fatalf("unexpected attach failure: %v", result.err)
		}
		losers++
	}
	if winners != 1 || losers != 1 || winnerSession == nil {
		t.Fatalf("expected one PostgreSQL owner, winners=%d losers=%d", winners, losers)
	}
	ownership, err := secondStore.GameplaySessions.GetActiveOwnershipByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("GetActiveOwnershipByCharacterID() error = %v", err)
	}
	if ownership.ServerInstanceID != winnerInstance || ownership.FencingToken != winnerSession.FencingToken {
		t.Fatalf("persistent winner mismatch: result=%s session=%+v ownership=%+v", winnerInstance, winnerSession, ownership)
	}

	if err := secondStore.SanitizeGameplaySessionLifecycle(context.Background(), time.Now()); err != nil {
		t.Fatalf("SanitizeGameplaySessionLifecycle() error = %v", err)
	}
	persisted, err := env.store.GameplaySessions.GetByID(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GameplaySessions.GetByID() error = %v", err)
	}
	if persisted.Status != sessionStatusAttached {
		t.Fatalf("startup sanitization on another instance closed a live owner: %+v", persisted)
	}

	ownerStore := env.store
	if winnerInstance == "pg-instance-b" {
		ownerStore = secondStore
	}
	renewed, err := ownerStore.GameplaySessions.RenewOwnership(context.Background(), character.ID, winnerSession.ID, winnerInstance, winnerSession.FencingToken, "dawn_plaza", time.Minute, 5*time.Minute)
	if err != nil {
		t.Fatalf("RenewOwnership(winner) error = %v", err)
	}
	if renewed.FencingToken != winnerSession.FencingToken {
		t.Fatalf("renewal changed fencing token: before=%d after=%d", winnerSession.FencingToken, renewed.FencingToken)
	}
	regionOwnerships, err := secondStore.GameplaySessions.ListActiveOwnershipsByRegion(context.Background(), "dawn_plaza")
	if err != nil || len(regionOwnerships) != 1 || regionOwnerships[0].CharacterID != character.ID || regionOwnerships[0].ServerInstanceID != winnerInstance {
		t.Fatalf("ListActiveOwnershipsByRegion(dawn_plaza)=%+v error=%v", regionOwnerships, err)
	}
	otherRegionOwnerships, err := secondStore.GameplaySessions.ListActiveOwnershipsByRegion(context.Background(), "gate_road")
	if err != nil || len(otherRegionOwnerships) != 0 {
		t.Fatalf("ListActiveOwnershipsByRegion(gate_road)=%+v error=%v", otherRegionOwnerships, err)
	}
	wrongInstance := "pg-instance-a"
	if winnerInstance == wrongInstance {
		wrongInstance = "pg-instance-b"
	}
	if _, err := ownerStore.GameplaySessions.RenewOwnership(context.Background(), character.ID, winnerSession.ID, wrongInstance, winnerSession.FencingToken, "dawn_plaza", time.Minute, 5*time.Minute); !errors.Is(err, errOwnershipStale) {
		t.Fatalf("expected wrong instance to be fenced, got %v", err)
	}
	released, err := ownerStore.GameplaySessions.ReleaseOwnership(context.Background(), character.ID, winnerSession.ID, winnerInstance, winnerSession.FencingToken)
	if err != nil || !released {
		t.Fatalf("ReleaseOwnership(winner) released=%v error=%v", released, err)
	}
	released, err = secondStore.GameplaySessions.ReleaseOwnership(context.Background(), character.ID, winnerSession.ID, winnerInstance, winnerSession.FencingToken)
	if err != nil || released {
		t.Fatalf("double PostgreSQL release must be a no-op, released=%v error=%v", released, err)
	}
	nextSession := &Session{
		ID:              "session_pg_ownership_next",
		AccountID:       accountID,
		CharacterID:     character.ID,
		AttachToken:     "attach_pg_ownership_next",
		AttachExpiresAt: time.Now().Add(5 * time.Minute),
		Status:          sessionStatusPendingAttach,
	}
	if err := env.store.GameplaySessions.Create(context.Background(), nextSession); err != nil {
		t.Fatalf("GameplaySessions.Create(next) error = %v", err)
	}
	next, err := secondStore.GameplaySessions.AcquireOwnership(context.Background(), nextSession.ID, nextSession.AttachToken, "pg-instance-c", time.Minute, 5*time.Minute)
	if err != nil {
		t.Fatalf("AcquireOwnership(after release) error = %v", err)
	}
	if next.Ownership.FencingToken != winnerSession.FencingToken+1 {
		t.Fatalf("PostgreSQL fence reset after release: winner=%+v next=%+v", winnerSession, next.Ownership)
	}
}
