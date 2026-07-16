package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	remoteTargetNoticeEventType = "presence.remote_target_notice.v1"
	presenceNoticeKind          = "presence_notice"
	maxRememberedGameplayEvents = 4096
)

type remoteTargetNoticePayload struct {
	ActorCharacterID       string `json:"actor_character_id"`
	TargetCharacterID      string `json:"target_character_id"`
	SourceServerInstanceID string `json:"source_server_instance_id"`
	ReasonCode             string `json:"reason_code"`
}

func buildRemoteTargetNoticeEvent(session *Session, actorCharacterID string, targetCharacterID string, sourceServerInstanceID string, ownership *SessionOwnership, command commandEnvelope) (*GameplayEvent, error) {
	if session == nil || ownership == nil || actorCharacterID == "" || targetCharacterID == "" || command.CommandSeq <= 0 {
		return nil, errors.New("incomplete remote target notice")
	}
	payload, err := json.Marshal(remoteTargetNoticePayload{
		ActorCharacterID:       actorCharacterID,
		TargetCharacterID:      targetCharacterID,
		SourceServerInstanceID: sourceServerInstanceID,
		ReasonCode:             "presence.target_remote",
	})
	if err != nil {
		return nil, err
	}
	return &GameplayEvent{
		IdempotencyKey:         fmt.Sprintf("gameplay-command/%s/%d/remote-target-notice", session.ID, command.CommandSeq),
		Type:                   remoteTargetNoticeEventType,
		Payload:                payload,
		TargetServerInstanceID: ownership.ServerInstanceID,
		TargetRegionID:         ownership.RegionID,
		TargetSessionID:        ownership.SessionID,
		TargetCharacterID:      targetCharacterID,
	}, nil
}

func (s *Server) startGameplayEventDispatcher(ctx context.Context) {
	if s == nil || s.store == nil || s.store.GameplayEvents == nil || s.gameplayEventWorkerID == "" {
		return
	}
	go func() {
		s.dispatchGameplayEventsOnce(ctx, s.gameplayEventWorkerID)
		pollTicker := time.NewTicker(s.config.GameplayEventPollInterval)
		cleanupTicker := time.NewTicker(s.config.GameplayEventCleanupInterval)
		defer pollTicker.Stop()
		defer cleanupTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-pollTicker.C:
				s.dispatchGameplayEventsOnce(ctx, s.gameplayEventWorkerID)
			case <-cleanupTicker.C:
				s.cleanupDeliveredGameplayEvents(ctx, time.Now())
			}
		}
	}()
}

func (s *Server) dispatchGameplayEventsOnce(ctx context.Context, workerID string) int {
	if s == nil || s.store == nil || s.store.GameplayEvents == nil {
		return 0
	}
	events, err := s.store.GameplayEvents.Claim(
		ctx,
		s.config.ServerInstanceID,
		workerID,
		time.Now(),
		s.config.GameplayEventClaimLease,
		s.config.GameplayEventBatchSize,
	)
	if err != nil {
		s.recordStoreError("gameplay_events.claim", err)
		return 0
	}
	for index := range events {
		event := &events[index]
		s.recordGameplayEvent("claimed", event, "")
		if deliveryErr := s.deliverGameplayEvent(ctx, event); deliveryErr != nil {
			s.failGameplayEvent(ctx, workerID, event, deliveryErr)
			continue
		}
		delivered, markErr := s.store.GameplayEvents.MarkDelivered(ctx, event.ID, workerID, time.Now())
		if markErr != nil {
			s.recordStoreError("gameplay_events.mark_delivered", markErr)
			s.recordGameplayEvent("failed", event, "mark_delivered_failed")
			continue
		}
		if !delivered {
			s.recordGameplayEvent("failed", event, "stale_claim")
			continue
		}
		s.recordGameplayEvent("delivered", event, "")
		s.recordSocialFanoutEvent("delivered", event, "")
	}
	return len(events)
}

func (s *Server) deliverGameplayEvent(ctx context.Context, event *GameplayEvent) error {
	if event == nil || event.ID <= 0 {
		return errors.New("invalid_event")
	}
	if event.TargetServerInstanceID != s.config.ServerInstanceID {
		return errors.New("wrong_target_instance")
	}
	if s.gameplayEventWasSeen(event.ID) {
		s.recordSocialFanoutEvent("duplicate", event, "")
		return nil
	}
	switch event.Type {
	case remoteTargetNoticeEventType:
		return s.deliverRemoteTargetNotice(ctx, event)
	case remoteChatMessageEventType:
		return s.deliverRemoteChatMessage(ctx, event)
	case remotePartyNoticeEventType:
		return s.deliverRemotePartyNotice(ctx, event)
	case remoteClanNoticeEventType:
		return s.deliverRemoteClanNotice(ctx, event)
	default:
		return errors.New("unsupported_event_type")
	}
}

