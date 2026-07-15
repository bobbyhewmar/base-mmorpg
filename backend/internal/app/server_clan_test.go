package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func findClanNotice(messages []map[string]any, status string) map[string]any {
	for _, message := range messages {
		if message["kind"] != clanNoticeKind {
			continue
		}
		if messageStatus, _ := message["status"].(string); messageStatus == status {
			return message
		}
	}
	return nil
}

func requireClanDeltaWithMemberCount(t *testing.T, messages []map[string]any, memberCount int) {
	t.Helper()
	delta := findOutboundMessage(messages, "delta")
	if delta == nil {
		t.Fatalf("expected delta, got %+v", messages)
	}
	self, ok := delta["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", delta)
	}
	switch clan := self["clan"].(type) {
	case *CharacterClanSnapshot:
		if clan == nil || len(clan.Members) != memberCount {
			t.Fatalf("expected %d clan members, got %+v", memberCount, clan)
		}
	case CharacterClanSnapshot:
		if len(clan.Members) != memberCount {
			t.Fatalf("expected %d clan members, got %+v", memberCount, clan.Members)
		}
	case map[string]any:
		members, ok := clan["members"].([]any)
		if !ok || len(members) != memberCount {
			t.Fatalf("expected %d clan members, got %+v", memberCount, clan["members"])
		}
	default:
		t.Fatalf("expected clan payload, got %+v", self["clan"])
	}
}

func requireClanDeltaCleared(t *testing.T, messages []map[string]any) {
	t.Helper()
	delta := findOutboundMessage(messages, "delta")
	if delta == nil {
		t.Fatalf("expected delta, got %+v", messages)
	}
	self, ok := delta["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", delta)
	}
	switch clan := self["clan"].(type) {
	case nil:
		return
	case *CharacterClanSnapshot:
		if clan == nil {
			return
		}
	}
	t.Fatalf("expected clan to be cleared, got %+v", self["clan"])
}

func TestServerClanCreatePersistsLeaderAndRejectsDuplicateOrSecondClan(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	founder := stagePartyTestClient(t, server, store, "sess_clan_founder", &Character{
		ID:           "char_clan_founder",
		AccountID:    "acc_clan_founder",
		Name:         "Founder",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})
	rival := stagePartyTestClient(t, server, store, "sess_clan_rival", &Character{
		ID:           "char_clan_rival",
		AccountID:    "acc_clan_rival",
		Name:         "Rival",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -9,
		PositionZ:    0,
	})

	createOutbound := dispatchPartyCommand(t, server, founder, "cmd_clan_create_1", 1, "create_clan", map[string]any{
		"name": "Nightfall",
	})
	if findOutboundMessage(createOutbound, "ack") == nil {
		t.Fatalf("expected create ack, got %+v", createOutbound)
	}
	requireClanDeltaWithMemberCount(t, createOutbound, 1)
	if findClanNotice(createOutbound, clanNoticeStatusCreated) == nil {
		t.Fatalf("expected created notice, got %+v", createOutbound)
	}

	clan, err := store.Clans.GetByCharacterID(context.Background(), founder.session.CharacterID)
	if err != nil {
		t.Fatalf("Clans.GetByCharacterID(founder) error = %v", err)
	}
	if clan.LeaderCharacterID != founder.session.CharacterID || clan.Name != "Nightfall" {
		t.Fatalf("unexpected clan after create = %+v", clan)
	}
	members, err := store.Clans.ListMembers(context.Background(), clan.ID)
	if err != nil {
		t.Fatalf("Clans.ListMembers() error = %v", err)
	}
	if len(members) != 1 || members[0].CharacterID != founder.session.CharacterID {
		t.Fatalf("expected founder as only clan member, got %+v", members)
	}

	duplicateOutbound := dispatchPartyCommand(t, server, rival, "cmd_clan_create_duplicate", 1, "create_clan", map[string]any{
		"name": "Nightfall",
	})
	requireRejectReason(t, duplicateOutbound, "clan.name_taken")

	secondCreateOutbound := dispatchPartyCommand(t, server, founder, "cmd_clan_create_second", 2, "create_clan", map[string]any{
		"name": "Daybreak",
	})
	requireRejectReason(t, secondCreateOutbound, "clan.already_in_clan")
}

