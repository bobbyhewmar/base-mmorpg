package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"time"
)

const maxGameplayEventPayloadBytes = 64 * 1024

func normalizeGameplayEvent(event *GameplayEvent, now time.Time) (*GameplayEvent, error) {
	if event == nil {
		return nil, errors.New("gameplay event is required")
	}
	clone := cloneGameplayEvent(event)
	clone.IdempotencyKey = strings.TrimSpace(clone.IdempotencyKey)
	clone.Type = strings.TrimSpace(clone.Type)
	clone.TargetServerInstanceID = strings.TrimSpace(clone.TargetServerInstanceID)
	clone.TargetRegionID = strings.TrimSpace(clone.TargetRegionID)
	clone.TargetSessionID = strings.TrimSpace(clone.TargetSessionID)
	clone.TargetCharacterID = strings.TrimSpace(clone.TargetCharacterID)
	if clone.IdempotencyKey == "" || len(clone.IdempotencyKey) > 240 {
		return nil, errors.New("invalid gameplay event idempotency key")
	}
	if clone.Type == "" || len(clone.Type) > 120 {
		return nil, errors.New("invalid gameplay event type")
	}
	if clone.TargetServerInstanceID == "" || len(clone.TargetServerInstanceID) > 160 {
		return nil, errors.New("gameplay event requires an exact target server instance")
	}
	if len(clone.TargetRegionID) > 160 || len(clone.TargetSessionID) > 240 || len(clone.TargetCharacterID) > 240 {
		return nil, errors.New("invalid gameplay event target")
	}
	if len(clone.Payload) == 0 {
		clone.Payload = json.RawMessage(`{}`)
	}
	if len(clone.Payload) > maxGameplayEventPayloadBytes || !json.Valid(clone.Payload) {
		return nil, errors.New("invalid gameplay event payload")
	}
	if err := normalizeGameplayEventProjectionMetadata(clone); err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = now.UTC()
	} else {
		clone.CreatedAt = clone.CreatedAt.UTC()
	}
	if clone.AvailableAt.IsZero() {
		clone.AvailableAt = clone.CreatedAt
	} else {
		clone.AvailableAt = clone.AvailableAt.UTC()
	}
	return clone, nil
}

func normalizeGameplayEventProjectionMetadata(event *GameplayEvent) error {
	if event == nil {
		return errors.New("gameplay event is required")
	}
	event.ProjectionSourceCharacterID = strings.TrimSpace(event.ProjectionSourceCharacterID)
	event.TargetCharacterID = strings.TrimSpace(event.TargetCharacterID)
	event.ProjectionAction = strings.TrimSpace(event.ProjectionAction)
	if event.Type != regionPlayerProjectionEventType {
		event.ProjectionSourceCharacterID = ""
		event.ProjectionSourceFencingToken = 0
		event.ProjectionVersion = 0
		event.ProjectionRecipientFencingToken = 0
		event.ProjectionAction = ""
		return nil
	}
	var payload regionPlayerProjectionPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return errors.New("invalid gameplay event payload")
	}
	if err := validateRegionPlayerProjectionPayload(payload); err != nil {
		return err
	}
	event.ProjectionSourceCharacterID = payload.CharacterID
	event.ProjectionSourceFencingToken = payload.FencingToken
	event.ProjectionVersion = payload.Version
	event.ProjectionRecipientFencingToken = payload.RecipientFencingToken
	event.ProjectionAction = payload.Action
	return nil
}

func eventIsOlderProjectionCandidate(event *GameplayEvent, supersession RegionProjectionSupersession) bool {
	if event == nil || event.Type != regionPlayerProjectionEventType || !event.SupersededAt.IsZero() || !event.DeliveredAt.IsZero() || !event.DeadLetteredAt.IsZero() {
		return false
	}
	if event.TargetServerInstanceID != supersession.TargetServerInstanceID ||
		event.TargetCharacterID != supersession.TargetCharacterID ||
		event.ProjectionSourceCharacterID != supersession.ProjectionSourceCharacterID ||
		event.ProjectionRecipientFencingToken != supersession.ProjectionRecipientFencingToken {
		return false
	}
	if event.ID == supersession.SupersedingEventID {
		return false
	}
	if event.ProjectionSourceFencingToken > supersession.ProjectionSourceFencingToken {
		return false
	}
	if event.ProjectionSourceFencingToken == supersession.ProjectionSourceFencingToken &&
		event.ProjectionVersion >= supersession.ProjectionVersion {
		return false
	}
	return true
}

