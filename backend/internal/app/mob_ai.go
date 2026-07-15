package app

import (
	"math"
	"sort"
	"time"
)

type mobPersonality string

const (
	mobPersonalityPassive    mobPersonality = "passive"
	mobPersonalityAggressive mobPersonality = "aggressive"
)

type mobAIState string

const (
	mobAIStateIdle  mobAIState = "idle"
	mobAIStateAggro mobAIState = "aggro"
	mobAIStateDead  mobAIState = "dead"
)

const (
	mobAIDefaultDelta         = 250 * time.Millisecond
	mobAggroLeashMultiplier   = 2.6
	mobAggroLeashMinimumRange = 15.0
)

func mobTemplateMoveSpeed(templateID string) float64 {
	switch templateID {
	case "mireling":
		return 4.05
	case "gloom_wisp":
		return 4.35
	case "ruin_stalker":
		return 4.65
	case "stonebound_raider":
		return 4.43
	case "ashen_howler":
		return 4.95
	case "gravewarden":
		return 3.9
	default:
		return 3.6
	}
}

func mobTemplateAggroRadius(templateID string) float64 {
	switch templateID {
	case "mireling":
		return 8.5
	case "gloom_wisp":
		return 9.5
	case "ruin_stalker":
		return 10.5
	case "stonebound_raider":
		return 11
	case "ashen_howler":
		return 12
	case "gravewarden":
		return 13
	default:
		return 8
	}
}

func mobTemplateAttackRange(templateID string) float64 {
	switch templateID {
	case "mireling":
		return 2.1
	case "gloom_wisp":
		return 2.2
	case "ruin_stalker":
		return 2.4
	case "stonebound_raider":
		return 2.4
	case "ashen_howler":
		return 2.5
	case "gravewarden":
		return 2.8
	default:
		return 2.1
	}
}

func mobTemplateAttackInterval(templateID string) time.Duration {
	switch templateID {
	case "mireling":
		return 1400 * time.Millisecond
	case "gloom_wisp":
		return 1320 * time.Millisecond
	case "ruin_stalker":
		return 1250 * time.Millisecond
	case "stonebound_raider":
		return 1200 * time.Millisecond
	case "ashen_howler":
		return 1120 * time.Millisecond
	case "gravewarden":
		return 1050 * time.Millisecond
	default:
		return 1400 * time.Millisecond
	}
}

func mobAggroLeashRadius(templateID string) float64 {
	return math.Max(mobAggroLeashMinimumRange, mobTemplateAggroRadius(templateID)*mobAggroLeashMultiplier)
}

func runtimeMobPersonality(entity runtimeEntity) mobPersonality {
	if personality, ok := entity.State["personality"].(string); ok {
		switch mobPersonality(personality) {
		case mobPersonalityAggressive:
			return mobPersonalityAggressive
		case mobPersonalityPassive:
			return mobPersonalityPassive
		}
	}
	return mobPersonalityPassive
}

func runtimeMobAIState(entity runtimeEntity) mobAIState {
	if !isRuntimeEntityAlive(entity) {
		return mobAIStateDead
	}
	if state, ok := entity.State["ai_state"].(string); ok {
		switch mobAIState(state) {
		case mobAIStateAggro:
			return mobAIStateAggro
		case mobAIStateDead:
			return mobAIStateDead
		case mobAIStateIdle:
			return mobAIStateIdle
		}
	}
	return mobAIStateIdle
}

func runtimeMobLastAIAt(entity runtimeEntity) int64 {
	if value, ok := entity.State["last_ai_at_ms"].(int64); ok {
		return value
	}
	if value, ok := entity.State["last_ai_at_ms"].(int); ok {
		return int64(value)
	}
	if value, ok := entity.State["last_ai_at_ms"].(float64); ok {
		return int64(value)
	}
	return 0
}

func runtimeMobNextAttackAt(entity runtimeEntity) int64 {
	if value, ok := entity.State["next_attack_at_ms"].(int64); ok {
		return value
	}
	if value, ok := entity.State["next_attack_at_ms"].(int); ok {
		return int64(value)
	}
	if value, ok := entity.State["next_attack_at_ms"].(float64); ok {
		return int64(value)
	}
	return 0
}

func runtimeMobSpawnPoint(entity runtimeEntity) runtimePoint {
	spawn := entity.Position
	if value, ok := entity.State["spawn_x"].(float64); ok {
		spawn.X = value
	}
	if value, ok := entity.State["spawn_x"].(int); ok {
		spawn.X = float64(value)
	}
	if value, ok := entity.State["spawn_z"].(float64); ok {
		spawn.Z = value
	}
	if value, ok := entity.State["spawn_z"].(int); ok {
		spawn.Z = float64(value)
	}
	return spawn
}