func TestServerClanInviteAuthorityLeaveKickAndDissolve(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	leader := stagePartyTestClient(t, server, store, "sess_clan_leader", &Character{
		ID:           "char_clan_leader",
		AccountID:    "acc_clan_leader",
		Name:         "Leader",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})
	member := stagePartyTestClient(t, server, store, "sess_clan_member", &Character{
		ID:           "char_clan_member",
		AccountID:    "acc_clan_member",
		Name:         "Member",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})
	outsider := stagePartyTestClient(t, server, store, "sess_clan_outsider", &Character{
		ID:           "char_clan_outsider",
		AccountID:    "acc_clan_outsider",
		Name:         "Outsider",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})

	_ = dispatchPartyCommand(t, server, leader, "cmd_clan_authority_create", 1, "create_clan", map[string]any{
		"name": "Nightfall",
	})

	aimPartyInviteTarget(leader, leader)
	selfInviteOutbound := dispatchPartyCommand(t, server, leader, "cmd_clan_self_invite", 2, "invite_clan_member", map[string]any{})
	requireRejectReason(t, selfInviteOutbound, "clan.target_invalid")

	aimPartyInviteTarget(leader, member)
	inviteOutbound := dispatchPartyCommand(t, server, leader, "cmd_clan_invite_member", 3, "invite_clan_member", map[string]any{})
	if findClanNotice(inviteOutbound, clanNoticeStatusInviteSent) == nil {
		t.Fatalf("expected invite sent notice, got %+v", inviteOutbound)
	}
	receivedInvite := findClanNotice(member.messages, clanNoticeStatusInviteReceived)
	inviteID, _ := receivedInvite["invite_id"].(string)
	if inviteID == "" {
		t.Fatalf("expected invite id, got %+v", receivedInvite)
	}
	member.resetMessages()
	leader.resetMessages()

	acceptOutbound := dispatchPartyCommand(t, server, member, "cmd_clan_accept_member", 1, "accept_clan_invite", map[string]any{
		"invite_id": inviteID,
	})
	if findOutboundMessage(acceptOutbound, "ack") == nil {
		t.Fatalf("expected accept ack, got %+v", acceptOutbound)
	}
	requireClanDeltaWithMemberCount(t, acceptOutbound, 2)
	if findClanNotice(acceptOutbound, clanNoticeStatusInviteAccepted) == nil {
		t.Fatalf("expected invite accepted notice, got %+v", acceptOutbound)
	}

	aimPartyInviteTarget(leader, member)
	targetAlreadyInClanOutbound := dispatchPartyCommand(t, server, leader, "cmd_clan_invite_existing_member", 4, "invite_clan_member", map[string]any{})
	requireRejectReason(t, targetAlreadyInClanOutbound, "clan.target_already_in_clan")

	aimPartyInviteTarget(member, outsider)
	nonLeaderInviteOutbound := dispatchPartyCommand(t, server, member, "cmd_clan_non_leader_invite", 2, "invite_clan_member", map[string]any{})
	requireRejectReason(t, nonLeaderInviteOutbound, "clan.leader_required")

	nonLeaderKickOutbound := dispatchPartyCommand(t, server, member, "cmd_clan_non_leader_kick", 3, "kick_clan_member", map[string]any{
		"target_character_id": leader.session.CharacterID,
	})
	requireRejectReason(t, nonLeaderKickOutbound, "clan.leader_required")

	leaderLeaveOutbound := dispatchPartyCommand(t, server, leader, "cmd_clan_leader_leave", 5, "leave_clan", map[string]any{})
	requireRejectReason(t, leaderLeaveOutbound, "clan.leader_cannot_leave")

	memberLeaveOutbound := dispatchPartyCommand(t, server, member, "cmd_clan_member_leave", 4, "leave_clan", map[string]any{})
	if findOutboundMessage(memberLeaveOutbound, "ack") == nil {
		t.Fatalf("expected member leave ack, got %+v", memberLeaveOutbound)
	}
	requireClanDeltaCleared(t, memberLeaveOutbound)
	if _, err := store.Clans.GetByCharacterID(context.Background(), member.session.CharacterID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected member to be removed from clan, got err = %v", err)
	}
	if clan, err := store.Clans.GetByCharacterID(context.Background(), leader.session.CharacterID); err != nil {
		t.Fatalf("expected leader clan to persist with one member, got err = %v", err)
	} else if clan.LeaderCharacterID != leader.session.CharacterID {
		t.Fatalf("unexpected clan after member leave = %+v", clan)
	}

	aimPartyInviteTarget(leader, outsider)
	_ = dispatchPartyCommand(t, server, leader, "cmd_clan_invite_outsider", 6, "invite_clan_member", map[string]any{})
	outsiderInvite := findClanNotice(outsider.messages, clanNoticeStatusInviteReceived)
	outsiderInviteID, _ := outsiderInvite["invite_id"].(string)
	if outsiderInviteID == "" {
		t.Fatalf("expected outsider invite id, got %+v", outsiderInvite)
	}

	nonLeaderDissolveOutbound := dispatchPartyCommand(t, server, outsider, "cmd_clan_non_leader_dissolve", 1, "dissolve_clan", map[string]any{})
	requireRejectReason(t, nonLeaderDissolveOutbound, "clan.not_in_clan")

	dissolveOutbound := dispatchPartyCommand(t, server, leader, "cmd_clan_dissolve", 7, "dissolve_clan", map[string]any{})
	if findOutboundMessage(dissolveOutbound, "ack") == nil {
		t.Fatalf("expected dissolve ack, got %+v", dissolveOutbound)
	}
	requireClanDeltaCleared(t, dissolveOutbound)
	if findClanNotice(dissolveOutbound, clanNoticeStatusClanDissolved) == nil {
		t.Fatalf("expected dissolve notice, got %+v", dissolveOutbound)
	}
	if _, err := store.Clans.GetByCharacterID(context.Background(), leader.session.CharacterID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected clan to be removed after dissolve, got err = %v", err)
	}
	if _, err := store.Clans.GetInviteByID(context.Background(), outsiderInviteID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected pending invite to be cleared on dissolve, got err = %v", err)
	}
}

