package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type partyTestClient struct {
	session  *Session
	runtime  *attachedRuntime
	messages []map[string]any
}

func stagePartyTestClient(t *testing.T, server *Server, store *Store, sessionID string, character *Character) *partyTestClient {
	t.Helper()

	if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
		t.Fatalf("CreateCharacterWithItemSeed(%s) error = %v", character.ID, err)
	}

	client := &partyTestClient{
		session: &Session{
			ID:          sessionID,
			AccountID:   character.AccountID,
			CharacterID: character.ID,
		},
		runtime: newAttachedRuntime(sessionID, character),
	}
	client.runtime.deferRewardResolution = true
	server.stageAttachedSession(sessionID, client.runtime, func(message map[string]any) bool {
		client.messages = append(client.messages, message)
		return true
	})
	client.messages = append(client.messages, server.activateAttachedSession(sessionID)...)
	t.Cleanup(func() {
		server.unregisterAttachedSession(sessionID)
	})
	return client
}

func (client *partyTestClient) resetMessages() {
	client.messages = nil
}

func mustMarshalCommandPayload(t *testing.T, payload any) json.RawMessage {
	t.Helper()
	if payload == nil {
		return json.RawMessage(`{}`)
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal(payload) error = %v", err)
	}
	return bytes
}

func dispatchPartyCommand(
	t *testing.T,
	server *Server,
	client *partyTestClient,
	commandID string,
	commandSeq int,
	commandType string,
	payload any,
) []map[string]any {
	t.Helper()

	outbound, _ := server.processGameplayCommandWithDedup(context.Background(), client.session, client.runtime, commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       commandID,
		CommandSeq:      commandSeq,
		Type:            commandType,
		Payload:         mustMarshalCommandPayload(t, payload),
	})
	return outbound
}

func findOutboundMessage(messages []map[string]any, kind string) map[string]any {
	for _, message := range messages {
		if message["kind"] == kind {
			return message
		}
	}
	return nil
}

func findPartyNotice(messages []map[string]any, status string) map[string]any {
	for _, message := range messages {
		if message["kind"] != partyNoticeKind {
			continue
		}
		if messageStatus, _ := message["status"].(string); messageStatus == status {
			return message
		}
	}
	return nil
}

func requireRejectReason(t *testing.T, messages []map[string]any, reasonCode string) {
	t.Helper()
	reject := findOutboundMessage(messages, "reject")
	if reject == nil {
		t.Fatalf("expected reject %s, got %+v", reasonCode, messages)
	}
	if reject["reason_code"] != reasonCode {
		t.Fatalf("expected reject %s, got %+v", reasonCode, reject)
	}
}

func requirePartyDeltaWithMemberCount(t *testing.T, messages []map[string]any, memberCount int) {
	t.Helper()
	delta := findOutboundMessage(messages, "delta")
	if delta == nil {
		t.Fatalf("expected delta, got %+v", messages)
	}
	self, ok := delta["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", delta)
	}
	switch party := self["party"].(type) {
	case *CharacterPartySnapshot:
		if len(party.Members) != memberCount {
			t.Fatalf("expected %d party members, got %+v", memberCount, party.Members)
		}
	case CharacterPartySnapshot:
		if len(party.Members) != memberCount {
			t.Fatalf("expected %d party members, got %+v", memberCount, party.Members)
		}
	case map[string]any:
		members, ok := party["members"].([]any)
		if !ok || len(members) != memberCount {
			t.Fatalf("expected %d party members, got %+v", memberCount, party["members"])
		}
	default:
		t.Fatalf("expected party payload, got %+v", self["party"])
	}
}

