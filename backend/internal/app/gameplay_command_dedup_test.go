package app

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

func marshalOutcomeJSON(t *testing.T, messages []map[string]any) string {
	t.Helper()

	bytes, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return string(bytes)
}

func countItemsByTemplate(items []CharacterItem, templateID string) int {
	total := 0
	for _, item := range items {
		if item.TemplateID == templateID {
			total += item.Quantity
		}
	}
	return total
}

func createDedupTestCharacter(t *testing.T, store *Store, characterID string) *Character {
	t.Helper()

	character := &Character{
		ID:           characterID,
		AccountID:    "acc_" + characterID,
		Name:         "Hero " + characterID,
		Race:         "Human",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
		IsEnterable:  true,
	}
	account := &Account{
		ID:          character.AccountID,
		Login:       character.AccountID + "@test",
		DisplayName: "Dedup " + characterID,
		State:       accountStateActive,
	}
	credential, err := newCredentialRecord(account.ID, "hunter123")
	if err != nil {
		t.Fatalf("newCredentialRecord() error = %v", err)
	}
	if err := store.CreateAccountWithCredential(context.Background(), account, credential); err != nil {
		t.Fatalf("CreateAccountWithCredential() error = %v", err)
	}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}
	return character
}

func createDedupTestSession(t *testing.T, store *Store, character *Character, sessionID string) *Session {
	t.Helper()

	session := &Session{
		ID:              sessionID,
		AccountID:       character.AccountID,
		CharacterID:     character.ID,
		AttachToken:     "attach_" + sessionID,
		AttachExpiresAt: time.Now().Add(5 * time.Minute),
		Status:          sessionStatusAttached,
	}
	if err := store.GameplaySessions.Create(context.Background(), session); err != nil {
		t.Fatalf("GameplaySessions.Create() error = %v", err)
	}
	return session
}

type scriptedMovementPlanner struct {
	version string
	resolve func(ctx context.Context, regionID string, start runtimePoint, destination runtimePoint, profile movementProfile) movementResolution
}

func (planner scriptedMovementPlanner) GeodataVersion(regionID string) string {
	if planner.version != "" {
		return planner.version
	}
	return "scripted_geo_v1"
}

func (planner scriptedMovementPlanner) Resolve(
	ctx context.Context,
	regionID string,
	start runtimePoint,
	destination runtimePoint,
	profile movementProfile,
) movementResolution {
	if planner.resolve == nil {
		return movementResolution{Status: movementPlanStatusCanceled}
	}
	return planner.resolve(ctx, regionID, start, destination, profile)
}

func TestProcessGameplayCommandWithDedupReplaysUseSkillWithoutDuplicatingSideEffects(t *testing.T) {
	store := newLootPickupTestStore(t)
	server := NewServer(":0", "", store)
	character := createDedupTestCharacter(t, store, "char_dedup_skill")
	session := createDedupTestSession(t, store, character, "sess_dedup_skill")
	runtime := newAttachedRuntime(session.ID, character)
	moveRuntimeNearMob(runtime, "mob_1")

	command := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_skill_dedup",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	}

	firstOutbound, firstFanOut := server.processGameplayCommandWithDedup(context.Background(), session, runtime, command)
	if !firstFanOut {
		t.Fatal("expected first use_skill to be treated as fresh command")
	}
	hpAfterFirst := runtime.knownEntities["mob_1"].State["hp"].(int)

	secondOutbound, secondFanOut := server.processGameplayCommandWithDedup(context.Background(), session, runtime, command)
	if secondFanOut {
		t.Fatal("expected replayed use_skill not to fan out again")
	}
	hpAfterReplay := runtime.knownEntities["mob_1"].State["hp"].(int)

	if hpAfterReplay != hpAfterFirst {
		t.Fatalf("expected replay not to apply extra damage, hp after first=%d after replay=%d", hpAfterFirst, hpAfterReplay)
	}
	if runtime.expectedCommandSeqValue() != 2 {
		t.Fatalf("expected replay not to advance sequence again, got %d", runtime.expectedCommandSeqValue())
	}
	if marshalOutcomeJSON(t, cloneOutboundMessages(firstOutbound)) != marshalOutcomeJSON(t, secondOutbound) {
		t.Fatalf("expected replay to reuse identical outbound outcome")
	}
}