func TestServerClanInviteRejectsClientAuthoredTargetPayload(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	leader := stagePartyTestClient(t, server, store, "sess_clan_payload_leader", &Character{
		ID:           "char_clan_payload_leader",
		AccountID:    "acc_clan_payload_leader",
		Name:         "PayloadLeader",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})
	recruit := stagePartyTestClient(t, server, store, "sess_clan_payload_recruit", &Character{
		ID:           "char_clan_payload_recruit",
		AccountID:    "acc_clan_payload_recruit",
		Name:         "PayloadRecruit",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})

	_ = dispatchPartyCommand(t, server, leader, "cmd_clan_payload_create", 1, "create_clan", map[string]any{
		"name": "PayloadClan",
	})

	inviteOutbound := dispatchPartyCommand(t, server, leader, "cmd_clan_payload_invite", 2, "invite_clan_member", map[string]any{
		"target_character_id": recruit.session.CharacterID,
	})
	requireRejectReason(t, inviteOutbound, "protocol.invalid_envelope")
	if findOutboundMessage(inviteOutbound, "ack") != nil {
		t.Fatalf("expected invalid target payload to fail before ack, got %+v", inviteOutbound)
	}
	if notice := findClanNotice(recruit.messages, clanNoticeStatusInviteReceived); notice != nil {
		t.Fatalf("expected no invite delivery from client-authored target, got %+v", recruit.messages)
	}
	if invites, err := store.Clans.ListPendingInvitesByInvitee(context.Background(), recruit.session.CharacterID, time.Now().UTC()); !errors.Is(err, errRecordNotFound) || len(invites) != 0 {
		t.Fatalf("expected no persisted invite from client-authored target, invites=%+v err=%v", invites, err)
	}
}

