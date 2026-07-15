package app

import "time"

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
	InvolvedCharacterID string
	ActionType          string
	Result              string
	OccurredAfter       *time.Time
	OccurredBefore      *time.Time
	Limit               int
	Offset              int
}
