package app

import (
	"context"
	"encoding/json"
	"errors"
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

func TestRegionChatFansOutLocallyAndAcrossInstancesWithDurableDedup(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "social_region")
	remoteOtherCharacter, remoteOtherSession := createOwnershipTestCharacterAndSession(t, fixture.storeA, "social_region_remote_other", "social_region_remote_other_session")
	remoteOtherOwned, err := fixture.storeB.GameplaySessions.AcquireOwnership(context.Background(), remoteOtherSession.ID, remoteOtherSession.AttachToken, "instance-b", time.Minute, 5*time.Minute)
	if err != nil {
		t.Fatalf("AcquireOwnership(remote other region) error=%v", err)
	}
	if _, err := fixture.storeB.GameplaySessions.RenewOwnership(context.Background(), remoteOtherCharacter.ID, remoteOtherOwned.Session.ID, "instance-b", remoteOtherOwned.Session.FencingToken, "gate_road", time.Minute, 5*time.Minute); err != nil {
		t.Fatalf("RenewOwnership(remote other region) error=%v", err)
	}
	remoteOtherCharacter.LastRegionID = "gate_road"
	remoteOtherRuntime := newAttachedRuntime(remoteOtherOwned.Session.ID, remoteOtherCharacter)
	remoteOtherMessages := make([]map[string]any, 0)
	fixture.serverB.registerAttachedSession(remoteOtherOwned.Session.ID, remoteOtherRuntime, func(message map[string]any) bool {
		remoteOtherMessages = append(remoteOtherMessages, message)
		return true
	}, remoteOtherOwned.Session)
	t.Cleanup(func() {
		fixture.serverB.unregisterAttachedSession(remoteOtherOwned.Session.ID, remoteOtherOwned.Session.FencingToken)
	})
	local := stageChatTestClient(t, fixture.serverA, fixture.storeA, "social_region_local_session", &Character{
		ID:           "social_region_local",
		AccountID:    "account_social_region_local",
		Name:         "Local Listener",
		BaseClass:    "Fighter",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
		CurrentCP:    80,
		CurrentHP:    122,
		CurrentMP:    58,
	})
	otherRegion := stageChatTestClient(t, fixture.serverA, fixture.storeA, "social_region_other_session", &Character{
		ID:           "social_region_other",
		AccountID:    "account_social_region_other",
		Name:         "Other Region Listener",
		BaseClass:    "Mage",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "gate_road",
		CurrentCP:    80,
		CurrentHP:    122,
		CurrentMP:    58,
	})
	fixture.resetMessages()
	local.resetMessages()
	otherRegion.resetMessages()

	command := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "remote_region_chat_command",
		CommandSeq:      1,
		Type:            "send_chat_message",
		Payload: mustMarshalCommandPayload(t, map[string]any{
			"channel": chatChannelRegion,
			"text":    "  same\nregion <b>only</b>  ",
		}),
	}
	first, shouldFanOut := fixture.serverA.processGameplayCommandWithDedup(context.Background(), fixture.actor.session, fixture.actor.runtime, command)
	if extractRejectReason(first) != "" || findChatMessage(first, chatChannelRegion) == nil {
		t.Fatalf("region chat result=%+v fanout=%v", first, shouldFanOut)
	}
	if localMessage := findChatMessage(local.messages, chatChannelRegion); localMessage == nil || localMessage["text"] != "same region <b>only</b>" {
		t.Fatalf("local region delivery=%+v", local.messages)
	}
	if otherMessage := findChatMessage(otherRegion.messages, chatChannelRegion); otherMessage != nil {
		t.Fatalf("other region received chat=%+v", otherRegion.messages)
	}
	if countMessageKind(fixture.targetMessageSnapshot(), chatMessageKind) != 0 {
		t.Fatalf("remote recipient received before outbox dispatch=%+v", fixture.targetMessageSnapshot())
	}
	if countMessageKind(remoteOtherMessages, chatMessageKind) != 0 {
		t.Fatalf("remote other-region recipient received before dispatch=%+v", remoteOtherMessages)
	}

	eventKey := "gameplay-command/" + fixture.actor.session.ID + "/1/social/chat-region/" + fixture.targetCharacter.ID
	event, err := fixture.storeA.GameplayEvents.GetByIdempotencyKey(context.Background(), eventKey)
	if err != nil || event.TargetRegionID != "dawn_plaza" || event.TargetServerInstanceID != "instance-b" {
		t.Fatalf("region chat event=%+v err=%v", event, err)
	}
	replayed, replayFanOut := fixture.serverA.processGameplayCommandWithDedup(context.Background(), fixture.actor.session, fixture.actor.runtime, command)
	if extractRejectReason(replayed) != "" || replayFanOut || fixture.backend.nextGameplayEventID != 1 || countMessageKind(local.messages, chatMessageKind) != 1 {
		t.Fatalf("region replay=%+v fanout=%v event_count=%d local=%+v", replayed, replayFanOut, fixture.backend.nextGameplayEventID, local.messages)
	}
	conflict := command
	conflict.CommandID = "remote_region_chat_conflict"
	conflict.Payload = mustMarshalCommandPayload(t, map[string]any{"channel": chatChannelRegion, "text": "different"})
	conflicting, _ := fixture.serverA.processGameplayCommandWithDedup(context.Background(), fixture.actor.session, fixture.actor.runtime, conflict)
	if reason := extractRejectReason(conflicting); reason != "sequence.conflicting_replay" {
		t.Fatalf("region conflicting replay reason=%q outbound=%+v", reason, conflicting)
	}
	if records, listErr := fixture.storeA.ChatMessages.ListByCharacterID(context.Background(), fixture.actorCharacter.ID); listErr != nil || len(records) != 1 {
		t.Fatalf("region chat history=%+v err=%v", records, listErr)
	}

	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/region-worker"); claimed != 1 {
		t.Fatalf("region chat claimed=%d", claimed)
	}
	remoteMessages := fixture.targetMessageSnapshot()
	remoteMessage := findChatMessage(remoteMessages, chatChannelRegion)
	if remoteMessage == nil || remoteMessage["region_id"] != "dawn_plaza" || remoteMessage["event_id"] == nil {
		t.Fatalf("remote region delivery=%+v", remoteMessages)
	}
	if countMessageKind(remoteOtherMessages, chatMessageKind) != 0 {
		t.Fatalf("remote other-region recipient received chat=%+v", remoteOtherMessages)
	}
	if receipt, receiptErr := fixture.storeB.GameplayReceipts.GetByEventID(context.Background(), event.ID); receiptErr != nil || receipt.ConsumedAt.IsZero() {
		t.Fatalf("region receipt=%+v error=%v", receipt, receiptErr)
	}

	restartedConsumer := NewServerWithConfig(":0", "", fixture.storeB, ServerConfig{ServerInstanceID: "instance-b"})
	if err := restartedConsumer.deliverGameplayEvent(context.Background(), event, "instance-b/restarted-region-worker"); err != nil {
		t.Fatalf("logical restart region redelivery error=%v", err)
	}
	if countMessageKind(fixture.targetMessageSnapshot(), chatMessageKind) != 1 {
		t.Fatalf("logical restart duplicated region chat=%+v", fixture.targetMessageSnapshot())
	}
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
	if err := fixture.serverB.deliverGameplayEvent(context.Background(), event, "instance-b/duplicate-worker"); err != nil {
		t.Fatalf("duplicate remote whisper delivery error=%v", err)
	}
	if countMessageKind(fixture.targetMessageSnapshot(), chatMessageKind) != 1 {
		t.Fatalf("duplicate remote whisper was rendered twice: %+v", fixture.targetMessageSnapshot())
	}
	receipt, err := fixture.storeB.GameplayReceipts.GetByEventID(context.Background(), event.ID)
	if err != nil || receipt.ConsumedAt.IsZero() || receipt.DeliveredAt.IsZero() || receipt.ServerInstanceID != "instance-b" {
		t.Fatalf("remote whisper receipt=%+v error=%v", receipt, err)
	}
	restartedConsumer := NewServerWithConfig(":0", "", fixture.storeB, ServerConfig{ServerInstanceID: "instance-b"})
	if err := restartedConsumer.deliverGameplayEvent(context.Background(), event, "instance-b/restarted-worker"); err != nil {
		t.Fatalf("logical restart redelivery error=%v", err)
	}
	if countMessageKind(fixture.targetMessageSnapshot(), chatMessageKind) != 1 {
		t.Fatalf("logical restart duplicated remote whisper: %+v", fixture.targetMessageSnapshot())
	}
}

