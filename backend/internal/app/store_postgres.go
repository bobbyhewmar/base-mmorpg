package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type postgresStoreBackend struct {
	db            *sql.DB
	lootMu        sync.Mutex
	collectedLoot map[string]struct{}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func encodeItemInstanceAttributesJSON(attrs *ItemInstanceAttributes) string {
	normalized := normalizeItemInstanceAttributes(attrs)
	if normalized == nil {
		return "{}"
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func decodeItemInstanceAttributesJSON(payload string) (*ItemInstanceAttributes, error) {
	if strings.TrimSpace(payload) == "" {
		return nil, nil
	}
	var attrs ItemInstanceAttributes
	if err := json.Unmarshal([]byte(payload), &attrs); err != nil {
		return nil, err
	}
	return normalizeItemInstanceAttributes(&attrs), nil
}

func scanCharacterItemRow(scanner rowScanner) (CharacterItem, error) {
	var item CharacterItem
	var containerKind string
	var equipSlot string
	var instanceAttributesJSON string
	if err := scanner.Scan(
		&item.ID,
		&item.CharacterID,
		&item.TemplateID,
		&item.Quantity,
		&containerKind,
		&equipSlot,
		&instanceAttributesJSON,
	); err != nil {
		return CharacterItem{}, err
	}
	item.ContainerKind = ItemContainer(containerKind)
	item.EquipSlot = EquipSlot(equipSlot)
	decodedAttributes, err := decodeItemInstanceAttributesJSON(instanceAttributesJSON)
	if err != nil {
		return CharacterItem{}, err
	}
	item.InstanceAttributes = decodedAttributes
	return item, nil
}

func (p *postgresStoreBackend) CreateCharacterWithItemSeed(ctx context.Context, character *Character, items []CharacterItem) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	characterState, _ := resourcePoolsForCharacter(character, items)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO characters (character_id, account_id, name, race, base_class, sex, hair_style, hair_color, skin_type, level, xp, current_cp, current_hp, current_mp, last_region_id, current_position_x, current_position_z, is_enterable)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`,
		characterState.ID,
		characterState.AccountID,
		characterState.Name,
		characterState.Race,
		characterState.BaseClass,
		characterState.Sex,
		characterState.HairStyle,
		characterState.HairColor,
		characterState.SkinType,
		characterState.Level,
		characterState.XP,
		characterState.CurrentCP,
		characterState.CurrentHP,
		characterState.CurrentMP,
		characterState.LastRegionID,
		characterState.PositionX,
		characterState.PositionZ,
		characterState.IsEnterable,
	); err != nil {
		return mapPostgresError(err)
	}

	for _, item := range items {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO character_items (item_instance_id, character_id, template_id, quantity, container_kind, equip_slot, instance_attributes_json)
			 VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7::jsonb)`,
			item.ID,
			item.CharacterID,
			item.TemplateID,
			item.Quantity,
			string(item.ContainerKind),
			string(item.EquipSlot),
			encodeItemInstanceAttributesJSON(item.InstanceAttributes),
		); err != nil {
			return mapPostgresError(err)
		}
	}

	for _, slot := range defaultCharacterHotbarState(&characterState).Slots {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO character_hotbar_loadouts (character_id, slot_index, entry_type, skill_id, item_instance_id, action_id, open_bar_count)
			 VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), $7)`,
			characterState.ID,
			slot.SlotIndex,
			slot.EntryType,
			slot.SkillID,
			slot.ItemInstanceID,
			slot.ActionID,
			1,
		); err != nil {
			return mapPostgresError(err)
		}
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO character_quests (character_id, quest_id, status, progress)
		 VALUES ($1, $2, $3, $4)`,
		characterState.ID,
		keeperRequestQuestDefinition.ID,
		string(questStatusAvailable),
		0,
	); err != nil {
		return mapPostgresError(err)
	}

	return tx.Commit()
}

func (p *postgresStoreBackend) CreateAccountWithCredential(ctx context.Context, account *Account, credential *CredentialRecord) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO accounts (account_id, login, display_name, state) VALUES ($1, $2, $3, $4)`,
		account.ID,
		strings.TrimSpace(strings.ToLower(account.Login)),
		account.DisplayName,
		string(account.State),
	); err != nil {
		return mapPostgresError(err)
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO account_credentials (account_id, password_hash, password_algorithm) VALUES ($1, $2, $3)`,
		credential.AccountID,
		credential.PasswordHash,
		credential.PasswordAlgorithm,
	); err != nil {
		return mapPostgresError(err)
	}

	return tx.Commit()
}

func (p *postgresStoreBackend) GetByLoginWithCredential(ctx context.Context, login string) (*Account, *CredentialRecord, error) {
	row := p.db.QueryRowContext(
		ctx,
		`SELECT a.account_id, a.login, a.display_name, a.state, c.password_hash, c.password_algorithm
		 FROM accounts a
		 JOIN account_credentials c ON c.account_id = a.account_id
		 WHERE a.login = $1`,
		strings.TrimSpace(strings.ToLower(login)),
	)

	account := &Account{}
	credential := &CredentialRecord{}
	var state string
	if err := row.Scan(&account.ID, &account.Login, &account.DisplayName, &state, &credential.PasswordHash, &credential.PasswordAlgorithm); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, errRecordNotFound
		}
		return nil, nil, err
	}
	account.State = AccountState(state)
	credential.AccountID = account.ID
	return account, credential, nil
}

func (p *postgresStoreBackend) Create(ctx context.Context, account *Account) error {
	_, err := p.db.ExecContext(
		ctx,
		`INSERT INTO accounts (account_id, login, display_name, state) VALUES ($1, $2, $3, $4)`,
		account.ID,
		strings.TrimSpace(strings.ToLower(account.Login)),
		account.DisplayName,
		string(account.State),
	)
	return mapPostgresError(err)
}

