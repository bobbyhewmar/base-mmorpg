package app

import "sort"

type SkillCategory string

const (
	skillCategoryActive  SkillCategory = "active"
	skillCategoryPassive SkillCategory = "passive"
)

type CharacterKnownSkill struct {
	SkillID   string        `json:"skill_id"`
	Category  SkillCategory `json:"category"`
	UnlockLvl int           `json:"unlock_level"`
}

type CharacterHotbarSlot struct {
	SlotIndex      int    `json:"slot_index"`
	EntryType      string `json:"entry_type,omitempty"`
	SkillID        string `json:"skill_id,omitempty"`
	ItemInstanceID string `json:"item_instance_id,omitempty"`
	ActionID       string `json:"action_id,omitempty"`
}

type CharacterHotbarState struct {
	OpenBarCount int                   `json:"open_bar_count"`
	Slots        []CharacterHotbarSlot `json:"slots"`
}

type classSkillGrant struct {
	SkillID     string
	Category    SkillCategory
	UnlockLevel int
}

type classTemplate struct {
	BaseClass           string
	Title               string
	ArchetypeID         string
	BaseStats           CharacterDerivedStats
	CPGrowth            int
	DefaultHotbarSkills []string
	LearnedSkills       []classSkillGrant
}

var classTemplates = map[string]classTemplate{
	"Fighter": {
		BaseClass:   "Fighter",
		Title:       "Gatebound Initiate",
		ArchetypeID: "dusk_vanguard",
		BaseStats: CharacterDerivedStats{
			MaxCP:     80,
			MaxHP:     122,
			MaxMP:     58,
			Attack:    17,
			Defense:   9,
			MoveSpeed: 8.6,
		},
		CPGrowth:            12,
		DefaultHotbarSkills: []string{"crescent_strike", "grave_bloom"},
		LearnedSkills: []classSkillGrant{
			{SkillID: "crescent_strike", Category: skillCategoryActive, UnlockLevel: 1},
			{SkillID: "iron_will", Category: skillCategoryPassive, UnlockLevel: 1},
			{SkillID: "grave_bloom", Category: skillCategoryActive, UnlockLevel: 2},
		},
	},
	"Mage": {
		BaseClass:   "Mage",
		Title:       "Ashen Scholar",
		ArchetypeID: "ashen_oracle",
		BaseStats: CharacterDerivedStats{
			MaxCP:     55,
			MaxHP:     92,
			MaxMP:     92,
			Attack:    13,
			Defense:   6,
			MoveSpeed: 8.2,
		},
		CPGrowth:            9,
		DefaultHotbarSkills: []string{"ember_shot", "astral_burst"},
		LearnedSkills: []classSkillGrant{
			{SkillID: "ember_shot", Category: skillCategoryActive, UnlockLevel: 1},
			{SkillID: "arcane_focus", Category: skillCategoryPassive, UnlockLevel: 1},
			{SkillID: "astral_burst", Category: skillCategoryActive, UnlockLevel: 2},
		},
	},
}

func classTemplateForBaseClass(baseClass string) classTemplate {
	template, exists := classTemplates[baseClass]
	if exists {
		return template
	}
	return classTemplates["Fighter"]
}

func learnedSkillsForCharacter(baseClass string, level int) []CharacterKnownSkill {
	template := classTemplateForBaseClass(baseClass)
	normalizedLevel := normalizedCharacterLevel(level)
	known := make([]CharacterKnownSkill, 0, len(template.LearnedSkills))
	for _, grant := range template.LearnedSkills {
		if normalizedLevel < grant.UnlockLevel {
			continue
		}
		known = append(known, CharacterKnownSkill{
			SkillID:   grant.SkillID,
			Category:  grant.Category,
			UnlockLvl: grant.UnlockLevel,
		})
	}
	sort.Slice(known, func(i, j int) bool {
		if known[i].Category == known[j].Category {
			if known[i].UnlockLvl == known[j].UnlockLvl {
				return known[i].SkillID < known[j].SkillID
			}
			return known[i].UnlockLvl < known[j].UnlockLvl
		}
		return known[i].Category < known[j].Category
	})
	return known
}

func knownSkillCategory(baseClass string, level int, skillID string) (SkillCategory, bool) {
	for _, skill := range learnedSkillsForCharacter(baseClass, level) {
		if skill.SkillID == skillID {
			return skill.Category, true
		}
	}
	return "", false
}

func defaultCharacterHotbarState(character *Character) CharacterHotbarState {
	template := classTemplateForBaseClass(character.BaseClass)
	slots := make([]CharacterHotbarSlot, 0, 36)
	for slotIndex := 0; slotIndex < 36; slotIndex++ {
		slot := CharacterHotbarSlot{SlotIndex: slotIndex}
		if slotIndex < len(template.DefaultHotbarSkills) {
			slot.EntryType = "skill"
			slot.SkillID = template.DefaultHotbarSkills[slotIndex]
		}
		slots = append(slots, slot)
	}
	return CharacterHotbarState{
		OpenBarCount: 1,
		Slots:        slots,
	}
}

func normalizeCharacterHotbarState(state CharacterHotbarState, character *Character) CharacterHotbarState {
	normalized := defaultCharacterHotbarState(character)
	if state.OpenBarCount >= 1 && state.OpenBarCount <= 3 {
		normalized.OpenBarCount = state.OpenBarCount
	}

	slotByIndex := map[int]CharacterHotbarSlot{}
	for _, slot := range state.Slots {
		if slot.SlotIndex < 0 || slot.SlotIndex >= 36 {
			continue
		}
		switch slot.EntryType {
		case "skill":
			slot.ItemInstanceID = ""
			slot.ActionID = ""
		case "item":
			slot.SkillID = ""
			slot.ActionID = ""
		case "action":
			slot.SkillID = ""
			slot.ItemInstanceID = ""
		default:
			slot.EntryType = ""
			slot.SkillID = ""
			slot.ItemInstanceID = ""
			slot.ActionID = ""
		}
		slotByIndex[slot.SlotIndex] = CharacterHotbarSlot{
			SlotIndex:      slot.SlotIndex,
			EntryType:      slot.EntryType,
			SkillID:        slot.SkillID,
			ItemInstanceID: slot.ItemInstanceID,
			ActionID:       slot.ActionID,
		}
	}
	for index := range normalized.Slots {
		if slot, exists := slotByIndex[index]; exists {
			normalized.Slots[index] = slot
		}
	}
	sort.Slice(normalized.Slots, func(i, j int) bool {
		return normalized.Slots[i].SlotIndex < normalized.Slots[j].SlotIndex
	})
	return normalized
}
