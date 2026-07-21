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

func (runtime *attachedRuntime) resolveAutoBasicAttackLocked(now time.Time) []map[string]any {
	if runtime.autoBasicAttack == nil || runtime.queuedBasicAttack != nil || runtime.isPlayerDead() {
		return nil
	}

	queued := runtime.autoBasicAttack
	target, exists := runtime.knownEntities[queued.TargetID]
	if !exists || target.EntityType != "mob" || !isRuntimeEntityAlive(target) {
		runtime.autoBasicAttack = nil
		runtime.queuedBasicAttack = nil
		runtime.clearActiveMovementLocked()
		return nil
	}
	if endsAt, cooling := runtime.cooldownEndsAt["basic_attack"]; cooling && now.Before(endsAt) {
		return nil
	}
	if distance(runtime.position, target.Position) > basicAttackRange {
		if runtime.activeMovement != nil {
			return nil
		}
		return runtime.queueBasicAttackApproachLocked(commandEnvelope{
			CommandID:  queued.CommandID,
			CommandSeq: queued.CommandSeq,
			Type:       "basic_attack",
		}, target, now)
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

func approachDestinationForRange(start, target runtimePoint, interactionRange float64) runtimePoint {
	if interactionRange <= 0 {
		return target
	}

	dx := start.X - target.X
	dz := start.Z - target.Z
	length := math.Hypot(dx, dz)
	if length <= 0.001 {
		return runtimePoint{X: target.X - interactionRange*0.85, Z: target.Z}
	}

	stopDistance := math.Max(0.25, interactionRange*0.85)
	if stopDistance >= interactionRange {
		stopDistance = interactionRange * 0.9
	}
	return runtimePoint{
		X: target.X + (dx/length)*stopDistance,
		Z: target.Z + (dz/length)*stopDistance,
	}
}

func resolveTargetApproachMovement(planner movementPlanner, regionID string, start runtimePoint, target runtimePoint, interactionRange float64) movementResolution {
	profile := movementProfile{ActorRadius: defaultMovementActorRadius}
	var firstRejected movementResolution
	for _, candidate := range targetApproachCandidates(start, target, interactionRange) {
		resolution := planner.Resolve(context.Background(), regionID, start, candidate, profile)
		if resolution.Status == movementPlanStatusCanceled {
			return resolution
		}
		if resolution.Status == movementPlanStatusRejected {
			if firstRejected.Status == "" {
				firstRejected = resolution
			}
			continue
		}
		if targetApproachPlanEndsInRange(resolution, target, interactionRange) {
			return resolution
		}
	}

	if firstRejected.Status == movementPlanStatusRejected {
		return firstRejected
	}
	return movementResolution{
		Status:           movementPlanStatusRejected,
		ReasonCode:       "movement.path_unreachable",
		CorrectionReason: "path_unreachable",
	}
}

func targetApproachPlanEndsInRange(resolution movementResolution, target runtimePoint, interactionRange float64) bool {
	if resolution.Status != movementPlanStatusAccepted {
		return false
	}
	return distance(resolution.Plan.AcceptedDestination, target) <= interactionRange+0.001
}

func targetApproachCandidates(start, target runtimePoint, interactionRange float64) []runtimePoint {
	if interactionRange <= 0 {
		return []runtimePoint{target}
	}

	baseAngle := math.Atan2(start.Z-target.Z, start.X-target.X)
	if distance(start, target) <= 0.001 {
		baseAngle = 0
	}

	radii := []float64{
		interactionRange - defaultMovementActorRadius - 0.25,
		interactionRange * 0.85,
		interactionRange * 0.65,
		interactionRange * 0.45,
		interactionRange * 0.25,
	}
	angleOffsets := []float64{
		0,
		math.Pi / 6,
		-math.Pi / 6,
		math.Pi / 4,
		-math.Pi / 4,
		math.Pi / 2,
		-math.Pi / 2,
		3 * math.Pi / 4,
		-3 * math.Pi / 4,
		math.Pi,
	}

	candidates := make([]runtimePoint, 0, 1+len(radii)*len(angleOffsets))
	candidates = append(candidates, approachDestinationForRange(start, target, interactionRange))
	for _, radius := range radii {
		if radius <= 0 {
			continue
		}
		for _, offset := range angleOffsets {
			angle := baseAngle + offset
			candidates = append(candidates, runtimePoint{
				X: target.X + math.Cos(angle)*radius,
				Z: target.Z + math.Sin(angle)*radius,
			})
		}
	}
	return candidates
}

func (runtime *attachedRuntime) syncMovementTo(now time.Time) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.advanceMovementLocked(now)
}

func (runtime *attachedRuntime) collectTickMessages(now time.Time) ([]map[string]any, bool, bool) {
	messages, changed, respawned, _ := runtime.collectTickMessagesWithStore(now, nil)
	return messages, changed, respawned
}

func (runtime *attachedRuntime) collectTickMessagesWithStore(now time.Time, store *Store) ([]map[string]any, bool, bool, *PvPKarmaRecoveryEvent) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	movementChanged := runtime.advanceMovementLocked(now)
	resourceChanged := runtime.applyIdleRegenLocked(now)
	mobPatches, mobAIChanged := runtime.resolveMobAILocked(now)
	pvpPresenceChanged := runtime.pvpStateDirty
	pvpFlagReason := ""
	var karmaRecoveryEvent *PvPKarmaRecoveryEvent
	if !runtime.pvpFlagUntil.IsZero() && !now.Before(runtime.pvpFlagUntil) {
		runtime.pvpFlagPersistenceDirty = true
		if store == nil || store.Characters == nil || store.Characters.UpdatePvPFlagUntil(context.Background(), runtime.characterID, time.Time{}) == nil {
			runtime.pvpFlagUntil = time.Time{}
			runtime.pvpFlagPersistenceDirty = false
			pvpPresenceChanged = true
			pvpFlagReason = "pvp.flag_expired"
		}
	} else if runtime.pvpFlagPersistenceDirty {
		if store == nil || store.Characters == nil || store.Characters.UpdatePvPFlagUntil(context.Background(), runtime.characterID, runtime.pvpFlagUntil) == nil {
			runtime.pvpFlagPersistenceDirty = false
		}
	}
	if store != nil && store.Characters != nil && !runtime.karmaRecoveryDueAt.IsZero() && !now.Before(runtime.karmaRecoveryDueAt) {
		recoveryCommit, err := store.Characters.ApplyKarmaRecovery(context.Background(), runtime.characterID, now.UTC(), "tick")
		if err == nil && recoveryCommit != nil {
			runtime.karma = recoveryCommit.State.Karma
			runtime.karmaRecoveryDueAt = recoveryCommit.State.KarmaRecoveryDueAt
			if recoveryCommit.Event != nil {
				karmaRecoveryEvent = recoveryCommit.Event
				pvpPresenceChanged = true
				runtime.pvpStateDirty = true
			}
		}
	}
	runtime.pvpStateDirty = false
	outbound := make([]map[string]any, 0, 2)
	if movementChanged || resourceChanged || mobAIChanged || pvpPresenceChanged {
		runtime.revision++
		extra := map[string]any{}
		if pvpFlagReason != "" {
			extra["pvp_flag_reason"] = pvpFlagReason
		}
		self := runtime.selfDelta(now, extra)
		if movementChanged {
			self = runtime.movementSelfDeltaLocked(now, "")
			if pvpFlagReason != "" {
				self["pvp_flag_reason"] = pvpFlagReason
			}
		}
		outbound = append(outbound, deltaMessage(
			runtime.revision,
			"",
			0,
			self,
			mobPatches,
			nil,
		))
	}

	queuedSkillMessages := runtime.resolveQueuedSkillLocked(now)
	queuedBasicAttackMessages := runtime.resolveQueuedBasicAttackLocked(now)
	autoBasicAttackMessages := runtime.resolveAutoBasicAttackLocked(now)
	queuedLootPickupMessages := runtime.resolveQueuedLootPickupLocked(now, store)
	outbound = append(outbound, queuedSkillMessages...)
	outbound = append(outbound, queuedBasicAttackMessages...)
	outbound = append(outbound, autoBasicAttackMessages...)
	outbound = append(outbound, queuedLootPickupMessages...)

	lifecycleMessages := runtime.collectLifecycleMessagesLocked(now)
	outbound = append(outbound, lifecycleMessages...)
	return outbound, movementChanged || pvpPresenceChanged, containsPlayerRespawnDelta(lifecycleMessages), karmaRecoveryEvent
}
