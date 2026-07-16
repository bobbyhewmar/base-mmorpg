package app

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	chatMessageKind      = "chat_message"
	chatMessageMaxLength = 240
	chatRateLimitBurst   = 4
	chatRateLimitWindow  = 4 * time.Second
)

func normalizeChatMessageText(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(raw))
	for _, value := range raw {
		switch {
		case value == '\n' || value == '\r' || value == '\t':
			builder.WriteRune(' ')
		case unicode.IsControl(value):
			continue
		default:
			builder.WriteRune(value)
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func chatMessagePayload(
	commandID string,
	commandSeq int,
	channel string,
	senderCharacterID string,
	senderName string,
	targetCharacterID string,
	targetCharacterName string,
	regionID string,
	text string,
	emittedAt time.Time,
) map[string]any {
	payload := map[string]any{
		"kind":                chatMessageKind,
		"emitted_at_ms":       emittedAt.UnixMilli(),
		"channel":             channel,
		"sender_character_id": senderCharacterID,
		"sender_name":         senderName,
		"text":                text,
	}
	if commandID != "" {
		payload["command_id"] = commandID
		payload["command_seq"] = commandSeq
	}
	if targetCharacterID != "" {
		payload["target_character_id"] = targetCharacterID
	}
	if targetCharacterName != "" {
		payload["target_character_name"] = targetCharacterName
	}
	if regionID != "" {
		payload["region_id"] = regionID
	}
	return payload
}

func (runtime *attachedRuntime) consumeChatRateLimit(now time.Time) bool {
	if runtime == nil {
		return false
	}
	if runtime.chatRateWindowStartedAt.IsZero() || now.Sub(runtime.chatRateWindowStartedAt) >= chatRateLimitWindow {
		runtime.chatRateWindowStartedAt = now
		runtime.chatRateWindowCount = 1
		return true
	}
	if runtime.chatRateWindowCount >= chatRateLimitBurst {
		return false
	}
	runtime.chatRateWindowCount++
	return true
}

func (s *Server) attachedSessionByCharacterName(characterName string) *attachedSession {
	if s == nil {
		return nil
	}

	normalizedName := normalizeName(characterName)
	if normalizedName == "" {
		return nil
	}

	s.attachedMu.Lock()
	defer s.attachedMu.Unlock()

	for _, attached := range s.attached {
		if attached == nil || attached.runtime == nil || attached.send == nil || !attached.ready {
			continue
		}
		if normalizeName(attached.runtime.characterName) == normalizedName {
			return attached
		}
	}
	return nil
}

func (s *Server) processChatCommand(ctx context.Context, session *Session, runtime *attachedRuntime, command commandEnvelope) []map[string]any {
	if session == nil || runtime == nil || s == nil || s.store == nil || s.store.ChatMessages == nil {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "internal.unexpected_error", "Gameplay chat pipeline is unavailable.")}
	}

	now := time.Now().UTC()
	runtime.mu.Lock()
	runtime.advanceMovementLocked(now)
	parsed, reject := runtime.preValidate(command)
	if reject != nil {
		runtime.mu.Unlock()
		return []map[string]any{reject}
	}

	runtime.expectedCommandSeq++
	outbound := []map[string]any{ackMessage(command.CommandID, command.CommandSeq)}

	channel := strings.TrimSpace(strings.ToLower(parsed.chatChannel))
	if channel != chatChannelRegion && channel != chatChannelParty && channel != chatChannelWhisper {
		runtime.mu.Unlock()
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "chat.channel_unknown", "Chat channel is not supported."))
	}

	text := normalizeChatMessageText(parsed.chatText)
	if text == "" {
		runtime.mu.Unlock()
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "chat.message_empty", "Chat message cannot be empty."))
	}
	if utf8.RuneCountInString(text) > chatMessageMaxLength {
		runtime.mu.Unlock()
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "chat.message_too_long", "Chat message exceeds the current maximum length."))
	}
	if !runtime.consumeChatRateLimit(now) {
		runtime.mu.Unlock()
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "chat.rate_limited", "Chat is temporarily rate limited for this session."))
	}

	actorCharacterID := runtime.characterID
	actorSessionID := runtime.sessionID
	actorName := runtime.characterName
	actorRegionID := runtime.regionID
	if channel == chatChannelRegion && strings.TrimSpace(actorRegionID) == "" {
		runtime.mu.Unlock()
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "chat.region_unavailable", "Region chat requires an authoritative current region."))
	}

	var partyMemberIDs []string
	if channel == chatChannelParty {
		if runtime.party == nil || len(runtime.party.Members) == 0 {
			runtime.mu.Unlock()
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "chat.party_required", "Party chat requires current party membership."))
		}
		partyMemberIDs = make([]string, 0, len(runtime.party.Members))
		for _, member := range runtime.party.Members {
			if member.CharacterID == "" {
				continue
			}
			partyMemberIDs = append(partyMemberIDs, member.CharacterID)
		}
	}
	runtime.mu.Unlock()

	targetCharacterName := ""
	targetCharacterID := ""
	var whisperTarget *attachedSession
	var whisperOwnership *SessionOwnership
	if channel == chatChannelWhisper {
		targetCharacterName = strings.TrimSpace(parsed.chatTargetName)
		if targetCharacterName == "" {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "chat.whisper_target_required", "Whisper requires a target character name."))
		}
		targetCharacter, targetErr := s.store.Characters.GetByName(ctx, targetCharacterName)
		if targetErr != nil {
			if !errors.Is(targetErr, errRecordNotFound) {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to resolve whisper target."))
			}
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "chat.whisper_target_not_found", "Whisper target is not currently online."))
		}
		targetCharacterID = targetCharacter.ID
		targetCharacterName = targetCharacter.Name
		presenceScope, targetAttached, ownership, presenceErr := s.resolveCharacterPresence(ctx, targetCharacterID)
		if presenceErr != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to resolve whisper target presence."))
		}
		switch presenceScope {
		case characterPresenceLocal:
			if targetAttached == nil || targetAttached.runtime == nil {
				return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "chat.whisper_target_not_found", "Whisper target is not currently online."))
			}
			whisperTarget = targetAttached
		case characterPresenceRemote:
			whisperOwnership = ownership
		default:
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "chat.whisper_target_not_found", "Whisper target is not currently online."))
		}
	}

	record := ChatMessageRecord{
		ID:                randomID("chat"),
		CharacterID:       actorCharacterID,
		AccountID:         session.AccountID,
		Channel:           channel,
		TargetCharacterID: targetCharacterID,
		Text:              text,
		SessionID:         session.ID,
		CommandID:         command.CommandID,
		CommandSeq:        command.CommandSeq,
		CreatedAt:         now,
	}
	if channel == chatChannelRegion {
		record.RegionID = actorRegionID
	}
	senderMessage := chatMessagePayload(
		command.CommandID,
		command.CommandSeq,
		channel,
		actorCharacterID,
		actorName,
		targetCharacterID,
		targetCharacterName,
		record.RegionID,
		text,
		now,
	)
	recipientMessage := chatMessagePayload(
		"",
		0,
		channel,
		actorCharacterID,
		actorName,
		targetCharacterID,
		targetCharacterName,
		record.RegionID,
		text,
		now,
	)
	var remoteWhisperEvent *GameplayEvent
	if channel == chatChannelWhisper && whisperOwnership != nil {
		var buildErr error
		remoteWhisperEvent, buildErr = buildRemoteSocialDeliveryEvent(
			session,
			command,
			whisperOwnership,
			targetCharacterID,
			remoteChatMessageEventType,
			"chat-whisper",
			recipientMessage,
		)
		if buildErr != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to queue remote whisper delivery."))
		}
	}
	remoteRegionEvents := make([]*GameplayEvent, 0)
	if channel == chatChannelRegion {
		ownerships, ownershipErr := s.store.GameplaySessions.ListActiveOwnershipsByRegion(ctx, actorRegionID)
		if ownershipErr != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to resolve authoritative region recipients."))
		}
		for index := range ownerships {
			ownership := &ownerships[index]
			if ownership.CharacterID == "" || ownership.CharacterID == actorCharacterID || ownership.ServerInstanceID == "" || ownership.SessionID == "" {
				continue
			}
			if ownership.ServerInstanceID == s.config.ServerInstanceID {
				continue
			}
			event, buildErr := buildRemoteSocialDeliveryEvent(
				session,
				command,
				ownership,
				ownership.CharacterID,
				remoteChatMessageEventType,
				"chat-region",
				recipientMessage,
			)
			if buildErr != nil {
				continue
			}
			remoteRegionEvents = append(remoteRegionEvents, event)
		}
	}
	if err := collectChatMessage(ctx, record); err != nil {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to stage chat history."))
	}
	if remoteWhisperEvent != nil {
		if err := collectGameplayEvent(ctx, remoteWhisperEvent); err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to queue remote whisper delivery."))
		}
	}
	for _, event := range remoteRegionEvents {
		if err := collectGameplayEvent(ctx, event); err != nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to queue remote region chat delivery."))
		}
	}

	switch channel {
	case chatChannelRegion:
		localRecipients := s.readyRegionTargets(actorSessionID, actorRegionID)
		deferSocialSideEffect(ctx, func() {
			s.recordRegionChatEvent("region_chat_produced", nil, "")
		})
		for _, target := range localRecipients {
			if target == nil {
				continue
			}
			targetSessionID := target.sessionID
			targetCharacterID := target.characterID
			deferSocialSideEffect(ctx, func() {
				if s.deliverLocalRegionChat(targetSessionID, targetCharacterID, actorRegionID, recipientMessage) {
					s.recordRegionChatEvent("local_delivered", nil, "")
				}
			})
		}
	case chatChannelParty:
		notified := map[string]struct{}{
			actorCharacterID: {},
		}
		for _, memberCharacterID := range partyMemberIDs {
			if memberCharacterID == "" {
				continue
			}
			if _, exists := notified[memberCharacterID]; exists {
				continue
			}
			notified[memberCharacterID] = struct{}{}
			if target := s.attachedSessionByCharacterID(memberCharacterID); target != nil {
				deferSocialSideEffect(ctx, func() { _ = target.sendSerialized(recipientMessage) })
			}
		}
	case chatChannelWhisper:
		if whisperTarget != nil && whisperTarget.runtime != nil && whisperTarget.runtime.characterID != actorCharacterID {
			deferSocialSideEffect(ctx, func() { _ = whisperTarget.sendSerialized(recipientMessage) })
		}
	}

	return append(outbound, senderMessage)
}

