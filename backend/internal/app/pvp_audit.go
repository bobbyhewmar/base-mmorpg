package app

import (
	"sort"
	"time"
)

const (
	pvpAttributionWindow  = 30 * time.Second
	pvpRepeatedKillWindow = 10 * time.Minute
)

type PvPCombatEvent struct {
	ID                    string    `json:"event_id"`
	AttackerCharacterID   string    `json:"attacker_character_id"`
	AttackerAccountID     string    `json:"attacker_account_id"`
	VictimCharacterID     string    `json:"victim_character_id"`
	VictimAccountID       string    `json:"victim_account_id"`
	ActionType            string    `json:"action_type"`
	SkillID               string    `json:"skill_id,omitempty"`
	Damage                int       `json:"damage"`
	CPDamage              int       `json:"cp_damage"`
	HPDamage              int       `json:"hp_damage"`
	Result                string    `json:"result"`
	KillerCharacterID     string    `json:"killer_character_id,omitempty"`
	AssistCharacterIDs    []string  `json:"assist_character_ids"`
	Suspicious            bool      `json:"suspicious"`
	RepeatedKillCount     int       `json:"repeated_kill_count"`
	AttackerFlaggedBefore bool      `json:"attacker_flagged_before"`
	AttackerFlaggedAfter  bool      `json:"attacker_flagged_after"`
	VictimFlaggedBefore   bool      `json:"victim_flagged_before"`
	VictimFlaggedAfter    bool      `json:"victim_flagged_after"`
	PvPKillsBefore        int       `json:"pvp_kills_before"`
	PvPKillsAfter         int       `json:"pvp_kills_after"`
	PKCountBefore         int       `json:"pk_count_before"`
	PKCountAfter          int       `json:"pk_count_after"`
	KarmaBefore           int       `json:"karma_before"`
	KarmaAfter            int       `json:"karma_after"`
	KarmaDelta            int       `json:"karma_delta"`
	SessionID             string    `json:"session_id"`
	CommandID             string    `json:"command_id"`
	CommandSeq            int       `json:"command_seq"`
	CreatedAt             time.Time `json:"created_at"`
}

type PvPCombatEventQuery struct {
	AttackerCharacterID string
	VictimCharacterID   string
	KillerCharacterID   string
	InvolvedCharacterID string
	ActionType          string
	Result              string
	Suspicious          *bool
	OccurredAfter       *time.Time
	OccurredBefore      *time.Time
	Limit               int
	Offset              int
}

