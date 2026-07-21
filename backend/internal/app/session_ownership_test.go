package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func createOwnershipTestCharacterAndSession(t *testing.T, store *Store, characterID string, sessionID string) (*Character, *Session) {
	t.Helper()
	character := &Character{
		ID:           characterID,
		AccountID:    "account_" + characterID,
		Name:         "Owner " + characterID,
		Race:         "Human",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		CurrentCP:    80,
		CurrentHP:    122,
		CurrentMP:    58,
		LastRegionID: "dawn_plaza",
		PositionX:    10,
		PositionZ:    10,
		IsEnterable:  true,
	}
	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed() error = %v", err)
	}
	session := &Session{
		ID:              sessionID,
		AccountID:       character.AccountID,
		CharacterID:     character.ID,
		AttachToken:     "attach_" + sessionID,
		AttachExpiresAt: time.Now().Add(5 * time.Minute),
		Status:          sessionStatusPendingAttach,
	}
	if err := store.GameplaySessions.Create(context.Background(), session); err != nil {
		t.Fatalf("GameplaySessions.Create() error = %v", err)
	}
	return character, session
}

func createOwnershipTestSession(t *testing.T, store *Store, character *Character, sessionID string) *Session {
	t.Helper()
	session := &Session{
		ID:              sessionID,
		AccountID:       character.AccountID,
		CharacterID:     character.ID,
		AttachToken:     "attach_" + sessionID,
		AttachExpiresAt: time.Now().Add(5 * time.Minute),
		Status:          sessionStatusPendingAttach,
	}
	if err := store.GameplaySessions.Create(context.Background(), session); err != nil {
		t.Fatalf("GameplaySessions.Create() error = %v", err)
	}
	return session
}

