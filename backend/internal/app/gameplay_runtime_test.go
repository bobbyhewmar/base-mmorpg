package app

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func moveRuntimeNearMob(runtime *attachedRuntime, mobID string) {
	entity := runtime.knownEntities[mobID]
	runtime.position = entity.Position
}

func testRuntimeMob(entityID string, templateID string, personality mobPersonality, position runtimePoint, maxHP int) runtimeEntity {
	return runtimeEntity{
		EntityID:   entityID,
		EntityType: "mob",
		TemplateID: templateID,
		Position:   position,
		State: map[string]any{
			"hp":          maxHP,
			"max_hp":      maxHP,
			"level":       1,
			"alive":       true,
			"personality": string(personality),
			"ai_state":    string(mobAIStateIdle),
			"spawn_x":     position.X,
			"spawn_z":     position.Z,
		},
	}
}

func newLootPickupTestStore(t *testing.T) *Store {
	t.Helper()

	store, err := NewStore("")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func deltaSelfStats(t *testing.T, message map[string]any) CharacterDerivedStats {
	t.Helper()

	self, ok := message["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", message["self"])
	}
	stats, ok := self["stats"].(CharacterDerivedStats)
	if !ok {
		t.Fatalf("expected delta self stats, got %+v", self["stats"])
	}
	return stats
}

func deltaSelfMovementMode(t *testing.T, message map[string]any) string {
	t.Helper()

	self, ok := message["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", message["self"])
	}
	movementMode, ok := self["movement_mode"].(string)
	if !ok {
		t.Fatalf("expected delta self movement_mode, got %+v", self["movement_mode"])
	}
	return movementMode
}

func deltaSelfHP(t *testing.T, message map[string]any) int {
	t.Helper()

	self, ok := message["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", message["self"])
	}
	hp, ok := self["hp"].(int)
	if !ok {
		t.Fatalf("expected delta self hp, got %+v", self["hp"])
	}
	return hp
}

func deltaSelfCP(t *testing.T, message map[string]any) int {
	t.Helper()

	self, ok := message["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", message["self"])
	}
	cp, ok := self["cp"].(int)
	if !ok {
		t.Fatalf("expected delta self cp, got %+v", self["cp"])
	}
	return cp
}

func deltaSelfMP(t *testing.T, message map[string]any) int {
	t.Helper()

	self, ok := message["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", message["self"])
	}
	mp, ok := self["mp"].(int)
	if !ok {
		t.Fatalf("expected delta self mp, got %+v", self["mp"])
	}
	return mp
}

func deltaSelfQuest(t *testing.T, message map[string]any) CharacterQuestSnapshot {
	t.Helper()

	self, ok := message["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", message["self"])
	}
	quest, ok := self["quest"].(CharacterQuestSnapshot)
	if !ok {
		t.Fatalf("expected delta self quest, got %+v", self["quest"])
	}
	return quest
}

func deltaSelfNPCInteraction(t *testing.T, message map[string]any) *CharacterNPCInteraction {
	t.Helper()

	self, ok := message["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", message["self"])
	}
	if self["npc_interaction"] == nil {
		return nil
	}
	interaction, ok := self["npc_interaction"].(*CharacterNPCInteraction)
	if !ok {
		t.Fatalf("expected delta self npc interaction, got %+v", self["npc_interaction"])
	}
	return interaction
}

func deltaSelfXP(t *testing.T, message map[string]any) int {
	t.Helper()

	self, ok := message["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", message["self"])
	}
	xp, ok := self["xp"].(int)
	if !ok {
		t.Fatalf("expected delta self xp, got %+v", self["xp"])
	}
	return xp
}

func deltaSelfLevel(t *testing.T, message map[string]any) int {
	t.Helper()

	self, ok := message["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", message["self"])
	}
	level, ok := self["level"].(int)
	if !ok {
		t.Fatalf("expected delta self level, got %+v", self["level"])
	}
	return level
}

func deltaSelfDead(t *testing.T, message map[string]any) bool {
	t.Helper()

	self, ok := message["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", message["self"])
	}
	dead, ok := self["dead"].(bool)
	if !ok {
		t.Fatalf("expected delta self dead flag, got %+v", self["dead"])
	}
	return dead
}

func deltaInventory(t *testing.T, message map[string]any) []CharacterItem {
	t.Helper()

	inventory, ok := message["inventory"].([]CharacterItem)
	if !ok {
		t.Fatalf("expected delta inventory payload, got %+v", message["inventory"])
	}
	return inventory
}

func deltaWarehouse(t *testing.T, message map[string]any) []CharacterItem {
	t.Helper()

	warehouse, ok := message["warehouse"].([]CharacterItem)
	if !ok {
		t.Fatalf("expected delta warehouse payload, got %+v", message["warehouse"])
	}
	return warehouse
}

func deltaSelfPosition(t *testing.T, message map[string]any) runtimePoint {
	t.Helper()

	self, ok := message["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", message["self"])
	}
	position, ok := self["position"].(runtimePoint)
	if !ok {
		t.Fatalf("expected delta self position, got %+v", self["position"])
	}
	return position
}

func deltaSelfPets(t *testing.T, message map[string]any) []CharacterPetSnapshot {
	t.Helper()

	self, ok := message["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", message["self"])
	}
	pets, ok := self["pets"].([]CharacterPetSnapshot)
	if !ok {
		t.Fatalf("expected delta self pets, got %+v", self["pets"])
	}
	return pets
}

func deltaSelfAuthoritativePath(t *testing.T, message map[string]any) []runtimePoint {
	t.Helper()

	self, ok := message["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", message["self"])
	}
	path, ok := self["authoritative_path"].([]runtimePoint)
	if !ok {
		t.Fatalf("expected authoritative_path in delta self payload, got %+v", self["authoritative_path"])
	}
	return path
}

func hotbarSnapshotPayload(t *testing.T, openBarCount int, overrides ...CharacterHotbarSlot) []byte {
	t.Helper()

	slots := make([]map[string]any, 36)
	for slotIndex := range slots {
		slots[slotIndex] = map[string]any{
			"slot_index": slotIndex,
		}
	}
	for _, override := range overrides {
		if override.SlotIndex < 0 || override.SlotIndex >= len(slots) {
			t.Fatalf("invalid test hotbar slot index %d", override.SlotIndex)
		}
		slot := map[string]any{
			"slot_index": override.SlotIndex,
		}
		if override.EntryType != "" {
			slot["entry_type"] = override.EntryType
		}
		if override.SkillID != "" {
			slot["skill_id"] = override.SkillID
		}
		if override.ItemInstanceID != "" {
			slot["item_instance_id"] = override.ItemInstanceID
		}
		if override.ActionID != "" {
			slot["action_id"] = override.ActionID
		}
		slots[override.SlotIndex] = slot
	}
	payload, err := json.Marshal(map[string]any{
		"open_bar_count": openBarCount,
		"slots":          slots,
	})
	if err != nil {
		t.Fatalf("json.Marshal hotbar payload error = %v", err)
	}
	return payload
}

func TestAttachedRuntimeUsesPersistedCharacterWorldState(t *testing.T) {
	runtime := newAttachedRuntime("sess_1", &Character{
		ID:           "char_1",
		LastRegionID: "west_field",
		PositionX:    21,
		PositionZ:    -3,
	})

	regionID, position := runtime.characterWorldState()
	if regionID != "west_field" {
		t.Fatalf("expected persisted region_id west_field, got %s", regionID)
	}
	if position.X != 21 || position.Z != -3 {
		t.Fatalf("expected persisted position (21,-3), got %+v", position)
	}
}

func TestAttachedRuntimeRejectsOutOfOrderSequence(t *testing.T) {
	runtime := newAttachedRuntime("sess_1", &Character{ID: "char_1", LastRegionID: "dawn_plaza"})

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      2,
		Type:            "move_intent",
		Payload:         []byte(`{"point":{"x":10,"z":4}}`),
	})

	if len(outbound) != 1 {
		t.Fatalf("expected a single reject message, got %d", len(outbound))
	}
	if outbound[0]["kind"] != "reject" {
		t.Fatalf("expected reject, got %v", outbound[0]["kind"])
	}
	if outbound[0]["reason_code"] != "sequence.out_of_order" {
		t.Fatalf("expected sequence.out_of_order, got %v", outbound[0]["reason_code"])
	}
}

func TestAttachedRuntimeRejectsTargetOutsideKnownSet(t *testing.T) {
	runtime := newAttachedRuntime("sess_1", &Character{ID: "char_1", LastRegionID: "dawn_plaza"})

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "select_target",
		Payload:         []byte(`{"target_id":"mob_missing"}`),
	})

	if len(outbound) != 2 {
		t.Fatalf("expected ack and reject, got %d messages", len(outbound))
	}
	if outbound[0]["kind"] != "ack" {
		t.Fatalf("expected first message to be ack, got %v", outbound[0]["kind"])
	}
	if outbound[1]["kind"] != "reject" {
		t.Fatalf("expected second message to be reject, got %v", outbound[1]["kind"])
	}
	if outbound[1]["reason_code"] != "world.entity_not_known" {
		t.Fatalf("expected world.entity_not_known, got %v", outbound[1]["reason_code"])
	}
}

func TestAttachedRuntimeSelectsKnownPlayerAuthoritatively(t *testing.T) {
	runtime := newAttachedRuntime("sess_1", &Character{ID: "char_1", LastRegionID: "dawn_plaza"})
	runtime.knownEntities["char_2"] = runtimeEntity{
		EntityID:   "char_2",
		EntityType: "player",
		TemplateID: "player_character",
		State:      map[string]any{"name": "Selene", "dead": true},
	}

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_select_player",
		CommandSeq:      1,
		Type:            "select_target",
		Payload:         []byte(`{"target_id":"char_2"}`),
	})

	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and authoritative delta, got %+v", outbound)
	}
	if runtime.targetID != "char_2" {
		t.Fatalf("expected authoritative player target char_2, got %q", runtime.targetID)
	}
	self, ok := outbound[1]["self"].(map[string]any)
	if !ok || self["target_id"] != "char_2" {
		t.Fatalf("expected correlated target_id in delta, got %+v", outbound[1])
	}
}

func TestAttachedRuntimeEarlyRejectsInvalidPayloadWithoutAck(t *testing.T) {
	runtime := newAttachedRuntime("sess_1", &Character{ID: "char_1", LastRegionID: "dawn_plaza"})

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "move_intent",
		Payload:         []byte(`{"point":"invalid"}`),
	})

	if len(outbound) != 1 {
		t.Fatalf("expected a single reject message, got %d", len(outbound))
	}
	if outbound[0]["kind"] != "reject" {
		t.Fatalf("expected reject, got %v", outbound[0]["kind"])
	}
	if outbound[0]["reason_code"] != "protocol.invalid_envelope" {
		t.Fatalf("expected protocol.invalid_envelope, got %v", outbound[0]["reason_code"])
	}
	if runtime.expectedCommandSeq != 1 {
		t.Fatalf("expected command sequence to remain at 1, got %d", runtime.expectedCommandSeq)
	}
}

func TestAttachedRuntimeRejectsClientSuppliedMovementWaypointsWithoutAck(t *testing.T) {
	runtime := newAttachedRuntime("sess_move_shape", &Character{ID: "char_move_shape", LastRegionID: "dawn_plaza"})

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_move_shape",
		CommandSeq:      1,
		Type:            "move_intent",
		Payload:         []byte(`{"point":{"x":12,"z":4},"waypoints":[{"x":0,"z":0}]}`),
	})

	if len(outbound) != 1 {
		t.Fatalf("expected a single reject message, got %d", len(outbound))
	}
	if outbound[0]["kind"] != "reject" {
		t.Fatalf("expected reject, got %v", outbound[0]["kind"])
	}
	if outbound[0]["reason_code"] != "protocol.invalid_envelope" {
		t.Fatalf("expected protocol.invalid_envelope, got %v", outbound[0]["reason_code"])
	}
	if runtime.expectedCommandSeq != 1 {
		t.Fatalf("expected command sequence to remain at 1, got %d", runtime.expectedCommandSeq)
	}
}

func TestAttachedRuntimeMoveIntentPublishesAuthoritativePathAndAdvancesOnTick(t *testing.T) {
	runtime := newAttachedRuntime("sess_move_path", &Character{
		ID:           "char_move_path",
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_move_path",
		CommandSeq:      1,
		Type:            "move_intent",
		Payload:         []byte(`{"point":{"x":16,"z":-14}}`),
	})

	if len(outbound) != 2 {
		t.Fatalf("expected ack and delta, got %+v", outbound)
	}
	if outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta, got %+v", outbound)
	}

	initialPath := deltaSelfAuthoritativePath(t, outbound[1])
	if len(initialPath) < 2 {
		t.Fatalf("expected an authoritative path, got %+v", initialPath)
	}
	if initialPath[0] != (runtimePoint{X: -8, Z: 0}) {
		t.Fatalf("expected path to start at current authoritative position, got %+v", initialPath[0])
	}
	if initialPath[len(initialPath)-1] != (runtimePoint{X: 16, Z: -14}) {
		t.Fatalf("expected path to end at requested destination, got %+v", initialPath[len(initialPath)-1])
	}

	tickMessages, movementChanged, _ := runtime.collectTickMessages(time.Now().Add(10 * time.Second))
	if !movementChanged {
		t.Fatalf("expected runtime movement to advance on tick")
	}
	if len(tickMessages) == 0 || tickMessages[0]["kind"] != "delta" {
		t.Fatalf("expected tick to emit movement delta, got %+v", tickMessages)
	}
	if deltaSelfPosition(t, tickMessages[0]) != (runtimePoint{X: 16, Z: -14}) {
		t.Fatalf("expected movement tick to settle on destination, got %+v", deltaSelfPosition(t, tickMessages[0]))
	}
	if path := deltaSelfAuthoritativePath(t, tickMessages[0]); len(path) != 0 {
		t.Fatalf("expected authoritative path to clear after arrival, got %+v", path)
	}
}

func TestAttachedRuntimeToggleWalkRunPublishesEffectiveSpeedAndPreservesMovementPath(t *testing.T) {
	runtime := newAttachedRuntime("sess_walk_toggle", &Character{
		ID:           "char_walk_toggle",
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})
	now := time.Now()
	runtime.setActiveMovementLocked(movementPlan{
		GeodataVersion:      "test_geo_v1",
		AcceptedDestination: runtimePoint{X: 12, Z: 0},
		Waypoints:           []runtimePoint{{X: 12, Z: 0}},
	}, now)

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_walk_toggle",
		CommandSeq:      1,
		Type:            "toggle_walk_run",
		Payload:         []byte(`{}`),
	})

	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta, got %+v", outbound)
	}
	if movementMode := deltaSelfMovementMode(t, outbound[1]); movementMode != "walk" {
		t.Fatalf("expected movement mode walk, got %s", movementMode)
	}
	stats := deltaSelfStats(t, outbound[1])
	if diff := stats.MoveSpeed - (runtime.movementRunSpeed * movementWalkSpeedRatio); diff < -0.0001 || diff > 0.0001 {
		t.Fatalf("expected walk speed %.3f, got %.3f", runtime.movementRunSpeed*movementWalkSpeedRatio, stats.MoveSpeed)
	}
	if path := deltaSelfAuthoritativePath(t, outbound[1]); len(path) != 2 {
		t.Fatalf("expected active movement path to remain authoritative after toggle, got %+v", path)
	}
	if runtime.activeMovement == nil {
		t.Fatal("expected active movement to remain in place after toggling movement mode")
	}
}