func cloneGameplayEvent(event *GameplayEvent) *GameplayEvent {
	if event == nil {
		return nil
	}
	clone := *event
	clone.Payload = append(json.RawMessage(nil), event.Payload...)
	return &clone
}

func sameGameplayEventIdentity(left *GameplayEvent, right *GameplayEvent) bool {
	if left == nil || right == nil {
		return false
	}
	return left.IdempotencyKey == right.IdempotencyKey &&
		left.Type == right.Type &&
		sameGameplayEventPayload(left.Payload, right.Payload) &&
		left.TargetServerInstanceID == right.TargetServerInstanceID &&
		left.TargetRegionID == right.TargetRegionID &&
		left.TargetSessionID == right.TargetSessionID &&
		left.TargetCharacterID == right.TargetCharacterID
}

func sameGameplayEventPayload(left json.RawMessage, right json.RawMessage) bool {
	var leftValue any
	var rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func summarizeGameplayEventError(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), "_")
	if value == "" {
		value = "delivery_failed"
	}
	if len(value) > 240 {
		value = value[:240]
	}
	return value
}

func (backend *memoryStoreBackend) createGameplayEventLocked(event *GameplayEvent) (bool, error) {
	normalized, err := normalizeGameplayEvent(event, time.Now())
	if err != nil {
		return false, err
	}
	if existingID, exists := backend.gameplayEventByKey[normalized.IdempotencyKey]; exists {
		existing := backend.gameplayEvents[existingID]
		if !sameGameplayEventIdentity(existing, normalized) {
			return false, errRecordConflict
		}
		event.ID = existing.ID
		return false, nil
	}
	backend.nextGameplayEventID++
	normalized.ID = backend.nextGameplayEventID
	backend.gameplayEvents[normalized.ID] = normalized
	backend.gameplayEventByKey[normalized.IdempotencyKey] = normalized.ID
	*event = *cloneGameplayEvent(normalized)
	return true, nil
}

func (repo memoryGameplayEventRepo) Create(_ context.Context, event *GameplayEvent) (bool, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()
	return repo.backend.createGameplayEventLocked(event)
}

func (repo memoryGameplayEventRepo) GetByID(_ context.Context, eventID int64) (*GameplayEvent, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()
	event := repo.backend.gameplayEvents[eventID]
	if event == nil {
		return nil, errRecordNotFound
	}
	return cloneGameplayEvent(event), nil
}

func (repo memoryGameplayEventRepo) GetByIdempotencyKey(_ context.Context, idempotencyKey string) (*GameplayEvent, error) {
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()
	eventID, exists := repo.backend.gameplayEventByKey[strings.TrimSpace(idempotencyKey)]
	if !exists || repo.backend.gameplayEvents[eventID] == nil {
		return nil, errRecordNotFound
	}
	return cloneGameplayEvent(repo.backend.gameplayEvents[eventID]), nil
}

