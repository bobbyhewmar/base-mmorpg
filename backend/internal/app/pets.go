package app

import (
	"sort"
	"time"
)

type PetKind string

const (
	petKindPet      PetKind = "pet"
	petKindMount    PetKind = "mount"
	petKindPetMount PetKind = "pet_mount"
)

type PetTemplate struct {
	ID                  string
	SourceMobTemplateID string
	BaseName            string
	Kind                PetKind
	TameRange           float64
	MountedMoveSpeed    float64
	VisualTemplateID    string
}

type CharacterPet struct {
	ID            string    `json:"pet_instance_id"`
	CharacterID   string    `json:"-"`
	PetTemplateID string    `json:"pet_template_id"`
	CustomName    string    `json:"custom_name,omitempty"`
	IsSummoned    bool      `json:"summoned"`
	IsMounted     bool      `json:"mounted"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CharacterPetSnapshot struct {
	PetInstanceID    string  `json:"pet_instance_id"`
	PetTemplateID    string  `json:"pet_template_id"`
	Name             string  `json:"name"`
	Kind             PetKind `json:"kind"`
	VisualTemplateID string  `json:"visual_template_id"`
	MountEligible    bool    `json:"mount_eligible"`
	Summoned         bool    `json:"summoned"`
	Mounted          bool    `json:"mounted"`
}

const (
	petEntityType        = "pet"
	defaultPetFollowX    = 1.8
	defaultPetFollowSide = 0.75
)

var petTemplates = map[string]PetTemplate{
	"mireling_strider": {
		ID:                  "mireling_strider",
		SourceMobTemplateID: "mireling",
		BaseName:            "Mireling Strider",
		Kind:                petKindPetMount,
		TameRange:           4.5,
		MountedMoveSpeed:    10.8,
		VisualTemplateID:    "mireling",
	},
}

func petTemplateByID(petTemplateID string) (PetTemplate, bool) {
	template, exists := petTemplates[petTemplateID]
	return template, exists
}

func tameablePetTemplateForMob(templateID string) (PetTemplate, bool) {
	for _, template := range petTemplates {
		if template.SourceMobTemplateID == templateID {
			return template, true
		}
	}
	return PetTemplate{}, false
}

func isMountEligible(kind PetKind) bool {
	return kind == petKindMount || kind == petKindPetMount
}

func petDisplayName(pet CharacterPet) string {
	if pet.CustomName != "" {
		return pet.CustomName
	}
	template, exists := petTemplateByID(pet.PetTemplateID)
	if !exists {
		return pet.PetTemplateID
	}
	return template.BaseName
}

func petSnapshot(pet CharacterPet) CharacterPetSnapshot {
	template, _ := petTemplateByID(pet.PetTemplateID)
	return CharacterPetSnapshot{
		PetInstanceID:    pet.ID,
		PetTemplateID:    pet.PetTemplateID,
		Name:             petDisplayName(pet),
		Kind:             template.Kind,
		VisualTemplateID: template.VisualTemplateID,
		MountEligible:    isMountEligible(template.Kind),
		Summoned:         pet.IsSummoned,
		Mounted:          pet.IsMounted,
	}
}

func cloneCharacterPets(pets []CharacterPet) []CharacterPet {
	result := make([]CharacterPet, len(pets))
	copy(result, pets)
	return result
}

func normalizeCharacterPets(pets []CharacterPet) []CharacterPet {
	normalized := cloneCharacterPets(pets)
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].CreatedAt.Equal(normalized[j].CreatedAt) {
			return normalized[i].ID < normalized[j].ID
		}
		return normalized[i].CreatedAt.Before(normalized[j].CreatedAt)
	})
	return normalized
}

func petSnapshots(pets []CharacterPet) []CharacterPetSnapshot {
	normalized := normalizeCharacterPets(pets)
	result := make([]CharacterPetSnapshot, 0, len(normalized))
	for _, pet := range normalized {
		result = append(result, petSnapshot(pet))
	}
	return result
}

func applyMountedPetMoveSpeed(stats CharacterDerivedStats, pets []CharacterPet) CharacterDerivedStats {
	for _, pet := range pets {
		if !pet.IsMounted {
			continue
		}
		template, exists := petTemplateByID(pet.PetTemplateID)
		if !exists || template.MountedMoveSpeed <= 0 {
			continue
		}
		stats.MoveSpeed = template.MountedMoveSpeed
		break
	}
	return stats
}