func TestAttachedRuntimeWalkMovementAdvancesAtOneThirdRunSpeed(t *testing.T) {
	newStraightRuntime := func(sessionID string) *attachedRuntime {
		runtime := newAttachedRuntime(sessionID, &Character{
			ID:           sessionID,
			LastRegionID: "dawn_plaza",
			PositionX:    0,
			PositionZ:    0,
		})
		return runtime
	}

	runRuntime := newStraightRuntime("sess_run_speed")
	walkRuntime := newStraightRuntime("sess_walk_speed")
	walkToggle := walkRuntime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_walk_mode",
		CommandSeq:      1,
		Type:            "toggle_walk_run",
		Payload:         []byte(`{}`),
	})
	if len(walkToggle) != 2 || walkToggle[1]["kind"] != "delta" {
		t.Fatalf("expected walk toggle delta, got %+v", walkToggle)
	}

	now := time.Now()
	plan := movementPlan{
		GeodataVersion:      "test_geo_v1",
		AcceptedDestination: runtimePoint{X: 12, Z: 0},
		Waypoints:           []runtimePoint{{X: 12, Z: 0}},
	}
	runRuntime.setActiveMovementLocked(plan, now)
	walkRuntime.setActiveMovementLocked(plan, now)

	if !runRuntime.advanceMovementLocked(now.Add(time.Second)) {
		t.Fatal("expected run movement to advance after one second")
	}
	if !walkRuntime.advanceMovementLocked(now.Add(time.Second)) {
		t.Fatal("expected walk movement to advance after one second")
	}
	if diff := walkRuntime.position.X - (runRuntime.position.X * movementWalkSpeedRatio); diff < -0.0001 || diff > 0.0001 {
		t.Fatalf("expected walk distance %.3f, got %.3f", runRuntime.position.X*movementWalkSpeedRatio, walkRuntime.position.X)
	}
	if walkRuntime.position.X >= runRuntime.position.X {
		t.Fatalf("expected walk movement to remain slower than run, got walk=%.3f run=%.3f", walkRuntime.position.X, runRuntime.position.X)
	}
	if walkRuntime.activeMovement == nil || runRuntime.activeMovement == nil {
		t.Fatalf("expected both runtimes to preserve active movement after partial advancement, walk=%+v run=%+v", walkRuntime.activeMovement, runRuntime.activeMovement)
	}
}

func TestAttachedRuntimeRegeneratesPoolsAfterStandingStill(t *testing.T) {
	runtime := newAttachedRuntime("sess_idle_regen_1", &Character{ID: "char_idle_regen_1", LastRegionID: "dawn_plaza"})
	now := time.Now()
	runtime.currentCP = 0
	runtime.currentHP = 40
	runtime.currentMP = 10
	runtime.stationarySince = now
	runtime.lastIdleRegenAt = now

	outbound, movementChanged, respawned := runtime.collectTickMessages(now.Add(5 * time.Second))
	if len(outbound) != 0 || movementChanged || respawned {
		t.Fatalf("expected no regen exactly at 5 seconds, got outbound=%+v movement=%v respawned=%v", outbound, movementChanged, respawned)
	}

	outbound, movementChanged, respawned = runtime.collectTickMessages(now.Add(6 * time.Second))
	if movementChanged || respawned {
		t.Fatalf("expected idle regen without movement or respawn, movement=%v respawned=%v", movementChanged, respawned)
	}
	if len(outbound) != 1 || outbound[0]["kind"] != "delta" {
		t.Fatalf("expected one idle regen delta, got %+v", outbound)
	}
	if cp := deltaSelfCP(t, outbound[0]); cp != 3 {
		t.Fatalf("expected cp to regenerate to 3, got %d", cp)
	}
	if hp := deltaSelfHP(t, outbound[0]); hp != 44 {
		t.Fatalf("expected hp to regenerate to 44, got %d", hp)
	}
	if mp := deltaSelfMP(t, outbound[0]); mp != 12 {
		t.Fatalf("expected mp to regenerate to 12, got %d", mp)
	}
}

func TestAttachedRuntimeRejectsUnreachableMoveIntentWithCorrection(t *testing.T) {
	planner := &staticRegionMovementPlanner{
		regions: map[string]regionGeodata{
			"sealed_field": {
				RegionID:   "sealed_field",
				Version:    "sealed_field_geo_v1",
				Bounds:     movementBounds{MinX: -6, MaxX: 6, MinZ: -3, MaxZ: 3},
				CellSize:   1,
				PathBudget: 200,
				Obstacles: []movementObstacle{
					rectObstacle(-0.5, 0.5, -3, 3),
				},
			},
		},
	}
	runtime := newAttachedRuntime("sess_move_reject", &Character{
		ID:           "char_move_reject",
		LastRegionID: "sealed_field",
		PositionX:    -4,
		PositionZ:    0,
	})
	runtime.movementPlanner = planner

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_move_reject",
		CommandSeq:      1,
		Type:            "move_intent",
		Payload:         []byte(`{"point":{"x":4,"z":0}}`),
	})

	if len(outbound) != 3 {
		t.Fatalf("expected ack, reject, and correction, got %+v", outbound)
	}
	if outbound[1]["kind"] != "reject" || outbound[1]["reason_code"] != "movement.path_unreachable" {
		t.Fatalf("expected movement.path_unreachable reject, got %+v", outbound[1])
	}
	if outbound[2]["kind"] != "position_correction" {
		t.Fatalf("expected position_correction after reject, got %+v", outbound[2])
	}
	if outbound[2]["reason"] != "path_unreachable" {
		t.Fatalf("expected correction reason path_unreachable, got %+v", outbound[2]["reason"])
	}
}

func TestAttachedRuntimeEarlyRejectsUnsupportedCommandWithoutAdvancingSequence(t *testing.T) {
	runtime := newAttachedRuntime("sess_1", &Character{ID: "char_1", LastRegionID: "dawn_plaza"})

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "dance",
		Payload:         []byte(`{"emote":"moonwalk"}`),
	})

	if len(outbound) != 1 {
		t.Fatalf("expected a single reject message, got %d", len(outbound))
	}
	if outbound[0]["kind"] != "reject" {
		t.Fatalf("expected reject, got %v", outbound[0]["kind"])
	}
	if runtime.expectedCommandSeq != 1 {
		t.Fatalf("expected command sequence to remain at 1, got %d", runtime.expectedCommandSeq)
	}

	validOutbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_2",
		CommandSeq:      1,
		Type:            "move_intent",
		Payload:         []byte(`{"point":{"x":10,"z":4}}`),
	})
	if len(validOutbound) < 2 || validOutbound[0]["kind"] != "ack" || validOutbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta after early reject preserved sequence, got %+v", validOutbound)
	}
}

func TestAttachedRuntimeRejectsUnknownSkill(t *testing.T) {
	runtime := newAttachedRuntime("sess_1", &Character{ID: "char_1", LastRegionID: "dawn_plaza"})

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"unknown_skill","target_id":"mob_1"}`),
	})

	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["reason_code"] != "combat.skill_unknown" {
		t.Fatalf("expected ack followed by combat.skill_unknown reject, got %+v", outbound)
	}
}

func TestAttachedRuntimeRejectsSkillNotLearnedYet(t *testing.T) {
	runtime := newAttachedRuntime("sess_locked_skill", &Character{
		ID:           "char_locked_skill",
		BaseClass:    "Fighter",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})
	moveRuntimeNearMob(runtime, "mob_1")

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_locked_skill",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"grave_bloom","target_id":"mob_1"}`),
	})

	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["reason_code"] != "combat.skill_not_learned" {
		t.Fatalf("expected ack followed by combat.skill_not_learned reject, got %+v", outbound)
	}
}

func TestAttachedRuntimeRejectsPassiveSkillActivation(t *testing.T) {
	runtime := newAttachedRuntime("sess_passive_skill", &Character{
		ID:           "char_passive_skill",
		BaseClass:    "Fighter",
		Level:        1,
		LastRegionID: "dawn_plaza",
	})
	moveRuntimeNearMob(runtime, "mob_1")

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_passive_skill",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"iron_will","target_id":"mob_1"}`),
	})

	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["reason_code"] != "combat.skill_not_active" {
		t.Fatalf("expected ack followed by combat.skill_not_active reject, got %+v", outbound)
	}
}

func TestAttachedRuntimeRejectsTargetRequiredWhenMissing(t *testing.T) {
	runtime := newAttachedRuntime("sess_1", &Character{ID: "char_1", LastRegionID: "dawn_plaza"})

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike"}`),
	})

	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["reason_code"] != "combat.target_required" {
		t.Fatalf("expected ack followed by combat.target_required reject, got %+v", outbound)
	}
}

func TestAttachedRuntimeBasicAttackAppliesPhysicalDamage(t *testing.T) {
	runtime := newAttachedRuntime("sess_basic_attack", &Character{ID: "char_basic_attack", LastRegionID: "dawn_plaza"})
	moveRuntimeNearMob(runtime, "mob_1")

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_basic_attack",
		CommandSeq:      1,
		Type:            "basic_attack",
		Payload:         []byte(`{"target_id":"mob_1"}`),
	})

	if len(outbound) < 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected basic attack to apply with ack and delta, got %+v", outbound)
	}
	mob := runtime.knownEntities["mob_1"]
	hp, _ := mob.State["hp"].(int)
	if hp >= 54 {
		t.Fatalf("expected basic attack to damage mob, got hp=%d", hp)
	}
	if runtime.cooldownEndsAt["basic_attack"].IsZero() {
		t.Fatal("expected basic attack cooldown to be tracked")
	}
	if runtime.autoBasicAttack == nil || runtime.autoBasicAttack.TargetID != "mob_1" {
		t.Fatalf("expected basic attack to enter server-owned auto attack, got %+v", runtime.autoBasicAttack)
	}
}

func TestAttachedRuntimeBasicAttackAutoRepeatsUntilTargetDies(t *testing.T) {
	runtime := newAttachedRuntime("sess_basic_attack_auto", &Character{ID: "char_basic_attack_auto", LastRegionID: "dawn_plaza"})
	moveRuntimeNearMob(runtime, "mob_1")
	target := runtime.knownEntities["mob_1"]
	target.State["hp"] = 20
	runtime.knownEntities["mob_1"] = target

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_basic_attack_auto",
		CommandSeq:      1,
		Type:            "basic_attack",
		Payload:         []byte(`{"target_id":"mob_1"}`),
	})
	if len(outbound) < 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected initial basic attack to apply with ack and delta, got %+v", outbound)
	}

	attackDeltas := 1
	messages, _, _ := runtime.collectTickMessages(runtime.cooldownEndsAt["basic_attack"].Add(10 * time.Millisecond))
	for _, message := range messages {
		if message["kind"] == "delta" && message["applies_to_command_id"] == "cmd_basic_attack_auto" {
			attackDeltas++
		}
	}

	mob := runtime.knownEntities["mob_1"]
	if isRuntimeEntityAlive(mob) {
		t.Fatalf("expected repeated basic attacks to defeat mob, got %+v", mob)
	}
	if attackDeltas < 2 {
		t.Fatalf("expected more than one basic attack delta, got %d", attackDeltas)
	}
	if runtime.autoBasicAttack != nil {
		t.Fatalf("expected auto attack to clear after target death, got %+v", runtime.autoBasicAttack)
	}
}

func TestAttachedRuntimeBasicAttackQueuesApproachWhenOutOfRange(t *testing.T) {
	runtime := newAttachedRuntime("sess_basic_attack_far", &Character{ID: "char_basic_attack_far", LastRegionID: "dawn_plaza"})
	runtime.position = runtimePoint{X: 20, Z: 10}
	runtime.knownEntities["mob_1"] = runtimeEntity{
		EntityID:   "mob_1",
		EntityType: "mob",
		TemplateID: "mireling",
		Position:   runtimePoint{X: 24, Z: 10},
		State: map[string]any{
			"hp":    54,
			"alive": true,
		},
	}

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_basic_attack_far",
		CommandSeq:      1,
		Type:            "basic_attack",
		Payload:         []byte(`{"target_id":"mob_1"}`),
	})

	if len(outbound) < 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected basic attack approach to return ack and movement delta, got %+v", outbound)
	}
	if runtime.queuedBasicAttack == nil || runtime.queuedBasicAttack.TargetID != "mob_1" {
		t.Fatalf("expected queued basic attack against mob_1, got %+v", runtime.queuedBasicAttack)
	}

	messages, _, _ := runtime.collectTickMessages(time.Now().Add(time.Second))
	if len(messages) == 0 {
		t.Fatal("expected queued basic attack to resolve after movement tick")
	}
	attackDelta := messages[len(messages)-1]
	if attackDelta["kind"] != "delta" {
		t.Fatalf("expected final queued basic attack resolution message to be delta, got %+v", attackDelta)
	}
	if deltaSelfPosition(t, attackDelta) != runtime.position {
		t.Fatalf("expected queued basic attack delta to carry settled position %+v, got %+v", runtime.position, deltaSelfPosition(t, attackDelta))
	}
	if path := deltaSelfAuthoritativePath(t, attackDelta); len(path) != 0 {
		t.Fatalf("expected queued basic attack delta to clear authoritative path, got %+v", path)
	}
	mob := runtime.knownEntities["mob_1"]
	hp, _ := mob.State["hp"].(int)
	if hp >= 54 {
		t.Fatalf("expected queued basic attack to damage mob, got hp=%d messages=%+v", hp, messages)
	}
}

func TestAttachedRuntimePassiveMobDoesNotAggroByProximity(t *testing.T) {
	runtime := newAttachedRuntime("sess_passive_mob_ai", &Character{ID: "char_passive_mob_ai", LastRegionID: "dawn_plaza"})
	runtime.position = runtimePoint{X: 0, Z: 0}
	runtime.knownEntities = map[string]runtimeEntity{
		"mob_passive": testRuntimeMob("mob_passive", "mireling", mobPersonalityPassive, runtimePoint{X: 5, Z: 0}, 54),
	}

	messages, _, _ := runtime.collectTickMessages(time.Now().Add(time.Second))
	if len(messages) != 0 {
		t.Fatalf("expected passive mob to remain idle near player, got %+v", messages)
	}
	mob := runtime.knownEntities["mob_passive"]
	if state := runtimeMobAIState(mob); state != mobAIStateIdle {
		t.Fatalf("expected passive mob to stay idle, got %s", state)
	}
	if mob.Position != (runtimePoint{X: 5, Z: 0}) {
		t.Fatalf("expected passive mob not to chase, got %+v", mob.Position)
	}
}

func TestAttachedRuntimeAggressiveMobAggrosAndChasesByProximity(t *testing.T) {
	runtime := newAttachedRuntime("sess_aggressive_mob_ai", &Character{ID: "char_aggressive_mob_ai", LastRegionID: "dawn_plaza"})
	runtime.position = runtimePoint{X: 0, Z: 0}
	runtime.knownEntities = map[string]runtimeEntity{
		"mob_aggressive": testRuntimeMob("mob_aggressive", "gloom_wisp", mobPersonalityAggressive, runtimePoint{X: 7, Z: 0}, 68),
	}

	messages, _, _ := runtime.collectTickMessages(time.Now().Add(time.Second))
	if len(messages) != 1 || messages[0]["kind"] != "delta" {
		t.Fatalf("expected aggressive mob AI delta, got %+v", messages)
	}
	mob := runtime.knownEntities["mob_aggressive"]
	if state := runtimeMobAIState(mob); state != mobAIStateAggro {
		t.Fatalf("expected aggressive mob to enter aggro, got %s", state)
	}
	if mob.Position.X >= 7 {
		t.Fatalf("expected aggressive mob to chase toward player, got %+v", mob.Position)
	}
	entities, ok := messages[0]["entities"].([]map[string]any)
	if !ok || len(entities) != 1 || entities[0]["ai_state"] != string(mobAIStateAggro) {
		t.Fatalf("expected mob aggro entity patch, got %+v", messages[0]["entities"])
	}
}