func TestMemorySessionOwnershipFencesConcurrentAttachRenewsAndExpires(t *testing.T) {
	store := newMemoryStore()
	character, firstSession := createOwnershipTestCharacterAndSession(t, store, "character_lease", "session_lease_a")
	secondSession := createOwnershipTestSession(t, store, character, "session_lease_b")

	first, err := store.GameplaySessions.AcquireOwnership(context.Background(), firstSession.ID, firstSession.AttachToken, "instance-a", 60*time.Millisecond, 5*time.Minute)
	if err != nil {
		t.Fatalf("AcquireOwnership(first) error = %v", err)
	}
	if first.Change != sessionOwnershipAcquired || first.Ownership.FencingToken != 1 {
		t.Fatalf("unexpected first acquisition: %+v", first)
	}
	if _, err := store.GameplaySessions.AcquireOwnership(context.Background(), first.Session.ID, first.Session.AttachToken, "instance-b", time.Minute, 5*time.Minute); !errors.Is(err, errOwnershipConflict) {
		t.Fatalf("remote instance replaced an unexpired same-session owner: %v", err)
	}
	if _, err := store.GameplaySessions.AcquireOwnership(context.Background(), secondSession.ID, secondSession.AttachToken, "instance-b", time.Minute, 5*time.Minute); !errors.Is(err, errOwnershipConflict) {
		t.Fatalf("expected active owner conflict, got %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	renewed, err := store.GameplaySessions.RenewOwnership(context.Background(), character.ID, first.Session.ID, "instance-a", first.Ownership.FencingToken, "dawn_plaza", runtimePoint{}, 60*time.Millisecond, 5*time.Minute)
	if err != nil {
		t.Fatalf("RenewOwnership() error = %v", err)
	}
	if renewed.FencingToken != first.Ownership.FencingToken || !renewed.LeaseExpiresAt.After(first.Ownership.LeaseExpiresAt) {
		t.Fatalf("renewal changed fence or failed to extend lease: before=%+v after=%+v", first.Ownership, renewed)
	}

	time.Sleep(90 * time.Millisecond)
	replacement, err := store.GameplaySessions.AcquireOwnership(context.Background(), secondSession.ID, secondSession.AttachToken, "instance-b", time.Minute, 5*time.Minute)
	if err != nil {
		t.Fatalf("AcquireOwnership(replacement) error = %v", err)
	}
	if replacement.Change != sessionOwnershipReplaced || replacement.Ownership.FencingToken != 2 || replacement.Previous == nil {
		t.Fatalf("unexpected replacement acquisition: %+v", replacement)
	}
	if _, err := store.GameplaySessions.RenewOwnership(context.Background(), character.ID, first.Session.ID, "instance-a", first.Ownership.FencingToken, "dawn_plaza", runtimePoint{}, time.Minute, 5*time.Minute); !errors.Is(err, errOwnershipStale) {
		t.Fatalf("expected stale old owner, got %v", err)
	}
	persistedFirst, err := store.GameplaySessions.GetByID(context.Background(), first.Session.ID)
	if err != nil {
		t.Fatalf("GetByID(first) error = %v", err)
	}
	if persistedFirst.Status != sessionStatusClosed {
		t.Fatalf("expected replaced session to be closed, got %+v", persistedFirst)
	}

	released, err := store.GameplaySessions.ReleaseOwnership(context.Background(), character.ID, replacement.Session.ID, "instance-b", replacement.Ownership.FencingToken)
	if err != nil || !released {
		t.Fatalf("ReleaseOwnership(first) released=%v error=%v", released, err)
	}
	released, err = store.GameplaySessions.ReleaseOwnership(context.Background(), character.ID, replacement.Session.ID, "instance-b", replacement.Ownership.FencingToken)
	if err != nil || released {
		t.Fatalf("double release must be idempotent, released=%v error=%v", released, err)
	}
	thirdSession := createOwnershipTestSession(t, store, character, "session_lease_c")
	third, err := store.GameplaySessions.AcquireOwnership(context.Background(), thirdSession.ID, thirdSession.AttachToken, "instance-c", time.Minute, 5*time.Minute)
	if err != nil {
		t.Fatalf("AcquireOwnership(after release) error = %v", err)
	}
	if third.Ownership.FencingToken != replacement.Ownership.FencingToken+1 {
		t.Fatalf("durable fence reset after release: replacement=%+v third=%+v", replacement.Ownership, third.Ownership)
	}
}

func TestConcurrentAttachAcrossInstancesHasOneDurableWinner(t *testing.T) {
	store := newMemoryStore()
	_, session := createOwnershipTestCharacterAndSession(t, store, "character_race", "session_race")
	serverA := NewServerWithConfig(":0", "", store, ServerConfig{ServerInstanceID: "instance-a"})
	serverB := NewServerWithConfig(":0", "", store, ServerConfig{ServerInstanceID: "instance-b"})

	start := make(chan struct{})
	type result struct {
		instance string
		session  *Session
		err      error
	}
	results := make(chan result, 2)
	var wait sync.WaitGroup
	for instance, server := range map[string]*Server{"instance-a": serverA, "instance-b": serverB} {
		wait.Add(1)
		go func(instance string, server *Server) {
			defer wait.Done()
			<-start
			attachedSession, _, err := server.attachSession(session.ID, session.AttachToken)
			results <- result{instance: instance, session: attachedSession, err: err}
		}(instance, server)
	}
	close(start)
	wait.Wait()
	close(results)

	winners := 0
	losers := 0
	winningInstance := ""
	for outcome := range results {
		if outcome.err == nil {
			winners++
			winningInstance = outcome.instance
			continue
		}
		if outcome.err.Error() != "session.invalid_attach_token" {
			t.Fatalf("unexpected attach loser error: %v", outcome.err)
		}
		losers++
	}
	if winners != 1 || losers != 1 {
		t.Fatalf("expected one winner and one loser, winners=%d losers=%d", winners, losers)
	}
	ownership, err := store.GameplaySessions.GetActiveOwnershipByCharacterID(context.Background(), session.CharacterID)
	if err != nil {
		t.Fatalf("GetActiveOwnershipByCharacterID() error = %v", err)
	}
	if ownership.ServerInstanceID != winningInstance || ownership.FencingToken != 1 {
		t.Fatalf("durable winner mismatch: winner=%s ownership=%+v", winningInstance, ownership)
	}
}

func TestStaleOwnerCommandRejectsBeforeAckAndDoesNotReserveDedup(t *testing.T) {
	store := newMemoryStore()
	character, firstSession := createOwnershipTestCharacterAndSession(t, store, "character_stale_command", "session_stale_a")
	secondSession := createOwnershipTestSession(t, store, character, "session_stale_b")
	first, err := store.GameplaySessions.AcquireOwnership(context.Background(), firstSession.ID, firstSession.AttachToken, "instance-a", 30*time.Millisecond, 5*time.Minute)
	if err != nil {
		t.Fatalf("AcquireOwnership(first) error = %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := store.GameplaySessions.AcquireOwnership(context.Background(), secondSession.ID, secondSession.AttachToken, "instance-b", time.Minute, 5*time.Minute); err != nil {
		t.Fatalf("AcquireOwnership(takeover) error = %v", err)
	}

	serverA := NewServerWithConfig(":0", "", store, ServerConfig{ServerInstanceID: "instance-a"})
	runtime := newAttachedRuntime(first.Session.ID, character)
	command := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "command_stale_owner",
		CommandSeq:      1,
		Type:            "clear_target",
		Payload:         json.RawMessage(`{}`),
	}
	outbound, shouldFanOut := serverA.processGameplayCommandWithDedup(context.Background(), first.Session, runtime, command)
	if shouldFanOut || len(outbound) != 1 || extractRejectReason(outbound) != "session.stale_owner" {
		t.Fatalf("expected early stale-owner reject, outbound=%+v fanout=%v", outbound, shouldFanOut)
	}
	if runtime.expectedCommandSeqValue() != 1 {
		t.Fatalf("stale owner command must not advance sequence, got %d", runtime.expectedCommandSeqValue())
	}
	if _, err := store.GameplayCommands.GetBySessionAndSeq(context.Background(), first.Session.ID, command.CommandSeq); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("stale owner command must not reserve dedup, got %v", err)
	}
}

func TestRemoteOwnedPlayerRejectsTargetAndPvPWithoutLocalFallback(t *testing.T) {
	store := newMemoryStore()
	actorCharacter, actorSession := createOwnershipTestCharacterAndSession(t, store, "character_local_actor", "session_local_actor")
	targetCharacter, targetSession := createOwnershipTestCharacterAndSession(t, store, "character_remote_target", "session_remote_target")
	actor, err := store.GameplaySessions.AcquireOwnership(context.Background(), actorSession.ID, actorSession.AttachToken, "instance-a", time.Minute, 5*time.Minute)
	if err != nil {
		t.Fatalf("AcquireOwnership(actor) error = %v", err)
	}
	if _, err := store.GameplaySessions.AcquireOwnership(context.Background(), targetSession.ID, targetSession.AttachToken, "instance-b", time.Minute, 5*time.Minute); err != nil {
		t.Fatalf("AcquireOwnership(target) error = %v", err)
	}
	serverA := NewServerWithConfig(":0", "", store, ServerConfig{ServerInstanceID: "instance-a"})
	runtime := newAttachedRuntime(actor.Session.ID, actorCharacter)
	runtime.knownEntities[targetCharacter.ID] = runtimeEntity{
		EntityID:   targetCharacter.ID,
		EntityType: "player",
		TemplateID: "remote_player",
		Position:   runtimePoint{X: 12, Z: 10},
		State:      map[string]any{"hp": 122, "alive": true},
	}
	serverA.registerAttachedSession(actor.Session.ID, runtime, func(map[string]any) bool { return true }, actor.Session)
	defer serverA.unregisterAttachedSession(actor.Session.ID, actor.Session.FencingToken)

	selectCommand := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "command_remote_select",
		CommandSeq:      1,
		Type:            "select_target",
		Payload:         json.RawMessage(`{"target_id":"character_remote_target"}`),
	}
	selected, _ := serverA.processGameplayCommandWithDedup(context.Background(), actor.Session, runtime, selectCommand)
	if extractRejectReason(selected) != "presence.target_remote" || runtime.targetID != "" {
		t.Fatalf("remote select must reject without local target success: outbound=%+v target=%q", selected, runtime.targetID)
	}

	attackCommand := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "command_remote_attack",
		CommandSeq:      2,
		Type:            "basic_attack",
		Payload:         json.RawMessage(`{"target_id":"character_remote_target"}`),
	}
	attacked, _ := serverA.processGameplayCommandWithDedup(context.Background(), actor.Session, runtime, attackCommand)
	if extractRejectReason(attacked) != "presence.target_remote" {
		t.Fatalf("remote PvP must reject with stable remote-owner reason, got %+v", attacked)
	}
	persistedTarget, err := store.Characters.GetByID(context.Background(), targetCharacter.ID)
	if err != nil {
		t.Fatalf("Characters.GetByID(target) error = %v", err)
	}
	if persistedTarget.CurrentCP != targetCharacter.CurrentCP || persistedTarget.CurrentHP != targetCharacter.CurrentHP {
		t.Fatalf("remote PvP fallback mutated target: before=%+v after=%+v", targetCharacter, persistedTarget)
	}
}

