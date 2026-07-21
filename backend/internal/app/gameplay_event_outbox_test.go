package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

func gameplayEventForTest(key string, targetInstance string) *GameplayEvent {
	return &GameplayEvent{
		IdempotencyKey:         key,
		Type:                   remoteTargetNoticeEventType,
		Payload:                json.RawMessage(`{"actor_character_id":"actor","target_character_id":"target","source_server_instance_id":"source","reason_code":"presence.target_remote","target_fencing_token":1}`),
		TargetServerInstanceID: targetInstance,
		TargetCharacterID:      "target",
	}
}

func regionProjectionEventForOutboxTest(t *testing.T, key string, targetInstance string, targetCharacterID string, recipientFence int64, sourceFence int64, version int64, action string) *GameplayEvent {
	t.Helper()
	payload, err := json.Marshal(regionPlayerProjectionPayload{
		Action:                 action,
		CharacterID:            "projection-source",
		DisplayName:            "Projection Source",
		RegionID:               "dawn_plaza",
		Position:               runtimePoint{X: float64(version), Z: -float64(version)},
		Facing:                 1.5,
		SourceSessionID:        "projection-source-session",
		SourceServerInstanceID: "instance-a",
		FencingToken:           sourceFence,
		Version:                version,
		RecipientFencingToken:  recipientFence,
	})
	if err != nil {
		t.Fatalf("marshal projection payload error=%v", err)
	}
	return &GameplayEvent{
		IdempotencyKey:         key,
		Type:                   regionPlayerProjectionEventType,
		Payload:                payload,
		TargetServerInstanceID: targetInstance,
		TargetRegionID:         "dawn_plaza",
		TargetSessionID:        "projection-target-session",
		TargetCharacterID:      targetCharacterID,
	}
}

func TestMemoryGameplayEventConcurrentClaimDoesNotDuplicateDelivery(t *testing.T) {
	backend := newMemoryStoreBackend()
	firstStore := newMemoryStoreWithBackend(backend)
	secondStore := newMemoryStoreWithBackend(backend)
	event := gameplayEventForTest("claim-once", "instance-b")
	created, err := firstStore.GameplayEvents.Create(context.Background(), event)
	if err != nil || !created {
		t.Fatalf("GameplayEvents.Create() created=%v error=%v", created, err)
	}

	start := make(chan struct{})
	claims := make(chan []GameplayEvent, 2)
	var wait sync.WaitGroup
	for index, store := range []*Store{firstStore, secondStore} {
		wait.Add(1)
		go func(index int, store *Store) {
			defer wait.Done()
			<-start
			claimed, claimErr := store.GameplayEvents.Claim(context.Background(), "instance-b", "worker-"+string(rune('a'+index)), time.Now(), time.Minute, 1)
			if claimErr != nil {
				t.Errorf("GameplayEvents.Claim() error=%v", claimErr)
			}
			claims <- claimed
		}(index, store)
	}
	close(start)
	wait.Wait()
	close(claims)

	claimedCount := 0
	claimOwnerID := ""
	for claimed := range claims {
		claimedCount += len(claimed)
		if len(claimed) == 1 {
			claimOwnerID = claimed[0].ClaimOwnerID
		}
	}
	if claimedCount != 1 || claimOwnerID == "" {
		t.Fatalf("concurrent claim count=%d owner=%q", claimedCount, claimOwnerID)
	}
	delivered, err := firstStore.GameplayEvents.MarkDelivered(context.Background(), event.ID, claimOwnerID, time.Now())
	if err != nil || !delivered {
		t.Fatalf("MarkDelivered() delivered=%v error=%v", delivered, err)
	}
	reclaimed, err := secondStore.GameplayEvents.Claim(context.Background(), "instance-b", "worker-c", time.Now(), time.Minute, 1)
	if err != nil || len(reclaimed) != 0 {
		t.Fatalf("delivered event was reclaimed: events=%+v error=%v", reclaimed, err)
	}
	identical := gameplayEventForTest("claim-once", "instance-b")
	if created, err := secondStore.GameplayEvents.Create(context.Background(), identical); err != nil || created || identical.ID != event.ID {
		t.Fatalf("identical idempotency replay created=%v event=%+v error=%v", created, identical, err)
	}
	conflicting := gameplayEventForTest("claim-once", "instance-c")
	if _, err := secondStore.GameplayEvents.Create(context.Background(), conflicting); !errors.Is(err, errRecordConflict) {
		t.Fatalf("conflicting idempotency key error=%v", err)
	}
}

