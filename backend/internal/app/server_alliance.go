package app

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	allianceNoticeKind                 = "alliance_notice"
	allianceNoticeStatusCreated        = "created"
	allianceNoticeStatusInviteSent     = "invite_sent"
	allianceNoticeStatusInviteReceived = "invite_received"
	allianceNoticeStatusInviteAccepted = "invite_accepted"
	allianceNoticeStatusInviteDeclined = "invite_declined"
	allianceNoticeStatusInviteExpired  = "invite_expired"
	allianceNoticeStatusClanJoined     = "clan_joined"
	allianceNoticeStatusClanLeft       = "clan_left"
	allianceNoticeStatusClanExpelled   = "clan_expelled"
	allianceNoticeStatusDissolved      = "alliance_dissolved"
)

func allianceNoticeMessage(
	status string,
	allianceID string,
	inviteID string,
	actorCharacterID string,
	actorName string,
	targetClanID string,
	targetClanName string,
	message string,
) map[string]any {
	payload := map[string]any{
		"kind":          allianceNoticeKind,
		"emitted_at_ms": time.Now().UnixMilli(),
		"status":        status,
		"message":       message,
	}
	if allianceID != "" {
		payload["alliance_id"] = allianceID
	}
	if inviteID != "" {
		payload["invite_id"] = inviteID
	}
	if actorCharacterID != "" {
		payload["actor_character_id"] = actorCharacterID
	}
	if actorName != "" {
		payload["actor_name"] = actorName
	}
	if targetClanID != "" {
		payload["target_clan_id"] = targetClanID
	}
	if targetClanName != "" {
		payload["target_clan_name"] = targetClanName
	}
	return payload
}

func validAllianceName(name string) (string, string) {
	trimmed := normalizeAllianceName(name)
	if normalizedAllianceLookupKey(trimmed) == "" {
		return "", "alliance.invalid_name"
	}
	if len([]rune(trimmed)) < allianceNameMinLength {
		return "", "alliance.name_too_short"
	}
	if len([]rune(trimmed)) > allianceNameMaxLength {
		return "", "alliance.name_too_long"
	}
	for _, r := range trimmed {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == ' ') {
			return "", "alliance.name_contains_invalid_characters"
		}
	}
	return trimmed, ""
}

func allianceNameValidationMessage(reasonCode string) string {
	switch reasonCode {
	case "alliance.invalid_name":
		return "Alliance name is invalid."
	case "alliance.name_too_short":
		return "Alliance name is too short."
	case "alliance.name_too_long":
		return "Alliance name is too long."
	case "alliance.name_contains_invalid_characters":
		return "Alliance name contains invalid characters."
	default:
		return "Alliance name is invalid."
	}
}

func allianceClanCap() int {
	return defaultAllianceClanCap
}

func containsAllianceMember(members []AllianceMember, clanID string) bool {
	for _, member := range members {
		if member.ClanID == clanID {
			return true
		}
	}
	return false
}

func clanNameOrID(clan *Clan) string {
	if clan == nil || clan.Name == "" {
		if clan == nil {
			return ""
		}
		return clan.ID
	}
	return clan.Name
}

func (s *Server) clanCharacterIDs(ctx context.Context, clanID string) []string {
	if s == nil || s.store == nil || s.store.Clans == nil || clanID == "" {
		return nil
	}
	members, err := s.store.Clans.ListMembers(ctx, clanID)
	if err != nil {
		return nil
	}
	characterIDs := make([]string, 0, len(members))
	for _, member := range members {
		characterIDs = append(characterIDs, member.CharacterID)
	}
	return characterIDs
}

func appendUniqueCharacterIDs(dst []string, ids ...string) []string {
	seen := map[string]struct{}{}
	for _, id := range dst {
		if id != "" {
			seen[id] = struct{}{}
		}
	}
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		dst = append(dst, id)
	}
	return dst
}

