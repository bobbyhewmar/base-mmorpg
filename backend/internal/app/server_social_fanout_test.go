package app

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type crossInstanceSocialFixture struct {
	backend          *memoryStoreBackend
	storeA           *Store
	storeB           *Store
	serverA          *Server
	serverB          *Server
	actorCharacter   *Character
	targetCharacter  *Character
	actor            *partyTestClient
	target           *partyTestClient
	actorMessages    []map[string]any
	targetMessages   []map[string]any
	actorMessagesMu  sync.Mutex
	targetMessagesMu sync.Mutex
}

func newCrossInstanceSocialFixture(t *testing.T, prefix string) *crossInstanceSocialFixture {
	t.Helper()
	backend := newMemoryStoreBackend()
	storeA := newMemoryStoreWithBackend(backend)
	storeB := newMemoryStoreWithBackend(backend)
	actorCharacter, actorSession := createOwnershipTestCharacterAndSession(t, storeA, prefix+"_actor", prefix+"_actor_session")
	targetCharacter, targetSession := createOwnershipTestCharacterAndSession(t, storeA, prefix+"_target", prefix+"_target_session")
	actorOwned, err := storeA.GameplaySessions.AcquireOwnership(context.Background(), actorSession.ID, actorSession.AttachToken, "instance-a", time.Minute, 5*time.Minute)
	if err != nil {
		t.Fatalf("AcquireOwnership(actor) error=%v", err)
	}
	targetOwned, err := storeB.GameplaySessions.AcquireOwnership(context.Background(), targetSession.ID, targetSession.AttachToken, "instance-b", time.Minute, 5*time.Minute)
	if err != nil {
		t.Fatalf("AcquireOwnership(target) error=%v", err)
	}
	fixture := &crossInstanceSocialFixture{
		backend:         backend,
		storeA:          storeA,
		storeB:          storeB,
		serverA:         NewServerWithConfig(":0", "", storeA, ServerConfig{ServerInstanceID: "instance-a"}),
		serverB:         NewServerWithConfig(":0", "", storeB, ServerConfig{ServerInstanceID: "instance-b"}),
		actorCharacter:  actorCharacter,
		targetCharacter: targetCharacter,
		actor: &partyTestClient{
			session: actorOwned.Session,
			runtime: newAttachedRuntime(actorOwned.Session.ID, actorCharacter),
		},
		target: &partyTestClient{
			session: targetOwned.Session,
			runtime: newAttachedRuntime(targetOwned.Session.ID, targetCharacter),
		},
	}
	fixture.serverA.registerAttachedSession(actorOwned.Session.ID, fixture.actor.runtime, func(message map[string]any) bool {
		fixture.actorMessagesMu.Lock()
		defer fixture.actorMessagesMu.Unlock()
		fixture.actorMessages = append(fixture.actorMessages, message)
		return true
	}, actorOwned.Session)
	fixture.serverB.registerAttachedSession(targetOwned.Session.ID, fixture.target.runtime, func(message map[string]any) bool {
		fixture.targetMessagesMu.Lock()
		defer fixture.targetMessagesMu.Unlock()
		fixture.targetMessages = append(fixture.targetMessages, message)
		return true
	}, targetOwned.Session)
	t.Cleanup(func() {
		fixture.serverA.unregisterAttachedSession(actorOwned.Session.ID, actorOwned.Session.FencingToken)
		fixture.serverB.unregisterAttachedSession(targetOwned.Session.ID, targetOwned.Session.FencingToken)
	})
	return fixture
}

func (fixture *crossInstanceSocialFixture) targetMessageSnapshot() []map[string]any {
	fixture.targetMessagesMu.Lock()
	defer fixture.targetMessagesMu.Unlock()
	return cloneOutboundMessages(fixture.targetMessages)
}

func (fixture *crossInstanceSocialFixture) actorMessageSnapshot() []map[string]any {
	fixture.actorMessagesMu.Lock()
	defer fixture.actorMessagesMu.Unlock()
	return cloneOutboundMessages(fixture.actorMessages)
}

