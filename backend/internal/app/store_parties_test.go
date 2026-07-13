package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryPartyRepositoryLifecycle(t *testing.T) {
	store := newMemoryStore()
	ctx := context.Background()

	leader := &Character{ID: "char_party_repo_leader", AccountID: "acc_party_repo_leader", Name: "Leader", BaseClass: "Fighter", Sex: "Male", Level: 1, LastRegionID: "dawn_plaza"}
	member := &Character{ID: "char_party_repo_member", AccountID: "acc_party_repo_member", Name: "Member", BaseClass: "Mage", Sex: "Female", Level: 1, LastRegionID: "dawn_plaza"}
	invitee := &Character{ID: "char_party_repo_invitee", AccountID: "acc_party_repo_invitee", Name: "Invitee", BaseClass: "Fighter", Sex: "Female", Level: 1, LastRegionID: "dawn_plaza"}
	for _, character := range []*Character{leader, member, invitee} {
		if err := store.CreateCharacterWithItemSeed(ctx, character, initialCharacterItemSeed(character)); err != nil {
			t.Fatalf("CreateCharacterWithItemSeed(%s) error = %v", character.ID, err)
		}
	}

	now := time.Now().UTC()
	party := &Party{
		ID:                "party_repo_1",
		LeaderCharacterID: leader.ID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := store.Parties.Create(ctx, party, PartyMember{
		PartyID:     party.ID,
		CharacterID: leader.ID,
		JoinedAt:    now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("Parties.Create() error = %v", err)
	}

	loadedParty, err := store.Parties.GetByCharacterID(ctx, leader.ID)
	if err != nil {
		t.Fatalf("Parties.GetByCharacterID(leader) error = %v", err)
	}
	if loadedParty.ID != party.ID || loadedParty.LeaderCharacterID != leader.ID {
		t.Fatalf("unexpected loaded party = %+v", loadedParty)
	}

	memberJoinedAt := now.Add(time.Second)
	if err := store.Parties.AddMember(ctx, &PartyMember{
		PartyID:     party.ID,
		CharacterID: member.ID,
		JoinedAt:    memberJoinedAt,
		CreatedAt:   memberJoinedAt,
		UpdatedAt:   memberJoinedAt,
	}); err != nil {
		t.Fatalf("Parties.AddMember() error = %v", err)
	}

	members, err := store.Parties.ListMembers(ctx, party.ID)
	if err != nil {
		t.Fatalf("Parties.ListMembers() error = %v", err)
	}
	if len(members) != 2 || members[0].CharacterID != leader.ID || members[1].CharacterID != member.ID {
		t.Fatalf("unexpected members ordering = %+v", members)
	}

	invite := &PartyInvite{
		ID:                 "party_invite_repo_1",
		PartyID:            party.ID,
		InviterCharacterID: leader.ID,
		InviteeCharacterID: invitee.ID,
		ExpiresAt:          now.Add(2 * time.Minute),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := store.Parties.CreateInvite(ctx, invite); err != nil {
		t.Fatalf("Parties.CreateInvite() error = %v", err)
	}

	pendingInvites, err := store.Parties.ListPendingInvitesByInvitee(ctx, invitee.ID, now)
	if err != nil {
		t.Fatalf("Parties.ListPendingInvitesByInvitee() error = %v", err)
	}
	if len(pendingInvites) != 1 || pendingInvites[0].ID != invite.ID {
		t.Fatalf("unexpected pending invites = %+v", pendingInvites)
	}

	if err := store.Parties.UpdateLeader(ctx, party.ID, member.ID); err != nil {
		t.Fatalf("Parties.UpdateLeader() error = %v", err)
	}
	reloadedParty, err := store.Parties.GetByID(ctx, party.ID)
	if err != nil {
		t.Fatalf("Parties.GetByID() error = %v", err)
	}
	if reloadedParty.LeaderCharacterID != member.ID {
		t.Fatalf("expected leader %s, got %+v", member.ID, reloadedParty)
	}

	if err := store.Parties.RemoveMember(ctx, party.ID, leader.ID); err != nil {
		t.Fatalf("Parties.RemoveMember() error = %v", err)
	}
	if _, err := store.Parties.GetByCharacterID(ctx, leader.ID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected leader membership removal, got err = %v", err)
	}

	if err := store.Parties.ExpireInvites(ctx, now.Add(3*time.Minute)); err != nil {
		t.Fatalf("Parties.ExpireInvites() error = %v", err)
	}
	if _, err := store.Parties.GetInviteByID(ctx, invite.ID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected invite expiration, got err = %v", err)
	}
}
