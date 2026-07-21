package app

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	errRecordNotFound        = errors.New("record not found")
	errRecordConflict        = errors.New("record conflict")
	errLootAlreadyCollected  = errors.New("loot already collected")
	errItemNotFound          = errors.New("item not found")
	errItemNotEquippable     = errors.New("item not equippable")
	errItemSlotMismatch      = errors.New("item slot mismatch")
	errItemNotEquipped       = errors.New("item not equipped")
	errItemNotStackable      = errors.New("item not stackable")
	errInvalidSplitQuantity  = errors.New("invalid split quantity")
	errItemMergeInvalid      = errors.New("invalid item merge")
	errItemNotConsumable     = errors.New("item not consumable")
	errPetNotFound           = errors.New("pet not found")
	errVendorOfferNotFound   = errors.New("vendor offer not found")
	errExchangeOfferNotFound = errors.New("exchange offer not found")
	errInsufficientFunds     = errors.New("insufficient funds")
	errInsufficientMaterials = errors.New("insufficient materials")
	errItemNotSellable       = errors.New("item not sellable")
	errWarehouseItemNotFound = errors.New("warehouse item not found")
	errPvPActorDead          = errors.New("pvp actor dead")
	errPvPTargetDead         = errors.New("pvp target dead")
	errPvPInsufficientMP     = errors.New("pvp insufficient mp")
	errPvPCooldownActive     = errors.New("pvp cooldown active")
	errSessionNotFound       = errors.New("session not found")
	errSessionNotAttachable  = errors.New("session not attachable")
	errSessionExpired        = errors.New("session expired")
	errInvalidAttachToken    = errors.New("invalid attach token")
	errOwnershipConflict     = errors.New("session ownership conflict")
	errOwnershipStale        = errors.New("session ownership stale")
	errOwnershipExpired      = errors.New("session ownership expired")
)

type AccountRepository interface {
	Create(ctx context.Context, account *Account) error
	GetByID(ctx context.Context, accountID string) (*Account, error)
	GetByLogin(ctx context.Context, login string) (*Account, error)
}

type CredentialRepository interface {
	Create(ctx context.Context, credential *CredentialRecord) error
	GetByAccountID(ctx context.Context, accountID string) (*CredentialRecord, error)
	Update(ctx context.Context, credential *CredentialRecord) error
}

type AccountSessionRepository interface {
	Create(ctx context.Context, session *AccountSession) error
	GetActiveByToken(ctx context.Context, token string, now time.Time) (*AccountSession, error)
	RevokeByToken(ctx context.Context, token string, now time.Time) error
}

type GameplayCommandRecordRepository interface {
	CreatePending(ctx context.Context, record *GameplayCommandRecord) error
	GetBySessionAndSeq(ctx context.Context, sessionID string, commandSeq int) (*GameplayCommandRecord, error)
	NextSeq(ctx context.Context, sessionID string) (int, error)
	UpdateOutcome(ctx context.Context, sessionID string, commandSeq int, status GameplayCommandRecordStatus, outboundMessages []map[string]any) error
}

type GameplayEventRepository interface {
	Create(ctx context.Context, event *GameplayEvent) (bool, error)
	GetByID(ctx context.Context, eventID int64) (*GameplayEvent, error)
	GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (*GameplayEvent, error)
	Claim(ctx context.Context, serverInstanceID string, claimOwnerID string, now time.Time, claimLease time.Duration, limit int) ([]GameplayEvent, error)
	MarkDelivered(ctx context.Context, eventID int64, claimOwnerID string, deliveredAt time.Time) (bool, error)
	MarkFailed(ctx context.Context, eventID int64, claimOwnerID string, failedAt time.Time, retryDelay time.Duration, maxRetries int, lastError string) (GameplayEventFailure, error)
	SupersedeRegionProjection(ctx context.Context, supersession RegionProjectionSupersession) (int, error)
	DeleteSupersededBefore(ctx context.Context, cutoff time.Time, limit int) (int, error)
	DeleteDeliveredBefore(ctx context.Context, cutoff time.Time, limit int) (int, error)
}