func (fixture *crossInstanceSocialFixture) resetMessages() {
	fixture.actorMessagesMu.Lock()
	fixture.actorMessages = nil
	fixture.actorMessagesMu.Unlock()
	fixture.targetMessagesMu.Lock()
	fixture.targetMessages = nil
	fixture.targetMessagesMu.Unlock()
}

func TestRemoteWhisperCrossesInstancesAndReplayDoesNotDuplicate(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "social_whisper")
	command := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "remote_whisper_command",
		CommandSeq:      1,
		Type:            "send_chat_message",
		Payload: mustMarshalCommandPayload(t, map[string]any{
			"channel":               chatChannelWhisper,
			"text":                  "  meet\nwithout <script>  ",
			"target_character_name": fixture.targetCharacter.Name,
		}),
	}
	first, _ := fixture.serverA.processGameplayCommandWithDedup(context.Background(), fixture.actor.session, fixture.actor.runtime, command)
	if extractRejectReason(first) != "" || findChatMessage(first, chatChannelWhisper) == nil {
		t.Fatalf("remote whisper result=%+v", first)
	}
	eventKey := "gameplay-command/" + fixture.actor.session.ID + "/1/social/chat-whisper/" + fixture.targetCharacter.ID
	event, err := fixture.storeA.GameplayEvents.GetByIdempotencyKey(context.Background(), eventKey)
	if err != nil || event.Type != remoteChatMessageEventType || event.TargetSessionID != fixture.target.session.ID {
		t.Fatalf("remote whisper event=%+v err=%v", event, err)
	}
	if records, listErr := fixture.storeA.ChatMessages.ListByCharacterID(context.Background(), fixture.actorCharacter.ID); listErr != nil || len(records) != 1 || records[0].Text != "meet without <script>" {
		t.Fatalf("remote whisper history=%+v err=%v", records, listErr)
	}

	replayed, shouldFanOut := fixture.serverA.processGameplayCommandWithDedup(context.Background(), fixture.actor.session, fixture.actor.runtime, command)
	if extractRejectReason(replayed) != "" || shouldFanOut || fixture.backend.nextGameplayEventID != 1 {
		t.Fatalf("remote whisper replay=%+v fanout=%v event_count=%d", replayed, shouldFanOut, fixture.backend.nextGameplayEventID)
	}
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/whisper-worker"); claimed != 1 {
		t.Fatalf("remote whisper claimed=%d", claimed)
	}
	targetMessages := fixture.targetMessageSnapshot()
	remoteMessage := findChatMessage(targetMessages, chatChannelWhisper)
	remoteEventID := float64(0)
	if remoteMessage != nil {
		remoteEventID, _ = remoteMessage["event_id"].(float64)
	}
	if remoteMessage == nil || int64(remoteEventID) != event.ID || remoteMessage["text"] != "meet without <script>" {
		t.Fatalf("remote whisper delivery=%+v", targetMessages)
	}
	if err := fixture.serverB.deliverGameplayEvent(context.Background(), event); err != nil {
		t.Fatalf("duplicate remote whisper delivery error=%v", err)
	}
	if countMessageKind(fixture.targetMessageSnapshot(), chatMessageKind) != 1 {
		t.Fatalf("duplicate remote whisper was rendered twice: %+v", fixture.targetMessageSnapshot())
	}
}