func (runtime *attachedRuntime) markMobAggroLocked(entityID string, targetID string, now time.Time) map[string]any {
	entity, exists := runtime.knownEntities[entityID]
	if !exists || entity.EntityType != "mob" || !isRuntimeEntityAlive(entity) {
		return nil
	}
	entity.State["ai_state"] = string(mobAIStateAggro)
	entity.State["aggro_target_id"] = targetID
	entity.State["last_ai_at_ms"] = now.UnixMilli()
	runtime.knownEntities[entityID] = entity
	return runtime.mobAIPatch(entity, true)
}

func (runtime *attachedRuntime) resolveMobAILocked(now time.Time) ([]map[string]any, bool) {
	ids := make([]string, 0, len(runtime.knownEntities))
	for entityID, entity := range runtime.knownEntities {
		if entity.EntityType == "mob" {
			ids = append(ids, entityID)
		}
	}
	sort.Strings(ids)

	patches := make([]map[string]any, 0, len(ids))
	selfChanged := false
	for _, entityID := range ids {
		entity := runtime.knownEntities[entityID]
		patch, changed, attacked := runtime.resolveSingleMobAILocked(entity, now)
		if changed && patch != nil {
			patches = append(patches, patch)
		}
		if attacked {
			selfChanged = true
		}
	}
	return patches, len(patches) > 0 || selfChanged
}

func (runtime *attachedRuntime) resolveSingleMobAILocked(entity runtimeEntity, now time.Time) (map[string]any, bool, bool) {
	if entity.EntityType != "mob" {
		return nil, false, false
	}
	if !isRuntimeEntityAlive(entity) {
		if runtimeMobAIState(entity) != mobAIStateDead {
			entity.State["ai_state"] = string(mobAIStateDead)
			runtime.knownEntities[entity.EntityID] = entity
			return runtime.mobAIPatch(entity, false), true, false
		}
		return nil, false, false
	}

	changed := false
	attacked := false
	if runtime.isPlayerDead() {
		if runtimeMobAIState(entity) == mobAIStateAggro {
			entity.State["ai_state"] = string(mobAIStateIdle)
			delete(entity.State, "aggro_target_id")
			changed = true
		}
		return runtime.returnMobToSpawnLocked(entity, now, changed, false)
	}

	personality := runtimeMobPersonality(entity)
	aiState := runtimeMobAIState(entity)
	playerDistance := distance(entity.Position, runtime.position)
	if aiState != mobAIStateAggro && personality == mobPersonalityAggressive && playerDistance <= mobTemplateAggroRadius(entity.TemplateID) {
		aiState = mobAIStateAggro
		entity.State["ai_state"] = string(mobAIStateAggro)
		entity.State["aggro_target_id"] = runtime.characterID
		changed = true
	}

	if aiState != mobAIStateAggro {
		return runtime.returnMobToSpawnLocked(entity, now, changed, false)
	}

	if playerDistance > mobAggroLeashRadius(entity.TemplateID) {
		entity.State["ai_state"] = string(mobAIStateIdle)
		delete(entity.State, "aggro_target_id")
		changed = true
		return runtime.returnMobToSpawnLocked(entity, now, changed, false)
	}

	entity.State["aggro_target_id"] = runtime.characterID
	if playerDistance > mobTemplateAttackRange(entity.TemplateID) {
		next, moved := runtime.nextMobChasePosition(entity, now)
		if moved {
			entity.Position = next
			entity.State["facing"] = angleBetween(entity.Position, runtime.position)
			changed = true
		}
		entity.State["last_ai_at_ms"] = now.UnixMilli()
		runtime.knownEntities[entity.EntityID] = entity
		if changed {
			return runtime.mobAIPatch(entity, true), true, false
		}
		return nil, false, false
	}

	if runtimeMobNextAttackAt(entity) <= now.UnixMilli() {
		damage := incomingPlayerDamage(mobTemplateAttack(entity.TemplateID), runtime.derivedStats.Defense)
		runtime.currentHP -= damage
		if runtime.currentHP < 0 {
			runtime.currentHP = 0
		}
		entity.State["next_attack_at_ms"] = now.Add(mobTemplateAttackInterval(entity.TemplateID)).UnixMilli()
		changed = true
		attacked = true
		if runtime.currentHP == 0 {
			runtime.enterDeadState(now)
		}
	}
	runtime.knownEntities[entity.EntityID] = entity
	if changed {
		return runtime.mobAIPatch(entity, true), true, attacked
	}
	return nil, false, attacked
}

