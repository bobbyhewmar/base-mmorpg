package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func findAllianceNotice(messages []map[string]any, status string) map[string]any {
	for _, message := range messages {
		if message["kind"] != allianceNoticeKind {
			continue
		}
		if messageStatus, _ := message["status"].(string); messageStatus == status {
			return message
		}
	}
	return nil
}

func requireAllianceDeltaWithClanCount(t *testing.T, messages []map[string]any, clanCount int) {
	t.Helper()
	delta := findOutboundMessage(messages, "delta")
	if delta == nil {
		t.Fatalf("expected delta, got %+v", messages)
	}
	self, ok := delta["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", delta)
	}
	switch alliance := self["alliance"].(type) {
	case *CharacterAllianceSnapshot:
		if alliance == nil || len(alliance.Members) != clanCount {
			t.Fatalf("expected %d alliance clans, got %+v", clanCount, alliance)
		}
	case CharacterAllianceSnapshot:
		if len(alliance.Members) != clanCount {
			t.Fatalf("expected %d alliance clans, got %+v", clanCount, alliance.Members)
		}
	case map[string]any:
		members, ok := alliance["members"].([]any)
		if !ok || len(members) != clanCount {
			t.Fatalf("expected %d alliance clans, got %+v", clanCount, alliance["members"])
		}
	default:
		t.Fatalf("expected alliance payload, got %+v", self["alliance"])
	}
}

func requireAllianceDeltaCleared(t *testing.T, messages []map[string]any) {
	t.Helper()
	delta := findOutboundMessage(messages, "delta")
	if delta == nil {
		t.Fatalf("expected delta, got %+v", messages)
	}
	self, ok := delta["self"].(map[string]any)
	if !ok {
		t.Fatalf("expected delta self payload, got %+v", delta)
	}
	switch alliance := self["alliance"].(type) {
	case nil:
		return
	case *CharacterAllianceSnapshot:
		if alliance == nil {
			return
		}
	}
	t.Fatalf("expected alliance to be cleared, got %+v", self["alliance"])
}

func setupAllianceClanLeader(t *testing.T, server *Server, store *Store, sessionID string, characterID string, accountID string, name string, clanName string, posX float64) *partyTestClient {
	t.Helper()
	client := stagePartyTestClient(t, server, store, sessionID, &Character{
		ID:           characterID,
		AccountID:    accountID,
		Name:         name,
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    posX,
		PositionZ:    0,
	})
	_ = dispatchPartyCommand(t, server, client, sessionID+"_create_clan", 1, "create_clan", map[string]any{
		"name": clanName,
	})
	client.resetMessages()
	return client
}