func TestRemotePartyInviteAndAcceptDeliverAuthoritativeState(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "social_party")
	aimPartyInviteTarget(fixture.actor, fixture.target)
	inviteOutbound := dispatchPartyCommand(t, fixture.serverA, fixture.actor, "remote_party_invite", 1, "invite_party_member", map[string]any{})
	if extractRejectReason(inviteOutbound) != "" {
		t.Fatalf("remote party invite=%+v", inviteOutbound)
	}
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/party-invite-worker"); claimed != 1 {
		t.Fatalf("remote party invite claimed=%d", claimed)
	}
	targetMessages := fixture.targetMessageSnapshot()
	inviteNotice := findPartyNotice(targetMessages, partyNoticeStatusInviteReceived)
	if inviteNotice == nil || inviteNotice["event_id"] == nil || findOutboundMessage(targetMessages, "delta") == nil {
		t.Fatalf("remote party invite delivery=%+v", targetMessages)
	}
	inviteID, _ := inviteNotice["invite_id"].(string)
	fixture.resetMessages()
	acceptOutbound := dispatchPartyCommand(t, fixture.serverB, fixture.target, "remote_party_accept", 1, "accept_party_invite", map[string]any{"invite_id": inviteID})
	if extractRejectReason(acceptOutbound) != "" {
		t.Fatalf("remote party accept=%+v", acceptOutbound)
	}
	if claimed := fixture.serverA.dispatchGameplayEventsOnce(context.Background(), "instance-a/party-accept-worker"); claimed != 1 {
		t.Fatalf("remote party accept claimed=%d", claimed)
	}
	actorMessages := fixture.actorMessageSnapshot()
	if findPartyNotice(actorMessages, partyNoticeStatusMemberJoined) == nil || findOutboundMessage(actorMessages, "delta") == nil {
		t.Fatalf("remote party accept delivery=%+v", actorMessages)
	}
	party, err := fixture.storeA.Parties.GetByCharacterID(context.Background(), fixture.targetCharacter.ID)
	if err != nil || party == nil {
		t.Fatalf("remote party membership missing: party=%+v err=%v", party, err)
	}
}

func TestRemotePartyDisconnectExpiryNoticeIsIdempotent(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "social_party_disconnect")
	aimPartyInviteTarget(fixture.actor, fixture.target)
	inviteOutbound := dispatchPartyCommand(t, fixture.serverA, fixture.actor, "remote_party_disconnect_invite", 1, "invite_party_member", map[string]any{})
	if extractRejectReason(inviteOutbound) != "" {
		t.Fatalf("remote party invite=%+v", inviteOutbound)
	}

	fixture.serverA.expirePartyInvitesForDisconnectedCharacter(context.Background(), fixture.actorCharacter.ID)
	fixture.serverA.expirePartyInvitesForDisconnectedCharacter(context.Background(), fixture.actorCharacter.ID)
	if fixture.backend.nextGameplayEventID != 2 {
		t.Fatalf("disconnect expiry event count=%d", fixture.backend.nextGameplayEventID)
	}
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/party-disconnect-worker"); claimed != 2 {
		t.Fatalf("remote party disconnect notices claimed=%d", claimed)
	}
	if findPartyNotice(fixture.targetMessageSnapshot(), partyNoticeStatusInviteExpired) == nil {
		t.Fatalf("remote party disconnect expiry delivery=%+v", fixture.targetMessageSnapshot())
	}
}

