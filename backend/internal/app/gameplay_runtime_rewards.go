package app

import (
	"context"
	"fmt"
	"math"
	"time"
)

func (runtime *attachedRuntime) processLootPickup(ctx context.Context, store *Store, command commandEnvelope) []map[string]any {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.advanceMovementLocked(time.Now())
	parsed, reject := runtime.preValidate(command)
	if reject != nil {
		return []map[string]any{reject}
	}

	runtime.expectedCommandSeq++
	outbound := []map[string]any{ackMessage(command.CommandID, command.CommandSeq)}
	if runtime.isPlayerDead() {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "combat.actor_dead", "Actor is currently dead."))
	}
	if parsed.commandType != "pick_up_loot" {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Unsupported gameplay command."))
	}

	loot, exists := runtime.knownEntities[parsed.lootID]
	if !exists || loot.EntityType != "loot" {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "world.entity_not_known", "Referenced loot is not in the current known-set."))
	}
	if lootPartyID, _ := loot.State["party_id"].(string); lootPartyID != "" {
		partyID := ""
		if runtime.party != nil {
			partyID = runtime.party.PartyID
		}
		eligibleCharacterIDs := runtimeLootEligibleCharacterIDs(loot)
		eligible := false
		for _, characterID := range eligibleCharacterIDs {
			if characterID == runtime.characterID {
				eligible = true
				break
			}
		}
		if !eligible || partyID != lootPartyID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "loot.party_ineligible", "Referenced loot is reserved for a different party reward scope."))
		}
	}
	if distance(runtime.position, loot.Position) > lootPickupRange {
		return append(outbound, runtime.queueLootPickupApproachLocked(command, loot, time.Now())...)
	}

	return append(outbound, runtime.collectLootPickupLocked(ctx, store, command.CommandID, command.CommandSeq, parsed.lootID, loot, time.Now())...)
}

func (runtime *attachedRuntime) collectLootPickupLocked(ctx context.Context, store *Store, commandID string, commandSeq int, lootID string, loot runtimeEntity, now time.Time) []map[string]any {
	quantity := runtimeLootQuantity(loot)
	if quantity <= 0 {
		return []map[string]any{rejectMessage(commandID, commandSeq, "world.entity_not_known", "Referenced loot is not available.")}
	}

	items, err := store.Items.PickUpLoot(ctx, runtime.characterID, lootID, loot.TemplateID, quantity)
	if err != nil {
		if err == errLootAlreadyCollected {
			return []map[string]any{rejectMessage(commandID, commandSeq, "loot.already_collected", "Referenced loot was already collected by another actor.")}
		}
		return []map[string]any{rejectMessage(commandID, commandSeq, "system.persistence_failed", "Unable to persist loot pickup.")}
	}

	delete(runtime.knownEntities, lootID)
	if runtime.targetID == lootID {
		runtime.targetID = ""
	}
	runtime.queuedLootPickup = nil
	runtime.clearActiveMovementLocked()
	runtime.revision++
	runtime.regionRevision++
	itemSnapshot := snapshotCharacterItems(items)
	return []map[string]any{
		deltaMessage(
			runtime.revision,
			commandID,
			commandSeq,
			runtime.movementSelfDeltaLocked(now, ""),
			nil,
			&itemSnapshot,
		),
		entityDisappearMessage(runtime.regionRevision, lootID, entityDisappearLoot),
	}
}

func (runtime *attachedRuntime) queueLootPickupApproachLocked(command commandEnvelope, loot runtimeEntity, now time.Time) []map[string]any {
	if runtime.movementPlanner == nil {
		return []map[string]any{
			rejectMessage(command.CommandID, command.CommandSeq, "movement.geodata_unavailable", "Movement geodata is unavailable for this region."),
			positionCorrectionMessage(command.CommandSeq, runtime.position, runtime.facing, "geodata_mismatch"),
		}
	}

	resolution := resolveLootApproachMovement(runtime.movementPlanner, runtime.regionID, runtime.position, loot.Position)
	if resolution.Status == movementPlanStatusRejected {
		return []map[string]any{
			rejectMessage(command.CommandID, command.CommandSeq, resolution.ReasonCode, movementRejectMessage(resolution.ReasonCode)),
			positionCorrectionMessage(command.CommandSeq, runtime.position, runtime.facing, resolution.CorrectionReason),
		}
	}

	runtime.queuedLootPickup = &queuedRuntimeLootPickup{
		CommandID:  command.CommandID,
		CommandSeq: command.CommandSeq,
		LootID:     loot.EntityID,
	}
	runtime.queuedSkill = nil
	runtime.queuedBasicAttack = nil
	runtime.targetID = ""
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
			runtime.movementSelfDeltaLocked(now, "loot_approach"),
			nil,
			nil,
		),
	}
}