func TestServerClanInviteExpiryAndDisconnectCancel(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	leader := stagePartyTestClient(t, server, store, "sess_clan_expiry_leader", &Character{
		ID:           "char_clan_expiry_leader",
		AccountID:    "acc_clan_expiry_leader",
		Name:         "Leader",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})
	recruit := stagePartyTestClient(t, server, store, "sess_clan_expiry_recruit", &Character{
		ID:           "char_clan_expiry_recruit",
		AccountID:    "acc_clan_expiry_recruit",
		Name:         "Recruit",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})
	secondRecruit := stagePartyTestClient(t, server, store, "sess_clan_disconnect_recruit", &Character{
		ID:           "char_clan_disconnect_recruit",
		AccountID:    "acc_clan_disconnect_recruit",
		Name:         "SecondRecruit",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})
	inviterDisconnectRecruit := stagePartyTestClient(t, server, store, "sess_clan_inviter_disconnect_recruit", &Character{
		ID:           "char_clan_inviter_disconnect_recruit",
		AccountID:    "acc_clan_inviter_disconnect_recruit",
		Name:         "ThirdRecruit",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})

	_ = dispatchPartyCommand(t, server, leader, "cmd_clan_expiry_create", 1, "create_clan", map[string]any{
		"name": "Nightfall",
	})

	aimPartyInviteTarget(leader, recruit)
	inviteOutbound := dispatchPartyCommand(t, server, leader, "cmd_clan_expiry_invite", 2, "invite_clan_member", map[string]any{})
	if findOutboundMessage(inviteOutbound, "ack") == nil {
		t.Fatalf("expected invite ack, got %+v", inviteOutbound)
	}
	receivedInvite := findClanNotice(recruit.messages, clanNoticeStatusInviteReceived)
	inviteID, _ := receivedInvite["invite_id"].(string)
	if inviteID == "" {
		t.Fatalf("expected invite id, got %+v", receivedInvite)
	}
	if invite, err := store.Clans.GetInviteByID(context.Background(), inviteID); err != nil {
		t.Fatalf("Clans.GetInviteByID() error = %v", err)
	} else if got := invite.ExpiresAt.Sub(invite.CreatedAt); got != clanInviteTTL {
		t.Fatalf("expected invite ttl %s, got %s", clanInviteTTL, got)
	}

	repo := store.Clans.(memoryClanRepo)
	repo.backend.mu.Lock()
	repo.backend.clanInvites[inviteID].ExpiresAt = time.Now().UTC().Add(-time.Second)
	repo.backend.mu.Unlock()

	lateAcceptOutbound := dispatchPartyCommand(t, server, recruit, "cmd_clan_expiry_accept", 1, "accept_clan_invite", map[string]any{
		"invite_id": inviteID,
	})
	requireRejectReason(t, lateAcceptOutbound, "clan.invite_expired")
	if _, err := store.Clans.GetInviteByID(context.Background(), inviteID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected expired invite to be removed, got err = %v", err)
	}

	leader.resetMessages()
	secondRecruit.resetMessages()
	aimPartyInviteTarget(leader, secondRecruit)
	_ = dispatchPartyCommand(t, server, leader, "cmd_clan_disconnect_invitee_invite", 3, "invite_clan_member", map[string]any{})
	disconnectInviteeNotice := findClanNotice(secondRecruit.messages, clanNoticeStatusInviteReceived)
	disconnectInviteeID, _ := disconnectInviteeNotice["invite_id"].(string)
	if disconnectInviteeID == "" {
		t.Fatalf("expected disconnect invitee id, got %+v", disconnectInviteeNotice)
	}
	server.closeAttachedSession(secondRecruit.session.ID)
	if _, err := store.Clans.GetInviteByID(context.Background(), disconnectInviteeID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected invite to be removed after invitee disconnect, got err = %v", err)
	}
	if findClanNotice(leader.messages, clanNoticeStatusInviteExpired) == nil {
		t.Fatalf("expected leader to receive invite_expired after invitee disconnect, got %+v", leader.messages)
	}

	leader.resetMessages()
	inviterDisconnectRecruit.resetMessages()
	aimPartyInviteTarget(leader, inviterDisconnectRecruit)
	_ = dispatchPartyCommand(t, server, leader, "cmd_clan_disconnect_inviter_invite", 4, "invite_clan_member", map[string]any{})
	disconnectInviterNotice := findClanNotice(inviterDisconnectRecruit.messages, clanNoticeStatusInviteReceived)
	disconnectInviterID, _ := disconnectInviterNotice["invite_id"].(string)
	if disconnectInviterID == "" {
		t.Fatalf("expected disconnect inviter invite id, got %+v", disconnectInviterNotice)
	}
	server.closeAttachedSession(leader.session.ID)
	if _, err := store.Clans.GetInviteByID(context.Background(), disconnectInviterID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected invite to be removed after inviter disconnect, got err = %v", err)
	}
	if findClanNotice(inviterDisconnectRecruit.messages, clanNoticeStatusInviteExpired) == nil {
		t.Fatalf("expected invitee to receive invite_expired after inviter disconnect, got %+v", inviterDisconnectRecruit.messages)
	}
}

