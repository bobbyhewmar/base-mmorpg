package app

import (
	"context"
	"math"
	"time"
)

type runtimeMovementState struct {
	GeodataVersion      string
	AcceptedDestination runtimePoint
	Waypoints           []runtimePoint
	LastAdvancedAt      time.Time
}

func cloneRuntimePath(points []runtimePoint) []runtimePoint {
	if len(points) == 0 {
		return nil
	}
	cloned := make([]runtimePoint, len(points))
	copy(cloned, points)
	return cloned
}

func (runtime *attachedRuntime) currentGeodataVersionLocked() string {
	if runtime.movementPlanner == nil {
		return "unavailable"
	}
	return runtime.movementPlanner.GeodataVersion(runtime.regionID)
}

func (runtime *attachedRuntime) setActiveMovementLocked(plan movementPlan, now time.Time) {
	runtime.activeMovement = &runtimeMovementState{
		GeodataVersion:      plan.GeodataVersion,
		AcceptedDestination: plan.AcceptedDestination,
		Waypoints:           cloneRuntimePath(plan.Waypoints),
		LastAdvancedAt:      now,
	}
}

func (runtime *attachedRuntime) clearActiveMovementLocked() {
	runtime.activeMovement = nil
}

func (runtime *attachedRuntime) resetIdleRegenClockLocked(now time.Time) {
	runtime.stationarySince = now
	runtime.lastIdleRegenAt = now
}

func (runtime *attachedRuntime) movementPathSnapshotLocked() []runtimePoint {
	if runtime.activeMovement == nil {
		return []runtimePoint{}
	}
	path := make([]runtimePoint, 0, len(runtime.activeMovement.Waypoints)+1)
	path = append(path, runtime.position)
	path = append(path, cloneRuntimePath(runtime.activeMovement.Waypoints)...)
	return path
}

func (runtime *attachedRuntime) movementSelfDeltaLocked(now time.Time, reason string) map[string]any {
	extra := map[string]any{
		"position":           runtime.position,
		"facing":             runtime.facing,
		"geodata_version":    runtime.currentGeodataVersionLocked(),
		"authoritative_path": runtime.movementPathSnapshotLocked(),
	}
	if reason != "" {
		extra["movement_reason"] = reason
	}
	return runtime.selfDelta(now, extra)
}

func (runtime *attachedRuntime) advanceMovementLocked(now time.Time) bool {
	if runtime.activeMovement == nil {
		return false
	}
	if runtime.isPlayerDead() {
		runtime.clearActiveMovementLocked()
		runtime.resetIdleRegenClockLocked(now)
		return false
	}
	if runtime.activeMovement.LastAdvancedAt.IsZero() || now.Before(runtime.activeMovement.LastAdvancedAt) {
		runtime.activeMovement.LastAdvancedAt = now
		return false
	}

	moveSpeed := runtime.derivedStats.MoveSpeed
	if moveSpeed <= 0 {
		moveSpeed = 1
	}
	remainingDistance := moveSpeed * now.Sub(runtime.activeMovement.LastAdvancedAt).Seconds()
	runtime.activeMovement.LastAdvancedAt = now
	if remainingDistance <= 0 {
		return false
	}

	changed := false
	for remainingDistance > 0 && runtime.activeMovement != nil && len(runtime.activeMovement.Waypoints) > 0 {
		nextWaypoint := runtime.activeMovement.Waypoints[0]
		legDistance := distance(runtime.position, nextWaypoint)
		if legDistance <= 0.001 {
			runtime.position = nextWaypoint
			runtime.activeMovement.Waypoints = runtime.activeMovement.Waypoints[1:]
			changed = true
			continue
		}

		runtime.facing = angleBetween(runtime.position, nextWaypoint)
		if remainingDistance >= legDistance {
			runtime.position = nextWaypoint
			runtime.activeMovement.Waypoints = runtime.activeMovement.Waypoints[1:]
			remainingDistance -= legDistance
			changed = true
			continue
		}

		ratio := remainingDistance / legDistance
		runtime.position = runtimePoint{
			X: runtime.position.X + (nextWaypoint.X-runtime.position.X)*ratio,
			Z: runtime.position.Z + (nextWaypoint.Z-runtime.position.Z)*ratio,
		}
		remainingDistance = 0
		changed = true
	}

	if runtime.activeMovement != nil && len(runtime.activeMovement.Waypoints) == 0 {
		runtime.clearActiveMovementLocked()
	}
	if changed {
		runtime.syncLocalPetEntityLocked()
		runtime.resetIdleRegenClockLocked(now)
	}
	return changed
}

