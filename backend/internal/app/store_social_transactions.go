package app

import (
	"context"
	"database/sql"
)

type socialCommandTransactionRunner interface {
	RunSocialCommandTransaction(ctx context.Context, run func(context.Context) error) error
}

type postgresTransactionContextKey struct{}

type postgresExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func postgresExecutorFromContext(ctx context.Context, fallback *sql.DB) postgresExecutor {
	if tx, _ := ctx.Value(postgresTransactionContextKey{}).(*sql.Tx); tx != nil {
		return tx
	}
	return fallback
}

func (backend *postgresStoreBackend) RunSocialCommandTransaction(ctx context.Context, run func(context.Context) error) error {
	if existing, _ := ctx.Value(postgresTransactionContextKey{}).(*sql.Tx); existing != nil {
		return run(ctx)
	}
	tx, err := backend.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := run(context.WithValue(ctx, postgresTransactionContextKey{}, tx)); err != nil {
		return err
	}
	return tx.Commit()
}

type memorySocialSnapshot struct {
	commandRecords      map[string]*GameplayCommandRecord
	gameplayEvents      map[int64]*GameplayEvent
	gameplayEventByKey  map[string]int64
	nextGameplayEventID int64
	parties             map[string]*Party
	partyMembers        map[string][]PartyMember
	partyByCharacter    map[string]string
	partyInvites        map[string]*PartyInvite
	clans               map[string]*Clan
	clanMembers         map[string][]ClanMember
	clanByCharacter     map[string]string
	clanInvites         map[string]*ClanInvite
	clanByName          map[string]string
}

func clonePointerMap[T any](source map[string]*T) map[string]*T {
	result := make(map[string]*T, len(source))
	for key, value := range source {
		if value == nil {
			result[key] = nil
			continue
		}
		clone := *value
		result[key] = &clone
	}
	return result
}

func cloneValueMap[T any](source map[string]T) map[string]T {
	result := make(map[string]T, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneSliceMap[T any](source map[string][]T) map[string][]T {
	result := make(map[string][]T, len(source))
	for key, value := range source {
		result[key] = append([]T(nil), value...)
	}
	return result
}

func cloneGameplayEventMap(source map[int64]*GameplayEvent) map[int64]*GameplayEvent {
	result := make(map[int64]*GameplayEvent, len(source))
	for key, value := range source {
		result[key] = cloneGameplayEvent(value)
	}
	return result
}

func (backend *memoryStoreBackend) socialSnapshot() memorySocialSnapshot {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return memorySocialSnapshot{
		commandRecords:      clonePointerMap(backend.commandRecords),
		gameplayEvents:      cloneGameplayEventMap(backend.gameplayEvents),
		gameplayEventByKey:  cloneValueMap(backend.gameplayEventByKey),
		nextGameplayEventID: backend.nextGameplayEventID,
		parties:             clonePointerMap(backend.parties),
		partyMembers:        cloneSliceMap(backend.partyMembers),
		partyByCharacter:    cloneValueMap(backend.partyByCharacter),
		partyInvites:        clonePointerMap(backend.partyInvites),
		clans:               clonePointerMap(backend.clans),
		clanMembers:         cloneSliceMap(backend.clanMembers),
		clanByCharacter:     cloneValueMap(backend.clanByCharacter),
		clanInvites:         clonePointerMap(backend.clanInvites),
		clanByName:          cloneValueMap(backend.clanByName),
	}
}

func (backend *memoryStoreBackend) restoreSocialSnapshot(snapshot memorySocialSnapshot) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.commandRecords = snapshot.commandRecords
	backend.gameplayEvents = snapshot.gameplayEvents
	backend.gameplayEventByKey = snapshot.gameplayEventByKey
	backend.nextGameplayEventID = snapshot.nextGameplayEventID
	backend.parties = snapshot.parties
	backend.partyMembers = snapshot.partyMembers
	backend.partyByCharacter = snapshot.partyByCharacter
	backend.partyInvites = snapshot.partyInvites
	backend.clans = snapshot.clans
	backend.clanMembers = snapshot.clanMembers
	backend.clanByCharacter = snapshot.clanByCharacter
	backend.clanInvites = snapshot.clanInvites
	backend.clanByName = snapshot.clanByName
}

func (backend *memoryStoreBackend) RunSocialCommandTransaction(ctx context.Context, run func(context.Context) error) error {
	backend.socialTxMu.Lock()
	defer backend.socialTxMu.Unlock()
	snapshot := backend.socialSnapshot()
	if err := run(ctx); err != nil {
		backend.restoreSocialSnapshot(snapshot)
		return err
	}
	return nil
}