func TestRemotePartyInviteAndAcceptDeliverAuthoritativeState(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "social_party")
	aimPartyInviteTarget(fixture.actor, fixture.target)
	inviteOutbound := dispatchPartyCommand(t, fixture.serverA, fixture.actor, "remote_party_invite", 1, "invite_party_member", map[string]any{})
	if extractRejectReason(inviteOutbound) != "" {
		t.Fatalf("remote party invite=%+v", inviteOutbound)
	}
	replayedInvite := dispatchPartyCommand(t, fixture.serverA, fixture.actor, "remote_party_invite", 1, "invite_party_member", map[string]any{})
	if marshalOutcomeJSON(t, cloneOutboundMessages(inviteOutbound)) != marshalOutcomeJSON(t, replayedInvite) || fixture.backend.nextGameplayEventID != 1 {
		t.Fatalf("remote party invite replay=%+v event_count=%d", replayedInvite, fixture.backend.nextGameplayEventID)
	}
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/party-invite-worker"); claimed != 1 {
		t.Fatalf("remote party invite claimed=%d", claimed)
	}
	targetMessages := fixture.targetMessageSnapshot()
	inviteNotice := findPartyNotice(targetMessages, partyNoticeStatusInviteReceived)
	if inviteNotice == nil || inviteNotice["event_id"] == nil || findOutboundMessage(targetMessages, "delta") == nil {
		t.Fatalf("remote party invite delivery=%+v", targetMessages)
	}
	if receipt, receiptErr := fixture.storeB.GameplayReceipts.GetByEventID(context.Background(), 1); receiptErr != nil || receipt.ConsumedAt.IsZero() {
		t.Fatalf("remote party invite receipt=%+v error=%v", receipt, receiptErr)
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
	replayedInvite := dispatchPartyCommand(t, fixture.serverA, fixture.actor, "remote_clan_invite", 2, "invite_clan_member", map[string]any{})
	if marshalOutcomeJSON(t, cloneOutboundMessages(inviteOutbound)) != marshalOutcomeJSON(t, replayedInvite) || fixture.backend.nextGameplayEventID != 1 {
		t.Fatalf("remote clan invite replay=%+v event_count=%d", replayedInvite, fixture.backend.nextGameplayEventID)
	}
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/clan-invite-worker"); claimed != 1 {
		t.Fatalf("remote clan invite claimed=%d", claimed)
	}
	targetMessages := fixture.targetMessageSnapshot()
	inviteNotice := findClanNotice(targetMessages, clanNoticeStatusInviteReceived)
	if inviteNotice == nil || inviteNotice["event_id"] == nil || findOutboundMessage(targetMessages, "delta") == nil {
		t.Fatalf("remote clan invite delivery=%+v", targetMessages)
	}
	if receipt, receiptErr := fixture.storeB.GameplayReceipts.GetByEventID(context.Background(), 1); receiptErr != nil || receipt.ConsumedAt.IsZero() {
		t.Fatalf("remote clan invite receipt=%+v error=%v", receipt, receiptErr)
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

func TestRemoteRegionChatDeadLettersStableStaleOwner(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "social_stale")
	notice := chatMessagePayload("", 0, chatChannelRegion, fixture.actorCharacter.ID, fixture.actorCharacter.Name, "", "", "dawn_plaza", "ownership drift", time.Now().UTC())
	payload, err := json.Marshal(remoteSocialDeliveryPayload{
		RecipientCharacterID:  fixture.targetCharacter.ID,
		RecipientFencingToken: fixture.target.session.FencingToken,
		Message:               notice,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error=%v", err)
	}
	event := &GameplayEvent{
		IdempotencyKey:         "social-stale-owner-event",
		Type:                   remoteChatMessageEventType,
		Payload:                payload,
		TargetServerInstanceID: "instance-b",
		TargetRegionID:         "dawn_plaza",
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
	if receipt, receiptErr := fixture.storeB.GameplayReceipts.GetByEventID(context.Background(), event.ID); !errors.Is(receiptErr, errRecordNotFound) || receipt != nil {
		t.Fatalf("dead-letter retained an unconsumed receipt: receipt=%+v error=%v", receipt, receiptErr)
	}
}

func TestRemoteRegionChatRejectsReusedSessionWithNewFence(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "social_same_session_new_fence")
	oldFence := fixture.target.session.FencingToken
	notice := chatMessagePayload("", 0, chatChannelRegion, fixture.actorCharacter.ID, fixture.actorCharacter.Name, "", "", "dawn_plaza", "must not cross takeover", time.Now().UTC())
	payload, err := json.Marshal(remoteSocialDeliveryPayload{
		RecipientCharacterID:  fixture.targetCharacter.ID,
		RecipientFencingToken: oldFence,
		Message:               notice,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error=%v", err)
	}
	event := &GameplayEvent{
		IdempotencyKey:         "social-same-session-new-fence",
		Type:                   remoteChatMessageEventType,
		Payload:                payload,
		TargetServerInstanceID: "instance-b",
		TargetRegionID:         "dawn_plaza",
		TargetSessionID:        fixture.target.session.ID,
		TargetCharacterID:      fixture.targetCharacter.ID,
	}
	if created, createErr := fixture.storeA.GameplayEvents.Create(context.Background(), event); createErr != nil || !created {
		t.Fatalf("GameplayEvents.Create() created=%v err=%v", created, createErr)
	}

	fixture.backend.mu.Lock()
	fixture.backend.sessionOwnerships[fixture.targetCharacter.ID].FencingToken = oldFence + 1
	fixture.backend.mu.Unlock()
	attached := fixture.serverB.attachedSessionBySessionID(fixture.target.session.ID)
	attached.fencingToken = oldFence + 1
	fixture.serverB.config.GameplayEventMaxRetries = 1

	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/same-session-new-fence"); claimed != 1 {
		t.Fatalf("stale fence event claimed=%d", claimed)
	}
	persisted, err := fixture.storeA.GameplayEvents.GetByIdempotencyKey(context.Background(), event.IdempotencyKey)
	if err != nil || persisted.DeadLetteredAt.IsZero() || persisted.LastError != "social.recipient_stale_owner" {
		t.Fatalf("stale fence event=%+v err=%v", persisted, err)
	}
	if countMessageKind(fixture.targetMessageSnapshot(), chatMessageKind) != 0 {
		t.Fatalf("new fence received stale remote chat: %+v", fixture.targetMessageSnapshot())
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