func resolveLootApproachMovement(planner movementPlanner, regionID string, start runtimePoint, lootPosition runtimePoint) movementResolution {
	profile := movementProfile{ActorRadius: defaultMovementActorRadius}
	directResolution := planner.Resolve(context.Background(), regionID, start, lootPosition, profile)
	if lootApproachPlanEndsInRange(directResolution, lootPosition) || directResolution.Status == movementPlanStatusCanceled {
		return directResolution
	}

	for _, candidate := range lootApproachCandidates(start, lootPosition) {
		resolution := planner.Resolve(context.Background(), regionID, start, candidate, profile)
		if resolution.Status == movementPlanStatusCanceled {
			return resolution
		}
		if lootApproachPlanEndsInRange(resolution, lootPosition) {
			return resolution
		}
	}

	if directResolution.Status == movementPlanStatusRejected {
		return directResolution
	}
	return movementResolution{
		Status:           movementPlanStatusRejected,
		ReasonCode:       "movement.path_unreachable",
		CorrectionReason: "path_unreachable",
	}
}

func lootApproachPlanEndsInRange(resolution movementResolution, lootPosition runtimePoint) bool {
	if resolution.Status != movementPlanStatusAccepted {
		return false
	}
	return distance(resolution.Plan.AcceptedDestination, lootPosition) <= lootPickupRange+lootPickupRangeEpsilon
}

func lootApproachCandidates(start runtimePoint, lootPosition runtimePoint) []runtimePoint {
	baseAngle := math.Atan2(start.Z-lootPosition.Z, start.X-lootPosition.X)
	if distance(start, lootPosition) <= 0.001 {
		baseAngle = 0
	}
	radii := []float64{
		lootPickupRange - defaultMovementActorRadius - 0.25,
		lootPickupRange * 0.5,
		lootPickupRange * 0.25,
	}
	angleOffsets := []float64{
		0,
		math.Pi / 4,
		-math.Pi / 4,
		math.Pi / 2,
		-math.Pi / 2,
		3 * math.Pi / 4,
		-3 * math.Pi / 4,
		math.Pi,
	}
	candidates := make([]runtimePoint, 0, len(radii)*len(angleOffsets))
	for _, radius := range radii {
		if radius <= 0 {
			continue
		}
		for _, offset := range angleOffsets {
			angle := baseAngle + offset
			candidates = append(candidates, runtimePoint{
				X: lootPosition.X + math.Cos(angle)*radius,
				Z: lootPosition.Z + math.Sin(angle)*radius,
			})
		}
	}
	return candidates
}

func runtimeLootQuantity(entity runtimeEntity) int {
	quantity, ok := entity.State["quantity"].(int)
	if ok {
		return quantity
	}
	if quantityFloat, ok := entity.State["quantity"].(float64); ok {
		return int(quantityFloat)
	}
	return 0
}

func runtimeLootEligibleCharacterIDs(entity runtimeEntity) []string {
	if entity.State == nil {
		return nil
	}
	switch value := entity.State["eligible_character_ids"].(type) {
	case []string:
		result := make([]string, 0, len(value))
		for _, characterID := range value {
			if characterID == "" {
				continue
			}
			result = append(result, characterID)
		}
		return result
	case []any:
		result := make([]string, 0, len(value))
		for _, entry := range value {
			characterID, _ := entry.(string)
			if characterID == "" {
				continue
			}
			result = append(result, characterID)
		}
		return result
	default:
		return nil
	}
}

func (runtime *attachedRuntime) consumePendingPartyRewardEvents() []pendingPartyRewardEvent {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if len(runtime.pendingPartyRewards) == 0 {
		return nil
	}
	result := make([]pendingPartyRewardEvent, len(runtime.pendingPartyRewards))
	copy(result, runtime.pendingPartyRewards)
	runtime.pendingPartyRewards = runtime.pendingPartyRewards[:0]
	return result
}

func (runtime *attachedRuntime) applyLootPartyEligibility(lootID string, partyID string, eligibleCharacterIDs []string) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	entity, exists := runtime.knownEntities[lootID]
	if !exists || entity.EntityType != "loot" {
		return
	}
	if entity.State == nil {
		entity.State = map[string]any{}
	}
	if partyID == "" || len(eligibleCharacterIDs) == 0 {
		delete(entity.State, "party_id")
		delete(entity.State, "eligible_character_ids")
		runtime.knownEntities[lootID] = entity
		return
	}
	eligibleCopy := make([]string, 0, len(eligibleCharacterIDs))
	for _, characterID := range eligibleCharacterIDs {
		if characterID == "" {
			continue
		}
		eligibleCopy = append(eligibleCopy, characterID)
	}
	entity.State["party_id"] = partyID
	entity.State["eligible_character_ids"] = eligibleCopy
	runtime.knownEntities[lootID] = entity
}