func TestAttachedRuntimePassiveMobAggrosAfterBeingAttacked(t *testing.T) {
	runtime := newAttachedRuntime("sess_passive_mob_attacked", &Character{ID: "char_passive_mob_attacked", LastRegionID: "dawn_plaza"})
	runtime.position = runtimePoint{X: 0, Z: 0}
	runtime.knownEntities = map[string]runtimeEntity{
		"mob_passive": testRuntimeMob("mob_passive", "mireling", mobPersonalityPassive, runtimePoint{X: 7, Z: 0}, 54),
	}

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_hit_passive",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_passive"}`),
	})
	if len(outbound) < 2 || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected skill to damage passive mob, got %+v", outbound)
	}
	if state := runtimeMobAIState(runtime.knownEntities["mob_passive"]); state != mobAIStateAggro {
		t.Fatalf("expected attacked passive mob to enter aggro, got %s", state)
	}
	hpAfterHit := runtime.currentHP

	messages, _, _ := runtime.collectTickMessages(time.Now().Add(time.Second))
	if len(messages) == 0 {
		t.Fatal("expected attacked passive mob to chase on tick")
	}
	mob := runtime.knownEntities["mob_passive"]
	if mob.Position.X >= 7 {
		t.Fatalf("expected attacked passive mob to chase player, got %+v", mob.Position)
	}
	if runtime.currentHP != hpAfterHit {
		t.Fatalf("expected out-of-range passive mob not to damage player before reaching range, before=%d after=%d", hpAfterHit, runtime.currentHP)
	}
}

func TestAttachedRuntimeAggressiveMobAttacksWhenInRange(t *testing.T) {
	runtime := newAttachedRuntime("sess_aggressive_mob_attack", &Character{ID: "char_aggressive_mob_attack", LastRegionID: "dawn_plaza"})
	runtime.position = runtimePoint{X: 0, Z: 0}
	startHP := runtime.currentHP
	runtime.knownEntities = map[string]runtimeEntity{
		"mob_aggressive": testRuntimeMob("mob_aggressive", "gloom_wisp", mobPersonalityAggressive, runtimePoint{X: 1.5, Z: 0}, 68),
	}

	messages, _, _ := runtime.collectTickMessages(time.Now().Add(time.Second))
	if len(messages) != 1 || messages[0]["kind"] != "delta" {
		t.Fatalf("expected aggressive mob attack delta, got %+v", messages)
	}
	if runtime.currentHP >= startHP {
		t.Fatalf("expected aggressive mob to damage player, before=%d after=%d", startHP, runtime.currentHP)
	}
	if hp := deltaSelfHP(t, messages[0]); hp != runtime.currentHP {
		t.Fatalf("expected self hp delta to match runtime hp, delta=%d runtime=%d", hp, runtime.currentHP)
	}
}

func TestAttachedRuntimeClearTargetCancelsTargetDrivenState(t *testing.T) {
	runtime := newAttachedRuntime("sess_clear_target", &Character{ID: "char_clear_target", LastRegionID: "dawn_plaza"})
	runtime.position = runtimePoint{X: 20, Z: 10}
	runtime.knownEntities["mob_1"] = runtimeEntity{
		EntityID:   "mob_1",
		EntityType: "mob",
		TemplateID: "mireling",
		Position:   runtimePoint{X: 27, Z: 10},
		State: map[string]any{
			"hp":    54,
			"alive": true,
		},
	}

	approach := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_basic_attack_far",
		CommandSeq:      1,
		Type:            "basic_attack",
		Payload:         []byte(`{"target_id":"mob_1"}`),
	})
	if len(approach) < 2 || approach[1]["kind"] != "delta" {
		t.Fatalf("expected basic attack approach to enter target-driven movement, got %+v", approach)
	}
	if runtime.targetID != "mob_1" || runtime.queuedBasicAttack == nil || runtime.autoBasicAttack == nil || runtime.activeMovement == nil {
		t.Fatalf("expected target-driven state before clear, target=%q queued=%+v auto=%+v movement=%+v", runtime.targetID, runtime.queuedBasicAttack, runtime.autoBasicAttack, runtime.activeMovement)
	}

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_clear_target",
		CommandSeq:      2,
		Type:            "clear_target",
		Payload:         []byte(`{}`),
	})

	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected clear_target to apply with ack and delta, got %+v", outbound)
	}
	self, ok := outbound[1]["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected clear_target delta self payload, got %+v", outbound[1]["self"])
	}
	if targetID, exists := self["target_id"]; exists && targetID != nil {
		t.Fatalf("expected target_id to be cleared, got %+v", targetID)
	}
	if runtime.targetID != "" || runtime.queuedSkill != nil || runtime.queuedBasicAttack != nil || runtime.autoBasicAttack != nil || runtime.queuedLootPickup != nil {
		t.Fatalf("expected all target-driven runtime state to clear, target=%q queuedSkill=%+v queuedBasic=%+v auto=%+v loot=%+v", runtime.targetID, runtime.queuedSkill, runtime.queuedBasicAttack, runtime.autoBasicAttack, runtime.queuedLootPickup)
	}
	if runtime.activeMovement != nil {
		t.Fatalf("expected target-driven movement to clear, got %+v", runtime.activeMovement)
	}
	if path := deltaSelfAuthoritativePath(t, outbound[1]); len(path) != 0 {
		t.Fatalf("expected clear_target delta to publish empty authoritative path, got %+v", path)
	}
}

func TestAttachedRuntimeRejectsCooldownActive(t *testing.T) {
	runtime := newAttachedRuntime("sess_1", &Character{ID: "char_1", LastRegionID: "dawn_plaza"})
	moveRuntimeNearMob(runtime, "mob_1")

	first := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})
	if len(first) < 2 || first[1]["kind"] != "delta" {
		t.Fatalf("expected first skill to apply, got %+v", first)
	}

	second := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_2",
		CommandSeq:      2,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})
	if len(second) != 2 || second[0]["kind"] != "ack" || second[1]["reason_code"] != "combat.cooldown_active" {
		t.Fatalf("expected ack followed by cooldown reject, got %+v", second)
	}
}

func TestAttachedRuntimeLoadsPersistedCooldownsAuthoritatively(t *testing.T) {
	now := time.Now()
	runtime := newAttachedRuntime("sess_cooldown_1", &Character{
		ID:           "char_cooldown_1",
		BaseClass:    "Fighter",
		Level:        2,
		LastRegionID: "dawn_plaza",
	})
	runtime.loadCooldownState([]CharacterSkillCooldown{
		{
			CharacterID: "char_cooldown_1",
			SkillID:     "grave_bloom",
			EndsAt:      now.Add(2 * time.Second),
		},
		{
			CharacterID: "char_cooldown_1",
			SkillID:     "crescent_strike",
			EndsAt:      now.Add(-250 * time.Millisecond),
		},
	}, now)
	moveRuntimeNearMob(runtime, "mob_1")

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_cooldown_1",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"grave_bloom","target_id":"mob_1"}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["reason_code"] != "combat.cooldown_active" {
		t.Fatalf("expected persisted grave_bloom cooldown to reject use_skill, got %+v", outbound)
	}

	snapshot := cooldownSnapshotFromRecords(runtime.characterCooldownState(now), now)
	if _, exists := snapshot["grave_bloom"]; !exists {
		t.Fatalf("expected active grave_bloom cooldown to remain in runtime snapshot, got %+v", snapshot)
	}
	if _, exists := snapshot["crescent_strike"]; exists {
		t.Fatalf("expected expired cooldown to be pruned from runtime snapshot, got %+v", snapshot)
	}
}

func TestAttachedRuntimeRejectsInsufficientMP(t *testing.T) {
	runtime := newAttachedRuntime("sess_mp_1", &Character{ID: "char_mp_1", LastRegionID: "dawn_plaza", CurrentMP: 5})
	moveRuntimeNearMob(runtime, "mob_1")

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_mp_1",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})

	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["reason_code"] != "combat.insufficient_mp" {
		t.Fatalf("expected ack followed by combat.insufficient_mp reject, got %+v", outbound)
	}
}

func TestAttachedRuntimeQueuesSkillApproachWhenTargetIsOutOfRange(t *testing.T) {
	runtime := newAttachedRuntime("sess_1", &Character{ID: "char_1", LastRegionID: "dawn_plaza"})
	runtime.position = runtimePoint{X: -95, Z: 0}

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})

	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack followed by skill approach delta, got %+v", outbound)
	}
	if runtime.queuedSkill == nil || runtime.queuedSkill.SkillID != "crescent_strike" || runtime.queuedSkill.TargetID != "mob_1" {
		t.Fatalf("expected queued crescent_strike approach, got %+v", runtime.queuedSkill)
	}

	tickMessages, movementChanged, respawned := runtime.collectTickMessages(time.Now().Add(5 * time.Second))
	if !movementChanged || respawned {
		t.Fatalf("expected movement without respawn, movement=%v respawned=%v", movementChanged, respawned)
	}
	if len(tickMessages) < 2 || tickMessages[len(tickMessages)-1]["kind"] != "delta" {
		t.Fatalf("expected movement delta followed by skill delta, got %+v", tickMessages)
	}
	previousRevision := 0
	for _, message := range tickMessages {
		if message["kind"] != "delta" {
			continue
		}
		revision, ok := message["revision"].(int)
		if !ok {
			t.Fatalf("expected integer revision in tick delta, got %+v", message)
		}
		if previousRevision != 0 && revision <= previousRevision {
			t.Fatalf("expected tick deltas to be emitted in increasing revision order, previous=%d current=%d messages=%+v", previousRevision, revision, tickMessages)
		}
		previousRevision = revision
	}
	if runtime.queuedSkill != nil {
		t.Fatalf("expected queued skill to resolve after reaching range, got %+v", runtime.queuedSkill)
	}
	skillDelta := tickMessages[len(tickMessages)-1]
	if deltaSelfPosition(t, skillDelta) != runtime.position {
		t.Fatalf("expected queued skill delta to carry settled position %+v, got %+v", runtime.position, deltaSelfPosition(t, skillDelta))
	}
	if path := deltaSelfAuthoritativePath(t, skillDelta); len(path) != 0 {
		t.Fatalf("expected queued skill delta to clear authoritative path, got %+v", path)
	}
	mob := runtime.knownEntities["mob_1"]
	if hp, _ := mob.State["hp"].(int); hp >= 54 {
		t.Fatalf("expected queued skill to damage mob after approach, got hp=%d", hp)
	}
}

func TestAttachedRuntimeRejectsSkillOnDeadTarget(t *testing.T) {
	runtime := newAttachedRuntime("sess_1", &Character{ID: "char_1", LastRegionID: "dawn_plaza"})
	entity := runtime.knownEntities["mob_1"]
	entity.State["hp"] = 0
	entity.State["alive"] = false
	runtime.knownEntities["mob_1"] = entity

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})

	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["reason_code"] != "combat.target_dead" {
		t.Fatalf("expected ack followed by combat.target_dead reject, got %+v", outbound)
	}
}

func TestAttachedRuntimeAppliesDeathStateAndClearsTarget(t *testing.T) {
	runtime := newAttachedRuntime("sess_1", &Character{ID: "char_1", LastRegionID: "dawn_plaza"})
	moveRuntimeNearMob(runtime, "mob_1")
	entity := runtime.knownEntities["mob_1"]
	entity.State["hp"] = 10
	entity.State["alive"] = true
	runtime.knownEntities["mob_1"] = entity

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})

	if len(outbound) < 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta at the start of lethal skill outbound, got %+v", outbound)
	}
	self, ok := outbound[1]["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", outbound[1]["self"])
	}
	if targetID, exists := self["target_id"]; exists && targetID != nil {
		t.Fatalf("expected target to be cleared after lethal hit, got %+v", targetID)
	}
	entities, ok := outbound[1]["entities"].([]map[string]any)
	if !ok || len(entities) != 1 {
		t.Fatalf("expected one entity patch, got %+v", outbound[1]["entities"])
	}
	if entities[0]["hp"] != 0 || entities[0]["alive"] != false {
		t.Fatalf("expected death state patch, got %+v", entities[0])
	}
}

func TestAttachedRuntimeEmitsDisappearAfterCorpseDelay(t *testing.T) {
	runtime := newAttachedRuntime("sess_1", &Character{ID: "char_1", LastRegionID: "dawn_plaza"})
	moveRuntimeNearMob(runtime, "mob_1")
	entity := runtime.knownEntities["mob_1"]
	entity.State["hp"] = 10
	entity.State["alive"] = true
	runtime.knownEntities["mob_1"] = entity

	runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})

	outbound := runtime.collectLifecycleMessages(time.Now().Add(corpseDespawnDelay + 10*time.Millisecond))
	if len(outbound) != 1 || outbound[0]["kind"] != "entity_disappear" {
		t.Fatalf("expected entity_disappear after corpse delay, got %+v", outbound)
	}
	if _, exists := runtime.knownEntities["mob_1"]; exists {
		t.Fatalf("expected mob_1 to be removed from known set after disappear")
	}
}

func TestAttachedRuntimeEmitsRespawnAppearAfterDisappear(t *testing.T) {
	runtime := newAttachedRuntime("sess_1", &Character{ID: "char_1", LastRegionID: "dawn_plaza"})
	moveRuntimeNearMob(runtime, "mob_1")
	entity := runtime.knownEntities["mob_1"]
	entity.State["hp"] = 10
	entity.State["alive"] = true
	runtime.knownEntities["mob_1"] = entity

	runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})

	runtime.collectLifecycleMessages(time.Now().Add(corpseDespawnDelay + 10*time.Millisecond))
	outbound := runtime.collectLifecycleMessages(time.Now().Add(corpseDespawnDelay + mobRespawnDelay + 10*time.Millisecond))
	if len(outbound) != 1 || outbound[0]["kind"] != "entity_appear" {
		t.Fatalf("expected entity_appear after respawn delay, got %+v", outbound)
	}
	entityPayload, ok := outbound[0]["entity"].(runtimeEntity)
	if !ok {
		t.Fatalf("expected runtimeEntity payload, got %+v", outbound[0]["entity"])
	}
	if entityPayload.EntityID != "mob_1" || !isRuntimeEntityAlive(entityPayload) {
		t.Fatalf("expected living respawn entity for mob_1, got %+v", entityPayload)
	}
	respawned, exists := runtime.knownEntities["mob_1"]
	if !exists || !isRuntimeEntityAlive(respawned) {
		t.Fatalf("expected mob_1 to be back in known set after respawn, got %+v", respawned)
	}
}

func TestAttachedRuntimeAppliesSkillDeltaWithCooldownAndDamage(t *testing.T) {
	runtime := newAttachedRuntime("sess_1", &Character{
		ID:           "char_1",
		BaseClass:    "Fighter",
		Level:        2,
		LastRegionID: "dawn_plaza",
	})
	moveRuntimeNearMob(runtime, "mob_1")

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"grave_bloom","target_id":"mob_1"}`),
	})

	if len(outbound) < 2 {
		t.Fatalf("expected ack and delta, got %+v", outbound)
	}
	delta, ok := outbound[1]["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", outbound[1])
	}
	cooldowns, ok := delta["cooldowns"].(map[string]int)
	if !ok {
		// go's interface decoding keeps concrete map type from construction, but guard anyway
		cooldownsAny, okAny := delta["cooldowns"].(map[string]any)
		if !okAny {
			t.Fatalf("expected cooldowns map, got %+v", delta["cooldowns"])
		}
		if cooldownsAny["grave_bloom"] == nil {
			t.Fatalf("expected grave_bloom cooldown in delta, got %+v", cooldownsAny)
		}
	} else if cooldowns["grave_bloom"] != 4500 {
		t.Fatalf("expected 4500ms cooldown, got %+v", cooldowns)
	}
	entities, ok := outbound[1]["entities"].([]map[string]any)
	if !ok {
		entitiesAny, okAny := outbound[1]["entities"].([]any)
		if !okAny || len(entitiesAny) == 0 {
			t.Fatalf("expected entities patch, got %+v", outbound[1]["entities"])
		}
		return
	}
	if len(entities) == 0 {
		t.Fatalf("expected at least one entity patch")
	}
}

func TestAttachedRuntimeAppliesSkillWhenTargetIsWithinRange(t *testing.T) {
	runtime := newAttachedRuntime("sess_1", &Character{ID: "char_1", LastRegionID: "dawn_plaza"})
	moveRuntimeNearMob(runtime, "mob_1")

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})

	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta for in-range skill, got %+v", outbound)
	}
}

func TestAttachedRuntimeDeathGeneratesAuthoritativeLoot(t *testing.T) {
	runtime := newAttachedRuntime("sess_1", &Character{ID: "char_1", LastRegionID: "dawn_plaza"})
	moveRuntimeNearMob(runtime, "mob_1")
	entity := runtime.knownEntities["mob_1"]
	entity.State["hp"] = 10
	entity.State["alive"] = true
	runtime.knownEntities["mob_1"] = entity

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})

	if len(outbound) != 3 {
		t.Fatalf("expected ack, delta and loot entity_appear, got %+v", outbound)
	}
	if outbound[2]["kind"] != "entity_appear" {
		t.Fatalf("expected loot entity_appear, got %+v", outbound[2])
	}
	entityPayload, ok := outbound[2]["entity"].(runtimeEntity)
	if !ok {
		t.Fatalf("expected runtimeEntity payload, got %+v", outbound[2]["entity"])
	}
	if entityPayload.EntityType != "loot" || entityPayload.TemplateID != "duskgold" {
		t.Fatalf("expected duskgold loot entity, got %+v", entityPayload)
	}
	if _, exists := runtime.knownEntities[entityPayload.EntityID]; !exists {
		t.Fatalf("expected loot entity to remain in runtime known set")
	}
}