func TestMemoryGameplayEventReceiptIsDurableAcrossConcurrentConsumers(t *testing.T) {
	backend := newMemoryStoreBackend()
	firstStore := newMemoryStoreWithBackend(backend)
	secondStore := newMemoryStoreWithBackend(backend)
	event := gameplayEventForTest("receipt-concurrent", "instance-b")
	if created, err := firstStore.GameplayEvents.Create(context.Background(), event); err != nil || !created {
		t.Fatalf("GameplayEvents.Create() created=%v error=%v", created, err)
	}
	receipt := GameplayEventReceipt{EventID: event.ID, RecipientCharacterID: "target", ServerInstanceID: "instance-b"}
	start := make(chan struct{})
	reservations := make(chan GameplayEventReceiptReservation, 2)
	var wait sync.WaitGroup
	for index, store := range []*Store{firstStore, secondStore} {
		wait.Add(1)
		go func(index int, store *Store) {
			defer wait.Done()
			<-start
			reservation, reserveErr := store.GameplayReceipts.Reserve(context.Background(), receipt, "receipt-worker-"+string(rune('a'+index)), time.Now(), time.Minute)
			if reserveErr != nil {
				t.Errorf("GameplayReceipts.Reserve() error=%v", reserveErr)
			}
			reservations <- reservation
		}(index, store)
	}
	close(start)
	wait.Wait()
	close(reservations)

	acquired := 0
	busy := 0
	owner := ""
	for reservation := range reservations {
		if reservation.Acquired {
			acquired++
			owner = reservation.Receipt.ClaimOwnerID
		}
		if reservation.Busy {
			busy++
		}
	}
	if acquired != 1 || busy != 1 || owner == "" {
		t.Fatalf("receipt reservations acquired=%d busy=%d owner=%q", acquired, busy, owner)
	}
	if consumed, err := firstStore.GameplayReceipts.MarkConsumed(context.Background(), event.ID, owner, time.Now()); err != nil || !consumed {
		t.Fatalf("GameplayReceipts.MarkConsumed() consumed=%v error=%v", consumed, err)
	}
	replayed, err := secondStore.GameplayReceipts.Reserve(context.Background(), receipt, "receipt-restart-worker", time.Now(), time.Minute)
	if err != nil || !replayed.Duplicate || replayed.Acquired || replayed.Receipt.ConsumedAt.IsZero() {
		t.Fatalf("durable receipt replay=%+v error=%v", replayed, err)
	}
}

