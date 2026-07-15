package app

import (
	"context"
	"errors"
	"time"
)

const (
	clanNoticeKind                 = "clan_notice"
	clanNoticeStatusCreated        = "created"
	clanNoticeStatusInviteSent     = "invite_sent"
	clanNoticeStatusInviteReceived = "invite_received"
	clanNoticeStatusInviteAccepted = "invite_accepted"
	clanNoticeStatusInviteDeclined = "invite_declined"
	clanNoticeStatusInviteExpired  = "invite_expired"
	clanNoticeStatusMemberJoined   = "member_joined"
	clanNoticeStatusMemberLeft     = "member_left"
	clanNoticeStatusMemberKicked   = "member_kicked"
	clanNoticeStatusClanDissolved  = "clan_dissolved"
)

func clanNoticeMessage(
	status string,
	clanID string,
	inviteID string,
	actorCharacterID string,
	actorName string,
	targetCharacterID string,
	targetName string,
	message string,
) map[string]any {
	payload := map[string]any{
		"kind":          clanNoticeKind,
		"emitted_at_ms": time.Now().UnixMilli(),
		"status":        status,
		"message":       message,
	}
	if clanID != "" {
		payload["clan_id"] = clanID
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
	if targetCharacterID != "" {
		payload["target_character_id"] = targetCharacterID
	}
	if targetName != "" {
		payload["target_name"] = targetName
	}
	return payload
}

func clanMemberSnapshotFromCharacter(character *Character, isLeader bool) CharacterClanMemberSnapshot {
	if character == nil {
		return CharacterClanMemberSnapshot{IsLeader: isLeader}
	}
	return CharacterClanMemberSnapshot{
		CharacterID: character.ID,
		Name:        character.Name,
		Level:       character.Level,
		BaseClass:   character.BaseClass,
		Online:      false,
		IsLeader:    isLeader,
	}
}

func validClanName(name string) (string, string) {
	trimmed := normalizeClanName(name)
	if normalizedClanLookupKey(trimmed) == "" {
		return "", "clan.invalid_name"
	}
	if len([]rune(trimmed)) < clanNameMinLength {
		return "", "clan.name_too_short"
	}
	if len([]rune(trimmed)) > clanNameMaxLength {
		return "", "clan.name_too_long"
	}
	for _, r := range trimmed {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == ' ') {
			return "", "clan.name_contains_invalid_characters"
		}
	}
	return trimmed, ""
}

func clanNameValidationMessage(reasonCode string) string {
	switch reasonCode {
	case "clan.invalid_name":
		return "Clan name is invalid."
	case "clan.name_too_short":
		return "Clan name is too short."
	case "clan.name_too_long":
		return "Clan name is too long."
	case "clan.name_contains_invalid_characters":
		return "Clan name contains invalid characters."
	default:
		return "Clan name is invalid."
	}
}