func TestRemoteClanInviteAcceptAndKickDeliverAuthoritativeState(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "social_clan")
	createOutbound := dispatchPartyCommand(t, fixture.serverA, fixture.actor, "remote_clan_create", 1, "create_clan", map[string]any{"name": "Remote Lantern"})
	if extractRejectReason(createOutbound) != "" {
		t.Fatalf("remote clan setup=%+v", createOutbound)
	}
	aimPartyInviteTarget(fixture.actor, fixture.target)
	inviteOutbound := dispatchPartyCommand(t, fixture.serverA, fixture.actor, "remote_clan_invite", 2, "invite_clan_member", map[string]any{})
	if extractRejectReason(inviteOutbound) != "" {
		t.Fatalf("remote clan invite=%+v", inviteOutbound)
	}
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/clan-invite-worker"); claimed != 1 {
		t.Fatalf("remote clan invite claimed=%d", claimed)
	}
	targetMessages := fixture.targetMessageSnapshot()
	inviteNotice := findClanNotice(targetMessages, clanNoticeStatusInviteReceived)
	if inviteNotice == nil || inviteNotice["event_id"] == nil || findOutboundMessage(targetMessages, "delta") == nil {
		t.Fatalf("remote clan invite delivery=%+v", targetMessages)
	}
	inviteID, _ := inviteNotice["invite_id"].(string)
	fixture.resetMessages()
	acceptOutbound := dispatchPartyCommand(t, fixture.serverB, fixture.target, "remote_clan_accept", 1, "accept_clan_invite", map[string]any{"invite_id": inviteID})
	if extractRejectReason(acceptOutbound) != "" {
		t.Fatalf("remote clan accept=%+v", acceptOutbound)
	}
	if claimed := fixture.serverA.dispatchGameplayEventsOnce(context.Background(), "instance-a/clan-accept-worker"); claimed != 1 {
		t.Fatalf("remote clan accept claimed=%d", claimed)
	}
	if findClanNotice(fixture.actorMessageSnapshot(), clanNoticeStatusMemberJoined) == nil {
		t.Fatalf("remote clan accept delivery=%+v", fixture.actorMessageSnapshot())
	}

	fixture.resetMessages()
	kickOutbound := dispatchPartyCommand(t, fixture.serverA, fixture.actor, "remote_clan_kick", 3, "kick_clan_member", map[string]any{"target_character_id": fixture.targetCharacter.ID})
	if extractRejectReason(kickOutbound) != "" {
		t.Fatalf("remote clan kick=%+v", kickOutbound)
	}
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/clan-kick-worker"); claimed != 1 {
		t.Fatalf("remote clan kick claimed=%d", claimed)
	}
	targetMessages = fixture.targetMessageSnapshot()
	if findClanNotice(targetMessages, clanNoticeStatusMemberKicked) == nil || findOutboundMessage(targetMessages, "delta") == nil {
		t.Fatalf("remote clan kick delivery=%+v", targetMessages)
	}
	if clan, err := fixture.storeA.Clans.GetByCharacterID(context.Background(), fixture.targetCharacter.ID); err == nil || clan != nil {
		t.Fatalf("remote clan kick did not clear membership: clan=%+v err=%v", clan, err)
	}
}

func TestRemoteClanDisconnectExpiryNoticeIsIdempotent(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "social_clan_disconnect")
	createOutbound := dispatchPartyCommand(t, fixture.serverA, fixture.actor, "remote_clan_disconnect_create", 1, "create_clan", map[string]any{"name": "Remote Ember"})
	if extractRejectReason(createOutbound) != "" {
		t.Fatalf("remote clan setup=%+v", createOutbound)
	}
	aimPartyInviteTarget(fixture.actor, fixture.target)
	inviteOutbound := dispatchPartyCommand(t, fixture.serverA, fixture.actor, "remote_clan_disconnect_invite", 2, "invite_clan_member", map[string]any{})
	if extractRejectReason(inviteOutbound) != "" {
		t.Fatalf("remote clan invite=%+v", inviteOutbound)
	}

	fixture.serverA.expireClanInvitesForDisconnectedCharacter(context.Background(), fixture.actorCharacter.ID)
	fixture.serverA.expireClanInvitesForDisconnectedCharacter(context.Background(), fixture.actorCharacter.ID)
	if fixture.backend.nextGameplayEventID != 2 {
		t.Fatalf("disconnect expiry event count=%d", fixture.backend.nextGameplayEventID)
	}
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/clan-disconnect-worker"); claimed != 2 {
		t.Fatalf("remote clan disconnect notices claimed=%d", claimed)
	}
	if findClanNotice(fixture.targetMessageSnapshot(), clanNoticeStatusInviteExpired) == nil {
		t.Fatalf("remote clan disconnect expiry delivery=%+v", fixture.targetMessageSnapshot())
	}
}

