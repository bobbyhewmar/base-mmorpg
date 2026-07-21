package app

import "time"

type CharacterDerivedStats struct {
	MaxCP     int     `json:"max_cp"`
	MaxHP     int     `json:"max_hp"`
	MaxMP     int     `json:"max_mp"`
	Attack    int     `json:"attack"`
	Defense   int     `json:"defense"`
	MoveSpeed float64 `json:"move_speed"`
}

type CharacterSelfState struct {
	Level           int                               `json:"level"`
	XP              int                               `json:"xp"`
	CP              int                               `json:"cp"`
	HP              int                               `json:"hp"`
	MP              int                               `json:"mp"`
	Dead            bool                              `json:"dead"`
	PvPFlagged      bool                              `json:"pvp_flagged"`
	PvPFlagUntilMS  *int64                            `json:"pvp_flag_until_ms"`
	PvPKills        int                               `json:"pvp_kills"`
	PKCount         int                               `json:"pk_count"`
	Karma           int                               `json:"karma"`
	MovementMode    string                            `json:"movement_mode"`
	Cooldowns       map[string]int                    `json:"cooldowns,omitempty"`
	Stats           CharacterDerivedStats             `json:"stats"`
	KnownSkills     []CharacterKnownSkill             `json:"known_skills"`
	Hotbar          CharacterHotbarState              `json:"hotbar"`
	Pets            []CharacterPetSnapshot            `json:"pets,omitempty"`
	Quest           CharacterQuestSnapshot            `json:"quest"`
	Party           *CharacterPartySnapshot           `json:"party,omitempty"`
	PartyInvites    []CharacterPartyInviteSnapshot    `json:"party_invites,omitempty"`
	Clan            *CharacterClanSnapshot            `json:"clan,omitempty"`
	ClanInvites     []CharacterClanInviteSnapshot     `json:"clan_invites,omitempty"`
	Alliance        *CharacterAllianceSnapshot        `json:"alliance,omitempty"`
	AllianceInvites []CharacterAllianceInviteSnapshot `json:"alliance_invites,omitempty"`
	NPCInteraction  *CharacterNPCInteraction          `json:"npc_interaction,omitempty"`
}

func baseCharacterDerivedStats(character *Character) CharacterDerivedStats {
	level := normalizedCharacterLevel(1)
	template := classTemplateForBaseClass("Fighter")
	if character != nil {
		level = normalizedCharacterLevel(character.Level)
		template = classTemplateForBaseClass(character.BaseClass)
	}
	levelOffset := level - 1
	return CharacterDerivedStats{
		MaxCP:     template.BaseStats.MaxCP + template.CPGrowth*levelOffset,
		MaxHP:     template.BaseStats.MaxHP + 18*levelOffset,
		MaxMP:     template.BaseStats.MaxMP + 7*levelOffset,
		Attack:    template.BaseStats.Attack + 4*levelOffset,
		Defense:   template.BaseStats.Defense + 2*levelOffset,
		MoveSpeed: template.BaseStats.MoveSpeed,
	}
}

const maxSupportedCharacterLevel = 5

func normalizedCharacterLevel(level int) int {
	if level <= 0 {
		return 1
	}
	if level > maxSupportedCharacterLevel {
		return maxSupportedCharacterLevel
	}
	return level
}

func characterXPThreshold(level int) int {
	level = normalizedCharacterLevel(level)
	return 70 + (level-1)*50
}

func applyCharacterXP(level int, currentXP int, gainedXP int) (nextLevel int, nextXP int, levelsGained int) {
	nextLevel = normalizedCharacterLevel(level)
	nextXP = max(0, currentXP) + max(0, gainedXP)

	for nextLevel < maxSupportedCharacterLevel && nextXP >= characterXPThreshold(nextLevel) {
		nextXP -= characterXPThreshold(nextLevel)
		nextLevel++
		levelsGained++
	}

	if nextLevel >= maxSupportedCharacterLevel {
		nextLevel = maxSupportedCharacterLevel
		threshold := characterXPThreshold(nextLevel)
		if threshold > 0 && nextXP > threshold {
			nextXP = threshold
		}
	}

	return nextLevel, nextXP, levelsGained
}