func TestAttachedRuntimeAwardsXPAndLevelsAuthoritativelyOnKill(t *testing.T) {
	runtime := newAttachedRuntime("sess_progress_1", &Character{
		ID:           "char_progress_1",
		LastRegionID: "dawn_plaza",
		Level:        1,
		XP:           60,
		CurrentHP:    120,
		CurrentMP:    58,
	})
	runtime.derivedStats = CharacterDerivedStats{
		MaxHP:     142,
		MaxMP:     58,
		Attack:    27,
		Defense:   15,
		MoveSpeed: 3.225,
	}
	moveRuntimeNearMob(runtime, "mob_1")
	entity := runtime.knownEntities["mob_1"]
	entity.State["hp"] = 10
	entity.State["alive"] = true
	runtime.knownEntities["mob_1"] = entity

	outbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_progress_1",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})

	if len(outbound) != 3 || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack, delta and loot entity_appear on lethal strike, got %+v", outbound)
	}
	if level := deltaSelfLevel(t, outbound[1]); level != 2 {
		t.Fatalf("expected level 2 after kill, got %d", level)
	}
	if xp := deltaSelfXP(t, outbound[1]); xp != 12 {
		t.Fatalf("expected rollover xp 12 after level up, got %d", xp)
	}
	if hp := deltaSelfHP(t, outbound[1]); hp != 160 {
		t.Fatalf("expected full hp 160 after level up, got %d", hp)
	}
	if mp := deltaSelfMP(t, outbound[1]); mp != 65 {
		t.Fatalf("expected full mp 65 after level up, got %d", mp)
	}
	stats := deltaSelfStats(t, outbound[1])
	if stats.MaxHP != 160 || stats.MaxMP != 65 || stats.Attack != 31 || stats.Defense != 17 {
		t.Fatalf("expected level-up derived stats, got %+v", stats)
	}
}

func TestAttachedRuntimeQueuesLootPickupApproachWhenOutOfReach(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_1", AccountID: "acc_1", Name: "Loot Walker", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	runtime := newAttachedRuntime("sess_1", character)
	runtime.position = runtimePoint{X: 0, Z: 0}
	runtime.knownEntities["loot_1"] = runtimeEntity{
		EntityID:   "loot_1",
		EntityType: "loot",
		TemplateID: "duskgold",
		Position:   runtimePoint{X: 10, Z: 0},
		State:      map[string]any{"quantity": 4},
	}

	outbound := runtime.processLootPickup(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "pick_up_loot",
		Payload:         []byte(`{"loot_id":"loot_1"}`),
	})

	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack followed by loot approach movement delta, got %+v", outbound)
	}
	if runtime.queuedLootPickup == nil || runtime.queuedLootPickup.LootID != "loot_1" {
		t.Fatalf("expected queued loot pickup for loot_1, got %+v", runtime.queuedLootPickup)
	}

	messages, _, _, _ := runtime.collectTickMessagesWithStore(time.Now().Add(2*time.Second), store)
	if len(messages) < 2 || messages[len(messages)-2]["kind"] != "delta" || messages[len(messages)-1]["kind"] != "entity_disappear" {
		t.Fatalf("expected queued loot pickup to resolve with inventory delta and disappear, got %+v", messages)
	}
	if deltaSelfPosition(t, messages[len(messages)-2]) != runtime.position {
		t.Fatalf("expected loot resolve delta to carry settled position %+v, got %+v", runtime.position, deltaSelfPosition(t, messages[len(messages)-2]))
	}
	if path := deltaSelfAuthoritativePath(t, messages[len(messages)-2]); len(path) != 0 {
		t.Fatalf("expected loot resolve delta to clear authoritative path, got %+v", path)
	}
	if _, exists := runtime.knownEntities["loot_1"]; exists {
		t.Fatal("expected loot_1 removed after queued pickup resolves")
	}
}

func TestAttachedRuntimeLootPickupApproachUsesPickupRangeDestination(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_1", AccountID: "acc_1", Name: "Range Loot Walker", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	lootPosition := runtimePoint{X: 10, Z: 0}
	runtime := newAttachedRuntime("sess_1", character)
	runtime.position = runtimePoint{X: 0, Z: 0}
	runtime.movementPlanner = scriptedMovementPlanner{
		version: "loot_approach_geo_v1",
		resolve: func(_ context.Context, _ string, _ runtimePoint, destination runtimePoint, _ movementProfile) movementResolution {
			if destination == lootPosition {
				outOfRangeDestination := runtimePoint{X: 10, Z: 6}
				return movementResolution{
					Status: movementPlanStatusAccepted,
					Plan: movementPlan{
						GeodataVersion:      "loot_approach_geo_v1",
						AcceptedDestination: outOfRangeDestination,
						Waypoints:           []runtimePoint{outOfRangeDestination},
					},
				}
			}
			return movementResolution{
				Status: movementPlanStatusAccepted,
				Plan: movementPlan{
					GeodataVersion:      "loot_approach_geo_v1",
					AcceptedDestination: destination,
					Waypoints:           []runtimePoint{destination},
				},
			}
		},
	}
	runtime.knownEntities["loot_1"] = runtimeEntity{
		EntityID:   "loot_1",
		EntityType: "loot",
		TemplateID: "duskgold",
		Position:   lootPosition,
		State:      map[string]any{"quantity": 4},
	}

	outbound := runtime.processLootPickup(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "pick_up_loot",
		Payload:         []byte(`{"loot_id":"loot_1"}`),
	})

	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack followed by loot approach movement delta, got %+v", outbound)
	}
	if runtime.activeMovement == nil {
		t.Fatal("expected loot pickup approach movement to be active")
	}
	if distance(runtime.activeMovement.AcceptedDestination, lootPosition) > lootPickupRange {
		t.Fatalf("expected accepted loot approach destination within pickup range, got %+v", runtime.activeMovement.AcceptedDestination)
	}

	messages, _, _, _ := runtime.collectTickMessagesWithStore(time.Now().Add(2*time.Second), store)
	if len(messages) < 2 || messages[len(messages)-2]["kind"] != "delta" || messages[len(messages)-1]["kind"] != "entity_disappear" {
		t.Fatalf("expected queued loot pickup to resolve after approach, got %+v", messages)
	}
	if deltaSelfPosition(t, messages[len(messages)-2]) != runtime.position {
		t.Fatalf("expected resolved loot approach delta to carry settled position %+v, got %+v", runtime.position, deltaSelfPosition(t, messages[len(messages)-2]))
	}
	if path := deltaSelfAuthoritativePath(t, messages[len(messages)-2]); len(path) != 0 {
		t.Fatalf("expected resolved loot approach delta to clear authoritative path, got %+v", path)
	}
}

func TestAttachedRuntimeAcceptsAdjacentLootPickup(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_1", AccountID: "acc_1", Name: "Adjacent Loot Hero", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	runtime := newAttachedRuntime("sess_1", character)
	runtime.position = runtimePoint{X: 0, Z: 0}
	runtime.knownEntities["loot_adjacent"] = runtimeEntity{
		EntityID:   "loot_adjacent",
		EntityType: "loot",
		TemplateID: "duskgold",
		Position:   runtimePoint{X: 4.4, Z: 0},
		State:      map[string]any{"quantity": 4},
	}

	outbound := runtime.processLootPickup(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "pick_up_loot",
		Payload:         []byte(`{"loot_id":"loot_adjacent"}`),
	})

	if len(outbound) != 3 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" || outbound[2]["kind"] != "entity_disappear" {
		t.Fatalf("expected adjacent loot pickup to apply, got %+v", outbound)
	}
	if _, exists := runtime.knownEntities["loot_adjacent"]; exists {
		t.Fatal("expected adjacent loot to leave the known set after pickup")
	}
}

func TestAttachedRuntimeLootPickupPersistsInventoryAndPreventsDuplication(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_1", AccountID: "acc_1", Name: "Loot Hero", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	runtime := newAttachedRuntime("sess_1", character)
	runtime.position = runtimePoint{X: -8, Z: 0}
	runtime.knownEntities["loot_1"] = runtimeEntity{
		EntityID:   "loot_1",
		EntityType: "loot",
		TemplateID: "duskgold",
		Position:   runtime.position,
		State:      map[string]any{"quantity": 4},
	}

	first := runtime.processLootPickup(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_1",
		CommandSeq:      1,
		Type:            "pick_up_loot",
		Payload:         []byte(`{"loot_id":"loot_1"}`),
	})
	if len(first) != 3 || first[0]["kind"] != "ack" || first[1]["kind"] != "delta" || first[2]["kind"] != "entity_disappear" {
		t.Fatalf("expected ack, delta and entity_disappear, got %+v", first)
	}
	deltaInventory, ok := first[1]["inventory"].([]CharacterItem)
	if !ok {
		t.Fatalf("expected inventory snapshot in delta, got %+v", first[1]["inventory"])
	}
	foundDuskgold := false
	for _, item := range deltaInventory {
		if item.TemplateID == "duskgold" && item.Quantity == 16 {
			foundDuskgold = true
		}
	}
	if !foundDuskgold {
		t.Fatalf("expected stacked duskgold quantity 16 after pickup, got %+v", deltaInventory)
	}
	if _, exists := runtime.knownEntities["loot_1"]; exists {
		t.Fatalf("expected loot_1 removed from runtime after pickup")
	}

	persistedItems, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() error = %v", err)
	}
	persistedDuskgold := 0
	for _, item := range persistedItems {
		if item.TemplateID == "duskgold" {
			persistedDuskgold += item.Quantity
		}
	}
	if persistedDuskgold != 16 {
		t.Fatalf("expected persisted duskgold quantity 16, got %d", persistedDuskgold)
	}

	second := runtime.processLootPickup(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_2",
		CommandSeq:      2,
		Type:            "pick_up_loot",
		Payload:         []byte(`{"loot_id":"loot_1"}`),
	})
	if len(second) != 2 || second[0]["kind"] != "ack" || second[1]["reason_code"] != "world.entity_not_known" {
		t.Fatalf("expected duplicate pickup to reject without duplicating items, got %+v", second)
	}

	persistedAfterDuplicate, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() after duplicate error = %v", err)
	}
	persistedDuskgold = 0
	for _, item := range persistedAfterDuplicate {
		if item.TemplateID == "duskgold" {
			persistedDuskgold += item.Quantity
		}
	}
	if persistedDuskgold != 16 {
		t.Fatalf("expected duplicate pickup to keep duskgold at 16, got %d", persistedDuskgold)
	}
}

func TestAttachedRuntimeFirstValidLootPickupWinsUnderContention(t *testing.T) {
	store := newLootPickupTestStore(t)
	characterOne := &Character{ID: "char_1", AccountID: "acc_1", Name: "Loot Winner", LastRegionID: "dawn_plaza"}
	characterTwo := &Character{ID: "char_2", AccountID: "acc_2", Name: "Loot Loser", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), characterOne, initialCharacterItemSeed(characterOne)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(characterOne) error = %v", err)
	}
	if err := store.CreateCharacterWithItemSeed(context.Background(), characterTwo, initialCharacterItemSeed(characterTwo)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(characterTwo) error = %v", err)
	}

	runtimeOne := newAttachedRuntime("sess_1", characterOne)
	runtimeTwo := newAttachedRuntime("sess_2", characterTwo)
	sharedLoot := runtimeEntity{
		EntityID:   "loot_contended",
		EntityType: "loot",
		TemplateID: "duskgold",
		Position:   runtimePoint{X: -8, Z: 0},
		State:      map[string]any{"quantity": 4},
	}
	runtimeOne.position = sharedLoot.Position
	runtimeTwo.position = sharedLoot.Position
	runtimeOne.knownEntities[sharedLoot.EntityID] = sharedLoot
	runtimeTwo.knownEntities[sharedLoot.EntityID] = sharedLoot

	type result struct {
		outbound []map[string]any
	}
	results := make([]result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		results[0].outbound = runtimeOne.processLootPickup(context.Background(), store, commandEnvelope{
			ProtocolVersion: 1,
			CommandID:       "cmd_1",
			CommandSeq:      1,
			Type:            "pick_up_loot",
			Payload:         []byte(`{"loot_id":"loot_contended"}`),
		})
	}()
	go func() {
		defer wg.Done()
		<-start
		results[1].outbound = runtimeTwo.processLootPickup(context.Background(), store, commandEnvelope{
			ProtocolVersion: 1,
			CommandID:       "cmd_1",
			CommandSeq:      1,
			Type:            "pick_up_loot",
			Payload:         []byte(`{"loot_id":"loot_contended"}`),
		})
	}()
	close(start)
	wg.Wait()

	successCount := 0
	rejectCount := 0
	winningCharacterID := ""
	for index, result := range results {
		if len(result.outbound) >= 3 && result.outbound[1]["kind"] == "delta" {
			successCount++
			if index == 0 {
				winningCharacterID = characterOne.ID
			} else {
				winningCharacterID = characterTwo.ID
			}
			continue
		}
		if len(result.outbound) == 2 && result.outbound[1]["reason_code"] == "loot.already_collected" {
			rejectCount++
			continue
		}
		t.Fatalf("expected one success and one contention reject, got %+v", result.outbound)
	}
	if successCount != 1 || rejectCount != 1 {
		t.Fatalf("expected exactly one success and one reject, got success=%d reject=%d", successCount, rejectCount)
	}

	itemsOne, err := store.Items.ListByCharacterID(context.Background(), characterOne.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID(characterOne) error = %v", err)
	}
	itemsTwo, err := store.Items.ListByCharacterID(context.Background(), characterTwo.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID(characterTwo) error = %v", err)
	}
	duskgoldOne := 0
	for _, item := range itemsOne {
		if item.TemplateID == "duskgold" {
			duskgoldOne += item.Quantity
		}
	}
	duskgoldTwo := 0
	for _, item := range itemsTwo {
		if item.TemplateID == "duskgold" {
			duskgoldTwo += item.Quantity
		}
	}
	if winningCharacterID == characterOne.ID {
		if duskgoldOne != 16 || duskgoldTwo != 12 {
			t.Fatalf("expected characterOne to win contention, got duskgoldOne=%d duskgoldTwo=%d", duskgoldOne, duskgoldTwo)
		}
	} else {
		if duskgoldOne != 12 || duskgoldTwo != 16 {
			t.Fatalf("expected characterTwo to win contention, got duskgoldOne=%d duskgoldTwo=%d", duskgoldOne, duskgoldTwo)
		}
	}
}

func TestAttachedRuntimeRejectsPartyLootPickupForIneligibleActor(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_party_loot_reject", AccountID: "acc_party_loot_reject", Name: "Loot Outsider", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	runtime := newAttachedRuntime("sess_party_loot_reject", character)
	runtime.position = runtimePoint{X: -8, Z: 0}
	runtime.knownEntities["loot_party_only"] = runtimeEntity{
		EntityID:   "loot_party_only",
		EntityType: "loot",
		TemplateID: "duskgold",
		Position:   runtime.position,
		State: map[string]any{
			"quantity":               4,
			"party_id":               "party_rewards_1",
			"eligible_character_ids": []string{"char_party_loot_member"},
		},
	}

	outbound := runtime.processLootPickup(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_party_loot_reject",
		CommandSeq:      1,
		Type:            "pick_up_loot",
		Payload:         []byte(`{"loot_id":"loot_party_only"}`),
	})

	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["reason_code"] != "loot.party_ineligible" {
		t.Fatalf("expected party-ineligible loot reject, got %+v", outbound)
	}
}