func TestServerAllianceCreateInviteAcceptLeaveExpelAndDissolve(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	founder := setupAllianceClanLeader(t, server, store, "sess_alliance_founder", "char_alliance_founder", "acc_alliance_founder", "Founder", "Nightfall", -8)
	vassal := setupAllianceClanLeader(t, server, store, "sess_alliance_vassal", "char_alliance_vassal", "acc_alliance_vassal", "Vassal", "Moonrise", -9)
	ally := setupAllianceClanLeader(t, server, store, "sess_alliance_ally", "char_alliance_ally", "acc_alliance_ally", "Ally", "Daybreak", -10)

	createAllianceOutbound := dispatchPartyCommand(t, server, founder, "cmd_alliance_create", 2, "create_alliance", map[string]any{
		"name": "Eclipse",
	})
	if findOutboundMessage(createAllianceOutbound, "ack") == nil {
		t.Fatalf("expected create alliance ack, got %+v", createAllianceOutbound)
	}
	requireAllianceDeltaWithClanCount(t, createAllianceOutbound, 1)
	if findAllianceNotice(createAllianceOutbound, allianceNoticeStatusCreated) == nil {
		t.Fatalf("expected alliance created notice, got %+v", createAllianceOutbound)
	}

	aimPartyInviteTarget(founder, vassal)
	inviteVassalOutbound := dispatchPartyCommand(t, server, founder, "cmd_alliance_invite_vassal", 3, "invite_alliance_clan", map[string]any{})
	if findAllianceNotice(inviteVassalOutbound, allianceNoticeStatusInviteSent) == nil {
		t.Fatalf("expected invite sent notice, got %+v", inviteVassalOutbound)
	}
	vassalInvite := findAllianceNotice(vassal.messages, allianceNoticeStatusInviteReceived)
	vassalInviteID, _ := vassalInvite["invite_id"].(string)
	if vassalInviteID == "" {
		t.Fatalf("expected vassal invite id, got %+v", vassalInvite)
	}
	vassal.resetMessages()
	founder.resetMessages()

	acceptVassalOutbound := dispatchPartyCommand(t, server, vassal, "cmd_alliance_accept_vassal", 2, "accept_alliance_invite", map[string]any{
		"invite_id": vassalInviteID,
	})
	requireAllianceDeltaWithClanCount(t, acceptVassalOutbound, 2)
	if findAllianceNotice(acceptVassalOutbound, allianceNoticeStatusInviteAccepted) == nil {
		t.Fatalf("expected accepted notice, got %+v", acceptVassalOutbound)
	}

	leaderLeaveOutbound := dispatchPartyCommand(t, server, founder, "cmd_alliance_leader_leave", 4, "leave_alliance", map[string]any{})
	requireRejectReason(t, leaderLeaveOutbound, "alliance.leader_clan_cannot_leave")

	memberLeaveOutbound := dispatchPartyCommand(t, server, vassal, "cmd_alliance_member_leave", 3, "leave_alliance", map[string]any{})
	requireAllianceDeltaCleared(t, memberLeaveOutbound)
	if _, err := store.Alliances.GetByCharacterID(context.Background(), vassal.session.CharacterID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected vassal clan to leave alliance, got err = %v", err)
	}

	aimPartyInviteTarget(founder, ally)
	inviteAllyOutbound := dispatchPartyCommand(t, server, founder, "cmd_alliance_invite_ally", 5, "invite_alliance_clan", map[string]any{})
	if findAllianceNotice(inviteAllyOutbound, allianceNoticeStatusInviteSent) == nil {
		t.Fatalf("expected ally invite sent notice, got %+v", inviteAllyOutbound)
	}
	allyInvite := findAllianceNotice(ally.messages, allianceNoticeStatusInviteReceived)
	allyInviteID, _ := allyInvite["invite_id"].(string)
	if allyInviteID == "" {
		t.Fatalf("expected ally invite id, got %+v", allyInvite)
	}
	ally.resetMessages()
	founder.resetMessages()

	acceptAllyOutbound := dispatchPartyCommand(t, server, ally, "cmd_alliance_accept_ally", 2, "accept_alliance_invite", map[string]any{
		"invite_id": allyInviteID,
	})
	requireAllianceDeltaWithClanCount(t, acceptAllyOutbound, 2)

	expelOutbound := dispatchPartyCommand(t, server, founder, "cmd_alliance_expel_ally", 6, "expel_alliance_clan", map[string]any{
		"target_clan_id": mustClanIDForCharacter(t, store, ally.session.CharacterID),
	})
	requireAllianceDeltaWithClanCount(t, expelOutbound, 1)
	if findAllianceNotice(expelOutbound, allianceNoticeStatusClanExpelled) == nil {
		t.Fatalf("expected expel notice, got %+v", expelOutbound)
	}

	dissolveOutbound := dispatchPartyCommand(t, server, founder, "cmd_alliance_dissolve", 7, "dissolve_alliance", map[string]any{})
	requireAllianceDeltaCleared(t, dissolveOutbound)
	if findAllianceNotice(dissolveOutbound, allianceNoticeStatusDissolved) == nil {
		t.Fatalf("expected dissolve notice, got %+v", dissolveOutbound)
	}
	if _, err := store.Alliances.GetByCharacterID(context.Background(), founder.session.CharacterID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected founder alliance to be removed, got err = %v", err)
	}
}

