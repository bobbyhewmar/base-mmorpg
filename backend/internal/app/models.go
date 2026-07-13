package app

import "time"

type AccountState string

const (
	accountStateActive              AccountState = "active"
	accountStatePendingVerification AccountState = "pending_verification"
	accountStateLocked              AccountState = "locked"
)

type Account struct {
	ID          string
	Login       string
	DisplayName string
	State       AccountState
}

type CredentialRecord struct {
	AccountID         string
	PasswordHash      string
	PasswordAlgorithm string
}

type AccountSession struct {
	Token     string
	AccountID string
	ExpiresAt time.Time
	RevokedAt *time.Time
}

type GameplayCommandRecordStatus string

const (
	gameplayCommandRecordStatusPending  GameplayCommandRecordStatus = "pending"
	gameplayCommandRecordStatusRejected GameplayCommandRecordStatus = "rejected"
	gameplayCommandRecordStatusApplied  GameplayCommandRecordStatus = "applied"
)

type GameplayCommandRecord struct {
	SessionID        string
	CommandSeq       int
	CommandID        string
	CommandType      string
	Status           GameplayCommandRecordStatus
	OutboundMessages []map[string]any
}

type Character struct {
	ID           string  `json:"character_id"`
	Name         string  `json:"name"`
	Race         string  `json:"race"`
	BaseClass    string  `json:"base_class"`
	Sex          string  `json:"sex"`
	HairStyle    int     `json:"hair_style"`
	HairColor    int     `json:"hair_color"`
	Face         int     `json:"face"`
	Level        int     `json:"level"`
	XP           int     `json:"-"`
	CurrentCP    int     `json:"-"`
	CurrentHP    int     `json:"-"`
	CurrentMP    int     `json:"-"`
	LastRegionID string  `json:"last_region_id"`
	PositionX    float64 `json:"-"`
	PositionZ    float64 `json:"-"`
	IsEnterable  bool    `json:"is_enterable"`
	AccountID    string  `json:"-"`
}

type CharacterSkillCooldown struct {
	CharacterID string    `json:"-"`
	SkillID     string    `json:"skill_id"`
	EndsAt      time.Time `json:"ends_at"`
}

type ItemContainer string

const (
	itemContainerInventory ItemContainer = "inventory"
	itemContainerEquipment ItemContainer = "equipment"
	itemContainerWarehouse ItemContainer = "warehouse"
)

type EquipSlot string

const (
	equipSlotWeapon EquipSlot = "weapon"
	equipSlotChest  EquipSlot = "chest"
	equipSlotGloves EquipSlot = "gloves"
	equipSlotBoots  EquipSlot = "boots"
)

type CharacterItem struct {
	ID                 string                  `json:"item_instance_id"`
	CharacterID        string                  `json:"-"`
	TemplateID         string                  `json:"template_id"`
	Quantity           int                     `json:"quantity"`
	ContainerKind      ItemContainer           `json:"container_kind"`
	EquipSlot          EquipSlot               `json:"equip_slot,omitempty"`
	InstanceAttributes *ItemInstanceAttributes `json:"instance_attributes,omitempty"`
}

type CharacterItemSnapshot struct {
	Inventory []CharacterItem `json:"inventory"`
	Equipment []CharacterItem `json:"equipment"`
	Warehouse []CharacterItem `json:"warehouse"`
}

type QuestStatus string

const (
	questStatusAvailable     QuestStatus = "available"
	questStatusActive        QuestStatus = "active"
	questStatusReadyToTurnIn QuestStatus = "ready_to_turn_in"
	questStatusCompleted     QuestStatus = "completed"
)

type CharacterQuestState struct {
	CharacterID string      `json:"-"`
	QuestID     string      `json:"quest_id"`
	Status      QuestStatus `json:"status"`
	Progress    int         `json:"progress"`
}

type CharacterQuestSnapshot struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Status      QuestStatus `json:"status"`
	Progress    int         `json:"progress"`
	Goal        int         `json:"goal"`
}

type CharacterNPCInteraction struct {
	NPCID string `json:"npc_id"`
	Kind  string `json:"kind"`
}

type ItemInstanceAttributes struct {
	MaxCP     int     `json:"max_cp,omitempty"`
	MaxHP     int     `json:"max_hp,omitempty"`
	MaxMP     int     `json:"max_mp,omitempty"`
	Attack    int     `json:"attack,omitempty"`
	Defense   int     `json:"defense,omitempty"`
	MoveSpeed float64 `json:"move_speed,omitempty"`
}

func normalizeItemInstanceAttributes(attrs *ItemInstanceAttributes) *ItemInstanceAttributes {
	if attrs == nil {
		return nil
	}
	if attrs.MaxCP == 0 && attrs.MaxHP == 0 && attrs.MaxMP == 0 && attrs.Attack == 0 && attrs.Defense == 0 && attrs.MoveSpeed == 0 {
		return nil
	}
	clone := *attrs
	return &clone
}

func cloneItemInstanceAttributes(attrs *ItemInstanceAttributes) *ItemInstanceAttributes {
	return normalizeItemInstanceAttributes(attrs)
}

type SessionStatus string

const (
	sessionStatusPendingAttach SessionStatus = "pending_attach"
	sessionStatusAttached      SessionStatus = "attached"
	sessionStatusExpired       SessionStatus = "expired"
	sessionStatusClosed        SessionStatus = "closed"
)

type Session struct {
	ID              string
	AccountID       string
	CharacterID     string
	AttachToken     string
	AttachExpiresAt time.Time
	Status          SessionStatus
}
