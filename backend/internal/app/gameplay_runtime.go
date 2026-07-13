package app

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"sort"
	"sync"
	"time"
)

type runtimePoint struct {
	X float64 `json:"x"`
	Z float64 `json:"z"`
}

type runtimeEntity struct {
	EntityID   string         `json:"entity_id"`
	EntityType string         `json:"entity_type"`
	TemplateID string         `json:"template_id"`
	Position   runtimePoint   `json:"position"`
	State      map[string]any `json:"state"`
}

type commandEnvelope struct {
	ProtocolVersion int             `json:"protocol_version"`
	CommandID       string          `json:"command_id"`
	CommandSeq      int             `json:"command_seq"`
	ClientSentAtMS  int64           `json:"client_sent_at_ms"`
	Type            string          `json:"type"`
	Payload         json.RawMessage `json:"payload"`
}

type parsedCommand struct {
	commandType     string
	movePoint       runtimePoint
	targetID        string
	skillID         string
	lootID          string
	itemID          string
	npcID           string
	npcActionID     string
	mergeItemID     string
	vendorOfferID   string
	exchangeOfferID string
	tradeOfferID    string
	inviteID        string
	chatChannel     string
	chatText        string
	chatTargetName  string
	hotbarState     CharacterHotbarState
	quantity        int
	equipSlot       EquipSlot
}

type preparedMovementIntent struct {
	CommandID    string
	CommandSeq   int
	RequestToken uint64
	Planner      movementPlanner
	RegionID     string
	Start        runtimePoint
	Destination  runtimePoint
	Profile      movementProfile
	StartedAt    time.Time
}

type queuedRuntimeSkill struct {
	CommandID  string
	CommandSeq int
	SkillID    string
	TargetID   string
}

type queuedRuntimeBasicAttack struct {
	CommandID  string
	CommandSeq int
	TargetID   string
}

type queuedRuntimeLootPickup struct {
	CommandID  string
	CommandSeq int
	LootID     string
}

type pendingPartyRewardEvent struct {
	XPAmount     int
	LootEntityID string
}

type skillDefinition struct {
	ID             string
	Category       SkillCategory
	BaseClass      string
	UnlockLevel    int
	TargetRequired bool
	TargetType     string
	Range          float64
	CooldownMS     int
	MPCost         int
	Power          int
	Radius         float64
	MaxTargets     int
}

type attachedRuntime struct {
	mu                      sync.Mutex
	sessionID               string
	characterID             string
	characterName           string
	characterRace           string
	characterBaseClass      string
	characterSex            string
	characterHairStyle      int
	characterHairColor      int
	characterFace           int
	characterLevel          int
	currentXP               int
	currentCP               int
	currentHP               int
	currentMP               int
	expectedCommandSeq      int
	revision                int
	regionRevision          int
	regionID                string
	position                runtimePoint
	facing                  float64
	respawnPosition         runtimePoint
	targetID                string
	knownEntities           map[string]runtimeEntity
	spawnEntities           map[string]runtimeEntity
	cooldownEndsAt          map[string]time.Time
	scheduledLifecycle      []scheduledLifecycleEvent
	nextLootSeq             int
	pendingLootAppears      []runtimeEntity
	derivedStats            CharacterDerivedStats
	hotbarState             CharacterHotbarState
	questState              CharacterQuestState
	party                   *CharacterPartySnapshot
	partyInvites            []CharacterPartyInviteSnapshot
	pets                    []CharacterPet
	characterItems          []CharacterItem
	movementPlanner         movementPlanner
	activeMovement          *runtimeMovementState
	pendingMoveToken        uint64
	stationarySince         time.Time
	lastIdleRegenAt         time.Time
	queuedSkill             *queuedRuntimeSkill
	queuedBasicAttack       *queuedRuntimeBasicAttack
	queuedLootPickup        *queuedRuntimeLootPickup
	pendingPartyRewards     []pendingPartyRewardEvent
	chatRateWindowStartedAt time.Time
	chatRateWindowCount     int
	deferRewardResolution   bool
}

type scheduledLifecycleEvent struct {
	dueAt    time.Time
	kind     string
	entityID string
	entity   runtimeEntity
}

const (
	corpseDespawnDelay     = 1500 * time.Millisecond
	mobRespawnDelay        = 4000 * time.Millisecond
	playerRespawnDelay     = 2500 * time.Millisecond
	entityDisappearDeath   = "defeated_despawn"
	entityDisappearLoot    = "picked_up"
	entityDisappearPlayer  = "removed"
	entityDisappearTamed   = "tamed"
	entityDisappearDismiss = "dismissed"
	lootPickupRange        = 4.5
	lootPickupRangeEpsilon = 0.001
	basicAttackRange       = 2.2
	basicAttackCooldown    = 750 * time.Millisecond
	playerTemplateID       = "player_character"
	idleRegenDelay         = 5 * time.Second
	idleRegenTick          = time.Second
	idleRegenPercent       = 0.03
)

func isRuntimeEntityAlive(entity runtimeEntity) bool {
	alive, ok := entity.State["alive"].(bool)
	if ok {
		return alive
	}
	currentHP, ok := entity.State["hp"].(int)
	if ok {
		return currentHP > 0
	}
	if hpFloat, ok := entity.State["hp"].(float64); ok {
		return hpFloat > 0
	}
	return true
}

func cloneRuntimeEntity(entity runtimeEntity) runtimeEntity {
	clonedState := make(map[string]any, len(entity.State))
	for key, value := range entity.State {
		clonedState[key] = value
	}
	return runtimeEntity{
		EntityID:   entity.EntityID,
		EntityType: entity.EntityType,
		TemplateID: entity.TemplateID,
		Position:   entity.Position,
		State:      clonedState,
	}
}