func (runtime *attachedRuntime) returnMobToSpawnLocked(entity runtimeEntity, now time.Time, alreadyChanged bool, alreadyAttacked bool) (map[string]any, bool, bool) {
	spawn := runtimeMobSpawnPoint(entity)
	changed := alreadyChanged
	if distance(entity.Position, spawn) > 0.1 {
		next := movePointAlongPath(entity.Position, []runtimePoint{spawn}, runtime.mobAIMoveDistance(entity, now))
		if distance(next, spawn) < 0.2 {
			next = spawn
		}
		if distance(entity.Position, next) > 0.001 {
			entity.Position = next
			entity.State["facing"] = angleBetween(entity.Position, spawn)
			changed = true
		}
	}
	entity.State["last_ai_at_ms"] = now.UnixMilli()
	runtime.knownEntities[entity.EntityID] = entity
	if changed {
		return runtime.mobAIPatch(entity, true), true, alreadyAttacked
	}
	return nil, false, alreadyAttacked
}

func (runtime *attachedRuntime) nextMobChasePosition(entity runtimeEntity, now time.Time) (runtimePoint, bool) {
	if runtime.movementPlanner == nil {
		return entity.Position, false
	}
	resolution := resolveTargetApproachMovement(
		runtime.movementPlanner,
		runtime.regionID,
		entity.Position,
		runtime.position,
		mobTemplateAttackRange(entity.TemplateID),
	)
	if resolution.Status != movementPlanStatusAccepted {
		return entity.Position, false
	}
	next := movePointAlongPath(entity.Position, resolution.Plan.Waypoints, runtime.mobAIMoveDistance(entity, now))
	return next, distance(entity.Position, next) > 0.001
}

func (runtime *attachedRuntime) mobAIMoveDistance(entity runtimeEntity, now time.Time) float64 {
	lastAIAtMS := runtimeMobLastAIAt(entity)
	delta := mobAIDefaultDelta
	if lastAIAtMS > 0 {
		lastAIAt := time.UnixMilli(lastAIAtMS)
		if now.After(lastAIAt) {
			delta = now.Sub(lastAIAt)
		}
	}
	return mobTemplateMoveSpeed(entity.TemplateID) * delta.Seconds()
}

func movePointAlongPath(start runtimePoint, waypoints []runtimePoint, step float64) runtimePoint {
	if step <= 0 || len(waypoints) == 0 {
		return start
	}
	position := start
	remaining := step
	for _, waypoint := range waypoints {
		legDistance := distance(position, waypoint)
		if legDistance <= 0.001 {
			position = waypoint
			continue
		}
		if remaining >= legDistance {
			position = waypoint
			remaining -= legDistance
			continue
		}
		ratio := remaining / legDistance
		return runtimePoint{
			X: position.X + (waypoint.X-position.X)*ratio,
			Z: position.Z + (waypoint.Z-position.Z)*ratio,
		}
	}
	return position
}

func (runtime *attachedRuntime) mobAIPatch(entity runtimeEntity, includePosition bool) map[string]any {
	patch := map[string]any{
		"entity_id":       entity.EntityID,
		"ai_state":        entity.State["ai_state"],
		"personality":     entity.State["personality"],
		"aggro_target_id": entity.State["aggro_target_id"],
	}
	if includePosition {
		patch["position"] = entity.Position
	}
	return patch
}

func (runtime *attachedRuntime) markMobAggroAndMaybeCounterAttackLocked(entityID string, now time.Time) bool {
	entity, exists := runtime.knownEntities[entityID]
	if !exists || entity.EntityType != "mob" || !isRuntimeEntityAlive(entity) {
		return false
	}
	entity.State["ai_state"] = string(mobAIStateAggro)
	entity.State["aggro_target_id"] = runtime.characterID
	entity.State["last_ai_at_ms"] = now.UnixMilli()
	runtime.knownEntities[entityID] = entity
	if distance(entity.Position, runtime.position) > mobTemplateAttackRange(entity.TemplateID) {
		return false
	}
	return runtime.mobAttackPlayerLocked(entityID, now)
}

func (runtime *attachedRuntime) mobAttackPlayerLocked(entityID string, now time.Time) bool {
	if entityID == "" || runtime.currentHP <= 0 {
		return false
	}
	entity, exists := runtime.knownEntities[entityID]
	if !exists || entity.EntityType != "mob" || !isRuntimeEntityAlive(entity) {
		return false
	}
	damage := incomingPlayerDamage(mobTemplateAttack(entity.TemplateID), runtime.derivedStats.Defense)
	runtime.currentHP -= damage
	if runtime.currentHP < 0 {
		runtime.currentHP = 0
	}
	entity.State["next_attack_at_ms"] = now.Add(mobTemplateAttackInterval(entity.TemplateID)).UnixMilli()
	runtime.knownEntities[entityID] = entity
	if runtime.currentHP == 0 {
		runtime.enterDeadState(now)
	}
	return true
}