func idleRegenAmount(maxValue int, ticks int) int {
	if maxValue <= 0 || ticks <= 0 {
		return 0
	}
	return max(1, int(math.Ceil(float64(maxValue)*idleRegenPercent))) * ticks
}

func (runtime *attachedRuntime) applyIdleRegenLocked(now time.Time) bool {
	if runtime.isPlayerDead() || runtime.activeMovement != nil {
		runtime.resetIdleRegenClockLocked(now)
		return false
	}
	if runtime.stationarySince.IsZero() || now.Before(runtime.stationarySince) {
		runtime.resetIdleRegenClockLocked(now)
		return false
	}

	regenStart := runtime.stationarySince.Add(idleRegenDelay)
	lastRegenAt := runtime.lastIdleRegenAt
	if lastRegenAt.IsZero() || lastRegenAt.Before(regenStart) {
		lastRegenAt = regenStart
	}
	if now.Before(lastRegenAt.Add(idleRegenTick)) {
		return false
	}

	ticks := int(now.Sub(lastRegenAt) / idleRegenTick)
	if ticks <= 0 {
		return false
	}

	previousCP := runtime.currentCP
	previousHP := runtime.currentHP
	previousMP := runtime.currentMP
	runtime.currentCP = min(runtime.derivedStats.MaxCP, runtime.currentCP+idleRegenAmount(runtime.derivedStats.MaxCP, ticks))
	runtime.currentHP = min(runtime.derivedStats.MaxHP, runtime.currentHP+idleRegenAmount(runtime.derivedStats.MaxHP, ticks))
	runtime.currentMP = min(runtime.derivedStats.MaxMP, runtime.currentMP+idleRegenAmount(runtime.derivedStats.MaxMP, ticks))
	runtime.lastIdleRegenAt = lastRegenAt.Add(time.Duration(ticks) * idleRegenTick)
	return runtime.currentCP != previousCP || runtime.currentHP != previousHP || runtime.currentMP != previousMP
}

func (runtime *attachedRuntime) resolveQueuedSkillLocked(now time.Time) []map[string]any {
	if runtime.queuedSkill == nil || runtime.isPlayerDead() {
		return nil
	}

	queued := runtime.queuedSkill
	skill, exists := supportedSkills[queued.SkillID]
	if !exists {
		runtime.queuedSkill = nil
		return nil
	}
	target, exists := runtime.knownEntities[queued.TargetID]
	if !exists || target.EntityType != "mob" || !isRuntimeEntityAlive(target) {
		runtime.queuedSkill = nil
		runtime.clearActiveMovementLocked()
		return []map[string]any{rejectMessage(queued.CommandID, queued.CommandSeq, "combat.target_dead", "Referenced target is no longer attackable.")}
	}
	if distance(runtime.position, target.Position) > skill.Range {
		if runtime.activeMovement == nil {
			runtime.queuedSkill = nil
			return []map[string]any{rejectMessage(queued.CommandID, queued.CommandSeq, "combat.target_out_of_range", "Referenced target is outside skill range.")}
		}
		return nil
	}
	if runtime.currentMP < skill.MPCost {
		runtime.queuedSkill = nil
		runtime.clearActiveMovementLocked()
		return []map[string]any{rejectMessage(queued.CommandID, queued.CommandSeq, "combat.insufficient_mp", "Actor lacks MP for this skill.")}
	}
	if endsAt, cooling := runtime.cooldownEndsAt[skill.ID]; cooling && now.Before(endsAt) {
		runtime.queuedSkill = nil
		runtime.clearActiveMovementLocked()
		return []map[string]any{rejectMessage(queued.CommandID, queued.CommandSeq, "combat.cooldown_active", "Skill is still on cooldown.")}
	}

	return runtime.activateSkillLocked(queued.CommandID, queued.CommandSeq, skill, target, now)
}