func (s *Server) buildAllianceSnapshot(ctx context.Context, alliance *Alliance) (*CharacterAllianceSnapshot, error) {
	if s == nil || s.store == nil || s.store.Alliances == nil || s.store.Clans == nil || alliance == nil {
		return nil, nil
	}

	members, err := s.store.Alliances.ListMembers(ctx, alliance.ID)
	if err != nil {
		if errors.Is(err, errRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	snapshot := &CharacterAllianceSnapshot{
		AllianceID:   alliance.ID,
		Name:         alliance.Name,
		LeaderClanID: alliance.LeaderClanID,
		ClanCap:      allianceClanCap(),
		Members:      make([]CharacterAllianceMemberSnapshot, 0, len(members)),
	}
	for _, member := range members {
		clan, err := s.store.Clans.GetByID(ctx, member.ClanID)
		if err != nil {
			if errors.Is(err, errRecordNotFound) {
				continue
			}
			return nil, err
		}
		if member.ClanID == alliance.LeaderClanID {
			snapshot.LeaderClanName = clanNameOrID(clan)
		}

		leaderName := clan.LeaderCharacterID
		if attached := s.attachedSessionByCharacterID(clan.LeaderCharacterID); attached != nil && attached.runtime != nil {
			leaderName = attached.runtime.clanRosterMemberSnapshot(false).Name
		} else if character, err := s.store.Characters.GetByID(ctx, clan.LeaderCharacterID); err == nil && character.Name != "" {
			leaderName = character.Name
		}
		clanMembers, err := s.store.Clans.ListMembers(ctx, clan.ID)
		memberCount := 0
		if err == nil {
			memberCount = len(clanMembers)
		}
		snapshot.Members = append(snapshot.Members, CharacterAllianceMemberSnapshot{
			ClanID:            clan.ID,
			Name:              clan.Name,
			LeaderCharacterID: clan.LeaderCharacterID,
			LeaderName:        leaderName,
			MemberCount:       memberCount,
			IsLeaderClan:      clan.ID == alliance.LeaderClanID,
		})
	}
	sortAllianceMemberSnapshots(snapshot.Members)
	return snapshot, nil
}

func (s *Server) loadCharacterAllianceState(
	ctx context.Context,
	characterID string,
	now time.Time,
) (*CharacterAllianceSnapshot, []CharacterAllianceInviteSnapshot, error) {
	if s == nil || s.store == nil || s.store.Alliances == nil || characterID == "" {
		return nil, nil, nil
	}
	if err := s.store.Alliances.ExpireInvites(ctx, now); err != nil {
		return nil, nil, err
	}

	var allianceSnapshot *CharacterAllianceSnapshot
	alliance, err := s.store.Alliances.GetByCharacterID(ctx, characterID)
	if err == nil {
		allianceSnapshot, err = s.buildAllianceSnapshot(ctx, alliance)
		if err != nil {
			return nil, nil, err
		}
	} else if !errors.Is(err, errRecordNotFound) {
		return nil, nil, err
	}

	invites := make([]CharacterAllianceInviteSnapshot, 0)
	pendingInvites, err := s.store.Alliances.ListPendingInvitesByInvitee(ctx, characterID, now)
	if err != nil && !errors.Is(err, errRecordNotFound) {
		return nil, nil, err
	}
	for _, invite := range pendingInvites {
		inviterName := invite.InviterCharacterID
		if attached := s.attachedSessionByCharacterID(invite.InviterCharacterID); attached != nil && attached.runtime != nil {
			inviterName = attached.runtime.clanRosterMemberSnapshot(false).Name
		} else if character, err := s.store.Characters.GetByID(ctx, invite.InviterCharacterID); err == nil && character.Name != "" {
			inviterName = character.Name
		}
		allianceName := invite.AllianceID
		if alliance, err := s.store.Alliances.GetByID(ctx, invite.AllianceID); err == nil && alliance.Name != "" {
			allianceName = alliance.Name
		}
		inviterClanName := invite.InviterClanID
		if clan, err := s.store.Clans.GetByID(ctx, invite.InviterClanID); err == nil && clan.Name != "" {
			inviterClanName = clan.Name
		}
		invites = append(invites, CharacterAllianceInviteSnapshot{
			InviteID:           invite.ID,
			AllianceID:         invite.AllianceID,
			AllianceName:       allianceName,
			InviterCharacterID: invite.InviterCharacterID,
			InviterName:        inviterName,
			InviterClanID:      invite.InviterClanID,
			InviterClanName:    inviterClanName,
			TargetClanID:       invite.TargetClanID,
			ExpiresAtMS:        invite.ExpiresAt.UnixMilli(),
		})
	}
	return allianceSnapshot, invites, nil
}

func (s *Server) sendAllianceStateRefresh(ctx context.Context, characterID string) {
	if characterID == "" {
		return
	}
	attached := s.attachedSessionByCharacterID(characterID)
	if attached == nil || attached.runtime == nil {
		return
	}
	alliance, invites, err := s.loadCharacterAllianceState(ctx, characterID, time.Now().UTC())
	if err != nil {
		s.recordStoreError("alliances.load_character_state", err, errRecordNotFound)
		return
	}
	_ = attached.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
		return []map[string]any{runtime.allianceDeltaMessage(alliance, invites)}
	})
}

func (s *Server) refreshAllianceStates(ctx context.Context, characterIDs []string) {
	deferredCharacterIDs := append([]string(nil), characterIDs...)
	if deferSocialSideEffect(ctx, func() { s.refreshAllianceStates(context.Background(), deferredCharacterIDs) }) {
		return
	}
	seen := map[string]struct{}{}
	for _, characterID := range characterIDs {
		if characterID == "" {
			continue
		}
		if _, exists := seen[characterID]; exists {
			continue
		}
		seen[characterID] = struct{}{}
		s.sendAllianceStateRefresh(ctx, characterID)
	}
}

