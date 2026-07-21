package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

const (
	pvpFlagDuration = 30 * time.Second
	pkKarmaGain     = 100
)

func (s *Server) processCombatCommand(ctx context.Context, session *Session, runtime *attachedRuntime, command commandEnvelope) ([]map[string]any, bool) {
	if !runtime.commandTargetsPlayer(command) {
		return runtime.processCommand(command), false
	}
	return s.processPvPCommand(ctx, session, runtime, command), true
}

func (runtime *attachedRuntime) commandTargetsPlayer(command commandEnvelope) bool {
	if runtime == nil || (command.Type != "basic_attack" && command.Type != "use_skill") {
		return false
	}
	var payload struct {
		TargetID string `json:"target_id"`
	}
	if err := json.Unmarshal(command.Payload, &payload); err != nil || payload.TargetID == "" {
		return false
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if payload.TargetID == runtime.characterID {
		return true
	}
	entity, exists := runtime.knownEntities[payload.TargetID]
	return exists && entity.EntityType == "player"
}

func (s *Server) processPvPCommand(ctx context.Context, session *Session, actor *attachedRuntime, command commandEnvelope) []map[string]any {
	if s == nil || session == nil || actor == nil || s.store == nil || s.store.Characters == nil {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "internal.unexpected_error", "Player combat pipeline is unavailable.")}
	}

	s.pvpMu.Lock()
	defer s.pvpMu.Unlock()

	actor.mu.Lock()
	actor.advanceMovementLocked(time.Now())
	parsed, reject := actor.preValidate(command)
	if reject != nil {
		actor.mu.Unlock()
		return []map[string]any{reject}
	}
	actor.expectedCommandSeq++
	outbound := []map[string]any{ackMessage(command.CommandID, command.CommandSeq)}
	if actor.isPlayerDead() {
		actor.mu.Unlock()
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "combat.actor_dead", "Actor is currently dead."))
	}
	if parsed.targetID == actor.characterID {
		actor.mu.Unlock()
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "pvp.self_target", "A player cannot attack themselves."))
	}
	knownTarget, targetKnown := actor.knownEntities[parsed.targetID]
	if !targetKnown {
		actor.mu.Unlock()
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "world.entity_not_known", "Referenced entity is not in the current known-set."))
	}
	if knownTarget.EntityType != "player" {
		actor.mu.Unlock()
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "world.entity_not_interactable", "Referenced entity is not a player combat target."))
	}
	actor.mu.Unlock()

	presenceScope, targetAttached, _, err := s.resolveCharacterPresence(ctx, parsed.targetID)
	if err != nil {
		s.recordStoreError("gameplay_sessions.resolve_pvp_target_presence", err)
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to resolve authoritative player presence."))
	}
	if presenceScope == characterPresenceRemote {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "presence.target_remote", "Referenced player is online on another server instance and cannot be attacked locally."))
	}
	if presenceScope != characterPresenceLocal || targetAttached == nil || targetAttached.runtime == nil {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "pvp.target_unavailable", "Referenced player is no longer available."))
	}
	if targetAttached.fencingToken != 0 {
		targetSession := &Session{
			ID:               targetAttached.sessionID,
			CharacterID:      targetAttached.characterID,
			ServerInstanceID: targetAttached.serverInstanceID,
			FencingToken:     targetAttached.fencingToken,
		}
		if err := s.renewSessionOwnership(ctx, targetSession, targetAttached.runtime, targetAttached.runtime.regionIDValue(), false); err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "pvp.target_unavailable", "Referenced player no longer has valid local ownership."))
		}
	}
	target := targetAttached.runtime
	now := time.Now()

	actor.mu.Lock()
	defer actor.mu.Unlock()
	target.mu.Lock()
	defer target.mu.Unlock()

	actor.advanceMovementLocked(now)
	target.advanceMovementLocked(now)
	knownTarget, targetKnown = actor.knownEntities[parsed.targetID]
	if !targetKnown || knownTarget.EntityType != "player" {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "world.entity_not_known", "Referenced entity is not in the current known-set."))
	}
	if actor.regionID != target.regionID {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "pvp.target_out_of_region", "Referenced player is outside the actor region."))
	}
	switch reason := pvpPolicyReason(actor.regionID, actor.position, target.position); reason {
	case "pvp.safe_zone":
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, reason, "Player combat is blocked in this safe area."))
	case "pvp.region_restricted":
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "pvp.region_restricted", "Player combat is not enabled in this region."))
	}
	sameParty, err := s.charactersShareParty(ctx, actor.characterID, target.characterID)
	if err != nil {
		s.recordStoreError("parties.inspect_pvp_relation", err)
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect player combat party relation."))
	}
	if sameParty {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "pvp.same_party", "Party members cannot attack each other."))
	}
	sameClan, err := s.charactersShareClan(ctx, actor.characterID, target.characterID)
	if err != nil {
		s.recordStoreError("clans.inspect_pvp_relation", err)
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect player combat clan relation."))
	}
	if sameClan {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "pvp.same_clan", "Clan members cannot attack each other."))
	}
	sameAlliance, err := s.charactersShareAlliance(ctx, actor.characterID, target.characterID)
	if err != nil {
		s.recordStoreError("alliances.inspect_pvp_relation", err)
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect player combat alliance relation."))
	}
	if sameAlliance {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "pvp.same_alliance", "Alliance members cannot attack each other in the current PvP slice."))
	}
	if target.isPlayerDead() {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "combat.target_dead", "Referenced target is already dead."))
	}

	damage, cooldownID, cooldownDuration, mpCost, combatReject := actor.resolvePvPAttackLocked(command, parsed, target, now)
	if combatReject != nil {
		return append(outbound, combatReject)
	}

	metadata := commandAuditMetadataFromContext(ctx)
	commit, err := s.store.Characters.ApplyPvPCombat(ctx, PvPCombatMutation{
		EventID:             randomID("pvp_event"),
		AttackerCharacterID: actor.characterID,
		VictimCharacterID:   target.characterID,
		ActionType:          command.Type,
		SkillID:             parsed.skillID,
		Damage:              damage,
		MPCost:              mpCost,
		CooldownID:          cooldownID,
		CooldownDuration:    cooldownDuration,
		SessionID:           metadata.SessionID,
		CommandID:           metadata.CommandID,
		CommandSeq:          metadata.CommandSeq,
		OccurredAt:          now,
	})
	if err != nil {
		switch {
		case errors.Is(err, errPvPActorDead):
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "combat.actor_dead", "Actor is currently dead."))
		case errors.Is(err, errPvPTargetDead):
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "combat.target_dead", "Referenced target is already dead."))
		case errors.Is(err, errPvPInsufficientMP):
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "combat.insufficient_mp", "Actor lacks MP for this skill."))
		case errors.Is(err, errPvPCooldownActive):
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "combat.cooldown_active", "Attack is still on cooldown."))
		default:
			s.recordStoreError("characters.apply_pvp_combat", err)
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist player combat outcome."))
		}
	}
	now = commit.Event.CreatedAt
	attackerState := commit.Attacker
	targetState := commit.Victim
	s.recordPvPCombatAuditEvent(commit.Event)
	for _, recoveryEvent := range commit.KarmaRecoveryEvents {
		recoveryState := attackerState
		if recoveryEvent.CharacterID == target.characterID {
			recoveryState = targetState
		}
		s.recordPvPKarmaRecoveryEvent(recoveryEvent, recoveryState)
	}
	actor.currentCP = attackerState.CurrentCP
	actor.currentHP = attackerState.CurrentHP
	actor.currentMP = attackerState.CurrentMP
	actor.pvpKills = attackerState.PvPKills
	actor.pkCount = attackerState.PKCount
	actor.karma = attackerState.Karma
	actor.pvpFlagUntil = attackerState.PvPFlagUntil
	actor.karmaRecoveryDueAt = attackerState.KarmaRecoveryDueAt
	actor.pvpFlagPersistenceDirty = false
	actor.targetID = target.characterID
	if commit.CooldownID != "" {
		actor.cooldownEndsAt[commit.CooldownID] = commit.CooldownEndsAt
	}
	actor.queuedSkill = nil
	actor.queuedBasicAttack = nil
	actor.autoBasicAttack = nil
	actor.queuedLootPickup = nil
	actor.clearActiveMovementLocked()

	target.currentCP = targetState.CurrentCP
	target.currentHP = targetState.CurrentHP
	target.currentMP = targetState.CurrentMP
	target.pvpKills = targetState.PvPKills
	target.pkCount = targetState.PKCount
	target.karma = targetState.Karma
	target.pvpFlagUntil = targetState.PvPFlagUntil
	target.karmaRecoveryDueAt = targetState.KarmaRecoveryDueAt
	target.pvpFlagPersistenceDirty = false
	target.pvpStateDirty = true
	if target.currentHP == 0 {
		target.enterDeadState(now)
		target.pvpFlagPersistenceDirty = false
	}

	dead := target.currentHP == 0
	entity := actor.knownEntities[target.characterID]
	entity.Position = target.position
	entity.State["cp"] = target.currentCP
	entity.State["hp"] = target.currentHP
	entity.State["dead"] = dead
	entity.State["alive"] = !dead
	entity.State["pvp_flagged"] = target.pvpFlaggedAt(now)
	entity.State["pvp_flag_until_ms"] = target.projectedPvPFlagUntilMS(now)
	entity.State["pvp_kills"] = target.pvpKills
	entity.State["pk_count"] = target.pkCount
	entity.State["karma"] = target.karma
	actor.knownEntities[target.characterID] = entity
	if dead {
		actor.targetID = ""
	}

	actor.revision++
	patch := map[string]any{
		"entity_id":         target.characterID,
		"position":          target.position,
		"cp":                target.currentCP,
		"hp":                target.currentHP,
		"dead":              dead,
		"alive":             !dead,
		"pvp_flagged":       target.pvpFlaggedAt(now),
		"pvp_flag_until_ms": target.projectedPvPFlagUntilMS(now),
		"pvp_kills":         target.pvpKills,
		"pk_count":          target.pkCount,
		"karma":             target.karma,
	}
	return append(outbound, deltaMessage(
		actor.revision,
		command.CommandID,
		command.CommandSeq,
		actor.movementSelfDeltaLocked(now, ""),
		[]map[string]any{patch},
		nil,
	))
}