var supportedSkills = map[string]skillDefinition{
	"crescent_strike": {
		ID:             "crescent_strike",
		Category:       skillCategoryActive,
		BaseClass:      "Fighter",
		UnlockLevel:    1,
		TargetRequired: true,
		TargetType:     "single_target_enemy",
		Range:          8,
		CooldownMS:     900,
		MPCost:         6,
		Power:          18,
	},
	"grave_bloom": {
		ID:             "grave_bloom",
		Category:       skillCategoryActive,
		BaseClass:      "Fighter",
		UnlockLevel:    2,
		TargetRequired: true,
		TargetType:     "target_centered_aoe",
		Range:          9,
		CooldownMS:     4500,
		MPCost:         14,
		Power:          40,
		Radius:         8,
		MaxTargets:     3,
	},
	"iron_will": {
		ID:          "iron_will",
		Category:    skillCategoryPassive,
		BaseClass:   "Fighter",
		UnlockLevel: 1,
	},
	"ember_shot": {
		ID:             "ember_shot",
		Category:       skillCategoryActive,
		BaseClass:      "Mage",
		UnlockLevel:    1,
		TargetRequired: true,
		TargetType:     "single_target_enemy",
		Range:          10,
		CooldownMS:     800,
		MPCost:         7,
		Power:          20,
	},
	"astral_burst": {
		ID:             "astral_burst",
		Category:       skillCategoryActive,
		BaseClass:      "Mage",
		UnlockLevel:    2,
		TargetRequired: true,
		TargetType:     "target_centered_aoe",
		Range:          10,
		CooldownMS:     4200,
		MPCost:         15,
		Power:          38,
		Radius:         7,
		MaxTargets:     3,
	},
	"arcane_focus": {
		ID:          "arcane_focus",
		Category:    skillCategoryPassive,
		BaseClass:   "Mage",
		UnlockLevel: 1,
	},
}

func newAttachedRuntime(sessionID string, character *Character) *attachedRuntime {
	state := persistedCharacterState(character)
	now := time.Now()
	knownEntities := map[string]runtimeEntity{
		"npc_wardkeeper": {
			EntityID:   "npc_wardkeeper",
			EntityType: "npc",
			TemplateID: "wardkeeper",
			Position:   runtimePoint{X: -2, Z: 10},
			State:      map[string]any{},
		},
		"npc_merchant": {
			EntityID:   "npc_merchant",
			EntityType: "npc",
			TemplateID: "merchant",
			Position:   runtimePoint{X: -10, Z: 8},
			State:      map[string]any{},
		},
		warehouseNPCEntityID: {
			EntityID:   warehouseNPCEntityID,
			EntityType: "npc",
			TemplateID: "warehouse_keeper",
			Position:   runtimePoint{X: -13, Z: 4},
			State:      map[string]any{},
		},
		"mob_1": {
			EntityID:   "mob_1",
			EntityType: "mob",
			TemplateID: "mireling",
			Position:   runtimePoint{X: 34, Z: 10},
			State: map[string]any{
				"hp":    54,
				"alive": true,
			},
		},
		"mob_2": {
			EntityID:   "mob_2",
			EntityType: "mob",
			TemplateID: "mireling",
			Position:   runtimePoint{X: 39, Z: 12},
			State: map[string]any{
				"hp":    54,
				"alive": true,
			},
		},
		"mob_3": {
			EntityID:   "mob_3",
			EntityType: "mob",
			TemplateID: "mireling",
			Position:   runtimePoint{X: 44, Z: 8},
			State: map[string]any{
				"hp":    54,
				"alive": true,
			},
		},
	}

	return &attachedRuntime{
		sessionID:          sessionID,
		characterID:        state.ID,
		characterName:      state.Name,
		characterRace:      state.Race,
		characterBaseClass: state.BaseClass,
		characterSex:       state.Sex,
		characterHairStyle: state.HairStyle,
		characterHairColor: state.HairColor,
		characterFace:      state.Face,
		characterLevel:     state.Level,
		currentXP:          state.XP,
		currentCP:          state.CurrentCP,
		currentHP:          state.CurrentHP,
		currentMP:          state.CurrentMP,
		expectedCommandSeq: 1,
		revision:           0,
		regionRevision:     1,
		regionID:           state.LastRegionID,
		position:           runtimePoint{X: state.PositionX, Z: state.PositionZ},
		facing:             0,
		respawnPosition:    runtimePoint{X: state.PositionX, Z: state.PositionZ},
		knownEntities:      knownEntities,
		spawnEntities: map[string]runtimeEntity{
			"mob_1": cloneRuntimeEntity(knownEntities["mob_1"]),
			"mob_2": cloneRuntimeEntity(knownEntities["mob_2"]),
			"mob_3": cloneRuntimeEntity(knownEntities["mob_3"]),
		},
		cooldownEndsAt:  map[string]time.Time{},
		nextLootSeq:     1,
		derivedStats:    baseCharacterDerivedStats(&state),
		hotbarState:     defaultCharacterHotbarState(&state),
		questState:      defaultCharacterQuestState(),
		movementPlanner: defaultMovementPlanner,
		stationarySince: now,
		lastIdleRegenAt: now,
	}
}

func (runtime *attachedRuntime) regionContextMessage() map[string]any {
	entities := make([]runtimeEntity, 0, len(runtime.knownEntities))
	keys := make([]string, 0, len(runtime.knownEntities))
	for entityID := range runtime.knownEntities {
		keys = append(keys, entityID)
	}
	sort.Strings(keys)
	for _, entityID := range keys {
		entity := runtime.knownEntities[entityID]
		entities = append(entities, cloneRuntimeEntity(entity))
	}

	return map[string]any{
		"kind":            "region_context",
		"emitted_at_ms":   time.Now().UnixMilli(),
		"region_revision": runtime.regionRevision,
		"region_id":       runtime.regionID,
		"geodata_version": runtime.currentGeodataVersionLocked(),
		"self_position":   runtime.position,
		"known_entities":  entities,
	}
}

func (runtime *attachedRuntime) processCommand(command commandEnvelope) []map[string]any {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.advanceMovementLocked(time.Now())
	parsed, reject := runtime.preValidate(command)
	if reject != nil {
		return []map[string]any{reject}
	}

	runtime.expectedCommandSeq++
	outbound := []map[string]any{ackMessage(command.CommandID, command.CommandSeq)}
	return append(outbound, runtime.domainValidateAndApply(command, parsed)...)
}