func (s *Server) refreshAllianceStatesExcept(ctx context.Context, characterIDs []string, exceptCharacterID string) {
	filtered := make([]string, 0, len(characterIDs))
	for _, characterID := range characterIDs {
		if characterID == exceptCharacterID {
			continue
		}
		filtered = append(filtered, characterID)
	}
	s.refreshAllianceStates(ctx, filtered)
}

func (s *Server) fanOutAllianceStateForCharacterExcept(ctx context.Context, characterID string, exceptCharacterID string) {
	if s == nil || s.store == nil || s.store.Alliances == nil || characterID == "" {
		return
	}

	affected := []string{characterID}
	alliance, err := s.store.Alliances.GetByCharacterID(ctx, characterID)
	if err == nil {
		members, membersErr := s.store.Alliances.ListMembers(ctx, alliance.ID)
		if membersErr == nil {
			for _, member := range members {
				affected = appendUniqueCharacterIDs(affected, s.clanCharacterIDs(ctx, member.ClanID)...)
			}
		}
		invites, inviteErr := s.store.Alliances.ListPendingInvitesByAlliance(ctx, alliance.ID, time.Now().UTC())
		if inviteErr == nil {
			for _, invite := range invites {
				affected = appendUniqueCharacterIDs(affected, invite.InviteeCharacterID)
			}
		}
	} else if !errors.Is(err, errRecordNotFound) {
		s.recordStoreError("alliances.get_by_character", err, errRecordNotFound)
		return
	}

	s.refreshAllianceStatesExcept(ctx, affected, exceptCharacterID)
}

func (s *Server) fanOutAllianceStateForCharacter(ctx context.Context, characterID string) {
	s.fanOutAllianceStateForCharacterExcept(ctx, characterID, "")
}

func (s *Server) expireAllianceInvitesForDisconnectedCharacter(ctx context.Context, characterID string) {
	if s == nil || s.store == nil || s.store.Alliances == nil || characterID == "" {
		return
	}

	now := time.Now().UTC()
	s.allianceMu.Lock()
	defer s.allianceMu.Unlock()

	inboundInvites, err := s.store.Alliances.ListPendingInvitesByInvitee(ctx, characterID, now)
	if err != nil && !errors.Is(err, errRecordNotFound) {
		s.recordStoreError("alliances.list_pending_invites_by_invitee", err, errRecordNotFound)
		return
	}
	outboundInvites, err := s.store.Alliances.ListPendingInvitesByInviter(ctx, characterID, now)
	if err != nil && !errors.Is(err, errRecordNotFound) {
		s.recordStoreError("alliances.list_pending_invites_by_inviter", err, errRecordNotFound)
		return
	}

	affected := make([]string, 0, len(inboundInvites)+len(outboundInvites))
	for _, invite := range inboundInvites {
		if err := s.store.Alliances.DeleteInvite(ctx, invite.ID); err != nil && !errors.Is(err, errRecordNotFound) {
			s.recordStoreError("alliances.delete_invite", err, errRecordNotFound)
			continue
		}
		affected = appendUniqueCharacterIDs(affected, invite.InviterCharacterID)
		if err := s.sendOrProduceLifecycleSocialMessage(
			ctx,
			invite.InviterCharacterID,
			remoteAllianceNoticeEventType,
			fmt.Sprintf("alliance-invite/%s/disconnect-expired/%s", invite.ID, invite.InviterCharacterID),
			allianceNoticeMessage(
				allianceNoticeStatusInviteExpired,
				invite.AllianceID,
				invite.ID,
				characterID,
				"",
				invite.TargetClanID,
				"",
				"Alliance invite expired because the invited clan leader disconnected.",
			),
		); err != nil {
			s.recordStoreError("alliances.publish_disconnect_expiry", err)
		}
	}
	for _, invite := range outboundInvites {
		if err := s.store.Alliances.DeleteInvite(ctx, invite.ID); err != nil && !errors.Is(err, errRecordNotFound) {
			s.recordStoreError("alliances.delete_invite", err, errRecordNotFound)
			continue
		}
		affected = appendUniqueCharacterIDs(affected, invite.InviteeCharacterID)
		if err := s.sendOrProduceLifecycleSocialMessage(
			ctx,
			invite.InviteeCharacterID,
			remoteAllianceNoticeEventType,
			fmt.Sprintf("alliance-invite/%s/disconnect-expired/%s", invite.ID, invite.InviteeCharacterID),
			allianceNoticeMessage(
				allianceNoticeStatusInviteExpired,
				invite.AllianceID,
				invite.ID,
				characterID,
				"",
				invite.TargetClanID,
				"",
				"Alliance invite expired because the inviting clan leader disconnected.",
			),
		); err != nil {
			s.recordStoreError("alliances.publish_disconnect_expiry", err)
		}
	}
	s.refreshAllianceStates(ctx, affected)
}