func TestProcessAsyncMovementCommandWithDedupCancelsSupersededPathAndAppliesNewestMoveSerially(t *testing.T) {
	store := newLootPickupTestStore(t)
	server := NewServer(":0", "", store)
	character := createDedupTestCharacter(t, store, "char_async_move")
	session := createDedupTestSession(t, store, character, "sess_async_move")
	runtime := newAttachedRuntime(session.ID, character)

	firstStarted := make(chan struct{}, 1)
	firstRelease := make(chan struct{})
	runtime.movementPlanner = scriptedMovementPlanner{
		version: "async_geo_v1",
		resolve: func(ctx context.Context, regionID string, start runtimePoint, destination runtimePoint, profile movementProfile) movementResolution {
			select {
			case firstStarted <- struct{}{}:
			default:
			}
			select {
			case <-ctx.Done():
				return movementResolution{Status: movementPlanStatusCanceled}
			case <-firstRelease:
				return movementResolution{
					Status: movementPlanStatusAccepted,
					Plan: movementPlan{
						GeodataVersion:      "async_geo_v1",
						AcceptedDestination: destination,
						Waypoints:           []runtimePoint{destination},
					},
				}
			}
		},
	}

	var (
		messages []map[string]any
		mu       sync.Mutex
	)
	attached := &attachedSession{
		sessionID: session.ID,
		runtime:   runtime,
		send: func(payload map[string]any) bool {
			mu.Lock()
			messages = append(messages, cloneOutboundMessages([]map[string]any{payload})[0])
			mu.Unlock()
			return true
		},
		ready: true,
	}

	firstCommand := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_move_async_1",
		CommandSeq:      1,
		Type:            "move_intent",
		Payload:         []byte(`{"point":{"x":12,"z":6}}`),
	}
	if !attached.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
		return server.processAsyncMovementCommandWithDedup(context.Background(), session, attached, runtime, firstCommand)
	}) {
		t.Fatal("expected first async movement dispatch to succeed")
	}
	select {
	case <-firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("expected first movement path resolver to start")
	}

	mu.Lock()
	if len(messages) != 1 || messages[0]["kind"] != "ack" || messages[0]["command_id"] != firstCommand.CommandID {
		t.Fatalf("expected immediate ack for first move, got %+v", messages)
	}
	mu.Unlock()

	runtime.movementPlanner = scriptedMovementPlanner{
		version: "async_geo_v1",
		resolve: func(ctx context.Context, regionID string, start runtimePoint, destination runtimePoint, profile movementProfile) movementResolution {
			return movementResolution{
				Status: movementPlanStatusAccepted,
				Plan: movementPlan{
					GeodataVersion:      "async_geo_v1",
					AcceptedDestination: destination,
					Waypoints:           []runtimePoint{destination},
				},
			}
		},
	}

	secondCommand := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_move_async_2",
		CommandSeq:      2,
		Type:            "move_intent",
		Payload:         []byte(`{"point":{"x":6,"z":0}}`),
	}
	if !attached.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
		return server.processAsyncMovementCommandWithDedup(context.Background(), session, attached, runtime, secondCommand)
	}) {
		t.Fatal("expected second async movement dispatch to succeed")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		messageCount := len(messages)
		mu.Unlock()
		if messageCount >= 3 {
			break
		}
		if time.Now().After(deadline) {
			mu.Lock()
			snapshot := cloneOutboundMessages(messages)
			mu.Unlock()
			t.Fatalf("expected second movement to reach async delta, got %+v", snapshot)
		}
		time.Sleep(10 * time.Millisecond)
	}

	close(firstRelease)
	time.Sleep(30 * time.Millisecond)

	mu.Lock()
	outbound := cloneOutboundMessages(messages)
	mu.Unlock()
	if len(outbound) != 3 {
		t.Fatalf("expected ack, ack, delta after superseding move, got %+v", outbound)
	}
	if outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "ack" || outbound[2]["kind"] != "delta" {
		t.Fatalf("expected ack, ack, delta ordering, got %+v", outbound)
	}
	if outbound[2]["applies_to_command_id"] != secondCommand.CommandID {
		t.Fatalf("expected authoritative delta to belong to the latest move, got %+v", outbound[2])
	}

	firstRecord, err := store.GameplayCommands.GetBySessionAndSeq(context.Background(), session.ID, 1)
	if err != nil {
		t.Fatalf("GameplayCommands.GetBySessionAndSeq(first) error = %v", err)
	}
	if firstRecord.Status != gameplayCommandRecordStatusRejected {
		t.Fatalf("expected superseded first move to finalize as rejected, got %+v", firstRecord)
	}
	if !isAckOnlyOutbound(firstRecord.OutboundMessages) {
		t.Fatalf("expected superseded first move to retain ack-only outcome, got %+v", firstRecord.OutboundMessages)
	}

	secondRecord, err := store.GameplayCommands.GetBySessionAndSeq(context.Background(), session.ID, 2)
	if err != nil {
		t.Fatalf("GameplayCommands.GetBySessionAndSeq(second) error = %v", err)
	}
	if secondRecord.Status != gameplayCommandRecordStatusApplied {
		t.Fatalf("expected second move to finalize as applied, got %+v", secondRecord)
	}
}

