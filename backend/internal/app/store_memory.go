package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type memoryStoreBackend struct {
	mu                 sync.Mutex
	accounts           map[string]*Account
	accountByLogin     map[string]string
	credentials        map[string]*CredentialRecord
	accountSessions    map[string]*AccountSession
	commandRecords     map[string]*GameplayCommandRecord
	characters         map[string]*Character
	characterCooldowns map[string][]CharacterSkillCooldown
	characterHotbars   map[string]CharacterHotbarState
	characterPets      map[string][]CharacterPet
	characterQuests    map[string][]CharacterQuestState
	parties            map[string]*Party
	partyMembers       map[string][]PartyMember
	partyByCharacter   map[string]string
	partyInvites       map[string]*PartyInvite
	clans              map[string]*Clan
	clanMembers        map[string][]ClanMember
	clanByCharacter    map[string]string
	clanInvites        map[string]*ClanInvite
	clanByName         map[string]string
	chatMessages       map[string][]ChatMessageRecord
	characterItems     map[string][]CharacterItem
	storageTransfers   map[string][]StorageTransferRecord
	actionLogs         map[string][]ActionLogRecord
	pvpCombatEvents    []PvPCombatEvent
	claimedLoot        map[string]struct{}
	nameIndex          map[string]string
	sessions           map[string]*Session
}

type memoryAccountRepo struct{ backend *memoryStoreBackend }
type memoryCredentialRepo struct{ backend *memoryStoreBackend }
type memoryAccountSessionRepo struct{ backend *memoryStoreBackend }
type memoryGameplayCommandRecordRepo struct{ backend *memoryStoreBackend }
type memoryCharacterRepo struct{ backend *memoryStoreBackend }
type memoryCharacterCooldownRepo struct{ backend *memoryStoreBackend }
type memoryCharacterHotbarRepo struct{ backend *memoryStoreBackend }
type memoryCharacterPetRepo struct{ backend *memoryStoreBackend }
type memoryCharacterQuestRepo struct{ backend *memoryStoreBackend }
type memoryPartyRepo struct{ backend *memoryStoreBackend }
type memoryClanRepo struct{ backend *memoryStoreBackend }
type memoryChatMessageRepo struct{ backend *memoryStoreBackend }
type memoryCharacterItemRepo struct{ backend *memoryStoreBackend }
type memoryStorageTransferRecordRepo struct{ backend *memoryStoreBackend }
type memoryActionLogRepo struct{ backend *memoryStoreBackend }
type memoryPvPCombatEventRepo struct{ backend *memoryStoreBackend }
type memoryGameplaySessionRepo struct{ backend *memoryStoreBackend }

func newMemoryStore() *Store {
	backend := &memoryStoreBackend{
		accounts:           map[string]*Account{},
		accountByLogin:     map[string]string{},
		credentials:        map[string]*CredentialRecord{},
		accountSessions:    map[string]*AccountSession{},
		commandRecords:     map[string]*GameplayCommandRecord{},
		characters:         map[string]*Character{},
		characterCooldowns: map[string][]CharacterSkillCooldown{},
		characterHotbars:   map[string]CharacterHotbarState{},
		characterPets:      map[string][]CharacterPet{},
		characterQuests:    map[string][]CharacterQuestState{},
		parties:            map[string]*Party{},
		partyMembers:       map[string][]PartyMember{},
		partyByCharacter:   map[string]string{},
		partyInvites:       map[string]*PartyInvite{},
		clans:              map[string]*Clan{},
		clanMembers:        map[string][]ClanMember{},
		clanByCharacter:    map[string]string{},
		clanInvites:        map[string]*ClanInvite{},
		clanByName:         map[string]string{},
		chatMessages:       map[string][]ChatMessageRecord{},
		characterItems:     map[string][]CharacterItem{},
		storageTransfers:   map[string][]StorageTransferRecord{},
		actionLogs:         map[string][]ActionLogRecord{},
		claimedLoot:        map[string]struct{}{},
		nameIndex:          map[string]string{},
		sessions:           map[string]*Session{},
	}
	return &Store{
		Mode:               "memory",
		Accounts:           memoryAccountRepo{backend: backend},
		Credentials:        memoryCredentialRepo{backend: backend},
		AccountSessions:    memoryAccountSessionRepo{backend: backend},
		GameplayCommands:   memoryGameplayCommandRecordRepo{backend: backend},
		Characters:         memoryCharacterRepo{backend: backend},
		CharacterCooldowns: memoryCharacterCooldownRepo{backend: backend},
		CharacterHotbars:   memoryCharacterHotbarRepo{backend: backend},
		CharacterPets:      memoryCharacterPetRepo{backend: backend},
		CharacterQuests:    memoryCharacterQuestRepo{backend: backend},
		Parties:            memoryPartyRepo{backend: backend},
		Clans:              memoryClanRepo{backend: backend},
		ChatMessages:       memoryChatMessageRepo{backend: backend},
		Items:              memoryCharacterItemRepo{backend: backend},
		StorageTransfers:   memoryStorageTransferRecordRepo{backend: backend},
		ActionLogs:         memoryActionLogRepo{backend: backend},
		PvPCombatEvents:    memoryPvPCombatEventRepo{backend: backend},
		GameplaySessions:   memoryGameplaySessionRepo{backend: backend},
		registration:       backend,
		loginLookup:        backend,
		characterSeed:      backend,
	}
}

func (m *memoryStoreBackend) CreateAccountWithCredential(_ context.Context, account *Account, credential *CredentialRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	login := strings.TrimSpace(strings.ToLower(account.Login))
	if _, exists := m.accountByLogin[login]; exists {
		return errRecordConflict
	}

	accountCopy := *account
	accountCopy.Login = login
	credentialCopy := *credential
	m.accounts[account.ID] = &accountCopy
	m.accountByLogin[login] = account.ID
	m.credentials[account.ID] = &credentialCopy
	return nil
}

func (m *memoryStoreBackend) GetByLoginWithCredential(_ context.Context, login string) (*Account, *CredentialRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	accountID, exists := m.accountByLogin[strings.TrimSpace(strings.ToLower(login))]
	if !exists {
		return nil, nil, errRecordNotFound
	}
	accountCopy := *m.accounts[accountID]
	credentialCopy := *m.credentials[accountID]
	return &accountCopy, &credentialCopy, nil
}

func (repo memoryAccountRepo) Create(_ context.Context, account *Account) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	login := strings.TrimSpace(strings.ToLower(account.Login))
	if _, exists := repo.backend.accountByLogin[login]; exists {
		return errRecordConflict
	}
	accountCopy := *account
	accountCopy.Login = login
	repo.backend.accounts[account.ID] = &accountCopy
	repo.backend.accountByLogin[login] = account.ID
	return nil
}

func (repo memoryAccountRepo) GetByID(_ context.Context, accountID string) (*Account, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	account, exists := repo.backend.accounts[accountID]
	if !exists {
		return nil, errRecordNotFound
	}
	accountCopy := *account
	return &accountCopy, nil
}

func (repo memoryAccountRepo) GetByLogin(_ context.Context, login string) (*Account, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	accountID, exists := repo.backend.accountByLogin[strings.TrimSpace(strings.ToLower(login))]
	if !exists {
		return nil, errRecordNotFound
	}
	accountCopy := *repo.backend.accounts[accountID]
	return &accountCopy, nil
}

func (repo memoryCredentialRepo) Create(_ context.Context, credential *CredentialRecord) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if _, exists := repo.backend.credentials[credential.AccountID]; exists {
		return errRecordConflict
	}
	credentialCopy := *credential
	repo.backend.credentials[credential.AccountID] = &credentialCopy
	return nil
}

func (repo memoryCredentialRepo) GetByAccountID(_ context.Context, accountID string) (*CredentialRecord, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	credential, exists := repo.backend.credentials[accountID]
	if !exists {
		return nil, errRecordNotFound
	}
	credentialCopy := *credential
	return &credentialCopy, nil
}