func persistedCharacterState(character *Character) Character {
	if character == nil {
		character = &Character{}
	}

	state := *character
	state.Level = normalizedCharacterLevel(state.Level)
	if _, ok := normalizeCanonicalHairColor(state.HairColor); !ok {
		state.HairColor = defaultHairColor
	}
	if state.XP < 0 {
		state.XP = 0
	}
	state.PvPKills = max(0, state.PvPKills)
	state.PKCount = max(0, state.PKCount)
	state.Karma = max(0, state.Karma)
	if state.CurrentHP <= 0 {
		state.CurrentHP = baseCharacterDerivedStats(&state).MaxHP
	}
	if state.CurrentCP <= 0 {
		state.CurrentCP = baseCharacterDerivedStats(&state).MaxCP
	}
	if state.CurrentMP <= 0 {
		state.CurrentMP = baseCharacterDerivedStats(&state).MaxMP
	}
	return state
}

func resourcePoolsForCharacter(character *Character, items []CharacterItem) (Character, CharacterDerivedStats) {
	state := persistedCharacterState(character)
	stats := deriveCharacterStats(&state, items)
	if state.CurrentCP > stats.MaxCP {
		state.CurrentCP = stats.MaxCP
	}
	if state.CurrentHP > stats.MaxHP {
		state.CurrentHP = stats.MaxHP
	}
	if state.CurrentMP > stats.MaxMP {
		state.CurrentMP = stats.MaxMP
	}
	return state, stats
}

func itemTemplateStatBonuses(templateID string) CharacterDerivedStats {
	switch templateID {
	case "ironwood_spear":
		return CharacterDerivedStats{
			Attack: 10,
		}
	case "novice_oak_staff":
		return CharacterDerivedStats{
			MaxMP:  14,
			Attack: 8,
		}
	case "wardkeeper_mantle":
		return CharacterDerivedStats{
			MaxHP:   20,
			Defense: 6,
		}
	case "moonthread_robe":
		return CharacterDerivedStats{
			MaxMP:   22,
			Defense: 4,
		}
	case "watcher_gloves":
		return CharacterDerivedStats{
			Attack:  4,
			Defense: 1,
		}
	case "runesewn_gloves":
		return CharacterDerivedStats{
			MaxMP:  8,
			Attack: 3,
		}
	case "pathrunner_boots":
		return CharacterDerivedStats{
			Defense:   1,
			MoveSpeed: 0.15,
		}
	case "whisperstep_boots":
		return CharacterDerivedStats{
			Defense:   1,
			MoveSpeed: 0.13,
		}
	case "ruinbound_greaves":
		return CharacterDerivedStats{
			Defense:   2,
			MoveSpeed: 0.225,
		}
	default:
		return CharacterDerivedStats{}
	}
}

func itemInstanceStatBonuses(attrs *ItemInstanceAttributes) CharacterDerivedStats {
	if attrs == nil {
		return CharacterDerivedStats{}
	}
	return CharacterDerivedStats{
		MaxCP:     attrs.MaxCP,
		MaxHP:     attrs.MaxHP,
		MaxMP:     attrs.MaxMP,
		Attack:    attrs.Attack,
		Defense:   attrs.Defense,
		MoveSpeed: attrs.MoveSpeed,
	}
}

func passiveSkillStatBonuses(character *Character) CharacterDerivedStats {
	if character == nil {
		return CharacterDerivedStats{}
	}

	bonuses := CharacterDerivedStats{}
	for _, skill := range learnedSkillsForCharacter(character.BaseClass, character.Level) {
		if skill.Category != skillCategoryPassive {
			continue
		}
		switch skill.SkillID {
		case "iron_will":
			bonuses.Defense += 3
			bonuses.MaxHP += 8
		case "arcane_focus":
			bonuses.MaxMP += 12
			bonuses.Attack += 2
		case "keen_senses":
			bonuses.Attack += 2
			bonuses.MoveSpeed += 0.1
		case "grave_resolve":
			bonuses.MaxCP += 6
			bonuses.MaxMP += 8
			bonuses.Defense += 1
		}
	}
	return bonuses
}

