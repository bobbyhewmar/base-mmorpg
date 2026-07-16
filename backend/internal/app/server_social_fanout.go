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
	postCommit  []func()
}

type socialCommandTransactionContextKey struct{}

type socialCommandTransactionState struct {
	err        error
	postCommit []func()
}

func socialCommandTransactionFromContext(ctx context.Context) *socialCommandTransactionState {
	state, _ := ctx.Value(socialCommandTransactionContextKey{}).(*socialCommandTransactionState)
	return state
}

func deferSocialSideEffect(ctx context.Context, effect func()) bool {
	if effect == nil {
		return false
	}
	state := socialCommandTransactionFromContext(ctx)
	if state != nil {
		state.postCommit = append(state.postCommit, effect)
		return true
	}
	collector, _ := ctx.Value(gameplayEventCollectorContextKey{}).(*gameplayEventCollector)
	if collector != nil {
		collector.postCommit = append(collector.postCommit, effect)
		return true
	}
	return false
}

type remoteSocialDeliveryPayload struct {
	RecipientCharacterID  string         `json:"recipient_character_id"`
	RecipientFencingToken int64          `json:"recipient_fencing_token"`
	Message               map[string]any `json:"message"`
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
	if ownership.CharacterID != recipientCharacterID || ownership.ServerInstanceID == "" || ownership.SessionID == "" || ownership.FencingToken <= 0 {
		return nil, errors.New("invalid remote social recipient ownership")
	}
	purpose = strings.Trim(strings.ToLower(strings.Join(strings.Fields(purpose), "-")), "-")
	if purpose == "" {
		return nil, errors.New("remote social delivery purpose is required")
	}
	payload, err := json.Marshal(remoteSocialDeliveryPayload{
		RecipientCharacterID:  recipientCharacterID,
		RecipientFencingToken: ownership.FencingToken,
		Message:               message,
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
		if attached == nil {
			return errors.New("social.socket_delivery_failed")
		}
		if deferSocialSideEffect(ctx, func() { _ = attached.sendSerialized(message) }) {
			return nil
		}
		if !attached.sendSerialized(message) {
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
		if ownership == nil || ownership.CharacterID != recipientCharacterID || ownership.ServerInstanceID == "" || ownership.SessionID == "" || ownership.FencingToken <= 0 {
			return errors.New("social.recipient_stale_owner")
		}
		payload, marshalErr := json.Marshal(remoteSocialDeliveryPayload{
			RecipientCharacterID:  recipientCharacterID,
			RecipientFencingToken: ownership.FencingToken,
			Message:               message,
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

func isRegionChatGameplayEvent(event *GameplayEvent) bool {
	if event == nil || event.Type != remoteChatMessageEventType {
		return false
	}
	var payload remoteSocialDeliveryPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.Message == nil {
		return false
	}
	channel, _ := payload.Message["channel"].(string)
	return channel == chatChannelRegion
}

func decodeRemoteSocialDelivery(event *GameplayEvent, expectedKind string) (*remoteSocialDeliveryPayload, error) {
	if event == nil {
		return nil, errors.New("social.invalid_event")
	}
	var payload remoteSocialDeliveryPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return nil, errors.New("social.invalid_payload")
	}
	if payload.RecipientCharacterID == "" || payload.RecipientCharacterID != event.TargetCharacterID || payload.RecipientFencingToken <= 0 || payload.Message == nil {
		return nil, errors.New("social.invalid_payload")
	}
	kind, _ := payload.Message["kind"].(string)
	if kind != expectedKind {
		return nil, errors.New("social.invalid_payload")
	}
	return &payload, nil
}

func (s *Server) resolveRemoteSocialRecipient(ctx context.Context, event *GameplayEvent, recipientFencingToken int64) (*attachedSession, *SessionOwnership, error) {
	scope, attached, ownership, err := s.resolveCharacterPresence(ctx, event.TargetCharacterID)
	if err != nil {
		return nil, nil, errors.New("social.presence_unavailable")
	}
	if scope == characterPresenceOffline {
		return nil, nil, errors.New("social.recipient_offline")
	}
	if scope != characterPresenceLocal || attached == nil || ownership == nil {
		return nil, nil, errors.New("social.recipient_stale_owner")
	}
	if event.TargetSessionID == "" || recipientFencingToken <= 0 || attached.sessionID != event.TargetSessionID || ownership.SessionID != event.TargetSessionID || attached.fencingToken != recipientFencingToken || ownership.FencingToken != recipientFencingToken {
		return nil, nil, errors.New("social.recipient_stale_owner")
	}
	return attached, ownership, nil
}

func (s *Server) deliverRemoteChatMessage(ctx context.Context, event *GameplayEvent) error {
	payload, err := decodeRemoteSocialDelivery(event, chatMessageKind)
	if err != nil {
		return err
	}
	channel, _ := payload.Message["channel"].(string)
	text, _ := payload.Message["text"].(string)
	targetCharacterID, _ := payload.Message["target_character_id"].(string)
	regionID, _ := payload.Message["region_id"].(string)
	if text == "" || normalizeChatMessageText(text) != text || utf8.RuneCountInString(text) > chatMessageMaxLength {
		return errors.New("social.invalid_payload")
	}
	switch channel {
	case chatChannelWhisper:
		if targetCharacterID != event.TargetCharacterID || regionID != "" {
			return errors.New("social.invalid_payload")
		}
	case chatChannelRegion:
		if targetCharacterID != "" || regionID == "" || regionID != event.TargetRegionID {
			return errors.New("social.invalid_payload")
		}
	default:
		return errors.New("social.invalid_payload")
	}
	attached, ownership, err := s.resolveRemoteSocialRecipient(ctx, event, payload.RecipientFencingToken)
	if err != nil {
		return err
	}
	if channel == chatChannelRegion && (ownership.RegionID != regionID || attached.runtime == nil || attached.runtime.regionIDValue() != regionID) {
		return errors.New("social.recipient_stale_owner")
	}
	payload.Message["event_id"] = event.ID
	if !attached.sendSerialized(payload.Message) {
		return errors.New("social.socket_delivery_failed")
	}
	return nil
}

func (s *Server) deliverRemotePartyNotice(ctx context.Context, event *GameplayEvent) error {
	payload, err := decodeRemoteSocialDelivery(event, partyNoticeKind)
	if err != nil {
		return err
	}
	attached, _, err := s.resolveRemoteSocialRecipient(ctx, event, payload.RecipientFencingToken)
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
	return nil
}

func (s *Server) deliverRemoteClanNotice(ctx context.Context, event *GameplayEvent) error {
	payload, err := decodeRemoteSocialDelivery(event, clanNoticeKind)
	if err != nil {
		return err
	}
	attached, _, err := s.resolveRemoteSocialRecipient(ctx, event, payload.RecipientFencingToken)
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

func (s *Server) recordRegionChatEnqueue(event *GameplayEvent, outboxResult string) {
	if !isRegionChatGameplayEvent(event) {
		return
	}
	result := "remote_enqueued"
	if outboxResult == "duplicate" {
		result = "duplicate"
	}
	s.recordRegionChatEvent(result, event, "")
}

func (s *Server) recordRegionChatEvent(result string, event *GameplayEvent, reasonCode string) {
	if s == nil || s.observer == nil {
		return
	}
	if event != nil && !isRegionChatGameplayEvent(event) {
		return
	}
	s.observer.incCounter("l2bg_region_chat_events_total", "Total authoritative region chat lifecycle events.", map[string]string{
		"result": result,
	}, 1)
	fields := map[string]any{
		"result":             result,
		"server_instance_id": s.config.ServerInstanceID,
	}
	if event != nil {
		fields["event_id"] = event.ID
		fields["event_type"] = event.Type
		fields["target_server_instance_id"] = event.TargetServerInstanceID
		fields["target_region_id"] = event.TargetRegionID
		fields["retry_count"] = event.RetryCount
	}
	if reasonCode != "" {
		fields["reason_code"] = reasonCode
	}
	level := "info"
	if result == "stale_owner" || result == "dead_letter" {
		level = "error"
	}
	s.observer.log(level, "region_chat", fields)
}