func (runtime *attachedRuntime) prepareAsyncMoveIntent(command commandEnvelope) (*preparedMovementIntent, []map[string]any) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.advanceMovementLocked(time.Now())
	parsed, reject := runtime.preValidate(command)
	if reject != nil {
		return nil, []map[string]any{reject}
	}

	runtime.expectedCommandSeq++
	outbound := []map[string]any{ackMessage(command.CommandID, command.CommandSeq)}
	if runtime.isPlayerDead() {
		return nil, append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "combat.actor_dead", "Actor is currently dead."))
	}
	if parsed.commandType != "move_intent" {
		return nil, append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Unsupported gameplay command."))
	}
	if runtime.movementPlanner == nil {
		return nil, append(outbound,
			rejectMessage(command.CommandID, command.CommandSeq, "movement.geodata_unavailable", "Movement geodata is unavailable for this region."),
			positionCorrectionMessage(command.CommandSeq, runtime.position, runtime.facing, "geodata_mismatch"),
		)
	}

	runtime.pendingMoveToken++
	return &preparedMovementIntent{
		CommandID:    command.CommandID,
		CommandSeq:   command.CommandSeq,
		RequestToken: runtime.pendingMoveToken,
		Planner:      runtime.movementPlanner,
		RegionID:     runtime.regionID,
		Start:        runtime.position,
		Destination:  parsed.movePoint,
		Profile: movementProfile{
			ActorRadius: defaultMovementActorRadius,
		},
		StartedAt: time.Now(),
	}, outbound
}

func (runtime *attachedRuntime) completeAsyncMoveIntent(request *preparedMovementIntent, resolution movementResolution) []map[string]any {
	if request == nil {
		return nil
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if request.RequestToken != runtime.pendingMoveToken {
		return nil
	}
	runtime.pendingMoveToken = 0
	if resolution.Status == movementPlanStatusCanceled {
		return nil
	}
	if resolution.Status == movementPlanStatusRejected {
		return []map[string]any{
			rejectMessage(request.CommandID, request.CommandSeq, resolution.ReasonCode, movementRejectMessage(resolution.ReasonCode)),
			positionCorrectionMessage(request.CommandSeq, runtime.position, runtime.facing, resolution.CorrectionReason),
		}
	}

	now := time.Now()
	runtime.queuedSkill = nil
	runtime.queuedBasicAttack = nil
	runtime.queuedLootPickup = nil
	runtime.setActiveMovementLocked(resolution.Plan, now)
	if len(resolution.Plan.Waypoints) > 0 && distance(runtime.position, resolution.Plan.Waypoints[0]) > 0.001 {
		runtime.facing = math.Atan2(
			resolution.Plan.Waypoints[0].Z-runtime.position.Z,
			resolution.Plan.Waypoints[0].X-runtime.position.X,
		)
	}
	runtime.revision++
	return []map[string]any{
		deltaMessage(
			runtime.revision,
			request.CommandID,
			request.CommandSeq,
			runtime.movementSelfDeltaLocked(now, "path_resolved"),
			nil,
			nil,
		),
	}
}

func (runtime *attachedRuntime) collectLifecycleMessages(now time.Time) []map[string]any {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return runtime.collectLifecycleMessagesLocked(now)
}

func (runtime *attachedRuntime) collectLifecycleMessagesLocked(now time.Time) []map[string]any {
	if runtime.activeMovement != nil && runtime.isPlayerDead() {
		runtime.clearActiveMovementLocked()
	}

	if len(runtime.scheduledLifecycle) == 0 {
		return nil
	}

	pending := runtime.scheduledLifecycle[:0]
	outbound := make([]map[string]any, 0, len(runtime.scheduledLifecycle))
	for _, event := range runtime.scheduledLifecycle {
		if now.Before(event.dueAt) {
			pending = append(pending, event)
			continue
		}

		switch event.kind {
		case "entity_disappear":
			entity, exists := runtime.knownEntities[event.entityID]
			if !exists || isRuntimeEntityAlive(entity) {
				continue
			}
			delete(runtime.knownEntities, event.entityID)
			if runtime.targetID == event.entityID {
				runtime.targetID = ""
			}
			runtime.regionRevision++
			outbound = append(outbound, entityDisappearMessage(runtime.regionRevision, event.entityID, entityDisappearDeath))
		case "entity_appear":
			if _, exists := runtime.knownEntities[event.entityID]; exists {
				continue
			}
			runtime.knownEntities[event.entityID] = cloneRuntimeEntity(event.entity)
			runtime.regionRevision++
			outbound = append(outbound, entityAppearMessage(runtime.regionRevision, cloneRuntimeEntity(event.entity)))
		case "player_respawn":
			if !runtime.isPlayerDead() {
				continue
			}
			runtime.currentHP = runtime.derivedStats.MaxHP
			runtime.currentCP = runtime.derivedStats.MaxCP
			runtime.currentMP = runtime.derivedStats.MaxMP
			runtime.position = runtime.respawnPosition
			runtime.facing = 0
			runtime.targetID = ""
			runtime.clearActiveMovementLocked()
			runtime.resetIdleRegenClockLocked(now)
			runtime.revision++
			outbound = append(outbound, deltaMessage(
				runtime.revision,
				"",
				0,
				runtime.movementSelfDeltaLocked(now, ""),
				nil,
				nil,
			))
		}
	}
	runtime.scheduledLifecycle = pending
	return outbound
}

func (runtime *attachedRuntime) domainValidateAndApply(command commandEnvelope, parsed *parsedCommand) []map[string]any {
	if runtime.isPlayerDead() {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "combat.actor_dead", "Actor is currently dead.")}
	}

	switch parsed.commandType {
	case "move_intent":
		if runtime.movementPlanner == nil {
			return []map[string]any{
				rejectMessage(command.CommandID, command.CommandSeq, "movement.geodata_unavailable", "Movement geodata is unavailable for this region."),
				positionCorrectionMessage(command.CommandSeq, runtime.position, runtime.facing, "geodata_mismatch"),
			}
		}
		resolution := runtime.movementPlanner.Resolve(context.Background(), runtime.regionID, runtime.position, parsed.movePoint, movementProfile{
			ActorRadius: defaultMovementActorRadius,
		})
		if resolution.Status == movementPlanStatusRejected {
			return []map[string]any{
				rejectMessage(command.CommandID, command.CommandSeq, resolution.ReasonCode, movementRejectMessage(resolution.ReasonCode)),
				positionCorrectionMessage(command.CommandSeq, runtime.position, runtime.facing, resolution.CorrectionReason),
			}
		}

		now := time.Now()
		runtime.queuedSkill = nil
		runtime.queuedBasicAttack = nil
		runtime.queuedLootPickup = nil
		runtime.setActiveMovementLocked(resolution.Plan, now)
		if len(resolution.Plan.Waypoints) > 0 && distance(runtime.position, resolution.Plan.Waypoints[0]) > 0.001 {
			runtime.facing = math.Atan2(
				resolution.Plan.Waypoints[0].Z-runtime.position.Z,
				resolution.Plan.Waypoints[0].X-runtime.position.X,
			)
		}
		runtime.revision++
		return []map[string]any{
			deltaMessage(
				runtime.revision,
				command.CommandID,
				command.CommandSeq,
				runtime.movementSelfDeltaLocked(now, "path_resolved"),
				nil,
				nil,
			),
		}
	case "select_target":
		entity, exists := runtime.knownEntities[parsed.targetID]
		if !exists {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "world.entity_not_known", "Referenced entity is not in the current known-set.")}
		}
		if entity.EntityType != "mob" {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "world.entity_not_interactable", "Referenced entity is not targetable.")}
		}
		if !isRuntimeEntityAlive(entity) {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "combat.target_dead", "Referenced target is already dead.")}
		}
		if runtime.queuedSkill != nil && runtime.queuedSkill.TargetID != parsed.targetID {
			runtime.queuedSkill = nil
		}
		if runtime.queuedBasicAttack != nil && runtime.queuedBasicAttack.TargetID != parsed.targetID {
			runtime.queuedBasicAttack = nil
		}
		runtime.queuedLootPickup = nil
		runtime.targetID = parsed.targetID
		runtime.revision++
		return []map[string]any{
			deltaMessage(runtime.revision, command.CommandID, command.CommandSeq, runtime.selfDelta(time.Now(), nil), nil, nil),
		}
	case "use_skill":
		skill, exists := supportedSkills[parsed.skillID]
		if !exists {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "combat.skill_unknown", "Skill is not supported.")}
		}
		knownCategory, known := knownSkillCategory(runtime.characterBaseClass, runtime.characterLevel, parsed.skillID)
		if !known {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "combat.skill_not_learned", "Skill is not learned by this character.")}
		}
		if knownCategory != skillCategoryActive || skill.Category != skillCategoryActive {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "combat.skill_not_active", "Passive skills cannot be activated directly.")}
		}
		if skill.TargetRequired && parsed.targetID == "" {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "combat.target_required", "A valid target is required for this skill.")}
		}
		target, exists := runtime.knownEntities[parsed.targetID]
		if skill.TargetRequired && !exists {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "world.entity_not_known", "Referenced entity is not in the current known-set.")}
		}
		if skill.TargetRequired && target.EntityType != "mob" {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "world.entity_not_interactable", "Referenced entity is not targetable.")}
		}
		if skill.TargetRequired && !isRuntimeEntityAlive(target) {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "combat.target_dead", "Referenced target is already dead.")}
		}
		if runtime.currentMP < skill.MPCost {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "combat.insufficient_mp", "Actor lacks MP for this skill.")}
		}
		now := time.Now()
		if endsAt, cooling := runtime.cooldownEndsAt[skill.ID]; cooling && now.Before(endsAt) {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "combat.cooldown_active", "Skill is still on cooldown.")}
		}
		if skill.TargetRequired && distance(runtime.position, target.Position) > skill.Range {
			return runtime.queueSkillApproachLocked(command, skill, target, now)
		}

		return runtime.activateSkillLocked(command.CommandID, command.CommandSeq, skill, target, now)
	case "basic_attack":
		target, exists := runtime.knownEntities[parsed.targetID]
		if !exists {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "world.entity_not_known", "Referenced entity is not in the current known-set.")}
		}
		if target.EntityType != "mob" {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "world.entity_not_interactable", "Referenced entity is not targetable.")}
		}
		if !isRuntimeEntityAlive(target) {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "combat.target_dead", "Referenced target is already dead.")}
		}
		now := time.Now()
		if endsAt, cooling := runtime.cooldownEndsAt["basic_attack"]; cooling && now.Before(endsAt) {
			return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "combat.cooldown_active", "Basic attack is still on cooldown.")}
		}
		if distance(runtime.position, target.Position) > basicAttackRange {
			return runtime.queueBasicAttackApproachLocked(command, target, now)
		}

		return runtime.activateBasicAttackLocked(command.CommandID, command.CommandSeq, target, now)
	default:
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Unsupported gameplay command.")}
	}
}

