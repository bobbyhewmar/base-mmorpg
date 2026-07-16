package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

func normalizeGameplayEventReceipt(receipt GameplayEventReceipt) (GameplayEventReceipt, error) {
	receipt.RecipientSessionID = strings.TrimSpace(receipt.RecipientSessionID)
	receipt.RecipientCharacterID = strings.TrimSpace(receipt.RecipientCharacterID)
	receipt.ServerInstanceID = strings.TrimSpace(receipt.ServerInstanceID)
	if receipt.EventID <= 0 || receipt.ServerInstanceID == "" || (receipt.RecipientSessionID == "" && receipt.RecipientCharacterID == "") {
		return GameplayEventReceipt{}, errors.New("invalid gameplay event receipt")
	}
	return receipt, nil
}

func sameGameplayEventReceiptIdentity(left GameplayEventReceipt, right GameplayEventReceipt) bool {
	return left.EventID == right.EventID &&
		left.RecipientSessionID == right.RecipientSessionID &&
		left.RecipientCharacterID == right.RecipientCharacterID &&
		left.ServerInstanceID == right.ServerInstanceID
}

func cloneGameplayEventReceipt(receipt *GameplayEventReceipt) *GameplayEventReceipt {
	if receipt == nil {
		return nil
	}
	clone := *receipt
	return &clone
}