type GameplayEventReceiptRepository interface {
	Reserve(ctx context.Context, receipt GameplayEventReceipt, claimOwnerID string, now time.Time, claimLease time.Duration) (GameplayEventReceiptReservation, error)
	MarkConsumed(ctx context.Context, eventID int64, claimOwnerID string, consumedAt time.Time) (bool, error)
	Release(ctx context.Context, eventID int64, claimOwnerID string) error
	GetByEventID(ctx context.Context, eventID int64) (*GameplayEventReceipt, error)
}

type gameplayCommandEventWriter interface {
	FinalizeGameplayCommandWithEvent(ctx context.Context, sessionID string, commandSeq int, status GameplayCommandRecordStatus, outboundMessages []map[string]any, event *GameplayEvent) (bool, error)
	FinalizeGameplayCommandWithEvents(ctx context.Context, sessionID string, commandSeq int, status GameplayCommandRecordStatus, outboundMessages []map[string]any, events []*GameplayEvent) (int, error)
	FinalizeGameplayCommandWithChatAndEvents(ctx context.Context, sessionID string, commandSeq int, status GameplayCommandRecordStatus, outboundMessages []map[string]any, chatMessage ChatMessageRecord, events []*GameplayEvent) (int, error)
}

type CharacterRepository interface {
	ListByAccountID(ctx context.Context, accountID string) ([]Character, error)
	CountByAccountID(ctx context.Context, accountID string) (int, error)
	GetByID(ctx context.Context, characterID string) (*Character, error)
	GetByName(ctx context.Context, characterName string) (*Character, error)
	Create(ctx context.Context, character *Character) error
	UpdateWorldState(ctx context.Context, characterID string, regionID string, positionX float64, positionZ float64) error
	UpdateProgression(ctx context.Context, characterID string, level int, xp int, currentCP int, currentHP int, currentMP int) error
	UpdatePvPFlagUntil(ctx context.Context, characterID string, flagUntil time.Time) error
	ApplyPvPCombat(ctx context.Context, mutation PvPCombatMutation) (*PvPCombatCommit, error)
	ApplyKarmaRecovery(ctx context.Context, characterID string, now time.Time, trigger string) (*CharacterKarmaRecoveryCommit, error)
	ListHighKarma(ctx context.Context, query PvPHighKarmaQuery) ([]PvPHighKarmaRecord, error)
}

type CharacterCooldownRepository interface {
	ListByCharacterID(ctx context.Context, characterID string) ([]CharacterSkillCooldown, error)
	ReplaceByCharacterID(ctx context.Context, characterID string, cooldowns []CharacterSkillCooldown) error
}

type CharacterHotbarRepository interface {
	ListByCharacterID(ctx context.Context, characterID string) (CharacterHotbarState, error)
	ReplaceByCharacterID(ctx context.Context, characterID string, hotbar CharacterHotbarState) error
}

type CharacterPetRepository interface {
	ListByCharacterID(ctx context.Context, characterID string) ([]CharacterPet, error)
	Create(ctx context.Context, pet *CharacterPet) error
	UpdateState(ctx context.Context, characterID string, petID string, summoned bool, mounted bool) error
}

type CharacterQuestRepository interface {
	ListByCharacterID(ctx context.Context, characterID string) ([]CharacterQuestState, error)
	UpsertByCharacterID(ctx context.Context, quest CharacterQuestState) error
	CompleteQuestWithItemReward(ctx context.Context, quest CharacterQuestState, rewardTemplateID string, rewardQuantity int) ([]CharacterItem, error)
}