func TestServerClanLifecycleIsReplaySafeAndCorrelatesAppliedDeltas(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)
	stage := func(sessionID string, characterID string, name string) *partyTestClient {
		return stagePartyTestClient(t, server, store, sessionID, &Character{
			ID:           characterID,
			AccountID:    "acc_" + characterID,
			Name:         name,
			BaseClass:    "Fighter",
			Sex:          "Male",
			Level:        1,
			LastRegionID: "dawn_plaza",
			PositionX:    -8,
			PositionZ:    0,
		})
	}
	leader := stage("sess_clan_replay_leader", "char_clan_replay_leader", "ReplayLeader")
	recruit := stage("sess_clan_replay_recruit", "char_clan_replay_recruit", "ReplayRecruit")

	dispatchReplay := func(client *partyTestClient, commandID string, commandSeq int, commandType string, payload any) []map[string]any {
		t.Helper()
		first := dispatchPartyCommand(t, server, client, commandID, commandSeq, commandType, payload)
		replay := dispatchPartyCommand(t, server, client, commandID, commandSeq, commandType, payload)
		if marshalOutcomeJSON(t, cloneOutboundMessages(first)) != marshalOutcomeJSON(t, replay) {
			t.Fatalf("expected deterministic replay for %s, first=%+v replay=%+v", commandType, first, replay)
		}
		if delta := findOutboundMessage(first, "delta"); delta != nil {
			if delta["applies_to_command_id"] != commandID || delta["applies_to_command_seq"] != commandSeq {
				t.Fatalf("expected correlated clan delta for %s, got %+v", commandType, delta)
			}
		}
		return first
	}

	createOutbound := dispatchReplay(leader, "cmd_clan_replay_create", 1, "create_clan", map[string]any{"name": "ReplayClan"})
	requireClanDeltaWithMemberCount(t, createOutbound, 1)

	aimPartyInviteTarget(leader, recruit)
	recruit.resetMessages()
	inviteOutbound := dispatchReplay(leader, "cmd_clan_replay_invite_decline", 2, "invite_clan_member", map[string]any{})
	if findClanNotice(inviteOutbound, clanNoticeStatusInviteSent) == nil {
		t.Fatalf("expected replay-safe invite success, got %+v", inviteOutbound)
	}
	declineInviteNotice := findClanNotice(recruit.messages, clanNoticeStatusInviteReceived)
	declineInviteID, _ := declineInviteNotice["invite_id"].(string)
	if declineInviteID == "" {
		t.Fatalf("expected decline invite id, got %+v", declineInviteNotice)
	}
	declineOutbound := dispatchReplay(recruit, "cmd_clan_replay_decline", 1, "decline_clan_invite", map[string]any{"invite_id": declineInviteID})
	if findClanNotice(declineOutbound, clanNoticeStatusInviteDeclined) == nil {
		t.Fatalf("expected replay-safe decline success, got %+v", declineOutbound)
	}
	if _, err := store.Clans.GetInviteByID(context.Background(), declineInviteID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected declined invite to stay consumed after replay, err=%v", err)
	}

	aimPartyInviteTarget(leader, recruit)
	recruit.resetMessages()
	_ = dispatchReplay(leader, "cmd_clan_replay_invite_accept", 3, "invite_clan_member", map[string]any{})
	acceptInviteNotice := findClanNotice(recruit.messages, clanNoticeStatusInviteReceived)
	acceptInviteID, _ := acceptInviteNotice["invite_id"].(string)
	acceptOutbound := dispatchReplay(recruit, "cmd_clan_replay_accept", 2, "accept_clan_invite", map[string]any{"invite_id": acceptInviteID})
	requireClanDeltaWithMemberCount(t, acceptOutbound, 2)
	clan, err := store.Clans.GetByCharacterID(context.Background(), leader.session.CharacterID)
	if err != nil {
		t.Fatalf("Clans.GetByCharacterID(leader) error = %v", err)
	}
	members, err := store.Clans.ListMembers(context.Background(), clan.ID)
	if err != nil || len(members) != 2 {
		t.Fatalf("expected accept replay to keep exactly two members, members=%+v err=%v", members, err)
	}

	leaveOutbound := dispatchReplay(recruit, "cmd_clan_replay_leave", 3, "leave_clan", map[string]any{})
	requireClanDeltaCleared(t, leaveOutbound)
	if _, err := store.Clans.GetByCharacterID(context.Background(), recruit.session.CharacterID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected leave replay to keep recruit outside clan, err=%v", err)
	}

	aimPartyInviteTarget(leader, recruit)
	recruit.resetMessages()
	_ = dispatchReplay(leader, "cmd_clan_replay_invite_kick", 4, "invite_clan_member", map[string]any{})
	kickInviteNotice := findClanNotice(recruit.messages, clanNoticeStatusInviteReceived)
	kickInviteID, _ := kickInviteNotice["invite_id"].(string)
	_ = dispatchReplay(recruit, "cmd_clan_replay_accept_kick", 4, "accept_clan_invite", map[string]any{"invite_id": kickInviteID})
	kickOutbound := dispatchReplay(leader, "cmd_clan_replay_kick", 5, "kick_clan_member", map[string]any{"target_character_id": recruit.session.CharacterID})
	requireClanDeltaWithMemberCount(t, kickOutbound, 1)
	if _, err := store.Clans.GetByCharacterID(context.Background(), recruit.session.CharacterID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected kick replay to keep recruit outside clan, err=%v", err)
	}

	dissolveOutbound := dispatchReplay(leader, "cmd_clan_replay_dissolve", 6, "dissolve_clan", map[string]any{})
	requireClanDeltaCleared(t, dissolveOutbound)
	if _, err := store.Clans.GetByID(context.Background(), clan.ID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected dissolve replay to keep clan deleted, err=%v", err)
	}

	conflictOutbound := dispatchPartyCommand(t, server, leader, "cmd_clan_replay_conflict", 6, "dissolve_clan", map[string]any{})
	requireRejectReason(t, conflictOutbound, "sequence.conflicting_replay")
	if findOutboundMessage(conflictOutbound, "ack") != nil {
		t.Fatalf("expected conflicting replay to fail before ack, got %+v", conflictOutbound)
	}
}