func (p *postgresStoreBackend) GetByID(ctx context.Context, accountID string) (*Account, error) {
	row := p.db.QueryRowContext(ctx, `SELECT account_id, login, display_name, state FROM accounts WHERE account_id = $1`, accountID)
	account := &Account{}
	var state string
	if err := row.Scan(&account.ID, &account.Login, &account.DisplayName, &state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	account.State = AccountState(state)
	return account, nil
}

func (p *postgresStoreBackend) GetByLogin(ctx context.Context, login string) (*Account, error) {
	row := p.db.QueryRowContext(
		ctx,
		`SELECT account_id, login, display_name, state FROM accounts WHERE login = $1`,
		strings.TrimSpace(strings.ToLower(login)),
	)
	account := &Account{}
	var state string
	if err := row.Scan(&account.ID, &account.Login, &account.DisplayName, &state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	account.State = AccountState(state)
	return account, nil
}

func (p *postgresStoreBackend) CreateCredential(ctx context.Context, credential *CredentialRecord) error {
	_, err := p.db.ExecContext(
		ctx,
		`INSERT INTO account_credentials (account_id, password_hash, password_algorithm) VALUES ($1, $2, $3)`,
		credential.AccountID,
		credential.PasswordHash,
		credential.PasswordAlgorithm,
	)
	return mapPostgresError(err)
}

func (p *postgresStoreBackend) GetByAccountID(ctx context.Context, accountID string) (*CredentialRecord, error) {
	row := p.db.QueryRowContext(
		ctx,
		`SELECT account_id, password_hash, password_algorithm FROM account_credentials WHERE account_id = $1`,
		accountID,
	)
	credential := &CredentialRecord{}
	if err := row.Scan(&credential.AccountID, &credential.PasswordHash, &credential.PasswordAlgorithm); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	return credential, nil
}

func (p *postgresStoreBackend) UpdateCredential(ctx context.Context, credential *CredentialRecord) error {
	result, err := p.db.ExecContext(
		ctx,
		`UPDATE account_credentials
		 SET password_hash = $2,
		     password_algorithm = $3,
		     updated_at = NOW()
		 WHERE account_id = $1`,
		credential.AccountID,
		credential.PasswordHash,
		credential.PasswordAlgorithm,
	)
	if err != nil {
		return mapPostgresError(err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errRecordNotFound
	}
	return nil
}

func (p *postgresStoreBackend) CreateAccountSession(ctx context.Context, session *AccountSession) error {
	_, err := p.db.ExecContext(
		ctx,
		`INSERT INTO account_sessions (access_token, account_id, expires_at, revoked_at)
		 VALUES ($1, $2, $3, $4)`,
		session.Token,
		session.AccountID,
		session.ExpiresAt,
		session.RevokedAt,
	)
	return mapPostgresError(err)
}

func (p *postgresStoreBackend) GetActiveAccountSessionByToken(ctx context.Context, token string, now time.Time) (*AccountSession, error) {
	row := p.db.QueryRowContext(
		ctx,
		`SELECT access_token, account_id, expires_at, revoked_at
		 FROM account_sessions
		 WHERE access_token = $1
		   AND revoked_at IS NULL
		   AND expires_at > $2`,
		token,
		now,
	)
	session := &AccountSession{}
	if err := row.Scan(&session.Token, &session.AccountID, &session.ExpiresAt, &session.RevokedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	return session, nil
}

func (p *postgresStoreBackend) RevokeAccountSessionByToken(ctx context.Context, token string, now time.Time) error {
	result, err := p.db.ExecContext(
		ctx,
		`UPDATE account_sessions
		 SET revoked_at = COALESCE(revoked_at, $2),
		     updated_at = NOW()
		 WHERE access_token = $1`,
		token,
		now,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errRecordNotFound
	}
	return nil
}

func (p *postgresStoreBackend) CreateGameplayCommandRecordPending(ctx context.Context, record *GameplayCommandRecord) error {
	_, err := p.db.ExecContext(
		ctx,
		`INSERT INTO gameplay_command_records (session_id, command_seq, command_id, command_type, status, outcome_json)
		 VALUES ($1, $2, $3, $4, $5, NULL)`,
		record.SessionID,
		record.CommandSeq,
		record.CommandID,
		record.CommandType,
		string(gameplayCommandRecordStatusPending),
	)
	return mapPostgresError(err)
}

func (p *postgresStoreBackend) GetGameplayCommandRecordBySessionAndSeq(ctx context.Context, sessionID string, commandSeq int) (*GameplayCommandRecord, error) {
	row := p.db.QueryRowContext(
		ctx,
		`SELECT session_id, command_seq, command_id, command_type, status, outcome_json
		 FROM gameplay_command_records
		 WHERE session_id = $1
		   AND command_seq = $2`,
		sessionID,
		commandSeq,
	)
	record := &GameplayCommandRecord{}
	var status string
	var outcomeJSON []byte
	if err := row.Scan(&record.SessionID, &record.CommandSeq, &record.CommandID, &record.CommandType, &status, &outcomeJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	record.Status = GameplayCommandRecordStatus(status)
	if len(outcomeJSON) > 0 {
		if err := json.Unmarshal(outcomeJSON, &record.OutboundMessages); err != nil {
			return nil, err
		}
	}
	return record, nil
}

func (p *postgresStoreBackend) UpdateGameplayCommandRecordOutcome(ctx context.Context, sessionID string, commandSeq int, status GameplayCommandRecordStatus, outboundMessages []map[string]any) error {
	outcomeJSON, err := json.Marshal(outboundMessages)
	if err != nil {
		return err
	}
	result, err := p.db.ExecContext(
		ctx,
		`UPDATE gameplay_command_records
		 SET status = $3,
		     outcome_json = $4,
		     updated_at = NOW()
		 WHERE session_id = $1
		   AND command_seq = $2`,
		sessionID,
		commandSeq,
		string(status),
		outcomeJSON,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errRecordNotFound
	}
	return nil
}

func (p *postgresStoreBackend) ListByAccountID(ctx context.Context, accountID string) ([]Character, error) {
	rows, err := p.db.QueryContext(
		ctx,
		`SELECT character_id, account_id, name, race, base_class, sex, hair_style, hair_color, skin_type, level, xp, current_cp, current_hp, current_mp, last_region_id, current_position_x, current_position_z, is_enterable
		 FROM characters
		 WHERE account_id = $1
		 ORDER BY name, character_id`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	characters := make([]Character, 0)
	for rows.Next() {
		var character Character
		if err := rows.Scan(
			&character.ID,
			&character.AccountID,
			&character.Name,
			&character.Race,
			&character.BaseClass,
			&character.Sex,
			&character.HairStyle,
			&character.HairColor,
			&character.SkinType,
			&character.Level,
			&character.XP,
			&character.CurrentCP,
			&character.CurrentHP,
			&character.CurrentMP,
			&character.LastRegionID,
			&character.PositionX,
			&character.PositionZ,
			&character.IsEnterable,
		); err != nil {
			return nil, err
		}
		characters = append(characters, character)
	}
	return characters, rows.Err()
}

func (p *postgresStoreBackend) CountByAccountID(ctx context.Context, accountID string) (int, error) {
	row := p.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM characters WHERE account_id = $1`, accountID)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (p *postgresStoreBackend) GetByIDCharacter(ctx context.Context, characterID string) (*Character, error) {
	row := p.db.QueryRowContext(
		ctx,
		`SELECT character_id, account_id, name, race, base_class, sex, hair_style, hair_color, skin_type, level, xp, current_cp, current_hp, current_mp, last_region_id, current_position_x, current_position_z, is_enterable
		 FROM characters
		 WHERE character_id = $1`,
		characterID,
	)
	var character Character
	if err := row.Scan(
		&character.ID,
		&character.AccountID,
		&character.Name,
		&character.Race,
		&character.BaseClass,
		&character.Sex,
		&character.HairStyle,
		&character.HairColor,
		&character.SkinType,
		&character.Level,
		&character.XP,
		&character.CurrentCP,
		&character.CurrentHP,
		&character.CurrentMP,
		&character.LastRegionID,
		&character.PositionX,
		&character.PositionZ,
		&character.IsEnterable,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	return &character, nil
}

func (p *postgresStoreBackend) CreateCharacter(ctx context.Context, character *Character) error {
	characterState, _ := resourcePoolsForCharacter(character, nil)
	_, err := p.db.ExecContext(
		ctx,
		`INSERT INTO characters (character_id, account_id, name, race, base_class, sex, hair_style, hair_color, skin_type, level, xp, current_cp, current_hp, current_mp, last_region_id, current_position_x, current_position_z, is_enterable)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`,
		characterState.ID,
		characterState.AccountID,
		characterState.Name,
		characterState.Race,
		characterState.BaseClass,
		characterState.Sex,
		characterState.HairStyle,
		characterState.HairColor,
		characterState.SkinType,
		characterState.Level,
		characterState.XP,
		characterState.CurrentCP,
		characterState.CurrentHP,
		characterState.CurrentMP,
		characterState.LastRegionID,
		characterState.PositionX,
		characterState.PositionZ,
		characterState.IsEnterable,
	)
	return mapPostgresError(err)
}

func (p *postgresStoreBackend) UpdateCharacterWorldState(ctx context.Context, characterID string, regionID string, positionX float64, positionZ float64) error {
	result, err := p.db.ExecContext(
		ctx,
		`UPDATE characters
		 SET last_region_id = $2,
		     current_position_x = $3,
		     current_position_z = $4,
		     updated_at = NOW()
		 WHERE character_id = $1`,
		characterID,
		regionID,
		positionX,
		positionZ,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errRecordNotFound
	}
	return nil
}

func (p *postgresStoreBackend) UpdateCharacterProgression(ctx context.Context, characterID string, level int, xp int, currentCP int, currentHP int, currentMP int) error {
	result, err := p.db.ExecContext(
		ctx,
		`UPDATE characters
		 SET level = $2,
		     xp = $3,
		     current_cp = $4,
		     current_hp = $5,
		     current_mp = $6,
		     updated_at = NOW()
		 WHERE character_id = $1`,
		characterID,
		normalizedCharacterLevel(level),
		max(0, xp),
		currentCP,
		currentHP,
		currentMP,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errRecordNotFound
	}
	return nil
}

func (p *postgresStoreBackend) ListCharacterCooldownsByCharacterID(ctx context.Context, characterID string) ([]CharacterSkillCooldown, error) {
	rows, err := p.db.QueryContext(
		ctx,
		`SELECT character_id, skill_id, ends_at
		 FROM character_skill_cooldowns
		 WHERE character_id = $1
		 ORDER BY skill_id`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cooldowns := make([]CharacterSkillCooldown, 0)
	for rows.Next() {
		var cooldown CharacterSkillCooldown
		if err := rows.Scan(&cooldown.CharacterID, &cooldown.SkillID, &cooldown.EndsAt); err != nil {
			return nil, err
		}
		cooldowns = append(cooldowns, cooldown)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cooldowns, nil
}

func (p *postgresStoreBackend) ReplaceCharacterCooldowns(ctx context.Context, characterID string, cooldowns []CharacterSkillCooldown) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(
		ctx,
		`DELETE FROM character_skill_cooldowns WHERE character_id = $1`,
		characterID,
	); err != nil {
		return err
	}

	for _, cooldown := range cooldowns {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO character_skill_cooldowns (character_id, skill_id, ends_at)
			 VALUES ($1, $2, $3)`,
			characterID,
			cooldown.SkillID,
			cooldown.EndsAt,
		); err != nil {
			return mapPostgresError(err)
		}
	}

	return tx.Commit()
}

func (p *postgresStoreBackend) ListCharacterHotbarStateByCharacterID(ctx context.Context, characterID string) (CharacterHotbarState, error) {
	rows, err := p.db.QueryContext(
		ctx,
		`SELECT slot_index, COALESCE(entry_type, ''), COALESCE(skill_id, ''), COALESCE(item_instance_id, ''), COALESCE(action_id, ''), open_bar_count
		 FROM character_hotbar_loadouts
		 WHERE character_id = $1
		 ORDER BY slot_index`,
		characterID,
	)
	if err != nil {
		return CharacterHotbarState{}, err
	}
	defer rows.Close()

	state := CharacterHotbarState{}
	for rows.Next() {
		var slot CharacterHotbarSlot
		if err := rows.Scan(&slot.SlotIndex, &slot.EntryType, &slot.SkillID, &slot.ItemInstanceID, &slot.ActionID, &state.OpenBarCount); err != nil {
			return CharacterHotbarState{}, err
		}
		state.Slots = append(state.Slots, slot)
	}
	if err := rows.Err(); err != nil {
		return CharacterHotbarState{}, err
	}
	if len(state.Slots) == 0 {
		return CharacterHotbarState{}, errRecordNotFound
	}
	return state, nil
}

func (p *postgresStoreBackend) ReplaceCharacterHotbarStateByCharacterID(ctx context.Context, characterID string, hotbar CharacterHotbarState) error {
	character, err := p.GetByIDCharacter(ctx, characterID)
	if err != nil {
		return err
	}
	normalized := normalizeCharacterHotbarState(hotbar, character)

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM character_hotbar_loadouts WHERE character_id = $1`, characterID); err != nil {
		return mapPostgresError(err)
	}
	for _, slot := range normalized.Slots {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO character_hotbar_loadouts (character_id, slot_index, entry_type, skill_id, item_instance_id, action_id, open_bar_count)
			 VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), $7)`,
			characterID,
			slot.SlotIndex,
			slot.EntryType,
			slot.SkillID,
			slot.ItemInstanceID,
			slot.ActionID,
			normalized.OpenBarCount,
		); err != nil {
			return mapPostgresError(err)
		}
	}

	return tx.Commit()
}

func (p *postgresStoreBackend) ListCharacterQuestsByCharacterID(ctx context.Context, characterID string) ([]CharacterQuestState, error) {
	rows, err := p.db.QueryContext(
		ctx,
		`SELECT character_id, quest_id, status, progress
		 FROM character_quests
		 WHERE character_id = $1
		 ORDER BY quest_id`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	quests := make([]CharacterQuestState, 0)
	for rows.Next() {
		var quest CharacterQuestState
		var status string
		if err := rows.Scan(&quest.CharacterID, &quest.QuestID, &status, &quest.Progress); err != nil {
			return nil, err
		}
		quest.Status = QuestStatus(status)
		quests = append(quests, normalizeCharacterQuestState(quest))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(quests) == 0 {
		return nil, errRecordNotFound
	}
	return quests, nil
}

func (p *postgresStoreBackend) UpsertCharacterQuestByCharacterID(ctx context.Context, quest CharacterQuestState) error {
	normalized := normalizeCharacterQuestState(quest)
	normalized.CharacterID = quest.CharacterID
	_, err := p.db.ExecContext(
		ctx,
		`INSERT INTO character_quests (character_id, quest_id, status, progress, updated_at)
		 VALUES ($1, $2, $3, $4, NOW())
		 ON CONFLICT (character_id, quest_id)
		 DO UPDATE SET
		   status = EXCLUDED.status,
		   progress = EXCLUDED.progress,
		   updated_at = NOW()`,
		normalized.CharacterID,
		normalized.QuestID,
		string(normalized.Status),
		normalized.Progress,
	)
	return mapPostgresError(err)
}

func (p *postgresStoreBackend) ListCharacterItemsByCharacterID(ctx context.Context, characterID string) ([]CharacterItem, error) {
	rows, err := p.db.QueryContext(
		ctx,
		`SELECT item_instance_id, character_id, template_id, quantity, container_kind, COALESCE(equip_slot, ''), COALESCE(instance_attributes_json::text, '{}')
		 FROM character_items
		 WHERE character_id = $1
		 ORDER BY container_kind, equip_slot, template_id, item_instance_id`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanCharacterItems(rows)
}

func (p *postgresStoreBackend) ListStorageTransferRecordsByCharacterID(ctx context.Context, characterID string) ([]StorageTransferRecord, error) {
	rows, err := p.db.QueryContext(
		ctx,
		`SELECT transfer_id, character_id, COALESCE(account_id, ''), source_item_instance_id, template_id, quantity, COALESCE(item_quantity_before, 0), COALESCE(item_quantity_after, 0), from_container_kind, to_container_kind, transfer_type, COALESCE(counterparty_entity_id, ''), COALESCE(session_id, ''), COALESCE(command_id, ''), COALESCE(command_seq, 0), created_at
		 FROM storage_transfer_records
		 WHERE character_id = $1
		 ORDER BY created_at, transfer_id`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]StorageTransferRecord, 0)
	for rows.Next() {
		var record StorageTransferRecord
		var fromContainer string
		var toContainer string
		if err := rows.Scan(
			&record.ID,
			&record.CharacterID,
			&record.AccountID,
			&record.SourceItemID,
			&record.TemplateID,
			&record.Quantity,
			&record.ItemQuantityBefore,
			&record.ItemQuantityAfter,
			&fromContainer,
			&toContainer,
			&record.TransferType,
			&record.CounterpartyEntity,
			&record.SessionID,
			&record.CommandID,
			&record.CommandSeq,
			&record.CreatedAt,
		); err != nil {
			return nil, err
		}
		record.FromContainerKind = ItemContainer(fromContainer)
		record.ToContainerKind = ItemContainer(toContainer)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (p *postgresStoreBackend) ListStorageTransferRecordsByFilter(ctx context.Context, query StorageTransferQuery) ([]StorageTransferRecord, error) {
	baseQuery := strings.Builder{}
	baseQuery.WriteString(
		`SELECT transfer_id, character_id, COALESCE(account_id, ''), source_item_instance_id, template_id, quantity, COALESCE(item_quantity_before, 0), COALESCE(item_quantity_after, 0), from_container_kind, to_container_kind, transfer_type, COALESCE(counterparty_entity_id, ''), COALESCE(session_id, ''), COALESCE(command_id, ''), COALESCE(command_seq, 0), created_at
		 FROM storage_transfer_records`,
	)

	conditions := make([]string, 0, 5)
	args := make([]any, 0, 7)
	if query.CharacterID != "" {
		args = append(args, query.CharacterID)
		conditions = append(conditions, fmt.Sprintf("character_id = $%d", len(args)))
	}
	if query.SourceItemID != "" {
		args = append(args, query.SourceItemID)
		conditions = append(conditions, fmt.Sprintf("source_item_instance_id = $%d", len(args)))
	}
	if query.TransferType != "" {
		args = append(args, query.TransferType)
		conditions = append(conditions, fmt.Sprintf("transfer_type = $%d", len(args)))
	}
	if query.OccurredAfter != nil {
		args = append(args, *query.OccurredAfter)
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if query.OccurredBefore != nil {
		args = append(args, *query.OccurredBefore)
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", len(args)))
	}
	if len(conditions) > 0 {
		baseQuery.WriteString(" WHERE ")
		baseQuery.WriteString(strings.Join(conditions, " AND "))
	}

	query.Limit, query.Offset = normalizeAuditPagination(query.Limit, query.Offset)
	args = append(args, query.Limit, query.Offset)
	baseQuery.WriteString(fmt.Sprintf(" ORDER BY created_at DESC, transfer_id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)))

	rows, err := p.db.QueryContext(ctx, baseQuery.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]StorageTransferRecord, 0)
	for rows.Next() {
		var record StorageTransferRecord
		var fromContainer string
		var toContainer string
		if err := rows.Scan(
			&record.ID,
			&record.CharacterID,
			&record.AccountID,
			&record.SourceItemID,
			&record.TemplateID,
			&record.Quantity,
			&record.ItemQuantityBefore,
			&record.ItemQuantityAfter,
			&fromContainer,
			&toContainer,
			&record.TransferType,
			&record.CounterpartyEntity,
			&record.SessionID,
			&record.CommandID,
			&record.CommandSeq,
			&record.CreatedAt,
		); err != nil {
			return nil, err
		}
		record.FromContainerKind = ItemContainer(fromContainer)
		record.ToContainerKind = ItemContainer(toContainer)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (p *postgresStoreBackend) ListActionLogsByCharacterID(ctx context.Context, characterID string) ([]ActionLogRecord, error) {
	rows, err := p.db.QueryContext(
		ctx,
		`SELECT action_log_id, character_id, COALESCE(account_id, ''), action_type, COALESCE(reference_id, ''), COALESCE(counterparty_entity_id, ''), COALESCE(item_instance_id, ''), COALESCE(template_id, ''), COALESCE(quantity, 0), COALESCE(item_quantity_before, 0), COALESCE(item_quantity_after, 0), COALESCE(currency_template_id, ''), COALESCE(currency_amount, 0), COALESCE(currency_balance_before, 0), COALESCE(currency_balance_after, 0), COALESCE(from_container_kind, ''), COALESCE(to_container_kind, ''), COALESCE(session_id, ''), COALESCE(command_id, ''), COALESCE(command_seq, 0), created_at
		 FROM action_logs
		 WHERE character_id = $1
		 ORDER BY created_at, action_log_id`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]ActionLogRecord, 0)
	for rows.Next() {
		var record ActionLogRecord
		var fromContainer string
		var toContainer string
		if err := rows.Scan(
			&record.ID,
			&record.CharacterID,
			&record.AccountID,
			&record.ActionType,
			&record.ReferenceID,
			&record.CounterpartyEntity,
			&record.ItemInstanceID,
			&record.TemplateID,
			&record.Quantity,
			&record.ItemQuantityBefore,
			&record.ItemQuantityAfter,
			&record.CurrencyTemplateID,
			&record.CurrencyAmount,
			&record.CurrencyBalanceBefore,
			&record.CurrencyBalanceAfter,
			&fromContainer,
			&toContainer,
			&record.SessionID,
			&record.CommandID,
			&record.CommandSeq,
			&record.CreatedAt,
		); err != nil {
			return nil, err
		}
		record.FromContainerKind = ItemContainer(fromContainer)
		record.ToContainerKind = ItemContainer(toContainer)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (p *postgresStoreBackend) ListActionLogsByFilter(ctx context.Context, query ActionLogQuery) ([]ActionLogRecord, error) {
	baseQuery := strings.Builder{}
	baseQuery.WriteString(
		`SELECT action_log_id, character_id, COALESCE(account_id, ''), action_type, COALESCE(reference_id, ''), COALESCE(counterparty_entity_id, ''), COALESCE(item_instance_id, ''), COALESCE(template_id, ''), COALESCE(quantity, 0), COALESCE(item_quantity_before, 0), COALESCE(item_quantity_after, 0), COALESCE(currency_template_id, ''), COALESCE(currency_amount, 0), COALESCE(currency_balance_before, 0), COALESCE(currency_balance_after, 0), COALESCE(from_container_kind, ''), COALESCE(to_container_kind, ''), COALESCE(session_id, ''), COALESCE(command_id, ''), COALESCE(command_seq, 0), created_at
		 FROM action_logs`,
	)

	conditions := make([]string, 0, 8)
	args := make([]any, 0, 10)
	if query.CharacterID != "" {
		args = append(args, query.CharacterID)
		conditions = append(conditions, fmt.Sprintf("character_id = $%d", len(args)))
	}
	if query.InvolvedCharacterID != "" {
		args = append(args, query.InvolvedCharacterID)
		conditions = append(conditions, fmt.Sprintf("(character_id = $%d OR COALESCE(counterparty_entity_id, '') = $%d)", len(args), len(args)))
	}
	if query.ItemInstanceID != "" {
		args = append(args, query.ItemInstanceID)
		conditions = append(conditions, fmt.Sprintf("item_instance_id = $%d", len(args)))
	}
	if query.ActionType != "" {
		args = append(args, query.ActionType)
		conditions = append(conditions, fmt.Sprintf("action_type = $%d", len(args)))
	}
	if len(query.ActionTypes) > 0 {
		placeholders := make([]string, 0, len(query.ActionTypes))
		for _, actionType := range query.ActionTypes {
			args = append(args, actionType)
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
		}
		conditions = append(conditions, "action_type IN ("+strings.Join(placeholders, ", ")+")")
	}
	if query.ReferenceID != "" {
		args = append(args, query.ReferenceID)
		conditions = append(conditions, fmt.Sprintf("reference_id = $%d", len(args)))
	}
	if query.OccurredAfter != nil {
		args = append(args, *query.OccurredAfter)
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if query.OccurredBefore != nil {
		args = append(args, *query.OccurredBefore)
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", len(args)))
	}
	if len(conditions) > 0 {
		baseQuery.WriteString(" WHERE ")
		baseQuery.WriteString(strings.Join(conditions, " AND "))
	}

	query.Limit, query.Offset = normalizeAuditPagination(query.Limit, query.Offset)
	args = append(args, query.Limit, query.Offset)
	baseQuery.WriteString(fmt.Sprintf(" ORDER BY created_at DESC, action_log_id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)))

	rows, err := p.db.QueryContext(ctx, baseQuery.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]ActionLogRecord, 0)
	for rows.Next() {
		var record ActionLogRecord
		var fromContainer string
		var toContainer string
		if err := rows.Scan(
			&record.ID,
			&record.CharacterID,
			&record.AccountID,
			&record.ActionType,
			&record.ReferenceID,
			&record.CounterpartyEntity,
			&record.ItemInstanceID,
			&record.TemplateID,
			&record.Quantity,
			&record.ItemQuantityBefore,
			&record.ItemQuantityAfter,
			&record.CurrencyTemplateID,
			&record.CurrencyAmount,
			&record.CurrencyBalanceBefore,
			&record.CurrencyBalanceAfter,
			&fromContainer,
			&toContainer,
			&record.SessionID,
			&record.CommandID,
			&record.CommandSeq,
			&record.CreatedAt,
		); err != nil {
			return nil, err
		}
		record.FromContainerKind = ItemContainer(fromContainer)
		record.ToContainerKind = ItemContainer(toContainer)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

type sqlContextExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (p *postgresStoreBackend) accountIDForCharacter(ctx context.Context, characterID string) (string, error) {
	row := p.db.QueryRowContext(ctx, `SELECT account_id FROM characters WHERE character_id = $1`, characterID)
	var accountID string
	if err := row.Scan(&accountID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errRecordNotFound
		}
		return "", err
	}
	return accountID, nil
}

func accountIDForCharacterTx(ctx context.Context, tx *sql.Tx, characterID string) (string, error) {
	row := tx.QueryRowContext(ctx, `SELECT account_id FROM characters WHERE character_id = $1`, characterID)
	var accountID string
	if err := row.Scan(&accountID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errRecordNotFound
		}
		return "", err
	}
	return accountID, nil
}

func applyStorageTransferAuditFromContext(ctx context.Context, accountID string, record StorageTransferRecord) StorageTransferRecord {
	record.AccountID = accountID
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

func applyActionLogAuditFromContext(ctx context.Context, accountID string, record ActionLogRecord) ActionLogRecord {
	record.AccountID = accountID
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

func insertStorageTransfer(ctx context.Context, exec sqlContextExecutor, record StorageTransferRecord) error {
	createdAt := record.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := exec.ExecContext(
		ctx,
		`INSERT INTO storage_transfer_records (
			transfer_id,
			character_id,
			account_id,
			source_item_instance_id,
			template_id,
			quantity,
			item_quantity_before,
			item_quantity_after,
			from_container_kind,
			to_container_kind,
			transfer_type,
			counterparty_entity_id,
			session_id,
			command_id,
			command_seq,
			created_at
		) VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, $9, $10, $11, NULLIF($12, ''), NULLIF($13, ''), NULLIF($14, ''), NULLIF($15, 0), $16)`,
		record.ID,
		record.CharacterID,
		record.AccountID,
		record.SourceItemID,
		record.TemplateID,
		record.Quantity,
		record.ItemQuantityBefore,
		record.ItemQuantityAfter,
		string(record.FromContainerKind),
		string(record.ToContainerKind),
		record.TransferType,
		record.CounterpartyEntity,
		record.SessionID,
		record.CommandID,
		record.CommandSeq,
		createdAt,
	)
	return mapPostgresError(err)
}

func recordStorageTransferTx(ctx context.Context, tx *sql.Tx, record StorageTransferRecord) error {
	return insertStorageTransfer(ctx, tx, record)
}

func insertActionLog(ctx context.Context, exec sqlContextExecutor, record ActionLogRecord) error {
	createdAt := record.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := exec.ExecContext(
		ctx,
		`INSERT INTO action_logs (
			action_log_id,
			character_id,
			account_id,
			action_type,
			reference_id,
			counterparty_entity_id,
			item_instance_id,
			template_id,
			quantity,
			item_quantity_before,
			item_quantity_after,
			currency_template_id,
			currency_amount,
			currency_balance_before,
			currency_balance_after,
			from_container_kind,
			to_container_kind,
			session_id,
			command_id,
			command_seq,
			created_at
		) VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, 0), $10, $11, NULLIF($12, ''), $13, $14, $15, NULLIF($16, ''), NULLIF($17, ''), NULLIF($18, ''), NULLIF($19, ''), NULLIF($20, 0), $21)`,
		record.ID,
		record.CharacterID,
		record.AccountID,
		record.ActionType,
		record.ReferenceID,
		record.CounterpartyEntity,
		record.ItemInstanceID,
		record.TemplateID,
		record.Quantity,
		record.ItemQuantityBefore,
		record.ItemQuantityAfter,
		record.CurrencyTemplateID,
		record.CurrencyAmount,
		record.CurrencyBalanceBefore,
		record.CurrencyBalanceAfter,
		string(record.FromContainerKind),
		string(record.ToContainerKind),
		record.SessionID,
		record.CommandID,
		record.CommandSeq,
		createdAt,
	)
	return mapPostgresError(err)
}

func recordActionLogTx(ctx context.Context, tx *sql.Tx, record ActionLogRecord) error {
	return insertActionLog(ctx, tx, record)
}

func grantCharacterInventoryItemTx(ctx context.Context, tx *sql.Tx, characterID string, templateID string, quantity int) error {
	if quantity <= 0 {
		return nil
	}

	if itemTemplateIsStackable(templateID) {
		var existingItemID string
		var existingQuantity int
		err := tx.QueryRowContext(
			ctx,
			`SELECT item_instance_id, quantity
			 FROM character_items
			 WHERE character_id = $1
			   AND template_id = $2
			   AND container_kind = $3
			   AND equip_slot IS NULL
			 FOR UPDATE`,
			characterID,
			templateID,
			string(itemContainerInventory),
		).Scan(&existingItemID, &existingQuantity)
		switch {
		case err == nil:
			_, err = tx.ExecContext(
				ctx,
				`UPDATE character_items
				 SET quantity = $3,
				     updated_at = NOW()
				 WHERE item_instance_id = $1
				   AND character_id = $2`,
				existingItemID,
				characterID,
				existingQuantity+quantity,
			)
			return mapPostgresError(err)
		case errors.Is(err, sql.ErrNoRows):
			_, err = tx.ExecContext(
				ctx,
				`INSERT INTO character_items (item_instance_id, character_id, template_id, quantity, container_kind, equip_slot, instance_attributes_json)
				 VALUES ($1, $2, $3, $4, $5, NULL, $6::jsonb)`,
				randomID("item"),
				characterID,
				templateID,
				quantity,
				string(itemContainerInventory),
				encodeItemInstanceAttributesJSON(nil),
			)
			return mapPostgresError(err)
		default:
			return err
		}
	}

	for count := 0; count < quantity; count++ {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO character_items (item_instance_id, character_id, template_id, quantity, container_kind, equip_slot, instance_attributes_json)
			 VALUES ($1, $2, $3, $4, $5, NULL, $6::jsonb)`,
			randomID("item"),
			characterID,
			templateID,
			1,
			string(itemContainerInventory),
			encodeItemInstanceAttributesJSON(nil),
		); err != nil {
			return mapPostgresError(err)
		}
	}
	return nil
}

func (p *postgresStoreBackend) PickUpLoot(ctx context.Context, characterID string, lootID string, templateID string, quantity int) ([]CharacterItem, error) {
	if quantity <= 0 {
		return nil, errRecordConflict
	}
	p.lootMu.Lock()
	if _, exists := p.collectedLoot[lootID]; exists {
		p.lootMu.Unlock()
		return nil, errLootAlreadyCollected
	}
	p.collectedLoot[lootID] = struct{}{}
	p.lootMu.Unlock()

	success := false
	defer func() {
		if success {
			return
		}
		p.lootMu.Lock()
		delete(p.collectedLoot, lootID)
		p.lootMu.Unlock()
	}()

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if itemTemplateIsStackable(templateID) {
		var existingItemID string
		var existingQuantity int
		err := tx.QueryRowContext(
			ctx,
			`SELECT item_instance_id, quantity
			 FROM character_items
			 WHERE character_id = $1
			   AND template_id = $2
			   AND container_kind = $3
			   AND equip_slot IS NULL
			 FOR UPDATE`,
			characterID,
			templateID,
			string(itemContainerInventory),
		).Scan(&existingItemID, &existingQuantity)
		switch {
		case err == nil:
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE character_items
				 SET quantity = $3,
				     updated_at = NOW()
				 WHERE item_instance_id = $1
				   AND character_id = $2`,
				existingItemID,
				characterID,
				existingQuantity+quantity,
			); err != nil {
				return nil, mapPostgresError(err)
			}
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO character_items (item_instance_id, character_id, template_id, quantity, container_kind, equip_slot, instance_attributes_json)
				 VALUES ($1, $2, $3, $4, $5, NULL, $6::jsonb)`,
				randomID("item"),
				characterID,
				templateID,
				quantity,
				string(itemContainerInventory),
				encodeItemInstanceAttributesJSON(nil),
			); err != nil {
				return nil, mapPostgresError(err)
			}
		default:
			return nil, err
		}
	} else {
		for count := 0; count < quantity; count++ {
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO character_items (item_instance_id, character_id, template_id, quantity, container_kind, equip_slot, instance_attributes_json)
				 VALUES ($1, $2, $3, $4, $5, NULL, $6::jsonb)`,
				randomID("item"),
				characterID,
				templateID,
				1,
				string(itemContainerInventory),
				encodeItemInstanceAttributesJSON(nil),
			); err != nil {
				return nil, mapPostgresError(err)
			}
		}
	}

	items, err := listCharacterItemsTx(ctx, tx, characterID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	success = true
	return items, nil
}

func (p *postgresStoreBackend) CompleteCharacterQuestWithItemReward(
	ctx context.Context,
	quest CharacterQuestState,
	rewardTemplateID string,
	rewardQuantity int,
) ([]CharacterItem, error) {
	normalized := normalizeCharacterQuestState(quest)
	normalized.CharacterID = quest.CharacterID

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO character_quests (character_id, quest_id, status, progress, updated_at)
		 VALUES ($1, $2, $3, $4, NOW())
		 ON CONFLICT (character_id, quest_id)
		 DO UPDATE SET
		   status = EXCLUDED.status,
		   progress = EXCLUDED.progress,
		   updated_at = NOW()`,
		normalized.CharacterID,
		normalized.QuestID,
		string(normalized.Status),
		normalized.Progress,
	); err != nil {
		return nil, mapPostgresError(err)
	}

	if err := grantCharacterInventoryItemTx(ctx, tx, normalized.CharacterID, rewardTemplateID, rewardQuantity); err != nil {
		return nil, err
	}

	items, err := listCharacterItemsTx(ctx, tx, normalized.CharacterID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (p *postgresStoreBackend) EquipItem(ctx context.Context, characterID string, itemID string) ([]CharacterItem, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	item, err := scanCharacterItemRow(tx.QueryRowContext(
		ctx,
		`SELECT item_instance_id, character_id, template_id, quantity, container_kind, COALESCE(equip_slot, ''), COALESCE(instance_attributes_json::text, '{}')
		 FROM character_items
		 WHERE character_id = $1
		   AND item_instance_id = $2
		 FOR UPDATE`,
		characterID,
		itemID,
	))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errItemNotFound
		}
		return nil, err
	}
	if item.ContainerKind != itemContainerInventory {
		return nil, errItemNotFound
	}

	slot, equipable := itemTemplateEquipSlot(item.TemplateID)
	if !equipable {
		return nil, errItemNotEquippable
	}

	var equippedItemID string
	err = tx.QueryRowContext(
		ctx,
		`SELECT item_instance_id
		 FROM character_items
		 WHERE character_id = $1
		   AND container_kind = $2
		   AND equip_slot = $3
		 FOR UPDATE`,
		characterID,
		string(itemContainerEquipment),
		string(slot),
	).Scan(&equippedItemID)
	switch {
	case err == nil:
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE character_items
			 SET container_kind = $3,
			     equip_slot = NULL,
			     updated_at = NOW()
			 WHERE item_instance_id = $1
			   AND character_id = $2`,
			equippedItemID,
			characterID,
			string(itemContainerInventory),
		); err != nil {
			return nil, err
		}
	case errors.Is(err, sql.ErrNoRows):
	default:
		return nil, err
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE character_items
		 SET container_kind = $3,
		     equip_slot = $4,
		     updated_at = NOW()
		 WHERE item_instance_id = $1
		   AND character_id = $2`,
		item.ID,
		characterID,
		string(itemContainerEquipment),
		string(slot),
	); err != nil {
		return nil, mapPostgresError(err)
	}

	items, err := listCharacterItemsTx(ctx, tx, characterID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (p *postgresStoreBackend) UnequipItem(ctx context.Context, characterID string, slot EquipSlot) ([]CharacterItem, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(
		ctx,
		`UPDATE character_items
		 SET container_kind = $3,
		     equip_slot = NULL,
		     updated_at = NOW()
		 WHERE character_id = $1
		   AND container_kind = $2
		   AND equip_slot = $4`,
		characterID,
		string(itemContainerEquipment),
		string(itemContainerInventory),
		string(slot),
	)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, errItemNotEquipped
	}

	items, err := listCharacterItemsTx(ctx, tx, characterID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (p *postgresStoreBackend) SplitStack(ctx context.Context, characterID string, itemID string, quantity int) ([]CharacterItem, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	item, err := scanCharacterItemRow(tx.QueryRowContext(
		ctx,
		`SELECT item_instance_id, character_id, template_id, quantity, container_kind, COALESCE(equip_slot, ''), COALESCE(instance_attributes_json::text, '{}')
		 FROM character_items
		 WHERE character_id = $1
		   AND item_instance_id = $2
		 FOR UPDATE`,
		characterID,
		itemID,
	))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errItemNotFound
		}
		return nil, err
	}
	if item.ContainerKind != itemContainerInventory {
		return nil, errItemNotFound
	}
	if !itemTemplateIsStackable(item.TemplateID) {
		return nil, errItemNotStackable
	}
	if quantity <= 0 || item.Quantity <= quantity {
		return nil, errInvalidSplitQuantity
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE character_items
		 SET quantity = $3,
		     updated_at = NOW()
		 WHERE item_instance_id = $1
		   AND character_id = $2`,
		item.ID,
		characterID,
		item.Quantity-quantity,
	); err != nil {
		return nil, mapPostgresError(err)
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO character_items (item_instance_id, character_id, template_id, quantity, container_kind, equip_slot, instance_attributes_json)
		 VALUES ($1, $2, $3, $4, $5, NULL, $6::jsonb)`,
		randomID("item"),
		characterID,
		item.TemplateID,
		quantity,
		string(itemContainerInventory),
		encodeItemInstanceAttributesJSON(item.InstanceAttributes),
	); err != nil {
		return nil, mapPostgresError(err)
	}

	items, err := listCharacterItemsTx(ctx, tx, characterID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (p *postgresStoreBackend) MergeStacks(ctx context.Context, characterID string, sourceItemID string, targetItemID string) ([]CharacterItem, error) {
	if sourceItemID == targetItemID {
		return nil, errItemMergeInvalid
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	loadItem := func(itemID string) (*CharacterItem, error) {
		item, err := scanCharacterItemRow(tx.QueryRowContext(
			ctx,
			`SELECT item_instance_id, character_id, template_id, quantity, container_kind, COALESCE(equip_slot, ''), COALESCE(instance_attributes_json::text, '{}')
			 FROM character_items
			 WHERE character_id = $1
			   AND item_instance_id = $2
			 FOR UPDATE`,
			characterID,
			itemID,
		))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, errItemNotFound
			}
			return nil, err
		}
		return &item, nil
	}

	firstID := sourceItemID
	secondID := targetItemID
	if secondID < firstID {
		firstID, secondID = secondID, firstID
	}

	firstItem, err := loadItem(firstID)
	if err != nil {
		return nil, err
	}
	secondItem, err := loadItem(secondID)
	if err != nil {
		return nil, err
	}

	sourceItem := firstItem
	targetItem := secondItem
	if sourceItem.ID != sourceItemID {
		sourceItem = secondItem
		targetItem = firstItem
	}
	if sourceItem.ContainerKind != itemContainerInventory || targetItem.ContainerKind != itemContainerInventory {
		return nil, errItemMergeInvalid
	}
	if sourceItem.TemplateID != targetItem.TemplateID {
		return nil, errItemMergeInvalid
	}
	if !itemTemplateIsStackable(sourceItem.TemplateID) {
		return nil, errItemNotStackable
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE character_items
		 SET quantity = $3,
		     updated_at = NOW()
		 WHERE item_instance_id = $1
		   AND character_id = $2`,
		targetItem.ID,
		characterID,
		targetItem.Quantity+sourceItem.Quantity,
	); err != nil {
		return nil, mapPostgresError(err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`DELETE FROM character_items
		 WHERE item_instance_id = $1
		   AND character_id = $2`,
		sourceItem.ID,
		characterID,
	); err != nil {
		return nil, mapPostgresError(err)
	}

	items, err := listCharacterItemsTx(ctx, tx, characterID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (p *postgresStoreBackend) UseConsumable(ctx context.Context, characterID string, itemID string) ([]CharacterItem, CharacterItem, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, CharacterItem{}, err
	}
	defer tx.Rollback()

	item, err := scanCharacterItemRow(tx.QueryRowContext(
		ctx,
		`SELECT item_instance_id, character_id, template_id, quantity, container_kind, COALESCE(equip_slot, ''), COALESCE(instance_attributes_json::text, '{}')
		 FROM character_items
		 WHERE character_id = $1
		   AND item_instance_id = $2
		 FOR UPDATE`,
		characterID,
		itemID,
	))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, CharacterItem{}, errItemNotFound
		}
		return nil, CharacterItem{}, err
	}
	if item.ContainerKind != itemContainerInventory {
		return nil, CharacterItem{}, errItemNotFound
	}
	if !itemTemplateIsConsumable(item.TemplateID) {
		return nil, CharacterItem{}, errItemNotConsumable
	}

	if item.Quantity <= 1 {
		if _, err := tx.ExecContext(
			ctx,
			`DELETE FROM character_items
			 WHERE item_instance_id = $1
			   AND character_id = $2`,
			item.ID,
			characterID,
		); err != nil {
			return nil, CharacterItem{}, mapPostgresError(err)
		}
	} else {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE character_items
			 SET quantity = $3,
			     updated_at = NOW()
			 WHERE item_instance_id = $1
			   AND character_id = $2`,
			item.ID,
			characterID,
			item.Quantity-1,
		); err != nil {
			return nil, CharacterItem{}, mapPostgresError(err)
		}
	}

	items, err := listCharacterItemsTx(ctx, tx, characterID)
	if err != nil {
		return nil, CharacterItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return nil, CharacterItem{}, err
	}
	return items, item, nil
}

func (p *postgresStoreBackend) BuyVendorOffer(ctx context.Context, characterID string, offer VendorOffer, quantity int) ([]CharacterItem, error) {
	if quantity <= 0 || offer.ID == "" || offer.TemplateID == "" || offer.PriceCurrencyTemplateID == "" || offer.PriceAmount <= 0 || offer.Quantity <= 0 {
		return nil, errVendorOfferNotFound
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	accountID, err := accountIDForCharacterTx(ctx, tx, characterID)
	if err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(
		ctx,
		`SELECT item_instance_id, character_id, template_id, quantity, container_kind, COALESCE(equip_slot, ''), COALESCE(instance_attributes_json::text, '{}')
		 FROM character_items
		 WHERE character_id = $1
		 ORDER BY container_kind, equip_slot, template_id, item_instance_id
		 FOR UPDATE`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items, err := scanCharacterItems(rows)
	if err != nil {
		return nil, err
	}

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
	for _, item := range items {
		if remainingCost <= 0 || item.ContainerKind != itemContainerInventory || item.TemplateID != offer.PriceCurrencyTemplateID {
			continue
		}
		if item.Quantity <= remainingCost {
			remainingCost -= item.Quantity
			if _, err := tx.ExecContext(
				ctx,
				`DELETE FROM character_items
				 WHERE item_instance_id = $1
				   AND character_id = $2`,
				item.ID,
				characterID,
			); err != nil {
				return nil, mapPostgresError(err)
			}
			continue
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE character_items
			 SET quantity = $3,
			     updated_at = NOW()
			 WHERE item_instance_id = $1
			   AND character_id = $2`,
			item.ID,
			characterID,
			item.Quantity-remainingCost,
		); err != nil {
			return nil, mapPostgresError(err)
		}
		remainingCost = 0
	}

	totalGranted := offer.Quantity * quantity
	purchasedItemID := ""
	purchasedQuantityAfter := 0
	if itemTemplateIsStackable(offer.TemplateID) {
		targetStackID := ""
		targetQuantity := 0
		for _, item := range items {
			if item.ContainerKind == itemContainerInventory && item.TemplateID == offer.TemplateID {
				targetStackID = item.ID
				targetQuantity = item.Quantity
				break
			}
		}
		if targetStackID != "" {
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE character_items
				 SET quantity = $3,
				     updated_at = NOW()
				 WHERE item_instance_id = $1
				   AND character_id = $2`,
				targetStackID,
				characterID,
				targetQuantity+totalGranted,
			); err != nil {
				return nil, mapPostgresError(err)
			}
			purchasedItemID = targetStackID
			purchasedQuantityAfter = targetQuantity + totalGranted
		} else {
			purchasedItemID = randomID("item")
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO character_items (item_instance_id, character_id, template_id, quantity, container_kind, equip_slot, instance_attributes_json)
				 VALUES ($1, $2, $3, $4, $5, NULL, $6::jsonb)`,
				purchasedItemID,
				characterID,
				offer.TemplateID,
				totalGranted,
				string(itemContainerInventory),
				encodeItemInstanceAttributesJSON(nil),
			); err != nil {
				return nil, mapPostgresError(err)
			}
			purchasedQuantityAfter = totalGranted
		}
	} else {
		for count := 0; count < totalGranted; count++ {
			nextItemID := randomID("item")
			if purchasedItemID == "" {
				purchasedItemID = nextItemID
			}
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO character_items (item_instance_id, character_id, template_id, quantity, container_kind, equip_slot, instance_attributes_json)
				 VALUES ($1, $2, $3, $4, $5, NULL, $6::jsonb)`,
				nextItemID,
				characterID,
				offer.TemplateID,
				1,
				string(itemContainerInventory),
				encodeItemInstanceAttributesJSON(nil),
			); err != nil {
				return nil, mapPostgresError(err)
			}
		}
		purchasedQuantityAfter = 1
	}

	if err := recordActionLogTx(ctx, tx, applyActionLogAuditFromContext(ctx, accountID, ActionLogRecord{
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
	})); err != nil {
		return nil, err
	}

	items, err = listCharacterItemsTx(ctx, tx, characterID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (p *postgresStoreBackend) ExchangeOffer(ctx context.Context, characterID string, offer ExchangeOffer, quantity int) ([]CharacterItem, error) {
	if quantity <= 0 || offer.ID == "" || offer.TemplateID == "" || offer.CostTemplateID == "" || offer.CostAmount <= 0 || offer.Quantity <= 0 {
		return nil, errExchangeOfferNotFound
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	accountID, err := accountIDForCharacterTx(ctx, tx, characterID)
	if err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(
		ctx,
		`SELECT item_instance_id, character_id, template_id, quantity, container_kind, COALESCE(equip_slot, ''), COALESCE(instance_attributes_json::text, '{}')
		 FROM character_items
		 WHERE character_id = $1
		 ORDER BY container_kind, equip_slot, template_id, item_instance_id
		 FOR UPDATE`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items, err := scanCharacterItems(rows)
	if err != nil {
		return nil, err
	}

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
	for _, item := range items {
		if remainingCost <= 0 || item.ContainerKind != itemContainerInventory || item.TemplateID != offer.CostTemplateID {
			continue
		}
		if item.Quantity <= remainingCost {
			remainingCost -= item.Quantity
			if _, err := tx.ExecContext(
				ctx,
				`DELETE FROM character_items
				 WHERE item_instance_id = $1
				   AND character_id = $2`,
				item.ID,
				characterID,
			); err != nil {
				return nil, mapPostgresError(err)
			}
			continue
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE character_items
			 SET quantity = $3,
			     updated_at = NOW()
			 WHERE item_instance_id = $1
			   AND character_id = $2`,
			item.ID,
			characterID,
			item.Quantity-remainingCost,
		); err != nil {
			return nil, mapPostgresError(err)
		}
		remainingCost = 0
	}

	totalGranted := offer.Quantity * quantity
	rewardItemID := ""
	rewardQuantityAfter := 0
	if itemTemplateIsStackable(offer.TemplateID) {
		targetStackID := ""
		targetQuantity := 0
		for _, item := range items {
			if item.ContainerKind == itemContainerInventory && item.TemplateID == offer.TemplateID {
				targetStackID = item.ID
				targetQuantity = item.Quantity
				break
			}
		}
		if targetStackID != "" {
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE character_items
				 SET quantity = $3,
				     updated_at = NOW()
				 WHERE item_instance_id = $1
				   AND character_id = $2`,
				targetStackID,
				characterID,
				targetQuantity+totalGranted,
			); err != nil {
				return nil, mapPostgresError(err)
			}
			rewardItemID = targetStackID
			rewardQuantityAfter = targetQuantity + totalGranted
		} else {
			rewardItemID = randomID("item")
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO character_items (item_instance_id, character_id, template_id, quantity, container_kind, equip_slot, instance_attributes_json)
				 VALUES ($1, $2, $3, $4, $5, NULL, $6::jsonb)`,
				rewardItemID,
				characterID,
				offer.TemplateID,
				totalGranted,
				string(itemContainerInventory),
				encodeItemInstanceAttributesJSON(nil),
			); err != nil {
				return nil, mapPostgresError(err)
			}
			rewardQuantityAfter = totalGranted
		}
	} else {
		for count := 0; count < totalGranted; count++ {
			nextItemID := randomID("item")
			if rewardItemID == "" {
				rewardItemID = nextItemID
			}
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO character_items (item_instance_id, character_id, template_id, quantity, container_kind, equip_slot, instance_attributes_json)
				 VALUES ($1, $2, $3, $4, $5, NULL, $6::jsonb)`,
				nextItemID,
				characterID,
				offer.TemplateID,
				1,
				string(itemContainerInventory),
				encodeItemInstanceAttributesJSON(nil),
			); err != nil {
				return nil, mapPostgresError(err)
			}
		}
		rewardQuantityAfter = 1
	}

	if err := recordActionLogTx(ctx, tx, applyActionLogAuditFromContext(ctx, accountID, ActionLogRecord{
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
	})); err != nil {
		return nil, err
	}

	items, err = listCharacterItemsTx(ctx, tx, characterID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (p *postgresStoreBackend) SellVendorItem(ctx context.Context, characterID string, itemID string, quantity int) ([]CharacterItem, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	accountID, err := accountIDForCharacterTx(ctx, tx, characterID)
	if err != nil {
		return nil, err
	}

	sourceItem, err := scanCharacterItemRow(tx.QueryRowContext(
		ctx,
		`SELECT item_instance_id, character_id, template_id, quantity, container_kind, COALESCE(equip_slot, ''), COALESCE(instance_attributes_json::text, '{}')
		 FROM character_items
		 WHERE character_id = $1
		   AND item_instance_id = $2
		 FOR UPDATE`,
		characterID,
		itemID,
	))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errItemNotFound
		}
		return nil, err
	}
	if sourceItem.ContainerKind != itemContainerInventory {
		return nil, errItemNotFound
	}
	if quantity <= 0 {
		return nil, errInvalidSplitQuantity
	}

	sellValue, sellable := vendorSellValue(sourceItem.TemplateID)
	if !sellable || sellValue.Amount <= 0 || sellValue.CurrencyTemplateID == "" {
		return nil, errItemNotSellable
	}

	if !itemTemplateIsStackable(sourceItem.TemplateID) {
		if quantity != 1 || sourceItem.Quantity != 1 {
			return nil, errInvalidSplitQuantity
		}
		if _, err := tx.ExecContext(
			ctx,
			`DELETE FROM character_items
			 WHERE item_instance_id = $1
			   AND character_id = $2`,
			sourceItem.ID,
			characterID,
		); err != nil {
			return nil, mapPostgresError(err)
		}
	} else {
		if quantity > sourceItem.Quantity {
			return nil, errInvalidSplitQuantity
		}
		if quantity == sourceItem.Quantity {
			if _, err := tx.ExecContext(
				ctx,
				`DELETE FROM character_items
				 WHERE item_instance_id = $1
				   AND character_id = $2`,
				sourceItem.ID,
				characterID,
			); err != nil {
				return nil, mapPostgresError(err)
			}
		} else {
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE character_items
				 SET quantity = $3,
				     updated_at = NOW()
				 WHERE item_instance_id = $1
				   AND character_id = $2`,
				sourceItem.ID,
				characterID,
				sourceItem.Quantity-quantity,
			); err != nil {
				return nil, mapPostgresError(err)
			}
		}
	}

	totalValue := sellValue.Amount * quantity
	currencyStackID := ""
	currencyStackQuantity := 0
	currencyRow := tx.QueryRowContext(
		ctx,
		`SELECT item_instance_id, quantity
		 FROM character_items
		 WHERE character_id = $1
		   AND container_kind = $2
		   AND template_id = $3
		 ORDER BY item_instance_id
		 LIMIT 1
		 FOR UPDATE`,
		characterID,
		string(itemContainerInventory),
		sellValue.CurrencyTemplateID,
	)
	switch err := currencyRow.Scan(&currencyStackID, &currencyStackQuantity); {
	case errors.Is(err, sql.ErrNoRows):
		currencyStackID = ""
	case err != nil:
		return nil, err
	}

	if currencyStackID != "" {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE character_items
			 SET quantity = $3,
			     updated_at = NOW()
			 WHERE item_instance_id = $1
			   AND character_id = $2`,
			currencyStackID,
			characterID,
			currencyStackQuantity+totalValue,
		); err != nil {
			return nil, mapPostgresError(err)
		}
	} else {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO character_items (item_instance_id, character_id, template_id, quantity, container_kind, equip_slot, instance_attributes_json)
			 VALUES ($1, $2, $3, $4, $5, NULL, $6::jsonb)`,
			randomID("item"),
			characterID,
			sellValue.CurrencyTemplateID,
			totalValue,
			string(itemContainerInventory),
			encodeItemInstanceAttributesJSON(nil),
		); err != nil {
			return nil, mapPostgresError(err)
		}
	}

	if err := recordActionLogTx(ctx, tx, applyActionLogAuditFromContext(ctx, accountID, ActionLogRecord{
		ID:                    randomID("action"),
		CharacterID:           characterID,
		ActionType:            "vendor_sell",
		CounterpartyEntity:    "npc_merchant",
		ItemInstanceID:        sourceItem.ID,
		TemplateID:            sourceItem.TemplateID,
		Quantity:              quantity,
		ItemQuantityBefore:    sourceItem.Quantity,
		ItemQuantityAfter:     max(0, sourceItem.Quantity-quantity),
		CurrencyTemplateID:    sellValue.CurrencyTemplateID,
		CurrencyAmount:        totalValue,
		CurrencyBalanceBefore: currencyStackQuantity,
		CurrencyBalanceAfter:  currencyStackQuantity + totalValue,
	})); err != nil {
		return nil, err
	}

	items, err := listCharacterItemsTx(ctx, tx, characterID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func storageItemNotFoundError(container ItemContainer) error {
	if container == itemContainerWarehouse {
		return errWarehouseItemNotFound
	}
	return errItemNotFound
}

func transferPostgresItemBetweenContainers(ctx context.Context, tx *sql.Tx, characterID string, itemID string, sourceContainer ItemContainer, targetContainer ItemContainer, quantity int) (CharacterItem, error) {
	sourceItem, err := scanCharacterItemRow(tx.QueryRowContext(
		ctx,
		`SELECT item_instance_id, character_id, template_id, quantity, container_kind, COALESCE(equip_slot, ''), COALESCE(instance_attributes_json::text, '{}')
		 FROM character_items
		 WHERE character_id = $1
		   AND item_instance_id = $2
		 FOR UPDATE`,
		characterID,
		itemID,
	))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CharacterItem{}, storageItemNotFoundError(sourceContainer)
		}
		return CharacterItem{}, err
	}
	if sourceItem.ContainerKind != sourceContainer {
		return CharacterItem{}, storageItemNotFoundError(sourceContainer)
	}
	if quantity <= 0 {
		return CharacterItem{}, errInvalidSplitQuantity
	}

	if !itemTemplateIsStackable(sourceItem.TemplateID) {
		if quantity != 1 || sourceItem.Quantity != 1 {
			return CharacterItem{}, errInvalidSplitQuantity
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE character_items
			 SET container_kind = $3,
			     equip_slot = NULL,
			     updated_at = NOW()
			 WHERE item_instance_id = $1
			   AND character_id = $2`,
			sourceItem.ID,
			characterID,
			string(targetContainer),
		); err != nil {
			return CharacterItem{}, mapPostgresError(err)
		}
		return sourceItem, nil
	}
	if quantity > sourceItem.Quantity {
		return CharacterItem{}, errInvalidSplitQuantity
	}

	targetStackID := ""
	targetStackQuantity := 0
	targetRow := tx.QueryRowContext(
		ctx,
		`SELECT item_instance_id, quantity
		 FROM character_items
		 WHERE character_id = $1
		   AND container_kind = $2
		   AND template_id = $3
		   AND item_instance_id <> $4
		 ORDER BY item_instance_id
		 LIMIT 1
		 FOR UPDATE`,
		characterID,
		string(targetContainer),
		sourceItem.TemplateID,
		sourceItem.ID,
	)
	switch err := targetRow.Scan(&targetStackID, &targetStackQuantity); {
	case errors.Is(err, sql.ErrNoRows):
		targetStackID = ""
	case err != nil:
		return CharacterItem{}, err
	}

	if quantity == sourceItem.Quantity {
		if targetStackID != "" {
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE character_items
				 SET quantity = $3,
				     updated_at = NOW()
				 WHERE item_instance_id = $1
				   AND character_id = $2`,
				targetStackID,
				characterID,
				targetStackQuantity+quantity,
			); err != nil {
				return CharacterItem{}, mapPostgresError(err)
			}
			if _, err := tx.ExecContext(
				ctx,
				`DELETE FROM character_items
				 WHERE item_instance_id = $1
				   AND character_id = $2`,
				sourceItem.ID,
				characterID,
			); err != nil {
				return CharacterItem{}, mapPostgresError(err)
			}
			return sourceItem, nil
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE character_items
			 SET container_kind = $3,
			     equip_slot = NULL,
			     updated_at = NOW()
			 WHERE item_instance_id = $1
			   AND character_id = $2`,
			sourceItem.ID,
			characterID,
			string(targetContainer),
		); err != nil {
			return CharacterItem{}, mapPostgresError(err)
		}
		return sourceItem, nil
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE character_items
		 SET quantity = $3,
		     updated_at = NOW()
		 WHERE item_instance_id = $1
		   AND character_id = $2`,
		sourceItem.ID,
		characterID,
		sourceItem.Quantity-quantity,
	); err != nil {
		return CharacterItem{}, mapPostgresError(err)
	}
	if targetStackID != "" {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE character_items
			 SET quantity = $3,
			     updated_at = NOW()
			 WHERE item_instance_id = $1
			   AND character_id = $2`,
			targetStackID,
			characterID,
			targetStackQuantity+quantity,
		); err != nil {
			return CharacterItem{}, mapPostgresError(err)
		}
		return sourceItem, nil
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO character_items (item_instance_id, character_id, template_id, quantity, container_kind, equip_slot, instance_attributes_json)
		 VALUES ($1, $2, $3, $4, $5, NULL, $6::jsonb)`,
		randomID("item"),
		characterID,
		sourceItem.TemplateID,
		quantity,
		string(targetContainer),
		encodeItemInstanceAttributesJSON(sourceItem.InstanceAttributes),
	); err != nil {
		return CharacterItem{}, mapPostgresError(err)
	}
	return sourceItem, nil
}

func (p *postgresStoreBackend) DepositWarehouseItem(ctx context.Context, characterID string, itemID string, quantity int) ([]CharacterItem, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	accountID, err := accountIDForCharacterTx(ctx, tx, characterID)
	if err != nil {
		return nil, err
	}

	sourceItem, err := transferPostgresItemBetweenContainers(ctx, tx, characterID, itemID, itemContainerInventory, itemContainerWarehouse, quantity)
	if err != nil {
		return nil, err
	}
	if err := recordStorageTransferTx(ctx, tx, applyStorageTransferAuditFromContext(ctx, accountID, StorageTransferRecord{
		ID:                 randomID("transfer"),
		CharacterID:        characterID,
		SourceItemID:       itemID,
		TemplateID:         sourceItem.TemplateID,
		Quantity:           quantity,
		ItemQuantityBefore: sourceItem.Quantity,
		ItemQuantityAfter:  max(0, sourceItem.Quantity-quantity),
		FromContainerKind:  itemContainerInventory,
		ToContainerKind:    itemContainerWarehouse,
		TransferType:       "warehouse_deposit",
		CounterpartyEntity: warehouseNPCEntityID,
	})); err != nil {
		return nil, err
	}
	if err := recordActionLogTx(ctx, tx, applyActionLogAuditFromContext(ctx, accountID, ActionLogRecord{
		ID:                 randomID("action"),
		CharacterID:        characterID,
		ActionType:         "warehouse_deposit",
		CounterpartyEntity: warehouseNPCEntityID,
		ItemInstanceID:     itemID,
		TemplateID:         sourceItem.TemplateID,
		Quantity:           quantity,
		ItemQuantityBefore: sourceItem.Quantity,
		ItemQuantityAfter:  max(0, sourceItem.Quantity-quantity),
		FromContainerKind:  itemContainerInventory,
		ToContainerKind:    itemContainerWarehouse,
	})); err != nil {
		return nil, err
	}
	items, err := listCharacterItemsTx(ctx, tx, characterID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (p *postgresStoreBackend) WithdrawWarehouseItem(ctx context.Context, characterID string, itemID string, quantity int) ([]CharacterItem, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	accountID, err := accountIDForCharacterTx(ctx, tx, characterID)
	if err != nil {
		return nil, err
	}

	sourceItem, err := transferPostgresItemBetweenContainers(ctx, tx, characterID, itemID, itemContainerWarehouse, itemContainerInventory, quantity)
	if err != nil {
		return nil, err
	}
	if err := recordStorageTransferTx(ctx, tx, applyStorageTransferAuditFromContext(ctx, accountID, StorageTransferRecord{
		ID:                 randomID("transfer"),
		CharacterID:        characterID,
		SourceItemID:       itemID,
		TemplateID:         sourceItem.TemplateID,
		Quantity:           quantity,
		ItemQuantityBefore: sourceItem.Quantity,
		ItemQuantityAfter:  max(0, sourceItem.Quantity-quantity),
		FromContainerKind:  itemContainerWarehouse,
		ToContainerKind:    itemContainerInventory,
		TransferType:       "warehouse_withdraw",
		CounterpartyEntity: warehouseNPCEntityID,
	})); err != nil {
		return nil, err
	}
	if err := recordActionLogTx(ctx, tx, applyActionLogAuditFromContext(ctx, accountID, ActionLogRecord{
		ID:                 randomID("action"),
		CharacterID:        characterID,
		ActionType:         "warehouse_withdraw",
		CounterpartyEntity: warehouseNPCEntityID,
		ItemInstanceID:     itemID,
		TemplateID:         sourceItem.TemplateID,
		Quantity:           quantity,
		ItemQuantityBefore: sourceItem.Quantity,
		ItemQuantityAfter:  max(0, sourceItem.Quantity-quantity),
		FromContainerKind:  itemContainerWarehouse,
		ToContainerKind:    itemContainerInventory,
	})); err != nil {
		return nil, err
	}
	items, err := listCharacterItemsTx(ctx, tx, characterID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func tradePostgresInventoryItemBetweenCharacters(
	ctx context.Context,
	tx *sql.Tx,
	sourceCharacterID string,
	targetCharacterID string,
	itemID string,
	quantity int,
) (CharacterItem, string, int, error) {
	firstCharacterID := sourceCharacterID
	secondCharacterID := targetCharacterID
	if secondCharacterID < firstCharacterID {
		firstCharacterID, secondCharacterID = secondCharacterID, firstCharacterID
	}

	rows, err := tx.QueryContext(
		ctx,
		`SELECT item_instance_id, character_id, template_id, quantity, container_kind, COALESCE(equip_slot, ''), COALESCE(instance_attributes_json::text, '{}')
		 FROM character_items
		 WHERE character_id = $1
		    OR character_id = $2
		 ORDER BY character_id, container_kind, equip_slot, template_id, item_instance_id
		 FOR UPDATE`,
		firstCharacterID,
		secondCharacterID,
	)
	if err != nil {
		return CharacterItem{}, "", 0, err
	}
	defer rows.Close()

	items, err := scanCharacterItems(rows)
	if err != nil {
		return CharacterItem{}, "", 0, err
	}

	sourceItems := make([]CharacterItem, 0)
	targetItems := make([]CharacterItem, 0)
	for _, item := range items {
		switch item.CharacterID {
		case sourceCharacterID:
			sourceItems = append(sourceItems, item)
		case targetCharacterID:
			targetItems = append(targetItems, item)
		}
	}

	sourceIndex := -1
	for index := range sourceItems {
		if sourceItems[index].ID == itemID {
			sourceIndex = index
			break
		}
	}
	if sourceIndex == -1 {
		return CharacterItem{}, "", 0, errItemNotFound
	}

	sourceItem := sourceItems[sourceIndex]
	if sourceItem.ContainerKind != itemContainerInventory {
		return CharacterItem{}, "", 0, errItemNotFound
	}
	if quantity <= 0 {
		return CharacterItem{}, "", 0, errInvalidSplitQuantity
	}

	if !itemTemplateIsStackable(sourceItem.TemplateID) {
		if quantity != 1 || sourceItem.Quantity != 1 {
			return CharacterItem{}, "", 0, errInvalidSplitQuantity
		}

		if _, err := tx.ExecContext(
			ctx,
			`UPDATE character_items
			 SET character_id = $2,
			     container_kind = $4,
			     equip_slot = NULL,
			     updated_at = NOW()
			 WHERE item_instance_id = $1
			   AND character_id = $3`,
			sourceItem.ID,
			targetCharacterID,
			sourceCharacterID,
			string(itemContainerInventory),
		); err != nil {
			return CharacterItem{}, "", 0, mapPostgresError(err)
		}
		return sourceItem, sourceItem.ID, 0, nil
	}

	if quantity > sourceItem.Quantity {
		return CharacterItem{}, "", 0, errInvalidSplitQuantity
	}

	targetStackID := ""
	targetStackQuantity := 0
	for _, item := range targetItems {
		if item.ContainerKind == itemContainerInventory && item.TemplateID == sourceItem.TemplateID && item.EquipSlot == "" {
			targetStackID = item.ID
			targetStackQuantity = item.Quantity
			break
		}
	}

	if quantity == sourceItem.Quantity {
		if targetStackID != "" {
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE character_items
				 SET quantity = $3,
				     updated_at = NOW()
				 WHERE item_instance_id = $1
				   AND character_id = $2`,
				targetStackID,
				targetCharacterID,
				targetStackQuantity+quantity,
			); err != nil {
				return CharacterItem{}, "", 0, mapPostgresError(err)
			}
			if _, err := tx.ExecContext(
				ctx,
				`DELETE FROM character_items
				 WHERE item_instance_id = $1
				   AND character_id = $2`,
				sourceItem.ID,
				sourceCharacterID,
			); err != nil {
				return CharacterItem{}, "", 0, mapPostgresError(err)
			}
			return sourceItem, targetStackID, targetStackQuantity, nil
		}

		if _, err := tx.ExecContext(
			ctx,
			`UPDATE character_items
			 SET character_id = $2,
			     container_kind = $4,
			     equip_slot = NULL,
			     updated_at = NOW()
			 WHERE item_instance_id = $1
			   AND character_id = $3`,
			sourceItem.ID,
			targetCharacterID,
			sourceCharacterID,
			string(itemContainerInventory),
		); err != nil {
			return CharacterItem{}, "", 0, mapPostgresError(err)
		}
		return sourceItem, sourceItem.ID, 0, nil
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE character_items
		 SET quantity = $3,
		     updated_at = NOW()
		 WHERE item_instance_id = $1
		   AND character_id = $2`,
		sourceItem.ID,
		sourceCharacterID,
		sourceItem.Quantity-quantity,
	); err != nil {
		return CharacterItem{}, "", 0, mapPostgresError(err)
	}

	if targetStackID != "" {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE character_items
			 SET quantity = $3,
			     updated_at = NOW()
			 WHERE item_instance_id = $1
			   AND character_id = $2`,
			targetStackID,
			targetCharacterID,
			targetStackQuantity+quantity,
		); err != nil {
			return CharacterItem{}, "", 0, mapPostgresError(err)
		}
		return sourceItem, targetStackID, targetStackQuantity, nil
	}

	targetItemID := randomID("item")
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO character_items (item_instance_id, character_id, template_id, quantity, container_kind, equip_slot, instance_attributes_json)
		 VALUES ($1, $2, $3, $4, $5, NULL, $6::jsonb)`,
		targetItemID,
		targetCharacterID,
		sourceItem.TemplateID,
		quantity,
		string(itemContainerInventory),
		encodeItemInstanceAttributesJSON(sourceItem.InstanceAttributes),
	); err != nil {
		return CharacterItem{}, "", 0, mapPostgresError(err)
	}
	return sourceItem, targetItemID, 0, nil
}

func (p *postgresStoreBackend) TradeInventoryItem(
	ctx context.Context,
	sourceCharacterID string,
	targetCharacterID string,
	itemID string,
	quantity int,
	referenceID string,
) ([]CharacterItem, []CharacterItem, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	sourceAccountID, err := accountIDForCharacterTx(ctx, tx, sourceCharacterID)
	if err != nil {
		return nil, nil, err
	}
	targetAccountID, err := accountIDForCharacterTx(ctx, tx, targetCharacterID)
	if err != nil {
		return nil, nil, err
	}

	sourceItem, targetItemID, targetQuantityBefore, err := tradePostgresInventoryItemBetweenCharacters(
		ctx,
		tx,
		sourceCharacterID,
		targetCharacterID,
		itemID,
		quantity,
	)
	if err != nil {
		return nil, nil, err
	}

	sourceItems, err := listCharacterItemsTx(ctx, tx, sourceCharacterID)
	if err != nil {
		return nil, nil, err
	}
	targetItems, err := listCharacterItemsTx(ctx, tx, targetCharacterID)
	if err != nil {
		return nil, nil, err
	}
	sourceQuantityAfter := 0
	for _, item := range sourceItems {
		if item.ID == sourceItem.ID {
			sourceQuantityAfter = item.Quantity
			break
		}
	}
	targetQuantityAfter := 0
	for _, item := range targetItems {
		if item.ID == targetItemID {
			targetQuantityAfter = item.Quantity
			break
		}
	}
	if err := recordActionLogTx(ctx, tx, applyActionLogAuditFromContext(ctx, targetAccountID, ActionLogRecord{
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
	})); err != nil {
		return nil, nil, err
	}
	if err := recordActionLogTx(ctx, tx, applyActionLogAuditFromContext(ctx, sourceAccountID, ActionLogRecord{
		ID:                 randomID("action"),
		CharacterID:        sourceCharacterID,
		ActionType:         "player_trade_send",
		ReferenceID:        referenceID,
		CounterpartyEntity: targetCharacterID,
		ItemInstanceID:     sourceItem.ID,
		TemplateID:         sourceItem.TemplateID,
		Quantity:           quantity,
		ItemQuantityBefore: sourceItem.Quantity,
		ItemQuantityAfter:  sourceQuantityAfter,
	})); err != nil {
		return nil, nil, err
	}
	if err := recordActionLogTx(ctx, tx, applyActionLogAuditFromContext(ctx, targetAccountID, ActionLogRecord{
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
	})); err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return sourceItems, targetItems, nil
}

func listCharacterItemsTx(ctx context.Context, tx *sql.Tx, characterID string) ([]CharacterItem, error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT item_instance_id, character_id, template_id, quantity, container_kind, COALESCE(equip_slot, ''), COALESCE(instance_attributes_json::text, '{}')
		 FROM character_items
		 WHERE character_id = $1
		 ORDER BY container_kind, equip_slot, template_id, item_instance_id`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanCharacterItems(rows)
}

func scanCharacterItems(rows *sql.Rows) ([]CharacterItem, error) {
	items := make([]CharacterItem, 0)
	for rows.Next() {
		item, err := scanCharacterItemRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (p *postgresStoreBackend) CreateSession(ctx context.Context, session *Session) error {
	_, err := p.db.ExecContext(
		ctx,
		`INSERT INTO gameplay_sessions (session_id, account_id, character_id, attach_token, status, attach_expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		session.ID,
		session.AccountID,
		session.CharacterID,
		session.AttachToken,
		string(session.Status),
		session.AttachExpiresAt,
	)
	return mapPostgresError(err)
}

func (p *postgresStoreBackend) GetByIDSession(ctx context.Context, sessionID string) (*Session, error) {
	row := p.db.QueryRowContext(
		ctx,
		`SELECT session_id, account_id, character_id, attach_token, status, attach_expires_at
		 FROM gameplay_sessions
		 WHERE session_id = $1`,
		sessionID,
	)
	session := &Session{}
	var status string
	if err := row.Scan(&session.ID, &session.AccountID, &session.CharacterID, &session.AttachToken, &status, &session.AttachExpiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	session.Status = SessionStatus(status)
	return session, nil
}

func (p *postgresStoreBackend) GetLatestPendingForCharacter(ctx context.Context, characterID string, now time.Time) (*Session, error) {
	row := p.db.QueryRowContext(
		ctx,
		`SELECT session_id, account_id, character_id, attach_token, status, attach_expires_at
		 FROM gameplay_sessions
		 WHERE character_id = $1
		   AND status = $2
		   AND attach_expires_at > $3
		 ORDER BY attach_expires_at DESC, session_id DESC
		 LIMIT 1`,
		characterID,
		string(sessionStatusPendingAttach),
		now,
	)
	session := &Session{}
	var status string
	if err := row.Scan(&session.ID, &session.AccountID, &session.CharacterID, &session.AttachToken, &status, &session.AttachExpiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	session.Status = SessionStatus(status)
	return session, nil
}

func (p *postgresStoreBackend) HasAttachedForCharacter(ctx context.Context, characterID string) (bool, error) {
	row := p.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*)
		 FROM gameplay_sessions
		 WHERE character_id = $1
		   AND status = $2`,
		characterID,
		string(sessionStatusAttached),
	)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (p *postgresStoreBackend) ExpireStalePendingAttach(ctx context.Context, characterID string, now time.Time) error {
	_, err := p.db.ExecContext(
		ctx,
		`UPDATE gameplay_sessions
		 SET status = $3, updated_at = NOW()
		 WHERE character_id = $1
		   AND status = $2
		   AND attach_expires_at <= $4`,
		characterID,
		string(sessionStatusPendingAttach),
		string(sessionStatusExpired),
		now,
	)
	return err
}

func (p *postgresStoreBackend) SanitizeStartupLifecycle(ctx context.Context, now time.Time) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE gameplay_sessions
		 SET status = $2, updated_at = NOW()
		 WHERE status = $1`,
		string(sessionStatusAttached),
		string(sessionStatusClosed),
	); err != nil {
		return err
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE gameplay_sessions
		 SET status = $2, updated_at = NOW()
		 WHERE status = $1
		   AND attach_expires_at <= $3`,
		string(sessionStatusPendingAttach),
		string(sessionStatusExpired),
		now,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (p *postgresStoreBackend) UpdateStatus(ctx context.Context, sessionID string, status SessionStatus) error {
	result, err := p.db.ExecContext(
		ctx,
		`UPDATE gameplay_sessions SET status = $2, updated_at = NOW() WHERE session_id = $1`,
		sessionID,
		string(status),
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errRecordNotFound
	}
	return nil
}

func (p *postgresStoreBackend) truncateAllTables(ctx context.Context) error {
	_, err := p.db.ExecContext(ctx, `TRUNCATE TABLE gameplay_command_records, clan_invites, clan_members, clans, party_invites, party_members, parties, chat_messages, account_sessions, gameplay_sessions, action_logs, storage_transfer_records, character_hotbar_loadouts, character_skill_cooldowns, character_items, character_quests, character_pets, characters, account_credentials, accounts RESTART IDENTITY CASCADE`)
	return err
}

func mapPostgresError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return errRecordConflict
	}
	return err
}

type postgresAccountRepo struct{ backend *postgresStoreBackend }
type postgresCredentialRepo struct{ backend *postgresStoreBackend }
type postgresAccountSessionRepo struct{ backend *postgresStoreBackend }
type postgresGameplayCommandRecordRepo struct{ backend *postgresStoreBackend }
type postgresCharacterRepo struct{ backend *postgresStoreBackend }
type postgresCharacterCooldownRepo struct{ backend *postgresStoreBackend }
type postgresCharacterHotbarRepo struct{ backend *postgresStoreBackend }
type postgresPartyRepo struct{ backend *postgresStoreBackend }
type postgresCharacterQuestRepo struct{ backend *postgresStoreBackend }
type postgresCharacterItemRepo struct{ backend *postgresStoreBackend }
type postgresStorageTransferRecordRepo struct{ backend *postgresStoreBackend }
type postgresActionLogRepo struct{ backend *postgresStoreBackend }
type postgresGameplaySessionRepo struct{ backend *postgresStoreBackend }

func (repo postgresAccountRepo) Create(ctx context.Context, account *Account) error {
	return repo.backend.Create(ctx, account)
}

func (repo postgresAccountRepo) GetByID(ctx context.Context, accountID string) (*Account, error) {
	return repo.backend.GetByID(ctx, accountID)
}

func (repo postgresAccountRepo) GetByLogin(ctx context.Context, login string) (*Account, error) {
	return repo.backend.GetByLogin(ctx, login)
}

func (repo postgresCredentialRepo) Create(ctx context.Context, credential *CredentialRecord) error {
	return repo.backend.CreateCredential(ctx, credential)
}

func (repo postgresCredentialRepo) GetByAccountID(ctx context.Context, accountID string) (*CredentialRecord, error) {
	return repo.backend.GetByAccountID(ctx, accountID)
}

func (repo postgresCredentialRepo) Update(ctx context.Context, credential *CredentialRecord) error {
	return repo.backend.UpdateCredential(ctx, credential)
}

func (repo postgresAccountSessionRepo) Create(ctx context.Context, session *AccountSession) error {
	return repo.backend.CreateAccountSession(ctx, session)
}

func (repo postgresAccountSessionRepo) GetActiveByToken(ctx context.Context, token string, now time.Time) (*AccountSession, error) {
	return repo.backend.GetActiveAccountSessionByToken(ctx, token, now)
}

func (repo postgresAccountSessionRepo) RevokeByToken(ctx context.Context, token string, now time.Time) error {
	return repo.backend.RevokeAccountSessionByToken(ctx, token, now)
}

func (repo postgresGameplayCommandRecordRepo) CreatePending(ctx context.Context, record *GameplayCommandRecord) error {
	return repo.backend.CreateGameplayCommandRecordPending(ctx, record)
}

func (repo postgresGameplayCommandRecordRepo) GetBySessionAndSeq(ctx context.Context, sessionID string, commandSeq int) (*GameplayCommandRecord, error) {
	return repo.backend.GetGameplayCommandRecordBySessionAndSeq(ctx, sessionID, commandSeq)
}

func (repo postgresGameplayCommandRecordRepo) UpdateOutcome(ctx context.Context, sessionID string, commandSeq int, status GameplayCommandRecordStatus, outboundMessages []map[string]any) error {
	return repo.backend.UpdateGameplayCommandRecordOutcome(ctx, sessionID, commandSeq, status, outboundMessages)
}

func (repo postgresCharacterRepo) ListByAccountID(ctx context.Context, accountID string) ([]Character, error) {
	characters, err := repo.backend.ListByAccountID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	sort.Slice(characters, func(i, j int) bool {
		if characters[i].Name == characters[j].Name {
			return characters[i].ID < characters[j].ID
		}
		return characters[i].Name < characters[j].Name
	})
	return characters, nil
}

func (repo postgresCharacterRepo) CountByAccountID(ctx context.Context, accountID string) (int, error) {
	return repo.backend.CountByAccountID(ctx, accountID)
}

func (repo postgresCharacterRepo) GetByID(ctx context.Context, characterID string) (*Character, error) {
	return repo.backend.GetByIDCharacter(ctx, characterID)
}

func (repo postgresCharacterRepo) Create(ctx context.Context, character *Character) error {
	return repo.backend.CreateCharacter(ctx, character)
}

func (repo postgresCharacterRepo) UpdateWorldState(ctx context.Context, characterID string, regionID string, positionX float64, positionZ float64) error {
	return repo.backend.UpdateCharacterWorldState(ctx, characterID, regionID, positionX, positionZ)
}

func (repo postgresCharacterRepo) UpdateProgression(ctx context.Context, characterID string, level int, xp int, currentCP int, currentHP int, currentMP int) error {
	return repo.backend.UpdateCharacterProgression(ctx, characterID, level, xp, currentCP, currentHP, currentMP)
}

func (repo postgresCharacterCooldownRepo) ListByCharacterID(ctx context.Context, characterID string) ([]CharacterSkillCooldown, error) {
	return repo.backend.ListCharacterCooldownsByCharacterID(ctx, characterID)
}

func (repo postgresCharacterCooldownRepo) ReplaceByCharacterID(ctx context.Context, characterID string, cooldowns []CharacterSkillCooldown) error {
	return repo.backend.ReplaceCharacterCooldowns(ctx, characterID, cooldowns)
}

func (repo postgresCharacterHotbarRepo) ListByCharacterID(ctx context.Context, characterID string) (CharacterHotbarState, error) {
	return repo.backend.ListCharacterHotbarStateByCharacterID(ctx, characterID)
}

func (repo postgresCharacterHotbarRepo) ReplaceByCharacterID(ctx context.Context, characterID string, hotbar CharacterHotbarState) error {
	return repo.backend.ReplaceCharacterHotbarStateByCharacterID(ctx, characterID, hotbar)
}

func (repo postgresCharacterQuestRepo) ListByCharacterID(ctx context.Context, characterID string) ([]CharacterQuestState, error) {
	return repo.backend.ListCharacterQuestsByCharacterID(ctx, characterID)
}

func (repo postgresCharacterQuestRepo) UpsertByCharacterID(ctx context.Context, quest CharacterQuestState) error {
	return repo.backend.UpsertCharacterQuestByCharacterID(ctx, quest)
}

func (repo postgresCharacterQuestRepo) CompleteQuestWithItemReward(
	ctx context.Context,
	quest CharacterQuestState,
	rewardTemplateID string,
	rewardQuantity int,
) ([]CharacterItem, error) {
	return repo.backend.CompleteCharacterQuestWithItemReward(ctx, quest, rewardTemplateID, rewardQuantity)
}

func (repo postgresCharacterItemRepo) ListByCharacterID(ctx context.Context, characterID string) ([]CharacterItem, error) {
	items, err := repo.backend.ListCharacterItemsByCharacterID(ctx, characterID)
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ContainerKind == items[j].ContainerKind {
			if items[i].EquipSlot == items[j].EquipSlot {
				if items[i].TemplateID == items[j].TemplateID {
					return items[i].ID < items[j].ID
				}
				return items[i].TemplateID < items[j].TemplateID
			}
			return items[i].EquipSlot < items[j].EquipSlot
		}
		return items[i].ContainerKind < items[j].ContainerKind
	})
	return items, nil
}

func (repo postgresCharacterItemRepo) PickUpLoot(ctx context.Context, characterID string, lootID string, templateID string, quantity int) ([]CharacterItem, error) {
	return repo.backend.PickUpLoot(ctx, characterID, lootID, templateID, quantity)
}

func (repo postgresCharacterItemRepo) EquipItem(ctx context.Context, characterID string, itemID string) ([]CharacterItem, error) {
	return repo.backend.EquipItem(ctx, characterID, itemID)
}

func (repo postgresCharacterItemRepo) UnequipItem(ctx context.Context, characterID string, slot EquipSlot) ([]CharacterItem, error) {
	return repo.backend.UnequipItem(ctx, characterID, slot)
}

func (repo postgresCharacterItemRepo) SplitStack(ctx context.Context, characterID string, itemID string, quantity int) ([]CharacterItem, error) {
	return repo.backend.SplitStack(ctx, characterID, itemID, quantity)
}

func (repo postgresCharacterItemRepo) MergeStacks(ctx context.Context, characterID string, sourceItemID string, targetItemID string) ([]CharacterItem, error) {
	return repo.backend.MergeStacks(ctx, characterID, sourceItemID, targetItemID)
}

func (repo postgresCharacterItemRepo) UseConsumable(ctx context.Context, characterID string, itemID string) ([]CharacterItem, CharacterItem, error) {
	return repo.backend.UseConsumable(ctx, characterID, itemID)
}

func (repo postgresCharacterItemRepo) BuyVendorOffer(ctx context.Context, characterID string, offer VendorOffer, quantity int) ([]CharacterItem, error) {
	return repo.backend.BuyVendorOffer(ctx, characterID, offer, quantity)
}

func (repo postgresCharacterItemRepo) ExchangeOffer(ctx context.Context, characterID string, offer ExchangeOffer, quantity int) ([]CharacterItem, error) {
	return repo.backend.ExchangeOffer(ctx, characterID, offer, quantity)
}

func (repo postgresCharacterItemRepo) SellVendorItem(ctx context.Context, characterID string, itemID string, quantity int) ([]CharacterItem, error) {
	return repo.backend.SellVendorItem(ctx, characterID, itemID, quantity)
}

func (repo postgresCharacterItemRepo) DepositWarehouseItem(ctx context.Context, characterID string, itemID string, quantity int) ([]CharacterItem, error) {
	return repo.backend.DepositWarehouseItem(ctx, characterID, itemID, quantity)
}

func (repo postgresCharacterItemRepo) WithdrawWarehouseItem(ctx context.Context, characterID string, itemID string, quantity int) ([]CharacterItem, error) {
	return repo.backend.WithdrawWarehouseItem(ctx, characterID, itemID, quantity)
}

func (repo postgresCharacterItemRepo) TradeInventoryItem(
	ctx context.Context,
	sourceCharacterID string,
	targetCharacterID string,
	itemID string,
	quantity int,
	referenceID string,
) ([]CharacterItem, []CharacterItem, error) {
	return repo.backend.TradeInventoryItem(ctx, sourceCharacterID, targetCharacterID, itemID, quantity, referenceID)
}

func (repo postgresStorageTransferRecordRepo) ListByCharacterID(ctx context.Context, characterID string) ([]StorageTransferRecord, error) {
	return repo.backend.ListStorageTransferRecordsByCharacterID(ctx, characterID)
}

func (repo postgresStorageTransferRecordRepo) ListByFilter(ctx context.Context, query StorageTransferQuery) ([]StorageTransferRecord, error) {
	return repo.backend.ListStorageTransferRecordsByFilter(ctx, query)
}

func (repo postgresActionLogRepo) ListByCharacterID(ctx context.Context, characterID string) ([]ActionLogRecord, error) {
	return repo.backend.ListActionLogsByCharacterID(ctx, characterID)
}

func (repo postgresActionLogRepo) ListByFilter(ctx context.Context, query ActionLogQuery) ([]ActionLogRecord, error) {
	return repo.backend.ListActionLogsByFilter(ctx, query)
}

func (repo postgresActionLogRepo) Create(ctx context.Context, record ActionLogRecord) error {
	accountID, err := repo.backend.accountIDForCharacter(ctx, record.CharacterID)
	if err != nil {
		return err
	}
	return insertActionLog(ctx, repo.backend.db, applyActionLogAuditFromContext(ctx, accountID, record))
}

func (repo postgresGameplaySessionRepo) Create(ctx context.Context, session *Session) error {
	return repo.backend.CreateSession(ctx, session)
}

func (repo postgresGameplaySessionRepo) GetByID(ctx context.Context, sessionID string) (*Session, error) {
	return repo.backend.GetByIDSession(ctx, sessionID)
}

func (repo postgresGameplaySessionRepo) GetLatestPendingForCharacter(ctx context.Context, characterID string, now time.Time) (*Session, error) {
	return repo.backend.GetLatestPendingForCharacter(ctx, characterID, now)
}

func (repo postgresGameplaySessionRepo) HasAttachedForCharacter(ctx context.Context, characterID string) (bool, error) {
	return repo.backend.HasAttachedForCharacter(ctx, characterID)
}

func (repo postgresGameplaySessionRepo) ExpireStalePendingAttach(ctx context.Context, characterID string, now time.Time) error {
	return repo.backend.ExpireStalePendingAttach(ctx, characterID, now)
}

func (repo postgresGameplaySessionRepo) SanitizeStartupLifecycle(ctx context.Context, now time.Time) error {
	return repo.backend.SanitizeStartupLifecycle(ctx, now)
}

func (repo postgresGameplaySessionRepo) UpdateStatus(ctx context.Context, sessionID string, status SessionStatus) error {
	return repo.backend.UpdateStatus(ctx, sessionID, status)
}