func TestRemoteSocialDeliveryDeadLettersStableStaleOwner(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "social_stale")
	notice := chatMessagePayload("", 0, chatChannelWhisper, fixture.actorCharacter.ID, fixture.actorCharacter.Name, fixture.targetCharacter.ID, fixture.targetCharacter.Name, "", "ownership drift", time.Now().UTC())
	payload, err := json.Marshal(remoteSocialDeliveryPayload{RecipientCharacterID: fixture.targetCharacter.ID, Message: notice})
	if err != nil {
		t.Fatalf("json.Marshal() error=%v", err)
	}
	event := &GameplayEvent{
		IdempotencyKey:         "social-stale-owner-event",
		Type:                   remoteChatMessageEventType,
		Payload:                payload,
		TargetServerInstanceID: "instance-b",
		TargetSessionID:        fixture.target.session.ID,
		TargetCharacterID:      fixture.targetCharacter.ID,
	}
	if created, createErr := fixture.storeA.GameplayEvents.Create(context.Background(), event); createErr != nil || !created {
		t.Fatalf("GameplayEvents.Create() created=%v err=%v", created, createErr)
	}
	if released, releaseErr := fixture.storeB.GameplaySessions.ReleaseOwnership(context.Background(), fixture.targetCharacter.ID, fixture.target.session.ID, "instance-b", fixture.target.session.FencingToken); releaseErr != nil || !released {
		t.Fatalf("ReleaseOwnership() released=%v err=%v", released, releaseErr)
	}
	replacement := createOwnershipTestSession(t, fixture.storeB, fixture.targetCharacter, "social_stale_replacement_session")
	if _, acquireErr := fixture.storeB.GameplaySessions.AcquireOwnership(context.Background(), replacement.ID, replacement.AttachToken, "instance-c", time.Minute, 5*time.Minute); acquireErr != nil {
		t.Fatalf("AcquireOwnership(replacement) error=%v", acquireErr)
	}
	fixture.serverB.config.GameplayEventMaxRetries = 1
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/stale-worker"); claimed != 1 {
		t.Fatalf("stale event claimed=%d", claimed)
	}
	persisted, err := fixture.storeA.GameplayEvents.GetByIdempotencyKey(context.Background(), event.IdempotencyKey)
	if err != nil || persisted.DeadLetteredAt.IsZero() || persisted.LastError != "social.recipient_stale_owner" {
		t.Fatalf("stale event result=%+v err=%v", persisted, err)
	}
	if countMessageKind(fixture.targetMessageSnapshot(), chatMessageKind) != 0 {
		t.Fatalf("stale owner received remote chat: %+v", fixture.targetMessageSnapshot())
	}
}

func TestWhisperToOfflineRecipientRejectsWithoutOutboxFallback(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "social_offline")
	if released, err := fixture.storeB.GameplaySessions.ReleaseOwnership(context.Background(), fixture.targetCharacter.ID, fixture.target.session.ID, "instance-b", fixture.target.session.FencingToken); err != nil || !released {
		t.Fatalf("ReleaseOwnership() released=%v err=%v", released, err)
	}

	command := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "offline_whisper_command",
		CommandSeq:      1,
		Type:            "send_chat_message",
		Payload: mustMarshalCommandPayload(t, map[string]any{
			"channel":               chatChannelWhisper,
			"text":                  "no local fallback",
			"target_character_name": fixture.targetCharacter.Name,
		}),
	}
	outbound, _ := fixture.serverA.processGameplayCommandWithDedup(context.Background(), fixture.actor.session, fixture.actor.runtime, command)
	if reason := extractRejectReason(outbound); reason != "chat.whisper_target_not_found" {
		t.Fatalf("offline whisper reason=%q outbound=%+v", reason, outbound)
	}
	if fixture.backend.nextGameplayEventID != 0 {
		t.Fatalf("offline whisper unexpectedly created %d outbox events", fixture.backend.nextGameplayEventID)
	}
	if countMessageKind(fixture.targetMessageSnapshot(), chatMessageKind) != 0 {
		t.Fatalf("offline whisper used local socket fallback: %+v", fixture.targetMessageSnapshot())
	}
}