func (repo memoryCredentialRepo) Update(_ context.Context, credential *CredentialRecord) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if _, exists := repo.backend.credentials[credential.AccountID]; !exists {
		return errRecordNotFound
	}
	credentialCopy := *credential
	repo.backend.credentials[credential.AccountID] = &credentialCopy
	return nil
}

func (repo memoryAccountSessionRepo) Create(_ context.Context, session *AccountSession) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	sessionCopy := *session
	repo.backend.accountSessions[session.Token] = &sessionCopy
	return nil
}

func (repo memoryAccountSessionRepo) GetActiveByToken(_ context.Context, token string, now time.Time) (*AccountSession, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	session, exists := repo.backend.accountSessions[token]
	if !exists {
		return nil, errRecordNotFound
	}
	if session.RevokedAt != nil || !session.ExpiresAt.After(now) {
		return nil, errRecordNotFound
	}
	sessionCopy := *session
	return &sessionCopy, nil
}

func (repo memoryAccountSessionRepo) RevokeByToken(_ context.Context, token string, now time.Time) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	session, exists := repo.backend.accountSessions[token]
	if !exists {
		return errRecordNotFound
	}
	revokedAt := now
	session.RevokedAt = &revokedAt
	return nil
}

func (repo memoryGameplayCommandRecordRepo) CreatePending(_ context.Context, record *GameplayCommandRecord) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	key := gameplayCommandRecordKey(record.SessionID, record.CommandSeq)
	if _, exists := repo.backend.commandRecords[key]; exists {
		return errRecordConflict
	}
	recordCopy := cloneGameplayCommandRecord(record)
	recordCopy.Status = gameplayCommandRecordStatusPending
	recordCopy.OutboundMessages = nil
	repo.backend.commandRecords[key] = recordCopy
	return nil
}

func (repo memoryGameplayCommandRecordRepo) GetBySessionAndSeq(_ context.Context, sessionID string, commandSeq int) (*GameplayCommandRecord, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	record, exists := repo.backend.commandRecords[gameplayCommandRecordKey(sessionID, commandSeq)]
	if !exists {
		return nil, errRecordNotFound
	}
	return cloneGameplayCommandRecord(record), nil
}

func (repo memoryGameplayCommandRecordRepo) UpdateOutcome(_ context.Context, sessionID string, commandSeq int, status GameplayCommandRecordStatus, outboundMessages []map[string]any) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	key := gameplayCommandRecordKey(sessionID, commandSeq)
	record, exists := repo.backend.commandRecords[key]
	if !exists {
		return errRecordNotFound
	}
	record.Status = status
	record.OutboundMessages = cloneOutboundMessages(outboundMessages)
	return nil
}

func (repo memoryCharacterRepo) ListByAccountID(_ context.Context, accountID string) ([]Character, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	characters := make([]Character, 0)
	for _, character := range repo.backend.characters {
		if character.AccountID == accountID {
			characters = append(characters, *character)
		}
	}
	sort.Slice(characters, func(i, j int) bool {
		if characters[i].Name == characters[j].Name {
			return characters[i].ID < characters[j].ID
		}
		return characters[i].Name < characters[j].Name
	})
	return characters, nil
}

func (repo memoryCharacterRepo) CountByAccountID(ctx context.Context, accountID string) (int, error) {
	characters, err := repo.ListByAccountID(ctx, accountID)
	if err != nil {
		return 0, err
	}
	return len(characters), nil
}

func (repo memoryCharacterRepo) GetByID(_ context.Context, characterID string) (*Character, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	character, exists := repo.backend.characters[characterID]
	if !exists {
		return nil, errRecordNotFound
	}
	characterCopy := *character
	return &characterCopy, nil
}

func (repo memoryCharacterRepo) Create(_ context.Context, character *Character) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	normalizedName := normalizeName(character.Name)
	if _, exists := repo.backend.nameIndex[normalizedName]; exists {
		return errRecordConflict
	}
	characterState := persistedCharacterState(character)
	characterCopy := characterState
	repo.backend.characters[character.ID] = &characterCopy
	repo.backend.nameIndex[normalizedName] = character.ID
	return nil
}

func (repo memoryCharacterRepo) UpdateWorldState(_ context.Context, characterID string, regionID string, positionX float64, positionZ float64) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	character, exists := repo.backend.characters[characterID]
	if !exists {
		return errRecordNotFound
	}
	character.LastRegionID = regionID
	character.PositionX = positionX
	character.PositionZ = positionZ
	return nil
}

func (repo memoryCharacterRepo) UpdateProgression(_ context.Context, characterID string, level int, xp int, currentCP int, currentHP int, currentMP int) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	character, exists := repo.backend.characters[characterID]
	if !exists {
		return errRecordNotFound
	}
	character.Level = normalizedCharacterLevel(level)
	character.XP = max(0, xp)
	character.CurrentCP = currentCP
	character.CurrentHP = currentHP
	character.CurrentMP = currentMP
	return nil
}

func (repo memoryCharacterRepo) UpdatePvPFlagUntil(_ context.Context, characterID string, flagUntil time.Time) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	character, exists := repo.backend.characters[characterID]
	if !exists {
		return errRecordNotFound
	}
	character.PvPFlagUntil = flagUntil.UTC()
	return nil
}

func (repo memoryCharacterRepo) ApplyPvPCombat(_ context.Context, mutation PvPCombatMutation) (*PvPCombatCommit, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	attackerCharacter, attackerExists := repo.backend.characters[mutation.AttackerCharacterID]
	targetCharacter, targetExists := repo.backend.characters[mutation.VictimCharacterID]
	if !attackerExists || !targetExists {
		return nil, errRecordNotFound
	}
	if mutation.OccurredAt.IsZero() {
		mutation.OccurredAt = time.Now().UTC()
	} else {
		mutation.OccurredAt = mutation.OccurredAt.UTC()
	}
	for _, event := range repo.backend.pvpCombatEvents {
		if mutation.SessionID != "" && mutation.CommandSeq > 0 && event.SessionID == mutation.SessionID && event.CommandSeq == mutation.CommandSeq {
			return nil, errRecordConflict
		}
	}
	for _, cooldown := range repo.backend.characterCooldowns[mutation.AttackerCharacterID] {
		if cooldown.SkillID == mutation.CooldownID && cooldown.EndsAt.After(mutation.OccurredAt) {
			return nil, errPvPCooldownActive
		}
	}
	priorVictimEvents := make([]PvPCombatEvent, 0)
	priorRepeatedKills := 0
	for _, event := range repo.backend.pvpCombatEvents {
		if event.VictimCharacterID == mutation.VictimCharacterID && !event.CreatedAt.Before(mutation.OccurredAt.Add(-pvpAttributionWindow)) && !event.CreatedAt.After(mutation.OccurredAt) {
			priorVictimEvents = append(priorVictimEvents, event)
		}
		if event.AttackerCharacterID == mutation.AttackerCharacterID && event.VictimCharacterID == mutation.VictimCharacterID &&
			(event.Result == "pvp_kill" || event.Result == "pk_kill") && !event.CreatedAt.Before(mutation.OccurredAt.Add(-pvpRepeatedKillWindow)) && !event.CreatedAt.After(mutation.OccurredAt) {
			priorRepeatedKills++
		}
	}
	commit, err := resolvePvPCombatMutation(*attackerCharacter, *targetCharacter, mutation, priorVictimEvents, priorRepeatedKills)
	if err != nil {
		return nil, err
	}
	applyCharacterPvPCombatState(attackerCharacter, commit.Attacker)
	applyCharacterPvPCombatState(targetCharacter, commit.Victim)
	if commit.CooldownID != "" && commit.CooldownEndsAt.After(mutation.OccurredAt) {
		cooldowns := repo.backend.characterCooldowns[mutation.AttackerCharacterID]
		replaced := false
		for index := range cooldowns {
			if cooldowns[index].SkillID != commit.CooldownID {
				continue
			}
			cooldowns[index].EndsAt = commit.CooldownEndsAt
			replaced = true
			break
		}
		if !replaced {
			cooldowns = append(cooldowns, CharacterSkillCooldown{CharacterID: mutation.AttackerCharacterID, SkillID: commit.CooldownID, EndsAt: commit.CooldownEndsAt})
		}
		repo.backend.characterCooldowns[mutation.AttackerCharacterID] = cooldowns
	}
	if targetCharacter.CurrentHP == 0 {
		delete(repo.backend.characterCooldowns, mutation.VictimCharacterID)
	}
	repo.backend.pvpCombatEvents = append(repo.backend.pvpCombatEvents, commit.Event)
	return commit, nil
}