func TestServerPartyInviteAcceptPersistsRosterAndDedupsMembership(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	leader := stagePartyTestClient(t, server, store, "sess_party_leader", &Character{
		ID:           "char_party_leader",
		AccountID:    "acc_party_leader",
		Name:         "Leader",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})
	recruit := stagePartyTestClient(t, server, store, "sess_party_recruit", &Character{
		ID:           "char_party_recruit",
		AccountID:    "acc_party_recruit",
		Name:         "Recruit",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})
	leader.resetMessages()
	recruit.resetMessages()

	inviteOutbound := dispatchPartyCommand(t, server, leader, "cmd_party_invite_1", 1, "invite_party_member", map[string]any{
		"target_character_id": recruit.session.CharacterID,
	})
	if findOutboundMessage(inviteOutbound, "ack") == nil {
		t.Fatalf("expected ack on invite, got %+v", inviteOutbound)
	}
	requirePartyDeltaWithMemberCount(t, inviteOutbound, 1)
	if findPartyNotice(inviteOutbound, partyNoticeStatusInviteSent) == nil {
		t.Fatalf("expected invite_sent notice, got %+v", inviteOutbound)
	}
	if findPartyNotice(recruit.messages, partyNoticeStatusInviteReceived) == nil {
		t.Fatalf("expected recruit invite_received notice, got %+v", recruit.messages)
	}

	receivedInvite := findPartyNotice(recruit.messages, partyNoticeStatusInviteReceived)
	inviteID, _ := receivedInvite["invite_id"].(string)
	if inviteID == "" {
		t.Fatalf("expected invite_id in recruit notice, got %+v", receivedInvite)
	}

	leader.resetMessages()
	recruit.resetMessages()
	acceptOutbound := dispatchPartyCommand(t, server, recruit, "cmd_party_accept_1", 1, "accept_party_invite", map[string]any{
		"invite_id": inviteID,
	})
	if findOutboundMessage(acceptOutbound, "ack") == nil {
		t.Fatalf("expected ack on accept, got %+v", acceptOutbound)
	}
	requirePartyDeltaWithMemberCount(t, acceptOutbound, 2)
	if findPartyNotice(acceptOutbound, partyNoticeStatusInviteAccepted) == nil {
		t.Fatalf("expected invite_accepted notice, got %+v", acceptOutbound)
	}
	if findPartyNotice(leader.messages, partyNoticeStatusMemberJoined) == nil {
		t.Fatalf("expected leader member_joined notice, got %+v", leader.messages)
	}

	party, err := store.Parties.GetByCharacterID(context.Background(), recruit.session.CharacterID)
	if err != nil {
		t.Fatalf("Parties.GetByCharacterID(recruit) error = %v", err)
	}
	members, err := store.Parties.ListMembers(context.Background(), party.ID)
	if err != nil {
		t.Fatalf("Parties.ListMembers() error = %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected two persisted party members, got %+v", members)
	}

	replayOutbound := dispatchPartyCommand(t, server, recruit, "cmd_party_accept_1", 1, "accept_party_invite", map[string]any{
		"invite_id": inviteID,
	})
	requirePartyDeltaWithMemberCount(t, replayOutbound, 2)
	replayedMembers, err := store.Parties.ListMembers(context.Background(), party.ID)
	if err != nil {
		t.Fatalf("Parties.ListMembers(replay) error = %v", err)
	}
	if len(replayedMembers) != 2 {
		t.Fatalf("expected membership dedup to keep two members, got %+v", replayedMembers)
	}
}

func TestServerPartyDeclineAndKickRespectAuthorityRules(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	leader := stagePartyTestClient(t, server, store, "sess_party_authority_leader", &Character{
		ID:           "char_party_authority_leader",
		AccountID:    "acc_party_authority_leader",
		Name:         "Leader",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})
	member := stagePartyTestClient(t, server, store, "sess_party_authority_member", &Character{
		ID:           "char_party_authority_member",
		AccountID:    "acc_party_authority_member",
		Name:         "Member",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})
	leader.resetMessages()
	member.resetMessages()

	_ = dispatchPartyCommand(t, server, leader, "cmd_party_invite_decline", 1, "invite_party_member", map[string]any{
		"target_character_id": member.session.CharacterID,
	})
	inviteID, _ := findPartyNotice(member.messages, partyNoticeStatusInviteReceived)["invite_id"].(string)
	member.resetMessages()
	leader.resetMessages()

	declineOutbound := dispatchPartyCommand(t, server, member, "cmd_party_decline", 1, "decline_party_invite", map[string]any{
		"invite_id": inviteID,
	})
	if findPartyNotice(declineOutbound, partyNoticeStatusInviteDeclined) == nil {
		t.Fatalf("expected invite_declined notice, got %+v", declineOutbound)
	}
	if findPartyNotice(leader.messages, partyNoticeStatusInviteDeclined) == nil {
		t.Fatalf("expected leader decline notice, got %+v", leader.messages)
	}

	leader.resetMessages()
	member.resetMessages()
	_ = dispatchPartyCommand(t, server, leader, "cmd_party_invite_accept", 2, "invite_party_member", map[string]any{
		"target_character_id": member.session.CharacterID,
	})
	inviteID, _ = findPartyNotice(member.messages, partyNoticeStatusInviteReceived)["invite_id"].(string)
	member.resetMessages()
	leader.resetMessages()
	_ = dispatchPartyCommand(t, server, member, "cmd_party_accept_authority", 2, "accept_party_invite", map[string]any{
		"invite_id": inviteID,
	})

	nonLeaderKickOutbound := dispatchPartyCommand(t, server, member, "cmd_party_kick_forbidden", 3, "kick_party_member", map[string]any{
		"target_character_id": leader.session.CharacterID,
	})
	requireRejectReason(t, nonLeaderKickOutbound, "party.leader_required")

	leader.resetMessages()
	member.resetMessages()
	leaderKickOutbound := dispatchPartyCommand(t, server, leader, "cmd_party_kick_allowed", 3, "kick_party_member", map[string]any{
		"target_character_id": member.session.CharacterID,
	})
	if findOutboundMessage(leaderKickOutbound, "ack") == nil {
		t.Fatalf("expected kick ack, got %+v", leaderKickOutbound)
	}
	if findPartyNotice(leaderKickOutbound, partyNoticeStatusMemberKicked) == nil {
		t.Fatalf("expected kick notice, got %+v", leaderKickOutbound)
	}
	if findPartyNotice(member.messages, partyNoticeStatusMemberKicked) == nil {
		t.Fatalf("expected kicked member notice, got %+v", member.messages)
	}
	if _, err := store.Parties.GetByCharacterID(context.Background(), member.session.CharacterID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected kicked member to be removed from party, got err = %v", err)
	}
}