func (runtime *attachedRuntime) patchLootAppearState(lootID string) *runtimeEntity {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	entity, exists := runtime.knownEntities[lootID]
	if !exists || entity.EntityType != "loot" {
		return nil
	}
	for index, pending := range runtime.pendingLootAppears {
		if pending.EntityID != lootID {
			continue
		}
		runtime.pendingLootAppears[index] = cloneRuntimeEntity(entity)
		break
	}
	entityCopy := cloneRuntimeEntity(entity)
	return &entityCopy
}

func (runtime *attachedRuntime) applySharedXP(amount int) bool {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if amount <= 0 {
		return false
	}
	previousLevel := runtime.characterLevel
	previousXP := runtime.currentXP
	previousCP := runtime.currentCP
	previousHP := runtime.currentHP
	previousMP := runtime.currentMP
	runtime.awardXP(amount)
	return runtime.characterLevel != previousLevel ||
		runtime.currentXP != previousXP ||
		runtime.currentCP != previousCP ||
		runtime.currentHP != previousHP ||
		runtime.currentMP != previousMP
}

func (runtime *attachedRuntime) applyDamage(entityID string, amount int) map[string]any {
	entity := runtime.knownEntities[entityID]
	wasAlive := isRuntimeEntityAlive(entity)
	currentHP, _ := entity.State["hp"].(int)
	if currentHP == 0 {
		if hpFloat, ok := entity.State["hp"].(float64); ok {
			currentHP = int(hpFloat)
		}
	}
	nextHP := currentHP - amount
	if nextHP < 0 {
		nextHP = 0
	}
	entity.State["hp"] = nextHP
	entity.State["alive"] = nextHP > 0
	if nextHP > 0 {
		entity.State["ai_state"] = string(mobAIStateAggro)
		entity.State["aggro_target_id"] = runtime.characterID
	} else {
		entity.State["ai_state"] = string(mobAIStateDead)
		delete(entity.State, "aggro_target_id")
	}
	runtime.knownEntities[entityID] = entity
	if wasAlive && nextHP == 0 {
		if runtime.queuedBasicAttack != nil && runtime.queuedBasicAttack.TargetID == entityID {
			runtime.queuedBasicAttack = nil
		}
		if runtime.autoBasicAttack != nil && runtime.autoBasicAttack.TargetID == entityID {
			runtime.autoBasicAttack = nil
		}
		xpReward := mobTemplateXPReward(entity.TemplateID)
		if nextQuestState, changed := questProgressedByMobKill(runtime.questState, entity.TemplateID); changed {
			nextQuestState.CharacterID = runtime.characterID
			runtime.questState = nextQuestState
		}
		lootEntityID := runtime.spawnLootForMob(entity)
		if runtime.deferRewardResolution {
			runtime.pendingPartyRewards = append(runtime.pendingPartyRewards, pendingPartyRewardEvent{
				XPAmount:     xpReward,
				LootEntityID: lootEntityID,
			})
		} else {
			runtime.awardXP(xpReward)
		}
		runtime.scheduleMobLifecycle(entityID, time.Now())
	}
	return map[string]any{
		"entity_id":       entityID,
		"hp":              nextHP,
		"alive":           nextHP > 0,
		"ai_state":        entity.State["ai_state"],
		"personality":     entity.State["personality"],
		"aggro_target_id": entity.State["aggro_target_id"],
	}
}

func (runtime *attachedRuntime) applyRetaliation(entityID string) {
	runtime.markMobAggroAndMaybeCounterAttackLocked(entityID, time.Now())
}

func (runtime *attachedRuntime) spawnLootForMob(entity runtimeEntity) string {
	templateID, quantity := runtime.mobLootDrop(entity.TemplateID)
	if templateID == "" || quantity <= 0 {
		return ""
	}
	lootID := fmt.Sprintf("loot_%d", runtime.nextLootSeq)
	runtime.nextLootSeq++
	lootEntity := runtimeEntity{
		EntityID:   lootID,
		EntityType: "loot",
		TemplateID: templateID,
		Position:   entity.Position,
		State: map[string]any{
			"quantity": quantity,
		},
	}
	runtime.knownEntities[lootID] = lootEntity
	runtime.pendingLootAppears = append(runtime.pendingLootAppears, lootEntity)
	return lootID
}