func TestMemorySocialTransactionRollsBackMutationOutcomeAndOutbox(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()
	if err := store.Characters.Create(ctx, &Character{ID: "leader", AccountID: "social-tx-account", Name: "Social Tx Leader", Race: "Human", BaseClass: "Fighter", Sex: "Male", Level: 1, IsEnterable: true}); err != nil {
		t.Fatalf("Characters.Create() error=%v", err)
	}
	record := &GameplayCommandRecord{SessionID: "social-tx-session", CommandSeq: 1, CommandID: "social-tx-command", CommandType: "invite_party_member", Status: gameplayCommandRecordStatusPending}
	if err := store.GameplayCommands.CreatePending(ctx, record); err != nil {
		t.Fatalf("CreatePending() error=%v", err)
	}
	party := &Party{ID: "social-tx-party", LeaderCharacterID: "leader", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	leader := PartyMember{PartyID: party.ID, CharacterID: "leader", JoinedAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	event := gameplayEventForTest("social-tx-event", "instance-b")
	sentinel := errors.New("force social transaction rollback")
	err := store.RunSocialCommandTransaction(ctx, func(txCtx context.Context) error {
		if createErr := store.Parties.Create(txCtx, party, leader); createErr != nil {
			return createErr
		}
		if _, finalizeErr := store.FinalizeGameplayCommandWithEvents(txCtx, record.SessionID, record.CommandSeq, gameplayCommandRecordStatusApplied, []map[string]any{{"kind": "delta"}}, []*GameplayEvent{event}); finalizeErr != nil {
			return finalizeErr
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("RunSocialCommandTransaction() error=%v", err)
	}
	if persistedParty, getErr := store.Parties.GetByID(ctx, party.ID); !errors.Is(getErr, errRecordNotFound) || persistedParty != nil {
		t.Fatalf("rolled back party=%+v error=%v", persistedParty, getErr)
	}
	if persistedEvent, getErr := store.GameplayEvents.GetByIdempotencyKey(ctx, event.IdempotencyKey); !errors.Is(getErr, errRecordNotFound) || persistedEvent != nil {
		t.Fatalf("rolled back event=%+v error=%v", persistedEvent, getErr)
	}
	persistedRecord, err := store.GameplayCommands.GetBySessionAndSeq(ctx, record.SessionID, record.CommandSeq)
	if err != nil || persistedRecord.Status != gameplayCommandRecordStatusPending || len(persistedRecord.OutboundMessages) != 0 {
		t.Fatalf("rolled back command=%+v error=%v", persistedRecord, err)
	}
}

func TestMemoryGameplayEventFailureAndRetentionAreSafe(t *testing.T) {
	store := newMemoryStore()
	old := time.Now().Add(-2 * time.Hour)
	deliveredEvent := gameplayEventForTest("retention-delivered", "retention-instance")
	deliveredEvent.CreatedAt = old
	deliveredEvent.AvailableAt = old
	if created, err := store.GameplayEvents.Create(context.Background(), deliveredEvent); err != nil || !created {
		t.Fatalf("create delivered event created=%v error=%v", created, err)
	}
	claimed, err := store.GameplayEvents.Claim(context.Background(), "retention-instance", "retention-worker", old.Add(time.Minute), time.Hour, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim delivered event events=%+v error=%v", claimed, err)
	}
	if delivered, err := store.GameplayEvents.MarkDelivered(context.Background(), deliveredEvent.ID, "retention-worker", old.Add(2*time.Minute)); err != nil || !delivered {
		t.Fatalf("mark old event delivered=%v error=%v", delivered, err)
	}

	pendingEvent := gameplayEventForTest("retention-pending", "pending-instance")
	pendingEvent.CreatedAt = old
	pendingEvent.AvailableAt = old
	if created, err := store.GameplayEvents.Create(context.Background(), pendingEvent); err != nil || !created {
		t.Fatalf("create pending event created=%v error=%v", created, err)
	}
	failingEvent := gameplayEventForTest("retention-failed", "failure-instance")
	if created, err := store.GameplayEvents.Create(context.Background(), failingEvent); err != nil || !created {
		t.Fatalf("create failing event created=%v error=%v", created, err)
	}
	claimed, err = store.GameplayEvents.Claim(context.Background(), "failure-instance", "failure-worker", time.Now(), time.Minute, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim failing event events=%+v error=%v", claimed, err)
	}
	failure, err := store.GameplayEvents.MarkFailed(context.Background(), failingEvent.ID, "failure-worker", time.Now(), time.Millisecond, 3, " target not local ")
	if err != nil || failure.RetryCount != 1 || failure.DeadLettered {
		t.Fatalf("MarkFailed() failure=%+v error=%v", failure, err)
	}
	persistedFailure, err := store.GameplayEvents.GetByIdempotencyKey(context.Background(), failingEvent.IdempotencyKey)
	if err != nil || persistedFailure.RetryCount != 1 || persistedFailure.LastError != "target_not_local" {
		t.Fatalf("failed event not persisted safely: event=%+v error=%v", persistedFailure, err)
	}

	deleted, err := store.GameplayEvents.DeleteDeliveredBefore(context.Background(), time.Now().Add(-time.Hour), 10)
	if err != nil || deleted != 1 {
		t.Fatalf("DeleteDeliveredBefore() deleted=%d error=%v", deleted, err)
	}
	if _, err := store.GameplayEvents.GetByIdempotencyKey(context.Background(), deliveredEvent.IdempotencyKey); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("old delivered event survived retention: %v", err)
	}
	if _, err := store.GameplayEvents.GetByIdempotencyKey(context.Background(), pendingEvent.IdempotencyKey); err != nil {
		t.Fatalf("retention removed pending event: %v", err)
	}
	if _, err := store.GameplayEvents.GetByIdempotencyKey(context.Background(), failingEvent.IdempotencyKey); err != nil {
		t.Fatalf("retention removed failed event: %v", err)
	}
}

func TestMemoryRegionProjectionSupersessionCompactsOnlyObsoleteRows(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()
	older := regionProjectionEventForOutboxTest(t, "projection-supersede-v1", "instance-b", "projection-target", 7, 3, 1, regionProjectionActionUpsert)
	if created, err := store.GameplayEvents.Create(ctx, older); err != nil || !created {
		t.Fatalf("create older projection created=%v error=%v", created, err)
	}
	newer := regionProjectionEventForOutboxTest(t, "projection-supersede-v2", "instance-b", "projection-target", 7, 3, 2, regionProjectionActionUpsert)
	if created, err := store.GameplayEvents.Create(ctx, newer); err != nil || !created {
		t.Fatalf("create newer projection created=%v error=%v", created, err)
	}
	superseded, err := store.GameplayEvents.SupersedeRegionProjection(ctx, RegionProjectionSupersession{
		TargetServerInstanceID:          newer.TargetServerInstanceID,
		TargetCharacterID:               newer.TargetCharacterID,
		ProjectionSourceCharacterID:     newer.ProjectionSourceCharacterID,
		ProjectionSourceFencingToken:    newer.ProjectionSourceFencingToken,
		ProjectionVersion:               newer.ProjectionVersion,
		ProjectionRecipientFencingToken: newer.ProjectionRecipientFencingToken,
		SupersedingEventID:              newer.ID,
		SupersededAt:                    time.Now().UTC(),
	})
	if err != nil || superseded != 1 {
		t.Fatalf("SupersedeRegionProjection() superseded=%d error=%v", superseded, err)
	}
	persistedOlder, err := store.GameplayEvents.GetByID(ctx, older.ID)
	if err != nil || persistedOlder.SupersededAt.IsZero() || persistedOlder.SupersededByEventID != newer.ID {
		t.Fatalf("older projection not superseded safely: event=%+v error=%v", persistedOlder, err)
	}
	claimed, err := store.GameplayEvents.Claim(ctx, "instance-b", "projection-worker", time.Now(), time.Minute, 10)
	if err != nil || len(claimed) != 1 || claimed[0].ID != newer.ID {
		t.Fatalf("Claim() did not keep only current projection: claimed=%+v error=%v", claimed, err)
	}
	deleted, err := store.GameplayEvents.DeleteSupersededBefore(ctx, time.Now().Add(time.Second), 10)
	if err != nil || deleted != 1 {
		t.Fatalf("DeleteSupersededBefore() deleted=%d error=%v", deleted, err)
	}
	if _, err := store.GameplayEvents.GetByID(ctx, older.ID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("obsolete projection survived compaction: %v", err)
	}
	if persistedNewer, err := store.GameplayEvents.GetByID(ctx, newer.ID); err != nil || !persistedNewer.SupersededAt.IsZero() {
		t.Fatalf("current projection was compacted incorrectly: event=%+v error=%v", persistedNewer, err)
	}
}

func TestRemoteTargetNoticeCrossesInstancesWithoutChangingCommandResult(t *testing.T) {
	backend := newMemoryStoreBackend()
	storeA := newMemoryStoreWithBackend(backend)
	storeB := newMemoryStoreWithBackend(backend)
	actorCharacter, actorSession := createOwnershipTestCharacterAndSession(t, storeA, "fanout_actor", "fanout_actor_session")
	targetCharacter, targetSession := createOwnershipTestCharacterAndSession(t, storeA, "fanout_target", "fanout_target_session")
	actorOwned, err := storeA.GameplaySessions.AcquireOwnership(context.Background(), actorSession.ID, actorSession.AttachToken, "instance-a", time.Minute, 5*time.Minute)
	if err != nil {
		t.Fatalf("AcquireOwnership(actor) error=%v", err)
	}
	targetOwned, err := storeB.GameplaySessions.AcquireOwnership(context.Background(), targetSession.ID, targetSession.AttachToken, "instance-b", time.Minute, 5*time.Minute)
	if err != nil {
		t.Fatalf("AcquireOwnership(target) error=%v", err)
	}
	serverA := NewServerWithConfig(":0", "", storeA, ServerConfig{ServerInstanceID: "instance-a"})
	serverB := NewServerWithConfig(":0", "", storeB, ServerConfig{ServerInstanceID: "instance-b"})
	actorRuntime := newAttachedRuntime(actorOwned.Session.ID, actorCharacter)
	actorRuntime.knownEntities[targetCharacter.ID] = runtimeEntity{
		EntityID:   targetCharacter.ID,
		EntityType: "player",
		TemplateID: "remote_player",
		Position:   runtimePoint{X: 12, Z: 10},
		State:      map[string]any{"hp": 122, "alive": true},
	}
	targetRuntime := newAttachedRuntime(targetOwned.Session.ID, targetCharacter)
	var deliveredMu sync.Mutex
	deliveredMessages := make([]map[string]any, 0)
	serverA.registerAttachedSession(actorOwned.Session.ID, actorRuntime, func(map[string]any) bool { return true }, actorOwned.Session)
	serverB.registerAttachedSession(targetOwned.Session.ID, targetRuntime, func(payload map[string]any) bool {
		deliveredMu.Lock()
		defer deliveredMu.Unlock()
		deliveredMessages = append(deliveredMessages, payload)
		return true
	}, targetOwned.Session)
	t.Cleanup(func() {
		serverA.unregisterAttachedSession(actorOwned.Session.ID, actorOwned.Session.FencingToken)
		serverB.unregisterAttachedSession(targetOwned.Session.ID, targetOwned.Session.FencingToken)
	})

	command := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "fanout_remote_target_command",
		CommandSeq:      1,
		Type:            "select_target",
		Payload:         json.RawMessage(`{"target_id":"fanout_target"}`),
	}
	first, _ := serverA.processGameplayCommandWithDedup(context.Background(), actorOwned.Session, actorRuntime, command)
	if extractRejectReason(first) != "presence.target_remote" || actorRuntime.targetID != "" {
		t.Fatalf("remote target result changed: outbound=%+v target=%q", first, actorRuntime.targetID)
	}
	replayed, shouldFanOut := serverA.processGameplayCommandWithDedup(context.Background(), actorOwned.Session, actorRuntime, command)
	if extractRejectReason(replayed) != "presence.target_remote" || shouldFanOut {
		t.Fatalf("identical replay changed result: outbound=%+v fanout=%v", replayed, shouldFanOut)
	}
	conflicting := command
	conflicting.CommandID = "fanout_remote_target_conflict"
	conflictOutbound, _ := serverA.processGameplayCommandWithDedup(context.Background(), actorOwned.Session, actorRuntime, conflicting)
	if extractRejectReason(conflictOutbound) != "sequence.conflicting_replay" {
		t.Fatalf("conflicting replay reason=%q outbound=%+v", extractRejectReason(conflictOutbound), conflictOutbound)
	}

	eventKey := "gameplay-command/" + actorOwned.Session.ID + "/1/remote-target-notice"
	event, err := storeB.GameplayEvents.GetByIdempotencyKey(context.Background(), eventKey)
	if err != nil || event.ID != 1 || event.TargetServerInstanceID != "instance-b" || event.TargetSessionID != targetOwned.Session.ID {
		t.Fatalf("outbox event mismatch: event=%+v error=%v", event, err)
	}
	if claimed := serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/test-worker"); claimed != 1 {
		t.Fatalf("dispatcher claimed=%d want=1", claimed)
	}
	if claimed := serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/test-worker-2"); claimed != 0 {
		t.Fatalf("dispatcher reclaimed delivered event: %d", claimed)
	}
	deliveredMu.Lock()
	defer deliveredMu.Unlock()
	if len(deliveredMessages) != 1 || deliveredMessages[0]["kind"] != presenceNoticeKind || deliveredMessages[0]["event_id"] != event.ID {
		t.Fatalf("remote notice delivery mismatch: %+v", deliveredMessages)
	}
	persisted, err := storeA.GameplayEvents.GetByIdempotencyKey(context.Background(), eventKey)
	if err != nil || persisted.DeliveredAt.IsZero() || persisted.RetryCount != 0 {
		t.Fatalf("delivered event state mismatch: event=%+v error=%v", persisted, err)
	}
}

