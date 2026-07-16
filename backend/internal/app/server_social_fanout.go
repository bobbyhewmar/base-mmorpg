package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	remoteChatMessageEventType = "social.chat_message.v1"
	remotePartyNoticeEventType = "social.party_notice.v1"
	remoteClanNoticeEventType  = "social.clan_notice.v1"
)

type gameplayEventCollectorContextKey struct{}

type gameplayEventCollector struct {
	events      []*GameplayEvent
	chatMessage *ChatMessageRecord
}

type remoteSocialDeliveryPayload struct {
	RecipientCharacterID string         `json:"recipient_character_id"`
	Message              map[string]any `json:"message"`
}

func withGameplayEventCollector(ctx context.Context, collector *gameplayEventCollector) context.Context {
	if collector == nil {
		return ctx
	}
	return context.WithValue(ctx, gameplayEventCollectorContextKey{}, collector)
}

func collectGameplayEvent(ctx context.Context, event *GameplayEvent) error {
	collector, _ := ctx.Value(gameplayEventCollectorContextKey{}).(*gameplayEventCollector)
	if collector == nil || event == nil {
		return errors.New("gameplay event collector is unavailable")
	}
	collector.events = append(collector.events, event)
	return nil
}

func collectChatMessage(ctx context.Context, record ChatMessageRecord) error {
	collector, _ := ctx.Value(gameplayEventCollectorContextKey{}).(*gameplayEventCollector)
	if collector == nil {
		return errors.New("gameplay event collector is unavailable")
	}
	clone := record
	collector.chatMessage = &clone
	return nil
}

func buildRemoteSocialDeliveryEvent(
	session *Session,
	command commandEnvelope,
	ownership *SessionOwnership,
	recipientCharacterID string,
	eventType string,
	purpose string,
	message map[string]any,
) (*GameplayEvent, error) {
	if session == nil || ownership == nil || command.CommandSeq <= 0 || recipientCharacterID == "" || message == nil {
		return nil, errors.New("incomplete remote social delivery")
	}
	if ownership.CharacterID != recipientCharacterID || ownership.ServerInstanceID == "" || ownership.SessionID == "" {
		return nil, errors.New("invalid remote social recipient ownership")
	}
	purpose = strings.Trim(strings.ToLower(strings.Join(strings.Fields(purpose), "-")), "-")
	if purpose == "" {
		return nil, errors.New("remote social delivery purpose is required")
	}
	payload, err := json.Marshal(remoteSocialDeliveryPayload{
		RecipientCharacterID: recipientCharacterID,
		Message:              message,
	})
	if err != nil {
		return nil, err
	}
	return &GameplayEvent{
		IdempotencyKey:         fmt.Sprintf("gameplay-command/%s/%d/social/%s/%s", session.ID, command.CommandSeq, purpose, recipientCharacterID),
		Type:                   eventType,
		Payload:                payload,
		TargetServerInstanceID: ownership.ServerInstanceID,
		TargetRegionID:         ownership.RegionID,
		TargetSessionID:        ownership.SessionID,
		TargetCharacterID:      recipientCharacterID,
	}, nil
}

func (s *Server) collectRemoteSocialDelivery(
	ctx context.Context,
	session *Session,
	command commandEnvelope,
	ownership *SessionOwnership,
	recipientCharacterID string,
	eventType string,
	purpose string,
	message map[string]any,
) error {
	event, err := buildRemoteSocialDeliveryEvent(session, command, ownership, recipientCharacterID, eventType, purpose, message)
	if err != nil {
		return err
	}
	return collectGameplayEvent(ctx, event)
}

