package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryClanAcceptInviteIsAtomic(t *testing.T) {
	store := newMemoryStore()
	now := time.Now().UTC()
	leader := &Character{
		ID:           "char_store_clan_leader",
		AccountID:    "acc_store_clan_leader",
		Name:         "StoreLeader",
		BaseClass:    "Fighter",
		Sex:          "Male",
		Level:        1,
		LastRegionID: "dawn_plaza",
	}
	recruit := &Character{
		ID:           "char_store_clan_recruit",
		AccountID:    "acc_store_clan_recruit",
		Name:         "StoreRecruit",
		BaseClass:    "Mage",
		Sex:          "Female",
		Level:        1,
		LastRegionID: "dawn_plaza",
	}
	for _, character := range []*Character{leader, recruit} {
		if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
			t.Fatalf("CreateCharacterWithItemSeed(%s) error = %v", character.ID, err)
		}
	}

	clan := &Clan{
		ID:                "clan_store_atomic",
		Name:              "AtomicClan",
		LeaderCharacterID: leader.ID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := store.Clans.Create(context.Background(), clan, ClanMember{
		ClanID:      clan.ID,
		CharacterID: leader.ID,
		JoinedAt:    now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("Clans.Create() error = %v", err)
	}
	invite := &ClanInvite{
		ID:                 "clan_invite_store_atomic",
		ClanID:             clan.ID,
		InviterCharacterID: leader.ID,
		InviteeCharacterID: recruit.ID,
		ExpiresAt:          now.Add(clanInviteTTL),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := store.Clans.CreateInvite(context.Background(), invite); err != nil {
		t.Fatalf("Clans.CreateInvite() error = %v", err)
	}

	invalidMember := &ClanMember{
		ClanID:      clan.ID,
		CharacterID: "char_wrong_recipient",
		JoinedAt:    now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := store.Clans.AcceptInvite(context.Background(), invite.ID, invalidMember); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected invalid recipient to reject atomically, got %v", err)
	}
	if _, err := store.Clans.GetInviteByID(context.Background(), invite.ID); err != nil {
		t.Fatalf("expected invite to remain after failed atomic accept, got %v", err)
	}
	members, err := store.Clans.ListMembers(context.Background(), clan.ID)
	if err != nil || len(members) != 1 {
		t.Fatalf("expected failed accept not to add membership, members=%+v err=%v", members, err)
	}

	validMember := &ClanMember{
		ClanID:      clan.ID,
		CharacterID: recruit.ID,
		JoinedAt:    now.Add(time.Millisecond),
		CreatedAt:   now.Add(time.Millisecond),
		UpdatedAt:   now.Add(time.Millisecond),
	}
	if err := store.Clans.AcceptInvite(context.Background(), invite.ID, validMember); err != nil {
		t.Fatalf("Clans.AcceptInvite() error = %v", err)
	}
	if _, err := store.Clans.GetInviteByID(context.Background(), invite.ID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("expected successful accept to consume invite, got %v", err)
	}
	members, err = store.Clans.ListMembers(context.Background(), clan.ID)
	if err != nil || len(members) != 2 {
		t.Fatalf("expected successful accept to add exactly one member, members=%+v err=%v", members, err)
	}
}

func TestMemoryClanInviteEnforcesSingleLiveOutboundAndInbound(t *testing.T) {
	store := newMemoryStore()
	now := time.Now().UTC()
	characters := []*Character{
		{ID: "char_clan_invite_leader_a", AccountID: "acc_a", Name: "LeaderA", BaseClass: "Fighter", Sex: "Male", Level: 1, LastRegionID: "dawn_plaza"},
		{ID: "char_clan_invite_leader_b", AccountID: "acc_b", Name: "LeaderB", BaseClass: "Fighter", Sex: "Male", Level: 1, LastRegionID: "dawn_plaza"},
		{ID: "char_clan_invite_recruit_a", AccountID: "acc_c", Name: "RecruitA", BaseClass: "Mage", Sex: "Female", Level: 1, LastRegionID: "dawn_plaza"},
		{ID: "char_clan_invite_recruit_b", AccountID: "acc_d", Name: "RecruitB", BaseClass: "Mage", Sex: "Female", Level: 1, LastRegionID: "dawn_plaza"},
	}
	for _, character := range characters {
		if err := store.CreateCharacterWithItemSeed(context.Background(), character, initialCharacterItemSeed(character)); err != nil {
			t.Fatalf("CreateCharacterWithItemSeed(%s) error = %v", character.ID, err)
		}
	}
	createClan := func(clanID string, name string, leader *Character) {
		t.Helper()
		err := store.Clans.Create(context.Background(), &Clan{ID: clanID, Name: name, LeaderCharacterID: leader.ID, CreatedAt: now, UpdatedAt: now}, ClanMember{
			ClanID: clanID, CharacterID: leader.ID, JoinedAt: now, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatalf("Clans.Create(%s) error = %v", clanID, err)
		}
	}
	createClan("clan_invite_a", "InviteA", characters[0])
	createClan("clan_invite_b", "InviteB", characters[1])
	first := &ClanInvite{ID: "invite_live_a", ClanID: "clan_invite_a", InviterCharacterID: characters[0].ID, InviteeCharacterID: characters[2].ID, ExpiresAt: now.Add(clanInviteTTL), CreatedAt: now, UpdatedAt: now}
	if err := store.Clans.CreateInvite(context.Background(), first); err != nil {
		t.Fatalf("Clans.CreateInvite(first) error = %v", err)
	}
	secondOutbound := &ClanInvite{ID: "invite_live_b", ClanID: "clan_invite_a", InviterCharacterID: characters[0].ID, InviteeCharacterID: characters[3].ID, ExpiresAt: now.Add(clanInviteTTL), CreatedAt: now, UpdatedAt: now}
	if err := store.Clans.CreateInvite(context.Background(), secondOutbound); !errors.Is(err, errRecordConflict) {
		t.Fatalf("expected second live outbound invite to conflict, got %v", err)
	}
	secondInbound := &ClanInvite{ID: "invite_live_c", ClanID: "clan_invite_b", InviterCharacterID: characters[1].ID, InviteeCharacterID: characters[2].ID, ExpiresAt: now.Add(clanInviteTTL), CreatedAt: now, UpdatedAt: now}
	if err := store.Clans.CreateInvite(context.Background(), secondInbound); !errors.Is(err, errRecordConflict) {
		t.Fatalf("expected second live inbound invite to conflict, got %v", err)
	}
}