func TestDispatcherFailureRetriesWithoutBreakingLaterCycles(t *testing.T) {
	store := newMemoryStore()
	server := NewServerWithConfig(":0", "", store, ServerConfig{
		ServerInstanceID:        "failure-instance",
		GameplayEventRetryDelay: time.Millisecond,
		GameplayEventMaxRetries: 3,
	})
	event := gameplayEventForTest("dispatcher-failure", "failure-instance")
	if created, err := store.GameplayEvents.Create(context.Background(), event); err != nil || !created {
		t.Fatalf("create event created=%v error=%v", created, err)
	}
	if claimed := server.dispatchGameplayEventsOnce(context.Background(), "failure-worker"); claimed != 1 {
		t.Fatalf("first cycle claimed=%d", claimed)
	}
	persisted, err := store.GameplayEvents.GetByIdempotencyKey(context.Background(), event.IdempotencyKey)
	if err != nil || persisted.RetryCount != 1 || persisted.LastError != "target_not_local" || !persisted.DeliveredAt.IsZero() {
		t.Fatalf("failed delivery state=%+v error=%v", persisted, err)
	}
	time.Sleep(2 * time.Millisecond)
	if claimed := server.dispatchGameplayEventsOnce(context.Background(), "failure-worker-2"); claimed != 1 {
		t.Fatalf("server did not continue retrying after failure, claimed=%d", claimed)
	}
	time.Sleep(3 * time.Millisecond)
	if claimed := server.dispatchGameplayEventsOnce(context.Background(), "failure-worker-3"); claimed != 1 {
		t.Fatalf("server did not reach dead-letter attempt, claimed=%d", claimed)
	}
	persisted, err = store.GameplayEvents.GetByIdempotencyKey(context.Background(), event.IdempotencyKey)
	if err != nil || persisted.RetryCount != 3 || persisted.DeadLetteredAt.IsZero() {
		t.Fatalf("dead-letter state=%+v error=%v", persisted, err)
	}
	if claimed := server.dispatchGameplayEventsOnce(context.Background(), "failure-worker-4"); claimed != 0 {
		t.Fatalf("dead-letter event was reclaimed, claimed=%d", claimed)
	}
}