func TestAttachedRuntimeFirstEligiblePartyLootPickupWinsUnderContention(t *testing.T) {
	store := newLootPickupTestStore(t)
	characterOne := &Character{ID: "char_party_loot_one", AccountID: "acc_party_loot_one", Name: "Loot One", LastRegionID: "dawn_plaza"}
	characterTwo := &Character{ID: "char_party_loot_two", AccountID: "acc_party_loot_two", Name: "Loot Two", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), characterOne, initialCharacterItemSeed(characterOne)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(characterOne) error = %v", err)
	}
	if err := store.CreateCharacterWithItemSeed(context.Background(), characterTwo, initialCharacterItemSeed(characterTwo)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(characterTwo) error = %v", err)
	}

	runtimeOne := newAttachedRuntime("sess_party_loot_one", characterOne)
	runtimeTwo := newAttachedRuntime("sess_party_loot_two", characterTwo)
	partySnapshot := &CharacterPartySnapshot{
		PartyID:           "party_rewards_3",
		LeaderCharacterID: characterOne.ID,
		Members: []CharacterPartyMemberSnapshot{
			{CharacterID: characterOne.ID, Name: characterOne.Name, IsLeader: true, Online: true},
			{CharacterID: characterTwo.ID, Name: characterTwo.Name, Online: true},
		},
	}
	runtimeOne.loadPartyState(partySnapshot, nil)
	runtimeTwo.loadPartyState(partySnapshot, nil)

	sharedLoot := runtimeEntity{
		EntityID:   "loot_party_contended",
		EntityType: "loot",
		TemplateID: "duskgold",
		Position:   runtimePoint{X: -8, Z: 0},
		State: map[string]any{
			"quantity":               4,
			"party_id":               "party_rewards_3",
			"eligible_character_ids": []string{characterOne.ID, characterTwo.ID},
		},
	}
	runtimeOne.position = sharedLoot.Position
	runtimeTwo.position = sharedLoot.Position
	runtimeOne.knownEntities[sharedLoot.EntityID] = sharedLoot
	runtimeTwo.knownEntities[sharedLoot.EntityID] = sharedLoot

	type result struct {
		outbound []map[string]any
	}
	results := make([]result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		results[0].outbound = runtimeOne.processLootPickup(context.Background(), store, commandEnvelope{
			ProtocolVersion: 1,
			CommandID:       "cmd_party_loot_one",
			CommandSeq:      1,
			Type:            "pick_up_loot",
			Payload:         []byte(`{"loot_id":"loot_party_contended"}`),
		})
	}()
	go func() {
		defer wg.Done()
		<-start
		results[1].outbound = runtimeTwo.processLootPickup(context.Background(), store, commandEnvelope{
			ProtocolVersion: 1,
			CommandID:       "cmd_party_loot_two",
			CommandSeq:      1,
			Type:            "pick_up_loot",
			Payload:         []byte(`{"loot_id":"loot_party_contended"}`),
		})
	}()
	close(start)
	wg.Wait()

	successCount := 0
	rejectCount := 0
	for _, result := range results {
		if len(result.outbound) >= 3 && result.outbound[1]["kind"] == "delta" {
			successCount++
			continue
		}
		if len(result.outbound) == 2 && result.outbound[1]["reason_code"] == "loot.already_collected" {
			rejectCount++
			continue
		}
		t.Fatalf("expected one eligible success and one contention reject, got %+v", result.outbound)
	}
	if successCount != 1 || rejectCount != 1 {
		t.Fatalf("expected exactly one success and one reject, got success=%d reject=%d", successCount, rejectCount)
	}
}

func TestAttachedRuntimeEquipItemMovesInventoryItemToEquipmentSlot(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_equip_1", AccountID: "acc_1", Name: "Equip Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}
	if _, err := store.Items.UnequipItem(context.Background(), character.ID, equipSlotWeapon); err != nil {
		t.Fatalf("Items.UnequipItem() setup error = %v", err)
	}

	items, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() setup error = %v", err)
	}
	weaponItemID := ""
	for _, item := range items {
		if item.TemplateID == "ironwood_spear" && item.ContainerKind == itemContainerInventory {
			weaponItemID = item.ID
			break
		}
	}
	if weaponItemID == "" {
		t.Fatalf("expected unequipped spear in inventory during setup")
	}

	runtime := newAttachedRuntime("sess_equip_1", character)
	outbound := runtime.processItemCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_equip_1",
		CommandSeq:      1,
		Type:            "equip_item",
		Payload:         []byte(`{"item_instance_id":"` + weaponItemID + `"}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from equip_item, got %+v", outbound)
	}

	equipment, ok := outbound[1]["equipment"].([]CharacterItem)
	if !ok {
		t.Fatalf("expected equipment snapshot in delta, got %+v", outbound[1]["equipment"])
	}
	foundWeapon := false
	for _, item := range equipment {
		if item.ID == weaponItemID && item.EquipSlot == equipSlotWeapon {
			foundWeapon = true
		}
	}
	if !foundWeapon {
		t.Fatalf("expected equipped spear in weapon slot, got %+v", equipment)
	}
	stats := deltaSelfStats(t, outbound[1])
	if stats.Attack != 27 || stats.Defense != 18 {
		t.Fatalf("expected equipped stats attack=27 defense=18, got %+v", stats)
	}
}

func TestAttachedRuntimeEquipItemMovesBootsIntoBootSlotAndUpdatesSpeed(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_equip_boots", AccountID: "acc_1", Name: "Boot Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	items, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() setup error = %v", err)
	}
	bootsItemID := ""
	for _, item := range items {
		if item.TemplateID == "pathrunner_boots" && item.ContainerKind == itemContainerInventory {
			bootsItemID = item.ID
			break
		}
	}
	if bootsItemID == "" {
		t.Fatalf("expected starter boots item in inventory during setup")
	}

	runtime := newAttachedRuntime("sess_equip_boots", character)
	outbound := runtime.processItemCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_equip_boots",
		CommandSeq:      1,
		Type:            "equip_item",
		Payload:         []byte(`{"item_instance_id":"` + bootsItemID + `"}`),
	})
	if len(outbound) != 2 || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from boots equip_item, got %+v", outbound)
	}

	equipment, ok := outbound[1]["equipment"].([]CharacterItem)
	if !ok {
		t.Fatalf("expected equipment snapshot in delta, got %+v", outbound[1]["equipment"])
	}
	foundBoots := false
	for _, item := range equipment {
		if item.ID == bootsItemID && item.EquipSlot == equipSlotBoots {
			foundBoots = true
		}
	}
	if !foundBoots {
		t.Fatalf("expected equipped boots in boots slot, got %+v", equipment)
	}
	stats := deltaSelfStats(t, outbound[1])
	if stats.Defense != 19 || stats.MoveSpeed != 3.375 {
		t.Fatalf("expected boots to raise defense to 19 and move speed to 3.375, got %+v", stats)
	}
}

func TestAttachedRuntimeEquipItemAppliesInstanceAttributesFromStarterGloves(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_equip_gloves", AccountID: "acc_1", Name: "Glove Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	items, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() setup error = %v", err)
	}
	glovesItemID := ""
	for _, item := range items {
		if item.TemplateID == "watcher_gloves" && item.ContainerKind == itemContainerInventory {
			glovesItemID = item.ID
			break
		}
	}
	if glovesItemID == "" {
		t.Fatalf("expected starter gloves item in inventory during setup")
	}

	runtime := newAttachedRuntime("sess_equip_gloves", character)
	outbound := runtime.processItemCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_equip_gloves",
		CommandSeq:      1,
		Type:            "equip_item",
		Payload:         []byte(`{"item_instance_id":"` + glovesItemID + `"}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from gloves equip_item, got %+v", outbound)
	}

	equipment, ok := outbound[1]["equipment"].([]CharacterItem)
	if !ok {
		t.Fatalf("expected equipment snapshot in delta, got %+v", outbound[1]["equipment"])
	}
	foundGloves := false
	for _, item := range equipment {
		if item.ID == glovesItemID && item.EquipSlot == equipSlotGloves {
			foundGloves = true
			if item.InstanceAttributes == nil || item.InstanceAttributes.Attack != 1 || item.InstanceAttributes.Defense != 1 {
				t.Fatalf("expected equipped gloves to keep instance attributes, got %+v", item.InstanceAttributes)
			}
		}
	}
	if !foundGloves {
		t.Fatalf("expected equipped gloves in gloves slot, got %+v", equipment)
	}

	stats := deltaSelfStats(t, outbound[1])
	if stats.Attack != 32 || stats.Defense != 20 {
		t.Fatalf("expected gloves to raise attack to 32 and defense to 20, got %+v", stats)
	}

	persistedItems, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() error = %v", err)
	}
	for _, item := range persistedItems {
		if item.ID == glovesItemID {
			if item.ContainerKind != itemContainerEquipment || item.EquipSlot != equipSlotGloves {
				t.Fatalf("expected persisted gloves to occupy gloves slot, got %+v", item)
			}
			if item.InstanceAttributes == nil || item.InstanceAttributes.Attack != 1 || item.InstanceAttributes.Defense != 1 {
				t.Fatalf("expected persisted gloves instance attributes to remain intact, got %+v", item.InstanceAttributes)
			}
			return
		}
	}
	t.Fatalf("expected persisted gloves item %s after equip", glovesItemID)
}

func TestAttachedRuntimeBuyVendorOfferUsesBackendDerivedPriceAndAddsInventoryItem(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_vendor_1", AccountID: "acc_1", Name: "Vendor Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	runtime := newAttachedRuntime("sess_vendor_1", character)
	runtime.position = runtime.knownEntities["npc_merchant"].Position

	outbound := runtime.processVendorCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_vendor_buy_1",
		CommandSeq:      1,
		Type:            "buy_item",
		Payload:         []byte(`{"vendor_offer_id":"merchant_spear_offer","quantity":1}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from buy_item, got %+v", outbound)
	}

	inventory := deltaInventory(t, outbound[1])
	duskgoldQuantity := 0
	inventorySpears := 0
	for _, item := range inventory {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			duskgoldQuantity += item.Quantity
		}
		if item.TemplateID == "ironwood_spear" && item.ContainerKind == itemContainerInventory {
			inventorySpears++
		}
	}
	if duskgoldQuantity != 4 {
		t.Fatalf("expected vendor purchase to deduct duskgold down to 4, got %+v", inventory)
	}
	if inventorySpears != 1 {
		t.Fatalf("expected vendor purchase to add one inventory spear, got %+v", inventory)
	}

	persistedItems, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() error = %v", err)
	}
	persistedGold := 0
	persistedInventorySpears := 0
	for _, item := range persistedItems {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			persistedGold += item.Quantity
		}
		if item.TemplateID == "ironwood_spear" && item.ContainerKind == itemContainerInventory {
			persistedInventorySpears++
		}
	}
	if persistedGold != 4 || persistedInventorySpears != 1 {
		t.Fatalf("expected persisted vendor purchase state to match inventory delta, got %+v", persistedItems)
	}

	actionLogs, err := store.ActionLogs.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ActionLogs.ListByCharacterID() error = %v", err)
	}
	if len(actionLogs) != 1 {
		t.Fatalf("expected one vendor buy action log, got %+v", actionLogs)
	}
	log := actionLogs[0]
	if log.ActionType != "vendor_buy" || log.ItemInstanceID == "" || log.AccountID != character.AccountID || log.CurrencyAmount >= 0 {
		t.Fatalf("unexpected vendor buy audit metadata = %+v", log)
	}
}

func TestAttachedRuntimeBuyVendorOfferRejectsInsufficientFunds(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_vendor_2", AccountID: "acc_1", Name: "Vendor Reject Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	runtime := newAttachedRuntime("sess_vendor_2", character)
	runtime.position = runtime.knownEntities["npc_merchant"].Position

	outbound := runtime.processVendorCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_vendor_buy_2",
		CommandSeq:      1,
		Type:            "buy_item",
		Payload:         []byte(`{"vendor_offer_id":"merchant_spear_offer","quantity":2}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["reason_code"] != "economy.insufficient_funds" {
		t.Fatalf("expected ack followed by economy.insufficient_funds, got %+v", outbound)
	}
}

func TestAttachedRuntimeBuyVendorOfferSupportsStackableMaterialBundles(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_vendor_bundle_1", AccountID: "acc_1", Name: "Bundle Buyer", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	runtime := newAttachedRuntime("sess_vendor_bundle_1", character)
	runtime.position = runtime.knownEntities["npc_merchant"].Position

	outbound := runtime.processVendorCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_vendor_buy_bundle_1",
		CommandSeq:      1,
		Type:            "buy_item",
		Payload:         []byte(`{"vendor_offer_id":"merchant_ruin_shard_bundle","quantity":2}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from stackable buy_item, got %+v", outbound)
	}

	inventory := deltaInventory(t, outbound[1])
	duskgoldQuantity := 0
	ruinShardQuantity := 0
	for _, item := range inventory {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			duskgoldQuantity += item.Quantity
		}
		if item.TemplateID == "ruin_shard" && item.ContainerKind == itemContainerInventory {
			ruinShardQuantity += item.Quantity
		}
	}
	if duskgoldQuantity != 4 {
		t.Fatalf("expected stackable vendor bundle to deduct duskgold down to 4, got %+v", inventory)
	}
	if ruinShardQuantity != 8 {
		t.Fatalf("expected stackable vendor bundle to add eight ruin shards, got %+v", inventory)
	}

	actionLogs, err := store.ActionLogs.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ActionLogs.ListByCharacterID() error = %v", err)
	}
	if len(actionLogs) != 1 {
		t.Fatalf("expected one vendor buy action log, got %+v", actionLogs)
	}
	log := actionLogs[0]
	if log.ActionType != "vendor_buy" || log.Quantity != 8 || log.ItemInstanceID == "" || log.CurrencyAmount >= 0 {
		t.Fatalf("unexpected stackable vendor buy audit log = %+v", log)
	}
}

func TestAttachedRuntimeExchangeOfferConsumesMaterialsAndAddsInventoryItem(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_vendor_exchange_1", AccountID: "acc_1", Name: "Vendor Exchanger", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	runtime := newAttachedRuntime("sess_vendor_exchange_1", character)
	runtime.position = runtime.knownEntities["npc_merchant"].Position

	outbound := runtime.processVendorCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_vendor_exchange_1",
		CommandSeq:      1,
		Type:            "exchange_item",
		Payload:         []byte(`{"exchange_offer_id":"merchant_mantle_exchange","quantity":1}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from exchange_item, got %+v", outbound)
	}

	inventory := deltaInventory(t, outbound[1])
	duskgoldQuantity := 0
	inventoryMantles := 0
	for _, item := range inventory {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			duskgoldQuantity += item.Quantity
		}
		if item.TemplateID == "wardkeeper_mantle" && item.ContainerKind == itemContainerInventory {
			inventoryMantles++
		}
	}
	if duskgoldQuantity != 2 {
		t.Fatalf("expected exchange to deduct duskgold down to 2, got %+v", inventory)
	}
	if inventoryMantles != 1 {
		t.Fatalf("expected exchange to add one inventory mantle, got %+v", inventory)
	}

	actionLogs, err := store.ActionLogs.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ActionLogs.ListByCharacterID() error = %v", err)
	}
	if len(actionLogs) != 1 {
		t.Fatalf("expected one vendor exchange action log, got %+v", actionLogs)
	}
	log := actionLogs[0]
	if log.ActionType != "vendor_exchange" || log.Quantity != 1 || log.ItemInstanceID == "" || log.AccountID != character.AccountID || log.CurrencyAmount >= 0 {
		t.Fatalf("unexpected vendor exchange audit metadata = %+v", log)
	}
}

func TestAttachedRuntimeExchangeOfferRejectsInsufficientMaterials(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_vendor_exchange_2", AccountID: "acc_1", Name: "Exchange Reject Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	runtime := newAttachedRuntime("sess_vendor_exchange_2", character)
	runtime.position = runtime.knownEntities["npc_merchant"].Position

	outbound := runtime.processVendorCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_vendor_exchange_2",
		CommandSeq:      1,
		Type:            "exchange_item",
		Payload:         []byte(`{"exchange_offer_id":"merchant_mantle_exchange","quantity":2}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["reason_code"] != "economy.exchange_insufficient_materials" {
		t.Fatalf("expected ack followed by economy.exchange_insufficient_materials, got %+v", outbound)
	}
}

func TestAttachedRuntimeExchangeOfferConsumesShardMaterialsAndAddsGreaves(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_vendor_exchange_shards_1", AccountID: "acc_1", Name: "Shard Exchanger", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	items := append(initialCharacterItemSeed(character), CharacterItem{
		ID:            "item_ruin_shards_inventory",
		CharacterID:   character.ID,
		TemplateID:    "ruin_shard",
		Quantity:      6,
		ContainerKind: itemContainerInventory,
	})
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, items); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	runtime := newAttachedRuntime("sess_vendor_exchange_shards_1", character)
	runtime.position = runtime.knownEntities["npc_merchant"].Position

	outbound := runtime.processVendorCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_vendor_exchange_shards_1",
		CommandSeq:      1,
		Type:            "exchange_item",
		Payload:         []byte(`{"exchange_offer_id":"merchant_ruinbound_greaves_exchange","quantity":1}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from shard exchange_item, got %+v", outbound)
	}

	inventory := deltaInventory(t, outbound[1])
	ruinShardQuantity := 0
	greavesQuantity := 0
	for _, item := range inventory {
		if item.TemplateID == "ruin_shard" && item.ContainerKind == itemContainerInventory {
			ruinShardQuantity += item.Quantity
		}
		if item.TemplateID == "ruinbound_greaves" && item.ContainerKind == itemContainerInventory {
			greavesQuantity += item.Quantity
		}
	}
	if ruinShardQuantity != 0 {
		t.Fatalf("expected shard exchange to consume all six ruin shards, got %+v", inventory)
	}
	if greavesQuantity != 1 {
		t.Fatalf("expected shard exchange to add one inventory greaves item, got %+v", inventory)
	}

	actionLogs, err := store.ActionLogs.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ActionLogs.ListByCharacterID() error = %v", err)
	}
	if len(actionLogs) != 1 {
		t.Fatalf("expected one vendor exchange action log, got %+v", actionLogs)
	}
	log := actionLogs[0]
	if log.ActionType != "vendor_exchange" || log.Quantity != 1 || log.ItemInstanceID == "" || log.AccountID != character.AccountID || log.CurrencyAmount >= 0 {
		t.Fatalf("unexpected shard exchange audit metadata = %+v", log)
	}
}