func TestServerClanMembershipRehydratesAfterDisconnectAndReconnect(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)
	leader := stagePartyTestClient(t, server, store, "sess_clan_reconnect_leader", &Character{
		ID:           "char_clan_reconnect_leader",
		AccountID:    "acc_clan_reconnect_leader",
		Name:         "ReconnectLeader",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})
	recruit := stagePartyTestClient(t, server, store, "sess_clan_reconnect_recruit", &Character{
		ID:           "char_clan_reconnect_recruit",
		AccountID:    "acc_clan_reconnect_recruit",
		Name:         "ReconnectRecruit",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})

	_ = dispatchPartyCommand(t, server, leader, "cmd_clan_reconnect_create", 1, "create_clan", map[string]any{"name": "ReconnectClan"})
	aimPartyInviteTarget(leader, recruit)
	recruit.resetMessages()
	_ = dispatchPartyCommand(t, server, leader, "cmd_clan_reconnect_invite", 2, "invite_clan_member", map[string]any{})
	inviteNotice := findClanNotice(recruit.messages, clanNoticeStatusInviteReceived)
	inviteID, _ := inviteNotice["invite_id"].(string)
	acceptOutbound := dispatchPartyCommand(t, server, recruit, "cmd_clan_reconnect_accept", 1, "accept_clan_invite", map[string]any{"invite_id": inviteID})
	requireClanDeltaWithMemberCount(t, acceptOutbound, 2)

	server.closeAttachedSession(recruit.session.ID)
	server.unregisterAttachedSession(recruit.session.ID)
	character, err := store.Characters.GetByID(context.Background(), recruit.session.CharacterID)
	if err != nil {
		t.Fatalf("Characters.GetByID(recruit) error = %v", err)
	}
	rehydrated, err := server.buildAttachedRuntime(context.Background(), recruit.session, character, time.Now().UTC())
	if err != nil {
		t.Fatalf("buildAttachedRuntime(reconnect) error = %v", err)
	}
	rehydrated.mu.Lock()
	rehydratedClan := cloneCharacterClanSnapshot(rehydrated.clan)
	rehydratedInvites := cloneCharacterClanInviteSnapshots(rehydrated.clanInvites)
	rehydrated.mu.Unlock()
	if rehydratedClan == nil || len(rehydratedClan.Members) != 2 || rehydratedClan.LeaderCharacterID != leader.session.CharacterID {
		t.Fatalf("expected authoritative clan membership after reconnect, got %+v", rehydratedClan)
	}
	if len(rehydratedInvites) != 0 {
		t.Fatalf("expected no stale invites after accepted membership reconnect, got %+v", rehydratedInvites)
	}
}