func TestProcessGameplayCommandWithDedupRejectsConflictingReplay(t *testing.T) {
	store := newLootPickupTestStore(t)
	server := NewServer(":0", "", store)
	character := createDedupTestCharacter(t, store, "char_dedup_conflict")
	session := createDedupTestSession(t, store, character, "sess_dedup_conflict")
	runtime := newAttachedRuntime(session.ID, character)
	moveRuntimeNearMob(runtime, "mob_1")

	firstCommand := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_skill_first",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	}
	server.processGameplayCommandWithDedup(context.Background(), session, runtime, firstCommand)
	hpAfterFirst := runtime.knownEntities["mob_1"].State["hp"].(int)

	conflictOutbound, shouldFanOut := server.processGameplayCommandWithDedup(context.Background(), session, runtime, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_skill_conflict",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})
	if shouldFanOut {
		t.Fatal("expected conflicting replay not to fan out")
	}
	if len(conflictOutbound) != 1 || conflictOutbound[0]["reason_code"] != "sequence.conflicting_replay" {
		t.Fatalf("expected sequence.conflicting_replay, got %+v", conflictOutbound)
	}
	if runtime.knownEntities["mob_1"].State["hp"].(int) != hpAfterFirst {
		t.Fatalf("expected conflicting replay not to mutate runtime state")
	}
}

func TestProcessGameplayCommandWithDedupReplaysLootPickupWithoutDuplicatingInventory(t *testing.T) {
	store := newLootPickupTestStore(t)
	server := NewServer(":0", "", store)
	character := createDedupTestCharacter(t, store, "char_dedup_loot")
	session := createDedupTestSession(t, store, character, "sess_dedup_loot")
	runtime := newAttachedRuntime(session.ID, character)
	runtime.position = runtimePoint{X: 12, Z: 4}
	runtime.knownEntities["loot_retry"] = runtimeEntity{
		EntityID:   "loot_retry",
		EntityType: "loot",
		TemplateID: "healing_potion",
		Position:   runtime.position,
		State: map[string]any{
			"quantity": 1,
		},
	}

	command := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_loot_retry",
		CommandSeq:      1,
		Type:            "pick_up_loot",
		Payload:         []byte(`{"loot_id":"loot_retry"}`),
	}

	firstOutbound, _ := server.processGameplayCommandWithDedup(context.Background(), session, runtime, command)
	itemsAfterFirst, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}
	potionsAfterFirst := countItemsByTemplate(itemsAfterFirst, "healing_potion")

	secondOutbound, secondFanOut := server.processGameplayCommandWithDedup(context.Background(), session, runtime, command)
	if secondFanOut {
		t.Fatal("expected replayed loot pickup not to fan out again")
	}
	itemsAfterReplay, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}
	potionsAfterReplay := countItemsByTemplate(itemsAfterReplay, "healing_potion")

	if potionsAfterReplay != potionsAfterFirst {
		t.Fatalf("expected replay not to duplicate loot, first=%d replay=%d", potionsAfterFirst, potionsAfterReplay)
	}
	if marshalOutcomeJSON(t, cloneOutboundMessages(firstOutbound)) != marshalOutcomeJSON(t, secondOutbound) {
		t.Fatalf("expected replayed loot pickup to reuse identical outbound outcome")
	}
}