func (s *Server) sendOrCollectSocialMessage(
	ctx context.Context,
	session *Session,
	command commandEnvelope,
	recipientCharacterID string,
	eventType string,
	purpose string,
	message map[string]any,
) error {
	scope, attached, ownership, err := s.resolveCharacterPresence(ctx, recipientCharacterID)
	if err != nil {
		return err
	}
	switch scope {
	case characterPresenceLocal:
		if attached == nil || !attached.sendSerialized(message) {
			return errors.New("social.socket_delivery_failed")
		}
	case characterPresenceRemote:
		return s.collectRemoteSocialDelivery(ctx, session, command, ownership, recipientCharacterID, eventType, purpose, message)
	}
	return nil
}

func (s *Server) sendOrProduceLifecycleSocialMessage(
	ctx context.Context,
	recipientCharacterID string,
	eventType string,
	idempotencyKey string,
	message map[string]any,
) error {
	if s == nil || s.store == nil || s.store.GameplayEvents == nil || recipientCharacterID == "" || strings.TrimSpace(idempotencyKey) == "" || message == nil {
		return errors.New("social.lifecycle_delivery_unavailable")
	}
	scope, attached, ownership, err := s.resolveCharacterPresence(ctx, recipientCharacterID)
	if err != nil {
		return err
	}
	switch scope {
	case characterPresenceLocal:
		if attached == nil || !attached.sendSerialized(message) {
			return errors.New("social.socket_delivery_failed")
		}
		return nil
	case characterPresenceRemote:
		if ownership == nil || ownership.CharacterID != recipientCharacterID || ownership.ServerInstanceID == "" || ownership.SessionID == "" {
			return errors.New("social.recipient_stale_owner")
		}
		payload, marshalErr := json.Marshal(remoteSocialDeliveryPayload{
			RecipientCharacterID: recipientCharacterID,
			Message:              message,
		})
		if marshalErr != nil {
			return marshalErr
		}
		event := &GameplayEvent{
			IdempotencyKey:         idempotencyKey,
			Type:                   eventType,
			Payload:                payload,
			TargetServerInstanceID: ownership.ServerInstanceID,
			TargetRegionID:         ownership.RegionID,
			TargetSessionID:        ownership.SessionID,
			TargetCharacterID:      recipientCharacterID,
		}
		created, createErr := s.store.GameplayEvents.Create(ctx, event)
		if createErr != nil {
			return createErr
		}
		result := "duplicate"
		if created {
			result = "produced"
		}
		s.recordGameplayEvent(result, event, "")
		s.recordSocialFanoutEvent(result, event, "")
	}
	return nil
}

func socialEventCategory(eventType string) string {
	switch eventType {
	case remoteChatMessageEventType:
		return "chat"
	case remotePartyNoticeEventType:
		return "party_notice"
	case remoteClanNoticeEventType:
		return "clan_notice"
	default:
		return ""
	}
}

func isSocialGameplayEvent(event *GameplayEvent) bool {
	return event != nil && socialEventCategory(event.Type) != ""
}

func decodeRemoteSocialDelivery(event *GameplayEvent, expectedKind string) (*remoteSocialDeliveryPayload, error) {
	if event == nil {
		return nil, errors.New("social.invalid_event")
	}
	var payload remoteSocialDeliveryPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return nil, errors.New("social.invalid_payload")
	}
	if payload.RecipientCharacterID == "" || payload.RecipientCharacterID != event.TargetCharacterID || payload.Message == nil {
		return nil, errors.New("social.invalid_payload")
	}
	kind, _ := payload.Message["kind"].(string)
	if kind != expectedKind {
		return nil, errors.New("social.invalid_payload")
	}
	return &payload, nil
}

func (s *Server) resolveRemoteSocialRecipient(ctx context.Context, event *GameplayEvent) (*attachedSession, error) {
	scope, attached, ownership, err := s.resolveCharacterPresence(ctx, event.TargetCharacterID)
	if err != nil {
		return nil, errors.New("social.presence_unavailable")
	}
	if scope == characterPresenceOffline {
		return nil, errors.New("social.recipient_offline")
	}
	if scope != characterPresenceLocal || attached == nil || ownership == nil {
		return nil, errors.New("social.recipient_stale_owner")
	}
	if event.TargetSessionID == "" || attached.sessionID != event.TargetSessionID || ownership.SessionID != event.TargetSessionID {
		return nil, errors.New("social.recipient_stale_owner")
	}
	return attached, nil
}