func TestPostgresGameplayEventClaimIsSafeAcrossStores(t *testing.T) {
	env := newPersistenceTestEnv(t)
	secondStore, err := NewStore(os.Getenv("L2BG_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("NewStore(second) error=%v", err)
	}
	defer secondStore.Close()
	event := gameplayEventForTest("postgres-claim-once", "pg-instance-b")
	if created, err := env.store.GameplayEvents.Create(context.Background(), event); err != nil || !created {
		t.Fatalf("create PostgreSQL event created=%v error=%v", created, err)
	}

	start := make(chan struct{})
	claims := make(chan []GameplayEvent, 2)
	errorsChannel := make(chan error, 2)
	var wait sync.WaitGroup
	for index, store := range []*Store{env.store, secondStore} {
		wait.Add(1)
		go func(index int, store *Store) {
			defer wait.Done()
			<-start
			claimed, claimErr := store.GameplayEvents.Claim(context.Background(), "pg-instance-b", "pg-worker-"+string(rune('a'+index)), time.Now(), time.Minute, 1)
			claims <- claimed
			errorsChannel <- claimErr
		}(index, store)
	}
	close(start)
	wait.Wait()
	close(claims)
	close(errorsChannel)
	for claimErr := range errorsChannel {
		if claimErr != nil {
			t.Fatalf("PostgreSQL claim error=%v", claimErr)
		}
	}
	claimedCount := 0
	claimOwnerID := ""
	for claimed := range claims {
		claimedCount += len(claimed)
		if len(claimed) == 1 {
			claimOwnerID = claimed[0].ClaimOwnerID
		}
	}
	if claimedCount != 1 || claimOwnerID == "" {
		t.Fatalf("PostgreSQL concurrent claim count=%d owner=%q", claimedCount, claimOwnerID)
	}
	if delivered, err := secondStore.GameplayEvents.MarkDelivered(context.Background(), event.ID, claimOwnerID, time.Now()); err != nil || !delivered {
		t.Fatalf("PostgreSQL MarkDelivered() delivered=%v error=%v", delivered, err)
	}
	if reclaimed, err := env.store.GameplayEvents.Claim(context.Background(), "pg-instance-b", "pg-worker-c", time.Now(), time.Minute, 1); err != nil || len(reclaimed) != 0 {
		t.Fatalf("PostgreSQL delivered event reclaimed=%+v error=%v", reclaimed, err)
	}
}

func TestPostgresGameplayCommandAndOutboxFinalizeAtomically(t *testing.T) {
	env := newPersistenceTestEnv(t)
	account := &Account{ID: "outbox_atomic_account", Login: "outbox.atomic@test", DisplayName: "Outbox Atomic", State: accountStateActive}
	if err := env.store.Accounts.Create(context.Background(), account); err != nil {
		t.Fatalf("Accounts.Create() error=%v", err)
	}
	character := &Character{
		ID:           "outbox_atomic_character",
		AccountID:    account.ID,
		Name:         "Outbox Atomic",
		Race:         "Human",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		CurrentCP:    80,
		CurrentHP:    122,
		CurrentMP:    58,
		LastRegionID: "dawn_plaza",
		IsEnterable:  true,
	}
	if err := env.store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error=%v", err)
	}
	session := &Session{
		ID:              "outbox_atomic_session",
		AccountID:       account.ID,
		CharacterID:     character.ID,
		AttachToken:     "outbox_atomic_attach",
		AttachExpiresAt: time.Now().Add(5 * time.Minute),
		Status:          sessionStatusPendingAttach,
	}
	if err := env.store.GameplaySessions.Create(context.Background(), session); err != nil {
		t.Fatalf("GameplaySessions.Create() error=%v", err)
	}
	pending := &GameplayCommandRecord{
		SessionID:   session.ID,
		CommandSeq:  1,
		CommandID:   "outbox_atomic_command",
		CommandType: "select_target",
	}
	if err := env.store.GameplayCommands.CreatePending(context.Background(), pending); err != nil {
		t.Fatalf("GameplayCommands.CreatePending() error=%v", err)
	}
	outbound := []map[string]any{rejectMessage(pending.CommandID, pending.CommandSeq, "presence.target_remote", "Remote target remains non-interactable.")}
	event := gameplayEventForTest("outbox-atomic-event", "pg-instance-b")
	created, err := env.store.FinalizeGameplayCommandWithEvent(context.Background(), session.ID, 1, gameplayCommandRecordStatusRejected, outbound, event)
	if err != nil || !created {
		t.Fatalf("FinalizeGameplayCommandWithEvent() created=%v error=%v", created, err)
	}
	record, err := env.store.GameplayCommands.GetBySessionAndSeq(context.Background(), session.ID, 1)
	if err != nil || record.Status != gameplayCommandRecordStatusRejected || extractRejectReason(record.OutboundMessages) != "presence.target_remote" {
		t.Fatalf("command outcome mismatch: record=%+v error=%v", record, err)
	}
	if persisted, err := env.store.GameplayEvents.GetByIdempotencyKey(context.Background(), event.IdempotencyKey); err != nil || persisted.ID != event.ID {
		t.Fatalf("event outcome mismatch: event=%+v error=%v", persisted, err)
	}
	created, err = env.store.FinalizeGameplayCommandWithEvent(context.Background(), session.ID, 1, gameplayCommandRecordStatusRejected, outbound, gameplayEventForTest("outbox-atomic-event", "pg-instance-b"))
	if err != nil || created {
		t.Fatalf("idempotent finalize duplicated event: created=%v error=%v", created, err)
	}

	secondPending := &GameplayCommandRecord{SessionID: session.ID, CommandSeq: 2, CommandID: "outbox_atomic_invalid", CommandType: "select_target"}
	if err := env.store.GameplayCommands.CreatePending(context.Background(), secondPending); err != nil {
		t.Fatalf("GameplayCommands.CreatePending(second) error=%v", err)
	}
	invalidEvent := gameplayEventForTest("outbox-atomic-invalid", "")
	if _, err := env.store.FinalizeGameplayCommandWithEvent(context.Background(), session.ID, 2, gameplayCommandRecordStatusRejected, outbound, invalidEvent); err == nil {
		t.Fatal("invalid outbox event unexpectedly committed")
	}
	rolledBack, err := env.store.GameplayCommands.GetBySessionAndSeq(context.Background(), session.ID, 2)
	if err != nil || rolledBack.Status != gameplayCommandRecordStatusPending || len(rolledBack.OutboundMessages) != 0 {
		t.Fatalf("command update was not rolled back with event failure: record=%+v error=%v", rolledBack, err)
	}

	chatPending := &GameplayCommandRecord{SessionID: session.ID, CommandSeq: 3, CommandID: "outbox_atomic_chat", CommandType: "send_chat_message"}
	if err := env.store.GameplayCommands.CreatePending(context.Background(), chatPending); err != nil {
		t.Fatalf("GameplayCommands.CreatePending(chat) error=%v", err)
	}
	chatRecord := ChatMessageRecord{
		ID:                "outbox_atomic_chat_message",
		CharacterID:       character.ID,
		AccountID:         account.ID,
		Channel:           chatChannelWhisper,
		TargetCharacterID: character.ID,
		Text:              "atomic remote chat",
		SessionID:         session.ID,
		CommandID:         chatPending.CommandID,
		CommandSeq:        chatPending.CommandSeq,
		CreatedAt:         time.Now().UTC(),
	}
	chatEvents := []*GameplayEvent{
		gameplayEventForTest("outbox-atomic-chat-event-a", "pg-instance-b"),
		gameplayEventForTest("outbox-atomic-chat-event-b", "pg-instance-c"),
	}
	chatOutbound := []map[string]any{chatMessagePayload(chatPending.CommandID, chatPending.CommandSeq, chatChannelWhisper, character.ID, character.Name, character.ID, character.Name, "", chatRecord.Text, chatRecord.CreatedAt)}
	createdCount, err := env.store.FinalizeGameplayCommandWithChatAndEvents(context.Background(), session.ID, chatPending.CommandSeq, gameplayCommandRecordStatusApplied, chatOutbound, chatRecord, chatEvents)
	if err != nil || createdCount != len(chatEvents) {
		t.Fatalf("FinalizeGameplayCommandWithChatAndEvents() created=%d error=%v", createdCount, err)
	}
	chatHistory, err := env.store.ChatMessages.ListByCharacterID(context.Background(), character.ID)
	if err != nil || len(chatHistory) != 1 || chatHistory[0].CommandID != chatPending.CommandID || chatHistory[0].CommandSeq != chatPending.CommandSeq {
		t.Fatalf("atomic chat history mismatch: records=%+v error=%v", chatHistory, err)
	}
	for _, chatEvent := range chatEvents {
		if persisted, eventErr := env.store.GameplayEvents.GetByIdempotencyKey(context.Background(), chatEvent.IdempotencyKey); eventErr != nil || persisted.ID != chatEvent.ID {
			t.Fatalf("atomic chat event mismatch: event=%+v error=%v", persisted, eventErr)
		}
	}
}

