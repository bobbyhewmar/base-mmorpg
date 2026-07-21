package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type pvpTestHarness struct {
	server        *Server
	store         *Store
	actorSession  *Session
	targetSession *Session
	actor         *attachedRuntime
	target        *attachedRuntime
}

func newPvPTestHarnessWithCharacters(t *testing.T, actorCharacter *Character, targetCharacter *Character) *pvpTestHarness {
	t.Helper()
	store := newMemoryStore()
	if actorCharacter == nil {
		actorCharacter = &Character{
			ID:           "char_pvp_actor",
			AccountID:    "acct_pvp_actor",
			Name:         "PvpActor",
			Race:         "Human",
			BaseClass:    "Fighter",
			Sex:          "Male",
			HairColor:    defaultHairColor,
			Level:        1,
			LastRegionID: startingRegionID,
			PositionX:    0,
			PositionZ:    0,
			IsEnterable:  true,
		}
	}
	if targetCharacter == nil {
		targetCharacter = &Character{
			ID:           "char_pvp_target",
			AccountID:    "acct_pvp_target",
			Name:         "PvpTarget",
			Race:         "Elf",
			BaseClass:    "Mage",
			Sex:          "Female",
			HairColor:    defaultHairColor,
			Level:        1,
			LastRegionID: startingRegionID,
			PositionX:    1,
			PositionZ:    0,
			IsEnterable:  true,
		}
	}
	if err := store.Characters.Create(context.Background(), actorCharacter); err != nil {
		t.Fatalf("create actor: %v", err)
	}
	if err := store.Characters.Create(context.Background(), targetCharacter); err != nil {
		t.Fatalf("create target: %v", err)
	}
	actor := newCleanAttachedRuntime("sess_pvp_actor", actorCharacter)
	target := newCleanAttachedRuntime("sess_pvp_target", targetCharacter)
	actor.seedKnownEntity(target.playerPresenceEntity())
	target.seedKnownEntity(actor.playerPresenceEntity())
	server := NewServer(":0", "", store)
	server.registerAttachedSession(actor.sessionID, actor, func(map[string]any) bool { return true })
	server.registerAttachedSession(target.sessionID, target, func(map[string]any) bool { return true })
	return &pvpTestHarness{
		server:        server,
		store:         store,
		actorSession:  &Session{ID: actor.sessionID, AccountID: actorCharacter.AccountID, CharacterID: actor.characterID, Status: sessionStatusAttached},
		targetSession: &Session{ID: target.sessionID, AccountID: targetCharacter.AccountID, CharacterID: target.characterID, Status: sessionStatusAttached},
		actor:         actor,
		target:        target,
	}
}

func newPvPTestHarness(t *testing.T, targetX float64) *pvpTestHarness {
	t.Helper()
	return newPvPTestHarnessWithCharacters(t, nil, &Character{
		ID:           "char_pvp_target",
		AccountID:    "acct_pvp_target",
		Name:         "PvpTarget",
		Race:         "Elf",
		BaseClass:    "Mage",
		Sex:          "Female",
		HairColor:    defaultHairColor,
		Level:        1,
		LastRegionID: startingRegionID,
		PositionX:    targetX,
		PositionZ:    0,
		IsEnterable:  true,
	})
}

func pvpCommand(commandID string, commandSeq int, commandType string, payload map[string]any) commandEnvelope {
	encoded, _ := json.Marshal(payload)
	return commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       commandID,
		CommandSeq:      commandSeq,
		Type:            commandType,
		Payload:         encoded,
	}
}

func pvpRejectReason(messages []map[string]any) string {
	for _, message := range messages {
		if message["kind"] == "reject" {
			reason, _ := message["reason_code"].(string)
			return reason
		}
	}
	return ""
}