func applyCharacterPvPCombatState(character *Character, state CharacterPvPCombatState) {
	character.CurrentCP = max(0, state.CurrentCP)
	character.CurrentHP = max(0, state.CurrentHP)
	character.CurrentMP = max(0, state.CurrentMP)
	character.PvPKills = max(0, state.PvPKills)
	character.PKCount = max(0, state.PKCount)
	character.Karma = max(0, state.Karma)
	character.PvPFlagUntil = state.PvPFlagUntil.UTC()
}

func (repo memoryCharacterCooldownRepo) ListByCharacterID(_ context.Context, characterID string) ([]CharacterSkillCooldown, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	cooldowns := repo.backend.characterCooldowns[characterID]
	result := make([]CharacterSkillCooldown, len(cooldowns))
	copy(result, cooldowns)
	sort.Slice(result, func(i, j int) bool {
		return result[i].SkillID < result[j].SkillID
	})
	return result, nil
}

func (repo memoryCharacterCooldownRepo) ReplaceByCharacterID(_ context.Context, characterID string, cooldowns []CharacterSkillCooldown) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if _, exists := repo.backend.characters[characterID]; !exists {
		return errRecordNotFound
	}

	replaced := make([]CharacterSkillCooldown, len(cooldowns))
	copy(replaced, cooldowns)
	sort.Slice(replaced, func(i, j int) bool {
		return replaced[i].SkillID < replaced[j].SkillID
	})
	repo.backend.characterCooldowns[characterID] = replaced
	return nil
}

func (repo memoryCharacterHotbarRepo) ListByCharacterID(_ context.Context, characterID string) (CharacterHotbarState, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	hotbar, exists := repo.backend.characterHotbars[characterID]
	if !exists {
		return CharacterHotbarState{}, errRecordNotFound
	}
	result := CharacterHotbarState{
		OpenBarCount: hotbar.OpenBarCount,
		Slots:        make([]CharacterHotbarSlot, len(hotbar.Slots)),
	}
	copy(result.Slots, hotbar.Slots)
	sort.Slice(result.Slots, func(i, j int) bool {
		return result.Slots[i].SlotIndex < result.Slots[j].SlotIndex
	})
	return result, nil
}

func (repo memoryCharacterHotbarRepo) ReplaceByCharacterID(_ context.Context, characterID string, hotbar CharacterHotbarState) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	character, exists := repo.backend.characters[characterID]
	if !exists {
		return errRecordNotFound
	}
	normalized := normalizeCharacterHotbarState(hotbar, character)
	replaced := CharacterHotbarState{
		OpenBarCount: normalized.OpenBarCount,
		Slots:        make([]CharacterHotbarSlot, len(normalized.Slots)),
	}
	copy(replaced.Slots, normalized.Slots)
	repo.backend.characterHotbars[characterID] = replaced
	return nil
}

func cloneCharacterQuestStates(quests []CharacterQuestState) []CharacterQuestState {
	result := make([]CharacterQuestState, len(quests))
	copy(result, quests)
	return result
}

func normalizeMemoryQuestState(characterID string, quest CharacterQuestState) CharacterQuestState {
	normalized := normalizeCharacterQuestState(quest)
	normalized.CharacterID = characterID
	return normalized
}

func (repo memoryCharacterQuestRepo) ListByCharacterID(_ context.Context, characterID string) ([]CharacterQuestState, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if _, exists := repo.backend.characters[characterID]; !exists {
		return nil, errRecordNotFound
	}
	quests := repo.backend.characterQuests[characterID]
	if len(quests) == 0 {
		return nil, errRecordNotFound
	}
	return cloneCharacterQuestStates(quests), nil
}

func (repo memoryCharacterQuestRepo) UpsertByCharacterID(_ context.Context, quest CharacterQuestState) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if _, exists := repo.backend.characters[quest.CharacterID]; !exists {
		return errRecordNotFound
	}
	normalized := normalizeMemoryQuestState(quest.CharacterID, quest)
	current := cloneCharacterQuestStates(repo.backend.characterQuests[quest.CharacterID])
	replaced := false
	for index := range current {
		if current[index].QuestID == normalized.QuestID {
			current[index] = normalized
			replaced = true
			break
		}
	}
	if !replaced {
		current = append(current, normalized)
	}
	repo.backend.characterQuests[quest.CharacterID] = current
	return nil
}

func grantMemoryInventoryItem(items []CharacterItem, characterID string, templateID string, quantity int) []CharacterItem {
	if quantity <= 0 {
		return items
	}
	if itemTemplateIsStackable(templateID) {
		for index := range items {
			if items[index].ContainerKind == itemContainerInventory && items[index].TemplateID == templateID && items[index].EquipSlot == "" {
				items[index].Quantity += quantity
				return items
			}
		}
		items = append(items, CharacterItem{
			ID:            randomID("item"),
			CharacterID:   characterID,
			TemplateID:    templateID,
			Quantity:      quantity,
			ContainerKind: itemContainerInventory,
		})
		return items
	}
	for count := 0; count < quantity; count++ {
		items = append(items, CharacterItem{
			ID:            randomID("item"),
			CharacterID:   characterID,
			TemplateID:    templateID,
			Quantity:      1,
			ContainerKind: itemContainerInventory,
		})
	}
	return items
}

func (repo memoryCharacterQuestRepo) CompleteQuestWithItemReward(
	_ context.Context,
	quest CharacterQuestState,
	rewardTemplateID string,
	rewardQuantity int,
) ([]CharacterItem, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if _, exists := repo.backend.characters[quest.CharacterID]; !exists {
		return nil, errRecordNotFound
	}
	normalized := normalizeMemoryQuestState(quest.CharacterID, quest)
	current := cloneCharacterQuestStates(repo.backend.characterQuests[quest.CharacterID])
	replaced := false
	for index := range current {
		if current[index].QuestID == normalized.QuestID {
			current[index] = normalized
			replaced = true
			break
		}
	}
	if !replaced {
		current = append(current, normalized)
	}
	repo.backend.characterQuests[quest.CharacterID] = current

	items := cloneCharacterItems(repo.backend.characterItems[quest.CharacterID])
	items = grantMemoryInventoryItem(items, quest.CharacterID, rewardTemplateID, rewardQuantity)
	repo.backend.characterItems[quest.CharacterID] = items

	result := cloneCharacterItems(items)
	sortCharacterItems(result)
	return result, nil
}

func (repo memoryCharacterItemRepo) ListByCharacterID(_ context.Context, characterID string) ([]CharacterItem, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	items := repo.backend.characterItems[characterID]
	result := cloneCharacterItems(items)
	sortCharacterItems(result)
	return result, nil
}

func (m *memoryStoreBackend) accountIDForCharacter(characterID string) string {
	if character, exists := m.characters[characterID]; exists && character != nil {
		return character.AccountID
	}
	return ""
}

func (m *memoryStoreBackend) applyStorageTransferAudit(ctx context.Context, record StorageTransferRecord) StorageTransferRecord {
	record.AccountID = m.accountIDForCharacter(record.CharacterID)
	metadata := commandAuditMetadataFromContext(ctx)
	if record.SessionID == "" {
		record.SessionID = metadata.SessionID
	}
	if record.CommandID == "" {
		record.CommandID = metadata.CommandID
	}
	if record.CommandSeq == 0 {
		record.CommandSeq = metadata.CommandSeq
	}
	return record
}

func (m *memoryStoreBackend) applyActionLogAudit(ctx context.Context, record ActionLogRecord) ActionLogRecord {
	record.AccountID = m.accountIDForCharacter(record.CharacterID)
	metadata := commandAuditMetadataFromContext(ctx)
	if record.SessionID == "" {
		record.SessionID = metadata.SessionID
	}
	if record.CommandID == "" {
		record.CommandID = metadata.CommandID
	}
	if record.CommandSeq == 0 {
		record.CommandSeq = metadata.CommandSeq
	}
	return record
}