func (s *Server) deliverRemoteChatMessage(ctx context.Context, event *GameplayEvent) error {
	payload, err := decodeRemoteSocialDelivery(event, chatMessageKind)
	if err != nil {
		return err
	}
	channel, _ := payload.Message["channel"].(string)
	text, _ := payload.Message["text"].(string)
	targetCharacterID, _ := payload.Message["target_character_id"].(string)
	if channel != chatChannelWhisper || targetCharacterID != event.TargetCharacterID || text == "" || normalizeChatMessageText(text) != text || utf8.RuneCountInString(text) > chatMessageMaxLength {
		return errors.New("social.invalid_payload")
	}
	attached, err := s.resolveRemoteSocialRecipient(ctx, event)
	if err != nil {
		return err
	}
	payload.Message["event_id"] = event.ID
	if !attached.sendSerialized(payload.Message) {
		return errors.New("social.socket_delivery_failed")
	}
	s.rememberGameplayEvent(event.ID)
	return nil
}

func (s *Server) deliverRemotePartyNotice(ctx context.Context, event *GameplayEvent) error {
	payload, err := decodeRemoteSocialDelivery(event, partyNoticeKind)
	if err != nil {
		return err
	}
	attached, err := s.resolveRemoteSocialRecipient(ctx, event)
	if err != nil {
		return err
	}
	party, invites, err := s.loadCharacterPartyState(ctx, event.TargetCharacterID, timeNowUTC())
	if err != nil {
		return errors.New("social.state_hydration_failed")
	}
	payload.Message["event_id"] = event.ID
	if !attached.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
		return []map[string]any{runtime.partyDeltaMessage(party, invites), payload.Message}
	}) {
		return errors.New("social.socket_delivery_failed")
	}
	s.rememberGameplayEvent(event.ID)
	return nil
}

func (s *Server) deliverRemoteClanNotice(ctx context.Context, event *GameplayEvent) error {
	payload, err := decodeRemoteSocialDelivery(event, clanNoticeKind)
	if err != nil {
		return err
	}
	attached, err := s.resolveRemoteSocialRecipient(ctx, event)
	if err != nil {
		return err
	}
	clan, invites, err := s.loadCharacterClanState(ctx, event.TargetCharacterID, timeNowUTC())
	if err != nil {
		return errors.New("social.state_hydration_failed")
	}
	payload.Message["event_id"] = event.ID
	if !attached.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
		return []map[string]any{runtime.clanDeltaMessage(clan, invites), payload.Message}
	}) {
		return errors.New("social.socket_delivery_failed")
	}
	s.rememberGameplayEvent(event.ID)
	return nil
}

func timeNowUTC() time.Time {
	return time.Now().UTC()
}

func (s *Server) recordSocialFanoutEvent(result string, event *GameplayEvent, reasonCode string) {
	if s == nil || s.observer == nil || !isSocialGameplayEvent(event) {
		return
	}
	category := socialEventCategory(event.Type)
	s.observer.incCounter("l2bg_social_fanout_events_total", "Total cross-instance social deliveries by category and lifecycle result.", map[string]string{
		"category": category,
		"result":   result,
	}, 1)
	fields := map[string]any{
		"result":                    result,
		"category":                  category,
		"event_id":                  event.ID,
		"event_type":                event.Type,
		"server_instance_id":        s.config.ServerInstanceID,
		"target_server_instance_id": event.TargetServerInstanceID,
		"retry_count":               event.RetryCount,
	}
	if reasonCode != "" {
		fields["reason_code"] = reasonCode
	}
	level := "info"
	if result == "stale_owner" || result == "dead_letter" || result == "failed" {
		level = "error"
	}
	s.observer.log(level, "social_fanout", fields)
}