func (runtime *attachedRuntime) activateSkillLocked(commandID string, commandSeq int, skill skillDefinition, target runtimeEntity, now time.Time) []map[string]any {
	runtime.cooldownEndsAt[skill.ID] = now.Add(time.Duration(skill.CooldownMS) * time.Millisecond)
	runtime.currentMP -= skill.MPCost
	runtime.targetID = target.EntityID
	runtime.queuedSkill = nil
	runtime.queuedBasicAttack = nil
	runtime.queuedLootPickup = nil
	runtime.clearActiveMovementLocked()
	entityPatches := runtime.applySkill(skill, target)
	runtime.applyRetaliation(target.EntityID)
	lootAppears := runtime.consumePendingLootAppears()
	if runtime.targetID != "" {
		targetEntity, exists := runtime.knownEntities[runtime.targetID]
		if !exists || !isRuntimeEntityAlive(targetEntity) {
			runtime.targetID = ""
		}
	}
	runtime.revision++
	outbound := []map[string]any{
		deltaMessage(
			runtime.revision,
			commandID,
			commandSeq,
			runtime.movementSelfDeltaLocked(now, ""),
			entityPatches,
			nil,
		),
	}
	for _, lootEntity := range lootAppears {
		runtime.regionRevision++
		outbound = append(outbound, entityAppearMessage(runtime.regionRevision, cloneRuntimeEntity(lootEntity)))
	}
	return outbound
}