func TestServerAllianceRejectsInvalidTargetAndExpiresInvites(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	founder := setupAllianceClanLeader(t, server, store, "sess_alliance_exp_founder", "char_alliance_exp_founder", "acc_alliance_exp_founder", "Founder", "Nightfall", -8)
	targetLeader := setupAllianceClanLeader(t, server, store, "sess_alliance_exp_target", "char_alliance_exp_target", "acc_alliance_exp_target", "TargetLeader", "Moonrise", -9)
	targetMember := stagePartyTestClient(t, server, store, "sess_alliance_exp_member", &Character{
		ID:           "char_alliance_exp_member",
		AccountID:    "acc_alliance_exp_member",
		Name:         "TargetMember",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
		PositionX:    -9,
		PositionZ:    0,
	})
	targetClanID := mustClanIDForCharacter(t, store, targetLeader.session.CharacterID)
	now := time.Now().UTC()
	if err := store.Clans.AddMember(context.Background(), &ClanMember{
		ClanID:      targetClanID,
		CharacterID: targetMember.session.CharacterID,
		JoinedAt:    now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("Clans.AddMember() error = %v", err)
	}
	targetMember.resetMessages()

	_ = dispatchPartyCommand(t, server, founder, "cmd_alliance_exp_create", 2, "create_alliance", map[string]any{
		"name": "Eclipse",
	})
	founder.resetMessages()

	aimPartyInviteTarget(founder, targetMember)
	nonLeaderTargetOutbound := dispatchPartyCommand(t, server, founder, "cmd_alliance_non_leader_target", 3, "invite_alliance_clan", map[string]any{})
	requireRejectReason(t, nonLeaderTargetOutbound, "alliance.target_must_be_clan_leader")

	aimPartyInviteTarget(founder, targetLeader)
	inviteOutbound := dispatchPartyCommand(t, server, founder, "cmd_alliance_invite_expiry", 4, "invite_alliance_clan", map[string]any{})
	targetInvite := findAllianceNotice(targetLeader.messages, allianceNoticeStatusInviteReceived)
	inviteID, _ := targetInvite["invite_id"].(string)
	if inviteID == "" {
		t.Fatalf("expected invite id, got %+v", targetInvite)
	}

	if err := store.Alliances.ExpireInvites(context.Background(), now.Add(allianceInviteTTL+time.Second)); err != nil {
		t.Fatalf("Alliances.ExpireInvites() error = %v", err)
	}
	expiredAcceptOutbound := dispatchPartyCommand(t, server, targetLeader, "cmd_alliance_expired_accept", 2, "accept_alliance_invite", map[string]any{
		"invite_id": inviteID,
	})
	requireRejectReason(t, expiredAcceptOutbound, "alliance.invite_expired")

	targetLeader.resetMessages()
	inviteOutbound = dispatchPartyCommand(t, server, founder, "cmd_alliance_invite_disconnect", 5, "invite_alliance_clan", map[string]any{})
	if findAllianceNotice(inviteOutbound, allianceNoticeStatusInviteSent) == nil {
		t.Fatalf("expected invite sent notice, got %+v", inviteOutbound)
	}
	server.expireAllianceInvitesForDisconnectedCharacter(context.Background(), founder.session.CharacterID)
	disconnectAcceptOutbound := dispatchPartyCommand(t, server, targetLeader, "cmd_alliance_disconnect_accept", 3, "accept_alliance_invite", map[string]any{
		"invite_id": findAllianceNotice(targetLeader.messages, allianceNoticeStatusInviteReceived)["invite_id"],
	})
	requireRejectReason(t, disconnectAcceptOutbound, "alliance.invite_expired")
}

func TestServerAllianceInviteExpiryPublishesLifecycleNotice(t *testing.T) {
	store := newMemoryStore()
	server := NewServer(":0", "", store)

	founder := setupAllianceClanLeader(t, server, store, "sess_alliance_notice_founder", "char_alliance_notice_founder", "acc_alliance_notice_founder", "Founder", "Nightfall", -8)
	targetLeader := setupAllianceClanLeader(t, server, store, "sess_alliance_notice_target", "char_alliance_notice_target", "acc_alliance_notice_target", "TargetLeader", "Moonrise", -9)

	_ = dispatchPartyCommand(t, server, founder, "cmd_alliance_notice_create", 2, "create_alliance", map[string]any{
		"name": "Eclipse",
	})
	founder.resetMessages()

	aimPartyInviteTarget(founder, targetLeader)
	inviteOutbound := dispatchPartyCommand(t, server, founder, "cmd_alliance_notice_invite", 3, "invite_alliance_clan", map[string]any{})
	if findAllianceNotice(inviteOutbound, allianceNoticeStatusInviteSent) == nil {
		t.Fatalf("expected invite sent notice, got %+v", inviteOutbound)
	}
	targetInvite := findAllianceNotice(targetLeader.messages, allianceNoticeStatusInviteReceived)
	inviteID, _ := targetInvite["invite_id"].(string)
	if inviteID == "" {
		t.Fatalf("expected invite id, got %+v", targetInvite)
	}
	founder.resetMessages()
	targetLeader.resetMessages()

	repo, ok := store.Alliances.(memoryAllianceRepo)
	if !ok {
		t.Fatalf("expected memoryAllianceRepo, got %T", store.Alliances)
	}
	repo.backend.mu.Lock()
	if invite, exists := repo.backend.allianceInvites[inviteID]; exists && invite != nil {
		invite.ExpiresAt = time.Now().Add(-time.Second)
	}
	repo.backend.mu.Unlock()

	server.expireAllianceInvitesForCharacter(context.Background(), targetLeader.session.CharacterID)

	if notice := findAllianceNotice(founder.messages, allianceNoticeStatusInviteExpired); notice == nil {
		t.Fatalf("expected founder to receive invite_expired notice, got %+v", founder.messages)
	}
	if notice := findAllianceNotice(targetLeader.messages, allianceNoticeStatusInviteExpired); notice == nil {
		t.Fatalf("expected target leader to receive invite_expired notice, got %+v", targetLeader.messages)
	}
	if _, err := store.Alliances.GetInviteByID(context.Background(), inviteID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected invite to be deleted after expiry, got err = %v", err)
	}
}

func mustClanIDForCharacter(t *testing.T, store *Store, characterID string) string {
	t.Helper()
	clan, err := store.Clans.GetByCharacterID(context.Background(), characterID)
	if err != nil {
		t.Fatalf("Clans.GetByCharacterID(%s) error = %v", characterID, err)
	}
	return clan.ID
}