func (m *memoryStoreBackend) recordStorageTransfer(ctx context.Context, characterID string, record StorageTransferRecord) {
	recordCopy := record
	if recordCopy.CreatedAt.IsZero() {
		recordCopy.CreatedAt = time.Now()
	}
	recordCopy = m.applyStorageTransferAudit(ctx, recordCopy)
	m.storageTransfers[characterID] = append(m.storageTransfers[characterID], recordCopy)
}

func (m *memoryStoreBackend) recordActionLog(ctx context.Context, characterID string, record ActionLogRecord) {
	recordCopy := record
	if recordCopy.CreatedAt.IsZero() {
		recordCopy.CreatedAt = time.Now()
	}
	recordCopy = m.applyActionLogAudit(ctx, recordCopy)
	m.actionLogs[characterID] = append(m.actionLogs[characterID], recordCopy)
}

func (repo memoryCharacterItemRepo) PickUpLoot(_ context.Context, characterID string, lootID string, templateID string, quantity int) ([]CharacterItem, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if quantity <= 0 {
		return nil, errRecordConflict
	}
	if _, exists := repo.backend.claimedLoot[lootID]; exists {
		return nil, errLootAlreadyCollected
	}
	repo.backend.claimedLoot[lootID] = struct{}{}

	items := repo.backend.characterItems[characterID]
	if itemTemplateIsStackable(templateID) {
		for index := range items {
			if items[index].ContainerKind == itemContainerInventory && items[index].TemplateID == templateID && items[index].EquipSlot == "" {
				items[index].Quantity += quantity
				repo.backend.characterItems[characterID] = items
				result := cloneCharacterItems(items)
				sortCharacterItems(result)
				return result, nil
			}
		}
	}

	for count := 0; count < quantity; count++ {
		itemQuantity := 1
		if itemTemplateIsStackable(templateID) {
			itemQuantity = quantity
			count = quantity
		}
		items = append(items, CharacterItem{
			ID:            randomID("item"),
			CharacterID:   characterID,
			TemplateID:    templateID,
			Quantity:      itemQuantity,
			ContainerKind: itemContainerInventory,
		})
	}
	repo.backend.characterItems[characterID] = items

	result := cloneCharacterItems(items)
	sortCharacterItems(result)
	return result, nil
}

func (repo memoryCharacterItemRepo) EquipItem(_ context.Context, characterID string, itemID string) ([]CharacterItem, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	items := repo.backend.characterItems[characterID]
	targetIndex := -1
	for index := range items {
		if items[index].ID == itemID {
			targetIndex = index
			break
		}
	}
	if targetIndex == -1 {
		return nil, errItemNotFound
	}
	if items[targetIndex].ContainerKind != itemContainerInventory {
		return nil, errItemNotFound
	}

	slot, equipable := itemTemplateEquipSlot(items[targetIndex].TemplateID)
	if !equipable {
		return nil, errItemNotEquippable
	}

	for index := range items {
		if index == targetIndex {
			continue
		}
		if items[index].ContainerKind == itemContainerEquipment && items[index].EquipSlot == slot {
			items[index].ContainerKind = itemContainerInventory
			items[index].EquipSlot = ""
			break
		}
	}

	items[targetIndex].ContainerKind = itemContainerEquipment
	items[targetIndex].EquipSlot = slot
	repo.backend.characterItems[characterID] = items

	result := cloneCharacterItems(items)
	sortCharacterItems(result)
	return result, nil
}

func (repo memoryCharacterItemRepo) UnequipItem(_ context.Context, characterID string, slot EquipSlot) ([]CharacterItem, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	items := repo.backend.characterItems[characterID]
	targetIndex := -1
	for index := range items {
		if items[index].ContainerKind == itemContainerEquipment && items[index].EquipSlot == slot {
			targetIndex = index
			break
		}
	}
	if targetIndex == -1 {
		return nil, errItemNotEquipped
	}

	items[targetIndex].ContainerKind = itemContainerInventory
	items[targetIndex].EquipSlot = ""
	repo.backend.characterItems[characterID] = items

	result := cloneCharacterItems(items)
	sortCharacterItems(result)
	return result, nil
}

func (repo memoryCharacterItemRepo) SplitStack(_ context.Context, characterID string, itemID string, quantity int) ([]CharacterItem, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	items := repo.backend.characterItems[characterID]
	targetIndex := -1
	for index := range items {
		if items[index].ID == itemID {
			targetIndex = index
			break
		}
	}
	if targetIndex == -1 || items[targetIndex].ContainerKind != itemContainerInventory {
		return nil, errItemNotFound
	}
	if !itemTemplateIsStackable(items[targetIndex].TemplateID) {
		return nil, errItemNotStackable
	}
	if quantity <= 0 || items[targetIndex].Quantity <= quantity {
		return nil, errInvalidSplitQuantity
	}

	items[targetIndex].Quantity -= quantity
	items = append(items, CharacterItem{
		ID:                 randomID("item"),
		CharacterID:        characterID,
		TemplateID:         items[targetIndex].TemplateID,
		Quantity:           quantity,
		ContainerKind:      itemContainerInventory,
		InstanceAttributes: cloneItemInstanceAttributes(items[targetIndex].InstanceAttributes),
	})
	repo.backend.characterItems[characterID] = items

	result := cloneCharacterItems(items)
	sortCharacterItems(result)
	return result, nil
}

func (repo memoryCharacterItemRepo) MergeStacks(_ context.Context, characterID string, sourceItemID string, targetItemID string) ([]CharacterItem, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if sourceItemID == targetItemID {
		return nil, errItemMergeInvalid
	}

	items := repo.backend.characterItems[characterID]
	sourceIndex := -1
	targetIndex := -1
	for index := range items {
		switch items[index].ID {
		case sourceItemID:
			sourceIndex = index
		case targetItemID:
			targetIndex = index
		}
	}
	if sourceIndex == -1 || targetIndex == -1 {
		return nil, errItemNotFound
	}
	source := items[sourceIndex]
	target := items[targetIndex]
	if source.ContainerKind != itemContainerInventory || target.ContainerKind != itemContainerInventory {
		return nil, errItemMergeInvalid
	}
	if source.TemplateID != target.TemplateID {
		return nil, errItemMergeInvalid
	}
	if !itemTemplateIsStackable(source.TemplateID) {
		return nil, errItemNotStackable
	}

	items[targetIndex].Quantity += source.Quantity
	items = append(items[:sourceIndex], items[sourceIndex+1:]...)
	repo.backend.characterItems[characterID] = items

	result := cloneCharacterItems(items)
	sortCharacterItems(result)
	return result, nil
}

func (repo memoryCharacterItemRepo) UseConsumable(_ context.Context, characterID string, itemID string) ([]CharacterItem, CharacterItem, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	items := repo.backend.characterItems[characterID]
	targetIndex := -1
	for index := range items {
		if items[index].ID == itemID {
			targetIndex = index
			break
		}
	}
	if targetIndex == -1 || items[targetIndex].ContainerKind != itemContainerInventory {
		return nil, CharacterItem{}, errItemNotFound
	}
	if !itemTemplateIsConsumable(items[targetIndex].TemplateID) {
		return nil, CharacterItem{}, errItemNotConsumable
	}

	consumed := items[targetIndex]
	if consumed.Quantity <= 1 {
		items = append(items[:targetIndex], items[targetIndex+1:]...)
	} else {
		items[targetIndex].Quantity--
	}
	repo.backend.characterItems[characterID] = items

	result := cloneCharacterItems(items)
	sortCharacterItems(result)
	return result, consumed, nil
}