func (runtime *attachedRuntime) queueSkillApproachLocked(command commandEnvelope, skill skillDefinition, target runtimeEntity, now time.Time) []map[string]any {
	if runtime.movementPlanner == nil {
		return []map[string]any{
			rejectMessage(command.CommandID, command.CommandSeq, "movement.geodata_unavailable", "Movement geodata is unavailable for this region."),
			positionCorrectionMessage(command.CommandSeq, runtime.position, runtime.facing, "geodata_mismatch"),
		}
	}

	resolution := runtime.movementPlanner.Resolve(context.Background(), runtime.regionID, runtime.position, target.Position, movementProfile{
		ActorRadius: defaultMovementActorRadius,
	})
	if resolution.Status == movementPlanStatusRejected {
		return []map[string]any{
			rejectMessage(command.CommandID, command.CommandSeq, resolution.ReasonCode, movementRejectMessage(resolution.ReasonCode)),
			positionCorrectionMessage(command.CommandSeq, runtime.position, runtime.facing, resolution.CorrectionReason),
		}
	}

	runtime.queuedSkill = &queuedRuntimeSkill{
		CommandID:  command.CommandID,
		CommandSeq: command.CommandSeq,
		SkillID:    skill.ID,
		TargetID:   target.EntityID,
	}
	runtime.queuedBasicAttack = nil
	runtime.queuedLootPickup = nil
	runtime.targetID = target.EntityID
	runtime.setActiveMovementLocked(resolution.Plan, now)
	if len(resolution.Plan.Waypoints) > 0 && distance(runtime.position, resolution.Plan.Waypoints[0]) > 0.001 {
		runtime.facing = math.Atan2(
			resolution.Plan.Waypoints[0].Z-runtime.position.Z,
			resolution.Plan.Waypoints[0].X-runtime.position.X,
		)
	}
	runtime.revision++
	return []map[string]any{
		deltaMessage(
			runtime.revision,
			command.CommandID,
			command.CommandSeq,
			runtime.movementSelfDeltaLocked(now, "skill_approach"),
			nil,
			nil,
		),
	}
}

func (runtime *attachedRuntime) activateBasicAttackLocked(commandID string, commandSeq int, target runtimeEntity, now time.Time) []map[string]any {
	runtime.cooldownEndsAt["basic_attack"] = now.Add(basicAttackCooldown)
	runtime.targetID = target.EntityID
	runtime.queuedSkill = nil
	runtime.queuedBasicAttack = nil
	runtime.queuedLootPickup = nil
	runtime.clearActiveMovementLocked()
	entityPatches := []map[string]any{runtime.applyDamage(target.EntityID, maxBasicAttackDamage(runtime.derivedStats.Attack, mobTemplateDefense(target.TemplateID)))}
	runtime.applyRetaliation(target.EntityID)
	lootAppears := runtime.consumePendingLootAppears()
	if runtime.targetID != "" {
		targetEntity, exists := runtime.knownEntities[runtime.targetID]
		if !exists || !isRuntimeEntityAlive(targetEntity) {
			runtime.targetID = ""
		}
	}
	runtime.revision++
	outbound := []map[string]any{
		deltaMessage(
			runtime.revision,
			commandID,
			commandSeq,
			runtime.movementSelfDeltaLocked(now, ""),
			entityPatches,
			nil,
		),
	}
	for _, lootEntity := range lootAppears {
		runtime.regionRevision++
		outbound = append(outbound, entityAppearMessage(runtime.regionRevision, cloneRuntimeEntity(lootEntity)))
	}
	return outbound
}

func (runtime *attachedRuntime) queueBasicAttackApproachLocked(command commandEnvelope, target runtimeEntity, now time.Time) []map[string]any {
	if runtime.movementPlanner == nil {
		return []map[string]any{
			rejectMessage(command.CommandID, command.CommandSeq, "movement.geodata_unavailable", "Movement geodata is unavailable for this region."),
			positionCorrectionMessage(command.CommandSeq, runtime.position, runtime.facing, "geodata_mismatch"),
		}
	}

	resolution := runtime.movementPlanner.Resolve(context.Background(), runtime.regionID, runtime.position, target.Position, movementProfile{
		ActorRadius: defaultMovementActorRadius,
	})
	if resolution.Status == movementPlanStatusRejected {
		return []map[string]any{
			rejectMessage(command.CommandID, command.CommandSeq, resolution.ReasonCode, movementRejectMessage(resolution.ReasonCode)),
			positionCorrectionMessage(command.CommandSeq, runtime.position, runtime.facing, resolution.CorrectionReason),
		}
	}

	runtime.queuedBasicAttack = &queuedRuntimeBasicAttack{
		CommandID:  command.CommandID,
		CommandSeq: command.CommandSeq,
		TargetID:   target.EntityID,
	}
	runtime.queuedSkill = nil
	runtime.queuedLootPickup = nil
	runtime.targetID = target.EntityID
	runtime.setActiveMovementLocked(resolution.Plan, now)
	if len(resolution.Plan.Waypoints) > 0 && distance(runtime.position, resolution.Plan.Waypoints[0]) > 0.001 {
		runtime.facing = math.Atan2(
			resolution.Plan.Waypoints[0].Z-runtime.position.Z,
			resolution.Plan.Waypoints[0].X-runtime.position.X,
		)
	}
	runtime.revision++
	return []map[string]any{
		deltaMessage(
			runtime.revision,
			command.CommandID,
			command.CommandSeq,
			runtime.movementSelfDeltaLocked(now, "basic_attack_approach"),
			nil,
			nil,
		),
	}
}

func clampRuntimePoint(point runtimePoint) runtimePoint {
	return runtimePoint{
		X: math.Max(-18, math.Min(97, point.X)),
		Z: math.Max(-16, math.Min(16, point.Z)),
	}
}

func ackMessage(commandID string, commandSeq int) map[string]any {
	return map[string]any{
		"kind":          "ack",
		"emitted_at_ms": time.Now().UnixMilli(),
		"command_id":    commandID,
		"command_seq":   commandSeq,
		"status":        "received",
	}
}

func rejectMessage(commandID string, commandSeq int, reasonCode, message string) map[string]any {
	payload := map[string]any{
		"kind":          "reject",
		"emitted_at_ms": time.Now().UnixMilli(),
		"reason_code":   reasonCode,
		"message":       message,
	}
	if commandID != "" {
		payload["command_id"] = commandID
		payload["command_seq"] = commandSeq
	}
	return payload
}