type PartyRepository interface {
	GetByID(ctx context.Context, partyID string) (*Party, error)
	GetByCharacterID(ctx context.Context, characterID string) (*Party, error)
	ListMembers(ctx context.Context, partyID string) ([]PartyMember, error)
	Create(ctx context.Context, party *Party, leader PartyMember) error
	AddMember(ctx context.Context, member *PartyMember) error
	RemoveMember(ctx context.Context, partyID string, characterID string) error
	UpdateLeader(ctx context.Context, partyID string, leaderCharacterID string) error
	Delete(ctx context.Context, partyID string) error
	ListPendingInvitesByInvitee(ctx context.Context, characterID string, now time.Time) ([]PartyInvite, error)
	ListPendingInvitesByInviter(ctx context.Context, characterID string, now time.Time) ([]PartyInvite, error)
	ListPendingInvitesByParty(ctx context.Context, partyID string, now time.Time) ([]PartyInvite, error)
	GetInviteByID(ctx context.Context, inviteID string) (*PartyInvite, error)
	CreateInvite(ctx context.Context, invite *PartyInvite) error
	DeleteInvite(ctx context.Context, inviteID string) error
	DeleteInvitesByParty(ctx context.Context, partyID string) error
	DeletePendingInviteForInvitee(ctx context.Context, partyID string, inviteeCharacterID string) error
	ExpireInvites(ctx context.Context, now time.Time) error
}

type ClanRepository interface {
	GetByID(ctx context.Context, clanID string) (*Clan, error)
	GetByName(ctx context.Context, name string) (*Clan, error)
	GetByCharacterID(ctx context.Context, characterID string) (*Clan, error)
	ListMembers(ctx context.Context, clanID string) ([]ClanMember, error)
	Create(ctx context.Context, clan *Clan, leader ClanMember) error
	AddMember(ctx context.Context, member *ClanMember) error
	RemoveMember(ctx context.Context, clanID string, characterID string) error
	Delete(ctx context.Context, clanID string) error
	ListPendingInvitesByInvitee(ctx context.Context, characterID string, now time.Time) ([]ClanInvite, error)
	ListPendingInvitesByInviter(ctx context.Context, characterID string, now time.Time) ([]ClanInvite, error)
	ListPendingInvitesByClan(ctx context.Context, clanID string, now time.Time) ([]ClanInvite, error)
	GetInviteByID(ctx context.Context, inviteID string) (*ClanInvite, error)
	CreateInvite(ctx context.Context, invite *ClanInvite) error
	AcceptInvite(ctx context.Context, inviteID string, member *ClanMember) error
	DeleteInvite(ctx context.Context, inviteID string) error
	DeleteInvitesByClan(ctx context.Context, clanID string) error
	DeletePendingInviteForInvitee(ctx context.Context, clanID string, inviteeCharacterID string) error
	ExpireInvites(ctx context.Context, now time.Time) error
}

type AllianceRepository interface {
	GetByID(ctx context.Context, allianceID string) (*Alliance, error)
	GetByName(ctx context.Context, name string) (*Alliance, error)
	GetByClanID(ctx context.Context, clanID string) (*Alliance, error)
	GetByCharacterID(ctx context.Context, characterID string) (*Alliance, error)
	ListMembers(ctx context.Context, allianceID string) ([]AllianceMember, error)
	Create(ctx context.Context, alliance *Alliance, founder AllianceMember) error
	AddMember(ctx context.Context, member *AllianceMember) error
	AcceptInvite(ctx context.Context, inviteID string, member *AllianceMember) error
	RemoveMember(ctx context.Context, allianceID string, clanID string) error
	Delete(ctx context.Context, allianceID string) error
	ListPendingInvitesByInvitee(ctx context.Context, characterID string, now time.Time) ([]AllianceInvite, error)
	ListPendingInvitesByInviter(ctx context.Context, characterID string, now time.Time) ([]AllianceInvite, error)
	ListPendingInvitesByAlliance(ctx context.Context, allianceID string, now time.Time) ([]AllianceInvite, error)
	ListPendingInvitesByTargetClan(ctx context.Context, clanID string, now time.Time) ([]AllianceInvite, error)
	ListExpiredInvitesByInvitee(ctx context.Context, characterID string, now time.Time) ([]AllianceInvite, error)
	GetInviteByID(ctx context.Context, inviteID string) (*AllianceInvite, error)
	CreateInvite(ctx context.Context, invite *AllianceInvite) error
	DeleteInvite(ctx context.Context, inviteID string) error
	DeleteInvitesByAlliance(ctx context.Context, allianceID string) error
	DeletePendingInviteForTargetClan(ctx context.Context, allianceID string, targetClanID string) error
	ExpireInvites(ctx context.Context, now time.Time) error
}

