package app

import (
	"context"
	"errors"
	"time"
)

const (
	defaultAuditQueryLimit = 50
	maxAuditQueryLimit     = 200
)

var tradeActionTypes = []string{
	"player_trade_offer",
	"player_trade_accept",
	"player_trade_decline",
	"player_trade_send",
	"player_trade_receive",
}

type StorageTransferRecord struct {
	ID                 string
	CharacterID        string
	AccountID          string
	SourceItemID       string
	TemplateID         string
	Quantity           int
	ItemQuantityBefore int
	ItemQuantityAfter  int
	FromContainerKind  ItemContainer
	ToContainerKind    ItemContainer
	TransferType       string
	CounterpartyEntity string
	SessionID          string
	CommandID          string
	CommandSeq         int
	CreatedAt          time.Time
}

type ActionLogRecord struct {
	ID                    string
	CharacterID           string
	AccountID             string
	ActionType            string
	ReferenceID           string
	CounterpartyEntity    string
	ItemInstanceID        string
	TemplateID            string
	Quantity              int
	ItemQuantityBefore    int
	ItemQuantityAfter     int
	CurrencyTemplateID    string
	CurrencyAmount        int
	CurrencyBalanceBefore int
	CurrencyBalanceAfter  int
	FromContainerKind     ItemContainer
	ToContainerKind       ItemContainer
	SessionID             string
	CommandID             string
	CommandSeq            int
	CreatedAt             time.Time
}

type StorageTransferQuery struct {
	CharacterID    string
	SourceItemID   string
	TransferType   string
	OccurredAfter  *time.Time
	OccurredBefore *time.Time
	Limit          int
	Offset         int
}

type ActionLogQuery struct {
	CharacterID         string
	InvolvedCharacterID string
	ItemInstanceID      string
	ActionType          string
	ActionTypes         []string
	ReferenceID         string
	OccurredAfter       *time.Time
	OccurredBefore      *time.Time
	Limit               int
	Offset              int
}

type commandAuditMetadata struct {
	SessionID  string
	CommandID  string
	CommandSeq int
}

type commandAuditContextKey struct{}

func withCommandAuditMetadata(ctx context.Context, metadata commandAuditMetadata) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, commandAuditContextKey{}, metadata)
}

func commandAuditMetadataFromContext(ctx context.Context) commandAuditMetadata {
	if ctx == nil {
		return commandAuditMetadata{}
	}
	metadata, _ := ctx.Value(commandAuditContextKey{}).(commandAuditMetadata)
	return metadata
}

func normalizeAuditPagination(limit int, offset int) (int, int) {
	if limit <= 0 {
		limit = defaultAuditQueryLimit
	}
	if limit > maxAuditQueryLimit {
		limit = maxAuditQueryLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func isTradeActionType(actionType string) bool {
	for _, candidate := range tradeActionTypes {
		if candidate == actionType {
			return true
		}
	}
	return false
}

type EconomyAuditService struct {
	actionLogs       ActionLogRepository
	storageTransfers StorageTransferRecordRepository
}

func NewEconomyAuditService(store *Store) *EconomyAuditService {
	if store == nil {
		return &EconomyAuditService{}
	}
	return &EconomyAuditService{
		actionLogs:       store.ActionLogs,
		storageTransfers: store.StorageTransfers,
	}
}

func (service *EconomyAuditService) ListEvents(ctx context.Context, query ActionLogQuery) ([]ActionLogRecord, error) {
	if service == nil || service.actionLogs == nil {
		return nil, errors.New("action log repository unavailable")
	}
	query.Limit, query.Offset = normalizeAuditPagination(query.Limit, query.Offset)
	return service.actionLogs.ListByFilter(ctx, query)
}

func (service *EconomyAuditService) ListWarehouseTransfers(ctx context.Context, query StorageTransferQuery) ([]StorageTransferRecord, error) {
	if service == nil || service.storageTransfers == nil {
		return nil, errors.New("storage transfer repository unavailable")
	}
	query.Limit, query.Offset = normalizeAuditPagination(query.Limit, query.Offset)
	return service.storageTransfers.ListByFilter(ctx, query)
}

func (service *EconomyAuditService) ListTrades(ctx context.Context, characterID string, limit int, offset int, occurredAfter *time.Time, occurredBefore *time.Time) ([]ActionLogRecord, error) {
	if service == nil || service.actionLogs == nil {
		return nil, errors.New("action log repository unavailable")
	}
	if characterID == "" {
		return nil, errors.New("character_id is required")
	}
	limit, offset = normalizeAuditPagination(limit, offset)
	return service.actionLogs.ListByFilter(ctx, ActionLogQuery{
		InvolvedCharacterID: characterID,
		ActionTypes:         tradeActionTypes,
		OccurredAfter:       occurredAfter,
		OccurredBefore:      occurredBefore,
		Limit:               limit,
		Offset:              offset,
	})
}