func transferMemoryItemBetweenContainers(items []CharacterItem, sourceIndex int, sourceContainer ItemContainer, targetContainer ItemContainer, quantity int) ([]CharacterItem, error) {
	if sourceIndex < 0 || sourceIndex >= len(items) {
		return nil, errItemNotFound
	}
	source := items[sourceIndex]
	if source.ContainerKind != sourceContainer {
		if sourceContainer == itemContainerWarehouse {
			return nil, errWarehouseItemNotFound
		}
		return nil, errItemNotFound
	}
	if quantity <= 0 {
		return nil, errInvalidSplitQuantity
	}
	if !itemTemplateIsStackable(source.TemplateID) {
		if quantity != 1 || source.Quantity != 1 {
			return nil, errInvalidSplitQuantity
		}
		items[sourceIndex].ContainerKind = targetContainer
		items[sourceIndex].EquipSlot = ""
		return items, nil
	}
	if quantity > source.Quantity {
		return nil, errInvalidSplitQuantity
	}

	targetIndex := -1
	for index := range items {
		if index == sourceIndex {
			continue
		}
		if items[index].ContainerKind == targetContainer && items[index].TemplateID == source.TemplateID && items[index].EquipSlot == "" {
			targetIndex = index
			break
		}
	}

	if quantity == source.Quantity {
		if targetIndex >= 0 {
			items[targetIndex].Quantity += quantity
			items = append(items[:sourceIndex], items[sourceIndex+1:]...)
			return items, nil
		}
		items[sourceIndex].ContainerKind = targetContainer
		items[sourceIndex].EquipSlot = ""
		return items, nil
	}

	items[sourceIndex].Quantity -= quantity
	if targetIndex >= 0 {
		items[targetIndex].Quantity += quantity
		return items, nil
	}

	items = append(items, CharacterItem{
		ID:                 randomID("item"),
		CharacterID:        source.CharacterID,
		TemplateID:         source.TemplateID,
		Quantity:           quantity,
		ContainerKind:      targetContainer,
		InstanceAttributes: cloneItemInstanceAttributes(source.InstanceAttributes),
	})
	return items, nil
}

func transferMemoryInventoryItemBetweenCharacters(
	sourceItems []CharacterItem,
	sourceIndex int,
	targetItems []CharacterItem,
	targetCharacterID string,
	quantity int,
) ([]CharacterItem, []CharacterItem, CharacterItem, string, error) {
	if sourceIndex < 0 || sourceIndex >= len(sourceItems) {
		return nil, nil, CharacterItem{}, "", errItemNotFound
	}

	source := sourceItems[sourceIndex]
	if source.ContainerKind != itemContainerInventory {
		return nil, nil, CharacterItem{}, "", errItemNotFound
	}
	if quantity <= 0 {
		return nil, nil, CharacterItem{}, "", errInvalidSplitQuantity
	}

	if !itemTemplateIsStackable(source.TemplateID) {
		if quantity != 1 || source.Quantity != 1 {
			return nil, nil, CharacterItem{}, "", errInvalidSplitQuantity
		}

		movedItem := source
		movedItem.CharacterID = targetCharacterID
		movedItem.ContainerKind = itemContainerInventory
		movedItem.EquipSlot = ""
		sourceItems = append(sourceItems[:sourceIndex], sourceItems[sourceIndex+1:]...)
		targetItems = append(targetItems, movedItem)
		return sourceItems, targetItems, source, movedItem.ID, nil
	}

	if quantity > source.Quantity {
		return nil, nil, CharacterItem{}, "", errInvalidSplitQuantity
	}

	targetIndex := -1
	for index := range targetItems {
		if targetItems[index].ContainerKind == itemContainerInventory && targetItems[index].TemplateID == source.TemplateID && targetItems[index].EquipSlot == "" {
			targetIndex = index
			break
		}
	}

	if quantity == source.Quantity {
		if targetIndex >= 0 {
			targetItems[targetIndex].Quantity += quantity
			sourceItems = append(sourceItems[:sourceIndex], sourceItems[sourceIndex+1:]...)
			return sourceItems, targetItems, source, targetItems[targetIndex].ID, nil
		}

		movedItem := source
		movedItem.CharacterID = targetCharacterID
		movedItem.ContainerKind = itemContainerInventory
		movedItem.EquipSlot = ""
		sourceItems = append(sourceItems[:sourceIndex], sourceItems[sourceIndex+1:]...)
		targetItems = append(targetItems, movedItem)
		return sourceItems, targetItems, source, movedItem.ID, nil
	}

	sourceItems[sourceIndex].Quantity -= quantity
	if targetIndex >= 0 {
		targetItems[targetIndex].Quantity += quantity
		return sourceItems, targetItems, source, targetItems[targetIndex].ID, nil
	}

	movedItem := CharacterItem{
		ID:                 randomID("item"),
		CharacterID:        targetCharacterID,
		TemplateID:         source.TemplateID,
		Quantity:           quantity,
		ContainerKind:      itemContainerInventory,
		InstanceAttributes: cloneItemInstanceAttributes(source.InstanceAttributes),
	}
	targetItems = append(targetItems, movedItem)
	return sourceItems, targetItems, source, movedItem.ID, nil
}

func (repo memoryCharacterItemRepo) BuyVendorOffer(ctx context.Context, characterID string, offer VendorOffer, quantity int) ([]CharacterItem, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if quantity <= 0 || offer.ID == "" || offer.TemplateID == "" || offer.PriceCurrencyTemplateID == "" || offer.PriceAmount <= 0 || offer.Quantity <= 0 {
		return nil, errVendorOfferNotFound
	}

	items := repo.backend.characterItems[characterID]
	totalCost := offer.PriceAmount * quantity
	availableFunds := 0
	purchasedQuantityBefore := 0
	for _, item := range items {
		if item.ContainerKind == itemContainerInventory && item.TemplateID == offer.PriceCurrencyTemplateID {
			availableFunds += item.Quantity
		}
		if item.ContainerKind == itemContainerInventory && item.TemplateID == offer.TemplateID {
			purchasedQuantityBefore += item.Quantity
		}
	}
	if availableFunds < totalCost {
		return nil, errInsufficientFunds
	}

	remainingCost := totalCost
	filtered := make([]CharacterItem, 0, len(items))
	for _, item := range items {
		if remainingCost > 0 && item.ContainerKind == itemContainerInventory && item.TemplateID == offer.PriceCurrencyTemplateID {
			if item.Quantity <= remainingCost {
				remainingCost -= item.Quantity
				continue
			}
			item.Quantity -= remainingCost
			remainingCost = 0
		}
		filtered = append(filtered, item)
	}
	items = filtered

	totalGranted := offer.Quantity * quantity
	purchasedItemID := ""
	purchasedQuantityAfter := 0
	if itemTemplateIsStackable(offer.TemplateID) {
		stacked := false
		for index := range items {
			if items[index].ContainerKind == itemContainerInventory && items[index].TemplateID == offer.TemplateID {
				items[index].Quantity += totalGranted
				purchasedItemID = items[index].ID
				purchasedQuantityAfter = items[index].Quantity
				stacked = true
				break
			}
		}
		if !stacked {
			purchasedItemID = randomID("item")
			items = append(items, CharacterItem{
				ID:            purchasedItemID,
				CharacterID:   characterID,
				TemplateID:    offer.TemplateID,
				Quantity:      totalGranted,
				ContainerKind: itemContainerInventory,
			})
			purchasedQuantityAfter = totalGranted
		}
	} else {
		for count := 0; count < totalGranted; count++ {
			nextItemID := randomID("item")
			if purchasedItemID == "" {
				purchasedItemID = nextItemID
			}
			items = append(items, CharacterItem{
				ID:            nextItemID,
				CharacterID:   characterID,
				TemplateID:    offer.TemplateID,
				Quantity:      1,
				ContainerKind: itemContainerInventory,
			})
		}
		purchasedQuantityAfter = 1
	}

	repo.backend.characterItems[characterID] = items
	repo.backend.recordActionLog(ctx, characterID, ActionLogRecord{
		ID:                    randomID("action"),
		CharacterID:           characterID,
		ActionType:            "vendor_buy",
		ReferenceID:           offer.ID,
		CounterpartyEntity:    offer.NPCEntityID,
		ItemInstanceID:        purchasedItemID,
		TemplateID:            offer.TemplateID,
		Quantity:              totalGranted,
		ItemQuantityBefore:    purchasedQuantityBefore,
		ItemQuantityAfter:     purchasedQuantityAfter,
		CurrencyTemplateID:    offer.PriceCurrencyTemplateID,
		CurrencyAmount:        -totalCost,
		CurrencyBalanceBefore: availableFunds,
		CurrencyBalanceAfter:  availableFunds - totalCost,
	})
	result := cloneCharacterItems(items)
	sortCharacterItems(result)
	return result, nil
}