func (s *Server) buildClanSnapshot(ctx context.Context, clan *Clan) (*CharacterClanSnapshot, error) {
	if s == nil || s.store == nil || s.store.Clans == nil || clan == nil {
		return nil, nil
	}

	members, err := s.store.Clans.ListMembers(ctx, clan.ID)
	if err != nil {
		if errors.Is(err, errRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	snapshot := &CharacterClanSnapshot{
		ClanID:            clan.ID,
		Name:              clan.Name,
		LeaderCharacterID: clan.LeaderCharacterID,
		Members:           make([]CharacterClanMemberSnapshot, 0, len(members)),
	}
	for _, member := range members {
		isLeader := member.CharacterID == clan.LeaderCharacterID
		if attached := s.attachedSessionByCharacterID(member.CharacterID); attached != nil && attached.runtime != nil {
			snapshot.Members = append(snapshot.Members, attached.runtime.clanRosterMemberSnapshot(isLeader))
			continue
		}
		character, err := s.store.Characters.GetByID(ctx, member.CharacterID)
		if err != nil {
			if errors.Is(err, errRecordNotFound) {
				snapshot.Members = append(snapshot.Members, CharacterClanMemberSnapshot{
					CharacterID: member.CharacterID,
					Name:        member.CharacterID,
					IsLeader:    isLeader,
				})
				continue
			}
			return nil, err
		}
		snapshot.Members = append(snapshot.Members, clanMemberSnapshotFromCharacter(character, isLeader))
	}
	sortClanMemberSnapshots(snapshot.Members)
	return snapshot, nil
}

func (s *Server) loadCharacterClanState(
	ctx context.Context,
	characterID string,
	now time.Time,
) (*CharacterClanSnapshot, []CharacterClanInviteSnapshot, error) {
	if s == nil || s.store == nil || s.store.Clans == nil || characterID == "" {
		return nil, nil, nil
	}
	if err := s.store.Clans.ExpireInvites(ctx, now); err != nil {
		return nil, nil, err
	}

	var clanSnapshot *CharacterClanSnapshot
	clan, err := s.store.Clans.GetByCharacterID(ctx, characterID)
	if err == nil {
		clanSnapshot, err = s.buildClanSnapshot(ctx, clan)
		if err != nil {
			return nil, nil, err
		}
	} else if !errors.Is(err, errRecordNotFound) {
		return nil, nil, err
	}

	invites := make([]CharacterClanInviteSnapshot, 0)
	pendingInvites, err := s.store.Clans.ListPendingInvitesByInvitee(ctx, characterID, now)
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
		clanName := invite.ClanID
		if clan, err := s.store.Clans.GetByID(ctx, invite.ClanID); err == nil && clan.Name != "" {
			clanName = clan.Name
		}
		invites = append(invites, CharacterClanInviteSnapshot{
			InviteID:           invite.ID,
			ClanID:             invite.ClanID,
			ClanName:           clanName,
			InviterCharacterID: invite.InviterCharacterID,
			InviterName:        inviterName,
			ExpiresAtMS:        invite.ExpiresAt.UnixMilli(),
		})
	}
	return clanSnapshot, invites, nil
}

func (s *Server) sendClanStateRefresh(ctx context.Context, characterID string) {
	if characterID == "" {
		return
	}
	attached := s.attachedSessionByCharacterID(characterID)
	if attached == nil || attached.runtime == nil {
		return
	}
	clan, invites, err := s.loadCharacterClanState(ctx, characterID, time.Now().UTC())
	if err != nil {
		s.recordStoreError("clans.load_character_state", err, errRecordNotFound)
		return
	}
	_ = attached.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
		return []map[string]any{runtime.clanDeltaMessage(clan, invites)}
	})
}

func (s *Server) refreshClanStates(ctx context.Context, characterIDs []string) {
	seen := map[string]struct{}{}
	for _, characterID := range characterIDs {
		if characterID == "" {
			continue
		}
		if _, exists := seen[characterID]; exists {
			continue
		}
		seen[characterID] = struct{}{}
		s.sendClanStateRefresh(ctx, characterID)
	}
}

func (s *Server) refreshClanStatesExcept(ctx context.Context, characterIDs []string, exceptCharacterID string) {
	filtered := make([]string, 0, len(characterIDs))
	for _, characterID := range characterIDs {
		if characterID == exceptCharacterID {
			continue
		}
		filtered = append(filtered, characterID)
	}
	s.refreshClanStates(ctx, filtered)
}

func (s *Server) fanOutClanStateForCharacterExcept(ctx context.Context, characterID string, exceptCharacterID string) {
	if s == nil || s.store == nil || s.store.Clans == nil || characterID == "" {
		return
	}

	affected := []string{characterID}
	clan, err := s.store.Clans.GetByCharacterID(ctx, characterID)
	if err == nil {
		members, membersErr := s.store.Clans.ListMembers(ctx, clan.ID)
		if membersErr == nil {
			for _, member := range members {
				affected = append(affected, member.CharacterID)
			}
		}
		invites, inviteErr := s.store.Clans.ListPendingInvitesByClan(ctx, clan.ID, time.Now().UTC())
		if inviteErr == nil {
			for _, invite := range invites {
				affected = append(affected, invite.InviteeCharacterID)
			}
		}
	} else if !errors.Is(err, errRecordNotFound) {
		s.recordStoreError("clans.get_by_character", err, errRecordNotFound)
		return
	}

	s.refreshClanStatesExcept(ctx, affected, exceptCharacterID)
}