func resolvePvPCombatMutation(attacker Character, victim Character, mutation PvPCombatMutation, priorVictimEvents []PvPCombatEvent, priorRepeatedKills int) (*PvPCombatCommit, error) {
	if attacker.ID == "" || victim.ID == "" {
		return nil, errRecordNotFound
	}
	if attacker.ID == victim.ID {
		return nil, errRecordConflict
	}
	if attacker.CurrentHP <= 0 {
		return nil, errPvPActorDead
	}
	if victim.CurrentHP <= 0 {
		return nil, errPvPTargetDead
	}
	if attacker.CurrentMP < max(0, mutation.MPCost) {
		return nil, errPvPInsufficientMP
	}

	now := mutation.OccurredAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	attackerFlaggedBefore := attacker.PvPFlagUntil.After(now)
	victimFlaggedBefore := victim.PvPFlagUntil.After(now)
	victimExposedForKill := victimFlaggedBefore || victim.Karma > 0
	nextCP, nextHP := applyPlayerDamage(victim.CurrentCP, victim.CurrentHP, mutation.Damage)
	cpDamage := max(0, victim.CurrentCP-nextCP)
	hpDamage := max(0, victim.CurrentHP-nextHP)

	attackerState := characterPvPCombatStateFromCharacter(attacker)
	attackerState.CurrentMP = max(0, attacker.CurrentMP-max(0, mutation.MPCost))
	attackerState.PvPFlagUntil = now.Add(pvpFlagDuration).UTC()
	if attacker.PvPFlagUntil.After(attackerState.PvPFlagUntil) {
		attackerState.PvPFlagUntil = attacker.PvPFlagUntil.UTC()
	}
	victimState := characterPvPCombatStateFromCharacter(victim)
	victimState.CurrentCP = nextCP
	victimState.CurrentHP = nextHP
	victimState.PvPFlagUntil = activePvPFlagUntil(victim.PvPFlagUntil, now)

	result := "hit"
	killerCharacterID := ""
	assistCharacterIDs := []string{}
	repeatedKillCount := 0
	suspicious := false
	if nextHP == 0 {
		victimState.PvPFlagUntil = time.Time{}
		killerCharacterID = attacker.ID
		assistCharacterIDs = relevantPvPAssistCharacterIDs(priorVictimEvents, attacker.ID)
		repeatedKillCount = max(0, priorRepeatedKills) + 1
		suspicious = repeatedKillCount >= 2
		if victimExposedForKill {
			result = "pvp_kill"
			attackerState.PvPKills++
		} else {
			result = "pk_kill"
			attackerState.PKCount++
			attackerState.Karma += pkKarmaGain
		}
	}

	eventID := mutation.EventID
	if eventID == "" {
		eventID = randomID("pvp_event")
	}
	event := PvPCombatEvent{
		ID:                    eventID,
		AttackerCharacterID:   attacker.ID,
		AttackerAccountID:     attacker.AccountID,
		VictimCharacterID:     victim.ID,
		VictimAccountID:       victim.AccountID,
		ActionType:            mutation.ActionType,
		SkillID:               mutation.SkillID,
		Damage:                cpDamage + hpDamage,
		CPDamage:              cpDamage,
		HPDamage:              hpDamage,
		Result:                result,
		KillerCharacterID:     killerCharacterID,
		AssistCharacterIDs:    assistCharacterIDs,
		Suspicious:            suspicious,
		RepeatedKillCount:     repeatedKillCount,
		AttackerFlaggedBefore: attackerFlaggedBefore,
		AttackerFlaggedAfter:  true,
		VictimFlaggedBefore:   victimFlaggedBefore,
		VictimFlaggedAfter:    !victimState.PvPFlagUntil.IsZero(),
		PvPKillsBefore:        attacker.PvPKills,
		PvPKillsAfter:         attackerState.PvPKills,
		PKCountBefore:         attacker.PKCount,
		PKCountAfter:          attackerState.PKCount,
		KarmaBefore:           attacker.Karma,
		KarmaAfter:            attackerState.Karma,
		KarmaDelta:            attackerState.Karma - attacker.Karma,
		SessionID:             mutation.SessionID,
		CommandID:             mutation.CommandID,
		CommandSeq:            mutation.CommandSeq,
		CreatedAt:             now,
	}
	return &PvPCombatCommit{
		Attacker:       attackerState,
		Victim:         victimState,
		Event:          event,
		CooldownID:     mutation.CooldownID,
		CooldownEndsAt: now.Add(max(0, mutation.CooldownDuration)).UTC(),
	}, nil
}

func characterPvPCombatStateFromCharacter(character Character) CharacterPvPCombatState {
	return CharacterPvPCombatState{
		CharacterID:  character.ID,
		CurrentCP:    max(0, character.CurrentCP),
		CurrentHP:    max(0, character.CurrentHP),
		CurrentMP:    max(0, character.CurrentMP),
		PvPKills:     max(0, character.PvPKills),
		PKCount:      max(0, character.PKCount),
		Karma:        max(0, character.Karma),
		PvPFlagUntil: character.PvPFlagUntil.UTC(),
	}
}

func relevantPvPAssistCharacterIDs(events []PvPCombatEvent, killerCharacterID string) []string {
	sorted := append([]PvPCombatEvent(nil), events...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].CreatedAt.Equal(sorted[j].CreatedAt) {
			return sorted[i].ID > sorted[j].ID
		}
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})
	seen := map[string]struct{}{}
	assists := make([]string, 0)
	for _, event := range sorted {
		if event.Result == "pvp_kill" || event.Result == "pk_kill" {
			break
		}
		if event.Result != "hit" || event.Damage <= 0 || event.AttackerCharacterID == "" || event.AttackerCharacterID == killerCharacterID {
			continue
		}
		if _, exists := seen[event.AttackerCharacterID]; exists {
			continue
		}
		seen[event.AttackerCharacterID] = struct{}{}
		assists = append(assists, event.AttackerCharacterID)
	}
	return assists
}