func TestAttachedRuntimeSellVendorItemUsesBackendDerivedValueAndRemovesInventoryItem(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_vendor_sell_1", AccountID: "acc_1", Name: "Vendor Seller", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	items := initialCharacterItemSeed(character)
	items = append(items, CharacterItem{
		ID:            "item_vendor_sell_weapon",
		CharacterID:   character.ID,
		TemplateID:    "ironwood_spear",
		Quantity:      1,
		ContainerKind: itemContainerInventory,
	})
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, items); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	runtime := newAttachedRuntime("sess_vendor_sell_1", character)
	runtime.position = runtime.knownEntities["npc_merchant"].Position

	outbound := runtime.processVendorCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_vendor_sell_1",
		CommandSeq:      1,
		Type:            "sell_item",
		Payload:         []byte(`{"item_instance_id":"item_vendor_sell_weapon","quantity":1}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from sell_item, got %+v", outbound)
	}

	inventory := deltaInventory(t, outbound[1])
	duskgoldQuantity := 0
	inventorySpears := 0
	for _, item := range inventory {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			duskgoldQuantity += item.Quantity
		}
		if item.TemplateID == "ironwood_spear" && item.ContainerKind == itemContainerInventory {
			inventorySpears++
		}
	}
	if duskgoldQuantity != 16 {
		t.Fatalf("expected vendor sale to raise duskgold to 16, got %+v", inventory)
	}
	if inventorySpears != 0 {
		t.Fatalf("expected sold inventory spear to be removed, got %+v", inventory)
	}

	actionLogs, err := store.ActionLogs.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ActionLogs.ListByCharacterID() error = %v", err)
	}
	if len(actionLogs) != 1 {
		t.Fatalf("expected one vendor sell action log, got %+v", actionLogs)
	}
	if actionLogs[0].ActionType != "vendor_sell" ||
		actionLogs[0].CounterpartyEntity != "npc_merchant" ||
		actionLogs[0].ItemInstanceID != "item_vendor_sell_weapon" ||
		actionLogs[0].TemplateID != "ironwood_spear" ||
		actionLogs[0].Quantity != 1 ||
		actionLogs[0].CurrencyTemplateID != "duskgold" ||
		actionLogs[0].CurrencyAmount != 4 {
		t.Fatalf("unexpected vendor sell action log = %+v", actionLogs[0])
	}
}

func TestAttachedRuntimeSellVendorItemRejectsUnsellableCurrency(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_vendor_sell_2", AccountID: "acc_1", Name: "Vendor Reject Seller", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	items, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() setup error = %v", err)
	}
	duskgoldItemID := ""
	for _, item := range items {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			duskgoldItemID = item.ID
			break
		}
	}
	if duskgoldItemID == "" {
		t.Fatalf("expected duskgold item during setup")
	}

	runtime := newAttachedRuntime("sess_vendor_sell_2", character)
	runtime.position = runtime.knownEntities["npc_merchant"].Position

	outbound := runtime.processVendorCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_vendor_sell_2",
		CommandSeq:      1,
		Type:            "sell_item",
		Payload:         []byte(`{"item_instance_id":"` + duskgoldItemID + `","quantity":1}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["reason_code"] != "economy.sell_not_allowed" {
		t.Fatalf("expected ack followed by economy.sell_not_allowed, got %+v", outbound)
	}
}

func TestAttachedRuntimeWarehouseTransferMovesStackBetweenInventoryAndWarehouse(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_storage_1", AccountID: "acc_1", Name: "Storage Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	items, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() setup error = %v", err)
	}
	duskgoldItemID := ""
	for _, item := range items {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			duskgoldItemID = item.ID
			break
		}
	}
	if duskgoldItemID == "" {
		t.Fatalf("expected duskgold item during setup")
	}

	runtime := newAttachedRuntime("sess_storage_1", character)
	runtime.position = runtime.knownEntities[warehouseNPCEntityID].Position

	depositOutbound := runtime.processWarehouseCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_storage_deposit_1",
		CommandSeq:      1,
		Type:            "deposit_item",
		Payload:         []byte(`{"item_instance_id":"` + duskgoldItemID + `","quantity":2}`),
	})
	if len(depositOutbound) != 2 || depositOutbound[0]["kind"] != "ack" || depositOutbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from warehouse deposit, got %+v", depositOutbound)
	}

	inventory := deltaInventory(t, depositOutbound[1])
	warehouse := deltaWarehouse(t, depositOutbound[1])
	inventoryGold := 0
	warehouseGold := 0
	warehouseItemID := ""
	for _, item := range inventory {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			inventoryGold += item.Quantity
		}
	}
	for _, item := range warehouse {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerWarehouse {
			warehouseGold += item.Quantity
			warehouseItemID = item.ID
		}
	}
	if inventoryGold != 10 || warehouseGold != 2 {
		t.Fatalf("expected warehouse deposit to split gold into inventory=10 warehouse=2, got inventory=%+v warehouse=%+v", inventory, warehouse)
	}
	if warehouseItemID == "" {
		t.Fatalf("expected a warehouse duskgold item after deposit")
	}

	withdrawOutbound := runtime.processWarehouseCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_storage_withdraw_1",
		CommandSeq:      2,
		Type:            "withdraw_item",
		Payload:         []byte(`{"item_instance_id":"` + warehouseItemID + `","quantity":1}`),
	})
	if len(withdrawOutbound) != 2 || withdrawOutbound[0]["kind"] != "ack" || withdrawOutbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from warehouse withdraw, got %+v", withdrawOutbound)
	}

	inventory = deltaInventory(t, withdrawOutbound[1])
	warehouse = deltaWarehouse(t, withdrawOutbound[1])
	inventoryGold = 0
	warehouseGold = 0
	for _, item := range inventory {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			inventoryGold += item.Quantity
		}
	}
	for _, item := range warehouse {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerWarehouse {
			warehouseGold += item.Quantity
		}
	}
	if inventoryGold != 11 || warehouseGold != 1 {
		t.Fatalf("expected warehouse withdraw to restore inventory=11 warehouse=1, got inventory=%+v warehouse=%+v", inventory, warehouse)
	}

	storageTransfers, err := store.StorageTransfers.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("StorageTransfers.ListByCharacterID() error = %v", err)
	}
	if len(storageTransfers) != 2 {
		t.Fatalf("expected two warehouse transfer records, got %+v", storageTransfers)
	}

	var depositRecord *StorageTransferRecord
	var withdrawRecord *StorageTransferRecord
	for index := range storageTransfers {
		switch storageTransfers[index].TransferType {
		case "warehouse_deposit":
			depositRecord = &storageTransfers[index]
		case "warehouse_withdraw":
			withdrawRecord = &storageTransfers[index]
		}
	}
	if depositRecord == nil ||
		depositRecord.SourceItemID != duskgoldItemID ||
		depositRecord.TemplateID != "duskgold" ||
		depositRecord.Quantity != 2 ||
		depositRecord.FromContainerKind != itemContainerInventory ||
		depositRecord.ToContainerKind != itemContainerWarehouse ||
		depositRecord.CounterpartyEntity != warehouseNPCEntityID {
		t.Fatalf("unexpected warehouse deposit audit record = %+v", depositRecord)
	}
	if withdrawRecord == nil ||
		withdrawRecord.SourceItemID != warehouseItemID ||
		withdrawRecord.TemplateID != "duskgold" ||
		withdrawRecord.Quantity != 1 ||
		withdrawRecord.FromContainerKind != itemContainerWarehouse ||
		withdrawRecord.ToContainerKind != itemContainerInventory ||
		withdrawRecord.CounterpartyEntity != warehouseNPCEntityID {
		t.Fatalf("unexpected warehouse withdraw audit record = %+v", withdrawRecord)
	}
}

func TestAttachedRuntimeWarehouseTransferRejectsOutOfRangeActor(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_storage_2", AccountID: "acc_1", Name: "Storage Reject Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	items, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() setup error = %v", err)
	}
	duskgoldItemID := ""
	for _, item := range items {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			duskgoldItemID = item.ID
			break
		}
	}
	if duskgoldItemID == "" {
		t.Fatalf("expected duskgold item during setup")
	}

	runtime := newAttachedRuntime("sess_storage_2", character)

	outbound := runtime.processWarehouseCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_storage_deposit_2",
		CommandSeq:      1,
		Type:            "deposit_item",
		Payload:         []byte(`{"item_instance_id":"` + duskgoldItemID + `","quantity":1}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["reason_code"] != "economy.storage_out_of_range" {
		t.Fatalf("expected ack followed by economy.storage_out_of_range, got %+v", outbound)
	}
}

func TestAttachedRuntimeWarehouseTransferMovesStackableMaterialBetweenInventoryAndWarehouse(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_storage_shards_1", AccountID: "acc_1", Name: "Storage Shards", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	items := append(initialCharacterItemSeed(character), CharacterItem{
		ID:            "item_ruin_shards_storage",
		CharacterID:   character.ID,
		TemplateID:    "ruin_shard",
		Quantity:      5,
		ContainerKind: itemContainerInventory,
	})
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, items); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	runtime := newAttachedRuntime("sess_storage_shards_1", character)
	runtime.position = runtime.knownEntities[warehouseNPCEntityID].Position

	depositOutbound := runtime.processWarehouseCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_storage_shards_deposit_1",
		CommandSeq:      1,
		Type:            "deposit_item",
		Payload:         []byte(`{"item_instance_id":"item_ruin_shards_storage","quantity":2}`),
	})
	if len(depositOutbound) != 2 || depositOutbound[0]["kind"] != "ack" || depositOutbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from shard warehouse deposit, got %+v", depositOutbound)
	}

	inventory := deltaInventory(t, depositOutbound[1])
	warehouse := deltaWarehouse(t, depositOutbound[1])
	inventoryShards := 0
	warehouseShards := 0
	warehouseItemID := ""
	for _, item := range inventory {
		if item.TemplateID == "ruin_shard" && item.ContainerKind == itemContainerInventory {
			inventoryShards += item.Quantity
		}
	}
	for _, item := range warehouse {
		if item.TemplateID == "ruin_shard" && item.ContainerKind == itemContainerWarehouse {
			warehouseShards += item.Quantity
			warehouseItemID = item.ID
		}
	}
	if inventoryShards != 3 || warehouseShards != 2 {
		t.Fatalf("expected shard warehouse deposit to split inventory=3 warehouse=2, got inventory=%+v warehouse=%+v", inventory, warehouse)
	}
	if warehouseItemID == "" {
		t.Fatalf("expected a warehouse ruin shard item after deposit")
	}

	withdrawOutbound := runtime.processWarehouseCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_storage_shards_withdraw_1",
		CommandSeq:      2,
		Type:            "withdraw_item",
		Payload:         []byte(`{"item_instance_id":"` + warehouseItemID + `","quantity":1}`),
	})
	if len(withdrawOutbound) != 2 || withdrawOutbound[0]["kind"] != "ack" || withdrawOutbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from shard warehouse withdraw, got %+v", withdrawOutbound)
	}

	inventory = deltaInventory(t, withdrawOutbound[1])
	warehouse = deltaWarehouse(t, withdrawOutbound[1])
	inventoryShards = 0
	warehouseShards = 0
	for _, item := range inventory {
		if item.TemplateID == "ruin_shard" && item.ContainerKind == itemContainerInventory {
			inventoryShards += item.Quantity
		}
	}
	for _, item := range warehouse {
		if item.TemplateID == "ruin_shard" && item.ContainerKind == itemContainerWarehouse {
			warehouseShards += item.Quantity
		}
	}
	if inventoryShards != 4 || warehouseShards != 1 {
		t.Fatalf("expected shard warehouse withdraw to restore inventory=4 warehouse=1, got inventory=%+v warehouse=%+v", inventory, warehouse)
	}
}

func TestAttachedRuntimeEquipItemRejectsNonEquippableInventoryItem(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_equip_2", AccountID: "acc_1", Name: "Equip Reject Hero", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	items, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() setup error = %v", err)
	}
	duskgoldItemID := ""
	for _, item := range items {
		if item.TemplateID == "duskgold" {
			duskgoldItemID = item.ID
			break
		}
	}
	if duskgoldItemID == "" {
		t.Fatalf("expected duskgold item during setup")
	}

	outbound := newAttachedRuntime("sess_equip_2", character).processItemCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_equip_1",
		CommandSeq:      1,
		Type:            "equip_item",
		Payload:         []byte(`{"item_instance_id":"` + duskgoldItemID + `"}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["reason_code"] != "inventory.item_not_equippable" {
		t.Fatalf("expected ack followed by inventory.item_not_equippable, got %+v", outbound)
	}
}

func TestAttachedRuntimeEquipItemSwapsExistingSlotOccupant(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_equip_3", AccountID: "acc_1", Name: "Equip Swap Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}
	backend := store.Items.(memoryCharacterItemRepo).backend
	backend.mu.Lock()
	backend.characterItems[character.ID] = append(backend.characterItems[character.ID], CharacterItem{
		ID:            "item_spare_weapon",
		CharacterID:   character.ID,
		TemplateID:    "ironwood_spear",
		Quantity:      1,
		ContainerKind: itemContainerInventory,
	})
	backend.mu.Unlock()

	runtime := newAttachedRuntime("sess_equip_3", character)
	outbound := runtime.processItemCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_equip_1",
		CommandSeq:      1,
		Type:            "equip_item",
		Payload:         []byte(`{"item_instance_id":"item_spare_weapon"}`),
	})
	if len(outbound) != 2 || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from equip swap, got %+v", outbound)
	}

	persistedItems, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() error = %v", err)
	}
	equippedWeaponCount := 0
	inventoryWeaponCount := 0
	for _, item := range persistedItems {
		if item.TemplateID != "ironwood_spear" {
			continue
		}
		if item.ContainerKind == itemContainerEquipment && item.EquipSlot == equipSlotWeapon {
			equippedWeaponCount++
		}
		if item.ContainerKind == itemContainerInventory {
			inventoryWeaponCount++
		}
	}
	if equippedWeaponCount != 1 || inventoryWeaponCount != 1 {
		t.Fatalf("expected equip swap to leave one spear equipped and one in inventory, got equipped=%d inventory=%d", equippedWeaponCount, inventoryWeaponCount)
	}
	stats := deltaSelfStats(t, outbound[1])
	if stats.Attack != 27 || stats.Defense != 18 {
		t.Fatalf("expected swap to preserve derived stats, got %+v", stats)
	}
}

