package app

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	partyNoticeKind                    = "party_notice"
	partyNoticeStatusInviteSent        = "invite_sent"
	partyNoticeStatusInviteReceived    = "invite_received"
	partyNoticeStatusInviteAccepted    = "invite_accepted"
	partyNoticeStatusInviteDeclined    = "invite_declined"
	partyNoticeStatusInviteExpired     = "invite_expired"
	partyNoticeStatusMemberJoined      = "member_joined"
	partyNoticeStatusMemberLeft        = "member_left"
	partyNoticeStatusMemberKicked      = "member_kicked"
	partyNoticeStatusLeaderTransferred = "leader_transferred"
	partyNoticeStatusPartyDissolved    = "party_dissolved"
)

func partyNoticeMessage(
	status string,
	partyID string,
	inviteID string,
	actorCharacterID string,
	actorName string,
	targetCharacterID string,
	targetName string,
	message string,
) map[string]any {
	payload := map[string]any{
		"kind":          partyNoticeKind,
		"emitted_at_ms": time.Now().UnixMilli(),
		"status":        status,
		"message":       message,
	}
	if partyID != "" {
		payload["party_id"] = partyID
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

func partyMemberSnapshotFromCharacter(character *Character, isLeader bool) CharacterPartyMemberSnapshot {
	if character == nil {
		return CharacterPartyMemberSnapshot{IsLeader: isLeader}
	}
	return CharacterPartyMemberSnapshot{
		CharacterID: character.ID,
		Name:        character.Name,
		Level:       character.Level,
		BaseClass:   character.BaseClass,
		HP:          character.CurrentHP,
		MP:          character.CurrentMP,
		Online:      false,
		IsLeader:    isLeader,
	}
}

func (s *Server) buildPartySnapshot(ctx context.Context, party *Party) (*CharacterPartySnapshot, error) {
	if s == nil || s.store == nil || s.store.Parties == nil || party == nil {
		return nil, nil
	}

	members, err := s.store.Parties.ListMembers(ctx, party.ID)
	if err != nil {
		if errors.Is(err, errRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	snapshot := &CharacterPartySnapshot{
		PartyID:           party.ID,
		LeaderCharacterID: party.LeaderCharacterID,
		Members:           make([]CharacterPartyMemberSnapshot, 0, len(members)),
	}
	for _, member := range members {
		isLeader := member.CharacterID == party.LeaderCharacterID
		if attached := s.attachedSessionByCharacterID(member.CharacterID); attached != nil && attached.runtime != nil {
			snapshot.Members = append(snapshot.Members, attached.runtime.partyRosterMemberSnapshot(isLeader))
			continue
		}
		character, err := s.store.Characters.GetByID(ctx, member.CharacterID)
		if err != nil {
			if errors.Is(err, errRecordNotFound) {
				snapshot.Members = append(snapshot.Members, CharacterPartyMemberSnapshot{
					CharacterID: member.CharacterID,
					Name:        member.CharacterID,
					IsLeader:    isLeader,
				})
				continue
			}
			return nil, err
		}
		snapshot.Members = append(snapshot.Members, partyMemberSnapshotFromCharacter(character, isLeader))
	}
	sortPartyMemberSnapshots(snapshot.Members)
	return snapshot, nil
}

func (s *Server) loadCharacterPartyState(
	ctx context.Context,
	characterID string,
	now time.Time,
) (*CharacterPartySnapshot, []CharacterPartyInviteSnapshot, error) {
	if s == nil || s.store == nil || s.store.Parties == nil || characterID == "" {
		return nil, nil, nil
	}
	if err := s.store.Parties.ExpireInvites(ctx, now); err != nil {
		return nil, nil, err
	}

	var partySnapshot *CharacterPartySnapshot
	party, err := s.store.Parties.GetByCharacterID(ctx, characterID)
	if err == nil {
		members, membersErr := s.store.Parties.ListMembers(ctx, party.ID)
		if membersErr != nil && !errors.Is(membersErr, errRecordNotFound) {
			return nil, nil, membersErr
		}
		if len(members) <= 1 {
			_ = s.store.Parties.DeleteInvitesByParty(ctx, party.ID)
			_ = s.store.Parties.Delete(ctx, party.ID)
			party = nil
		}
	}
	if party != nil {
		partySnapshot, err = s.buildPartySnapshot(ctx, party)
		if err != nil {
			return nil, nil, err
		}
	} else if err != nil && !errors.Is(err, errRecordNotFound) {
		return nil, nil, err
	}

	invites := make([]CharacterPartyInviteSnapshot, 0)
	pendingInvites, err := s.store.Parties.ListPendingInvitesByInvitee(ctx, characterID, now)
	if err != nil && !errors.Is(err, errRecordNotFound) {
		return nil, nil, err
	}
	for _, invite := range pendingInvites {
		inviterName := invite.InviterCharacterID
		if attached := s.attachedSessionByCharacterID(invite.InviterCharacterID); attached != nil && attached.runtime != nil {
			inviterName = attached.runtime.partyRosterMemberSnapshot(false).Name
		} else if character, err := s.store.Characters.GetByID(ctx, invite.InviterCharacterID); err == nil && character.Name != "" {
			inviterName = character.Name
		}
		invites = append(invites, CharacterPartyInviteSnapshot{
			InviteID:           invite.ID,
			PartyID:            invite.PartyID,
			InviterCharacterID: invite.InviterCharacterID,
			InviterName:        inviterName,
			ExpiresAtMS:        invite.ExpiresAt.UnixMilli(),
		})
	}
	return partySnapshot, invites, nil
}

func (s *Server) sendPartyStateRefresh(ctx context.Context, characterID string) {
	if characterID == "" {
		return
	}
	attached := s.attachedSessionByCharacterID(characterID)
	if attached == nil || attached.runtime == nil {
		return
	}
	party, invites, err := s.loadCharacterPartyState(ctx, characterID, time.Now().UTC())
	if err != nil {
		s.recordStoreError("parties.load_character_state", err, errRecordNotFound)
		return
	}
	_ = attached.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
		return []map[string]any{runtime.partyDeltaMessage(party, invites)}
	})
}

func (s *Server) refreshPartyStates(ctx context.Context, characterIDs []string) {
	seen := map[string]struct{}{}
	for _, characterID := range characterIDs {
		if characterID == "" {
			continue
		}
		if _, exists := seen[characterID]; exists {
			continue
		}
		seen[characterID] = struct{}{}
		s.sendPartyStateRefresh(ctx, characterID)
	}
}

func (s *Server) refreshPartyStatesExcept(ctx context.Context, characterIDs []string, exceptCharacterID string) {
	filtered := make([]string, 0, len(characterIDs))
	for _, characterID := range characterIDs {
		if characterID == exceptCharacterID {
			continue
		}
		filtered = append(filtered, characterID)
	}
	s.refreshPartyStates(ctx, filtered)
}

func (s *Server) fanOutPartyStateForCharacterExcept(ctx context.Context, characterID string, exceptCharacterID string) {
	if s == nil || s.store == nil || s.store.Parties == nil || characterID == "" {
		return
	}

	affected := []string{characterID}
	party, err := s.store.Parties.GetByCharacterID(ctx, characterID)
	if err == nil {
		members, membersErr := s.store.Parties.ListMembers(ctx, party.ID)
		if membersErr == nil {
			for _, member := range members {
				affected = append(affected, member.CharacterID)
			}
		}
	} else if !errors.Is(err, errRecordNotFound) {
		s.recordStoreError("parties.get_by_character", err, errRecordNotFound)
		return
	}

	s.refreshPartyStatesExcept(ctx, affected, exceptCharacterID)
}

func (s *Server) fanOutPartyStateForCharacter(ctx context.Context, characterID string) {
	s.fanOutPartyStateForCharacterExcept(ctx, characterID, "")
}

func (s *Server) expirePartyInvitesForDisconnectedCharacter(ctx context.Context, characterID string) {
	if s == nil || s.store == nil || s.store.Parties == nil || characterID == "" {
		return
	}

	now := time.Now().UTC()
	s.partyMu.Lock()
	defer s.partyMu.Unlock()

	inboundInvites, err := s.store.Parties.ListPendingInvitesByInvitee(ctx, characterID, now)
	if err != nil && !errors.Is(err, errRecordNotFound) {
		s.recordStoreError("parties.list_pending_invites_by_invitee", err, errRecordNotFound)
		return
	}
	outboundInvites, err := s.store.Parties.ListPendingInvitesByInviter(ctx, characterID, now)
	if err != nil && !errors.Is(err, errRecordNotFound) {
		s.recordStoreError("parties.list_pending_invites_by_inviter", err, errRecordNotFound)
		return
	}

	affected := make([]string, 0, len(inboundInvites)+len(outboundInvites))
	for _, invite := range inboundInvites {
		if err := s.deleteInviteAndMaybeOrphanParty(ctx, &invite, now); err != nil {
			s.recordStoreError("parties.delete_invite", err, errRecordNotFound)
			continue
		}
		affected = append(affected, invite.InviterCharacterID)
		if err := s.sendOrProduceLifecycleSocialMessage(
			ctx,
			invite.InviterCharacterID,
			remotePartyNoticeEventType,
			fmt.Sprintf("party-invite/%s/disconnect-expired/%s", invite.ID, invite.InviterCharacterID),
			partyNoticeMessage(
				partyNoticeStatusInviteExpired,
				invite.PartyID,
				invite.ID,
				characterID,
				"",
				"",
				"",
				"Party invite expired because the invited player disconnected.",
			),
		); err != nil {
			s.recordStoreError("parties.publish_disconnect_expiry", err)
		}
	}
	for _, invite := range outboundInvites {
		if err := s.deleteInviteAndMaybeOrphanParty(ctx, &invite, now); err != nil {
			s.recordStoreError("parties.delete_invite", err, errRecordNotFound)
			continue
		}
		affected = append(affected, invite.InviteeCharacterID)
		if err := s.sendOrProduceLifecycleSocialMessage(
			ctx,
			invite.InviteeCharacterID,
			remotePartyNoticeEventType,
			fmt.Sprintf("party-invite/%s/disconnect-expired/%s", invite.ID, invite.InviteeCharacterID),
			partyNoticeMessage(
				partyNoticeStatusInviteExpired,
				invite.PartyID,
				invite.ID,
				characterID,
				"",
				"",
				"",
				"Party invite expired because the inviter disconnected.",
			),
		); err != nil {
			s.recordStoreError("parties.publish_disconnect_expiry", err)
		}
	}
	s.refreshPartyStates(ctx, affected)
}

func (s *Server) rejectPartyCommandWithRefresh(
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
	party, invites, err := s.loadCharacterPartyState(ctx, characterID, time.Now().UTC())
	if err != nil {
		s.recordStoreError("parties.load_character_state", err, errRecordNotFound)
		return next
	}
	next = append(next, runtime.partyDeltaMessage(party, invites))
	return next
}

func containsPartyMember(members []PartyMember, characterID string) bool {
	for _, member := range members {
		if member.CharacterID == characterID {
			return true
		}
	}
	return false
}

func selectDeterministicPartyLeader(members []PartyMember) string {
	normalized := normalizePartyMembers(members)
	if len(normalized) == 0 {
		return ""
	}
	return normalized[0].CharacterID
}

func (s *Server) cleanupOrphanParty(ctx context.Context, partyID string, now time.Time) error {
	if s == nil || s.store == nil || s.store.Parties == nil || partyID == "" {
		return nil
	}

	members, err := s.store.Parties.ListMembers(ctx, partyID)
	if err != nil && !errors.Is(err, errRecordNotFound) {
		return err
	}
	if len(members) > 0 {
		return nil
	}

	invites, err := s.store.Parties.ListPendingInvitesByParty(ctx, partyID, now)
	if err != nil && !errors.Is(err, errRecordNotFound) {
		return err
	}
	if len(invites) > 0 {
		return nil
	}

	if err := s.store.Parties.Delete(ctx, partyID); err != nil && !errors.Is(err, errRecordNotFound) {
		return err
	}
	return nil
}

func (s *Server) deleteInviteAndMaybeOrphanParty(ctx context.Context, invite *PartyInvite, now time.Time) error {
	if invite == nil {
		return nil
	}
	if err := s.store.Parties.DeleteInvite(ctx, invite.ID); err != nil && !errors.Is(err, errRecordNotFound) {
		return err
	}
	return s.cleanupOrphanParty(ctx, invite.PartyID, now)
}

func (s *Server) processPartyCommand(ctx context.Context, session *Session, runtime *attachedRuntime, command commandEnvelope) []map[string]any {
	if session == nil || runtime == nil || s == nil || s.store == nil || s.store.Parties == nil {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "internal.unexpected_error", "Gameplay party pipeline is unavailable.")}
	}

	s.partyMu.Lock()
	defer s.partyMu.Unlock()

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
	actorRegionID := runtime.regionID
	inviteTargetID := runtime.targetID
	knownTarget, targetKnown := runtime.knownEntities[inviteTargetID]
	runtime.mu.Unlock()

	now := time.Now().UTC()
	if err := s.store.Parties.ExpireInvites(ctx, now); err != nil {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist party state."))
	}

	switch parsed.commandType {
	case "invite_party_member":
		if inviteTargetID == "" || !targetKnown || knownTarget.EntityType != "player" {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.target_not_known", "Referenced player is not in the current known-set."))
		}
		if inviteTargetID == actorCharacterID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.target_invalid", "Character cannot invite itself to a party."))
		}

		presenceScope, targetAttached, targetOwnership, presenceErr := s.resolveCharacterPresence(ctx, inviteTargetID)
		if presenceErr != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to resolve authoritative player presence."))
		}
		if presenceScope != characterPresenceLocal && presenceScope != characterPresenceRemote {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.target_not_online", "Referenced player is not currently available for party invitation."))
		}
		if presenceScope == characterPresenceLocal && (targetAttached == nil || targetAttached.runtime == nil || targetAttached.runtime.regionIDValue() != actorRegionID) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.target_not_online", "Referenced player is not currently available for party invitation."))
		}
		if presenceScope == characterPresenceRemote && (targetOwnership == nil || targetOwnership.RegionID != actorRegionID) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.target_not_online", "Referenced player is not currently available for party invitation."))
		}

		if _, err := s.store.Parties.GetByCharacterID(ctx, inviteTargetID); err == nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.target_already_in_party", "Referenced player is already in a party."))
		} else if !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect party state."))
		}

		currentParty, err := s.store.Parties.GetByCharacterID(ctx, actorCharacterID)
		if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect party state."))
		}
		if err == nil && currentParty.LeaderCharacterID != actorCharacterID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.leader_required", "Only the party leader can invite a new member."))
		}

		outboundInvites, err := s.store.Parties.ListPendingInvitesByInviter(ctx, actorCharacterID, now)
		if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect party invites."))
		}
		if len(outboundInvites) > 0 {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.invite_already_pending", "Only one outbound party invite can remain pending at a time."))
		}

		createdPartyThisCommand := false
		if currentParty != nil {
			members, err := s.store.Parties.ListMembers(ctx, currentParty.ID)
			if err != nil {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect party members."))
			}
			if len(members) >= partyMaxMembers {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.party_full", "Party is already at the maximum size."))
			}
			pendingInvites, err := s.store.Parties.ListPendingInvitesByParty(ctx, currentParty.ID, now)
			if err != nil && !errors.Is(err, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect party invites."))
			}
			if len(pendingInvites) > 0 {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.invite_already_pending", "Only one outbound party invite can remain pending at a time."))
			}
		} else {
			currentParty = &Party{
				ID:                randomID("party"),
				LeaderCharacterID: actorCharacterID,
				CreatedAt:         now,
				UpdatedAt:         now,
			}
			if err := s.store.Parties.Create(ctx, currentParty, PartyMember{}); err != nil {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist party state."))
			}
			createdPartyThisCommand = true
		}

		invite := &PartyInvite{
			ID:                 randomID("party_invite"),
			PartyID:            currentParty.ID,
			InviterCharacterID: actorCharacterID,
			InviteeCharacterID: inviteTargetID,
			ExpiresAt:          now.Add(partyInviteTTL),
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := s.store.Parties.CreateInvite(ctx, invite); err != nil {
			if createdPartyThisCommand {
				_ = s.cleanupOrphanParty(ctx, currentParty.ID, now)
			}
			if errors.Is(err, errRecordConflict) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.invite_already_pending", "Referenced player already has a pending party invite."))
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist party invite state."))
		}

		actorParty, actorInvites, err := s.loadCharacterPartyState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project party state."))
		}
		targetParty, targetInvites, err := s.loadCharacterPartyState(ctx, inviteTargetID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project party state."))
		}

		targetName := inviteTargetID
		if attached := s.attachedSessionByCharacterID(inviteTargetID); attached != nil && attached.runtime != nil {
			targetName = attached.runtime.partyRosterMemberSnapshot(false).Name
		} else if character, characterErr := s.store.Characters.GetByID(ctx, inviteTargetID); characterErr == nil && character.Name != "" {
			targetName = character.Name
		}
		targetNotice := partyNoticeMessage(
			partyNoticeStatusInviteReceived,
			currentParty.ID,
			invite.ID,
			actorCharacterID,
			actorName,
			inviteTargetID,
			targetName,
			actorName+" invited you to a party.",
		)
		if targetAttached != nil {
			_ = targetAttached.dispatchAll(func(targetRuntime *attachedRuntime) []map[string]any {
				return []map[string]any{
					targetRuntime.partyDeltaMessage(targetParty, targetInvites),
					targetNotice,
				}
			})
		} else if targetOwnership != nil {
			if err := s.collectRemoteSocialDelivery(ctx, session, command, targetOwnership, inviteTargetID, remotePartyNoticeEventType, "party-invite-received", targetNotice); err != nil {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to queue remote party invitation."))
			}
		}

		outbound = append(outbound, runtime.partyDeltaMessage(actorParty, actorInvites))
		outbound = append(outbound, partyNoticeMessage(
			partyNoticeStatusInviteSent,
			currentParty.ID,
			invite.ID,
			actorCharacterID,
			actorName,
			inviteTargetID,
			targetName,
			"Party invite sent to "+targetName+".",
		))
		return outbound
	case "accept_party_invite":
		invite, err := s.store.Parties.GetInviteByID(ctx, parsed.inviteID)
		if err != nil {
			return s.rejectPartyCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "party.invite_expired", "Party invite is no longer valid.")
		}
		if invite.InviteeCharacterID != actorCharacterID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.invite_not_recipient", "Party invite is not assigned to this actor."))
		}
		if !invite.ExpiresAt.After(now) {
			_ = s.deleteInviteAndMaybeOrphanParty(ctx, invite, now)
			return s.rejectPartyCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "party.invite_expired", "Party invite has expired.")
		}
		if _, err := s.store.Parties.GetByCharacterID(ctx, actorCharacterID); err == nil {
			_ = s.deleteInviteAndMaybeOrphanParty(ctx, invite, now)
			return s.rejectPartyCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "party.already_in_party", "Character is already in a party.")
		} else if !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect party state."))
		}

		inviterScope, inviterAttached, _, inviterPresenceErr := s.resolveCharacterPresence(ctx, invite.InviterCharacterID)
		if inviterPresenceErr != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to resolve party inviter presence."))
		}
		if (inviterScope != characterPresenceLocal && inviterScope != characterPresenceRemote) || (inviterScope == characterPresenceLocal && (inviterAttached == nil || inviterAttached.runtime == nil)) {
			_ = s.deleteInviteAndMaybeOrphanParty(ctx, invite, now)
			return s.rejectPartyCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "party.invite_expired", "Party invite is no longer valid.")
		}
		party, err := s.store.Parties.GetByID(ctx, invite.PartyID)
		if err != nil {
			_ = s.deleteInviteAndMaybeOrphanParty(ctx, invite, now)
			return s.rejectPartyCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "party.invite_not_found", "Party invite is no longer valid.")
		}

		members, err := s.store.Parties.ListMembers(ctx, party.ID)
		if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect party members."))
		}
		if containsPartyMember(members, actorCharacterID) {
			_ = s.deleteInviteAndMaybeOrphanParty(ctx, invite, now)
			return s.rejectPartyCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "party.already_in_party", "Character is already in a party.")
		}
		if len(members) >= partyMaxMembers {
			_ = s.deleteInviteAndMaybeOrphanParty(ctx, invite, now)
			return s.rejectPartyCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "party.party_full", "Party is already at the maximum size.")
		}
		if len(members) == 0 {
			if existingParty, err := s.store.Parties.GetByCharacterID(ctx, invite.InviterCharacterID); err == nil && existingParty != nil {
				_ = s.deleteInviteAndMaybeOrphanParty(ctx, invite, now)
				return s.rejectPartyCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "party.invite_expired", "Party invite is no longer valid.")
			} else if err != nil && !errors.Is(err, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect party state."))
			}
			if err := s.store.Parties.AddMember(ctx, &PartyMember{
				PartyID:     party.ID,
				CharacterID: invite.InviterCharacterID,
				JoinedAt:    invite.CreatedAt,
				CreatedAt:   invite.CreatedAt,
				UpdatedAt:   now,
			}); err != nil {
				_ = s.deleteInviteAndMaybeOrphanParty(ctx, invite, now)
				if errors.Is(err, errRecordConflict) {
					return s.rejectPartyCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "party.invite_expired", "Party invite is no longer valid.")
				}
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist party membership."))
			}
			members, err = s.store.Parties.ListMembers(ctx, party.ID)
			if err != nil {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect updated party members."))
			}
		} else {
			if party.LeaderCharacterID != invite.InviterCharacterID || !containsPartyMember(members, invite.InviterCharacterID) {
				_ = s.deleteInviteAndMaybeOrphanParty(ctx, invite, now)
				return s.rejectPartyCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "party.invite_expired", "Party invite is no longer valid.")
			}
		}

		if err := s.store.Parties.AddMember(ctx, &PartyMember{
			PartyID:     party.ID,
			CharacterID: actorCharacterID,
			JoinedAt:    now,
			CreatedAt:   now,
			UpdatedAt:   now,
		}); err != nil {
			if errors.Is(err, errRecordConflict) {
				_ = s.deleteInviteAndMaybeOrphanParty(ctx, invite, now)
				return s.rejectPartyCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "party.already_in_party", "Character is already in a party.")
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist party membership."))
		}
		_ = s.store.Parties.DeleteInvite(ctx, invite.ID)

		updatedMembers, err := s.store.Parties.ListMembers(ctx, party.ID)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect updated party members."))
		}
		affected := make([]string, 0, len(updatedMembers))
		for _, member := range updatedMembers {
			affected = append(affected, member.CharacterID)
		}

		actorParty, actorInvites, err := s.loadCharacterPartyState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project party state."))
		}
		s.refreshPartyStatesExcept(ctx, affected, actorCharacterID)

		for _, member := range updatedMembers {
			if member.CharacterID == actorCharacterID {
				continue
			}
			_ = s.sendOrCollectSocialMessage(ctx, session, command, member.CharacterID, remotePartyNoticeEventType, "party-member-joined", partyNoticeMessage(
				partyNoticeStatusMemberJoined,
				party.ID,
				invite.ID,
				actorCharacterID,
				actorName,
				"",
				"",
				actorName+" joined the party.",
			))
		}

		outbound = append(outbound, runtime.partyDeltaMessage(actorParty, actorInvites))
		outbound = append(outbound, partyNoticeMessage(
			partyNoticeStatusInviteAccepted,
			party.ID,
			invite.ID,
			actorCharacterID,
			actorName,
			"",
			"",
			"You join the party.",
		))
		return outbound
	case "decline_party_invite":
		invite, err := s.store.Parties.GetInviteByID(ctx, parsed.inviteID)
		if err != nil {
			return s.rejectPartyCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "party.invite_not_found", "Party invite is not available.")
		}
		if invite.InviteeCharacterID != actorCharacterID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.invite_not_recipient", "Party invite is not assigned to this actor."))
		}
		if !invite.ExpiresAt.After(now) {
			_ = s.deleteInviteAndMaybeOrphanParty(ctx, invite, now)
			return s.rejectPartyCommandWithRefresh(ctx, outbound, runtime, actorCharacterID, command, "party.invite_expired", "Party invite has expired.")
		}
		if err := s.deleteInviteAndMaybeOrphanParty(ctx, invite, now); err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist party invite state."))
		}

		actorParty, actorInvites, err := s.loadCharacterPartyState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project party state."))
		}
		_ = s.sendOrCollectSocialMessage(ctx, session, command, invite.InviterCharacterID, remotePartyNoticeEventType, "party-invite-declined", partyNoticeMessage(
			partyNoticeStatusInviteDeclined,
			invite.PartyID,
			invite.ID,
			actorCharacterID,
			actorName,
			"",
			"",
			actorName+" declined your party invite.",
		))

		outbound = append(outbound, runtime.partyDeltaMessage(actorParty, actorInvites))
		outbound = append(outbound, partyNoticeMessage(
			partyNoticeStatusInviteDeclined,
			invite.PartyID,
			invite.ID,
			actorCharacterID,
			actorName,
			"",
			"",
			"Party invite declined.",
		))
		return outbound
	case "leave_party":
		party, err := s.store.Parties.GetByCharacterID(ctx, actorCharacterID)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.not_in_party", "Character is not currently in a party."))
		}
		pendingInvites, _ := s.store.Parties.ListPendingInvitesByParty(ctx, party.ID, now)
		pendingInviteTargets := make([]string, 0, len(pendingInvites))
		for _, invite := range pendingInvites {
			pendingInviteTargets = append(pendingInviteTargets, invite.InviteeCharacterID)
		}

		if err := s.store.Parties.RemoveMember(ctx, party.ID, actorCharacterID); err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist party membership."))
		}

		remainingMembers, err := s.store.Parties.ListMembers(ctx, party.ID)
		if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect updated party members."))
		}
		if len(remainingMembers) <= 1 {
			affected := []string{actorCharacterID}
			for _, member := range remainingMembers {
				affected = append(affected, member.CharacterID)
			}
			affected = append(affected, pendingInviteTargets...)
			_ = s.store.Parties.DeleteInvitesByParty(ctx, party.ID)
			if err := s.store.Parties.Delete(ctx, party.ID); err != nil {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist party state."))
			}
			s.refreshPartyStatesExcept(ctx, affected, actorCharacterID)
			for _, member := range remainingMembers {
				_ = s.sendOrCollectSocialMessage(ctx, session, command, member.CharacterID, remotePartyNoticeEventType, "party-dissolved", partyNoticeMessage(
					partyNoticeStatusPartyDissolved,
					party.ID,
					"",
					actorCharacterID,
					actorName,
					"",
					"",
					"The party dissolved after "+actorName+" left.",
				))
			}
			for _, inviteeCharacterID := range pendingInviteTargets {
				_ = s.sendOrCollectSocialMessage(ctx, session, command, inviteeCharacterID, remotePartyNoticeEventType, "party-invite-expired", partyNoticeMessage(
					partyNoticeStatusInviteExpired,
					party.ID,
					"",
					actorCharacterID,
					actorName,
					"",
					"",
					"Party invite expired because the party dissolved.",
				))
			}
			outbound = append(outbound, runtime.partyDeltaMessage(nil, nil))
			outbound = append(outbound, partyNoticeMessage(
				partyNoticeStatusPartyDissolved,
				party.ID,
				"",
				actorCharacterID,
				actorName,
				"",
				"",
				"You leave the party and it dissolves.",
			))
			return outbound
		}

		noticeStatus := partyNoticeStatusMemberLeft
		noticeMessage := actorName + " left the party."
		if party.LeaderCharacterID == actorCharacterID {
			nextLeaderID := selectDeterministicPartyLeader(remainingMembers)
			if nextLeaderID == "" {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to select the next party leader."))
			}
			if err := s.store.Parties.UpdateLeader(ctx, party.ID, nextLeaderID); err != nil {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist party leadership."))
			}
			_ = s.store.Parties.DeleteInvitesByParty(ctx, party.ID)
			noticeStatus = partyNoticeStatusLeaderTransferred
			noticeMessage = actorName + " left the party. Leadership transfers to the next member."
			for _, inviteeCharacterID := range pendingInviteTargets {
				_ = s.sendOrCollectSocialMessage(ctx, session, command, inviteeCharacterID, remotePartyNoticeEventType, "party-invite-expired", partyNoticeMessage(
					partyNoticeStatusInviteExpired,
					party.ID,
					"",
					actorCharacterID,
					actorName,
					"",
					"",
					"Party invite expired because the party leader left.",
				))
			}
		}

		affected := []string{actorCharacterID}
		for _, member := range remainingMembers {
			affected = append(affected, member.CharacterID)
		}
		s.refreshPartyStatesExcept(ctx, affected, actorCharacterID)

		for _, member := range remainingMembers {
			_ = s.sendOrCollectSocialMessage(ctx, session, command, member.CharacterID, remotePartyNoticeEventType, "party-"+noticeStatus, partyNoticeMessage(
				noticeStatus,
				party.ID,
				"",
				actorCharacterID,
				actorName,
				"",
				"",
				noticeMessage,
			))
		}

		outbound = append(outbound, runtime.partyDeltaMessage(nil, nil))
		outbound = append(outbound, partyNoticeMessage(
			partyNoticeStatusMemberLeft,
			party.ID,
			"",
			actorCharacterID,
			actorName,
			"",
			"",
			"You leave the party.",
		))
		return outbound
	case "kick_party_member":
		if parsed.targetID == "" {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.member_not_found", "Referenced party member is not available."))
		}
		if parsed.targetID == actorCharacterID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.cannot_kick_self", "Party leader cannot kick itself."))
		}
		party, err := s.store.Parties.GetByCharacterID(ctx, actorCharacterID)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.not_in_party", "Character is not currently in a party."))
		}
		if party.LeaderCharacterID != actorCharacterID {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.leader_required", "Only the party leader can remove a member."))
		}
		members, err := s.store.Parties.ListMembers(ctx, party.ID)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect party members."))
		}
		if !containsPartyMember(members, parsed.targetID) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "party.member_not_found", "Referenced party member is not currently in the party."))
		}
		pendingInvites, _ := s.store.Parties.ListPendingInvitesByParty(ctx, party.ID, now)
		pendingInviteTargets := make([]string, 0, len(pendingInvites))
		for _, invite := range pendingInvites {
			pendingInviteTargets = append(pendingInviteTargets, invite.InviteeCharacterID)
		}
		if err := s.store.Parties.RemoveMember(ctx, party.ID, parsed.targetID); err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist party membership."))
		}

		targetName := parsed.targetID
		if attached := s.attachedSessionByCharacterID(parsed.targetID); attached != nil && attached.runtime != nil {
			targetName = attached.runtime.partyRosterMemberSnapshot(false).Name
		} else if character, err := s.store.Characters.GetByID(ctx, parsed.targetID); err == nil && character.Name != "" {
			targetName = character.Name
		}

		updatedMembers, err := s.store.Parties.ListMembers(ctx, party.ID)
		if err != nil && !errors.Is(err, errRecordNotFound) {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to inspect updated party members."))
		}
		if len(updatedMembers) <= 1 {
			affected := []string{actorCharacterID, parsed.targetID}
			for _, member := range updatedMembers {
				affected = append(affected, member.CharacterID)
			}
			affected = append(affected, pendingInviteTargets...)
			_ = s.store.Parties.DeleteInvitesByParty(ctx, party.ID)
			if err := s.store.Parties.Delete(ctx, party.ID); err != nil {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist party state."))
			}
			s.refreshPartyStatesExcept(ctx, affected, actorCharacterID)
			for _, inviteeCharacterID := range pendingInviteTargets {
				_ = s.sendOrCollectSocialMessage(ctx, session, command, inviteeCharacterID, remotePartyNoticeEventType, "party-invite-expired", partyNoticeMessage(
					partyNoticeStatusInviteExpired,
					party.ID,
					"",
					actorCharacterID,
					actorName,
					"",
					"",
					"Party invite expired because the party dissolved.",
				))
			}
			_ = s.sendOrCollectSocialMessage(ctx, session, command, parsed.targetID, remotePartyNoticeEventType, "party-member-kicked", partyNoticeMessage(
				partyNoticeStatusMemberKicked,
				party.ID,
				"",
				actorCharacterID,
				actorName,
				parsed.targetID,
				targetName,
				"You were removed from the party by "+actorName+".",
			))
			outbound = append(outbound, runtime.partyDeltaMessage(nil, nil))
			outbound = append(outbound, partyNoticeMessage(
				partyNoticeStatusPartyDissolved,
				party.ID,
				"",
				actorCharacterID,
				actorName,
				parsed.targetID,
				targetName,
				targetName+" was removed and the party dissolved.",
			))
			return outbound
		}
		affected := []string{actorCharacterID, parsed.targetID}
		for _, member := range updatedMembers {
			affected = append(affected, member.CharacterID)
		}
		s.refreshPartyStatesExcept(ctx, affected, actorCharacterID)

		for _, member := range updatedMembers {
			_ = s.sendOrCollectSocialMessage(ctx, session, command, member.CharacterID, remotePartyNoticeEventType, "party-member-kicked", partyNoticeMessage(
				partyNoticeStatusMemberKicked,
				party.ID,
				"",
				actorCharacterID,
				actorName,
				parsed.targetID,
				targetName,
				targetName+" was removed from the party.",
			))
		}
		_ = s.sendOrCollectSocialMessage(ctx, session, command, parsed.targetID, remotePartyNoticeEventType, "party-member-kicked", partyNoticeMessage(
			partyNoticeStatusMemberKicked,
			party.ID,
			"",
			actorCharacterID,
			actorName,
			parsed.targetID,
			targetName,
			"You were removed from the party by "+actorName+".",
		))

		actorParty, actorInvites, err := s.loadCharacterPartyState(ctx, actorCharacterID, now)
		if err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to project party state."))
		}
		outbound = append(outbound, runtime.partyDeltaMessage(actorParty, actorInvites))
		outbound = append(outbound, partyNoticeMessage(
			partyNoticeStatusMemberKicked,
			party.ID,
			"",
			actorCharacterID,
			actorName,
			parsed.targetID,
			targetName,
			targetName+" was removed from the party.",
		))
		return outbound
	default:
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "protocol.invalid_envelope", "Unsupported gameplay command."))
	}
}