func (repo memoryCharacterItemRepo) ExchangeOffer(ctx context.Context, characterID string, offer ExchangeOffer, quantity int) ([]CharacterItem, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if quantity <= 0 || offer.ID == "" || offer.TemplateID == "" || offer.CostTemplateID == "" || offer.CostAmount <= 0 || offer.Quantity <= 0 {
		return nil, errExchangeOfferNotFound
	}

	items := repo.backend.characterItems[characterID]
	totalCost := offer.CostAmount * quantity
	availableMaterials := 0
	rewardQuantityBefore := 0
	for _, item := range items {
		if item.ContainerKind == itemContainerInventory && item.TemplateID == offer.CostTemplateID {
			availableMaterials += item.Quantity
		}
		if item.ContainerKind == itemContainerInventory && item.TemplateID == offer.TemplateID {
			rewardQuantityBefore += item.Quantity
		}
	}
	if availableMaterials < totalCost {
		return nil, errInsufficientMaterials
	}

	remainingCost := totalCost
	filtered := make([]CharacterItem, 0, len(items))
	for _, item := range items {
		if remainingCost > 0 && item.ContainerKind == itemContainerInventory && item.TemplateID == offer.CostTemplateID {
			if item.Quantity <= remainingCost {
				remainingCost -= item.Quantity
				continue
			}
			item.Quantity -= remainingCost
			remainingCost = 0
		}
		filtered = append(filtered, item)
	}
	items = filtered

	totalGranted := offer.Quantity * quantity
	rewardItemID := ""
	rewardQuantityAfter := 0
	if itemTemplateIsStackable(offer.TemplateID) {
		stacked := false
		for index := range items {
			if items[index].ContainerKind == itemContainerInventory && items[index].TemplateID == offer.TemplateID {
				items[index].Quantity += totalGranted
				rewardItemID = items[index].ID
				rewardQuantityAfter = items[index].Quantity
				stacked = true
				break
			}
		}
		if !stacked {
			rewardItemID = randomID("item")
			items = append(items, CharacterItem{
				ID:            rewardItemID,
				CharacterID:   characterID,
				TemplateID:    offer.TemplateID,
				Quantity:      totalGranted,
				ContainerKind: itemContainerInventory,
			})
			rewardQuantityAfter = totalGranted
		}
	} else {
		for count := 0; count < totalGranted; count++ {
			nextItemID := randomID("item")
			if rewardItemID == "" {
				rewardItemID = nextItemID
			}
			items = append(items, CharacterItem{
				ID:            nextItemID,
				CharacterID:   characterID,
				TemplateID:    offer.TemplateID,
				Quantity:      1,
				ContainerKind: itemContainerInventory,
			})
		}
		rewardQuantityAfter = 1
	}

	repo.backend.characterItems[characterID] = items
	repo.backend.recordActionLog(ctx, characterID, ActionLogRecord{
		ID:                    randomID("action"),
		CharacterID:           characterID,
		ActionType:            "vendor_exchange",
		ReferenceID:           offer.ID,
		CounterpartyEntity:    offer.NPCEntityID,
		ItemInstanceID:        rewardItemID,
		TemplateID:            offer.TemplateID,
		Quantity:              totalGranted,
		ItemQuantityBefore:    rewardQuantityBefore,
		ItemQuantityAfter:     rewardQuantityAfter,
		CurrencyTemplateID:    offer.CostTemplateID,
		CurrencyAmount:        -totalCost,
		CurrencyBalanceBefore: availableMaterials,
		CurrencyBalanceAfter:  availableMaterials - totalCost,
	})
	result := cloneCharacterItems(items)
	sortCharacterItems(result)
	return result, nil
}

func (repo memoryCharacterItemRepo) SellVendorItem(ctx context.Context, characterID string, itemID string, quantity int) ([]CharacterItem, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	items := repo.backend.characterItems[characterID]
	sourceIndex := -1
	for index := range items {
		if items[index].ID == itemID {
			sourceIndex = index
			break
		}
	}
	if sourceIndex == -1 || items[sourceIndex].ContainerKind != itemContainerInventory {
		return nil, errItemNotFound
	}
	if quantity <= 0 {
		return nil, errInvalidSplitQuantity
	}

	source := items[sourceIndex]
	sourceQuantityBefore := source.Quantity
	sellValue, sellable := vendorSellValue(source.TemplateID)
	if !sellable || sellValue.Amount <= 0 || sellValue.CurrencyTemplateID == "" {
		return nil, errItemNotSellable
	}

	if !itemTemplateIsStackable(source.TemplateID) {
		if quantity != 1 || source.Quantity != 1 {
			return nil, errInvalidSplitQuantity
		}
		items = append(items[:sourceIndex], items[sourceIndex+1:]...)
	} else {
		if quantity > source.Quantity {
			return nil, errInvalidSplitQuantity
		}
		if quantity == source.Quantity {
			items = append(items[:sourceIndex], items[sourceIndex+1:]...)
		} else {
			items[sourceIndex].Quantity -= quantity
		}
	}

	totalValue := sellValue.Amount * quantity
	currencyBalanceBefore := 0
	stacked := false
	for index := range items {
		if items[index].ContainerKind == itemContainerInventory && items[index].TemplateID == sellValue.CurrencyTemplateID && items[index].EquipSlot == "" {
			currencyBalanceBefore = items[index].Quantity
			items[index].Quantity += totalValue
			stacked = true
			break
		}
	}
	if !stacked {
		items = append(items, CharacterItem{
			ID:            randomID("item"),
			CharacterID:   characterID,
			TemplateID:    sellValue.CurrencyTemplateID,
			Quantity:      totalValue,
			ContainerKind: itemContainerInventory,
		})
	}

	repo.backend.characterItems[characterID] = items
	repo.backend.recordActionLog(ctx, characterID, ActionLogRecord{
		ID:                    randomID("action"),
		CharacterID:           characterID,
		ActionType:            "vendor_sell",
		CounterpartyEntity:    "npc_merchant",
		ItemInstanceID:        source.ID,
		TemplateID:            source.TemplateID,
		Quantity:              quantity,
		ItemQuantityBefore:    sourceQuantityBefore,
		ItemQuantityAfter:     max(0, sourceQuantityBefore-quantity),
		CurrencyTemplateID:    sellValue.CurrencyTemplateID,
		CurrencyAmount:        totalValue,
		CurrencyBalanceBefore: currencyBalanceBefore,
		CurrencyBalanceAfter:  currencyBalanceBefore + totalValue,
	})
	result := cloneCharacterItems(items)
	sortCharacterItems(result)
	return result, nil
}

func (repo memoryCharacterItemRepo) DepositWarehouseItem(ctx context.Context, characterID string, itemID string, quantity int) ([]CharacterItem, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	items := repo.backend.characterItems[characterID]
	sourceIndex := -1
	for index := range items {
		if items[index].ID == itemID {
			sourceIndex = index
			break
		}
	}
	if sourceIndex == -1 {
		return nil, errItemNotFound
	}
	sourceQuantityBefore := items[sourceIndex].Quantity

	updatedItems, err := transferMemoryItemBetweenContainers(items, sourceIndex, itemContainerInventory, itemContainerWarehouse, quantity)
	if err != nil {
		return nil, err
	}
	repo.backend.characterItems[characterID] = updatedItems
	repo.backend.recordStorageTransfer(ctx, characterID, StorageTransferRecord{
		ID:                 randomID("transfer"),
		CharacterID:        characterID,
		SourceItemID:       itemID,
		TemplateID:         items[sourceIndex].TemplateID,
		Quantity:           quantity,
		ItemQuantityBefore: sourceQuantityBefore,
		ItemQuantityAfter:  max(0, sourceQuantityBefore-quantity),
		FromContainerKind:  itemContainerInventory,
		ToContainerKind:    itemContainerWarehouse,
		TransferType:       "warehouse_deposit",
		CounterpartyEntity: warehouseNPCEntityID,
	})
	repo.backend.recordActionLog(ctx, characterID, ActionLogRecord{
		ID:                 randomID("action"),
		CharacterID:        characterID,
		ActionType:         "warehouse_deposit",
		CounterpartyEntity: warehouseNPCEntityID,
		ItemInstanceID:     itemID,
		TemplateID:         items[sourceIndex].TemplateID,
		Quantity:           quantity,
		ItemQuantityBefore: sourceQuantityBefore,
		ItemQuantityAfter:  max(0, sourceQuantityBefore-quantity),
		FromContainerKind:  itemContainerInventory,
		ToContainerKind:    itemContainerWarehouse,
	})
	result := cloneCharacterItems(updatedItems)
	sortCharacterItems(result)
	return result, nil
}