func TestPostgresGameplayEventRetentionDeletesOnlyOldDeliveredRows(t *testing.T) {
	env := newPersistenceTestEnv(t)
	old := time.Now().Add(-2 * time.Hour)
	deliveredEvent := gameplayEventForTest("pg-retention-delivered", "pg-retention")
	deliveredEvent.CreatedAt = old
	deliveredEvent.AvailableAt = old
	if created, err := env.store.GameplayEvents.Create(context.Background(), deliveredEvent); err != nil || !created {
		t.Fatalf("create delivered event created=%v error=%v", created, err)
	}
	claimed, err := env.store.GameplayEvents.Claim(context.Background(), "pg-retention", "pg-retention-worker", old.Add(time.Minute), time.Hour, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim delivered event events=%+v error=%v", claimed, err)
	}
	if delivered, err := env.store.GameplayEvents.MarkDelivered(context.Background(), deliveredEvent.ID, "pg-retention-worker", old.Add(2*time.Minute)); err != nil || !delivered {
		t.Fatalf("mark delivered event delivered=%v error=%v", delivered, err)
	}
	postgresEvents, ok := env.store.GameplayEvents.(postgresGameplayEventRepo)
	if !ok {
		t.Fatalf("expected PostgreSQL gameplay event repo, got %T", env.store.GameplayEvents)
	}
	if _, err := postgresEvents.backend.db.ExecContext(context.Background(), `UPDATE gameplay_event_outbox SET delivered_at = $2 WHERE event_id = $1`, deliveredEvent.ID, old.Add(2*time.Minute)); err != nil {
		t.Fatalf("backdate delivered event for retention test: %v", err)
	}
	pendingEvent := gameplayEventForTest("pg-retention-pending", "pg-pending")
	pendingEvent.CreatedAt = old
	pendingEvent.AvailableAt = old
	if created, err := env.store.GameplayEvents.Create(context.Background(), pendingEvent); err != nil || !created {
		t.Fatalf("create pending event created=%v error=%v", created, err)
	}

	deleted, err := env.store.GameplayEvents.DeleteDeliveredBefore(context.Background(), time.Now().Add(-time.Hour), 10)
	if err != nil || deleted != 1 {
		t.Fatalf("DeleteDeliveredBefore() deleted=%d error=%v", deleted, err)
	}
	if _, err := env.store.GameplayEvents.GetByIdempotencyKey(context.Background(), deliveredEvent.IdempotencyKey); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("old delivered PostgreSQL event survived retention: %v", err)
	}
	if _, err := env.store.GameplayEvents.GetByIdempotencyKey(context.Background(), pendingEvent.IdempotencyKey); err != nil {
		t.Fatalf("pending PostgreSQL event was removed by retention: %v", err)
	}
}