func TestPvPSelectionDoesNotDamageAndBasicAttackIsBackendOwned(t *testing.T) {
	h := newPvPTestHarness(t, 1)
	initialCP := h.target.currentCP
	selectMessages, _ := h.server.processGameplayCommandWithDedup(context.Background(), h.actorSession, h.actor, pvpCommand("cmd_select_player", 1, "select_target", map[string]any{
		"target_id": h.target.characterID,
	}))
	if pvpRejectReason(selectMessages) != "" {
		t.Fatalf("select rejected: %#v", selectMessages)
	}
	if h.target.currentCP != initialCP {
		t.Fatalf("select_target changed target CP: got %d want %d", h.target.currentCP, initialCP)
	}

	attackMessages, _ := h.server.processGameplayCommandWithDedup(context.Background(), h.actorSession, h.actor, pvpCommand("cmd_attack_player", 2, "basic_attack", map[string]any{
		"target_id": h.target.characterID,
	}))
	if pvpRejectReason(attackMessages) != "" {
		t.Fatalf("basic attack rejected: %#v", attackMessages)
	}
	if h.target.currentCP >= initialCP {
		t.Fatalf("basic attack did not reduce target CP: got %d initial %d", h.target.currentCP, initialCP)
	}
	if !h.actor.pvpFlaggedAt(time.Now()) {
		t.Fatal("successful hostile attack did not start PvP flag")
	}
	loadedActor, err := h.store.Characters.GetByID(context.Background(), h.actor.characterID)
	if err != nil {
		t.Fatal(err)
	}
	loadedTarget, err := h.store.Characters.GetByID(context.Background(), h.target.characterID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedTarget.CurrentCP != h.target.currentCP || loadedActor.PKCount != 0 || loadedActor.Karma != 0 || !loadedActor.PvPFlagUntil.After(time.Now()) {
		t.Fatalf("unexpected persisted combat state: actor=%+v target=%+v", loadedActor, loadedTarget)
	}
	events, err := h.store.PvPCombatEvents.ListByFilter(context.Background(), PvPCombatEventQuery{AttackerCharacterID: h.actor.characterID})
	if err != nil || len(events) != 1 {
		t.Fatalf("expected one PvP audit event: events=%+v err=%v", events, err)
	}
	if events[0].ActionType != "basic_attack" || events[0].CPDamage <= 0 || events[0].HPDamage != 0 || events[0].Result != "hit" || !events[0].AttackerFlaggedAfter {
		t.Fatalf("unexpected PvP audit event: %+v", events[0])
	}
}

func TestPvPSingleTargetSkillConsumesAuthoritativeMPAndDamagesPlayer(t *testing.T) {
	h := newPvPTestHarness(t, 4)
	initialMP := h.actor.currentMP
	initialCP := h.target.currentCP
	messages, _ := h.server.processGameplayCommandWithDedup(context.Background(), h.actorSession, h.actor, pvpCommand("cmd_skill_player", 1, "use_skill", map[string]any{
		"skill_id":  "crescent_strike",
		"target_id": h.target.characterID,
	}))
	if pvpRejectReason(messages) != "" {
		t.Fatalf("skill rejected: %#v", messages)
	}
	if h.actor.currentMP != initialMP-supportedSkills["crescent_strike"].MPCost {
		t.Fatalf("unexpected MP after skill: got %d", h.actor.currentMP)
	}
	if h.target.currentCP >= initialCP {
		t.Fatalf("skill did not damage player CP: got %d", h.target.currentCP)
	}
}

func TestPvPIdenticalReplayDoesNotApplyDamageTwiceAndConflictRejects(t *testing.T) {
	h := newPvPTestHarness(t, 1)
	command := pvpCommand("cmd_pvp_replay", 1, "basic_attack", map[string]any{"target_id": h.target.characterID})
	first, _ := h.server.processGameplayCommandWithDedup(context.Background(), h.actorSession, h.actor, command)
	if pvpRejectReason(first) != "" {
		t.Fatalf("first command rejected: %#v", first)
	}
	cpAfterFirst := h.target.currentCP
	replay, shouldFanOut := h.server.processGameplayCommandWithDedup(context.Background(), h.actorSession, h.actor, command)
	if pvpRejectReason(replay) != "" || shouldFanOut {
		t.Fatalf("unexpected identical replay outcome: messages=%#v fanout=%v", replay, shouldFanOut)
	}
	if h.target.currentCP != cpAfterFirst {
		t.Fatalf("identical replay reapplied damage: got CP %d want %d", h.target.currentCP, cpAfterFirst)
	}

	conflict := pvpCommand("cmd_pvp_conflict", 1, "basic_attack", map[string]any{"target_id": h.target.characterID})
	conflictingMessages, _ := h.server.processGameplayCommandWithDedup(context.Background(), h.actorSession, h.actor, conflict)
	if reason := pvpRejectReason(conflictingMessages); reason != "sequence.conflicting_replay" {
		t.Fatalf("conflicting replay reason = %q messages=%#v", reason, conflictingMessages)
	}
	if h.target.currentCP != cpAfterFirst {
		t.Fatalf("conflicting replay reapplied damage: got CP %d want %d", h.target.currentCP, cpAfterFirst)
	}
	events, err := h.store.PvPCombatEvents.ListByFilter(context.Background(), PvPCombatEventQuery{AttackerCharacterID: h.actor.characterID})
	if err != nil || len(events) != 1 {
		t.Fatalf("replay duplicated PvP audit event: events=%+v err=%v", events, err)
	}
}

func TestAuthoritativeClearTargetCannotMaskEarlierInvalidPvPTarget(t *testing.T) {
	h := newPvPTestHarness(t, 1)
	h.actor.targetID = h.target.characterID
	h.target.currentHP = 0
	attack := pvpCommand("cmd_dead_before_clear", 1, "basic_attack", map[string]any{"target_id": h.target.characterID})
	first, _ := h.server.processGameplayCommandWithDedup(context.Background(), h.actorSession, h.actor, attack)
	if reason := pvpRejectReason(first); reason != "combat.target_dead" {
		t.Fatalf("dead target reason = %q messages=%#v", reason, first)
	}
	clearMessages, _ := h.server.processGameplayCommandWithDedup(context.Background(), h.actorSession, h.actor, pvpCommand("cmd_clear_after_reject", 2, "clear_target", map[string]any{}))
	if reason := pvpRejectReason(clearMessages); reason != "" || h.actor.targetID != "" {
		t.Fatalf("authoritative clear_target failed: messages=%#v target=%q", clearMessages, h.actor.targetID)
	}
	replay, _ := h.server.processGameplayCommandWithDedup(context.Background(), h.actorSession, h.actor, attack)
	if reason := pvpRejectReason(replay); reason != "combat.target_dead" {
		t.Fatalf("clear_target masked durable rejected outcome: reason=%q messages=%#v", reason, replay)
	}
}

func TestPvPEligibilityRejectsInvalidTargetsWithoutDamage(t *testing.T) {
	tests := []struct {
		name       string
		prepare    func(*pvpTestHarness)
		command    func(*pvpTestHarness) commandEnvelope
		wantReason string
	}{
		{
			name: "self",
			command: func(h *pvpTestHarness) commandEnvelope {
				return pvpCommand("cmd_self", 1, "basic_attack", map[string]any{"target_id": h.actor.characterID})
			},
			wantReason: "pvp.self_target",
		},
		{
			name: "unknown",
			command: func(*pvpTestHarness) commandEnvelope {
				return pvpCommand("cmd_unknown", 1, "basic_attack", map[string]any{"target_id": "char_unknown"})
			},
			wantReason: "world.entity_not_known",
		},
		{
			name: "same party",
			prepare: func(h *pvpTestHarness) {
				now := time.Now()
				party := &Party{ID: "party_same", LeaderCharacterID: h.actor.characterID, CreatedAt: now, UpdatedAt: now}
				if err := h.store.Parties.Create(context.Background(), party, PartyMember{PartyID: party.ID, CharacterID: h.actor.characterID, JoinedAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
					t.Fatal(err)
				}
				if err := h.store.Parties.AddMember(context.Background(), &PartyMember{PartyID: party.ID, CharacterID: h.target.characterID, JoinedAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
					t.Fatal(err)
				}
			},
			command: func(h *pvpTestHarness) commandEnvelope {
				return pvpCommand("cmd_same_party", 1, "basic_attack", map[string]any{"target_id": h.target.characterID})
			},
			wantReason: "pvp.same_party",
		},
		{
			name: "same clan",
			prepare: func(h *pvpTestHarness) {
				now := time.Now()
				clan := &Clan{ID: "clan_same", Name: "SameClan", LeaderCharacterID: h.actor.characterID, CreatedAt: now, UpdatedAt: now}
				if err := h.store.Clans.Create(context.Background(), clan, ClanMember{ClanID: clan.ID, CharacterID: h.actor.characterID, JoinedAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
					t.Fatal(err)
				}
				if err := h.store.Clans.AddMember(context.Background(), &ClanMember{ClanID: clan.ID, CharacterID: h.target.characterID, JoinedAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
					t.Fatal(err)
				}
			},
			command: func(h *pvpTestHarness) commandEnvelope {
				return pvpCommand("cmd_same_clan", 1, "basic_attack", map[string]any{"target_id": h.target.characterID})
			},
			wantReason: "pvp.same_clan",
		},
		{
			name: "same alliance",
			prepare: func(h *pvpTestHarness) {
				now := time.Now()
				actorClan := &Clan{ID: "clan_actor", Name: "ActorClan", LeaderCharacterID: h.actor.characterID, CreatedAt: now, UpdatedAt: now}
				if err := h.store.Clans.Create(context.Background(), actorClan, ClanMember{ClanID: actorClan.ID, CharacterID: h.actor.characterID, JoinedAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
					t.Fatal(err)
				}
				targetClan := &Clan{ID: "clan_target", Name: "TargetClan", LeaderCharacterID: h.target.characterID, CreatedAt: now, UpdatedAt: now}
				if err := h.store.Clans.Create(context.Background(), targetClan, ClanMember{ClanID: targetClan.ID, CharacterID: h.target.characterID, JoinedAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
					t.Fatal(err)
				}
				alliance := &Alliance{ID: "alliance_same", Name: "SameAlliance", LeaderClanID: actorClan.ID, CreatedAt: now, UpdatedAt: now}
				founder := AllianceMember{AllianceID: alliance.ID, ClanID: actorClan.ID, JoinedAt: now, CreatedAt: now, UpdatedAt: now}
				if err := h.store.Alliances.Create(context.Background(), alliance, founder); err != nil {
					t.Fatal(err)
				}
				if err := h.store.Alliances.AddMember(context.Background(), &AllianceMember{AllianceID: alliance.ID, ClanID: targetClan.ID, JoinedAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
					t.Fatal(err)
				}
			},
			command: func(h *pvpTestHarness) commandEnvelope {
				return pvpCommand("cmd_same_alliance", 1, "basic_attack", map[string]any{"target_id": h.target.characterID})
			},
			wantReason: "pvp.same_alliance",
		},
		{
			name:    "dead target",
			prepare: func(h *pvpTestHarness) { h.target.currentHP = 0 },
			command: func(h *pvpTestHarness) commandEnvelope {
				return pvpCommand("cmd_dead_target", 1, "basic_attack", map[string]any{"target_id": h.target.characterID})
			},
			wantReason: "combat.target_dead",
		},
		{
			name: "out of region",
			prepare: func(h *pvpTestHarness) {
				h.target.regionID = "dawn_plaza"
			},
			command: func(h *pvpTestHarness) commandEnvelope {
				return pvpCommand("cmd_out_of_region", 1, "basic_attack", map[string]any{"target_id": h.target.characterID})
			},
			wantReason: "pvp.target_out_of_region",
		},
		{
			name: "restricted region",
			prepare: func(h *pvpTestHarness) {
				h.actor.regionID = "safe_haven"
				h.target.regionID = "safe_haven"
			},
			command: func(h *pvpTestHarness) commandEnvelope {
				return pvpCommand("cmd_restricted", 1, "basic_attack", map[string]any{"target_id": h.target.characterID})
			},
			wantReason: "pvp.region_restricted",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newPvPTestHarness(t, 1)
			if test.prepare != nil {
				test.prepare(h)
			}
			initialCP := h.target.currentCP
			messages, _ := h.server.processGameplayCommandWithDedup(context.Background(), h.actorSession, h.actor, test.command(h))
			if reason := pvpRejectReason(messages); reason != test.wantReason {
				t.Fatalf("reject reason = %q want %q messages=%#v", reason, test.wantReason, messages)
			}
			if h.target.currentCP != initialCP {
				t.Fatalf("rejected command changed target CP: got %d want %d", h.target.currentCP, initialCP)
			}
		})
	}
}

func TestPvPSafeZonePolicyBlocksBasicAttackAndSkill(t *testing.T) {
	for _, commandType := range []string{"basic_attack", "use_skill"} {
		t.Run(commandType, func(t *testing.T) {
			h := newPvPTestHarness(t, -7)
			h.actor.position = runtimePoint{X: -8, Z: 0}
			h.target.position = runtimePoint{X: -7, Z: 0}
			payload := map[string]any{"target_id": h.target.characterID}
			if commandType == "use_skill" {
				payload["skill_id"] = "crescent_strike"
			}
			initialCP := h.target.currentCP
			messages, _ := h.server.processGameplayCommandWithDedup(context.Background(), h.actorSession, h.actor, pvpCommand("cmd_safe_"+commandType, 1, commandType, payload))
			if reason := pvpRejectReason(messages); reason != "pvp.safe_zone" {
				t.Fatalf("reason = %q messages=%#v", reason, messages)
			}
			if h.target.currentCP != initialCP || h.actor.pvpFlaggedAt(time.Now()) {
				t.Fatalf("safe-zone reject changed combat state: target_cp=%d messages=%#v", h.target.currentCP, messages)
			}
		})
	}
}

func TestPvPRejectsOutOfRangeUnavailableAndUnsupportedAoe(t *testing.T) {
	t.Run("basic attack out of range", func(t *testing.T) {
		h := newPvPTestHarness(t, basicAttackRange+1)
		messages, _ := h.server.processGameplayCommandWithDedup(context.Background(), h.actorSession, h.actor, pvpCommand("cmd_far", 1, "basic_attack", map[string]any{"target_id": h.target.characterID}))
		if reason := pvpRejectReason(messages); reason != "combat.out_of_range" {
			t.Fatalf("reason = %q messages=%#v", reason, messages)
		}
	})

	t.Run("target disconnected after known-set seed", func(t *testing.T) {
		h := newPvPTestHarness(t, 1)
		h.server.attachedMu.Lock()
		delete(h.server.attached, h.target.sessionID)
		h.server.attachedMu.Unlock()
		messages, _ := h.server.processGameplayCommandWithDedup(context.Background(), h.actorSession, h.actor, pvpCommand("cmd_unavailable", 1, "basic_attack", map[string]any{"target_id": h.target.characterID}))
		if reason := pvpRejectReason(messages); reason != "pvp.target_unavailable" {
			t.Fatalf("reason = %q messages=%#v", reason, messages)
		}
	})

	t.Run("aoe skill is outside first PvP slice", func(t *testing.T) {
		h := newPvPTestHarness(t, 4)
		h.actor.characterLevel = 2
		messages, _ := h.server.processGameplayCommandWithDedup(context.Background(), h.actorSession, h.actor, pvpCommand("cmd_aoe", 1, "use_skill", map[string]any{
			"skill_id":  "grave_bloom",
			"target_id": h.target.characterID,
		}))
		if reason := pvpRejectReason(messages); reason != "pvp.skill_not_supported" {
			t.Fatalf("reason = %q messages=%#v", reason, messages)
		}
	})
}

func TestPvPKillClassificationPersistenceAndAuthoritativeRespawn(t *testing.T) {
	t.Run("unflagged target becomes PK", func(t *testing.T) {
		h := newPvPTestHarness(t, 1)
		h.target.currentCP = 0
		h.target.currentHP = 1
		if err := h.store.Characters.UpdateProgression(context.Background(), h.target.characterID, h.target.characterLevel, h.target.currentXP, 0, 1, h.target.currentMP); err != nil {
			t.Fatal(err)
		}
		h.target.targetID = h.actor.characterID
		h.target.queuedSkill = &queuedRuntimeSkill{}
		h.target.queuedBasicAttack = &queuedRuntimeBasicAttack{}
		h.target.autoBasicAttack = &queuedRuntimeBasicAttack{}
		h.target.queuedLootPickup = &queuedRuntimeLootPickup{}
		h.target.activeMovement = &runtimeMovementState{}
		h.target.cooldownEndsAt["crescent_strike"] = time.Now().Add(time.Minute)
		if err := h.store.CharacterCooldowns.ReplaceByCharacterID(context.Background(), h.target.characterID, h.target.characterCooldownState(time.Now())); err != nil {
			t.Fatal(err)
		}
		messages, _ := h.server.processGameplayCommandWithDedup(context.Background(), h.actorSession, h.actor, pvpCommand("cmd_pk_kill", 1, "basic_attack", map[string]any{"target_id": h.target.characterID}))
		if pvpRejectReason(messages) != "" {
			t.Fatalf("kill rejected: %#v", messages)
		}
		if h.actor.pkCount != 1 || h.actor.karma != pkKarmaGain || h.actor.pvpKills != 0 {
			t.Fatalf("unexpected PK state: kills=%d pk=%d karma=%d", h.actor.pvpKills, h.actor.pkCount, h.actor.karma)
		}
		if !h.target.isPlayerDead() {
			t.Fatal("target did not enter authoritative dead state")
		}
		if h.target.targetID != "" || h.target.queuedSkill != nil || h.target.queuedBasicAttack != nil || h.target.autoBasicAttack != nil || h.target.queuedLootPickup != nil || h.target.activeMovement != nil || len(h.target.cooldownEndsAt) != 0 {
			t.Fatalf("dead target retained offensive runtime state: %+v", h.target)
		}
		loaded, err := h.store.Characters.GetByID(context.Background(), h.actor.characterID)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.PKCount != 1 || loaded.Karma != pkKarmaGain {
			t.Fatalf("PK state not persisted: %+v", loaded)
		}
		events, err := h.store.PvPCombatEvents.ListByFilter(context.Background(), PvPCombatEventQuery{Result: "pk_kill"})
		if err != nil || len(events) != 1 || events[0].PKCountAfter != 1 || events[0].KarmaDelta != pkKarmaGain || events[0].HPDamage != 1 {
			t.Fatalf("unexpected PK audit classification: events=%+v err=%v", events, err)
		}

		tickMessages, _, respawned := h.target.collectTickMessages(time.Now().Add(playerRespawnDelay + time.Second))
		if !respawned || len(tickMessages) == 0 || h.target.isPlayerDead() {
			t.Fatalf("authoritative respawn did not resolve: respawned=%v messages=%#v", respawned, tickMessages)
		}
		if h.target.targetID != "" || h.target.activeMovement != nil || h.target.currentCP != h.target.derivedStats.MaxCP || h.target.currentHP != h.target.derivedStats.MaxHP || h.target.currentMP != h.target.derivedStats.MaxMP {
			t.Fatalf("respawn did not rehydrate a clean state: %+v", h.target)
		}
	})

	t.Run("flagged target becomes PvP kill", func(t *testing.T) {
		h := newPvPTestHarness(t, 1)
		h.target.currentCP = 0
		h.target.currentHP = 1
		h.target.pvpFlagUntil = time.Now().Add(time.Minute)
		if err := h.store.Characters.UpdateProgression(context.Background(), h.target.characterID, h.target.characterLevel, h.target.currentXP, 0, 1, h.target.currentMP); err != nil {
			t.Fatal(err)
		}
		if err := h.store.Characters.UpdatePvPFlagUntil(context.Background(), h.target.characterID, h.target.pvpFlagUntil); err != nil {
			t.Fatal(err)
		}
		messages, _ := h.server.processGameplayCommandWithDedup(context.Background(), h.actorSession, h.actor, pvpCommand("cmd_pvp_kill", 1, "basic_attack", map[string]any{"target_id": h.target.characterID}))
		if pvpRejectReason(messages) != "" {
			t.Fatalf("kill rejected: %#v", messages)
		}
		if h.actor.pvpKills != 1 || h.actor.pkCount != 0 || h.actor.karma != 0 {
			t.Fatalf("unexpected PvP kill state: kills=%d pk=%d karma=%d", h.actor.pvpKills, h.actor.pkCount, h.actor.karma)
		}
		events, err := h.store.PvPCombatEvents.ListByFilter(context.Background(), PvPCombatEventQuery{Result: "pvp_kill"})
		if err != nil || len(events) != 1 || events[0].PvPKillsAfter != 1 || events[0].KarmaDelta != 0 || !events[0].VictimFlaggedBefore || events[0].VictimFlaggedAfter {
			t.Fatalf("unexpected PvP kill audit classification: events=%+v err=%v", events, err)
		}
	})
}

func TestPvPFlagExpiresThroughAuthoritativeTick(t *testing.T) {
	h := newPvPTestHarness(t, 1)
	flagUntil := time.Now().Add(time.Second)
	h.actor.pvpFlagUntil = flagUntil
	if err := h.store.Characters.UpdatePvPFlagUntil(context.Background(), h.actor.characterID, flagUntil); err != nil {
		t.Fatal(err)
	}
	messages, presenceChanged, _, _ := h.actor.collectTickMessagesWithStore(time.Now().Add(2*time.Second), h.store)
	if !presenceChanged || len(messages) == 0 || h.actor.pvpFlaggedAt(time.Now().Add(2*time.Second)) {
		t.Fatalf("flag expiration was not projected authoritatively: changed=%v messages=%#v", presenceChanged, messages)
	}
	self, _ := messages[0]["self"].(map[string]any)
	if self["pvp_flag_reason"] != "pvp.flag_expired" || self["pvp_flagged"] != false || self["pvp_flag_until_ms"] != nil {
		t.Fatalf("missing authoritative flag-expiry transition: %#v", messages)
	}
	loaded, err := h.store.Characters.GetByID(context.Background(), h.actor.characterID)
	if err != nil || !loaded.PvPFlagUntil.IsZero() {
		t.Fatalf("expired flag was not cleared durably: character=%+v err=%v", loaded, err)
	}
}

func TestPvPHydrationRestoresActiveFlagAndDropsExpiredFlag(t *testing.T) {
	flagUntil := time.Now().Add(time.Minute).UTC()
	character := &Character{
		ID:           "char_pvp_hydration",
		Name:         "HydratedPvp",
		BaseClass:    "Fighter",
		LastRegionID: startingRegionID,
		PvPKills:     7,
		PKCount:      3,
		Karma:        300,
		PvPFlagUntil: flagUntil,
	}
	runtime := newCleanAttachedRuntime("sess_pvp_hydration", character)
	runtime.mu.Lock()
	snapshot := runtime.selfDelta(time.Now(), nil)
	runtime.mu.Unlock()
	if snapshot["pvp_kills"] != 7 || snapshot["pk_count"] != 3 || snapshot["karma"] != 300 {
		t.Fatalf("durable PvP counters missing from hydration: %#v", snapshot)
	}
	if snapshot["pvp_flagged"] != true || snapshot["pvp_flag_until_ms"] != flagUntil.UnixMilli() {
		t.Fatalf("active PvP flag missing after reconnect hydration: %#v", snapshot)
	}
	selfState := selfStateFromItems(character, nil, nil, CharacterHotbarState{}, nil, CharacterQuestState{}, nil, nil, nil, nil, nil, nil)
	if !selfState.PvPFlagged || selfState.PvPFlagUntilMS == nil || *selfState.PvPFlagUntilMS != flagUntil.UnixMilli() {
		t.Fatalf("world-enter snapshot omitted active PvP flag: %+v", selfState)
	}
	character.PvPFlagUntil = time.Now().Add(-time.Second)
	expiredRuntime := newCleanAttachedRuntime("sess_pvp_hydration_expired", character)
	expiredRuntime.mu.Lock()
	expiredSnapshot := expiredRuntime.selfDelta(time.Now(), nil)
	expiredRuntime.mu.Unlock()
	if expiredSnapshot["pvp_flagged"] != false || expiredSnapshot["pvp_flag_until_ms"] != nil {
		t.Fatalf("expired PvP flag was resurrected by hydration: %#v", expiredSnapshot)
	}
}

func TestPvPReconnectCleansAlreadyExpiredDurableDeadline(t *testing.T) {
	store := newMemoryStore()
	character := &Character{
		ID:           "char_pvp_expired_reconnect",
		AccountID:    "acct_pvp_expired_reconnect",
		Name:         "ExpiredReconnect",
		BaseClass:    "Fighter",
		LastRegionID: startingRegionID,
		PvPFlagUntil: time.Now().Add(-time.Minute),
	}
	if err := store.Characters.Create(context.Background(), character); err != nil {
		t.Fatal(err)
	}
	runtime := newCleanAttachedRuntime("sess_pvp_expired_reconnect", character)
	if !runtime.pvpFlagPersistenceDirty || runtime.pvpFlaggedAt(time.Now()) {
		t.Fatalf("expired reconnect state was not scheduled for cleanup: %+v", runtime)
	}
	runtime.collectTickMessagesWithStore(time.Now(), store)
	loaded, err := store.Characters.GetByID(context.Background(), character.ID)
	if err != nil || !loaded.PvPFlagUntil.IsZero() || runtime.pvpFlagPersistenceDirty {
		t.Fatalf("expired reconnect deadline was not cleaned: character=%+v runtime_dirty=%v err=%v", loaded, runtime.pvpFlagPersistenceDirty, err)
	}
}

func TestInternalPvPAuditEndpointRequiresTokenAndFiltersResults(t *testing.T) {
	h := newPvPTestHarness(t, 1)
	messages, _ := h.server.processGameplayCommandWithDedup(context.Background(), h.actorSession, h.actor, pvpCommand("cmd_pvp_audit", 1, "basic_attack", map[string]any{"target_id": h.target.characterID}))
	if pvpRejectReason(messages) != "" {
		t.Fatalf("attack rejected: %#v", messages)
	}
	h.server.config.InternalAuditEnabled = true
	h.server.config.InternalAuditToken = "pvp-audit-secret"
	httpServer := httptest.NewServer(h.server.mux)
	defer httpServer.Close()

	unauthorized, err := http.Get(httpServer.URL + "/internal/pvp/events")
	if err != nil {
		t.Fatal(err)
	}
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.StatusCode)
	}
	_ = unauthorized.Body.Close()

	request, err := http.NewRequest(http.MethodGet, httpServer.URL+"/internal/pvp/events?character_id="+h.actor.characterID+"&result=hit", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Internal-Audit-Token", "pvp-audit-secret")
	response, err := httpServer.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("authorized status = %d", response.StatusCode)
	}
	var payload struct {
		Events []PvPCombatEvent `json:"events"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Events) != 1 || payload.Events[0].CommandID != "cmd_pvp_audit" || payload.Events[0].SessionID != h.actorSession.ID {
		t.Fatalf("unexpected audit endpoint payload: %+v", payload.Events)
	}

	now := time.Now().UTC()
	for index, eventID := range []string{"audit_first_kill", "audit_repeated_kill"} {
		if err := h.store.Characters.UpdateProgression(context.Background(), h.target.characterID, 1, 0, 0, 1, h.target.currentMP); err != nil {
			t.Fatal(err)
		}
		mutation := pvpStoreMutation(eventID, &Character{ID: h.actor.characterID}, &Character{ID: h.target.characterID}, 10, now.Add(time.Duration(index)*time.Second))
		mutation.ActionType = "use_skill"
		mutation.SkillID = "crescent_strike"
		if _, err := h.store.Characters.ApplyPvPCombat(context.Background(), mutation); err != nil {
			t.Fatal(err)
		}
	}

	filteredURL := httpServer.URL + "/internal/pvp/events?killer_character_id=" + h.actor.characterID +
		"&victim_character_id=" + h.target.characterID +
		"&suspicious=true&action=use_skill&result=pk_kill&from=" + now.Add(-time.Minute).Format(time.RFC3339)
	filteredRequest, err := http.NewRequest(http.MethodGet, filteredURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	filteredRequest.Header.Set("X-Internal-Audit-Token", "pvp-audit-secret")
	filteredResponse, err := httpServer.Client().Do(filteredRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer filteredResponse.Body.Close()
	if filteredResponse.StatusCode != http.StatusOK {
		t.Fatalf("filtered audit status = %d", filteredResponse.StatusCode)
	}
	if err := json.NewDecoder(filteredResponse.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Events) != 1 || payload.Events[0].ID != "audit_repeated_kill" || !payload.Events[0].Suspicious || payload.Events[0].RepeatedKillCount != 2 {
		t.Fatalf("unexpected filtered repeated-kill payload: %+v", payload.Events)
	}

	invalidRequest, err := http.NewRequest(http.MethodGet, httpServer.URL+"/internal/pvp/events?suspicious=maybe", nil)
	if err != nil {
		t.Fatal(err)
	}
	invalidRequest.Header.Set("X-Internal-Audit-Token", "pvp-audit-secret")
	invalidResponse, err := httpServer.Client().Do(invalidRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer invalidResponse.Body.Close()
	if invalidResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid suspicious filter status = %d", invalidResponse.StatusCode)
	}
}

func TestPvPKarmaStateProjectsThroughAuthoritativeTick(t *testing.T) {
	h := newPvPTestHarness(t, 1)
	now := time.Now().UTC()
	h.actor.karma = 20
	h.actor.karmaRecoveryDueAt = now.Add(5 * time.Minute)
	h.actor.pvpStateDirty = true

	messages, presenceChanged, _, recoveryEvent := h.actor.collectTickMessagesWithStore(now, nil)
	if !presenceChanged || recoveryEvent != nil || len(messages) == 0 {
		t.Fatalf("authoritative tick did not project karma state: changed=%v event=%+v messages=%#v", presenceChanged, recoveryEvent, messages)
	}
	self, _ := messages[0]["self"].(map[string]any)
	if self["karma"] != 20 {
		t.Fatalf("self delta missing karma state: %#v", self)
	}
}

func TestInternalPvPRecoveryCorrelationAndHighKarmaEndpoints(t *testing.T) {
	now := time.Now().UTC()
	h := newPvPTestHarnessWithCharacters(t, &Character{
		ID:                 "char_pvp_actor",
		AccountID:          "acct_pvp_actor",
		Name:               "PvpActor",
		Race:               "Human",
		BaseClass:          "Fighter",
		Sex:                "Male",
		HairColor:          defaultHairColor,
		Level:              1,
		Karma:              240,
		KarmaRecoveryDueAt: now.Add(-time.Second),
		KarmaHighSince:     now.Add(-20 * time.Minute),
		LastRegionID:       startingRegionID,
		PositionX:          0,
		PositionZ:          0,
		IsEnterable:        true,
	}, nil)
	h.server.config.InternalAuditEnabled = true
	h.server.config.InternalAuditToken = "pvp-audit-secret"

	if _, err := h.store.Characters.ApplyKarmaRecovery(context.Background(), h.actor.characterID, now, "attach"); err != nil {
		t.Fatal(err)
	}
	storedActor, err := h.store.Characters.GetByID(context.Background(), h.actor.characterID)
	if err != nil {
		t.Fatal(err)
	}
	if storedActor.Karma != 220 {
		t.Fatalf("unexpected actor state after attach recovery: %+v", storedActor)
	}
	recoveryEvents, err := h.store.PvPCombatEvents.ListKarmaRecoveryEvents(context.Background(), PvPKarmaRecoveryEventQuery{
		CharacterID: h.actor.characterID,
		Limit:       10,
	})
	if err != nil || len(recoveryEvents) != 1 || recoveryEvents[0].Trigger != "attach" {
		t.Fatalf("unexpected direct recovery events: events=%+v err=%v", recoveryEvents, err)
	}
	for index, eventID := range []string{"corr_http_first", "corr_http_second"} {
		if err := h.store.Characters.UpdateProgression(context.Background(), h.target.characterID, 1, 0, 0, 1, h.target.currentMP); err != nil {
			t.Fatal(err)
		}
		if _, err := h.store.Characters.ApplyPvPCombat(context.Background(), pvpStoreMutation(eventID, &Character{ID: h.actor.characterID}, &Character{ID: h.target.characterID}, 10, now.Add(time.Duration(index)*time.Second))); err != nil {
			t.Fatal(err)
		}
	}

	recoveryRequest := httptest.NewRequest(http.MethodGet, "/internal/pvp/recovery?character_id="+h.actor.characterID+"&limit=10", nil)
	recoveryRequest.Header.Set("X-Internal-Audit-Token", "pvp-audit-secret")
	recoveryRecorder := httptest.NewRecorder()
	h.server.handleInternalPvPKarmaRecoveryEvents(recoveryRecorder, recoveryRequest)
	recoveryResponse := recoveryRecorder.Result()
	defer recoveryResponse.Body.Close()
	if recoveryResponse.StatusCode != http.StatusOK {
		t.Fatalf("recovery endpoint status = %d", recoveryResponse.StatusCode)
	}
	var recoveryPayload struct {
		Events []PvPKarmaRecoveryEvent `json:"events"`
	}
	if err := json.NewDecoder(recoveryResponse.Body).Decode(&recoveryPayload); err != nil {
		t.Fatal(err)
	}
	if len(recoveryPayload.Events) != 1 || recoveryPayload.Events[0].Trigger != "attach" {
		t.Fatalf("unexpected recovery endpoint payload: body=%s events=%+v", recoveryRecorder.Body.String(), recoveryPayload.Events)
	}

	correlationRequest := httptest.NewRequest(http.MethodGet, "/internal/pvp/correlations?account_id="+h.actorSession.AccountID+"&min_repeated_kill_count=2&limit=10", nil)
	correlationRequest.Header.Set("X-Internal-Audit-Token", "pvp-audit-secret")
	correlationRecorder := httptest.NewRecorder()
	h.server.handleInternalPvPCorrelations(correlationRecorder, correlationRequest)
	correlationResponse := correlationRecorder.Result()
	defer correlationResponse.Body.Close()
	if correlationResponse.StatusCode != http.StatusOK {
		t.Fatalf("correlation endpoint status = %d", correlationResponse.StatusCode)
	}
	var correlationPayload struct {
		Correlations []PvPAccountCorrelationRecord `json:"correlations"`
	}
	if err := json.NewDecoder(correlationResponse.Body).Decode(&correlationPayload); err != nil {
		t.Fatal(err)
	}
	if len(correlationPayload.Correlations) != 1 || correlationPayload.Correlations[0].MaxRepeatedKillCount != 2 {
		t.Fatalf("unexpected correlation payload: %+v", correlationPayload.Correlations)
	}

	highKarmaRequest := httptest.NewRequest(http.MethodGet, "/internal/pvp/high-karma?persistent=true&minimum_karma=100&limit=10", nil)
	highKarmaRequest.Header.Set("X-Internal-Audit-Token", "pvp-audit-secret")
	highKarmaRecorder := httptest.NewRecorder()
	h.server.handleInternalPvPHighKarma(highKarmaRecorder, highKarmaRequest)
	highKarmaResponse := highKarmaRecorder.Result()
	defer highKarmaResponse.Body.Close()
	if highKarmaResponse.StatusCode != http.StatusOK {
		t.Fatalf("high-karma endpoint status = %d", highKarmaResponse.StatusCode)
	}
	var highKarmaPayload struct {
		Characters []PvPHighKarmaRecord `json:"characters"`
	}
	if err := json.NewDecoder(highKarmaResponse.Body).Decode(&highKarmaPayload); err != nil {
		t.Fatal(err)
	}
	if len(highKarmaPayload.Characters) != 1 || highKarmaPayload.Characters[0].CharacterID != h.actor.characterID || !highKarmaPayload.Characters[0].PersistentHighKarma {
		t.Fatalf("unexpected high-karma payload: %+v", highKarmaPayload.Characters)
	}
}
