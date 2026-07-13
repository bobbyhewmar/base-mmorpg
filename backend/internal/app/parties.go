package app

import (
	"sort"
	"time"
)

const partyInviteTTL = 90 * time.Second

type Party struct {
	ID                string
	LeaderCharacterID string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type PartyMember struct {
	PartyID     string
	CharacterID string
	JoinedAt    time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type PartyInvite struct {
	ID                 string
	PartyID            string
	InviterCharacterID string
	InviteeCharacterID string
	ExpiresAt          time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type CharacterPartyMemberSnapshot struct {
	CharacterID string `json:"character_id"`
	Name        string `json:"name"`
	Level       int    `json:"level"`
	BaseClass   string `json:"base_class"`
	HP          int    `json:"hp"`
	MP          int    `json:"mp"`
	Online      bool   `json:"online"`
	IsLeader    bool   `json:"is_leader"`
}

type CharacterPartySnapshot struct {
	PartyID           string                         `json:"party_id"`
	LeaderCharacterID string                         `json:"leader_character_id"`
	Members           []CharacterPartyMemberSnapshot `json:"members"`
}

type CharacterPartyInviteSnapshot struct {
	InviteID           string `json:"invite_id"`
	PartyID            string `json:"party_id"`
	InviterCharacterID string `json:"inviter_character_id"`
	InviterName        string `json:"inviter_name"`
	ExpiresAtMS        int64  `json:"expires_at_ms"`
}

func cloneParty(party *Party) *Party {
	if party == nil {
		return nil
	}
	cloned := *party
	return &cloned
}

func clonePartyMembers(members []PartyMember) []PartyMember {
	if len(members) == 0 {
		return nil
	}
	cloned := make([]PartyMember, len(members))
	copy(cloned, members)
	return cloned
}

func clonePartyInvites(invites []PartyInvite) []PartyInvite {
	if len(invites) == 0 {
		return nil
	}
	cloned := make([]PartyInvite, len(invites))
	copy(cloned, invites)
	return cloned
}

func normalizePartyMembers(members []PartyMember) []PartyMember {
	normalized := clonePartyMembers(members)
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].JoinedAt.Equal(normalized[j].JoinedAt) {
			return normalized[i].CharacterID < normalized[j].CharacterID
		}
		return normalized[i].JoinedAt.Before(normalized[j].JoinedAt)
	})
	return normalized
}

func normalizePartyInvites(invites []PartyInvite) []PartyInvite {
	normalized := clonePartyInvites(invites)
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].ExpiresAt.Equal(normalized[j].ExpiresAt) {
			return normalized[i].ID < normalized[j].ID
		}
		return normalized[i].ExpiresAt.Before(normalized[j].ExpiresAt)
	})
	return normalized
}

func cloneCharacterPartySnapshot(snapshot *CharacterPartySnapshot) *CharacterPartySnapshot {
	if snapshot == nil {
		return nil
	}
	cloned := &CharacterPartySnapshot{
		PartyID:           snapshot.PartyID,
		LeaderCharacterID: snapshot.LeaderCharacterID,
		Members:           make([]CharacterPartyMemberSnapshot, len(snapshot.Members)),
	}
	copy(cloned.Members, snapshot.Members)
	return cloned
}

func cloneCharacterPartyInviteSnapshots(invites []CharacterPartyInviteSnapshot) []CharacterPartyInviteSnapshot {
	if len(invites) == 0 {
		return nil
	}
	cloned := make([]CharacterPartyInviteSnapshot, len(invites))
	copy(cloned, invites)
	return cloned
}