func (repo memoryGameplayEventRepo) Claim(_ context.Context, serverInstanceID string, claimOwnerID string, now time.Time, claimLease time.Duration, limit int) ([]GameplayEvent, error) {
	serverInstanceID = strings.TrimSpace(serverInstanceID)
	claimOwnerID = strings.TrimSpace(claimOwnerID)
	if serverInstanceID == "" || claimOwnerID == "" || claimLease <= 0 || limit <= 0 {
		return nil, errors.New("invalid gameplay event claim")
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()
	ids := make([]int64, 0, len(repo.backend.gameplayEvents))
	for eventID := range repo.backend.gameplayEvents {
		ids = append(ids, eventID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	claimed := make([]GameplayEvent, 0, min(limit, len(ids)))
	for _, eventID := range ids {
		if len(claimed) >= limit {
			break
		}
		event := repo.backend.gameplayEvents[eventID]
		if event == nil || event.TargetServerInstanceID != serverInstanceID || !event.DeliveredAt.IsZero() || !event.DeadLetteredAt.IsZero() || !event.SupersededAt.IsZero() || event.AvailableAt.After(now) {
			continue
		}
		if event.ClaimOwnerID != "" && event.ClaimDeadlineAt.After(now) {
			continue
		}
		event.ClaimedAt = now
		event.ClaimOwnerID = claimOwnerID
		event.ClaimDeadlineAt = now.Add(claimLease)
		claimed = append(claimed, *cloneGameplayEvent(event))
	}
	return claimed, nil
}

func (repo memoryGameplayEventRepo) MarkDelivered(_ context.Context, eventID int64, claimOwnerID string, deliveredAt time.Time) (bool, error) {
	if deliveredAt.IsZero() {
		deliveredAt = time.Now()
	}
	deliveredAt = deliveredAt.UTC()
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()
	event := repo.backend.gameplayEvents[eventID]
	if event == nil {
		return false, errRecordNotFound
	}
	if event.ClaimOwnerID != strings.TrimSpace(claimOwnerID) || event.ClaimDeadlineAt.IsZero() || !event.ClaimDeadlineAt.After(deliveredAt) || !event.DeliveredAt.IsZero() || !event.DeadLetteredAt.IsZero() || !event.SupersededAt.IsZero() {
		return false, nil
	}
	event.DeliveredAt = deliveredAt
	event.ClaimOwnerID = ""
	event.ClaimDeadlineAt = time.Time{}
	return true, nil
}

func (repo memoryGameplayEventRepo) MarkFailed(_ context.Context, eventID int64, claimOwnerID string, failedAt time.Time, retryDelay time.Duration, maxRetries int, lastError string) (GameplayEventFailure, error) {
	if failedAt.IsZero() {
		failedAt = time.Now()
	}
	if maxRetries <= 0 {
		maxRetries = 1
	}
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()
	event := repo.backend.gameplayEvents[eventID]
	if event == nil {
		return GameplayEventFailure{}, errRecordNotFound
	}
	if event.ClaimOwnerID != strings.TrimSpace(claimOwnerID) || !event.DeliveredAt.IsZero() || !event.DeadLetteredAt.IsZero() || !event.SupersededAt.IsZero() {
		return GameplayEventFailure{}, errOwnershipStale
	}
	event.RetryCount++
	event.LastError = summarizeGameplayEventError(lastError)
	event.ClaimOwnerID = ""
	event.ClaimDeadlineAt = time.Time{}
	deadLettered := event.RetryCount >= maxRetries
	if deadLettered {
		event.DeadLetteredAt = failedAt.UTC()
	} else {
		event.AvailableAt = failedAt.UTC().Add(max(0, retryDelay))
	}
	return GameplayEventFailure{RetryCount: event.RetryCount, DeadLettered: deadLettered}, nil
}

func (repo memoryGameplayEventRepo) SupersedeRegionProjection(_ context.Context, supersession RegionProjectionSupersession) (int, error) {
	if supersession.SupersedingEventID <= 0 || supersession.ProjectionSourceCharacterID == "" || supersession.TargetServerInstanceID == "" || supersession.TargetCharacterID == "" || supersession.ProjectionRecipientFencingToken <= 0 || supersession.ProjectionSourceFencingToken <= 0 || supersession.ProjectionVersion <= 0 {
		return 0, errors.New("invalid region projection supersession")
	}
	if supersession.SupersededAt.IsZero() {
		supersession.SupersededAt = time.Now().UTC()
	}
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()
	count := 0
	for _, event := range repo.backend.gameplayEvents {
		if !eventIsOlderProjectionCandidate(event, supersession) {
			continue
		}
		event.SupersededAt = supersession.SupersededAt
		event.SupersededByEventID = supersession.SupersedingEventID
		event.ClaimOwnerID = ""
		event.ClaimDeadlineAt = time.Time{}
		count++
	}
	return count, nil
}

func (repo memoryGameplayEventRepo) DeleteSupersededBefore(_ context.Context, cutoff time.Time, limit int) (int, error) {
	if cutoff.IsZero() || limit <= 0 {
		return 0, nil
	}
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()
	ids := make([]int64, 0, len(repo.backend.gameplayEvents))
	for eventID, event := range repo.backend.gameplayEvents {
		if event != nil && !event.SupersededAt.IsZero() && event.SupersededAt.Before(cutoff) {
			ids = append(ids, eventID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) > limit {
		ids = ids[:limit]
	}
	for _, eventID := range ids {
		event := repo.backend.gameplayEvents[eventID]
		delete(repo.backend.gameplayEventByKey, event.IdempotencyKey)
		delete(repo.backend.gameplayEvents, eventID)
	}
	return len(ids), nil
}

func (repo memoryGameplayEventRepo) DeleteDeliveredBefore(_ context.Context, cutoff time.Time, limit int) (int, error) {
	if cutoff.IsZero() || limit <= 0 {
		return 0, nil
	}
	repo.backend.mu.Lock()
	defer repo.backend.mu.Unlock()
	ids := make([]int64, 0, len(repo.backend.gameplayEvents))
	for eventID, event := range repo.backend.gameplayEvents {
		if event != nil && !event.DeliveredAt.IsZero() && event.DeliveredAt.Before(cutoff) {
			ids = append(ids, eventID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) > limit {
		ids = ids[:limit]
	}
	for _, eventID := range ids {
		event := repo.backend.gameplayEvents[eventID]
		delete(repo.backend.gameplayEventByKey, event.IdempotencyKey)
		delete(repo.backend.gameplayEvents, eventID)
	}
	return len(ids), nil
}

func (backend *memoryStoreBackend) FinalizeGameplayCommandWithEvent(_ context.Context, sessionID string, commandSeq int, status GameplayCommandRecordStatus, outboundMessages []map[string]any, event *GameplayEvent) (bool, error) {
	created, err := backend.FinalizeGameplayCommandWithEvents(context.Background(), sessionID, commandSeq, status, outboundMessages, []*GameplayEvent{event})
	return created == 1, err
}

func (backend *memoryStoreBackend) FinalizeGameplayCommandWithEvents(_ context.Context, sessionID string, commandSeq int, status GameplayCommandRecordStatus, outboundMessages []map[string]any, events []*GameplayEvent) (int, error) {
	normalizedEvents := make([]*GameplayEvent, 0, len(events))
	normalizedByKey := make(map[string]*GameplayEvent, len(events))
	for _, event := range events {
		normalized, err := normalizeGameplayEvent(event, time.Now())
		if err != nil {
			return 0, err
		}
		if previous := normalizedByKey[normalized.IdempotencyKey]; previous != nil && !sameGameplayEventIdentity(previous, normalized) {
			return 0, errRecordConflict
		}
		normalizedByKey[normalized.IdempotencyKey] = normalized
		normalizedEvents = append(normalizedEvents, normalized)
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	record := backend.commandRecords[gameplayCommandRecordKey(sessionID, commandSeq)]
	if record == nil {
		return 0, errRecordNotFound
	}
	for _, normalized := range normalizedEvents {
		if existingID, exists := backend.gameplayEventByKey[normalized.IdempotencyKey]; exists {
			if !sameGameplayEventIdentity(backend.gameplayEvents[existingID], normalized) {
				return 0, errRecordConflict
			}
		}
	}

	record.Status = status
	record.OutboundMessages = cloneOutboundMessages(outboundMessages)
	createdCount := 0
	for index, normalized := range normalizedEvents {
		created, err := backend.createGameplayEventLocked(normalized)
		if err != nil {
			return 0, err
		}
		if created {
			createdCount++
		}
		*events[index] = *cloneGameplayEvent(normalized)
	}
	return createdCount, nil
}

func (backend *memoryStoreBackend) FinalizeGameplayCommandWithChatAndEvents(_ context.Context, sessionID string, commandSeq int, status GameplayCommandRecordStatus, outboundMessages []map[string]any, chatMessage ChatMessageRecord, events []*GameplayEvent) (int, error) {
	chatMessage = normalizeChatMessageRecord(chatMessage)
	if chatMessage.ID == "" || chatMessage.CharacterID == "" || chatMessage.Channel == "" || chatMessage.Text == "" {
		return 0, errors.New("invalid chat message record")
	}
	normalizedEvents := make([]*GameplayEvent, 0, len(events))
	normalizedByKey := make(map[string]*GameplayEvent, len(events))
	for _, event := range events {
		normalized, err := normalizeGameplayEvent(event, time.Now())
		if err != nil {
			return 0, err
		}
		if previous := normalizedByKey[normalized.IdempotencyKey]; previous != nil && !sameGameplayEventIdentity(previous, normalized) {
			return 0, errRecordConflict
		}
		normalizedByKey[normalized.IdempotencyKey] = normalized
		normalizedEvents = append(normalizedEvents, normalized)
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	record := backend.commandRecords[gameplayCommandRecordKey(sessionID, commandSeq)]
	if record == nil {
		return 0, errRecordNotFound
	}
	for _, normalized := range normalizedEvents {
		if existingID, exists := backend.gameplayEventByKey[normalized.IdempotencyKey]; exists && !sameGameplayEventIdentity(backend.gameplayEvents[existingID], normalized) {
			return 0, errRecordConflict
		}
	}
	record.Status = status
	record.OutboundMessages = cloneOutboundMessages(outboundMessages)
	backend.chatMessages[chatMessage.CharacterID] = append(backend.chatMessages[chatMessage.CharacterID], chatMessage)
	createdCount := 0
	for index, normalized := range normalizedEvents {
		created, err := backend.createGameplayEventLocked(normalized)
		if err != nil {
			return 0, err
		}
		if created {
			createdCount++
		}
		*events[index] = *cloneGameplayEvent(normalized)
	}
	return createdCount, nil
}

type postgresGameplayEventRepo struct{ backend *postgresStoreBackend }

const gameplayEventColumns = `event_id, idempotency_key, event_type, payload_json,
       target_server_instance_id, target_region_id, target_session_id, target_character_id,
       projection_source_character_id, projection_source_fencing_token, projection_version,
       projection_recipient_fencing_token, projection_action,
       created_at, available_at, claimed_at, claim_owner_id, claim_deadline_at,
       delivered_at, dead_lettered_at, superseded_at, superseded_by_event_id, retry_count, last_error`

const qualifiedGameplayEventColumns = `event.event_id, event.idempotency_key, event.event_type, event.payload_json,
       event.target_server_instance_id, event.target_region_id, event.target_session_id, event.target_character_id,
       event.projection_source_character_id, event.projection_source_fencing_token, event.projection_version,
       event.projection_recipient_fencing_token, event.projection_action,
       event.created_at, event.available_at, event.claimed_at, event.claim_owner_id, event.claim_deadline_at,
       event.delivered_at, event.dead_lettered_at, event.superseded_at, event.superseded_by_event_id, event.retry_count, event.last_error`

func scanGameplayEvent(scanner rowScanner) (*GameplayEvent, error) {
	event := &GameplayEvent{}
	var payload []byte
	var claimedAt, claimDeadlineAt, deliveredAt, deadLetteredAt, supersededAt sql.NullTime
	var claimOwnerID, targetRegionID, targetSessionID, targetCharacterID, lastError, projectionSourceCharacterID, projectionAction sql.NullString
	var projectionSourceFencingToken, projectionVersion, projectionRecipientFencingToken, supersededByEventID sql.NullInt64
	if err := scanner.Scan(
		&event.ID,
		&event.IdempotencyKey,
		&event.Type,
		&payload,
		&event.TargetServerInstanceID,
		&targetRegionID,
		&targetSessionID,
		&targetCharacterID,
		&projectionSourceCharacterID,
		&projectionSourceFencingToken,
		&projectionVersion,
		&projectionRecipientFencingToken,
		&projectionAction,
		&event.CreatedAt,
		&event.AvailableAt,
		&claimedAt,
		&claimOwnerID,
		&claimDeadlineAt,
		&deliveredAt,
		&deadLetteredAt,
		&supersededAt,
		&supersededByEventID,
		&event.RetryCount,
		&lastError,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errRecordNotFound
		}
		return nil, err
	}
	event.Payload = append(json.RawMessage(nil), payload...)
	event.TargetRegionID = targetRegionID.String
	event.TargetSessionID = targetSessionID.String
	event.TargetCharacterID = targetCharacterID.String
	event.ProjectionSourceCharacterID = projectionSourceCharacterID.String
	event.ProjectionSourceFencingToken = projectionSourceFencingToken.Int64
	event.ProjectionVersion = projectionVersion.Int64
	event.ProjectionRecipientFencingToken = projectionRecipientFencingToken.Int64
	event.ProjectionAction = projectionAction.String
	event.ClaimedAt = claimedAt.Time
	event.ClaimOwnerID = claimOwnerID.String
	event.ClaimDeadlineAt = claimDeadlineAt.Time
	event.DeliveredAt = deliveredAt.Time
	event.DeadLetteredAt = deadLetteredAt.Time
	event.SupersededAt = supersededAt.Time
	event.SupersededByEventID = supersededByEventID.Int64
	event.LastError = lastError.String
	return event, nil
}

func insertGameplayEvent(ctx context.Context, executor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, event *GameplayEvent) (bool, error) {
	normalized, err := normalizeGameplayEvent(event, time.Now())
	if err != nil {
		return false, err
	}
	row := executor.QueryRowContext(ctx,
		`INSERT INTO gameplay_event_outbox (
		   idempotency_key, event_type, payload_json, target_server_instance_id,
		   target_region_id, target_session_id, target_character_id,
		   projection_source_character_id, projection_source_fencing_token, projection_version,
		   projection_recipient_fencing_token, projection_action, created_at, available_at
		 ) VALUES ($1, $2, $3::jsonb, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, 0), NULLIF($10, 0), NULLIF($11, 0), NULLIF($12, ''), $13, $14)
		 ON CONFLICT (idempotency_key) DO NOTHING
		 RETURNING `+gameplayEventColumns,
		normalized.IdempotencyKey,
		normalized.Type,
		[]byte(normalized.Payload),
		normalized.TargetServerInstanceID,
		normalized.TargetRegionID,
		normalized.TargetSessionID,
		normalized.TargetCharacterID,
		normalized.ProjectionSourceCharacterID,
		normalized.ProjectionSourceFencingToken,
		normalized.ProjectionVersion,
		normalized.ProjectionRecipientFencingToken,
		normalized.ProjectionAction,
		normalized.CreatedAt,
		normalized.AvailableAt,
	)
	created, err := scanGameplayEvent(row)
	if errors.Is(err, errRecordNotFound) {
		existing, lookupErr := scanGameplayEvent(executor.QueryRowContext(ctx, `SELECT `+gameplayEventColumns+` FROM gameplay_event_outbox WHERE idempotency_key = $1`, normalized.IdempotencyKey))
		if lookupErr != nil {
			return false, lookupErr
		}
		if !sameGameplayEventIdentity(existing, normalized) {
			return false, errRecordConflict
		}
		event.ID = existing.ID
		return false, nil
	}
	if err != nil {
		return false, err
	}
	*event = *cloneGameplayEvent(created)
	return true, nil
}

func (repo postgresGameplayEventRepo) Create(ctx context.Context, event *GameplayEvent) (bool, error) {
	return insertGameplayEvent(ctx, repo.backend.db, event)
}

func (repo postgresGameplayEventRepo) GetByID(ctx context.Context, eventID int64) (*GameplayEvent, error) {
	return scanGameplayEvent(repo.backend.db.QueryRowContext(ctx, `SELECT `+gameplayEventColumns+` FROM gameplay_event_outbox WHERE event_id = $1`, eventID))
}

func (repo postgresGameplayEventRepo) GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (*GameplayEvent, error) {
	return scanGameplayEvent(repo.backend.db.QueryRowContext(ctx, `SELECT `+gameplayEventColumns+` FROM gameplay_event_outbox WHERE idempotency_key = $1`, strings.TrimSpace(idempotencyKey)))
}

func (repo postgresGameplayEventRepo) Claim(ctx context.Context, serverInstanceID string, claimOwnerID string, _ time.Time, claimLease time.Duration, limit int) ([]GameplayEvent, error) {
	serverInstanceID = strings.TrimSpace(serverInstanceID)
	claimOwnerID = strings.TrimSpace(claimOwnerID)
	if serverInstanceID == "" || claimOwnerID == "" || claimLease <= 0 || limit <= 0 {
		return nil, errors.New("invalid gameplay event claim")
	}
	leaseMilliseconds := max(int64(1), claimLease.Milliseconds())
	rows, err := repo.backend.db.QueryContext(ctx,
		`WITH candidates AS (
		   SELECT event_id
		   FROM gameplay_event_outbox
		   WHERE target_server_instance_id = $1
		     AND delivered_at IS NULL
		     AND dead_lettered_at IS NULL
		     AND superseded_at IS NULL
		     AND available_at <= NOW()
		     AND (claim_deadline_at IS NULL OR claim_deadline_at <= NOW())
		   ORDER BY event_id
		   FOR UPDATE SKIP LOCKED
		   LIMIT $4
		 )
		 UPDATE gameplay_event_outbox event
		 SET claimed_at = NOW(),
		     claim_owner_id = $2,
		     claim_deadline_at = NOW() + ($3 * INTERVAL '1 millisecond')
		 FROM candidates
		 WHERE event.event_id = candidates.event_id
		 RETURNING `+qualifiedGameplayEventColumns,
		serverInstanceID,
		claimOwnerID,
		leaseMilliseconds,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]GameplayEvent, 0)
	for rows.Next() {
		event, err := scanGameplayEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, *event)
	}
	return events, rows.Err()
}

func (repo postgresGameplayEventRepo) MarkDelivered(ctx context.Context, eventID int64, claimOwnerID string, _ time.Time) (bool, error) {
	result, err := repo.backend.db.ExecContext(ctx,
		`UPDATE gameplay_event_outbox
		 SET delivered_at = NOW(),
		     claim_owner_id = NULL,
		     claim_deadline_at = NULL
		 WHERE event_id = $1
		   AND claim_owner_id = $2
		   AND claim_deadline_at > NOW()
		   AND delivered_at IS NULL
		   AND dead_lettered_at IS NULL
		   AND superseded_at IS NULL`,
		eventID,
		strings.TrimSpace(claimOwnerID),
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (repo postgresGameplayEventRepo) MarkFailed(ctx context.Context, eventID int64, claimOwnerID string, _ time.Time, retryDelay time.Duration, maxRetries int, lastError string) (GameplayEventFailure, error) {
	if maxRetries <= 0 {
		maxRetries = 1
	}
	retryMilliseconds := max(int64(0), retryDelay.Milliseconds())
	var retryCount int
	var deadLetteredAt sql.NullTime
	err := repo.backend.db.QueryRowContext(ctx,
		`UPDATE gameplay_event_outbox
		 SET retry_count = retry_count + 1,
		     last_error = $3,
		     available_at = CASE WHEN retry_count + 1 >= $5 THEN available_at ELSE NOW() + ($4 * INTERVAL '1 millisecond') END,
		     dead_lettered_at = CASE WHEN retry_count + 1 >= $5 THEN NOW() ELSE NULL END,
		     claim_owner_id = NULL,
		     claim_deadline_at = NULL
		 WHERE event_id = $1
		   AND claim_owner_id = $2
		   AND delivered_at IS NULL
		   AND dead_lettered_at IS NULL
		   AND superseded_at IS NULL
		 RETURNING retry_count, dead_lettered_at`,
		eventID,
		strings.TrimSpace(claimOwnerID),
		summarizeGameplayEventError(lastError),
		retryMilliseconds,
		maxRetries,
	).Scan(&retryCount, &deadLetteredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GameplayEventFailure{}, errOwnershipStale
	}
	if err != nil {
		return GameplayEventFailure{}, err
	}
	return GameplayEventFailure{RetryCount: retryCount, DeadLettered: deadLetteredAt.Valid}, nil
}

func (repo postgresGameplayEventRepo) SupersedeRegionProjection(ctx context.Context, supersession RegionProjectionSupersession) (int, error) {
	if supersession.SupersedingEventID <= 0 || strings.TrimSpace(supersession.TargetServerInstanceID) == "" || strings.TrimSpace(supersession.TargetCharacterID) == "" || strings.TrimSpace(supersession.ProjectionSourceCharacterID) == "" || supersession.ProjectionRecipientFencingToken <= 0 || supersession.ProjectionSourceFencingToken <= 0 || supersession.ProjectionVersion <= 0 {
		return 0, errors.New("invalid region projection supersession")
	}
	if supersession.SupersededAt.IsZero() {
		supersession.SupersededAt = time.Now().UTC()
	}
	result, err := repo.backend.db.ExecContext(ctx,
		`UPDATE gameplay_event_outbox
		 SET superseded_at = $2,
		     superseded_by_event_id = $3,
		     claim_owner_id = NULL,
		     claim_deadline_at = NULL
		 WHERE event_type = $1
		   AND target_server_instance_id = $4
		   AND target_character_id = $5
		   AND projection_source_character_id = $6
		   AND projection_recipient_fencing_token = $7
		   AND delivered_at IS NULL
		   AND dead_lettered_at IS NULL
		   AND superseded_at IS NULL
		   AND event_id <> $3
		   AND (
		     projection_source_fencing_token < $8
		     OR (
		       projection_source_fencing_token = $8
		       AND projection_version < $9
		     )
		   )`,
		regionPlayerProjectionEventType,
		supersession.SupersededAt.UTC(),
		supersession.SupersedingEventID,
		strings.TrimSpace(supersession.TargetServerInstanceID),
		strings.TrimSpace(supersession.TargetCharacterID),
		strings.TrimSpace(supersession.ProjectionSourceCharacterID),
		supersession.ProjectionRecipientFencingToken,
		supersession.ProjectionSourceFencingToken,
		supersession.ProjectionVersion,
	)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	return int(rows), err
}

func (repo postgresGameplayEventRepo) DeleteSupersededBefore(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	if cutoff.IsZero() || limit <= 0 {
		return 0, nil
	}
	result, err := repo.backend.db.ExecContext(ctx,
		`WITH obsolete AS (
		   SELECT event_id
		   FROM gameplay_event_outbox
		   WHERE superseded_at IS NOT NULL
		     AND superseded_at < $1
		   ORDER BY event_id
		   LIMIT $2
		 )
		 DELETE FROM gameplay_event_outbox event
		 USING obsolete
		 WHERE event.event_id = obsolete.event_id`,
		cutoff.UTC(),
		limit,
	)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	return int(rows), err
}

func (repo postgresGameplayEventRepo) DeleteDeliveredBefore(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	if cutoff.IsZero() || limit <= 0 {
		return 0, nil
	}
	result, err := repo.backend.db.ExecContext(ctx,
		`WITH expired AS (
		   SELECT event_id
		   FROM gameplay_event_outbox
		   WHERE delivered_at IS NOT NULL
		     AND delivered_at < $1
		   ORDER BY event_id
		   LIMIT $2
		 )
		 DELETE FROM gameplay_event_outbox event
		 USING expired
		 WHERE event.event_id = expired.event_id`,
		cutoff.UTC(),
		limit,
	)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	return int(rows), err
}

func (backend *postgresStoreBackend) FinalizeGameplayCommandWithEvent(ctx context.Context, sessionID string, commandSeq int, status GameplayCommandRecordStatus, outboundMessages []map[string]any, event *GameplayEvent) (bool, error) {
	created, err := backend.FinalizeGameplayCommandWithEvents(ctx, sessionID, commandSeq, status, outboundMessages, []*GameplayEvent{event})
	return created == 1, err
}

func (backend *postgresStoreBackend) FinalizeGameplayCommandWithEvents(ctx context.Context, sessionID string, commandSeq int, status GameplayCommandRecordStatus, outboundMessages []map[string]any, events []*GameplayEvent) (int, error) {
	outcomeJSON, err := json.Marshal(outboundMessages)
	if err != nil {
		return 0, err
	}
	createdCount := 0
	err = backend.RunSocialCommandTransaction(ctx, func(txCtx context.Context) error {
		executor := postgresExecutorFromContext(txCtx, backend.db)
		result, execErr := executor.ExecContext(txCtx,
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
		if execErr != nil {
			return execErr
		}
		rows, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return rowsErr
		}
		if rows == 0 {
			return errRecordNotFound
		}
		for _, event := range events {
			created, insertErr := insertGameplayEvent(txCtx, executor, event)
			if insertErr != nil {
				return insertErr
			}
			if created {
				createdCount++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return createdCount, nil
}

func (backend *postgresStoreBackend) FinalizeGameplayCommandWithChatAndEvents(ctx context.Context, sessionID string, commandSeq int, status GameplayCommandRecordStatus, outboundMessages []map[string]any, chatMessage ChatMessageRecord, events []*GameplayEvent) (int, error) {
	outcomeJSON, err := json.Marshal(outboundMessages)
	if err != nil {
		return 0, err
	}
	createdCount := 0
	err = backend.RunSocialCommandTransaction(ctx, func(txCtx context.Context) error {
		executor := postgresExecutorFromContext(txCtx, backend.db)
		result, execErr := executor.ExecContext(txCtx,
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
		if execErr != nil {
			return execErr
		}
		rows, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return rowsErr
		}
		if rows == 0 {
			return errRecordNotFound
		}
		if insertErr := insertChatMessage(txCtx, executor, normalizeChatMessageRecord(chatMessage)); insertErr != nil {
			return insertErr
		}
		for _, event := range events {
			created, insertErr := insertGameplayEvent(txCtx, executor, event)
			if insertErr != nil {
				return insertErr
			}
			if created {
				createdCount++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return createdCount, nil
}