func TestPostgresRegionProjectionSupersessionCompactsOnlyObsoleteRows(t *testing.T) {
	env := newPersistenceTestEnv(t)
	ctx := context.Background()
	older := regionProjectionEventForOutboxTest(t, "pg-projection-supersede-v1", "pg-instance-b", "projection-target", 11, 5, 1, regionProjectionActionUpsert)
	if created, err := env.store.GameplayEvents.Create(ctx, older); err != nil || !created {
		t.Fatalf("create older PostgreSQL projection created=%v error=%v", created, err)
	}
	newer := regionProjectionEventForOutboxTest(t, "pg-projection-supersede-v2", "pg-instance-b", "projection-target", 11, 5, 2, regionProjectionActionDespawn)
	if created, err := env.store.GameplayEvents.Create(ctx, newer); err != nil || !created {
		t.Fatalf("create newer PostgreSQL projection created=%v error=%v", created, err)
	}
	superseded, err := env.store.GameplayEvents.SupersedeRegionProjection(ctx, RegionProjectionSupersession{
		TargetServerInstanceID:          newer.TargetServerInstanceID,
		TargetCharacterID:               newer.TargetCharacterID,
		ProjectionSourceCharacterID:     newer.ProjectionSourceCharacterID,
		ProjectionSourceFencingToken:    newer.ProjectionSourceFencingToken,
		ProjectionVersion:               newer.ProjectionVersion,
		ProjectionRecipientFencingToken: newer.ProjectionRecipientFencingToken,
		SupersedingEventID:              newer.ID,
		SupersededAt:                    time.Now().UTC(),
	})
	if err != nil || superseded != 1 {
		t.Fatalf("SupersedeRegionProjection() superseded=%d error=%v", superseded, err)
	}
	persistedOlder, err := env.store.GameplayEvents.GetByID(ctx, older.ID)
	if err != nil || persistedOlder.SupersededAt.IsZero() || persistedOlder.SupersededByEventID != newer.ID {
		t.Fatalf("older PostgreSQL projection not superseded safely: event=%+v error=%v", persistedOlder, err)
	}
	claimed, err := env.store.GameplayEvents.Claim(ctx, "pg-instance-b", "pg-projection-worker", time.Now(), time.Minute, 10)
	if err != nil || len(claimed) != 1 || claimed[0].ID != newer.ID {
		t.Fatalf("PostgreSQL Claim() did not keep only current projection: claimed=%+v error=%v", claimed, err)
	}
	deleted, err := env.store.GameplayEvents.DeleteSupersededBefore(ctx, time.Now().Add(time.Second), 10)
	if err != nil || deleted != 1 {
		t.Fatalf("PostgreSQL DeleteSupersededBefore() deleted=%d error=%v", deleted, err)
	}
	if _, err := env.store.GameplayEvents.GetByID(ctx, older.ID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("obsolete PostgreSQL projection survived compaction: %v", err)
	}
	if persistedNewer, err := env.store.GameplayEvents.GetByID(ctx, newer.ID); err != nil || !persistedNewer.SupersededAt.IsZero() {
		t.Fatalf("current PostgreSQL projection was compacted incorrectly: event=%+v error=%v", persistedNewer, err)
	}
}