func (s *Server) charactersShareParty(ctx context.Context, actorCharacterID string, targetCharacterID string) (bool, error) {
	if s.store.Parties == nil {
		return false, errors.New("party repository unavailable")
	}
	actorParty, err := s.store.Parties.GetByCharacterID(ctx, actorCharacterID)
	if err != nil && !errors.Is(err, errRecordNotFound) {
		return false, err
	}
	if actorParty == nil || actorParty.ID == "" {
		return false, nil
	}
	targetParty, err := s.store.Parties.GetByCharacterID(ctx, targetCharacterID)
	if err != nil {
		if errors.Is(err, errRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return targetParty != nil && targetParty.ID == actorParty.ID, nil
}

func (s *Server) charactersShareClan(ctx context.Context, actorCharacterID string, targetCharacterID string) (bool, error) {
	if s.store.Clans == nil {
		return false, errors.New("clan repository unavailable")
	}
	actorClan, err := s.store.Clans.GetByCharacterID(ctx, actorCharacterID)
	if err != nil && !errors.Is(err, errRecordNotFound) {
		return false, err
	}
	if actorClan == nil || actorClan.ID == "" {
		return false, nil
	}
	targetClan, err := s.store.Clans.GetByCharacterID(ctx, targetCharacterID)
	if err != nil {
		if errors.Is(err, errRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return targetClan != nil && targetClan.ID == actorClan.ID, nil
}

func (s *Server) charactersShareAlliance(ctx context.Context, actorCharacterID string, targetCharacterID string) (bool, error) {
	if s.store.Alliances == nil {
		return false, errors.New("alliance repository unavailable")
	}
	actorAlliance, err := s.store.Alliances.GetByCharacterID(ctx, actorCharacterID)
	if err != nil && !errors.Is(err, errRecordNotFound) {
		return false, err
	}
	if actorAlliance == nil || actorAlliance.ID == "" {
		return false, nil
	}
	targetAlliance, err := s.store.Alliances.GetByCharacterID(ctx, targetCharacterID)
	if err != nil {
		if errors.Is(err, errRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return targetAlliance != nil && targetAlliance.ID == actorAlliance.ID, nil
}

func (runtime *attachedRuntime) resolvePvPAttackLocked(command commandEnvelope, parsed *parsedCommand, target *attachedRuntime, now time.Time) (int, string, time.Duration, int, map[string]any) {
	switch parsed.commandType {
	case "basic_attack":
		if endsAt, cooling := runtime.cooldownEndsAt["basic_attack"]; cooling && now.Before(endsAt) {
			return 0, "", 0, 0, rejectMessage(command.CommandID, command.CommandSeq, "combat.cooldown_active", "Basic attack is still on cooldown.")
		}
		if distance(runtime.position, target.position) > basicAttackRange {
			return 0, "", 0, 0, rejectMessage(command.CommandID, command.CommandSeq, "combat.out_of_range", "Referenced player is outside basic attack range.")
		}
		return maxBasicAttackDamage(runtime.derivedStats.Attack, target.derivedStats.Defense), "basic_attack", basicAttackCooldown, 0, nil
	case "use_skill":
		skill, exists := supportedSkills[parsed.skillID]
		if !exists {
			return 0, "", 0, 0, rejectMessage(command.CommandID, command.CommandSeq, "combat.skill_unknown", "Skill is not supported.")
		}
		knownCategory, known := knownSkillCategory(runtime.characterBaseClass, runtime.characterLevel, parsed.skillID)
		if !known {
			return 0, "", 0, 0, rejectMessage(command.CommandID, command.CommandSeq, "combat.skill_not_learned", "Skill is not learned by this character.")
		}
		if knownCategory != skillCategoryActive || skill.Category != skillCategoryActive {
			return 0, "", 0, 0, rejectMessage(command.CommandID, command.CommandSeq, "combat.skill_not_active", "Passive skills cannot be activated directly.")
		}
		if skill.TargetType != "single_target_enemy" {
			return 0, "", 0, 0, rejectMessage(command.CommandID, command.CommandSeq, "pvp.skill_not_supported", "This skill is not enabled for player combat in the current slice.")
		}
		if runtime.currentMP < skill.MPCost {
			return 0, "", 0, 0, rejectMessage(command.CommandID, command.CommandSeq, "combat.insufficient_mp", "Actor lacks MP for this skill.")
		}
		if endsAt, cooling := runtime.cooldownEndsAt[skill.ID]; cooling && now.Before(endsAt) {
			return 0, "", 0, 0, rejectMessage(command.CommandID, command.CommandSeq, "combat.cooldown_active", "Skill is still on cooldown.")
		}
		if distance(runtime.position, target.position) > skill.Range {
			return 0, "", 0, 0, rejectMessage(command.CommandID, command.CommandSeq, "combat.out_of_range", "Referenced player is outside skill range.")
		}
		return maxSingleTargetDamage(runtime.derivedStats.Attack, skill.Power, target.derivedStats.Defense), skill.ID, time.Duration(skill.CooldownMS) * time.Millisecond, skill.MPCost, nil
	default:
		return 0, "", 0, 0, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Unsupported player combat command.")
	}
}

func applyPlayerDamage(currentCP int, currentHP int, damage int) (int, int) {
	nextCP := max(0, currentCP)
	nextHP := max(0, currentHP)
	remaining := max(0, damage)
	if nextCP >= remaining {
		return nextCP - remaining, nextHP
	}
	remaining -= nextCP
	nextCP = 0
	nextHP = max(0, nextHP-remaining)
	return nextCP, nextHP
}