type ChatMessageRepository interface {
	Create(ctx context.Context, record ChatMessageRecord) error
	ListByCharacterID(ctx context.Context, characterID string) ([]ChatMessageRecord, error)
	ListByFilter(ctx context.Context, query ChatMessageQuery) ([]ChatMessageRecord, error)
}

type CharacterItemRepository interface {
	ListByCharacterID(ctx context.Context, characterID string) ([]CharacterItem, error)
	PickUpLoot(ctx context.Context, characterID string, lootID string, templateID string, quantity int) ([]CharacterItem, error)
	EquipItem(ctx context.Context, characterID string, itemID string) ([]CharacterItem, error)
	UnequipItem(ctx context.Context, characterID string, slot EquipSlot) ([]CharacterItem, error)
	SplitStack(ctx context.Context, characterID string, itemID string, quantity int) ([]CharacterItem, error)
	MergeStacks(ctx context.Context, characterID string, sourceItemID string, targetItemID string) ([]CharacterItem, error)
	UseConsumable(ctx context.Context, characterID string, itemID string) ([]CharacterItem, CharacterItem, error)
	BuyVendorOffer(ctx context.Context, characterID string, offer VendorOffer, quantity int) ([]CharacterItem, error)
	ExchangeOffer(ctx context.Context, characterID string, offer ExchangeOffer, quantity int) ([]CharacterItem, error)
	SellVendorItem(ctx context.Context, characterID string, itemID string, quantity int) ([]CharacterItem, error)
	DepositWarehouseItem(ctx context.Context, characterID string, itemID string, quantity int) ([]CharacterItem, error)
	WithdrawWarehouseItem(ctx context.Context, characterID string, itemID string, quantity int) ([]CharacterItem, error)
	TradeInventoryItem(ctx context.Context, sourceCharacterID string, targetCharacterID string, itemID string, quantity int, referenceID string) ([]CharacterItem, []CharacterItem, error)
}

type StorageTransferRecordRepository interface {
	ListByCharacterID(ctx context.Context, characterID string) ([]StorageTransferRecord, error)
	ListByFilter(ctx context.Context, query StorageTransferQuery) ([]StorageTransferRecord, error)
}

type ActionLogRepository interface {
	ListByCharacterID(ctx context.Context, characterID string) ([]ActionLogRecord, error)
	ListByFilter(ctx context.Context, query ActionLogQuery) ([]ActionLogRecord, error)
	Create(ctx context.Context, record ActionLogRecord) error
}

type PvPCombatEventRepository interface {
	ListByFilter(ctx context.Context, query PvPCombatEventQuery) ([]PvPCombatEvent, error)
	ListAccountCorrelations(ctx context.Context, query PvPAccountCorrelationQuery) ([]PvPAccountCorrelationRecord, error)
	ListKarmaRecoveryEvents(ctx context.Context, query PvPKarmaRecoveryEventQuery) ([]PvPKarmaRecoveryEvent, error)
}

type GameplaySessionRepository interface {
	Create(ctx context.Context, session *Session) error
	GetByID(ctx context.Context, sessionID string) (*Session, error)
	GetLatestPendingForCharacter(ctx context.Context, characterID string, now time.Time) (*Session, error)
	HasAttachedForCharacter(ctx context.Context, characterID string) (bool, error)
	ExpireStalePendingAttach(ctx context.Context, characterID string, now time.Time) error
	SanitizeStartupLifecycle(ctx context.Context, now time.Time) error
	UpdateStatus(ctx context.Context, sessionID string, status SessionStatus) error
	AcquireOwnership(ctx context.Context, sessionID string, attachToken string, serverInstanceID string, leaseDuration time.Duration, attachTokenTTL time.Duration) (*SessionOwnershipAcquisition, error)
	RenewOwnership(ctx context.Context, characterID string, sessionID string, serverInstanceID string, fencingToken int64, regionID string, position runtimePoint, leaseDuration time.Duration, attachTokenTTL time.Duration) (*SessionOwnership, error)
	RefreshOwnershipAnchor(ctx context.Context, characterID string, sessionID string, serverInstanceID string, fencingToken int64, regionID string, position runtimePoint) (*SessionOwnership, error)
	ReleaseOwnership(ctx context.Context, characterID string, sessionID string, serverInstanceID string, fencingToken int64) (bool, error)
	GetActiveOwnershipByCharacterID(ctx context.Context, characterID string) (*SessionOwnership, error)
	ListActiveOwnershipsByRegion(ctx context.Context, regionID string) ([]SessionOwnership, error)
	GetActiveSessionForCharacter(ctx context.Context, characterID string) (*Session, error)
}