func (s *Server) fanOutClanStateForCharacter(ctx context.Context, characterID string) {
	s.fanOutClanStateForCharacterExcept(ctx, characterID, "")
}

func (s *Server) expireClanInvitesForDisconnectedCharacter(ctx context.Context, characterID string) {
	if s == nil || s.store == nil || s.store.Clans == nil || characterID == "" {
		return
	}

	now := time.Now().UTC()
	s.clanMu.Lock()
	defer s.clanMu.Unlock()

	inboundInvites, err := s.store.Clans.ListPendingInvitesByInvitee(ctx, characterID, now)
	if err != nil && !errors.Is(err, errRecordNotFound) {
		s.recordStoreError("clans.list_pending_invites_by_invitee", err, errRecordNotFound)
		return
	}
	outboundInvites, err := s.store.Clans.ListPendingInvitesByInviter(ctx, characterID, now)
	if err != nil && !errors.Is(err, errRecordNotFound) {
		s.recordStoreError("clans.list_pending_invites_by_inviter", err, errRecordNotFound)
		return
	}

	affected := make([]string, 0, len(inboundInvites)+len(outboundInvites))
	for _, invite := range inboundInvites {
		if err := s.store.Clans.DeleteInvite(ctx, invite.ID); err != nil && !errors.Is(err, errRecordNotFound) {
			s.recordStoreError("clans.delete_invite", err, errRecordNotFound)
			continue
		}
		affected = append(affected, invite.InviterCharacterID)
		if attached := s.attachedSessionByCharacterID(invite.InviterCharacterID); attached != nil {
			_ = attached.sendSerialized(clanNoticeMessage(
				clanNoticeStatusInviteExpired,
				invite.ClanID,
				invite.ID,
				characterID,
				"",
				"",
				"",
				"Clan invite expired because the invited player disconnected.",
			))
		}
	}
	for _, invite := range outboundInvites {
		if err := s.store.Clans.DeleteInvite(ctx, invite.ID); err != nil && !errors.Is(err, errRecordNotFound) {
			s.recordStoreError("clans.delete_invite", err, errRecordNotFound)
			continue
		}
		affected = append(affected, invite.InviteeCharacterID)
		if attached := s.attachedSessionByCharacterID(invite.InviteeCharacterID); attached != nil {
			_ = attached.sendSerialized(clanNoticeMessage(
				clanNoticeStatusInviteExpired,
				invite.ClanID,
				invite.ID,
				characterID,
				"",
				"",
				"",
				"Clan invite expired because the inviter disconnected.",
			))
		}
	}
	s.refreshClanStates(ctx, affected)
}

func (s *Server) rejectClanCommandWithRefresh(
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
	clan, invites, err := s.loadCharacterClanState(ctx, characterID, time.Now().UTC())
	if err != nil {
		s.recordStoreError("clans.load_character_state", err, errRecordNotFound)
		return next
	}
	next = append(next, runtime.clanDeltaMessage(clan, invites))
	return next
}

func containsClanMember(members []ClanMember, characterID string) bool {
	for _, member := range members {
		if member.CharacterID == characterID {
			return true
		}
	}
	return false
}