func (runtime *attachedRuntime) resolveQueuedBasicAttackLocked(now time.Time) []map[string]any {
	if runtime.queuedBasicAttack == nil || runtime.isPlayerDead() {
		return nil
	}

	queued := runtime.queuedBasicAttack
	target, exists := runtime.knownEntities[queued.TargetID]
	if !exists || target.EntityType != "mob" || !isRuntimeEntityAlive(target) {
		runtime.queuedBasicAttack = nil
		runtime.clearActiveMovementLocked()
		return []map[string]any{rejectMessage(queued.CommandID, queued.CommandSeq, "combat.target_dead", "Referenced target is no longer attackable.")}
	}
	if distance(runtime.position, target.Position) > basicAttackRange {
		if runtime.activeMovement == nil {
			runtime.queuedBasicAttack = nil
			return []map[string]any{rejectMessage(queued.CommandID, queued.CommandSeq, "combat.target_out_of_range", "Referenced target is outside basic attack range.")}
		}
		return nil
	}
	if endsAt, cooling := runtime.cooldownEndsAt["basic_attack"]; cooling && now.Before(endsAt) {
		runtime.queuedBasicAttack = nil
		runtime.clearActiveMovementLocked()
		return []map[string]any{rejectMessage(queued.CommandID, queued.CommandSeq, "combat.cooldown_active", "Basic attack is still on cooldown.")}
	}

	return runtime.activateBasicAttackLocked(queued.CommandID, queued.CommandSeq, target, now)
}

func (runtime *attachedRuntime) resolveQueuedLootPickupLocked(now time.Time, store *Store) []map[string]any {
	if runtime.queuedLootPickup == nil || runtime.isPlayerDead() || store == nil {
		return nil
	}

	queued := runtime.queuedLootPickup
	loot, exists := runtime.knownEntities[queued.LootID]
	if !exists || loot.EntityType != "loot" {
		runtime.queuedLootPickup = nil
		runtime.clearActiveMovementLocked()
		return []map[string]any{rejectMessage(queued.CommandID, queued.CommandSeq, "world.entity_not_known", "Referenced loot is not in the current known-set.")}
	}
	if distance(runtime.position, loot.Position) > lootPickupRange {
		if runtime.activeMovement == nil {
			runtime.queuedLootPickup = nil
			return []map[string]any{rejectMessage(queued.CommandID, queued.CommandSeq, "world.loot_out_of_reach", "Referenced loot is still out of reach.")}
		}
		return nil
	}

	return runtime.collectLootPickupLocked(context.Background(), store, queued.CommandID, queued.CommandSeq, queued.LootID, loot, now)
}

func angleBetween(start, destination runtimePoint) float64 {
	return math.Atan2(destination.Z-start.Z, destination.X-start.X)
}

func (runtime *attachedRuntime) syncMovementTo(now time.Time) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.advanceMovementLocked(now)
}

func (runtime *attachedRuntime) collectTickMessages(now time.Time) ([]map[string]any, bool, bool) {
	return runtime.collectTickMessagesWithStore(now, nil)
}

func (runtime *attachedRuntime) collectTickMessagesWithStore(now time.Time, store *Store) ([]map[string]any, bool, bool) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	movementChanged := runtime.advanceMovementLocked(now)
	resourceChanged := runtime.applyIdleRegenLocked(now)
	queuedSkillMessages := runtime.resolveQueuedSkillLocked(now)
	queuedBasicAttackMessages := runtime.resolveQueuedBasicAttackLocked(now)
	queuedLootPickupMessages := runtime.resolveQueuedLootPickupLocked(now, store)
	outbound := make([]map[string]any, 0, 2+len(queuedSkillMessages)+len(queuedBasicAttackMessages)+len(queuedLootPickupMessages))
	if movementChanged || resourceChanged {
		runtime.revision++
		self := runtime.selfDelta(now, nil)
		if movementChanged {
			self = runtime.movementSelfDeltaLocked(now, "")
		}
		outbound = append(outbound, deltaMessage(
			runtime.revision,
			"",
			0,
			self,
			nil,
			nil,
		))
	}
	outbound = append(outbound, queuedSkillMessages...)
	outbound = append(outbound, queuedBasicAttackMessages...)
	outbound = append(outbound, queuedLootPickupMessages...)

	lifecycleMessages := runtime.collectLifecycleMessagesLocked(now)
	outbound = append(outbound, lifecycleMessages...)
	return outbound, movementChanged, containsPlayerRespawnDelta(lifecycleMessages)
}