func TestAttachedRuntimeUnequipItemMovesEquippedItemBackToInventory(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_equip_4", AccountID: "acc_1", Name: "Unequip Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	runtime := newAttachedRuntime("sess_equip_4", character)
	outbound := runtime.processItemCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_unequip_1",
		CommandSeq:      1,
		Type:            "unequip_item",
		Payload:         []byte(`{"equip_slot":"weapon"}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from unequip_item, got %+v", outbound)
	}

	persistedItems, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() error = %v", err)
	}
	weaponInInventory := false
	for _, item := range persistedItems {
		if item.TemplateID == "ironwood_spear" && item.ContainerKind == itemContainerInventory && item.EquipSlot == "" {
			weaponInInventory = true
		}
	}
	if !weaponInInventory {
		t.Fatalf("expected unequip to return weapon to inventory, got %+v", persistedItems)
	}
	stats := deltaSelfStats(t, outbound[1])
	if stats.Attack != 17 || stats.Defense != 18 {
		t.Fatalf("expected unequip to revert weapon attack bonus only, got %+v", stats)
	}
}

func TestAttachedRuntimeUnequipChestItemRevertsDefenseBonus(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_equip_5", AccountID: "acc_1", Name: "Unequip Chest Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	runtime := newAttachedRuntime("sess_equip_5", character)
	outbound := runtime.processItemCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_unequip_chest",
		CommandSeq:      1,
		Type:            "unequip_item",
		Payload:         []byte(`{"equip_slot":"chest"}`),
	})
	if len(outbound) != 2 || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from chest unequip, got %+v", outbound)
	}

	stats := deltaSelfStats(t, outbound[1])
	if stats.Attack != 27 || stats.Defense != 12 || stats.MaxHP != 130 {
		t.Fatalf("expected chest unequip to revert defense and max hp bonus, got %+v", stats)
	}
}

func TestAttachedRuntimeSplitItemStackCreatesSecondInventoryStack(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_stack_split_1", AccountID: "acc_1", Name: "Stack Split Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	items, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() setup error = %v", err)
	}
	duskgoldItemID := ""
	for _, item := range items {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			duskgoldItemID = item.ID
			break
		}
	}
	if duskgoldItemID == "" {
		t.Fatalf("expected duskgold item in inventory during setup")
	}

	outbound := newAttachedRuntime("sess_stack_split_1", character).processItemCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_stack_split_1",
		CommandSeq:      1,
		Type:            "split_item_stack",
		Payload:         []byte(`{"item_instance_id":"` + duskgoldItemID + `","quantity":1}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from split_item_stack, got %+v", outbound)
	}

	inventory := deltaInventory(t, outbound[1])
	duskgoldStacks := 0
	totalDuskgold := 0
	foundSingleSplit := false
	for _, item := range inventory {
		if item.TemplateID != "duskgold" {
			continue
		}
		duskgoldStacks++
		totalDuskgold += item.Quantity
		if item.ID != duskgoldItemID && item.Quantity == 1 {
			foundSingleSplit = true
		}
	}
	if duskgoldStacks != 2 || totalDuskgold != 12 || !foundSingleSplit {
		t.Fatalf("expected split to produce two duskgold stacks totaling 12, got %+v", inventory)
	}

	persistedItems, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() error = %v", err)
	}
	persistedStacks := 0
	persistedTotal := 0
	for _, item := range persistedItems {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			persistedStacks++
			persistedTotal += item.Quantity
		}
	}
	if persistedStacks != 2 || persistedTotal != 12 {
		t.Fatalf("expected persisted split stacks totaling 12, got %+v", persistedItems)
	}
}

func TestAttachedRuntimeMergeItemStacksCombinesSplitStacks(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_stack_merge_1", AccountID: "acc_1", Name: "Stack Merge Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	items, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() setup error = %v", err)
	}
	duskgoldItemID := ""
	for _, item := range items {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			duskgoldItemID = item.ID
			break
		}
	}
	if duskgoldItemID == "" {
		t.Fatalf("expected duskgold item in inventory during setup")
	}

	splitItems, err := store.Items.SplitStack(context.Background(), character.ID, duskgoldItemID, 1)
	if err != nil {
		t.Fatalf("Items.SplitStack() setup error = %v", err)
	}

	sourceItemID := ""
	targetItemID := ""
	for _, item := range splitItems {
		if item.TemplateID != "duskgold" || item.ContainerKind != itemContainerInventory {
			continue
		}
		if item.Quantity == 1 {
			sourceItemID = item.ID
		}
		if item.Quantity == 11 {
			targetItemID = item.ID
		}
	}
	if sourceItemID == "" || targetItemID == "" {
		t.Fatalf("expected split setup to produce 1 and 11 duskgold stacks, got %+v", splitItems)
	}

	outbound := newAttachedRuntime("sess_stack_merge_1", character).processItemCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_stack_merge_1",
		CommandSeq:      1,
		Type:            "merge_item_stacks",
		Payload:         []byte(`{"source_item_instance_id":"` + sourceItemID + `","target_item_instance_id":"` + targetItemID + `"}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from merge_item_stacks, got %+v", outbound)
	}

	inventory := deltaInventory(t, outbound[1])
	duskgoldStacks := 0
	for _, item := range inventory {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			duskgoldStacks++
			if item.ID != targetItemID || item.Quantity != 12 {
				t.Fatalf("expected merged target stack quantity 12, got %+v", item)
			}
		}
	}
	if duskgoldStacks != 1 {
		t.Fatalf("expected exactly one duskgold stack after merge, got %+v", inventory)
	}

	persistedItems, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() error = %v", err)
	}
	persistedStacks := 0
	for _, item := range persistedItems {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			persistedStacks++
			if item.ID != targetItemID || item.Quantity != 12 {
				t.Fatalf("expected persisted merged target stack quantity 12, got %+v", item)
			}
		}
	}
	if persistedStacks != 1 {
		t.Fatalf("expected exactly one persisted duskgold stack after merge, got %+v", persistedItems)
	}
}

func TestAttachedRuntimeSplitItemStackRejectsInvalidQuantity(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_stack_split_2", AccountID: "acc_1", Name: "Split Reject Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	items, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() setup error = %v", err)
	}
	duskgoldItemID := ""
	for _, item := range items {
		if item.TemplateID == "duskgold" && item.ContainerKind == itemContainerInventory {
			duskgoldItemID = item.ID
			break
		}
	}
	if duskgoldItemID == "" {
		t.Fatalf("expected duskgold item in inventory during setup")
	}

	outbound := newAttachedRuntime("sess_stack_split_2", character).processItemCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_stack_split_2",
		CommandSeq:      1,
		Type:            "split_item_stack",
		Payload:         []byte(`{"item_instance_id":"` + duskgoldItemID + `","quantity":12}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["reason_code"] != "inventory.split_invalid_quantity" {
		t.Fatalf("expected ack followed by inventory.split_invalid_quantity, got %+v", outbound)
	}
}

func TestAttachedRuntimeMergeItemStacksRejectsDifferentTemplates(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_stack_merge_2", AccountID: "acc_1", Name: "Merge Reject Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	items, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID() setup error = %v", err)
	}
	duskgoldItemID := ""
	glovesItemID := ""
	for _, item := range items {
		if item.ContainerKind != itemContainerInventory {
			continue
		}
		if item.TemplateID == "duskgold" {
			duskgoldItemID = item.ID
		}
		if item.TemplateID == "watcher_gloves" {
			glovesItemID = item.ID
		}
	}
	if duskgoldItemID == "" || glovesItemID == "" {
		t.Fatalf("expected duskgold and gloves inventory items during setup, got %+v", items)
	}

	outbound := newAttachedRuntime("sess_stack_merge_2", character).processItemCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_stack_merge_2",
		CommandSeq:      1,
		Type:            "merge_item_stacks",
		Payload:         []byte(`{"source_item_instance_id":"` + duskgoldItemID + `","target_item_instance_id":"` + glovesItemID + `"}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["reason_code"] != "inventory.merge_invalid" {
		t.Fatalf("expected ack followed by inventory.merge_invalid, got %+v", outbound)
	}
}

func TestAttachedRuntimeUseSkillAppliesDerivedAttackBonusFromEquippedWeapon(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_equip_6", AccountID: "acc_1", Name: "Combat Stats Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	items, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}

	runtime := newAttachedRuntime("sess_equip_6", character)
	runtime.derivedStats = deriveCharacterStats(character, items)
	moveRuntimeNearMob(runtime, "mob_1")

	withWeapon := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_skill_with_weapon",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})
	if len(withWeapon) != 2 || withWeapon[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta for weapon skill use, got %+v", withWeapon)
	}
	withWeaponEntities, ok := withWeapon[1]["entities"].([]map[string]any)
	if !ok || len(withWeaponEntities) != 1 {
		t.Fatalf("expected single entity patch, got %+v", withWeapon[1]["entities"])
	}
	withWeaponHP, ok := withWeaponEntities[0]["hp"].(int)
	if !ok {
		t.Fatalf("expected integer hp patch, got %+v", withWeaponEntities[0]["hp"])
	}

	if _, err := store.Items.UnequipItem(context.Background(), character.ID, equipSlotWeapon); err != nil {
		t.Fatalf("Items.UnequipItem() error = %v", err)
	}
	withoutWeaponItems, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() after unequip error = %v", err)
	}
	withoutWeaponRuntime := newAttachedRuntime("sess_equip_7", character)
	withoutWeaponRuntime.derivedStats = deriveCharacterStats(character, withoutWeaponItems)
	moveRuntimeNearMob(withoutWeaponRuntime, "mob_1")

	withoutWeapon := withoutWeaponRuntime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_skill_without_weapon",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})
	if len(withoutWeapon) != 2 || withoutWeapon[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta for unequipped skill use, got %+v", withoutWeapon)
	}
	withoutWeaponEntities, ok := withoutWeapon[1]["entities"].([]map[string]any)
	if !ok || len(withoutWeaponEntities) != 1 {
		t.Fatalf("expected single entity patch without weapon, got %+v", withoutWeapon[1]["entities"])
	}
	withoutWeaponHP, ok := withoutWeaponEntities[0]["hp"].(int)
	if !ok {
		t.Fatalf("expected integer hp patch without weapon, got %+v", withoutWeaponEntities[0]["hp"])
	}

	if withWeaponHP >= withoutWeaponHP {
		t.Fatalf("expected equipped weapon to deal more damage, got with=%d without=%d", withWeaponHP, withoutWeaponHP)
	}
}

func TestAttachedRuntimeUseSkillMitigatesIncomingDamageWithDerivedDefense(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := &Character{ID: "char_equip_8", AccountID: "acc_1", Name: "Defense Stats Hero", BaseClass: "Fighter", LastRegionID: "dawn_plaza"}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}

	withChestItems, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}

	withChestRuntime := newAttachedRuntime("sess_equip_8", character)
	withChestRuntime.derivedStats = deriveCharacterStats(character, withChestItems)
	moveRuntimeNearMob(withChestRuntime, "mob_1")

	withChestOutbound := withChestRuntime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_skill_with_chest",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})
	if len(withChestOutbound) != 2 || withChestOutbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta for chest-equipped skill use, got %+v", withChestOutbound)
	}
	withChestStats := deltaSelfStats(t, withChestOutbound[1])
	withChestHP := deltaSelfHP(t, withChestOutbound[1])
	if withChestStats.Defense != 18 {
		t.Fatalf("expected equipped chest defense 18, got %+v", withChestStats)
	}
	if withChestHP != 121 {
		t.Fatalf("expected chest-equipped retaliation to reduce hp to 121, got %d", withChestHP)
	}

	if _, err := store.Items.UnequipItem(context.Background(), character.ID, equipSlotChest); err != nil {
		t.Fatalf("Items.UnequipItem() chest error = %v", err)
	}
	withoutChestItems, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() after chest unequip error = %v", err)
	}

	withoutChestRuntime := newAttachedRuntime("sess_equip_9", character)
	withoutChestRuntime.derivedStats = deriveCharacterStats(character, withoutChestItems)
	moveRuntimeNearMob(withoutChestRuntime, "mob_1")

	withoutChestOutbound := withoutChestRuntime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_skill_without_chest",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})
	if len(withoutChestOutbound) != 2 || withoutChestOutbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta for chest-unequipped skill use, got %+v", withoutChestOutbound)
	}
	withoutChestStats := deltaSelfStats(t, withoutChestOutbound[1])
	withoutChestHP := deltaSelfHP(t, withoutChestOutbound[1])
	if withoutChestStats.Defense != 12 {
		t.Fatalf("expected unequipped chest defense 12, got %+v", withoutChestStats)
	}
	if withoutChestHP != 119 {
		t.Fatalf("expected chest-unequipped retaliation to reduce hp to 119, got %d", withoutChestHP)
	}
	if withChestHP <= withoutChestHP {
		t.Fatalf("expected equipped chest to mitigate more damage, got with=%d without=%d", withChestHP, withoutChestHP)
	}
}

func TestAttachedRuntimePlayerDeathBlocksIncompatibleCommandsAndRespawns(t *testing.T) {
	runtime := newAttachedRuntime("sess_dead_1", &Character{ID: "char_dead_1", LastRegionID: "dawn_plaza"})
	runtime.currentHP = 1
	runtime.derivedStats = CharacterDerivedStats{
		MaxHP:     122,
		MaxMP:     58,
		Attack:    17,
		Defense:   0,
		MoveSpeed: 3.225,
	}
	moveRuntimeNearMob(runtime, "mob_1")

	deathOutbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_death",
		CommandSeq:      1,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})
	if len(deathOutbound) != 2 || deathOutbound[1]["kind"] != "delta" {
		t.Fatalf("expected death path to emit ack and delta, got %+v", deathOutbound)
	}
	if hp := deltaSelfHP(t, deathOutbound[1]); hp != 0 {
		t.Fatalf("expected death delta hp 0, got %d", hp)
	}
	if !deltaSelfDead(t, deathOutbound[1]) {
		t.Fatalf("expected death delta to mark actor dead")
	}
	checkpointRegionID, checkpointPosition := runtime.characterWorldState()
	if checkpointRegionID != "dawn_plaza" || checkpointPosition != runtime.respawnPosition {
		t.Fatalf("expected death checkpoint to resolve to respawn position, got region=%s position=%+v", checkpointRegionID, checkpointPosition)
	}
	checkpointLevel, checkpointXP, checkpointCP, checkpointHP, checkpointMP := runtime.characterProgressionState()
	if checkpointLevel != 1 || checkpointXP != 0 || checkpointCP != runtime.derivedStats.MaxCP || checkpointHP != runtime.derivedStats.MaxHP || checkpointMP != runtime.derivedStats.MaxMP {
		t.Fatalf("expected death checkpoint to restore full progression state, got level=%d xp=%d cp=%d hp=%d mp=%d", checkpointLevel, checkpointXP, checkpointCP, checkpointHP, checkpointMP)
	}

	rejectMoveOutbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_move_while_dead",
		CommandSeq:      2,
		Type:            "move_intent",
		Payload:         []byte(`{"point":{"x":0,"z":0}}`),
	})
	if len(rejectMoveOutbound) != 2 || rejectMoveOutbound[0]["kind"] != "ack" || rejectMoveOutbound[1]["reason_code"] != "combat.actor_dead" {
		t.Fatalf("expected ack and combat.actor_dead reject while dead, got %+v", rejectMoveOutbound)
	}

	rejectSkillOutbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_skill_while_dead",
		CommandSeq:      3,
		Type:            "use_skill",
		Payload:         []byte(`{"skill_id":"crescent_strike","target_id":"mob_1"}`),
	})
	if len(rejectSkillOutbound) != 2 || rejectSkillOutbound[0]["kind"] != "ack" || rejectSkillOutbound[1]["reason_code"] != "combat.actor_dead" {
		t.Fatalf("expected ack and combat.actor_dead reject for skill while dead, got %+v", rejectSkillOutbound)
	}

	respawnOutbound := runtime.collectLifecycleMessages(time.Now().Add(playerRespawnDelay + 10*time.Millisecond))
	if len(respawnOutbound) != 1 || respawnOutbound[0]["kind"] != "delta" {
		t.Fatalf("expected respawn lifecycle to emit delta, got %+v", respawnOutbound)
	}
	if hp := deltaSelfHP(t, respawnOutbound[0]); hp != 122 {
		t.Fatalf("expected respawn delta hp 122, got %d", hp)
	}
	if deltaSelfDead(t, respawnOutbound[0]) {
		t.Fatalf("expected respawn delta to clear dead state")
	}

	postRespawnMoveOutbound := runtime.processCommand(commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_move_after_respawn",
		CommandSeq:      4,
		Type:            "move_intent",
		Payload:         []byte(`{"point":{"x":0,"z":0}}`),
	})
	if len(postRespawnMoveOutbound) != 2 || postRespawnMoveOutbound[0]["kind"] != "ack" || postRespawnMoveOutbound[1]["kind"] != "delta" {
		t.Fatalf("expected move to succeed after respawn, got %+v", postRespawnMoveOutbound)
	}
}

func TestAttachedRuntimeInteractNpcProjectsQuestInteraction(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := createDedupTestCharacter(t, store, "char_quest_interaction")
	runtime := newAttachedRuntime("sess_quest_interaction", character)
	runtime.position = runtime.knownEntities["npc_wardkeeper"].Position

	outbound := runtime.processNPCCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_interact_npc",
		CommandSeq:      1,
		Type:            "interact_npc",
		Payload:         []byte(`{"npc_id":"npc_wardkeeper"}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from interact_npc, got %+v", outbound)
	}

	quest := deltaSelfQuest(t, outbound[1])
	if quest.ID != keeperRequestQuestDefinition.ID || quest.Status != questStatusAvailable || quest.Progress != 0 {
		t.Fatalf("expected available keeper quest snapshot, got %+v", quest)
	}
	interaction := deltaSelfNPCInteraction(t, outbound[1])
	if interaction == nil || interaction.NPCID != "npc_wardkeeper" || interaction.Kind != npcInteractionWardkeeperNew {
		t.Fatalf("expected wardkeeper available interaction, got %+v", interaction)
	}
}