func deriveCharacterStats(character *Character, items []CharacterItem) CharacterDerivedStats {
	stats := baseCharacterDerivedStats(character)
	passiveBonuses := passiveSkillStatBonuses(character)
	stats.MaxHP += passiveBonuses.MaxHP
	stats.MaxCP += passiveBonuses.MaxCP
	stats.MaxMP += passiveBonuses.MaxMP
	stats.Attack += passiveBonuses.Attack
	stats.Defense += passiveBonuses.Defense
	stats.MoveSpeed += passiveBonuses.MoveSpeed
	for _, item := range items {
		if item.ContainerKind != itemContainerEquipment {
			continue
		}
		bonus := itemTemplateStatBonuses(item.TemplateID)
		instanceBonus := itemInstanceStatBonuses(item.InstanceAttributes)
		stats.MaxHP += bonus.MaxHP
		stats.MaxCP += bonus.MaxCP
		stats.MaxMP += bonus.MaxMP
		stats.Attack += bonus.Attack
		stats.Defense += bonus.Defense
		stats.MoveSpeed += bonus.MoveSpeed
		stats.MaxHP += instanceBonus.MaxHP
		stats.MaxCP += instanceBonus.MaxCP
		stats.MaxMP += instanceBonus.MaxMP
		stats.Attack += instanceBonus.Attack
		stats.Defense += instanceBonus.Defense
		stats.MoveSpeed += instanceBonus.MoveSpeed
	}
	return stats
}

func selfStateFromItems(
	character *Character,
	items []CharacterItem,
	cooldowns map[string]int,
	hotbar CharacterHotbarState,
	pets []CharacterPet,
	quest CharacterQuestState,
	party *CharacterPartySnapshot,
	partyInvites []CharacterPartyInviteSnapshot,
	clan *CharacterClanSnapshot,
	clanInvites []CharacterClanInviteSnapshot,
	alliance *CharacterAllianceSnapshot,
	allianceInvites []CharacterAllianceInviteSnapshot,
) CharacterSelfState {
	state, stats := resourcePoolsForCharacter(character, items)
	stats = applyMountedPetMoveSpeed(stats, pets)
	var pvpFlagUntilMS *int64
	if state.PvPFlagUntil.After(time.Now()) {
		value := state.PvPFlagUntil.UnixMilli()
		pvpFlagUntilMS = &value
	}
	return CharacterSelfState{
		Level:           state.Level,
		XP:              state.XP,
		CP:              state.CurrentCP,
		HP:              state.CurrentHP,
		MP:              state.CurrentMP,
		Dead:            false,
		PvPFlagged:      pvpFlagUntilMS != nil,
		PvPFlagUntilMS:  pvpFlagUntilMS,
		PvPKills:        state.PvPKills,
		PKCount:         state.PKCount,
		Karma:           state.Karma,
		MovementMode:    string(movementModeRun),
		Cooldowns:       cooldowns,
		Stats:           stats,
		KnownSkills:     learnedSkillsForCharacter(state.BaseClass, state.Level),
		Hotbar:          normalizeCharacterHotbarState(hotbar, &state),
		Pets:            petSnapshots(pets),
		Quest:           questSnapshot(quest),
		Party:           cloneCharacterPartySnapshot(party),
		PartyInvites:    cloneCharacterPartyInviteSnapshots(partyInvites),
		Clan:            cloneCharacterClanSnapshot(clan),
		ClanInvites:     cloneCharacterClanInviteSnapshots(clanInvites),
		Alliance:        cloneCharacterAllianceSnapshot(alliance),
		AllianceInvites: cloneCharacterAllianceInviteSnapshots(allianceInvites),
	}
}

func initialCharacterCurrentHP(character *Character, stats CharacterDerivedStats) int {
	baseHP := baseCharacterDerivedStats(character).MaxHP
	if baseHP <= 0 {
		baseHP = stats.MaxHP
	}
	if stats.MaxHP > 0 && baseHP > stats.MaxHP {
		return stats.MaxHP
	}
	return baseHP
}

func initialCharacterCurrentMP(character *Character, stats CharacterDerivedStats) int {
	baseMP := baseCharacterDerivedStats(character).MaxMP
	if baseMP <= 0 {
		baseMP = stats.MaxMP
	}
	if stats.MaxMP > 0 && baseMP > stats.MaxMP {
		return stats.MaxMP
	}
	return baseMP
}

func mobTemplateDefense(templateID string) int {
	switch templateID {
	case "mireling":
		return 3
	case "gloom_wisp":
		return 4
	case "ruin_stalker":
		return 5
	case "stonebound_raider":
		return 7
	case "ashen_howler":
		return 10
	case "gravewarden":
		return 14
	default:
		return 0
	}
}

func mobTemplateAttack(templateID string) int {
	switch templateID {
	case "mireling":
		return 8
	case "gloom_wisp":
		return 10
	case "ruin_stalker":
		return 12
	case "stonebound_raider":
		return 15
	case "ashen_howler":
		return 21
	case "gravewarden":
		return 28
	default:
		return 0
	}
}