func TestProcessGameplayCommandWithDedupReplaysEquipItemWithoutDuplicatingMutation(t *testing.T) {
	store := newLootPickupTestStore(t)
	server := NewServer(":0", "", store)
	character := createDedupTestCharacter(t, store, "char_dedup_equip")
	session := createDedupTestSession(t, store, character, "sess_dedup_equip")
	runtime := newAttachedRuntime(session.ID, character)

	items, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}
	var chestItemID string
	for _, item := range items {
		if item.TemplateID == "wardkeeper_mantle" {
			chestItemID = item.ID
			break
		}
	}
	if chestItemID == "" {
		t.Fatal("expected starter chest item")
	}
	items, err = store.Items.UnequipItem(context.Background(), character.ID, equipSlotChest)
	if err != nil {
		t.Fatalf("Items.UnequipItem() error = %v", err)
	}
	runtime.derivedStats = deriveCharacterStats(character, items)

	command := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_equip_retry",
		CommandSeq:      1,
		Type:            "equip_item",
		Payload:         []byte(`{"item_instance_id":"` + chestItemID + `"}`),
	}

	firstOutbound, _ := server.processGameplayCommandWithDedup(context.Background(), session, runtime, command)
	itemsAfterFirst, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}
	firstStats := runtime.derivedStats

	secondOutbound, secondFanOut := server.processGameplayCommandWithDedup(context.Background(), session, runtime, command)
	if secondFanOut {
		t.Fatal("expected replayed equip_item not to fan out again")
	}
	itemsAfterReplay, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}

	if marshalOutcomeJSON(t, cloneOutboundMessages(firstOutbound)) != marshalOutcomeJSON(t, secondOutbound) {
		t.Fatalf("expected replayed equip_item to reuse identical outbound outcome")
	}
	if marshalOutcomeJSON(t, []map[string]any{{"items": itemsAfterFirst}}) != marshalOutcomeJSON(t, []map[string]any{{"items": itemsAfterReplay}}) {
		t.Fatalf("expected replay not to mutate inventory placement again")
	}
	if runtime.derivedStats != firstStats {
		t.Fatalf("expected replay not to mutate derived stats again")
	}
}

func TestProcessGameplayCommandWithDedupReplaysUnequipItemWithoutDuplicatingMutation(t *testing.T) {
	store := newLootPickupTestStore(t)
	server := NewServer(":0", "", store)
	character := createDedupTestCharacter(t, store, "char_dedup_unequip")
	session := createDedupTestSession(t, store, character, "sess_dedup_unequip")
	runtime := newAttachedRuntime(session.ID, character)

	items, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}
	var chestItemID string
	for _, item := range items {
		if item.TemplateID == "wardkeeper_mantle" {
			chestItemID = item.ID
			break
		}
	}
	if chestItemID == "" {
		t.Fatal("expected starter chest item")
	}
	runtime.derivedStats = deriveCharacterStats(character, items)

	command := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_unequip_retry",
		CommandSeq:      1,
		Type:            "unequip_item",
		Payload:         []byte(`{"equip_slot":"chest"}`),
	}

	firstOutbound, _ := server.processGameplayCommandWithDedup(context.Background(), session, runtime, command)
	itemsAfterFirst, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}
	firstStats := runtime.derivedStats

	secondOutbound, secondFanOut := server.processGameplayCommandWithDedup(context.Background(), session, runtime, command)
	if secondFanOut {
		t.Fatal("expected replayed unequip_item not to fan out again")
	}
	itemsAfterReplay, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}

	if marshalOutcomeJSON(t, cloneOutboundMessages(firstOutbound)) != marshalOutcomeJSON(t, secondOutbound) {
		t.Fatalf("expected replayed unequip_item to reuse identical outbound outcome")
	}
	if marshalOutcomeJSON(t, []map[string]any{{"items": itemsAfterFirst}}) != marshalOutcomeJSON(t, []map[string]any{{"items": itemsAfterReplay}}) {
		t.Fatalf("expected replay not to mutate unequip placement again")
	}
	if runtime.derivedStats != firstStats {
		t.Fatalf("expected replay not to mutate derived stats again")
	}
}

