package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type postgresExecContext interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func cloneChatMessageRecords(records []ChatMessageRecord) []ChatMessageRecord {
	if len(records) == 0 {
		return nil
	}
	cloned := make([]ChatMessageRecord, len(records))
	copy(cloned, records)
	return cloned
}

func sortChatMessageRecords(records []ChatMessageRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].ID > records[j].ID
		}
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
}

func normalizeChatMessageRecord(record ChatMessageRecord) ChatMessageRecord {
	record.Channel = strings.TrimSpace(strings.ToLower(record.Channel))
	record.AllianceID = strings.TrimSpace(record.AllianceID)
	record.TargetCharacterID = strings.TrimSpace(record.TargetCharacterID)
	record.RegionID = strings.TrimSpace(record.RegionID)
	record.Text = strings.TrimSpace(record.Text)
	record.SessionID = strings.TrimSpace(record.SessionID)
	record.CommandID = strings.TrimSpace(record.CommandID)
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	return record
}

func applyChatMessageAuditFromContext(ctx context.Context, accountID string, record ChatMessageRecord) ChatMessageRecord {
	record = normalizeChatMessageRecord(record)
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

func (repo memoryChatMessageRepo) Create(ctx context.Context, record ChatMessageRecord) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	record = applyChatMessageAuditFromContext(ctx, repo.backend.accountIDForCharacter(record.CharacterID), record)
	repo.backend.chatMessages[record.CharacterID] = append(repo.backend.chatMessages[record.CharacterID], record)
	return nil
}

func (repo memoryChatMessageRepo) ListByCharacterID(_ context.Context, characterID string) ([]ChatMessageRecord, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	records := cloneChatMessageRecords(repo.backend.chatMessages[characterID])
	sortChatMessageRecords(records)
	return records, nil
}

