package app

import (
	"sort"
	"strings"
	"time"
)

const (
	clanInviteTTL       = 10 * time.Second
	clanNameMinLength   = 3
	clanNameMaxLength   = 16
)

type Clan struct {
	ID                string
	Name              string
	LeaderCharacterID string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type ClanMember struct {
	ClanID      string
	CharacterID string
	JoinedAt    time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ClanInvite struct {
	ID                 string
	ClanID             string
	InviterCharacterID string
	InviteeCharacterID string
	ExpiresAt          time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type CharacterClanMemberSnapshot struct {
	CharacterID string `json:"character_id"`
	Name        string `json:"name"`
	Level       int    `json:"level"`
	BaseClass   string `json:"base_class"`
	Online      bool   `json:"online"`
	IsLeader    bool   `json:"is_leader"`
}

type CharacterClanSnapshot struct {
	ClanID            string                        `json:"clan_id"`
	Name              string                        `json:"name"`
	LeaderCharacterID string                        `json:"leader_character_id"`
	Members           []CharacterClanMemberSnapshot `json:"members"`
}

type CharacterClanInviteSnapshot struct {
	InviteID           string `json:"invite_id"`
	ClanID             string `json:"clan_id"`
	ClanName           string `json:"clan_name"`
	InviterCharacterID string `json:"inviter_character_id"`
	InviterName        string `json:"inviter_name"`
	ExpiresAtMS        int64  `json:"expires_at_ms"`
}

func normalizeClanName(name string) string {
	return strings.TrimSpace(name)
}

func normalizedClanLookupKey(name string) string {
	return normalizeName(name)
}

func cloneClan(clan *Clan) *Clan {
	if clan == nil {
		return nil
	}
	cloned := *clan
	return &cloned
}

func cloneClanMembers(members []ClanMember) []ClanMember {
	if len(members) == 0 {
		return nil
	}
	cloned := make([]ClanMember, len(members))
	copy(cloned, members)
	return cloned
}

func cloneClanInvites(invites []ClanInvite) []ClanInvite {
	if len(invites) == 0 {
		return nil
	}
	cloned := make([]ClanInvite, len(invites))
	copy(cloned, invites)
	return cloned
}

func normalizeClanMembers(members []ClanMember) []ClanMember {
	normalized := cloneClanMembers(members)
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].JoinedAt.Equal(normalized[j].JoinedAt) {
			return normalized[i].CharacterID < normalized[j].CharacterID
		}
		return normalized[i].JoinedAt.Before(normalized[j].JoinedAt)
	})
	return normalized
}

func normalizeClanInvites(invites []ClanInvite) []ClanInvite {
	normalized := cloneClanInvites(invites)
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].ExpiresAt.Equal(normalized[j].ExpiresAt) {
			return normalized[i].ID < normalized[j].ID
		}
		return normalized[i].ExpiresAt.Before(normalized[j].ExpiresAt)
	})
	return normalized
}

func cloneCharacterClanSnapshot(snapshot *CharacterClanSnapshot) *CharacterClanSnapshot {
	if snapshot == nil {
		return nil
	}
	cloned := &CharacterClanSnapshot{
		ClanID:            snapshot.ClanID,
		Name:              snapshot.Name,
		LeaderCharacterID: snapshot.LeaderCharacterID,
		Members:           make([]CharacterClanMemberSnapshot, len(snapshot.Members)),
	}
	copy(cloned.Members, snapshot.Members)
	return cloned
}

func cloneCharacterClanInviteSnapshots(invites []CharacterClanInviteSnapshot) []CharacterClanInviteSnapshot {
	if len(invites) == 0 {
		return nil
	}
	cloned := make([]CharacterClanInviteSnapshot, len(invites))
	copy(cloned, invites)
	return cloned
}