func TestProcessGameplayCommandWithDedupReplaysHotbarStateWithoutDiverging(t *testing.T) {
	store := newLootPickupTestStore(t)
	server := NewServer(":0", "", store)
	character := createDedupTestCharacter(t, store, "char_dedup_hotbar")
	session := createDedupTestSession(t, store, character, "sess_dedup_hotbar")
	runtime := newAttachedRuntime(session.ID, character)
	items, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}

	itemID := ""
	for _, item := range items {
		if item.TemplateID == "duskgold" {
			itemID = item.ID
			break
		}
	}
	if itemID == "" {
		t.Fatalf("expected seeded duskgold item, got %+v", items)
	}

	command := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_hotbar_dedup",
		CommandSeq:      1,
		Type:            "set_hotbar_state",
		Payload: hotbarSnapshotPayload(t, 2,
			CharacterHotbarSlot{SlotIndex: 0, EntryType: "action", ActionID: "pick_up_nearby"},
			CharacterHotbarSlot{SlotIndex: 1, EntryType: "item", ItemInstanceID: itemID},
		),
	}

	firstOutbound, firstFanOut := server.processGameplayCommandWithDedup(context.Background(), session, runtime, command)
	if !firstFanOut {
		t.Fatal("expected first set_hotbar_state to be treated as fresh command")
	}
	firstHotbar, err := store.CharacterHotbars.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("CharacterHotbars.ListByCharacterID() error = %v", err)
	}

	secondOutbound, secondFanOut := server.processGameplayCommandWithDedup(context.Background(), session, runtime, command)
	if secondFanOut {
		t.Fatal("expected replayed set_hotbar_state not to fan out again")
	}
	secondHotbar, err := store.CharacterHotbars.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("CharacterHotbars.ListByCharacterID() replay error = %v", err)
	}

	if marshalOutcomeJSON(t, cloneOutboundMessages(firstOutbound)) != marshalOutcomeJSON(t, secondOutbound) {
		t.Fatalf("expected replayed set_hotbar_state to reuse identical outbound outcome")
	}
	if marshalOutcomeJSON(t, []map[string]any{{"hotbar": firstHotbar}}) != marshalOutcomeJSON(t, []map[string]any{{"hotbar": secondHotbar}}) {
		t.Fatalf("expected hotbar replay not to change persisted state")
	}
	if runtime.expectedCommandSeqValue() != 2 {
		t.Fatalf("expected replay not to advance sequence again, got %d", runtime.expectedCommandSeqValue())
	}
}