func (repo memoryChatMessageRepo) ListByFilter(_ context.Context, query ChatMessageQuery) ([]ChatMessageRecord, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()

	query.Limit, query.Offset = normalizeAuditPagination(query.Limit, query.Offset)
	records := make([]ChatMessageRecord, 0)
	for characterID, stored := range repo.backend.chatMessages {
		if query.CharacterID != "" && characterID != query.CharacterID {
			continue
		}
		for _, record := range stored {
			if query.AllianceID != "" && record.AllianceID != query.AllianceID {
				continue
			}
			if query.TargetCharacterID != "" && record.TargetCharacterID != query.TargetCharacterID {
				continue
			}
			if query.Channel != "" && record.Channel != query.Channel {
				continue
			}
			if query.RegionID != "" && record.RegionID != query.RegionID {
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
	sortChatMessageRecords(records)
	if query.Offset >= len(records) {
		return []ChatMessageRecord{}, nil
	}
	end := query.Offset + query.Limit
	if end > len(records) {
		end = len(records)
	}
	return cloneChatMessageRecords(records[query.Offset:end]), nil
}

func insertChatMessage(ctx context.Context, exec postgresExecContext, record ChatMessageRecord) error {
	record = normalizeChatMessageRecord(record)
	_, err := exec.ExecContext(
		ctx,
		`INSERT INTO chat_messages (
			chat_message_id,
			character_id,
			account_id,
			channel,
			alliance_id,
			target_character_id,
			region_id,
			text,
			session_id,
			command_id,
			command_seq,
			created_at
		) VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8, NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, 0), $12)`,
		record.ID,
		record.CharacterID,
		record.AccountID,
		record.Channel,
		record.AllianceID,
		record.TargetCharacterID,
		record.RegionID,
		record.Text,
		record.SessionID,
		record.CommandID,
		record.CommandSeq,
		record.CreatedAt,
	)
	return mapPostgresError(err)
}

func scanChatMessageRows(rows *sql.Rows) ([]ChatMessageRecord, error) {
	defer rows.Close()

	records := make([]ChatMessageRecord, 0)
	for rows.Next() {
		var record ChatMessageRecord
		if err := rows.Scan(
			&record.ID,
			&record.CharacterID,
			&record.AccountID,
			&record.Channel,
			&record.AllianceID,
			&record.TargetCharacterID,
			&record.RegionID,
			&record.Text,
			&record.SessionID,
			&record.CommandID,
			&record.CommandSeq,
			&record.CreatedAt,
		); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (p *postgresStoreBackend) ListChatMessagesByCharacterID(ctx context.Context, characterID string) ([]ChatMessageRecord, error) {
	rows, err := p.db.QueryContext(
		ctx,
		`SELECT chat_message_id, character_id, COALESCE(account_id, ''), channel, COALESCE(alliance_id, ''), COALESCE(target_character_id, ''), COALESCE(region_id, ''), text, COALESCE(session_id, ''), COALESCE(command_id, ''), COALESCE(command_seq, 0), created_at
		 FROM chat_messages
		 WHERE character_id = $1
		 ORDER BY created_at DESC, chat_message_id DESC`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	return scanChatMessageRows(rows)
}

func (p *postgresStoreBackend) ListChatMessagesByFilter(ctx context.Context, query ChatMessageQuery) ([]ChatMessageRecord, error) {
	query.Limit, query.Offset = normalizeAuditPagination(query.Limit, query.Offset)

	var baseQuery strings.Builder
	baseQuery.WriteString(
		`SELECT chat_message_id, character_id, COALESCE(account_id, ''), channel, COALESCE(alliance_id, ''), COALESCE(target_character_id, ''), COALESCE(region_id, ''), text, COALESCE(session_id, ''), COALESCE(command_id, ''), COALESCE(command_seq, 0), created_at
		 FROM chat_messages`,
	)
	conditions := make([]string, 0, 7)
	args := make([]any, 0, 8)
	if query.CharacterID != "" {
		args = append(args, query.CharacterID)
		conditions = append(conditions, fmt.Sprintf("character_id = $%d", len(args)))
	}
	if query.AllianceID != "" {
		args = append(args, query.AllianceID)
		conditions = append(conditions, fmt.Sprintf("alliance_id = $%d", len(args)))
	}
	if query.TargetCharacterID != "" {
		args = append(args, query.TargetCharacterID)
		conditions = append(conditions, fmt.Sprintf("target_character_id = $%d", len(args)))
	}
	if query.Channel != "" {
		args = append(args, strings.TrimSpace(strings.ToLower(query.Channel)))
		conditions = append(conditions, fmt.Sprintf("channel = $%d", len(args)))
	}
	if query.RegionID != "" {
		args = append(args, query.RegionID)
		conditions = append(conditions, fmt.Sprintf("region_id = $%d", len(args)))
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
	args = append(args, query.Limit, query.Offset)
	baseQuery.WriteString(fmt.Sprintf(" ORDER BY created_at DESC, chat_message_id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)))

	rows, err := p.db.QueryContext(ctx, baseQuery.String(), args...)
	if err != nil {
		return nil, err
	}
	return scanChatMessageRows(rows)
}

type postgresChatMessageRepo struct{ backend *postgresStoreBackend }

func (repo postgresChatMessageRepo) Create(ctx context.Context, record ChatMessageRecord) error {
	accountID, err := repo.backend.accountIDForCharacter(ctx, record.CharacterID)
	if err != nil && !errors.Is(err, errRecordNotFound) {
		return err
	}
	return insertChatMessage(ctx, repo.backend.db, applyChatMessageAuditFromContext(ctx, accountID, record))
}

func (repo postgresChatMessageRepo) ListByCharacterID(ctx context.Context, characterID string) ([]ChatMessageRecord, error) {
	return repo.backend.ListChatMessagesByCharacterID(ctx, characterID)
}

func (repo postgresChatMessageRepo) ListByFilter(ctx context.Context, query ChatMessageQuery) ([]ChatMessageRecord, error) {
	return repo.backend.ListChatMessagesByFilter(ctx, query)
}