func TestRemoteOwnedPlayerReceivesPartyAndClanInviteThroughOutbox(t *testing.T) {
	store := newMemoryStore()
	actorCharacter, actorSession := createOwnershipTestCharacterAndSession(t, store, "character_remote_inviter", "session_remote_inviter")
	targetCharacter, targetSession := createOwnershipTestCharacterAndSession(t, store, "character_remote_invitee", "session_remote_invitee")
	actor, err := store.GameplaySessions.AcquireOwnership(context.Background(), actorSession.ID, actorSession.AttachToken, "instance-a", time.Minute, 5*time.Minute)
	if err != nil {
		t.Fatalf("AcquireOwnership(actor) error = %v", err)
	}
	if _, err := store.GameplaySessions.AcquireOwnership(context.Background(), targetSession.ID, targetSession.AttachToken, "instance-b", time.Minute, 5*time.Minute); err != nil {
		t.Fatalf("AcquireOwnership(target) error = %v", err)
	}
	serverA := NewServerWithConfig(":0", "", store, ServerConfig{ServerInstanceID: "instance-a"})
	runtime := newAttachedRuntime(actor.Session.ID, actorCharacter)
	runtime.targetID = targetCharacter.ID
	runtime.knownEntities[targetCharacter.ID] = runtimeEntity{
		EntityID:   targetCharacter.ID,
		EntityType: "player",
		TemplateID: "remote_player",
		Position:   runtimePoint{X: 12, Z: 10},
		State:      map[string]any{"hp": 122, "alive": true},
	}
	serverA.registerAttachedSession(actor.Session.ID, runtime, func(map[string]any) bool { return true }, actor.Session)
	defer serverA.unregisterAttachedSession(actor.Session.ID, actor.Session.FencingToken)

	partyInvite := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "command_remote_party_invite",
		CommandSeq:      1,
		Type:            "invite_party_member",
		Payload:         json.RawMessage(`{}`),
	}
	partyOutbound, _ := serverA.processGameplayCommandWithDedup(context.Background(), actor.Session, runtime, partyInvite)
	if extractRejectReason(partyOutbound) != "" || gameplayCommandRecordStatusFromOutbound(partyOutbound) != gameplayCommandRecordStatusApplied {
		t.Fatalf("remote party invite must apply authoritatively, got %+v", partyOutbound)
	}
	partyEventKey := "gameplay-command/" + actor.Session.ID + "/1/social/party-invite-received/" + targetCharacter.ID
	if event, eventErr := store.GameplayEvents.GetByIdempotencyKey(context.Background(), partyEventKey); eventErr != nil || event.Type != remotePartyNoticeEventType || event.TargetServerInstanceID != "instance-b" {
		t.Fatalf("remote party invite event mismatch: event=%+v err=%v", event, eventErr)
	}

	createClan := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "command_remote_inviter_create_clan",
		CommandSeq:      2,
		Type:            "create_clan",
		Payload:         json.RawMessage(`{"name":"Remote Fence"}`),
	}
	clanCreated, _ := serverA.processGameplayCommandWithDedup(context.Background(), actor.Session, runtime, createClan)
	if extractRejectReason(clanCreated) != "" || gameplayCommandRecordStatusFromOutbound(clanCreated) != gameplayCommandRecordStatusApplied {
		t.Fatalf("expected clan setup to apply, got %+v", clanCreated)
	}
	clanInvite := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "command_remote_clan_invite",
		CommandSeq:      3,
		Type:            "invite_clan_member",
		Payload:         json.RawMessage(`{}`),
	}
	clanOutbound, _ := serverA.processGameplayCommandWithDedup(context.Background(), actor.Session, runtime, clanInvite)
	if extractRejectReason(clanOutbound) != "" || gameplayCommandRecordStatusFromOutbound(clanOutbound) != gameplayCommandRecordStatusApplied {
		t.Fatalf("remote clan invite must apply authoritatively, got %+v", clanOutbound)
	}
	if invites, invitesErr := store.Clans.ListPendingInvitesByInvitee(context.Background(), targetCharacter.ID, time.Now()); invitesErr != nil || len(invites) != 1 {
		t.Fatalf("remote clan invite did not persist canonical state: invites=%+v err=%v", invites, invitesErr)
	}
	clanEventKey := "gameplay-command/" + actor.Session.ID + "/3/social/clan-invite-received/" + targetCharacter.ID
	if event, eventErr := store.GameplayEvents.GetByIdempotencyKey(context.Background(), clanEventKey); eventErr != nil || event.Type != remoteClanNoticeEventType || event.TargetServerInstanceID != "instance-b" {
		t.Fatalf("remote clan invite event mismatch: event=%+v err=%v", event, eventErr)
	}
}

func TestDoubleUnregisterIsIdempotent(t *testing.T) {
	store := newMemoryStore()
	character, session := createOwnershipTestCharacterAndSession(t, store, "character_unregister", "session_unregister")
	server := NewServer(":0", "", store)
	runtime := newAttachedRuntime(session.ID, character)
	server.registerAttachedSession(session.ID, runtime, func(map[string]any) bool { return true })

	server.unregisterAttachedSession(session.ID)
	server.unregisterAttachedSession(session.ID)
	metrics := server.observer.renderPrometheus()
	if strings.Contains(metrics, "l2bg_attached_sessions_active -1") || strings.Contains(metrics, "l2bg_region_occupancy{region_id=\"dawn_plaza\"} -1") {
		t.Fatalf("double unregister decremented gauges below zero:\n%s", metrics)
	}
}