func TestGameplayCommandDedupPersistsAcrossStoreRestart(t *testing.T) {
	databaseURL := os.Getenv("L2BG_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("L2BG_TEST_DATABASE_URL not configured")
	}

	env := newPersistenceTestEnv(t)
	_, accessToken := registerAndLogin(t, env, "persist.command-dedup@test")
	characterResponse := postJSON(t, env.httpServer.Client(), env.httpServer.URL+"/v1/characters", map[string]any{
		"race":       "Human",
		"base_class": "Fighter",
		"sex":        "Male",
		"hair_style": 1,
		"hair_color": "#6b4e37",
		"skin_type":  2,
		"name":       "Dedup Persist",
	}, accessToken)
	if characterResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create character status = %d", characterResponse.StatusCode)
	}
	characterPayload := decodeBody[map[string]any](t, characterResponse)
	characterID := characterPayload["character"].(map[string]any)["character_id"].(string)
	character, err := env.store.Characters.GetByID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Characters.GetByID() error = %v", err)
	}
	session := createDedupTestSession(t, env.store, character, "sess_persist_dedup")
	runtime := newAttachedRuntime(session.ID, character)
	moveRuntimeNearMob(runtime, "mob_1")

	command := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_persist_dedup",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	}

	firstOutbound, _ := env.server.processGameplayCommandWithDedup(context.Background(), session, runtime, command)
	hpAfterFirst := runtime.knownEntities["mob_1"].State["hp"].(int)

	restartedStore, err := NewStore(databaseURL)
	if err != nil {
		t.Fatalf("NewStore(restart) error = %v", err)
	}
	defer restartedStore.Close()
	restartedServer := NewServer(":0", "ws://example.test/v1/gameplay/ws", restartedStore)
	restartedCharacter, err := restartedStore.Characters.GetByID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("restarted Characters.GetByID() error = %v", err)
	}
	restartedRuntime := newAttachedRuntime(session.ID, restartedCharacter)
	moveRuntimeNearMob(restartedRuntime, "mob_1")
	hpBeforeReplay := restartedRuntime.knownEntities["mob_1"].State["hp"].(int)

	replayOutbound, replayFanOut := restartedServer.processGameplayCommandWithDedup(context.Background(), session, restartedRuntime, command)
	if replayFanOut {
		t.Fatal("expected persisted replay not to fan out again")
	}
	if marshalOutcomeJSON(t, cloneOutboundMessages(firstOutbound)) != marshalOutcomeJSON(t, replayOutbound) {
		t.Fatalf("expected restarted replay to reuse persisted outbound outcome")
	}
	if restartedRuntime.knownEntities["mob_1"].State["hp"].(int) != hpBeforeReplay {
		t.Fatalf("expected persisted replay not to reapply damage after restart")
	}
	if hpAfterFirst >= hpBeforeReplay {
		t.Fatalf("expected first command to apply damage before restart")
	}
}

func TestProcessGameplayCommandWithDedupReplaysTameWithoutDuplicatingOwnership(t *testing.T) {
	store := newLootPickupTestStore(t)
	server := NewServer(":0", "", store)
	character := createDedupTestCharacter(t, store, "char_dedup_pet")
	session := createDedupTestSession(t, store, character, "sess_dedup_pet")
	runtime := newAttachedRuntime(session.ID, character)
	moveRuntimeNearMob(runtime, "mob_1")

	command := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_pet_dedup",
		CommandSeq:      1,
		Type:            "tame_mob",
		Payload:         []byte(`{"target_id":"mob_1"}`),
	}

	firstOutbound, firstFanOut := server.processGameplayCommandWithDedup(context.Background(), session, runtime, command)
	if !firstFanOut {
		t.Fatal("expected first tame_mob to be treated as a fresh command")
	}
	if len(runtime.pets) != 1 {
		t.Fatalf("expected one runtime pet after first tame, got %+v", runtime.pets)
	}

	secondOutbound, secondFanOut := server.processGameplayCommandWithDedup(context.Background(), session, runtime, command)
	if secondFanOut {
		t.Fatal("expected replayed tame_mob not to fan out again")
	}
	if len(runtime.pets) != 1 {
		t.Fatalf("expected replay not to duplicate runtime pet ownership, got %+v", runtime.pets)
	}

	persistedPets, err := store.CharacterPets.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("CharacterPets.ListByCharacterID() error = %v", err)
	}
	if len(persistedPets) != 1 {
		t.Fatalf("expected one persisted pet after replay, got %+v", persistedPets)
	}
	if runtime.expectedCommandSeqValue() != 2 {
		t.Fatalf("expected replay not to advance sequence again, got %d", runtime.expectedCommandSeqValue())
	}
	if marshalOutcomeJSON(t, cloneOutboundMessages(firstOutbound)) != marshalOutcomeJSON(t, secondOutbound) {
		t.Fatalf("expected replayed tame_mob to reuse identical outbound outcome")
	}
}