type authRegistrationWriter interface {
	CreateAccountWithCredential(ctx context.Context, account *Account, credential *CredentialRecord) error
}

type authLookupReader interface {
	GetByLoginWithCredential(ctx context.Context, login string) (*Account, *CredentialRecord, error)
}

type characterBootstrapWriter interface {
	CreateCharacterWithItemSeed(ctx context.Context, character *Character, items []CharacterItem) error
}

type Store struct {
	Mode               string
	Accounts           AccountRepository
	Credentials        CredentialRepository
	Characters         CharacterRepository
	CharacterCooldowns CharacterCooldownRepository
	CharacterHotbars   CharacterHotbarRepository
	CharacterPets      CharacterPetRepository
	CharacterQuests    CharacterQuestRepository
	Parties            PartyRepository
	Clans              ClanRepository
	Alliances          AllianceRepository
	ChatMessages       ChatMessageRepository
	Items              CharacterItemRepository
	StorageTransfers   StorageTransferRecordRepository
	ActionLogs         ActionLogRepository
	PvPCombatEvents    PvPCombatEventRepository
	GameplaySessions   GameplaySessionRepository
	AccountSessions    AccountSessionRepository
	GameplayCommands   GameplayCommandRecordRepository
	GameplayEvents     GameplayEventRepository
	GameplayReceipts   GameplayEventReceiptRepository
	registration       authRegistrationWriter
	loginLookup        authLookupReader
	characterSeed      characterBootstrapWriter
	commandEventWriter gameplayCommandEventWriter
	socialTransactions socialCommandTransactionRunner
	closeFn            func() error
}

func (s *Store) RunSocialCommandTransaction(ctx context.Context, run func(context.Context) error) error {
	if s == nil || s.socialTransactions == nil {
		return errors.New("social command transaction runner is unavailable")
	}
	return s.socialTransactions.RunSocialCommandTransaction(ctx, run)
}