func (runtime *attachedRuntime) consumePendingLootAppears() []runtimeEntity {
	if len(runtime.pendingLootAppears) == 0 {
		return nil
	}
	result := make([]runtimeEntity, len(runtime.pendingLootAppears))
	copy(result, runtime.pendingLootAppears)
	runtime.pendingLootAppears = runtime.pendingLootAppears[:0]
	return result
}

func (runtime *attachedRuntime) mobLootDrop(templateID string) (string, int) {
	switch templateID {
	case "mireling":
		return "duskgold", 4
	case "gloom_wisp":
		return "duskgold", 5
	case "ruin_stalker":
		return "duskgold", 6
	case "stonebound_raider":
		return "duskgold", 8
	case "ashen_howler":
		return "duskgold", 12
	case "gravewarden":
		return "duskgold", 18
	default:
		return "", 0
	}
}

func mobTemplateXPReward(templateID string) int {
	switch templateID {
	case "mireling":
		return 22
	case "gloom_wisp":
		return 28
	case "ruin_stalker":
		return 34
	case "stonebound_raider":
		return 48
	case "ashen_howler":
		return 76
	case "gravewarden":
		return 112
	default:
		return 0
	}
}

func (runtime *attachedRuntime) enterDeadState(now time.Time) {
	runtime.targetID = ""
	runtime.queuedSkill = nil
	runtime.queuedBasicAttack = nil
	runtime.autoBasicAttack = nil
	runtime.cooldownEndsAt = map[string]time.Time{}

	filtered := runtime.scheduledLifecycle[:0]
	for _, event := range runtime.scheduledLifecycle {
		if event.kind != "player_respawn" {
			filtered = append(filtered, event)
		}
	}
	runtime.scheduledLifecycle = append(filtered, scheduledLifecycleEvent{
		dueAt: now.Add(playerRespawnDelay),
		kind:  "player_respawn",
	})
}

func (runtime *attachedRuntime) reconcileResourcePools() {
	if runtime.currentCP > runtime.derivedStats.MaxCP {
		runtime.currentCP = runtime.derivedStats.MaxCP
	}
	if runtime.currentHP > runtime.derivedStats.MaxHP {
		runtime.currentHP = runtime.derivedStats.MaxHP
	}
	if runtime.currentMP > runtime.derivedStats.MaxMP {
		runtime.currentMP = runtime.derivedStats.MaxMP
	}
}

func (runtime *attachedRuntime) awardXP(amount int) {
	if amount <= 0 {
		return
	}

	nextLevel, nextXP, levelsGained := applyCharacterXP(runtime.characterLevel, runtime.currentXP, amount)
	runtime.currentXP = nextXP
	if levelsGained == 0 {
		return
	}

	runtime.characterLevel = nextLevel
	runtime.derivedStats.MaxCP += classTemplateForBaseClass(runtime.characterBaseClass).CPGrowth * levelsGained
	runtime.derivedStats.MaxHP += 18 * levelsGained
	runtime.derivedStats.MaxMP += 7 * levelsGained
	runtime.derivedStats.Attack += 4 * levelsGained
	runtime.derivedStats.Defense += 2 * levelsGained
	runtime.currentCP = runtime.derivedStats.MaxCP
	runtime.currentHP = runtime.derivedStats.MaxHP
	runtime.currentMP = runtime.derivedStats.MaxMP
}

func (runtime *attachedRuntime) scheduleMobLifecycle(entityID string, deathAt time.Time) {
	spawn, exists := runtime.spawnEntities[entityID]
	if !exists {
		return
	}
	filtered := runtime.scheduledLifecycle[:0]
	for _, event := range runtime.scheduledLifecycle {
		if event.entityID != entityID {
			filtered = append(filtered, event)
		}
	}
	runtime.scheduledLifecycle = filtered

	respawnEntity := cloneRuntimeEntity(spawn)
	respawnEntity.State["alive"] = true
	runtime.scheduledLifecycle = append(runtime.scheduledLifecycle,
		scheduledLifecycleEvent{
			dueAt:    deathAt.Add(corpseDespawnDelay),
			kind:     "entity_disappear",
			entityID: entityID,
		},
		scheduledLifecycleEvent{
			dueAt:    deathAt.Add(corpseDespawnDelay + mobRespawnDelay),
			kind:     "entity_appear",
			entityID: entityID,
			entity:   respawnEntity,
		},
	)
}