func (s *Server) deliverRemoteTargetNotice(ctx context.Context, event *GameplayEvent) error {
	var payload remoteTargetNoticePayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return errors.New("invalid_payload")
	}
	if payload.ActorCharacterID == "" || payload.TargetCharacterID == "" || payload.TargetCharacterID != event.TargetCharacterID || payload.ReasonCode != "presence.target_remote" {
		return errors.New("invalid_payload")
	}
	scope, attached, ownership, err := s.resolveCharacterPresence(ctx, event.TargetCharacterID)
	if err != nil {
		return errors.New("presence_unavailable")
	}
	if scope != characterPresenceLocal || attached == nil || ownership == nil {
		return errors.New("target_not_local")
	}
	if event.TargetSessionID != "" && (attached.sessionID != event.TargetSessionID || ownership.SessionID != event.TargetSessionID) {
		return errors.New("target_session_changed")
	}
	notice := map[string]any{
		"kind":               presenceNoticeKind,
		"event_id":           event.ID,
		"notice_type":        "remote_target_attempt",
		"actor_character_id": payload.ActorCharacterID,
		"reason_code":        payload.ReasonCode,
		"occurred_at_ms":     event.CreatedAt.UnixMilli(),
		"emitted_at_ms":      time.Now().UnixMilli(),
	}
	if !attached.sendSerialized(notice) {
		return errors.New("socket_delivery_failed")
	}
	s.rememberGameplayEvent(event.ID)
	return nil
}

func (s *Server) failGameplayEvent(ctx context.Context, workerID string, event *GameplayEvent, deliveryErr error) {
	if event == nil {
		return
	}
	failureCode := summarizeGameplayEventError(deliveryErr.Error())
	delay := s.config.GameplayEventRetryDelay
	for attempt := 0; attempt < event.RetryCount && attempt < 5; attempt++ {
		delay *= 2
	}
	failure, err := s.store.GameplayEvents.MarkFailed(
		ctx,
		event.ID,
		workerID,
		time.Now(),
		delay,
		s.config.GameplayEventMaxRetries,
		failureCode,
	)
	if err != nil {
		s.recordStoreError("gameplay_events.mark_failed", err, errOwnershipStale)
		return
	}
	event.RetryCount = failure.RetryCount
	s.recordGameplayEvent("failed", event, failureCode)
	if failureCode == "social.recipient_stale_owner" {
		s.recordSocialFanoutEvent("stale_owner", event, failureCode)
	} else {
		s.recordSocialFanoutEvent("failed", event, failureCode)
	}
	if failure.DeadLettered {
		s.recordGameplayEvent("dead_lettered", event, failureCode)
		s.recordSocialFanoutEvent("dead_letter", event, failureCode)
		return
	}
	s.recordGameplayEvent("retried", event, failureCode)
}

func (s *Server) cleanupDeliveredGameplayEvents(ctx context.Context, now time.Time) int {
	if s == nil || s.store == nil || s.store.GameplayEvents == nil {
		return 0
	}
	deleted, err := s.store.GameplayEvents.DeleteDeliveredBefore(ctx, now.Add(-s.config.GameplayEventRetention), s.config.GameplayEventBatchSize*8)
	if err != nil {
		s.recordStoreError("gameplay_events.retention", err)
		return 0
	}
	if deleted > 0 {
		s.observer.incCounter("l2bg_gameplay_outbox_events_total", "Total durable gameplay outbox events by lifecycle result.", map[string]string{"result": "expired"}, float64(deleted))
		s.observer.log("info", "gameplay_outbox_retention", map[string]any{
			"result":             "expired",
			"server_instance_id": s.config.ServerInstanceID,
			"event_count":        deleted,
		})
	}
	return deleted
}

func (s *Server) gameplayEventWasSeen(eventID int64) bool {
	if s == nil || eventID <= 0 {
		return false
	}
	s.gameplayEventMu.Lock()
	defer s.gameplayEventMu.Unlock()
	_, exists := s.gameplayEventSeen[eventID]
	return exists
}

func (s *Server) rememberGameplayEvent(eventID int64) {
	if s == nil || eventID <= 0 {
		return
	}
	s.gameplayEventMu.Lock()
	defer s.gameplayEventMu.Unlock()
	if _, exists := s.gameplayEventSeen[eventID]; exists {
		return
	}
	s.gameplayEventSeen[eventID] = struct{}{}
	s.gameplayEventOrder = append(s.gameplayEventOrder, eventID)
	if len(s.gameplayEventOrder) <= maxRememberedGameplayEvents {
		return
	}
	oldest := s.gameplayEventOrder[0]
	s.gameplayEventOrder = s.gameplayEventOrder[1:]
	delete(s.gameplayEventSeen, oldest)
}

func (s *Server) recordGameplayEvent(result string, event *GameplayEvent, failureCode string) {
	if s == nil || s.observer == nil {
		return
	}
	s.observer.incCounter("l2bg_gameplay_outbox_events_total", "Total durable gameplay outbox events by lifecycle result.", map[string]string{
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
		fields["retry_count"] = event.RetryCount
	}
	if failureCode != "" {
		fields["failure_code"] = failureCode
	}
	level := "info"
	if result == "failed" || result == "dead_lettered" {
		level = "error"
	}
	s.observer.log(level, "gameplay_outbox", fields)
}