func (repo memoryCharacterItemRepo) WithdrawWarehouseItem(ctx context.Context, characterID string, itemID string, quantity int) ([]CharacterItem, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	items := repo.backend.characterItems[characterID]
	sourceIndex := -1
	for index := range items {
		if items[index].ID == itemID {
			sourceIndex = index
			break
		}
	}
	if sourceIndex == -1 {
		return nil, errWarehouseItemNotFound
	}
	sourceQuantityBefore := items[sourceIndex].Quantity

	updatedItems, err := transferMemoryItemBetweenContainers(items, sourceIndex, itemContainerWarehouse, itemContainerInventory, quantity)
	if err != nil {
		return nil, err
	}
	repo.backend.characterItems[characterID] = updatedItems
	repo.backend.recordStorageTransfer(ctx, characterID, StorageTransferRecord{
		ID:                 randomID("transfer"),
		CharacterID:        characterID,
		SourceItemID:       itemID,
		TemplateID:         items[sourceIndex].TemplateID,
		Quantity:           quantity,
		ItemQuantityBefore: sourceQuantityBefore,
		ItemQuantityAfter:  max(0, sourceQuantityBefore-quantity),
		FromContainerKind:  itemContainerWarehouse,
		ToContainerKind:    itemContainerInventory,
		TransferType:       "warehouse_withdraw",
		CounterpartyEntity: warehouseNPCEntityID,
	})
	repo.backend.recordActionLog(ctx, characterID, ActionLogRecord{
		ID:                 randomID("action"),
		CharacterID:        characterID,
		ActionType:         "warehouse_withdraw",
		CounterpartyEntity: warehouseNPCEntityID,
		ItemInstanceID:     itemID,
		TemplateID:         items[sourceIndex].TemplateID,
		Quantity:           quantity,
		ItemQuantityBefore: sourceQuantityBefore,
		ItemQuantityAfter:  max(0, sourceQuantityBefore-quantity),
		FromContainerKind:  itemContainerWarehouse,
		ToContainerKind:    itemContainerInventory,
	})
	result := cloneCharacterItems(updatedItems)
	sortCharacterItems(result)
	return result, nil
}

func (repo memoryCharacterItemRepo) TradeInventoryItem(
	ctx context.Context,
	sourceCharacterID string,
	targetCharacterID string,
	itemID string,
	quantity int,
	referenceID string,
) ([]CharacterItem, []CharacterItem, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	if _, exists := repo.backend.characters[sourceCharacterID]; !exists {
		return nil, nil, errRecordNotFound
	}
	if _, exists := repo.backend.characters[targetCharacterID]; !exists {
		return nil, nil, errRecordNotFound
	}

	sourceItems := repo.backend.characterItems[sourceCharacterID]
	targetItems := repo.backend.characterItems[targetCharacterID]
	sourceIndex := -1
	for index := range sourceItems {
		if sourceItems[index].ID == itemID {
			sourceIndex = index
			break
		}
	}
	if sourceIndex == -1 {
		return nil, nil, errItemNotFound
	}
	sourceQuantityBefore := sourceItems[sourceIndex].Quantity

	updatedSourceItems, updatedTargetItems, sourceItem, targetItemID, err := transferMemoryInventoryItemBetweenCharacters(
		sourceItems,
		sourceIndex,
		targetItems,
		targetCharacterID,
		quantity,
	)
	if err != nil {
		return nil, nil, err
	}

	repo.backend.characterItems[sourceCharacterID] = updatedSourceItems
	repo.backend.characterItems[targetCharacterID] = updatedTargetItems
	sourceQuantityAfter := 0
	for _, item := range updatedSourceItems {
		if item.ID == sourceItem.ID {
			sourceQuantityAfter = item.Quantity
			break
		}
	}
	targetQuantityBefore := 0
	for _, item := range targetItems {
		if item.ID == targetItemID {
			targetQuantityBefore = item.Quantity
			break
		}
	}
	targetQuantityAfter := 0
	for _, item := range updatedTargetItems {
		if item.ID == targetItemID {
			targetQuantityAfter = item.Quantity
			break
		}
	}
	repo.backend.recordActionLog(ctx, targetCharacterID, ActionLogRecord{
		ID:                 randomID("action"),
		CharacterID:        targetCharacterID,
		ActionType:         "player_trade_accept",
		ReferenceID:        referenceID,
		CounterpartyEntity: sourceCharacterID,
		ItemInstanceID:     targetItemID,
		TemplateID:         sourceItem.TemplateID,
		Quantity:           quantity,
		ItemQuantityBefore: targetQuantityBefore,
		ItemQuantityAfter:  targetQuantityAfter,
	})
	repo.backend.recordActionLog(ctx, sourceCharacterID, ActionLogRecord{
		ID:                 randomID("action"),
		CharacterID:        sourceCharacterID,
		ActionType:         "player_trade_send",
		ReferenceID:        referenceID,
		CounterpartyEntity: targetCharacterID,
		ItemInstanceID:     sourceItem.ID,
		TemplateID:         sourceItem.TemplateID,
		Quantity:           quantity,
		ItemQuantityBefore: sourceQuantityBefore,
		ItemQuantityAfter:  sourceQuantityAfter,
	})
	repo.backend.recordActionLog(ctx, targetCharacterID, ActionLogRecord{
		ID:                 randomID("action"),
		CharacterID:        targetCharacterID,
		ActionType:         "player_trade_receive",
		ReferenceID:        referenceID,
		CounterpartyEntity: sourceCharacterID,
		ItemInstanceID:     targetItemID,
		TemplateID:         sourceItem.TemplateID,
		Quantity:           quantity,
		ItemQuantityBefore: targetQuantityBefore,
		ItemQuantityAfter:  targetQuantityAfter,
	})

	sourceResult := cloneCharacterItems(updatedSourceItems)
	targetResult := cloneCharacterItems(updatedTargetItems)
	sortCharacterItems(sourceResult)
	sortCharacterItems(targetResult)
	return sourceResult, targetResult, nil
}

func (repo memoryStorageTransferRecordRepo) ListByCharacterID(_ context.Context, characterID string) ([]StorageTransferRecord, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	records := append([]StorageTransferRecord(nil), repo.backend.storageTransfers[characterID]...)
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].ID < records[j].ID
		}
		return records[i].CreatedAt.Before(records[j].CreatedAt)
	})
	return records, nil
}

func (repo memoryStorageTransferRecordRepo) ListByFilter(_ context.Context, query StorageTransferQuery) ([]StorageTransferRecord, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	records := make([]StorageTransferRecord, 0)
	for characterID, stored := range repo.backend.storageTransfers {
		if query.CharacterID != "" && characterID != query.CharacterID {
			continue
		}
		for _, record := range stored {
			if query.SourceItemID != "" && record.SourceItemID != query.SourceItemID {
				continue
			}
			if query.TransferType != "" && record.TransferType != query.TransferType {
				continue
			}
			if query.OccurredAfter != nil && record.CreatedAt.Before(*query.OccurredAfter) {
				continue
			}
			if query.OccurredBefore != nil && record.CreatedAt.After(*query.OccurredBefore) {
				continue
			}
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].ID > records[j].ID
		}
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	limit, offset := normalizeAuditPagination(query.Limit, query.Offset)
	if offset >= len(records) {
		return []StorageTransferRecord{}, nil
	}
	end := offset + limit
	if end > len(records) {
		end = len(records)
	}
	return append([]StorageTransferRecord(nil), records[offset:end]...), nil
}

