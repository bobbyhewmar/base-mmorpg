package app

import "sort"

type partyLootEligibility struct {
	lootEntityID          string
	partyID               string
	eligibleCharacterIDs  []string
}

func (s *Server) applyPartyRewardSharing(session *Session, runtime *attachedRuntime, command commandEnvelope, outboundMessages []map[string]any) []map[string]any {
	if s == nil || session == nil || runtime == nil {
		return outboundMessages
	}

	rewardEvents := runtime.consumePendingPartyRewardEvents()
	if len(rewardEvents) == 0 {
		return outboundMessages
	}

	rewardShares := map[string]int{}
	lootAssignments := make([]partyLootEligibility, 0, len(rewardEvents))
	sourceCharacterID := session.CharacterID

	for _, rewardEvent := range rewardEvents {
		partyID, recipients := s.resolveEligiblePartyRewardRecipients(runtime)
		if len(recipients) <= 1 {
			rewardShares[sourceCharacterID] += rewardEvent.XPAmount
			continue
		}

		eligibleCharacterIDs := make([]string, 0, len(recipients))
		for _, recipient := range recipients {
			eligibleCharacterIDs = append(eligibleCharacterIDs, recipient.runtime.characterID)
		}

		baseShare := 0
		remainder := 0
		if rewardEvent.XPAmount > 0 {
			baseShare = rewardEvent.XPAmount / len(recipients)
			remainder = rewardEvent.XPAmount % len(recipients)
		}
		for index, recipient := range recipients {
			share := baseShare
			if index < remainder {
				share++
			}
			rewardShares[recipient.runtime.characterID] += share
		}

		if rewardEvent.LootEntityID != "" {
			lootAssignments = append(lootAssignments, partyLootEligibility{
				lootEntityID:         rewardEvent.LootEntityID,
				partyID:              partyID,
				eligibleCharacterIDs: eligibleCharacterIDs,
			})
		}
	}

	if sourceShare := rewardShares[sourceCharacterID]; sourceShare > 0 {
		changed := runtime.applySharedXP(sourceShare)
		s.persistCharacterProgression(sourceCharacterID, runtime)
		if changed {
			outboundMessages = ensureOutboundSelfDelta(runtime, command, outboundMessages)
		}
	}

	for _, assignment := range lootAssignments {
		runtime.applyLootPartyEligibility(assignment.lootEntityID, assignment.partyID, assignment.eligibleCharacterIDs)
		if patchedEntity := runtime.patchLootAppearState(assignment.lootEntityID); patchedEntity != nil {
			outboundMessages = patchOutboundEntityAppear(outboundMessages, assignment.lootEntityID, *patchedEntity)
		}
	}

	for characterID, share := range rewardShares {
		if characterID == sourceCharacterID || share <= 0 {
			continue
		}
		attached := s.attachedSessionByCharacterID(characterID)
		if attached == nil || attached.runtime == nil {
			continue
		}
		changed := attached.runtime.applySharedXP(share)
		s.persistCharacterProgression(characterID, attached.runtime)
		if changed {
			_ = attached.sendSerialized(attached.runtime.progressionDeltaMessage("", 0))
			s.fanOutPresenceState(attached.sessionID, attached.runtime)
		}
	}

	return outboundMessages
}

func (s *Server) resolveEligiblePartyRewardRecipients(sourceRuntime *attachedRuntime) (string, []*attachedSession) {
	if s == nil || sourceRuntime == nil {
		return "", nil
	}

	party := sourceRuntime.partySnapshot()
	if party == nil || party.PartyID == "" || len(party.Members) == 0 {
		return "", nil
	}

	sourceRegionID := sourceRuntime.regionIDValue()
	recipients := make([]*attachedSession, 0, len(party.Members))
	for _, member := range party.Members {
		if member.CharacterID == "" {
			continue
		}
		attached := s.attachedSessionByCharacterID(member.CharacterID)
		if attached == nil || attached.runtime == nil {
			continue
		}
		if attached.runtime.partyIDValue() != party.PartyID {
			continue
		}
		if attached.runtime.regionIDValue() != sourceRegionID {
			continue
		}
		if attached.runtime.isPlayerDeadValue() {
			continue
		}
		recipients = append(recipients, attached)
	}

	sort.Slice(recipients, func(i, j int) bool {
		return recipients[i].runtime.characterID < recipients[j].runtime.characterID
	})
	return party.PartyID, recipients
}

func patchOutboundSelfDelta(outboundMessages []map[string]any, patchedSelf map[string]any) []map[string]any {
	if patchedSelf == nil {
		return outboundMessages
	}
	for index, outbound := range outboundMessages {
		kind, _ := outbound["kind"].(string)
		if kind != "delta" {
			continue
		}
		outboundMessages[index]["self"] = patchedSelf
		return outboundMessages
	}
	return outboundMessages
}

func ensureOutboundSelfDelta(runtime *attachedRuntime, command commandEnvelope, outboundMessages []map[string]any) []map[string]any {
	if runtime == nil {
		return outboundMessages
	}

	for _, outbound := range outboundMessages {
		kind, _ := outbound["kind"].(string)
		if kind != "delta" {
			continue
		}
		return patchOutboundSelfDelta(outboundMessages, runtime.selfDeltaSnapshot())
	}

	return append(outboundMessages, runtime.progressionDeltaMessage(command.CommandID, command.CommandSeq))
}

func patchOutboundEntityAppear(outboundMessages []map[string]any, entityID string, entity runtimeEntity) []map[string]any {
	if entityID == "" {
		return outboundMessages
	}
	for index, outbound := range outboundMessages {
		kind, _ := outbound["kind"].(string)
		if kind != "entity_appear" {
			continue
		}
		currentEntity, ok := outbound["entity"].(runtimeEntity)
		if !ok || currentEntity.EntityID != entityID {
			continue
		}
		outboundMessages[index]["entity"] = entity
		return outboundMessages
	}
	return outboundMessages
}