func (s *Server) processClanCommand(ctx context.Context, session *Session, runtime *attachedRuntime, command commandEnvelope) []map[string]any {
	if session == nil || runtime == nil || s == nil || s.store == nil || s.store.Clans == nil {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "internal.unexpected_error", "Gameplay clan pipeline is unavailable.")}
	}

	s.clanMu.Lock()
	defer s.clanMu.Unlock()

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
	if err := s.store.Clans.ExpireInvites(ctx, now); err != nil {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist clan state."))
	}

	switch parsed.commandType {
	case "create_clan":
		normalizedName, reasonCode := validClanName(parsed.clanName)
		if reasonCode != "" {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, reasonCode, clanNameValidationMessage(reasonCode)))
		}
		if _, err := s.store.Clans.GetByCharacterID(ctx, actorCharacterID); err == nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.already_in_clan", "Character is already in a clan."))
		} else if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan state."))
		}
		if _, err := s.store.Clans.GetByName(ctx, normalizedName); err == nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.name_taken", "Clan name is already in use."))
		} else if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan name availability."))
		}

		clan := &Clan{
			ID:                randomID("clan"),
			Name:              normalizedName,
			LeaderCharacterID: actorCharacterID,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		leaderMember := ClanMember{
			ClanID:      clan.ID,
			CharacterID: actorCharacterID,
			JoinedAt:    now,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.store.Clans.Create(ctx, clan, leaderMember); err != nil {
			if errors.Is(err, errRecordConflict) {
				if _, actorClanErr := s.store.Clans.GetByCharacterID(ctx, actorCharacterID); actorClanErr == nil {
					return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.already_in_clan", "Character is already in a clan."))
				}
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.name_taken", "Clan name is already in use."))
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist clan state."))
		}

		actorClan, actorInvites, err := s.loadCharacterClanState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project clan state."))
		}
		outbound = append(outbound, runtime.clanCommandDeltaMessage(actorClan, actorInvites, command))
		outbound = append(outbound, clanNoticeMessage(
			clanNoticeStatusCreated,
			clan.ID,
			"",
			actorCharacterID,
			actorName,
			"",
			"",
			"You found the clan "+normalizedName+".",
		))
		return outbound
	case "invite_clan_member":
		if inviteTargetID == "" || !targetKnown || knownTarget.EntityType != "player" {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.target_not_known", "Referenced player is not in the current known-set."))
		}
		if inviteTargetID == actorCharacterID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.target_invalid", "Character cannot invite itself to a clan."))
		}
		if s.attachedSessionByCharacterID(inviteTargetID) == nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.target_not_online", "Referenced player is not currently available for clan invitation."))
		}

		clan, err := s.store.Clans.GetByCharacterID(ctx, actorCharacterID)
		if err != nil {
			if errors.Is(err, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.not_in_clan", "Character is not currently in a clan."))
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan state."))
		}
		if clan.LeaderCharacterID != actorCharacterID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.leader_required", "Only the current clan leader can do that."))
		}
		if _, err := s.store.Clans.GetByCharacterID(ctx, inviteTargetID); err == nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.target_already_in_clan", "Referenced player is already in a clan."))
		} else if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan state."))
		}
		if pendingForInvitee, err := s.store.Clans.ListPendingInvitesByInvitee(ctx, inviteTargetID, now); err == nil && len(pendingForInvitee) > 0 {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.invite_already_pending", "Referenced player already has a pending clan invite."))
		} else if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan invites."))
		}
		if pendingForClan, err := s.store.Clans.ListPendingInvitesByClan(ctx, clan.ID, now); err == nil && len(pendingForClan) > 0 {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.invite_already_pending", "Clan already has a pending invite."))
		} else if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan invites."))
		}

		invite := &ClanInvite{
			ID:                 randomID("clan_invite"),
			ClanID:             clan.ID,
			InviterCharacterID: actorCharacterID,
			InviteeCharacterID: inviteTargetID,
			ExpiresAt:          now.Add(clanInviteTTL),
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := s.store.Clans.CreateInvite(ctx, invite); err != nil {
			if errors.Is(err, errRecordConflict) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.invite_already_pending", "Referenced player already has a pending clan invite."))
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist clan invite."))
		}

		actorClan, actorInvites, err := s.loadCharacterClanState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project clan state."))
		}
		targetClan, targetInvites, err := s.loadCharacterClanState(ctx, inviteTargetID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project clan state."))
		}
		if attached := s.attachedSessionByCharacterID(inviteTargetID); attached != nil {
			_ = attached.dispatchAll(func(targetRuntime *attachedRuntime) []map[string]any {
				return []map[string]any{
					targetRuntime.clanDeltaMessage(targetClan, targetInvites),
					clanNoticeMessage(
						clanNoticeStatusInviteReceived,
						clan.ID,
						invite.ID,
						actorCharacterID,
						actorName,
						inviteTargetID,
						"",
						actorName+" invited you to join "+clan.Name+".",
					),
				}
			})
		}
		outbound = append(outbound, runtime.clanCommandDeltaMessage(actorClan, actorInvites, command))
		outbound = append(outbound, clanNoticeMessage(
			clanNoticeStatusInviteSent,
			clan.ID,
			invite.ID,
			actorCharacterID,
			actorName,
			inviteTargetID,
			"",
			"Clan invitation sent to the current player target.",
		))
		return outbound
	case "accept_clan_invite":
		if _, err := s.store.Clans.GetByCharacterID(ctx, actorCharacterID); err == nil {
			return s.rejectClanCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "clan.already_in_clan", "Character is already in a clan.")
		} else if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan state."))
		}

		invite, err := s.store.Clans.GetInviteByID(ctx, parsed.inviteID)
		if err != nil {
			if !errors.Is(err, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan invite state."))
			}
			return s.rejectClanCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "clan.invite_expired", "Clan invite is no longer valid.")
		}
		if invite.InviteeCharacterID != actorCharacterID {
			return s.rejectClanCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "clan.invite_not_recipient", "Clan invite belongs to another character.")
		}
		if !invite.ExpiresAt.After(now) {
			_ = s.store.Clans.DeleteInvite(ctx, invite.ID)
			return s.rejectClanCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "clan.invite_expired", "Clan invite has expired.")
		}

		clan, err := s.store.Clans.GetByID(ctx, invite.ClanID)
		if err != nil {
			if !errors.Is(err, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan state."))
			}
			_ = s.store.Clans.DeleteInvite(ctx, invite.ID)
			return s.rejectClanCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "clan.invite_expired", "Clan invite is no longer valid.")
		}
		inviterAttached := s.attachedSessionByCharacterID(invite.InviterCharacterID)
		if inviterAttached == nil {
			_ = s.store.Clans.DeleteInvite(ctx, invite.ID)
			return s.rejectClanCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "clan.invite_expired", "Clan invite is no longer valid.")
		}
		if clan.LeaderCharacterID != invite.InviterCharacterID {
			_ = s.store.Clans.DeleteInvite(ctx, invite.ID)
			return s.rejectClanCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "clan.invite_expired", "Clan invite is no longer valid.")
		}

		members, err := s.store.Clans.ListMembers(ctx, clan.ID)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan members."))
		}
		if !containsClanMember(members, invite.InviterCharacterID) {
			_ = s.store.Clans.DeleteInvite(ctx, invite.ID)
			return s.rejectClanCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "clan.invite_expired", "Clan invite is no longer valid.")
		}

		if err := s.store.Clans.AcceptInvite(ctx, invite.ID, &ClanMember{
			ClanID:      clan.ID,
			CharacterID: actorCharacterID,
			JoinedAt:    now,
			CreatedAt:   now,
			UpdatedAt:   now,
		}); err != nil {
			if errors.Is(err, errRecordConflict) {
				return s.rejectClanCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "clan.already_in_clan", "Character is already in a clan.")
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist clan membership."))
		}

		affected := make([]string, 0, len(members)+1)
		for _, member := range members {
			affected = append(affected, member.CharacterID)
		}
		affected = append(affected, actorCharacterID)
		actorClan, actorInvites, err := s.loadCharacterClanState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project clan state."))
		}
		s.refreshClanStatesExcept(ctx, affected, actorCharacterID)
		for _, member := range members {
			if attached := s.attachedSessionByCharacterID(member.CharacterID); attached != nil {
				_ = attached.sendSerialized(clanNoticeMessage(
					clanNoticeStatusMemberJoined,
					clan.ID,
					invite.ID,
					actorCharacterID,
					actorName,
					"",
					"",
					actorName+" joined the clan.",
				))
			}
		}
		outbound = append(outbound, runtime.clanCommandDeltaMessage(actorClan, actorInvites, command))
		outbound = append(outbound, clanNoticeMessage(
			clanNoticeStatusInviteAccepted,
			clan.ID,
			invite.ID,
			actorCharacterID,
			actorName,
			"",
			"",
			"You join the clan "+clan.Name+".",
		))
		return outbound
	case "decline_clan_invite":
		invite, err := s.store.Clans.GetInviteByID(ctx, parsed.inviteID)
		if err != nil {
			if !errors.Is(err, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan invite state."))
			}
			return s.rejectClanCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "clan.invite_expired", "Clan invite is no longer valid.")
		}
		if invite.InviteeCharacterID != actorCharacterID {
			return s.rejectClanCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "clan.invite_not_recipient", "Clan invite belongs to another character.")
		}
		if err := s.store.Clans.DeleteInvite(ctx, invite.ID); err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist clan invite state."))
		}

		actorClan, actorInvites, err := s.loadCharacterClanState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project clan state."))
		}
		outbound = append(outbound, runtime.clanCommandDeltaMessage(actorClan, actorInvites, command))
		if attached := s.attachedSessionByCharacterID(invite.InviterCharacterID); attached != nil {
			_ = attached.sendSerialized(clanNoticeMessage(
				clanNoticeStatusInviteDeclined,
				invite.ClanID,
				invite.ID,
				actorCharacterID,
				actorName,
				"",
				"",
				actorName+" declined the clan invitation.",
			))
			s.sendClanStateRefresh(ctx, invite.InviterCharacterID)
		}
		outbound = append(outbound, clanNoticeMessage(
			clanNoticeStatusInviteDeclined,
			invite.ClanID,
			invite.ID,
			actorCharacterID,
			actorName,
			"",
			"",
			"You decline the clan invitation.",
		))
		return outbound
	case "leave_clan":
		clan, err := s.store.Clans.GetByCharacterID(ctx, actorCharacterID)
		if err != nil {
			if errors.Is(err, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.not_in_clan", "Character is not currently in a clan."))
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan state."))
		}
		if clan.LeaderCharacterID == actorCharacterID {
			return s.rejectClanCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "clan.leader_cannot_leave", "Clan leader cannot leave without dissolving the clan in this phase.")
		}
		members, err := s.store.Clans.ListMembers(ctx, clan.ID)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan members."))
		}
		if !containsClanMember(members, actorCharacterID) {
			return s.rejectClanCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "clan.not_in_clan", "Character is not currently in a clan.")
		}
		if err := s.store.Clans.RemoveMember(ctx, clan.ID, actorCharacterID); err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist clan state."))
		}

		affected := make([]string, 0, len(members))
		for _, member := range members {
			affected = append(affected, member.CharacterID)
		}
		actorClan, actorInvites, err := s.loadCharacterClanState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project clan state."))
		}
		s.refreshClanStatesExcept(ctx, affected, actorCharacterID)
		for _, member := range members {
			if member.CharacterID == actorCharacterID {
				continue
			}
			if attached := s.attachedSessionByCharacterID(member.CharacterID); attached != nil {
				_ = attached.sendSerialized(clanNoticeMessage(
					clanNoticeStatusMemberLeft,
					clan.ID,
					"",
					actorCharacterID,
					actorName,
					"",
					"",
					actorName+" left the clan.",
				))
			}
		}
		outbound = append(outbound, runtime.clanCommandDeltaMessage(actorClan, actorInvites, command))
		outbound = append(outbound, clanNoticeMessage(
			clanNoticeStatusMemberLeft,
			clan.ID,
			"",
			actorCharacterID,
			actorName,
			"",
			"",
			"You leave the clan.",
		))
		return outbound
	case "kick_clan_member":
		clan, err := s.store.Clans.GetByCharacterID(ctx, actorCharacterID)
		if err != nil {
			if errors.Is(err, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.not_in_clan", "Character is not currently in a clan."))
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan state."))
		}
		if clan.LeaderCharacterID != actorCharacterID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.leader_required", "Only the current clan leader can do that."))
		}
		if parsed.targetID == actorCharacterID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.cannot_kick_self", "Clan leader cannot remove itself."))
		}
		members, err := s.store.Clans.ListMembers(ctx, clan.ID)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan members."))
		}
		if !containsClanMember(members, parsed.targetID) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.member_not_found", "Referenced player is not currently in the clan."))
		}
		if err := s.store.Clans.RemoveMember(ctx, clan.ID, parsed.targetID); err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist clan state."))
		}

		targetName := parsed.targetID
		if character, err := s.store.Characters.GetByID(ctx, parsed.targetID); err == nil && character.Name != "" {
			targetName = character.Name
		}
		affected := make([]string, 0, len(members))
		for _, member := range members {
			affected = append(affected, member.CharacterID)
		}
		actorClan, actorInvites, err := s.loadCharacterClanState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project clan state."))
		}
		s.refreshClanStatesExcept(ctx, affected, actorCharacterID)
		for _, member := range members {
			if member.CharacterID == actorCharacterID || member.CharacterID == parsed.targetID {
				continue
			}
			if attached := s.attachedSessionByCharacterID(member.CharacterID); attached != nil {
				_ = attached.sendSerialized(clanNoticeMessage(
					clanNoticeStatusMemberKicked,
					clan.ID,
					"",
					actorCharacterID,
					actorName,
					parsed.targetID,
					targetName,
					targetName+" was removed from the clan.",
				))
			}
		}
		if attached := s.attachedSessionByCharacterID(parsed.targetID); attached != nil {
			_ = attached.sendSerialized(clanNoticeMessage(
				clanNoticeStatusMemberKicked,
				clan.ID,
				"",
				actorCharacterID,
				actorName,
				parsed.targetID,
				targetName,
				"You were removed from the clan by "+actorName+".",
			))
		}
		outbound = append(outbound, runtime.clanCommandDeltaMessage(actorClan, actorInvites, command))
		outbound = append(outbound, clanNoticeMessage(
			clanNoticeStatusMemberKicked,
			clan.ID,
			"",
			actorCharacterID,
			actorName,
			parsed.targetID,
			targetName,
			targetName+" was removed from the clan.",
		))
		return outbound
	case "dissolve_clan":
		clan, err := s.store.Clans.GetByCharacterID(ctx, actorCharacterID)
		if err != nil {
			if errors.Is(err, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.not_in_clan", "Character is not currently in a clan."))
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan state."))
		}
		if clan.LeaderCharacterID != actorCharacterID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "clan.leader_required", "Only the current clan leader can do that."))
		}

		members, err := s.store.Clans.ListMembers(ctx, clan.ID)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan members."))
		}
		pendingInvites, err := s.store.Clans.ListPendingInvitesByClan(ctx, clan.ID, now)
		if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect clan invites."))
		}
		if err := s.store.Clans.Delete(ctx, clan.ID); err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist clan state."))
		}

		affected := make([]string, 0, len(members)+len(pendingInvites))
		for _, member := range members {
			affected = append(affected, member.CharacterID)
		}
		for _, invite := range pendingInvites {
			affected = append(affected, invite.InviteeCharacterID)
		}
		s.refreshClanStatesExcept(ctx, affected, actorCharacterID)
		for _, invite := range pendingInvites {
			if attached := s.attachedSessionByCharacterID(invite.InviteeCharacterID); attached != nil {
				_ = attached.sendSerialized(clanNoticeMessage(
					clanNoticeStatusInviteExpired,
					clan.ID,
					invite.ID,
					actorCharacterID,
					actorName,
					"",
					"",
					"Clan invite expired because the clan was dissolved.",
				))
			}
		}
		for _, member := range members {
			if member.CharacterID == actorCharacterID {
				continue
			}
			if attached := s.attachedSessionByCharacterID(member.CharacterID); attached != nil {
				_ = attached.sendSerialized(clanNoticeMessage(
					clanNoticeStatusClanDissolved,
					clan.ID,
					"",
					actorCharacterID,
					actorName,
					"",
					"",
					"The clan was dissolved by "+actorName+".",
				))
			}
		}
		actorClan, actorInvites, err := s.loadCharacterClanState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project clan state."))
		}
		outbound = append(outbound, runtime.clanCommandDeltaMessage(actorClan, actorInvites, command))
		outbound = append(outbound, clanNoticeMessage(
			clanNoticeStatusClanDissolved,
			clan.ID,
			"",
			actorCharacterID,
			actorName,
			"",
			"",
			"You dissolve the clan.",
		))
		return outbound
	default:
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Unsupported gameplay command."))
	}
}