func TestPostgresGameplayEventReceiptSerializesConcurrentConsumers(t *testing.T) {
	env := newPersistenceTestEnv(t)
	event := gameplayEventForTest("pg-receipt-concurrent", "pg-instance-b")
	if created, err := env.store.GameplayEvents.Create(context.Background(), event); err != nil || !created {
		t.Fatalf("GameplayEvents.Create() created=%v error=%v", created, err)
	}
	receipt := GameplayEventReceipt{EventID: event.ID, RecipientCharacterID: "target", ServerInstanceID: "pg-instance-b"}
	start := make(chan struct{})
	reservations := make(chan GameplayEventReceiptReservation, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			reservation, reserveErr := env.store.GameplayReceipts.Reserve(context.Background(), receipt, "pg-receipt-worker-"+string(rune('a'+index)), time.Now(), time.Minute)
			if reserveErr != nil {
				t.Errorf("GameplayReceipts.Reserve() error=%v", reserveErr)
			}
			reservations <- reservation
		}(index)
	}
	close(start)
	wait.Wait()
	close(reservations)

	acquired := 0
	busy := 0
	owner := ""
	for reservation := range reservations {
		if reservation.Acquired {
			acquired++
			owner = reservation.Receipt.ClaimOwnerID
		}
		if reservation.Busy {
			busy++
		}
	}
	if acquired != 1 || busy != 1 || owner == "" {
		t.Fatalf("PostgreSQL receipt reservations acquired=%d busy=%d owner=%q", acquired, busy, owner)
	}
	if consumed, err := env.store.GameplayReceipts.MarkConsumed(context.Background(), event.ID, owner, time.Now()); err != nil || !consumed {
		t.Fatalf("GameplayReceipts.MarkConsumed() consumed=%v error=%v", consumed, err)
	}
	replayed, err := env.store.GameplayReceipts.Reserve(context.Background(), receipt, "pg-restart-worker", time.Now(), time.Minute)
	if err != nil || !replayed.Duplicate || replayed.Receipt.ConsumedAt.IsZero() {
		t.Fatalf("PostgreSQL durable receipt replay=%+v error=%v", replayed, err)
	}
}

func TestPostgresSocialTransactionRollsBackMutationOutcomeAndOutbox(t *testing.T) {
	env := newPersistenceTestEnv(t)
	ctx := context.Background()
	accountID, _ := registerAndLogin(t, env, "social.atomic.rollback@test")
	character := &Character{
		ID:           "char_social_atomic_rollback",
		AccountID:    accountID,
		Name:         "Social Atomic Rollback",
		Race:         "Human",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
		IsEnterable:  true,
	}
	if err := env.store.CreateCharacterWithItemSeed(ctx, character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error=%v", err)
	}
	session := &Session{
		ID:              "session_social_atomic_rollback",
		AccountID:       accountID,
		CharacterID:     character.ID,
		AttachToken:     "attach_social_atomic_rollback",
		AttachExpiresAt: time.Now().Add(time.Minute),
		Status:          sessionStatusPendingAttach,
	}
	if err := env.store.GameplaySessions.Create(ctx, session); err != nil {
		t.Fatalf("GameplaySessions.Create() error=%v", err)
	}
	record := &GameplayCommandRecord{SessionID: session.ID, CommandSeq: 1, CommandID: "pg-social-atomic-command", CommandType: "create_clan", Status: gameplayCommandRecordStatusPending}
	if err := env.store.GameplayCommands.CreatePending(ctx, record); err != nil {
		t.Fatalf("CreatePending() error=%v", err)
	}
	party := &Party{ID: "pg-social-atomic-party", LeaderCharacterID: character.ID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	leader := PartyMember{PartyID: party.ID, CharacterID: character.ID, JoinedAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	event := gameplayEventForTest("pg-social-atomic-event", "pg-instance-b")
	event.TargetCharacterID = character.ID
	sentinel := errors.New("force postgres social transaction rollback")
	err := env.store.RunSocialCommandTransaction(ctx, func(txCtx context.Context) error {
		if createErr := env.store.Parties.Create(txCtx, party, leader); createErr != nil {
			return createErr
		}
		if _, finalizeErr := env.store.FinalizeGameplayCommandWithEvents(txCtx, session.ID, record.CommandSeq, gameplayCommandRecordStatusApplied, []map[string]any{{"kind": "delta"}}, []*GameplayEvent{event}); finalizeErr != nil {
			return finalizeErr
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("RunSocialCommandTransaction() error=%v", err)
	}
	if persistedParty, getErr := env.store.Parties.GetByID(ctx, party.ID); !errors.Is(getErr, errRecordNotFound) || persistedParty != nil {
		t.Fatalf("rolled back PostgreSQL party=%+v error=%v", persistedParty, getErr)
	}
	if persistedEvent, getErr := env.store.GameplayEvents.GetByIdempotencyKey(ctx, event.IdempotencyKey); !errors.Is(getErr, errRecordNotFound) || persistedEvent != nil {
		t.Fatalf("rolled back PostgreSQL event=%+v error=%v", persistedEvent, getErr)
	}
	persistedRecord, err := env.store.GameplayCommands.GetBySessionAndSeq(ctx, session.ID, record.CommandSeq)
	if err != nil || persistedRecord.Status != gameplayCommandRecordStatusPending || len(persistedRecord.OutboundMessages) != 0 {
		t.Fatalf("rolled back PostgreSQL command=%+v error=%v", persistedRecord, err)
	}
}