func deltaMessage(revision int, commandID string, commandSeq int, self map[string]any, entities []map[string]any, itemSnapshot *CharacterItemSnapshot) map[string]any {
	payload := map[string]any{
		"kind":                   "delta",
		"emitted_at_ms":          time.Now().UnixMilli(),
		"revision":               revision,
		"applies_to_command_id":  commandID,
		"applies_to_command_seq": commandSeq,
		"self":                   self,
		"entities":               entities,
	}
	if itemSnapshot != nil {
		payload["inventory"] = itemSnapshot.Inventory
		payload["equipment"] = itemSnapshot.Equipment
		payload["warehouse"] = itemSnapshot.Warehouse
	}
	return payload
}

func positionCorrectionMessage(commandSeq int, position runtimePoint, facing float64, reason string) map[string]any {
	return map[string]any{
		"kind":                   "position_correction",
		"emitted_at_ms":          time.Now().UnixMilli(),
		"applies_to_command_seq": commandSeq,
		"position":               position,
		"facing":                 facing,
		"reason":                 reason,
	}
}

func movementRejectMessage(reasonCode string) string {
	switch reasonCode {
	case "movement.destination_blocked":
		return "Movement destination is blocked by terrain or an obstacle."
	case "movement.destination_out_of_bounds":
		return "Movement destination is outside the authoritative region bounds."
	case "movement.path_budget_exceeded":
		return "Movement path resolution exceeded the safe pathfinding budget."
	case "movement.path_unreachable":
		return "Movement destination cannot be reached from the current position."
	case "movement.geodata_unavailable":
		return "Movement geodata is unavailable for this region."
	case "movement.geodata_mismatch":
		return "Movement position is incompatible with the current geodata version."
	default:
		return "Movement request was rejected."
	}
}

func entityDisappearMessage(regionRevision int, entityID string, reason string) map[string]any {
	return map[string]any{
		"kind":            "entity_disappear",
		"emitted_at_ms":   time.Now().UnixMilli(),
		"region_revision": regionRevision,
		"entity_id":       entityID,
		"reason":          reason,
	}
}

func entityAppearMessage(regionRevision int, entity runtimeEntity) map[string]any {
	return map[string]any{
		"kind":            "entity_appear",
		"emitted_at_ms":   time.Now().UnixMilli(),
		"region_revision": regionRevision,
		"entity":          entity,
	}
}

func (runtime *attachedRuntime) projectedTargetID() any {
	if runtime.targetID == "" {
		return nil
	}
	return runtime.targetID
}

func (runtime *attachedRuntime) selfDelta(now time.Time, extra map[string]any) map[string]any {
	payload := map[string]any{
		"target_id":     runtime.projectedTargetID(),
		"cooldowns":     runtime.currentCooldownSnapshot(now),
		"dead":          runtime.isPlayerDead(),
		"facing":        runtime.facing,
		"level":         runtime.characterLevel,
		"xp":            runtime.currentXP,
		"cp":            runtime.currentCP,
		"hp":            runtime.currentHP,
		"mp":            runtime.currentMP,
		"stats":         runtime.derivedStats,
		"known_skills":  runtime.knownSkillsSnapshot(),
		"hotbar":        runtime.hotbarSnapshot(),
		"pets":          runtime.petSnapshotsLocked(),
		"quest":         runtime.questSnapshotLocked(),
		"party":         cloneCharacterPartySnapshot(runtime.party),
		"party_invites": cloneCharacterPartyInviteSnapshots(runtime.partyInvites),
	}
	for key, value := range extra {
		payload[key] = value
	}
	return payload
}

func (runtime *attachedRuntime) knownSkillsSnapshot() []CharacterKnownSkill {
	return learnedSkillsForCharacter(runtime.characterBaseClass, runtime.characterLevel)
}

func (runtime *attachedRuntime) hotbarSnapshot() CharacterHotbarState {
	character := &Character{
		BaseClass: runtime.characterBaseClass,
		Level:     runtime.characterLevel,
	}
	return normalizeCharacterHotbarState(runtime.hotbarState, character)
}

func (runtime *attachedRuntime) questSnapshotLocked() CharacterQuestSnapshot {
	state := runtime.questState
	state.CharacterID = runtime.characterID
	return questSnapshot(state)
}

func (runtime *attachedRuntime) loadQuestState(quests []CharacterQuestState) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.questState = primaryQuestState(quests, runtime.characterID)
}

func (runtime *attachedRuntime) loadPartyState(party *CharacterPartySnapshot, invites []CharacterPartyInviteSnapshot) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.party = cloneCharacterPartySnapshot(party)
	runtime.partyInvites = cloneCharacterPartyInviteSnapshots(invites)
}

func (runtime *attachedRuntime) partyInviteExpirationDue(now time.Time) bool {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	for _, invite := range runtime.partyInvites {
		if invite.ExpiresAtMS <= now.UnixMilli() {
			return true
		}
	}
	return false
}

func (runtime *attachedRuntime) partyDeltaMessage(
	party *CharacterPartySnapshot,
	invites []CharacterPartyInviteSnapshot,
) map[string]any {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.party = cloneCharacterPartySnapshot(party)
	runtime.partyInvites = cloneCharacterPartyInviteSnapshots(invites)
	runtime.revision++
	return deltaMessage(runtime.revision, "", 0, runtime.selfDelta(time.Now(), nil), nil, nil)
}

func (runtime *attachedRuntime) partyRosterMemberSnapshot(isLeader bool) CharacterPartyMemberSnapshot {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return CharacterPartyMemberSnapshot{
		CharacterID: runtime.characterID,
		Name:        runtime.characterName,
		Level:       runtime.characterLevel,
		BaseClass:   runtime.characterBaseClass,
		HP:          runtime.currentHP,
		MP:          runtime.currentMP,
		Online:      true,
		IsLeader:    isLeader,
	}
}

func (runtime *attachedRuntime) characterQuestState() CharacterQuestState {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	state := runtime.questState
	state.CharacterID = runtime.characterID
	return normalizeCharacterQuestState(state)
}

func (runtime *attachedRuntime) characterWorldState() (string, runtimePoint) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.advanceMovementLocked(time.Now())
	if runtime.isPlayerDead() {
		return runtime.regionID, runtime.respawnPosition
	}
	return runtime.regionID, runtime.position
}