func TestServerPartyLeaderLeaveTransfersLeadershipDeterministically(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	leader := stagePartyTestClient(t, server, store, "sess_party_transfer_leader", &Character{
		ID:           "char_party_transfer_leader",
		AccountID:    "acc_party_transfer_leader",
		Name:         "Leader",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})
	firstMember := stagePartyTestClient(t, server, store, "sess_party_transfer_first", &Character{
		ID:           "char_party_transfer_first",
		AccountID:    "acc_party_transfer_first",
		Name:         "First",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})
	secondMember := stagePartyTestClient(t, server, store, "sess_party_transfer_second", &Character{
		ID:           "char_party_transfer_second",
		AccountID:    "acc_party_transfer_second",
		Name:         "Second",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -8,
		PositionZ:    0,
	})
	leader.resetMessages()
	firstMember.resetMessages()
	secondMember.resetMessages()

	_ = dispatchPartyCommand(t, server, leader, "cmd_party_transfer_invite_first", 1, "invite_party_member", map[string]any{
		"target_character_id": firstMember.session.CharacterID,
	})
	firstInviteID, _ := findPartyNotice(firstMember.messages, partyNoticeStatusInviteReceived)["invite_id"].(string)
	firstMember.resetMessages()
	leader.resetMessages()
	_ = dispatchPartyCommand(t, server, firstMember, "cmd_party_transfer_accept_first", 1, "accept_party_invite", map[string]any{
		"invite_id": firstInviteID,
	})

	time.Sleep(5 * time.Millisecond)
	leader.resetMessages()
	secondMember.resetMessages()
	_ = dispatchPartyCommand(t, server, leader, "cmd_party_transfer_invite_second", 2, "invite_party_member", map[string]any{
		"target_character_id": secondMember.session.CharacterID,
	})
	secondInviteID, _ := findPartyNotice(secondMember.messages, partyNoticeStatusInviteReceived)["invite_id"].(string)
	secondMember.resetMessages()
	leader.resetMessages()
	_ = dispatchPartyCommand(t, server, secondMember, "cmd_party_transfer_accept_second", 1, "accept_party_invite", map[string]any{
		"invite_id": secondInviteID,
	})

	firstMember.resetMessages()
	secondMember.resetMessages()
	leaveOutbound := dispatchPartyCommand(t, server, leader, "cmd_party_transfer_leave", 3, "leave_party", map[string]any{})
	if findOutboundMessage(leaveOutbound, "ack") == nil {
		t.Fatalf("expected leave ack, got %+v", leaveOutbound)
	}
	if findPartyNotice(leaveOutbound, partyNoticeStatusMemberLeft) == nil {
		t.Fatalf("expected leader leave notice, got %+v", leaveOutbound)
	}
	if findPartyNotice(firstMember.messages, partyNoticeStatusLeaderTransferred) == nil {
		t.Fatalf("expected first remaining member to receive leader transfer notice, got %+v", firstMember.messages)
	}
	if findPartyNotice(secondMember.messages, partyNoticeStatusLeaderTransferred) == nil {
		t.Fatalf("expected second remaining member to receive leader transfer notice, got %+v", secondMember.messages)
	}

	party, err := store.Parties.GetByCharacterID(context.Background(), firstMember.session.CharacterID)
	if err != nil {
		t.Fatalf("Parties.GetByCharacterID(firstMember) error = %v", err)
	}
	if party.LeaderCharacterID != firstMember.session.CharacterID {
		t.Fatalf("expected deterministic transfer to first member, got %+v", party)
	}
	members, err := store.Parties.ListMembers(context.Background(), party.ID)
	if err != nil {
		t.Fatalf("Parties.ListMembers() error = %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected two remaining members after leader leaves, got %+v", members)
	}
}