func (repo memoryActionLogRepo) ListByCharacterID(_ context.Context, characterID string) ([]ActionLogRecord, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	records := append([]ActionLogRecord(nil), repo.backend.actionLogs[characterID]...)
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].ID < records[j].ID
		}
		return records[i].CreatedAt.Before(records[j].CreatedAt)
	})
	return records, nil
}

func (repo memoryActionLogRepo) ListByFilter(_ context.Context, query ActionLogQuery) ([]ActionLogRecord, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	records := make([]ActionLogRecord, 0)
	for characterID, stored := range repo.backend.actionLogs {
		if query.CharacterID != "" && characterID != query.CharacterID {
			continue
		}
		for _, record := range stored {
			if query.InvolvedCharacterID != "" && record.CharacterID != query.InvolvedCharacterID && record.CounterpartyEntity != query.InvolvedCharacterID {
				continue
			}
			if query.ItemInstanceID != "" && record.ItemInstanceID != query.ItemInstanceID {
				continue
			}
			if query.ActionType != "" && record.ActionType != query.ActionType {
				continue
			}
			if len(query.ActionTypes) > 0 {
				matched := false
				for _, actionType := range query.ActionTypes {
					if record.ActionType == actionType {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			if query.ReferenceID != "" && record.ReferenceID != query.ReferenceID {
				continue
			}
			if query.OccurredAfter != nil && record.CreatedAt.Before(*query.OccurredAfter) {
				continue
			}
			if query.OccurredBefore != nil && record.CreatedAt.After(*query.OccurredBefore) {
				continue
			}
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].ID > records[j].ID
		}
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	limit, offset := normalizeAuditPagination(query.Limit, query.Offset)
	if offset >= len(records) {
		return []ActionLogRecord{}, nil
	}
	end := offset + limit
	if end > len(records) {
		end = len(records)
	}
	return append([]ActionLogRecord(nil), records[offset:end]...), nil
}

func (repo memoryActionLogRepo) Create(ctx context.Context, record ActionLogRecord) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	repo.backend.recordActionLog(ctx, record.CharacterID, record)
	return nil
}

func (repo memoryPvPCombatEventRepo) ListByFilter(_ context.Context, query PvPCombatEventQuery) ([]PvPCombatEvent, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	records := make([]PvPCombatEvent, 0)
	for _, record := range repo.backend.pvpCombatEvents {
		if query.AttackerCharacterID != "" && record.AttackerCharacterID != query.AttackerCharacterID {
			continue
		}
		if query.VictimCharacterID != "" && record.VictimCharacterID != query.VictimCharacterID {
			continue
		}
		if query.KillerCharacterID != "" && record.KillerCharacterID != query.KillerCharacterID {
			continue
		}
		if query.InvolvedCharacterID != "" && record.AttackerCharacterID != query.InvolvedCharacterID && record.VictimCharacterID != query.InvolvedCharacterID {
			continue
		}
		if query.ActionType != "" && record.ActionType != query.ActionType {
			continue
		}
		if query.Result != "" && record.Result != query.Result {
			continue
		}
		if query.Suspicious != nil && record.Suspicious != *query.Suspicious {
			continue
		}
		if query.OccurredAfter != nil && record.CreatedAt.Before(*query.OccurredAfter) {
			continue
		}
		if query.OccurredBefore != nil && record.CreatedAt.After(*query.OccurredBefore) {
			continue
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].ID > records[j].ID
		}
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	limit, offset := normalizeAuditPagination(query.Limit, query.Offset)
	if offset >= len(records) {
		return []PvPCombatEvent{}, nil
	}
	end := min(len(records), offset+limit)
	return append([]PvPCombatEvent(nil), records[offset:end]...), nil
}

func (repo memoryGameplaySessionRepo) Create(_ context.Context, session *Session) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	repo.backend.sessions[session.ID] = cloneSession(session)
	return nil
}

func (repo memoryGameplaySessionRepo) GetByID(_ context.Context, sessionID string) (*Session, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	session, exists := repo.backend.sessions[sessionID]
	if !exists {
		return nil, errRecordNotFound
	}
	return cloneSession(session), nil
}

func (repo memoryGameplaySessionRepo) GetLatestPendingForCharacter(_ context.Context, characterID string, now time.Time) (*Session, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	var latest *Session
	for _, session := range repo.backend.sessions {
		if session.CharacterID != characterID {
			continue
		}
		if session.Status != sessionStatusPendingAttach {
			continue
		}
		if now.After(session.AttachExpiresAt) {
			continue
		}
		if latest == nil || session.AttachExpiresAt.After(latest.AttachExpiresAt) {
			latest = cloneSession(session)
		}
	}
	if latest == nil {
		return nil, errRecordNotFound
	}
	return latest, nil
}

func (repo memoryGameplaySessionRepo) HasAttachedForCharacter(_ context.Context, characterID string) (bool, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	for _, session := range repo.backend.sessions {
		if session.CharacterID == characterID && session.Status == sessionStatusAttached {
			return true, nil
		}
	}
	return false, nil
}

func (repo memoryGameplaySessionRepo) ExpireStalePendingAttach(_ context.Context, characterID string, now time.Time) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	for _, session := range repo.backend.sessions {
		if session.CharacterID == characterID && session.Status == sessionStatusPendingAttach && now.After(session.AttachExpiresAt) {
			session.Status = sessionStatusExpired
		}
	}
	return nil
}

func (repo memoryGameplaySessionRepo) SanitizeStartupLifecycle(_ context.Context, now time.Time) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	for _, session := range repo.backend.sessions {
		switch session.Status {
		case sessionStatusAttached:
			session.Status = sessionStatusClosed
		case sessionStatusPendingAttach:
			if now.After(session.AttachExpiresAt) {
				session.Status = sessionStatusExpired
			}
		}
	}
	return nil
}

func (repo memoryGameplaySessionRepo) UpdateStatus(_ context.Context, sessionID string, status SessionStatus) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	session, exists := repo.backend.sessions[sessionID]
	if !exists {
		return errRecordNotFound
	}
	session.Status = status
	return nil
}

func cloneSession(session *Session) *Session {
	sessionCopy := *session
	return &sessionCopy
}

func gameplayCommandRecordKey(sessionID string, commandSeq int) string {
	return fmt.Sprintf("%s:%d", sessionID, commandSeq)
}

func cloneGameplayCommandRecord(record *GameplayCommandRecord) *GameplayCommandRecord {
	if record == nil {
		return nil
	}
	recordCopy := *record
	recordCopy.OutboundMessages = cloneOutboundMessages(record.OutboundMessages)
	return &recordCopy
}

func cloneCharacterItems(items []CharacterItem) []CharacterItem {
	result := make([]CharacterItem, len(items))
	for index, item := range items {
		result[index] = item
		result[index].InstanceAttributes = cloneItemInstanceAttributes(item.InstanceAttributes)
	}
	return result
}

func (m *memoryStoreBackend) CreateCharacterWithItemSeed(_ context.Context, character *Character, items []CharacterItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	normalizedName := normalizeName(character.Name)
	if _, exists := m.nameIndex[normalizedName]; exists {
		return errRecordConflict
	}

	characterState, _ := resourcePoolsForCharacter(character, items)
	characterCopy := characterState
	m.characters[character.ID] = &characterCopy
	m.nameIndex[normalizedName] = character.ID

	itemCopies := cloneCharacterItems(items)
	m.characterItems[character.ID] = itemCopies
	m.characterHotbars[character.ID] = defaultCharacterHotbarState(&characterCopy)
	m.characterQuests[character.ID] = []CharacterQuestState{
		normalizeMemoryQuestState(character.ID, defaultCharacterQuestState()),
	}
	return nil
}
