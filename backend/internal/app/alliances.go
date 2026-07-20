package app

import (
	"sort"
	"strings"
	"time"
)

const (
	allianceInviteTTL      = 10 * time.Second
	allianceNameMinLength  = 3
	allianceNameMaxLength  = 16
	defaultAllianceClanCap = 3
)

type Alliance struct {
	ID           string
	Name         string
	LeaderClanID string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type AllianceMember struct {
	AllianceID string
	ClanID     string
	JoinedAt   time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type AllianceInvite struct {
	ID                 string
	AllianceID         string
	InviterClanID      string
	InviterCharacterID string
	TargetClanID       string
	InviteeCharacterID string
	ExpiresAt          time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type CharacterAllianceMemberSnapshot struct {
	ClanID            string `json:"clan_id"`
	Name              string `json:"name"`
	LeaderCharacterID string `json:"leader_character_id"`
	LeaderName        string `json:"leader_name"`
	MemberCount       int    `json:"member_count"`
	IsLeaderClan      bool   `json:"is_leader_clan"`
}

type CharacterAllianceSnapshot struct {
	AllianceID     string                            `json:"alliance_id"`
	Name           string                            `json:"name"`
	LeaderClanID   string                            `json:"leader_clan_id"`
	LeaderClanName string                            `json:"leader_clan_name"`
	ClanCap        int                               `json:"clan_cap"`
	Members        []CharacterAllianceMemberSnapshot `json:"members"`
}

type CharacterAllianceInviteSnapshot struct {
	InviteID           string `json:"invite_id"`
	AllianceID         string `json:"alliance_id"`
	AllianceName       string `json:"alliance_name"`
	InviterCharacterID string `json:"inviter_character_id"`
	InviterName        string `json:"inviter_name"`
	InviterClanID      string `json:"inviter_clan_id"`
	InviterClanName    string `json:"inviter_clan_name"`
	TargetClanID       string `json:"target_clan_id"`
	ExpiresAtMS        int64  `json:"expires_at_ms"`
}

func normalizeAllianceName(name string) string {
	return strings.TrimSpace(name)
}

func normalizedAllianceLookupKey(name string) string {
	return normalizeName(name)
}

func cloneAlliance(alliance *Alliance) *Alliance {
	if alliance == nil {
		return nil
	}
	cloned := *alliance
	return &cloned
}

func cloneAllianceMembers(members []AllianceMember) []AllianceMember {
	if len(members) == 0 {
		return nil
	}
	cloned := make([]AllianceMember, len(members))
	copy(cloned, members)
	return cloned
}

func cloneAllianceInvites(invites []AllianceInvite) []AllianceInvite {
	if len(invites) == 0 {
		return nil
	}
	cloned := make([]AllianceInvite, len(invites))
	copy(cloned, invites)
	return cloned
}

func normalizeAllianceMembers(members []AllianceMember) []AllianceMember {
	normalized := cloneAllianceMembers(members)
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].JoinedAt.Equal(normalized[j].JoinedAt) {
			return normalized[i].ClanID < normalized[j].ClanID
		}
		return normalized[i].JoinedAt.Before(normalized[j].JoinedAt)
	})
	return normalized
}

func normalizeAllianceInvites(invites []AllianceInvite) []AllianceInvite {
	normalized := cloneAllianceInvites(invites)
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].ExpiresAt.Equal(normalized[j].ExpiresAt) {
			return normalized[i].ID < normalized[j].ID
		}
		return normalized[i].ExpiresAt.Before(normalized[j].ExpiresAt)
	})
	return normalized
}

func cloneCharacterAllianceSnapshot(snapshot *CharacterAllianceSnapshot) *CharacterAllianceSnapshot {
	if snapshot == nil {
		return nil
	}
	cloned := &CharacterAllianceSnapshot{
		AllianceID:     snapshot.AllianceID,
		Name:           snapshot.Name,
		LeaderClanID:   snapshot.LeaderClanID,
		LeaderClanName: snapshot.LeaderClanName,
		ClanCap:        snapshot.ClanCap,
		Members:        make([]CharacterAllianceMemberSnapshot, len(snapshot.Members)),
	}
	copy(cloned.Members, snapshot.Members)
	return cloned
}

func cloneCharacterAllianceInviteSnapshots(invites []CharacterAllianceInviteSnapshot) []CharacterAllianceInviteSnapshot {
	if len(invites) == 0 {
		return nil
	}
	cloned := make([]CharacterAllianceInviteSnapshot, len(invites))
	copy(cloned, invites)
	return cloned
}
