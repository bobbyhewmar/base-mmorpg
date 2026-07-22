package app

import (
	"encoding/json"
	"time"
)

type AccountState string

const (
	accountStateActive              AccountState = "active"
	accountStatePendingVerification AccountState = "pending_verification"
	accountStateLocked              AccountState = "locked"
)

type Account struct {
	ID          string
	Login       string
	Email       string
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

type GameplayEvent struct {
	ID                              int64
	IdempotencyKey                  string
	Type                            string
	Payload                         json.RawMessage
	TargetServerInstanceID          string
	TargetRegionID                  string
	TargetSessionID                 string
	TargetCharacterID               string
	ProjectionSourceCharacterID     string
	ProjectionSourceFencingToken    int64
	ProjectionVersion               int64
	ProjectionRecipientFencingToken int64
	ProjectionAction                string
	CreatedAt                       time.Time
	AvailableAt                     time.Time
	ClaimedAt                       time.Time
	ClaimOwnerID                    string
	ClaimDeadlineAt                 time.Time
	DeliveredAt                     time.Time
	DeadLetteredAt                  time.Time
	SupersededAt                    time.Time
	SupersededByEventID             int64
	RetryCount                      int
	LastError                       string
}

type GameplayEventFailure struct {
	RetryCount   int
	DeadLettered bool
}

type RegionProjectionSupersession struct {
	TargetServerInstanceID          string
	TargetCharacterID               string
	ProjectionSourceCharacterID     string
	ProjectionSourceFencingToken    int64
	ProjectionVersion               int64
	ProjectionRecipientFencingToken int64
	SupersedingEventID              int64
	SupersededAt                    time.Time
}

type GameplayEventReceipt struct {
	EventID              int64
	RecipientSessionID   string
	RecipientCharacterID string
	ServerInstanceID     string
	ClaimOwnerID         string
	ClaimDeadlineAt      time.Time
	DeliveredAt          time.Time
	ConsumedAt           time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type GameplayEventReceiptReservation struct {
	Receipt   GameplayEventReceipt
	Acquired  bool
	Duplicate bool
	Busy      bool
}

type Character struct {
	ID                 string    `json:"character_id"`
	Name               string    `json:"name"`
	Race               string    `json:"race"`
	BaseClass          string    `json:"base_class"`
	Sex                string    `json:"sex"`
	HairStyle          int       `json:"hair_style"`
	HairColor          string    `json:"hair_color"`
	SkinType           int       `json:"skin_type"`
	Level              int       `json:"level"`
	XP                 int       `json:"-"`
	CurrentCP          int       `json:"-"`
	CurrentHP          int       `json:"-"`
	CurrentMP          int       `json:"-"`
	PvPKills           int       `json:"-"`
	PKCount            int       `json:"-"`
	Karma              int       `json:"-"`
	PvPFlagUntil       time.Time `json:"-"`
	KarmaRecoveryDueAt time.Time `json:"-"`
	KarmaHighSince     time.Time `json:"-"`
	LastRegionID       string    `json:"last_region_id"`
	PositionX          float64   `json:"-"`
	PositionZ          float64   `json:"-"`
	IsEnterable        bool      `json:"is_enterable"`
	AccountID          string    `json:"-"`
}

type CharacterPvPCombatState struct {
	CharacterID        string
	CurrentCP          int
	CurrentHP          int
	CurrentMP          int
	PvPKills           int
	PKCount            int
	Karma              int
	PvPFlagUntil       time.Time
	KarmaRecoveryDueAt time.Time
	KarmaHighSince     time.Time
}

type PvPCombatMutation struct {
	EventID             string
	AttackerCharacterID string
	VictimCharacterID   string
	ActionType          string
	SkillID             string
	Damage              int
	MPCost              int
	CooldownID          string
	CooldownDuration    time.Duration
	SessionID           string
	CommandID           string
	CommandSeq          int
	OccurredAt          time.Time
}

type PvPCombatCommit struct {
	Attacker            CharacterPvPCombatState
	Victim              CharacterPvPCombatState
	Event               PvPCombatEvent
	KarmaRecoveryEvents []PvPKarmaRecoveryEvent
	CooldownID          string
	CooldownEndsAt      time.Time
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
	ID               string
	AccountID        string
	CharacterID      string
	AttachToken      string
	AttachExpiresAt  time.Time
	Status           SessionStatus
	ServerInstanceID string
	FencingToken     int64
	LeaseExpiresAt   time.Time
}

type SessionOwnership struct {
	CharacterID      string
	SessionID        string
	ServerInstanceID string
	FencingToken     int64
	RegionID         string
	PositionX        float64
	PositionZ        float64
	LeaseExpiresAt   time.Time
	AcquiredAt       time.Time
	RenewedAt        time.Time
}

type SessionOwnershipChange string

const (
	sessionOwnershipAcquired SessionOwnershipChange = "acquired"
	sessionOwnershipReplaced SessionOwnershipChange = "replaced"
)

type SessionOwnershipAcquisition struct {
	Session   *Session
	Ownership SessionOwnership
	Change    SessionOwnershipChange
	Previous  *SessionOwnership
}