func NewStore(databaseURL string) (*Store, error) {
	if databaseURL == "" {
		return newMemoryStore(), nil
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	backend := &postgresStoreBackend{
		db:            db,
		collectedLoot: map[string]struct{}{},
	}
	return &Store{
		Mode:               "postgres",
		Accounts:           postgresAccountRepo{backend: backend},
		Credentials:        postgresCredentialRepo{backend: backend},
		Characters:         postgresCharacterRepo{backend: backend},
		CharacterCooldowns: postgresCharacterCooldownRepo{backend: backend},
		CharacterHotbars:   postgresCharacterHotbarRepo{backend: backend},
		CharacterPets:      postgresCharacterPetRepo{backend: backend},
		CharacterQuests:    postgresCharacterQuestRepo{backend: backend},
		Parties:            postgresPartyRepo{backend: backend},
		Clans:              postgresClanRepo{backend: backend},
		Alliances:          postgresAllianceRepo{backend: backend},
		ChatMessages:       postgresChatMessageRepo{backend: backend},
		Items:              postgresCharacterItemRepo{backend: backend},
		StorageTransfers:   postgresStorageTransferRecordRepo{backend: backend},
		ActionLogs:         postgresActionLogRepo{backend: backend},
		PvPCombatEvents:    postgresPvPCombatEventRepo{backend: backend},
		GameplaySessions:   postgresGameplaySessionRepo{backend: backend},
		AccountSessions:    postgresAccountSessionRepo{backend: backend},
		GameplayCommands:   postgresGameplayCommandRecordRepo{backend: backend},
		GameplayEvents:     postgresGameplayEventRepo{backend: backend},
		GameplayReceipts:   postgresGameplayEventReceiptRepo{backend: backend},
		registration:       backend,
		loginLookup:        backend,
		characterSeed:      backend,
		commandEventWriter: backend,
		socialTransactions: backend,
		closeFn:            db.Close,
	}, nil
}

func (s *Store) FinalizeGameplayCommandWithEvent(ctx context.Context, sessionID string, commandSeq int, status GameplayCommandRecordStatus, outboundMessages []map[string]any, event *GameplayEvent) (bool, error) {
	if s == nil || s.commandEventWriter == nil {
		return false, errors.New("atomic gameplay command event writer is unavailable")
	}
	return s.commandEventWriter.FinalizeGameplayCommandWithEvent(ctx, sessionID, commandSeq, status, outboundMessages, event)
}

func (s *Store) FinalizeGameplayCommandWithEvents(ctx context.Context, sessionID string, commandSeq int, status GameplayCommandRecordStatus, outboundMessages []map[string]any, events []*GameplayEvent) (int, error) {
	if s == nil || s.commandEventWriter == nil {
		return 0, errors.New("atomic gameplay command event writer is unavailable")
	}
	return s.commandEventWriter.FinalizeGameplayCommandWithEvents(ctx, sessionID, commandSeq, status, outboundMessages, events)
}

func (s *Store) FinalizeGameplayCommandWithChatAndEvents(ctx context.Context, sessionID string, commandSeq int, status GameplayCommandRecordStatus, outboundMessages []map[string]any, chatMessage ChatMessageRecord, events []*GameplayEvent) (int, error) {
	if s == nil || s.commandEventWriter == nil {
		return 0, errors.New("atomic gameplay chat command writer is unavailable")
	}
	return s.commandEventWriter.FinalizeGameplayCommandWithChatAndEvents(ctx, sessionID, commandSeq, status, outboundMessages, chatMessage, events)
}

func (s *Store) Close() error {
	if s == nil || s.closeFn == nil {
		return nil
	}
	return s.closeFn()
}

func (s *Store) CreateAccountWithCredential(ctx context.Context, account *Account, credential *CredentialRecord) error {
	if s.registration != nil {
		return s.registration.CreateAccountWithCredential(ctx, account, credential)
	}

	if err := s.Accounts.Create(ctx, account); err != nil {
		return err
	}
	return s.Credentials.Create(ctx, credential)
}

func (s *Store) GetByLoginWithCredential(ctx context.Context, login string) (*Account, *CredentialRecord, error) {
	if s.loginLookup != nil {
		return s.loginLookup.GetByLoginWithCredential(ctx, login)
	}

	account, err := s.Accounts.GetByLogin(ctx, login)
	if err != nil {
		return nil, nil, err
	}
	credential, err := s.Credentials.GetByAccountID(ctx, account.ID)
	if err != nil {
		return nil, nil, err
	}
	return account, credential, nil
}

func (s *Store) CreateCharacterWithItemSeed(ctx context.Context, character *Character, items []CharacterItem) error {
	if s.characterSeed != nil {
		return s.characterSeed.CreateCharacterWithItemSeed(ctx, character, items)
	}

	if err := s.Characters.Create(ctx, character); err != nil {
		return err
	}
	return nil
}

func (s *Store) SanitizeGameplaySessionLifecycle(ctx context.Context, now time.Time) error {
	if s == nil || s.GameplaySessions == nil {
		return nil
	}
	return s.GameplaySessions.SanitizeStartupLifecycle(ctx, now)
}

func (s *Store) UpdateCredential(ctx context.Context, credential *CredentialRecord) error {
	if s == nil || s.Credentials == nil {
		return errRecordNotFound
	}
	return s.Credentials.Update(ctx, credential)
}