func (runtime *attachedRuntime) characterProgressionState() (int, int, int, int, int) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.isPlayerDead() {
		return runtime.characterLevel, runtime.currentXP, runtime.derivedStats.MaxCP, runtime.derivedStats.MaxHP, runtime.derivedStats.MaxMP
	}
	return runtime.characterLevel, runtime.currentXP, runtime.currentCP, runtime.currentHP, runtime.currentMP
}

func (runtime *attachedRuntime) characterCooldownState(now time.Time) []CharacterSkillCooldown {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	cooldowns := make([]CharacterSkillCooldown, 0, len(runtime.cooldownEndsAt))
	for skillID, endsAt := range runtime.cooldownEndsAt {
		if !endsAt.After(now) {
			continue
		}
		cooldowns = append(cooldowns, CharacterSkillCooldown{
			CharacterID: runtime.characterID,
			SkillID:     skillID,
			EndsAt:      endsAt,
		})
	}
	sort.Slice(cooldowns, func(i, j int) bool {
		return cooldowns[i].SkillID < cooldowns[j].SkillID
	})
	return cooldowns
}

func (runtime *attachedRuntime) loadCooldownState(cooldowns []CharacterSkillCooldown, now time.Time) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.cooldownEndsAt = cooldownEndsAtFromRecords(cooldowns, now)
}

func (runtime *attachedRuntime) expectedCommandSeqValue() int {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return runtime.expectedCommandSeq
}

func (runtime *attachedRuntime) regionIDValue() string {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return runtime.regionID
}