func (s *Server) rejectAllianceCommandWithRefresh(
	ctx context.Context,
	outbound []map[string]any,
	runtime *attachedRuntime,
	characterID string,
	command commandEnvelope,
	reasonCode string,
	message string,
) []map[string]any {
	if runtime == nil {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, reasonCode, message))
	}

	next := append(outbound, rejectMessage(command.CommandID, command.CommandSeq, reasonCode, message))
	alliance, invites, err := s.loadCharacterAllianceState(ctx, characterID, time.Now().UTC())
	if err != nil {
		s.recordStoreError("alliances.load_character_state", err, errRecordNotFound)
		return next
	}
	next = append(next, runtime.allianceDeltaMessage(alliance, invites))
	return next
}

func (s *Server) processAllianceCommand(ctx context.Context, session *Session, runtime *attachedRuntime, command commandEnvelope) []map[string]any {
	if session == nil || runtime == nil || s == nil || s.store == nil || s.store.Alliances == nil || s.store.Clans == nil {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "internal.unexpected_error", "Gameplay alliance pipeline is unavailable.")}
	}

	s.allianceMu.Lock()
	defer s.allianceMu.Unlock()

	runtime.mu.Lock()
	runtime.advanceMovementLocked(time.Now())
	parsed, reject := runtime.preValidate(command)
	if reject != nil {
		runtime.mu.Unlock()
		return []map[string]any{reject}
	}

	runtime.expectedCommandSeq++
	outbound := []map[string]any{ackMessage(command.CommandID, command.CommandSeq)}
	if runtime.isPlayerDead() {
		runtime.mu.Unlock()
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "combat.actor_dead", "Actor is currently dead."))
	}

	actorCharacterID := runtime.characterID
	actorName := runtime.characterName
	inviteTargetID := runtime.targetID
	knownTarget, targetKnown := runtime.knownEntities[inviteTargetID]
	runtime.mu.Unlock()

	now := time.Now().UTC()
	if err := s.store.Alliances.ExpireInvites(ctx, now); err != nil {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist alliance state."))
	}

	actorClan, err := s.store.Clans.GetByCharacterID(ctx, actorCharacterID)
	if err != nil {
		if errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.actor_has_no_clan", "Character must belong to a clan first."))
		}
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan state."))
	}

	switch parsed.commandType {
	case "create_alliance":
		normalizedName, reasonCode := validAllianceName(parsed.clanName)
		if reasonCode != "" {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, reasonCode, allianceNameValidationMessage(reasonCode)))
		}
		if actorClan.LeaderCharacterID != actorCharacterID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.clan_leader_required", "Only the current clan leader can do that."))
		}
		if _, err := s.store.Alliances.GetByClanID(ctx, actorClan.ID); err == nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.already_in_alliance", "Clan is already in an alliance."))
		} else if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance state."))
		}
		if _, err := s.store.Alliances.GetByName(ctx, normalizedName); err == nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.name_taken", "Alliance name is already in use."))
		} else if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance name availability."))
		}

		alliance := &Alliance{
			ID:           randomID("alliance"),
			Name:         normalizedName,
			LeaderClanID: actorClan.ID,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		founderMember := AllianceMember{
			AllianceID: alliance.ID,
			ClanID:     actorClan.ID,
			JoinedAt:   now,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := s.store.Alliances.Create(ctx, alliance, founderMember); err != nil {
			if errors.Is(err, errRecordConflict) {
				if _, actorAllianceErr := s.store.Alliances.GetByClanID(ctx, actorClan.ID); actorAllianceErr == nil {
					return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.already_in_alliance", "Clan is already in an alliance."))
				}
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.name_taken", "Alliance name is already in use."))
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist alliance state."))
		}

		actorAlliance, actorInvites, err := s.loadCharacterAllianceState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project alliance state."))
		}
		outbound = append(outbound, runtime.allianceCommandDeltaMessage(actorAlliance, actorInvites, command))
		outbound = append(outbound, allianceNoticeMessage(
			allianceNoticeStatusCreated,
			alliance.ID,
			"",
			actorCharacterID,
			actorName,
			actorClan.ID,
			actorClan.Name,
			"You found the alliance "+normalizedName+".",
		))
		return outbound
	case "invite_alliance_clan":
		if actorClan.LeaderCharacterID != actorCharacterID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.clan_leader_required", "Only the current clan leader can do that."))
		}
		if inviteTargetID == "" || !targetKnown || knownTarget.EntityType != "player" {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.target_not_known", "Referenced player is not in the current known-set."))
		}
		if inviteTargetID == actorCharacterID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.target_invalid", "Clan cannot invite itself to an alliance."))
		}
		presenceScope, targetAttached, targetOwnership, presenceErr := s.resolveCharacterPresence(ctx, inviteTargetID)
		if presenceErr != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to resolve authoritative player presence."))
		}
		if presenceScope != characterPresenceLocal && presenceScope != characterPresenceRemote {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.target_not_online", "Referenced player is not currently available for alliance invitation."))
		}

		alliance, err := s.store.Alliances.GetByClanID(ctx, actorClan.ID)
		if err != nil {
			if errors.Is(err, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.not_in_alliance", "Clan is not currently in an alliance."))
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance state."))
		}
		if alliance.LeaderClanID != actorClan.ID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.leader_required", "Only the current leader clan can do that."))
		}
		targetClan, err := s.store.Clans.GetByCharacterID(ctx, inviteTargetID)
		if err != nil {
			if errors.Is(err, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.target_has_no_clan", "Referenced player must lead a clan first."))
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect target clan state."))
		}
		if targetClan.ID == actorClan.ID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.target_invalid", "Clan cannot invite itself to an alliance."))
		}
		if targetClan.LeaderCharacterID != inviteTargetID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.target_must_be_clan_leader", "Referenced player must be the target clan leader."))
		}
		if _, err := s.store.Alliances.GetByClanID(ctx, targetClan.ID); err == nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.target_already_in_alliance", "Target clan is already in an alliance."))
		} else if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect target alliance state."))
		}
		if pendingForTargetClan, err := s.store.Alliances.ListPendingInvitesByTargetClan(ctx, targetClan.ID, now); err == nil && len(pendingForTargetClan) > 0 {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.invite_already_pending", "Target clan already has a pending alliance invite."))
		} else if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance invites."))
		}
		if pendingForAlliance, err := s.store.Alliances.ListPendingInvitesByAlliance(ctx, alliance.ID, now); err == nil && len(pendingForAlliance) > 0 {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.invite_already_pending", "Alliance already has a pending invite."))
		} else if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance invites."))
		}
		members, err := s.store.Alliances.ListMembers(ctx, alliance.ID)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance members."))
		}
		if len(members) >= allianceClanCap() {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.clan_cap_reached", "Alliance clan cap has been reached."))
		}

		invite := &AllianceInvite{
			ID:                 randomID("alliance_invite"),
			AllianceID:         alliance.ID,
			InviterClanID:      actorClan.ID,
			InviterCharacterID: actorCharacterID,
			TargetClanID:       targetClan.ID,
			InviteeCharacterID: inviteTargetID,
			ExpiresAt:          now.Add(allianceInviteTTL),
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := s.store.Alliances.CreateInvite(ctx, invite); err != nil {
			if errors.Is(err, errRecordConflict) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.invite_already_pending", "Target clan already has a pending alliance invite."))
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist alliance invite."))
		}

		actorAlliance, actorInvites, err := s.loadCharacterAllianceState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project alliance state."))
		}
		targetAlliance, targetInvites, err := s.loadCharacterAllianceState(ctx, inviteTargetID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project alliance state."))
		}
		targetNotice := allianceNoticeMessage(
			allianceNoticeStatusInviteReceived,
			alliance.ID,
			invite.ID,
			actorCharacterID,
			actorName,
			targetClan.ID,
			targetClan.Name,
			actorName+" invited your clan to join "+alliance.Name+".",
		)
		if targetAttached != nil {
			dispatch := func() {
				_ = targetAttached.dispatchAll(func(targetRuntime *attachedRuntime) []map[string]any {
					return []map[string]any{
						targetRuntime.allianceDeltaMessage(targetAlliance, targetInvites),
						targetNotice,
					}
				})
			}
			if !deferSocialSideEffect(ctx, dispatch) {
				dispatch()
			}
		} else if targetOwnership != nil {
			if err := s.collectRemoteSocialDelivery(ctx, session, command, targetOwnership, inviteTargetID, remoteAllianceNoticeEventType, "alliance-invite-received", targetNotice); err != nil {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to queue remote alliance invitation."))
			}
		}
		outbound = append(outbound, runtime.allianceCommandDeltaMessage(actorAlliance, actorInvites, command))
		outbound = append(outbound, allianceNoticeMessage(
			allianceNoticeStatusInviteSent,
			alliance.ID,
			invite.ID,
			actorCharacterID,
			actorName,
			targetClan.ID,
			targetClan.Name,
			"Alliance invitation sent to the target clan leader.",
		))
		return outbound
	case "accept_alliance_invite":
		if _, err := s.store.Alliances.GetByClanID(ctx, actorClan.ID); err == nil {
			return s.rejectAllianceCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "alliance.already_in_alliance", "Clan is already in an alliance.")
		} else if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance state."))
		}
		if actorClan.LeaderCharacterID != actorCharacterID {
			return s.rejectAllianceCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "alliance.clan_leader_required", "Only the current clan leader can answer alliance invites.")
		}

		invite, err := s.store.Alliances.GetInviteByID(ctx, parsed.inviteID)
		if err != nil {
			if !errors.Is(err, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance invite state."))
			}
			return s.rejectAllianceCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "alliance.invite_expired", "Alliance invite is no longer valid.")
		}
		if invite.InviteeCharacterID != actorCharacterID {
			return s.rejectAllianceCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "alliance.invite_not_recipient", "Alliance invite belongs to another clan leader.")
		}
		if invite.TargetClanID != actorClan.ID {
			return s.rejectAllianceCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "alliance.invite_expired", "Alliance invite is no longer valid.")
		}
		if !invite.ExpiresAt.After(now) {
			_ = s.store.Alliances.DeleteInvite(ctx, invite.ID)
			return s.rejectAllianceCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "alliance.invite_expired", "Alliance invite has expired.")
		}

		alliance, err := s.store.Alliances.GetByID(ctx, invite.AllianceID)
		if err != nil {
			if !errors.Is(err, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance state."))
			}
			_ = s.store.Alliances.DeleteInvite(ctx, invite.ID)
			return s.rejectAllianceCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "alliance.invite_expired", "Alliance invite is no longer valid.")
		}
		inviterScope, inviterAttached, _, inviterPresenceErr := s.resolveCharacterPresence(ctx, invite.InviterCharacterID)
		if inviterPresenceErr != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to resolve alliance inviter presence."))
		}
		if (inviterScope != characterPresenceLocal && inviterScope != characterPresenceRemote) || (inviterScope == characterPresenceLocal && inviterAttached == nil) {
			_ = s.store.Alliances.DeleteInvite(ctx, invite.ID)
			return s.rejectAllianceCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "alliance.invite_expired", "Alliance invite is no longer valid.")
		}
		inviterClan, err := s.store.Clans.GetByID(ctx, invite.InviterClanID)
		if err != nil || inviterClan.LeaderCharacterID != invite.InviterCharacterID || inviterClan.ID != alliance.LeaderClanID {
			_ = s.store.Alliances.DeleteInvite(ctx, invite.ID)
			return s.rejectAllianceCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "alliance.invite_expired", "Alliance invite is no longer valid.")
		}
		members, err := s.store.Alliances.ListMembers(ctx, alliance.ID)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance members."))
		}
		if !containsAllianceMember(members, inviterClan.ID) || len(members) >= allianceClanCap() {
			_ = s.store.Alliances.DeleteInvite(ctx, invite.ID)
			return s.rejectAllianceCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "alliance.invite_expired", "Alliance invite is no longer valid.")
		}

		if err := s.store.Alliances.AcceptInvite(ctx, invite.ID, &AllianceMember{
			AllianceID: alliance.ID,
			ClanID:     actorClan.ID,
			JoinedAt:   now,
			CreatedAt:  now,
			UpdatedAt:  now,
		}); err != nil {
			if errors.Is(err, errRecordConflict) {
				return s.rejectAllianceCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "alliance.already_in_alliance", "Clan is already in an alliance.")
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist alliance membership."))
		}

		affected := make([]string, 0)
		for _, member := range members {
			affected = appendUniqueCharacterIDs(affected, s.clanCharacterIDs(ctx, member.ClanID)...)
		}
		affected = appendUniqueCharacterIDs(affected, s.clanCharacterIDs(ctx, actorClan.ID)...)
		actorAlliance, actorInvites, err := s.loadCharacterAllianceState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project alliance state."))
		}
		s.refreshAllianceStatesExcept(ctx, affected, actorCharacterID)
		for _, member := range members {
			for _, characterID := range s.clanCharacterIDs(ctx, member.ClanID) {
				_ = s.sendOrCollectSocialMessage(ctx, session, command, characterID, remoteAllianceNoticeEventType, "alliance-clan-joined", allianceNoticeMessage(
					allianceNoticeStatusClanJoined,
					alliance.ID,
					invite.ID,
					actorCharacterID,
					actorName,
					actorClan.ID,
					actorClan.Name,
					actorClan.Name+" joined the alliance.",
				))
			}
		}
		outbound = append(outbound, runtime.allianceCommandDeltaMessage(actorAlliance, actorInvites, command))
		outbound = append(outbound, allianceNoticeMessage(
			allianceNoticeStatusInviteAccepted,
			alliance.ID,
			invite.ID,
			actorCharacterID,
			actorName,
			actorClan.ID,
			actorClan.Name,
			"Your clan joins the alliance "+alliance.Name+".",
		))
		return outbound
	case "decline_alliance_invite":
		if actorClan.LeaderCharacterID != actorCharacterID {
			return s.rejectAllianceCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "alliance.clan_leader_required", "Only the current clan leader can answer alliance invites.")
		}
		invite, err := s.store.Alliances.GetInviteByID(ctx, parsed.inviteID)
		if err != nil {
			if !errors.Is(err, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance invite state."))
			}
			return s.rejectAllianceCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "alliance.invite_expired", "Alliance invite is no longer valid.")
		}
		if invite.InviteeCharacterID != actorCharacterID {
			return s.rejectAllianceCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "alliance.invite_not_recipient", "Alliance invite belongs to another clan leader.")
		}
		if err := s.store.Alliances.DeleteInvite(ctx, invite.ID); err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist alliance invite state."))
		}

		actorAlliance, actorInvites, err := s.loadCharacterAllianceState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project alliance state."))
		}
		outbound = append(outbound, runtime.allianceCommandDeltaMessage(actorAlliance, actorInvites, command))
		_ = s.sendOrCollectSocialMessage(ctx, session, command, invite.InviterCharacterID, remoteAllianceNoticeEventType, "alliance-invite-declined", allianceNoticeMessage(
			allianceNoticeStatusInviteDeclined,
			invite.AllianceID,
			invite.ID,
			actorCharacterID,
			actorName,
			actorClan.ID,
			actorClan.Name,
			actorClan.Name+" declined the alliance invitation.",
		))
		s.sendAllianceStateRefresh(ctx, invite.InviterCharacterID)
		outbound = append(outbound, allianceNoticeMessage(
			allianceNoticeStatusInviteDeclined,
			invite.AllianceID,
			invite.ID,
			actorCharacterID,
			actorName,
			actorClan.ID,
			actorClan.Name,
			"Your clan declines the alliance invitation.",
		))
		return outbound
	case "leave_alliance":
		if actorClan.LeaderCharacterID != actorCharacterID {
			return s.rejectAllianceCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "alliance.clan_leader_required", "Only the current clan leader can do that.")
		}
		alliance, err := s.store.Alliances.GetByClanID(ctx, actorClan.ID)
		if err != nil {
			if errors.Is(err, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.not_in_alliance", "Clan is not currently in an alliance."))
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance state."))
		}
		if alliance.LeaderClanID == actorClan.ID {
			return s.rejectAllianceCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "alliance.leader_clan_cannot_leave", "Leader clan cannot leave the alliance in this phase.")
		}
		members, err := s.store.Alliances.ListMembers(ctx, alliance.ID)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance members."))
		}
		if !containsAllianceMember(members, actorClan.ID) {
			return s.rejectAllianceCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "alliance.not_in_alliance", "Clan is not currently in an alliance.")
		}
		if err := s.store.Alliances.RemoveMember(ctx, alliance.ID, actorClan.ID); err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist alliance state."))
		}

		affected := make([]string, 0)
		for _, member := range members {
			affected = appendUniqueCharacterIDs(affected, s.clanCharacterIDs(ctx, member.ClanID)...)
		}
		actorAlliance, actorInvites, err := s.loadCharacterAllianceState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project alliance state."))
		}
		s.refreshAllianceStatesExcept(ctx, affected, actorCharacterID)
		for _, member := range members {
			if member.ClanID == actorClan.ID {
				continue
			}
			for _, characterID := range s.clanCharacterIDs(ctx, member.ClanID) {
				_ = s.sendOrCollectSocialMessage(ctx, session, command, characterID, remoteAllianceNoticeEventType, "alliance-clan-left", allianceNoticeMessage(
					allianceNoticeStatusClanLeft,
					alliance.ID,
					"",
					actorCharacterID,
					actorName,
					actorClan.ID,
					actorClan.Name,
					actorClan.Name+" left the alliance.",
				))
			}
		}
		outbound = append(outbound, runtime.allianceCommandDeltaMessage(actorAlliance, actorInvites, command))
		outbound = append(outbound, allianceNoticeMessage(
			allianceNoticeStatusClanLeft,
			alliance.ID,
			"",
			actorCharacterID,
			actorName,
			actorClan.ID,
			actorClan.Name,
			"Your clan leaves the alliance.",
		))
		return outbound
	case "expel_alliance_clan":
		if actorClan.LeaderCharacterID != actorCharacterID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.clan_leader_required", "Only the current clan leader can do that."))
		}
		alliance, err := s.store.Alliances.GetByClanID(ctx, actorClan.ID)
		if err != nil {
			if errors.Is(err, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.not_in_alliance", "Clan is not currently in an alliance."))
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance state."))
		}
		if alliance.LeaderClanID != actorClan.ID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.leader_required", "Only the current leader clan can do that."))
		}
		if parsed.targetID == "" || parsed.targetID == actorClan.ID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.cannot_expel_leader_clan", "Leader clan cannot expel itself."))
		}
		members, err := s.store.Alliances.ListMembers(ctx, alliance.ID)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance members."))
		}
		if !containsAllianceMember(members, parsed.targetID) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.member_not_found", "Referenced clan is not currently in the alliance."))
		}
		targetClan, err := s.store.Clans.GetByID(ctx, parsed.targetID)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect target clan state."))
		}
		if err := s.store.Alliances.RemoveMember(ctx, alliance.ID, parsed.targetID); err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist alliance state."))
		}

		affected := make([]string, 0)
		for _, member := range members {
			affected = appendUniqueCharacterIDs(affected, s.clanCharacterIDs(ctx, member.ClanID)...)
		}
		actorAlliance, actorInvites, err := s.loadCharacterAllianceState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project alliance state."))
		}
		s.refreshAllianceStatesExcept(ctx, affected, actorCharacterID)
		for _, member := range members {
			if member.ClanID == actorClan.ID || member.ClanID == targetClan.ID {
				continue
			}
			for _, characterID := range s.clanCharacterIDs(ctx, member.ClanID) {
				_ = s.sendOrCollectSocialMessage(ctx, session, command, characterID, remoteAllianceNoticeEventType, "alliance-clan-expelled", allianceNoticeMessage(
					allianceNoticeStatusClanExpelled,
					alliance.ID,
					"",
					actorCharacterID,
					actorName,
					targetClan.ID,
					targetClan.Name,
					targetClan.Name+" was expelled from the alliance.",
				))
			}
		}
		for _, characterID := range s.clanCharacterIDs(ctx, targetClan.ID) {
			_ = s.sendOrCollectSocialMessage(ctx, session, command, characterID, remoteAllianceNoticeEventType, "alliance-clan-expelled", allianceNoticeMessage(
				allianceNoticeStatusClanExpelled,
				alliance.ID,
				"",
				actorCharacterID,
				actorName,
				targetClan.ID,
				targetClan.Name,
				"Your clan was expelled from the alliance by "+actorName+".",
			))
		}
		outbound = append(outbound, runtime.allianceCommandDeltaMessage(actorAlliance, actorInvites, command))
		outbound = append(outbound, allianceNoticeMessage(
			allianceNoticeStatusClanExpelled,
			alliance.ID,
			"",
			actorCharacterID,
			actorName,
			targetClan.ID,
			targetClan.Name,
			targetClan.Name+" was expelled from the alliance.",
		))
		return outbound
	case "dissolve_alliance":
		if actorClan.LeaderCharacterID != actorCharacterID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.clan_leader_required", "Only the current clan leader can do that."))
		}
		alliance, err := s.store.Alliances.GetByClanID(ctx, actorClan.ID)
		if err != nil {
			if errors.Is(err, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.not_in_alliance", "Clan is not currently in an alliance."))
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance state."))
		}
		if alliance.LeaderClanID != actorClan.ID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.leader_required", "Only the current leader clan can do that."))
		}
		members, err := s.store.Alliances.ListMembers(ctx, alliance.ID)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance members."))
		}
		if len(members) != 1 || members[0].ClanID != actorClan.ID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "alliance.dissolve_requires_only_leader_clan", "Alliance can only be dissolved when only the leader clan remains."))
		}
		pendingInvites, err := s.store.Alliances.ListPendingInvitesByAlliance(ctx, alliance.ID, now)
		if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect alliance invites."))
		}
		if err := s.store.Alliances.Delete(ctx, alliance.ID); err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist alliance state."))
		}

		affected := appendUniqueCharacterIDs([]string{}, s.clanCharacterIDs(ctx, actorClan.ID)...)
		for _, invite := range pendingInvites {
			affected = appendUniqueCharacterIDs(affected, invite.InviteeCharacterID)
		}
		s.refreshAllianceStatesExcept(ctx, affected, actorCharacterID)
		for _, invite := range pendingInvites {
			_ = s.sendOrCollectSocialMessage(ctx, session, command, invite.InviteeCharacterID, remoteAllianceNoticeEventType, "alliance-invite-expired", allianceNoticeMessage(
				allianceNoticeStatusInviteExpired,
				alliance.ID,
				invite.ID,
				actorCharacterID,
				actorName,
				invite.TargetClanID,
				"",
				"Alliance invite expired because the alliance was dissolved.",
			))
		}
		actorAlliance, actorInvites, err := s.loadCharacterAllianceState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project alliance state."))
		}
		outbound = append(outbound, runtime.allianceCommandDeltaMessage(actorAlliance, actorInvites, command))
		outbound = append(outbound, allianceNoticeMessage(
			allianceNoticeStatusDissolved,
			alliance.ID,
			"",
			actorCharacterID,
			actorName,
			actorClan.ID,
			actorClan.Name,
			"You dissolve the alliance.",
		))
		return outbound
	default:
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Unsupported gameplay command."))
	}
}