func TestAttachedRuntimeAcceptTaskPersistsQuestState(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := createDedupTestCharacter(t, store, "char_quest_accept")
	runtime := newAttachedRuntime("sess_quest_accept", character)
	runtime.position = runtime.knownEntities["npc_wardkeeper"].Position

	outbound := runtime.processNPCCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_accept_task",
		CommandSeq:      1,
		Type:            "interact_npc",
		Payload:         []byte(`{"npc_id":"npc_wardkeeper","action_id":"accept_task"}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from accept_task, got %+v", outbound)
	}

	quest := deltaSelfQuest(t, outbound[1])
	if quest.Status != questStatusActive || quest.Progress != 0 {
		t.Fatalf("expected accepted quest to become active, got %+v", quest)
	}
	if interaction := deltaSelfNPCInteraction(t, outbound[1]); interaction != nil {
		t.Fatalf("expected accept_task delta to clear npc interaction, got %+v", interaction)
	}

	persisted, err := store.CharacterQuests.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("CharacterQuests.ListByCharacterID() error = %v", err)
	}
	primary := primaryQuestState(persisted, character.ID)
	if primary.Status != questStatusActive || primary.Progress != 0 {
		t.Fatalf("expected persisted quest to become active, got %+v", primary)
	}
}

func TestAttachedRuntimeTurnInTaskGrantsRewardAndPersistsCompletion(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := createDedupTestCharacter(t, store, "char_quest_turnin")
	runtime := newAttachedRuntime("sess_quest_turnin", character)
	runtime.position = runtime.knownEntities["npc_wardkeeper"].Position

	readyQuest := defaultCharacterQuestState()
	readyQuest.CharacterID = character.ID
	readyQuest.Status = questStatusReadyToTurnIn
	readyQuest.Progress = keeperRequestQuestDefinition.Goal
	if err := store.CharacterQuests.UpsertByCharacterID(context.Background(), readyQuest); err != nil {
		t.Fatalf("CharacterQuests.UpsertByCharacterID() error = %v", err)
	}
	runtime.loadQuestState([]CharacterQuestState{readyQuest})

	outbound := runtime.processNPCCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_turn_in_task",
		CommandSeq:      1,
		Type:            "interact_npc",
		Payload:         []byte(`{"npc_id":"npc_wardkeeper","action_id":"turn_in_task"}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from turn_in_task, got %+v", outbound)
	}

	quest := deltaSelfQuest(t, outbound[1])
	if quest.Status != questStatusCompleted || quest.Progress != keeperRequestQuestDefinition.Goal {
		t.Fatalf("expected completed quest snapshot after turn-in, got %+v", quest)
	}
	if interaction := deltaSelfNPCInteraction(t, outbound[1]); interaction != nil {
		t.Fatalf("expected turn_in_task delta to clear npc interaction, got %+v", interaction)
	}

	inventory := deltaInventory(t, outbound[1])
	if countItemsByTemplate(inventory, keeperRequestQuestDefinition.RewardTemplate) != 1 {
		t.Fatalf("expected quest reward to appear once in inventory delta, got %+v", inventory)
	}

	persistedQuests, err := store.CharacterQuests.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("CharacterQuests.ListByCharacterID() error = %v", err)
	}
	if primary := primaryQuestState(persistedQuests, character.ID); primary.Status != questStatusCompleted {
		t.Fatalf("expected persisted quest to be completed, got %+v", primary)
	}

	persistedItems, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}
	if countItemsByTemplate(persistedItems, keeperRequestQuestDefinition.RewardTemplate) != 2 {
		t.Fatalf("expected starter chest plus reward mantle after turn-in, got %+v", persistedItems)
	}
}

func TestAttachedRuntimeUseItemConsumesHealingPotionAndHeals(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := createDedupTestCharacter(t, store, "char_use_item")
	items, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}

	healingPotionID := ""
	for _, item := range items {
		if item.TemplateID == "healing_potion" && item.ContainerKind == itemContainerInventory {
			healingPotionID = item.ID
			break
		}
	}
	if healingPotionID == "" {
		t.Fatalf("expected seeded healing potion in inventory, got %+v", items)
	}

	runtime := newAttachedRuntime("sess_use_item", character)
	runtime.derivedStats = deriveCharacterStats(character, items)
	runtime.currentHP = runtime.derivedStats.MaxHP - 50

	outbound := runtime.processItemCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_use_item",
		CommandSeq:      1,
		Type:            "use_item",
		Payload:         []byte(`{"item_instance_id":"` + healingPotionID + `"}`),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta from use_item, got %+v", outbound)
	}

	expectedHP := runtime.derivedStats.MaxHP - 5
	if hp := deltaSelfHP(t, outbound[1]); hp != expectedHP {
		t.Fatalf("expected healed hp %d after use_item, got %d", expectedHP, hp)
	}
	if countItemsByTemplate(deltaInventory(t, outbound[1]), "healing_potion") != 2 {
		t.Fatalf("expected delta inventory to contain two remaining healing potions, got %+v", deltaInventory(t, outbound[1]))
	}

	persistedItems, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}
	if countItemsByTemplate(persistedItems, "healing_potion") != 2 {
		t.Fatalf("expected persisted healing potion count 2 after use_item, got %+v", persistedItems)
	}
}

func TestAttachedRuntimePersistsMultitypeHotbarState(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := createDedupTestCharacter(t, store, "char_hotbar_runtime")
	runtime := newAttachedRuntime("sess_hotbar_runtime", character)
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

	outbound := runtime.processHotbarCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_hotbar_runtime",
		CommandSeq:      1,
		Type:            "set_hotbar_state",
		Payload: hotbarSnapshotPayload(t, 2,
			CharacterHotbarSlot{SlotIndex: 0, EntryType: "action", ActionID: "basic_attack"},
			CharacterHotbarSlot{SlotIndex: 1, EntryType: "item", ItemInstanceID: itemID},
			CharacterHotbarSlot{SlotIndex: 2, EntryType: "skill", SkillID: "crescent_strike"},
			CharacterHotbarSlot{SlotIndex: 3},
		),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected hotbar update to emit ack and delta, got %+v", outbound)
	}

	persisted, err := store.CharacterHotbars.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("CharacterHotbars.ListByCharacterID() error = %v", err)
	}
	if persisted.OpenBarCount != 2 {
		t.Fatalf("expected persisted open_bar_count 2, got %+v", persisted)
	}
	slots := map[int]CharacterHotbarSlot{}
	for _, slot := range persisted.Slots {
		slots[slot.SlotIndex] = slot
	}
	if slots[0].EntryType != "action" || slots[0].ActionID != "basic_attack" {
		t.Fatalf("expected slot 0 action binding, got %+v", slots[0])
	}
	if slots[1].EntryType != "item" || slots[1].ItemInstanceID != itemID {
		t.Fatalf("expected slot 1 item binding, got %+v", slots[1])
	}
	if slots[2].EntryType != "skill" || slots[2].SkillID != "crescent_strike" {
		t.Fatalf("expected slot 2 skill binding, got %+v", slots[2])
	}
	if slots[3].EntryType != "" {
		t.Fatalf("expected slot 3 to be empty, got %+v", slots[3])
	}
}

func TestDefaultCharacterHotbarExcludesUnavailableLevelLockedSkills(t *testing.T) {
	levelOne := defaultCharacterHotbarState(&Character{
		ID:        "char_hotbar_level_1",
		BaseClass: "Fighter",
		Level:     1,
	})
	if levelOne.Slots[0].EntryType != "skill" || levelOne.Slots[0].SkillID != "crescent_strike" {
		t.Fatalf("expected level 1 starter skill in slot 0, got %+v", levelOne.Slots[0])
	}
	if levelOne.Slots[1].EntryType != "" || levelOne.Slots[1].SkillID != "" {
		t.Fatalf("expected level 1 default slot 1 to stay empty, got %+v", levelOne.Slots[1])
	}

	normalized := normalizeCharacterHotbarState(CharacterHotbarState{
		OpenBarCount: 1,
		Slots: []CharacterHotbarSlot{
			{SlotIndex: 0, EntryType: "action", ActionID: "basic_attack"},
		},
	}, &Character{ID: "char_hotbar_normalized", BaseClass: "Fighter", Level: 1})
	if normalized.Slots[0].EntryType != "action" || normalized.Slots[0].ActionID != "basic_attack" {
		t.Fatalf("expected overridden action in slot 0, got %+v", normalized.Slots[0])
	}
	if normalized.Slots[1].EntryType != "" || normalized.Slots[1].SkillID != "" {
		t.Fatalf("expected normalized omitted slot 1 not to inherit locked skill, got %+v", normalized.Slots[1])
	}
}

func TestAttachedRuntimeRejectsPassiveSkillHotbarBinding(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := createDedupTestCharacter(t, store, "char_hotbar_passive")
	runtime := newAttachedRuntime("sess_hotbar_passive", character)

	outbound := runtime.processHotbarCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_hotbar_passive",
		CommandSeq:      1,
		Type:            "set_hotbar_state",
		Payload: hotbarSnapshotPayload(t, 1,
			CharacterHotbarSlot{SlotIndex: 0, EntryType: "skill", SkillID: "iron_will"},
		),
	})
	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["reason_code"] != "hotbar.skill_not_available" {
		t.Fatalf("expected passive skill hotbar binding to be rejected, got %+v", outbound)
	}
}

func TestAttachedRuntimeTameMobCreatesOwnedSummonedPet(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := createDedupTestCharacter(t, store, "char_pet_tame")
	runtime := newAttachedRuntime("sess_pet_tame", character)
	moveRuntimeNearMob(runtime, "mob_1")

	outbound := runtime.processPetCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_pet_tame",
		CommandSeq:      1,
		Type:            "tame_mob",
		Payload:         []byte(`{"target_id":"mob_1"}`),
	})

	if len(outbound) != 4 {
		t.Fatalf("expected ack, delta, mob disappear, pet appear; got %+v", outbound)
	}
	if outbound[0]["kind"] != "ack" || outbound[1]["kind"] != "delta" {
		t.Fatalf("expected ack and delta first, got %+v", outbound)
	}
	if outbound[2]["kind"] != "entity_disappear" || outbound[2]["reason"] != entityDisappearTamed {
		t.Fatalf("expected tamed mob disappear, got %+v", outbound[2])
	}
	if outbound[3]["kind"] != "entity_appear" {
		t.Fatalf("expected pet appear, got %+v", outbound[3])
	}

	pets := deltaSelfPets(t, outbound[1])
	if len(pets) != 1 {
		t.Fatalf("expected one owned pet in self delta, got %+v", pets)
	}
	if !pets[0].Summoned || pets[0].Mounted {
		t.Fatalf("expected new pet to be summoned and not mounted, got %+v", pets[0])
	}
	if pets[0].PetTemplateID != "mireling_strider" || pets[0].VisualTemplateID != "mireling" {
		t.Fatalf("unexpected pet snapshot %+v", pets[0])
	}
	if _, exists := runtime.knownEntities["mob_1"]; exists {
		t.Fatalf("expected tamed mob to leave known entities")
	}
	if len(runtime.pets) != 1 {
		t.Fatalf("expected runtime pet roster to contain one pet, got %+v", runtime.pets)
	}
}

func TestAttachedRuntimeRejectsTameMobWhenOutOfRange(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := createDedupTestCharacter(t, store, "char_pet_tame_reject")
	runtime := newAttachedRuntime("sess_pet_tame_reject", character)

	outbound := runtime.processPetCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_pet_tame_reject",
		CommandSeq:      1,
		Type:            "tame_mob",
		Payload:         []byte(`{"target_id":"mob_1"}`),
	})

	if len(outbound) != 2 || outbound[0]["kind"] != "ack" || outbound[1]["reason_code"] != "pet.tame_out_of_range" {
		t.Fatalf("expected pet.tame_out_of_range reject, got %+v", outbound)
	}
	if len(runtime.pets) != 0 {
		t.Fatalf("expected no pet ownership to be created, got %+v", runtime.pets)
	}
}

func TestAttachedRuntimeMountAndDismountPetAdjustMoveSpeedAuthoritatively(t *testing.T) {
	store := newLootPickupTestStore(t)
	character := createDedupTestCharacter(t, store, "char_pet_mount")
	items, err := store.Items.ListByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("Items.ListByCharacterID() error = %v", err)
	}
	runtime := newAttachedRuntime("sess_pet_mount", character)
	runtime.recalculateDerivedStatsLocked(items)
	moveRuntimeNearMob(runtime, "mob_1")

	tameOutbound := runtime.processPetCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_pet_mount_tame",
		CommandSeq:      1,
		Type:            "tame_mob",
		Payload:         []byte(`{"target_id":"mob_1"}`),
	})
	if len(tameOutbound) < 2 || tameOutbound[1]["kind"] != "delta" {
		t.Fatalf("expected tame delta before mount, got %+v", tameOutbound)
	}

	baseMoveSpeed := runtime.derivedStats.MoveSpeed
	if baseMoveSpeed >= 4.05 {
		t.Fatalf("expected pre-mount speed below mounted speed, got %v", baseMoveSpeed)
	}

	mountOutbound := runtime.processPetCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_pet_mount",
		CommandSeq:      2,
		Type:            "mount_pet",
		Payload:         []byte(`{}`),
	})
	if len(mountOutbound) != 2 || mountOutbound[0]["kind"] != "ack" || mountOutbound[1]["kind"] != "delta" {
		t.Fatalf("expected mount ack and delta, got %+v", mountOutbound)
	}
	mountedStats := deltaSelfStats(t, mountOutbound[1])
	if mountedStats.MoveSpeed != 4.05 {
		t.Fatalf("expected mounted move speed 4.05, got %+v", mountedStats)
	}
	if runtime.derivedStats.MoveSpeed != 4.05 {
		t.Fatalf("expected runtime derived move speed 4.05 after mount, got %v", runtime.derivedStats.MoveSpeed)
	}

	dismissWhileMounted := runtime.processPetCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_pet_dismiss_while_mounted",
		CommandSeq:      3,
		Type:            "dismiss_pet",
		Payload:         []byte(`{}`),
	})
	if len(dismissWhileMounted) != 2 || dismissWhileMounted[1]["reason_code"] != "mount.dismount_required" {
		t.Fatalf("expected mount.dismount_required reject, got %+v", dismissWhileMounted)
	}

	dismountOutbound := runtime.processPetCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_pet_dismount",
		CommandSeq:      4,
		Type:            "dismount_pet",
		Payload:         []byte(`{}`),
	})
	if len(dismountOutbound) != 2 || dismountOutbound[0]["kind"] != "ack" || dismountOutbound[1]["kind"] != "delta" {
		t.Fatalf("expected dismount ack and delta, got %+v", dismountOutbound)
	}
	dismountedStats := deltaSelfStats(t, dismountOutbound[1])
	if dismountedStats.MoveSpeed != baseMoveSpeed {
		t.Fatalf("expected dismounted move speed %v, got %+v", baseMoveSpeed, dismountedStats)
	}

	repeatDismount := runtime.processPetCommand(context.Background(), store, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "cmd_pet_dismount_again",
		CommandSeq:      5,
		Type:            "dismount_pet",
		Payload:         []byte(`{}`),
	})
	if len(repeatDismount) != 2 || repeatDismount[1]["reason_code"] != "mount.not_mounted" {
		t.Fatalf("expected mount.not_mounted reject, got %+v", repeatDismount)
	}
}