func (runtime *attachedRuntime) seedKnownEntity(entity runtimeEntity) {
	if entity.EntityID == "" || entity.EntityType == "" || entity.EntityID == runtime.characterID {
		return
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if _, exists := runtime.knownEntities[entity.EntityID]; exists {
		return
	}
	runtime.knownEntities[entity.EntityID] = cloneRuntimeEntity(entity)
}

func (runtime *attachedRuntime) playerPresenceEntity() runtimeEntity {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return runtime.playerPresenceEntityLocked()
}

func (runtime *attachedRuntime) playerPresenceEntityLocked() runtimeEntity {
	return runtimeEntity{
		EntityID:   runtime.characterID,
		EntityType: "player",
		TemplateID: playerTemplateID,
		Position:   runtime.position,
		State:      runtime.playerPresenceStateLocked(),
	}
}

func (runtime *attachedRuntime) playerPresenceStateLocked() map[string]any {
	return map[string]any{
		"name":           runtime.characterName,
		"level":          runtime.characterLevel,
		"race":           runtime.characterRace,
		"base_class":     runtime.characterBaseClass,
		"sex":            runtime.characterSex,
		"hair_style":     runtime.characterHairStyle,
		"hair_color":     runtime.characterHairColor,
		"face":           runtime.characterFace,
		"cp":             runtime.currentCP,
		"hp":             runtime.currentHP,
		"dead":           runtime.isPlayerDead(),
		"facing":         runtime.facing,
		"mounted_pet_id": runtime.projectedMountedPetIDLocked(),
	}
}

func (runtime *attachedRuntime) inventoryDeltaMessage(commandID string, commandSeq int, items []CharacterItem) map[string]any {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.revision++
	itemSnapshot := snapshotCharacterItems(items)
	return deltaMessage(
		runtime.revision,
		commandID,
		commandSeq,
		runtime.selfDelta(time.Now(), nil),
		nil,
		&itemSnapshot,
	)
}

func playerPresencePatchFromEntity(entity runtimeEntity) map[string]any {
	patch := map[string]any{
		"entity_id": entity.EntityID,
		"position":  entity.Position,
	}
	for _, key := range []string{"name", "level", "race", "base_class", "sex", "hair_style", "hair_color", "face", "hp", "dead", "facing", "mounted_pet_id"} {
		if value, exists := entity.State[key]; exists {
			patch[key] = value
		}
	}
	return patch
}

func (runtime *attachedRuntime) applyRemotePlayerAppear(entity runtimeEntity) map[string]any {
	if entity.EntityType != "player" || entity.EntityID == "" || entity.EntityID == runtime.characterID {
		return nil
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if _, exists := runtime.knownEntities[entity.EntityID]; exists {
		return nil
	}
	runtime.knownEntities[entity.EntityID] = cloneRuntimeEntity(entity)
	runtime.regionRevision++
	return entityAppearMessage(runtime.regionRevision, cloneRuntimeEntity(entity))
}

func (runtime *attachedRuntime) applyRemotePlayerDisappear(entityID string, reason string) map[string]any {
	if entityID == "" || entityID == runtime.characterID {
		return nil
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	entity, exists := runtime.knownEntities[entityID]
	if !exists || entity.EntityType != "player" {
		return nil
	}
	delete(runtime.knownEntities, entityID)
	if runtime.targetID == entityID {
		runtime.targetID = ""
	}
	runtime.regionRevision++
	return entityDisappearMessage(runtime.regionRevision, entityID, reason)
}

func (runtime *attachedRuntime) applyRemotePlayerState(entity runtimeEntity) map[string]any {
	if entity.EntityType != "player" || entity.EntityID == "" || entity.EntityID == runtime.characterID {
		return nil
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	existing, exists := runtime.knownEntities[entity.EntityID]
	if !exists || existing.EntityType != "player" {
		return nil
	}
	existingPatch := playerPresencePatchFromEntity(existing)
	nextPatch := playerPresencePatchFromEntity(entity)
	if reflect.DeepEqual(existingPatch, nextPatch) {
		return nil
	}
	runtime.knownEntities[entity.EntityID] = cloneRuntimeEntity(entity)
	runtime.revision++
	return deltaMessage(runtime.revision, "", 0, nil, []map[string]any{nextPatch}, nil)
}

func (runtime *attachedRuntime) applySkill(skill skillDefinition, target runtimeEntity) []map[string]any {
	switch skill.TargetType {
	case "single_target_enemy":
		damage := maxSingleTargetDamage(runtime.derivedStats.Attack, skill.Power, mobTemplateDefense(target.TemplateID))
		return []map[string]any{runtime.applyDamage(target.EntityID, damage)}
	case "target_centered_aoe":
		targets := runtime.collectAoeTargets(target.Position, skill.Radius, skill.MaxTargets)
		patches := make([]map[string]any, 0, len(targets))
		damagePerTarget := maxAoeDamageBudget(runtime.derivedStats.Attack, skill.Power)
		if len(targets) > 1 {
			damagePerTarget = int(math.Max(12, math.Round(float64(damagePerTarget)/float64(len(targets)))))
		}
		for _, entity := range targets {
			damage := maxAoeTargetDamage(damagePerTarget, mobTemplateDefense(entity.TemplateID))
			patches = append(patches, runtime.applyDamage(entity.EntityID, damage))
		}
		return patches
	default:
		return []map[string]any{}
	}
}

func (runtime *attachedRuntime) applyRemoteEntityAppear(entity runtimeEntity) map[string]any {
	if entity.EntityID == "" || entity.EntityID == runtime.characterID || entity.EntityType == "player" {
		return nil
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if _, exists := runtime.knownEntities[entity.EntityID]; exists {
		return nil
	}
	runtime.knownEntities[entity.EntityID] = cloneRuntimeEntity(entity)
	runtime.regionRevision++
	return entityAppearMessage(runtime.regionRevision, cloneRuntimeEntity(entity))
}

func (runtime *attachedRuntime) applyRemoteEntityDisappear(entityID string, reason string) map[string]any {
	if entityID == "" || entityID == runtime.characterID {
		return nil
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	entity, exists := runtime.knownEntities[entityID]
	if !exists || entity.EntityType == "player" {
		return nil
	}
	delete(runtime.knownEntities, entityID)
	if runtime.targetID == entityID {
		runtime.targetID = ""
	}
	runtime.regionRevision++
	return entityDisappearMessage(runtime.regionRevision, entityID, reason)
}

func (runtime *attachedRuntime) applyLootEntityAppear(entity runtimeEntity) map[string]any {
	if entity.EntityType != "loot" {
		return nil
	}
	return runtime.applyRemoteEntityAppear(entity)
}

func (runtime *attachedRuntime) applyLootEntityDisappear(entityID string, reason string) map[string]any {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	entity, exists := runtime.knownEntities[entityID]
	if !exists || entity.EntityType != "loot" {
		return nil
	}
	delete(runtime.knownEntities, entityID)
	if runtime.targetID == entityID {
		runtime.targetID = ""
	}
	runtime.regionRevision++
	return entityDisappearMessage(runtime.regionRevision, entityID, reason)
}

func (runtime *attachedRuntime) partyIDValue() string {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.party == nil {
		return ""
	}
	return runtime.party.PartyID
}

func (runtime *attachedRuntime) isPlayerDeadValue() bool {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return runtime.isPlayerDead()
}

func (runtime *attachedRuntime) partySnapshot() *CharacterPartySnapshot {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return cloneCharacterPartySnapshot(runtime.party)
}

func (runtime *attachedRuntime) selfDeltaSnapshot() map[string]any {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return runtime.selfDelta(time.Now(), nil)
}

func (runtime *attachedRuntime) progressionDeltaMessage(commandID string, commandSeq int) map[string]any {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.revision++
	return deltaMessage(runtime.revision, commandID, commandSeq, runtime.selfDelta(time.Now(), nil), nil, nil)
}

func (runtime *attachedRuntime) collectAoeTargets(center runtimePoint, radius float64, maxTargets int) []runtimeEntity {
	candidates := make([]runtimeEntity, 0, len(runtime.knownEntities))
	for _, entity := range runtime.knownEntities {
		if entity.EntityType != "mob" {
			continue
		}
		alive, _ := entity.State["alive"].(bool)
		if !alive {
			continue
		}
		if distance(center, entity.Position) <= radius {
			candidates = append(candidates, entity)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return distance(center, candidates[i].Position) < distance(center, candidates[j].Position)
	})
	if len(candidates) > maxTargets {
		candidates = candidates[:maxTargets]
	}
	return candidates
}

func distance(left runtimePoint, right runtimePoint) float64 {
	return math.Hypot(left.X-right.X, left.Z-right.Z)
}

func maxSingleTargetDamage(attack int, power int, targetDefense int) int {
	return int(math.Max(8, math.Round(float64(attack+power)-float64(targetDefense)*0.4)))
}

func maxAoeDamageBudget(attack int, power int) int {
	return int(math.Max(18, float64(attack+power)))
}

func maxAoeTargetDamage(baseDamage int, targetDefense int) int {
	return int(math.Max(7, float64(baseDamage)-math.Round(float64(targetDefense)*0.4)))
}

func maxBasicAttackDamage(attack int, targetDefense int) int {
	return int(math.Max(3, math.Round(float64(attack)-float64(targetDefense)*0.35)))
}

func incomingPlayerDamage(enemyAttack int, playerDefense int) int {
	return int(math.Max(1, math.Round(float64(enemyAttack)-float64(playerDefense)*0.4)))
}

func (runtime *attachedRuntime) currentCooldownSnapshot(now time.Time) map[string]int {
	cooldowns := map[string]int{}
	if runtime.isPlayerDead() {
		return cooldowns
	}
	for skillID, endsAt := range runtime.cooldownEndsAt {
		if now.Before(endsAt) {
			cooldowns[skillID] = int(endsAt.Sub(now).Milliseconds())
		}
	}
	return cooldowns
}

func cooldownEndsAtFromRecords(cooldowns []CharacterSkillCooldown, now time.Time) map[string]time.Time {
	result := map[string]time.Time{}
	for _, cooldown := range cooldowns {
		if cooldown.SkillID == "" || !cooldown.EndsAt.After(now) {
			continue
		}
		result[cooldown.SkillID] = cooldown.EndsAt
	}
	return result
}

func cooldownSnapshotFromRecords(cooldowns []CharacterSkillCooldown, now time.Time) map[string]int {
	result := map[string]int{}
	for _, cooldown := range cooldowns {
		if cooldown.SkillID == "" || !cooldown.EndsAt.After(now) {
			continue
		}
		result[cooldown.SkillID] = int(cooldown.EndsAt.Sub(now).Milliseconds())
	}
	return result
}

func (runtime *attachedRuntime) isPlayerDead() bool {
	return runtime.currentHP <= 0
}