func (s *Server) deliverLocalRegionChat(sessionID string, characterID string, regionID string, message map[string]any) bool {
	if s == nil || sessionID == "" || characterID == "" || regionID == "" || message == nil {
		return false
	}
	s.attachedMu.Lock()
	target := s.attached[sessionID]
	valid := target != nil && target.ready && target.runtime != nil && target.send != nil && target.characterID == characterID
	fencingToken := int64(0)
	targetInstanceID := ""
	if target != nil {
		fencingToken = target.fencingToken
		targetInstanceID = target.serverInstanceID
	}
	s.attachedMu.Unlock()
	if !valid || target.runtime.regionIDValue() != regionID {
		return false
	}
	if fencingToken != 0 {
		ownershipCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		ownership, err := s.store.GameplaySessions.GetActiveOwnershipByCharacterID(ownershipCtx, characterID)
		if err != nil || ownership.SessionID != sessionID || ownership.ServerInstanceID != targetInstanceID || ownership.FencingToken != fencingToken || ownership.RegionID != regionID {
			s.recordRegionChatEvent("stale_owner", nil, "social.recipient_stale_owner")
			return false
		}
	}
	return target.sendSerialized(message)
}

func (s *Server) recordRegionChatReplay(command commandEnvelope, outbound []map[string]any) {
	if command.Type != "send_chat_message" {
		return
	}
	for _, message := range outbound {
		if message["kind"] == chatMessageKind && message["channel"] == chatChannelRegion && message["command_id"] == command.CommandID {
			s.recordRegionChatEvent("duplicate", nil, "")
			return
		}
	}
}