func (repo memoryGameplayEventReceiptRepo) Reserve(_ context.Context, receipt GameplayEventReceipt, claimOwnerID string, now time.Time, claimLease time.Duration) (GameplayEventReceiptReservation, error) {
	normalized, err := normalizeGameplayEventReceipt(receipt)
	claimOwnerID = strings.TrimSpace(claimOwnerID)
	if err != nil || claimOwnerID == "" || claimLease <= 0 {
		if err != nil {
			return GameplayEventReceiptReservation{}, err
		}
		return GameplayEventReceiptReservation{}, errors.New("invalid gameplay event receipt reservation")
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()

	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()
	if _, exists := repo.backend.gameplayEvents[normalized.EventID]; !exists {
		return GameplayEventReceiptReservation{}, errRecordNotFound
	}
	existing := repo.backend.gameplayReceipts[normalized.EventID]
	if existing == nil {
		normalized.ClaimOwnerID = claimOwnerID
		normalized.ClaimDeadlineAt = now.Add(claimLease)
		normalized.CreatedAt = now
		normalized.UpdatedAt = now
		repo.backend.gameplayReceipts[normalized.EventID] = cloneGameplayEventReceipt(&normalized)
		return GameplayEventReceiptReservation{Receipt: normalized, Acquired: true}, nil
	}
	if !sameGameplayEventReceiptIdentity(*existing, normalized) {
		return GameplayEventReceiptReservation{}, errRecordConflict
	}
	if !existing.ConsumedAt.IsZero() {
		return GameplayEventReceiptReservation{Receipt: *cloneGameplayEventReceipt(existing), Duplicate: true}, nil
	}
	if existing.ClaimOwnerID != "" && existing.ClaimOwnerID != claimOwnerID && existing.ClaimDeadlineAt.After(now) {
		return GameplayEventReceiptReservation{Receipt: *cloneGameplayEventReceipt(existing), Busy: true}, nil
	}
	existing.ClaimOwnerID = claimOwnerID
	existing.ClaimDeadlineAt = now.Add(claimLease)
	existing.UpdatedAt = now
	return GameplayEventReceiptReservation{Receipt: *cloneGameplayEventReceipt(existing), Acquired: true}, nil
}

func (repo memoryGameplayEventReceiptRepo) MarkConsumed(_ context.Context, eventID int64, claimOwnerID string, consumedAt time.Time) (bool, error) {
	if consumedAt.IsZero() {
		consumedAt = time.Now()
	}
	consumedAt = consumedAt.UTC()
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()
	receipt := repo.backend.gameplayReceipts[eventID]
	if receipt == nil {
		return false, errRecordNotFound
	}
	if !receipt.ConsumedAt.IsZero() {
		return true, nil
	}
	if receipt.ClaimOwnerID != strings.TrimSpace(claimOwnerID) {
		return false, errOwnershipStale
	}
	receipt.DeliveredAt = consumedAt
	receipt.ConsumedAt = consumedAt
	receipt.ClaimOwnerID = ""
	receipt.ClaimDeadlineAt = time.Time{}
	receipt.UpdatedAt = consumedAt
	return true, nil
}

func (repo memoryGameplayEventReceiptRepo) Release(_ context.Context, eventID int64, claimOwnerID string) error {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()
	receipt := repo.backend.gameplayReceipts[eventID]
	if receipt == nil || !receipt.ConsumedAt.IsZero() {
		return nil
	}
	if receipt.ClaimOwnerID != strings.TrimSpace(claimOwnerID) {
		return errOwnershipStale
	}
	delete(repo.backend.gameplayReceipts, eventID)
	return nil
}

func (repo memoryGameplayEventReceiptRepo) GetByEventID(_ context.Context, eventID int64) (*GameplayEventReceipt, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()
	receipt := repo.backend.gameplayReceipts[eventID]
	if receipt == nil {
		return nil, errRecordNotFound
	}
	return cloneGameplayEventReceipt(receipt), nil
}

type postgresGameplayEventReceiptRepo struct{ backend *postgresStoreBackend }

const gameplayEventReceiptColumns = `event_id, recipient_session_id, recipient_character_id, server_instance_id,
       claim_owner_id, claim_deadline_at, delivered_at, consumed_at, created_at, updated_at`

func scanGameplayEventReceipt(scanner rowScanner) (*GameplayEventReceipt, error) {
	receipt := &GameplayEventReceipt{}
	var recipientSessionID, recipientCharacterID, claimOwnerID sql.NullString
	var claimDeadlineAt, deliveredAt, consumedAt sql.NullTime
	if err := scanner.Scan(
		&receipt.EventID,
		&recipientSessionID,
		&recipientCharacterID,
		&receipt.ServerInstanceID,
		&claimOwnerID,
		&claimDeadlineAt,
		&deliveredAt,
		&consumedAt,
		&receipt.CreatedAt,
		&receipt.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	receipt.RecipientSessionID = recipientSessionID.String
	receipt.RecipientCharacterID = recipientCharacterID.String
	receipt.ClaimOwnerID = claimOwnerID.String
	receipt.ClaimDeadlineAt = claimDeadlineAt.Time
	receipt.DeliveredAt = deliveredAt.Time
	receipt.ConsumedAt = consumedAt.Time
	return receipt, nil
}

func (repo postgresGameplayEventReceiptRepo) Reserve(ctx context.Context, receipt GameplayEventReceipt, claimOwnerID string, _ time.Time, claimLease time.Duration) (GameplayEventReceiptReservation, error) {
	normalized, err := normalizeGameplayEventReceipt(receipt)
	claimOwnerID = strings.TrimSpace(claimOwnerID)
	if err != nil || claimOwnerID == "" || claimLease <= 0 {
		if err != nil {
			return GameplayEventReceiptReservation{}, err
		}
		return GameplayEventReceiptReservation{}, errors.New("invalid gameplay event receipt reservation")
	}
	tx, err := repo.backend.db.BeginTx(ctx, nil)
	if err != nil {
		return GameplayEventReceiptReservation{}, err
	}
	defer tx.Rollback()

	created, insertErr := scanGameplayEventReceipt(tx.QueryRowContext(ctx,
		`INSERT INTO gameplay_event_receipts (
		   event_id, recipient_session_id, recipient_character_id, server_instance_id,
		   claim_owner_id, claim_deadline_at, created_at, updated_at
		 ) VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4, $5, NOW() + ($6 * INTERVAL '1 millisecond'), NOW(), NOW())
		 ON CONFLICT (event_id) DO NOTHING
		 RETURNING `+gameplayEventReceiptColumns,
		normalized.EventID,
		normalized.RecipientSessionID,
		normalized.RecipientCharacterID,
		normalized.ServerInstanceID,
		claimOwnerID,
		claimLease.Milliseconds(),
	))
	if insertErr == nil {
		if err := tx.Commit(); err != nil {
			return GameplayEventReceiptReservation{}, err
		}
		return GameplayEventReceiptReservation{Receipt: *created, Acquired: true}, nil
	}
	if !errors.Is(insertErr, errRecordNotFound) {
		return GameplayEventReceiptReservation{}, insertErr
	}

	existing, err := scanGameplayEventReceipt(tx.QueryRowContext(ctx,
		`SELECT `+gameplayEventReceiptColumns+`
		 FROM gameplay_event_receipts
		 WHERE event_id = $1
		 FOR UPDATE`,
		normalized.EventID,
	))
	if err != nil {
		return GameplayEventReceiptReservation{}, err
	}
	if !sameGameplayEventReceiptIdentity(*existing, normalized) {
		return GameplayEventReceiptReservation{}, errRecordConflict
	}
	if !existing.ConsumedAt.IsZero() {
		if err := tx.Commit(); err != nil {
			return GameplayEventReceiptReservation{}, err
		}
		return GameplayEventReceiptReservation{Receipt: *existing, Duplicate: true}, nil
	}
	var databaseNow time.Time
	if err := tx.QueryRowContext(ctx, `SELECT NOW()`).Scan(&databaseNow); err != nil {
		return GameplayEventReceiptReservation{}, err
	}
	if existing.ClaimOwnerID != "" && existing.ClaimOwnerID != claimOwnerID && existing.ClaimDeadlineAt.After(databaseNow) {
		if err := tx.Commit(); err != nil {
			return GameplayEventReceiptReservation{}, err
		}
		return GameplayEventReceiptReservation{Receipt: *existing, Busy: true}, nil
	}
	acquired, err := scanGameplayEventReceipt(tx.QueryRowContext(ctx,
		`UPDATE gameplay_event_receipts
		 SET claim_owner_id = $2,
		     claim_deadline_at = NOW() + ($3 * INTERVAL '1 millisecond'),
		     updated_at = NOW()
		 WHERE event_id = $1
		 RETURNING `+gameplayEventReceiptColumns,
		normalized.EventID,
		claimOwnerID,
		claimLease.Milliseconds(),
	))
	if err != nil {
		return GameplayEventReceiptReservation{}, err
	}
	if err := tx.Commit(); err != nil {
		return GameplayEventReceiptReservation{}, err
	}
	return GameplayEventReceiptReservation{Receipt: *acquired, Acquired: true}, nil
}

func (repo postgresGameplayEventReceiptRepo) MarkConsumed(ctx context.Context, eventID int64, claimOwnerID string, _ time.Time) (bool, error) {
	result, err := repo.backend.db.ExecContext(ctx,
		`UPDATE gameplay_event_receipts
		 SET delivered_at = COALESCE(delivered_at, NOW()),
		     consumed_at = COALESCE(consumed_at, NOW()),
		     claim_owner_id = NULL,
		     claim_deadline_at = NULL,
		     updated_at = NOW()
		 WHERE event_id = $1
		   AND (claim_owner_id = $2 OR consumed_at IS NOT NULL)`,
		eventID,
		strings.TrimSpace(claimOwnerID),
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (repo postgresGameplayEventReceiptRepo) Release(ctx context.Context, eventID int64, claimOwnerID string) error {
	_, err := repo.backend.db.ExecContext(ctx,
		`DELETE FROM gameplay_event_receipts
		 WHERE event_id = $1
		   AND claim_owner_id = $2
		   AND consumed_at IS NULL`,
		eventID,
		strings.TrimSpace(claimOwnerID),
	)
	return err
}

func (repo postgresGameplayEventReceiptRepo) GetByEventID(ctx context.Context, eventID int64) (*GameplayEventReceipt, error) {
	return scanGameplayEventReceipt(repo.backend.db.QueryRowContext(ctx,
		`SELECT `+gameplayEventReceiptColumns+`
		 FROM gameplay_event_receipts
		 WHERE event_id = $1`,
		eventID,
	))
}
