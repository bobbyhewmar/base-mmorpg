package app

import (
	"context"
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
	if channel == chatChannelWhisper {
		targetCharacterName = strings.TrimSpace(parsed.chatTargetName)
		if targetCharacterName == "" {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "chat.whisper_target_required", "Whisper requires a target character name."))
		}
		whisperTarget = s.attachedSessionByCharacterName(targetCharacterName)
		if whisperTarget == nil || whisperTarget.runtime == nil {
			return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "chat.whisper_target_not_found", "Whisper target is not currently online."))
		}
		targetCharacterID = whisperTarget.runtime.characterID
		targetCharacterName = whisperTarget.runtime.characterName
	}

	record := ChatMessageRecord{
		ID:                randomID("chat"),
		CharacterID:       actorCharacterID,
		Channel:           channel,
		TargetCharacterID: targetCharacterID,
		Text:              text,
		CreatedAt:         now,
	}
	if channel == chatChannelRegion {
		record.RegionID = actorRegionID
	}
	if err := s.store.ChatMessages.Create(ctx, record); err != nil {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to persist chat history."))
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

	switch channel {
	case chatChannelRegion:
		for _, target := range s.readyRegionTargets(actorSessionID, actorRegionID) {
			if target == nil {
				continue
			}
			target.sendSerialized(recipientMessage)
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
				target.sendSerialized(recipientMessage)
			}
		}
	case chatChannelWhisper:
		if whisperTarget != nil && whisperTarget.runtime != nil && whisperTarget.runtime.characterID != actorCharacterID {
			whisperTarget.sendSerialized(recipientMessage)
		}
	}

	return append(outbound, senderMessage)
}
