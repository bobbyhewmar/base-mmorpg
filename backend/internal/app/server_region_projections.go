package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"
)

const (
	regionPlayerProjectionEventType = "presence.region_player_projection.v1"
	regionProjectionActionUpsert    = "upsert"
	regionProjectionActionDespawn   = "despawn"
	regionProjectionDisappear       = "remote_projection_despawn"
	regionProjectionExpired         = "remote_projection_expired"
	regionProjectionSuperseded      = "projection_row_superseded"
	regionProjectionStaleSkipped    = "stale_delivery_skipped"
	regionProjectionCompacted       = "compacted_obsolete"
	regionProjectionOutOfInterest   = "out_of_interest"
)

type regionPlayerProjectionPayload struct {
	Action                 string        `json:"action"`
	CharacterID            string        `json:"character_id"`
	DisplayName            string        `json:"display_name,omitempty"`
	RegionID               string        `json:"region_id"`
	Position               runtimePoint  `json:"position"`
	Facing                 float64       `json:"facing"`
	Moving                 bool          `json:"moving"`
	MovementDestination    *runtimePoint `json:"movement_destination,omitempty"`
	VisualTargetID         string        `json:"visual_target_id,omitempty"`
	Race                   string        `json:"race,omitempty"`
	BaseClass              string        `json:"base_class,omitempty"`
	Sex                    string        `json:"sex,omitempty"`
	HairStyle              int           `json:"hair_style,omitempty"`
	HairColor              string        `json:"hair_color,omitempty"`
	SkinType               int           `json:"skin_type,omitempty"`
	SourceSessionID        string        `json:"source_session_id"`
	SourceServerInstanceID string        `json:"source_server_instance_id"`
	FencingToken           int64         `json:"fencing_token"`
	Version                int64         `json:"version"`
	RecipientFencingToken  int64         `json:"recipient_fencing_token"`
}

type regionProjectionPublishRequest struct {
	Payload regionPlayerProjectionPayload
}

type remotePlayerProjectionMeta struct {
	FencingToken int64
	Version      int64
	LastSeenAt   time.Time
	Visible      bool
}

type regionProjectionApplyResult string

const (
	regionProjectionApplied   regionProjectionApplyResult = "projection_consumed"
	regionProjectionDespawned regionProjectionApplyResult = "despawned"
	regionProjectionStale     regionProjectionApplyResult = "stale_ignored"
	regionProjectionDuplicate regionProjectionApplyResult = "duplicate"
)

func (runtime *attachedRuntime) regionProjectionPayload() regionPlayerProjectionPayload {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	payload := regionPlayerProjectionPayload{
		CharacterID:    runtime.characterID,
		DisplayName:    runtime.characterName,
		RegionID:       runtime.regionID,
		Position:       runtime.position,
		Facing:         runtime.facing,
		Moving:         runtime.activeMovement != nil,
		VisualTargetID: runtime.targetID,
		Race:           runtime.characterRace,
		BaseClass:      runtime.characterBaseClass,
		Sex:            runtime.characterSex,
		HairStyle:      runtime.characterHairStyle,
		HairColor:      runtime.characterHairColor,
		SkinType:       runtime.characterSkinType,
	}
	if runtime.activeMovement != nil {
		destination := runtime.activeMovement.AcceptedDestination
		payload.MovementDestination = &destination
	}
	return payload
}

func (attached *attachedSession) nextRegionProjectionRequests(action string, now time.Time) []regionProjectionPublishRequest {
	if attached == nil || attached.runtime == nil || attached.fencingToken <= 0 || attached.serverInstanceID == "" {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	snapshot := attached.runtime.regionProjectionPayload()
	if snapshot.CharacterID == "" || snapshot.RegionID == "" {
		return nil
	}

	attached.projectionMu.Lock()
	defer attached.projectionMu.Unlock()

	base := snapshot
	base.SourceSessionID = attached.sessionID
	base.SourceServerInstanceID = attached.serverInstanceID
	base.FencingToken = attached.fencingToken
	requests := make([]regionProjectionPublishRequest, 0, 2)
	if action == regionProjectionActionUpsert && attached.projectionRegionID != "" && attached.projectionRegionID != snapshot.RegionID {
		attached.projectionVersion++
		despawn := base
		despawn.Action = regionProjectionActionDespawn
		despawn.RegionID = attached.projectionRegionID
		despawn.Version = attached.projectionVersion
		requests = append(requests, regionProjectionPublishRequest{Payload: despawn})
	}
	attached.projectionVersion++
	base.Action = action
	base.Version = attached.projectionVersion
	if action == regionProjectionActionDespawn && attached.projectionRegionID != "" {
		base.RegionID = attached.projectionRegionID
	}
	requests = append(requests, regionProjectionPublishRequest{Payload: base})
	attached.projectionPublishedAt = now
	if action == regionProjectionActionDespawn {
		attached.projectionRegionID = ""
	} else {
		attached.projectionRegionID = snapshot.RegionID
	}
	return requests
}

func (attached *attachedSession) regionProjectionHeartbeatDue(now time.Time, interval time.Duration) bool {
	if attached == nil || interval <= 0 {
		return false
	}
	attached.projectionMu.Lock()
	defer attached.projectionMu.Unlock()
	return attached.projectionPublishedAt.IsZero() || !now.Before(attached.projectionPublishedAt.Add(interval))
}

func (attached *attachedSession) ownershipAnchorRefreshDue(now time.Time, interval time.Duration) bool {
	if attached == nil || interval <= 0 {
		return false
	}
	attached.projectionMu.Lock()
	defer attached.projectionMu.Unlock()
	return attached.ownershipAnchorAt.IsZero() || !now.Before(attached.ownershipAnchorAt.Add(interval))
}

func (attached *attachedSession) markOwnershipAnchorRefreshed(now time.Time) {
	if attached == nil {
		return
	}
	attached.projectionMu.Lock()
	defer attached.projectionMu.Unlock()
	attached.ownershipAnchorAt = now
}

func (attached *attachedSession) regionProjectionRegion() string {
	if attached == nil {
		return ""
	}
	attached.projectionMu.Lock()
	defer attached.projectionMu.Unlock()
	return attached.projectionRegionID
}

func (s *Server) enqueueRegionPlayerProjection(attached *attachedSession, action string, now time.Time) {
	if s == nil || attached == nil || s.regionProjectionQueue == nil {
		return
	}
	for _, request := range attached.nextRegionProjectionRequests(action, now) {
		s.enqueueRegionProjectionRequest(request)
	}
}

func (s *Server) enqueueRegionProjectionRequest(request regionProjectionPublishRequest) {
	if s == nil || s.regionProjectionQueue == nil {
		return
	}
	if s.coalescePendingRegionProjection(request) {
		s.recordRegionProjectionQueueEvent("coalesced", &request.Payload)
		return
	}
	select {
	case s.regionProjectionQueue <- request:
		s.recordRegionProjectionQueueEvent("enqueued", &request.Payload)
	default:
		if s.coalesceRegionProjection(request) {
			s.recordRegionProjectionQueueEvent("coalesced", &request.Payload)
		} else {
			s.recordRegionProjectionQueueEvent("dropped", &request.Payload)
		}
	}
}

func (s *Server) coalescePendingRegionProjection(request regionProjectionPublishRequest) bool {
	if s == nil {
		return false
	}
	key := regionProjectionPressureKey(request.Payload)
	s.regionProjectionMu.Lock()
	defer s.regionProjectionMu.Unlock()
	existing, exists := s.regionProjectionSpill[key]
	if !exists {
		return false
	}
	if request.Payload.Version > existing.Payload.Version {
		s.regionProjectionSpill[key] = request
	}
	return true
}

func (s *Server) startRegionProjectionPublisher(ctx context.Context) {
	if s == nil || s.regionProjectionQueue == nil {
		return
	}
	go func() {
		drainTicker := time.NewTicker(10 * time.Millisecond)
		defer drainTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case request := <-s.regionProjectionQueue:
				s.recordRegionProjectionQueueEvent("dequeued", &request.Payload)
				s.publishRegionProjectionRequest(ctx, request)
			case <-drainTicker.C:
				if request, ok := s.takeCoalescedRegionProjection(); ok {
					s.recordRegionProjectionQueueEvent("dequeued", &request.Payload)
					s.publishRegionProjectionRequest(ctx, request)
				}
			}
		}
	}()
}

func regionProjectionPressureKey(payload regionPlayerProjectionPayload) string {
	return fmt.Sprintf("%s/%d/%s", payload.CharacterID, payload.FencingToken, payload.RegionID)
}

func (s *Server) coalesceRegionProjection(request regionProjectionPublishRequest) bool {
	if s == nil {
		return false
	}
	key := regionProjectionPressureKey(request.Payload)
	s.regionProjectionMu.Lock()
	defer s.regionProjectionMu.Unlock()
	if existing, exists := s.regionProjectionSpill[key]; exists {
		if request.Payload.Version > existing.Payload.Version {
			s.regionProjectionSpill[key] = request
		}
		return true
	}
	if len(s.regionProjectionSpill) >= s.regionProjectionLimit {
		return false
	}
	s.regionProjectionSpill[key] = request
	return true
}

func (s *Server) takeCoalescedRegionProjection() (regionProjectionPublishRequest, bool) {
	if s == nil {
		return regionProjectionPublishRequest{}, false
	}
	s.regionProjectionMu.Lock()
	defer s.regionProjectionMu.Unlock()
	for key, request := range s.regionProjectionSpill {
		delete(s.regionProjectionSpill, key)
		return request, true
	}
	return regionProjectionPublishRequest{}, false
}

func (s *Server) recordRegionProjectionQueueState() {
	if s == nil || s.observer == nil || s.regionProjectionQueue == nil {
		return
	}
	s.regionProjectionMu.Lock()
	spillDepth := len(s.regionProjectionSpill)
	s.regionProjectionMu.Unlock()
	s.observer.setGauge("l2bg_region_projection_queue_depth", "Current in-process regional projection publication queue depth.", nil, float64(len(s.regionProjectionQueue)))
	s.observer.setGauge("l2bg_region_projection_queue_capacity", "Configured in-process regional projection publication queue capacity.", nil, float64(cap(s.regionProjectionQueue)))
	s.observer.setGauge("l2bg_region_projection_queue_coalesced_depth", "Current bounded regional projection coalescing spill depth.", nil, float64(spillDepth))
}

func (s *Server) recordRegionProjectionQueueEvent(result string, payload *regionPlayerProjectionPayload) {
	if s == nil || s.observer == nil {
		return
	}
	s.observer.incCounter("l2bg_region_projection_queue_events_total", "Total regional projection publication queue events by result.", map[string]string{"result": result}, 1)
	s.recordRegionProjectionQueueState()
	if result != "coalesced" && result != "dropped" {
		return
	}
	s.recordRegionProjectionEvent("projection_queue_pressure", nil, payload, "publisher_queue_full")
	s.observer.log("warn", "region_projection_queue_pressure", map[string]any{
		"result":             result,
		"server_instance_id": s.config.ServerInstanceID,
	})
}

func (s *Server) publishAttachedRegionProjectionNow(ctx context.Context, attached *attachedSession, action string, now time.Time) int {
	produced := 0
	for _, request := range attached.nextRegionProjectionRequests(action, now) {
		produced += s.publishRegionProjectionRequest(ctx, request)
	}
	return produced
}

func (s *Server) publishRegionProjectionRequest(ctx context.Context, request regionProjectionPublishRequest) int {
	if s == nil || s.store == nil || s.store.GameplayEvents == nil || s.store.GameplaySessions == nil {
		return 0
	}
	payload := request.Payload
	if err := validateRegionPlayerProjectionPayload(payload); err != nil {
		s.recordRegionProjectionEvent("failed", nil, &payload, "invalid_payload")
		return 0
	}
	ownerships, err := s.store.GameplaySessions.ListActiveOwnershipsByRegion(ctx, payload.RegionID)
	if err != nil {
		s.recordStoreError("region_projection.list_ownerships", err)
		s.recordRegionProjectionEvent("failed", nil, &payload, "presence_unavailable")
		return 0
	}
	produced := 0
	for index := range ownerships {
		recipient := &ownerships[index]
		if recipient.CharacterID == "" || recipient.SessionID == "" || recipient.ServerInstanceID == "" || recipient.FencingToken <= 0 {
			continue
		}
		if recipient.CharacterID == payload.CharacterID || recipient.ServerInstanceID == s.config.ServerInstanceID {
			continue
		}
		if !s.regionProjectionRecipientEligible(payload, recipient) {
			s.recordRegionProjectionEvent(regionProjectionOutOfInterest, nil, &payload, "")
			continue
		}
		recipientPayload := payload
		recipientPayload.RecipientFencingToken = recipient.FencingToken
		encoded, marshalErr := json.Marshal(recipientPayload)
		if marshalErr != nil {
			s.recordRegionProjectionEvent("failed", nil, &recipientPayload, "invalid_payload")
			continue
		}
		event := &GameplayEvent{
			IdempotencyKey: fmt.Sprintf(
				"region-player-projection/%s/%d/%d/%s/%d",
				payload.CharacterID,
				payload.FencingToken,
				payload.Version,
				recipient.CharacterID,
				recipient.FencingToken,
			),
			Type:                            regionPlayerProjectionEventType,
			Payload:                         encoded,
			TargetServerInstanceID:          recipient.ServerInstanceID,
			TargetRegionID:                  payload.RegionID,
			TargetSessionID:                 recipient.SessionID,
			TargetCharacterID:               recipient.CharacterID,
			ProjectionSourceCharacterID:     payload.CharacterID,
			ProjectionSourceFencingToken:    payload.FencingToken,
			ProjectionVersion:               payload.Version,
			ProjectionRecipientFencingToken: recipient.FencingToken,
			ProjectionAction:                payload.Action,
		}
		created, createErr := s.store.GameplayEvents.Create(ctx, event)
		if createErr != nil {
			s.recordStoreError("region_projection.create", createErr, errRecordConflict)
			s.recordRegionProjectionEvent("failed", event, &payload, "outbox_create_failed")
			continue
		}
		result := "duplicate"
		if created {
			result = "projection_produced"
			produced++
			supersededCount, supersedeErr := s.store.GameplayEvents.SupersedeRegionProjection(ctx, RegionProjectionSupersession{
				TargetServerInstanceID:          recipient.ServerInstanceID,
				TargetCharacterID:               recipient.CharacterID,
				ProjectionSourceCharacterID:     payload.CharacterID,
				ProjectionSourceFencingToken:    payload.FencingToken,
				ProjectionVersion:               payload.Version,
				ProjectionRecipientFencingToken: recipient.FencingToken,
				SupersedingEventID:              event.ID,
				SupersededAt:                    event.CreatedAt,
			})
			if supersedeErr != nil {
				s.recordStoreError("region_projection.supersede", supersedeErr)
				s.recordRegionProjectionEvent("failed", event, &payload, "supersession_failed")
			} else {
				for supersededIndex := 0; supersededIndex < supersededCount; supersededIndex++ {
					s.recordRegionProjectionEvent(regionProjectionSuperseded, event, &payload, "")
				}
			}
		}
		s.recordGameplayEvent(map[bool]string{true: "produced", false: "duplicate"}[created], event, "")
		s.recordRegionProjectionEvent(result, event, &payload, "")
	}
	return produced
}

func (s *Server) regionProjectionRecipientEligible(payload regionPlayerProjectionPayload, recipient *SessionOwnership) bool {
	if recipient == nil {
		return false
	}
	if payload.Action == regionProjectionActionDespawn {
		return true
	}
	radius := s.config.RegionProjectionInterestRadius
	if radius <= 0 {
		return true
	}
	recipientPosition := runtimePoint{X: recipient.PositionX, Z: recipient.PositionZ}
	if !finiteProjectionNumber(recipientPosition.X) || !finiteProjectionNumber(recipientPosition.Z) {
		return true
	}
	return distance(payload.Position, recipientPosition) <= radius
}

func validateRegionPlayerProjectionPayload(payload regionPlayerProjectionPayload) error {
	if payload.Action != regionProjectionActionUpsert && payload.Action != regionProjectionActionDespawn {
		return errors.New("invalid action")
	}
	if strings.TrimSpace(payload.CharacterID) == "" || strings.TrimSpace(payload.RegionID) == "" || strings.TrimSpace(payload.SourceSessionID) == "" || strings.TrimSpace(payload.SourceServerInstanceID) == "" || payload.FencingToken <= 0 || payload.Version <= 0 {
		return errors.New("incomplete projection")
	}
	if !finiteProjectionNumber(payload.Position.X) || !finiteProjectionNumber(payload.Position.Z) || !finiteProjectionNumber(payload.Facing) {
		return errors.New("invalid projection coordinates")
	}
	if payload.Action == regionProjectionActionUpsert && strings.TrimSpace(payload.DisplayName) == "" {
		return errors.New("missing display name")
	}
	if payload.MovementDestination != nil && (!finiteProjectionNumber(payload.MovementDestination.X) || !finiteProjectionNumber(payload.MovementDestination.Z)) {
		return errors.New("invalid movement destination")
	}
	return nil
}

func finiteProjectionNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (payload regionPlayerProjectionPayload) runtimeEntity() runtimeEntity {
	visualTarget := any(nil)
	if payload.VisualTargetID != "" {
		visualTarget = payload.VisualTargetID
	}
	movementDestination := any(nil)
	if payload.MovementDestination != nil {
		movementDestination = *payload.MovementDestination
	}
	return runtimeEntity{
		EntityID:   payload.CharacterID,
		EntityType: "player",
		TemplateID: playerTemplateID,
		Position:   payload.Position,
		State: map[string]any{
			"name":                 payload.DisplayName,
			"race":                 payload.Race,
			"base_class":           payload.BaseClass,
			"sex":                  payload.Sex,
			"hair_style":           payload.HairStyle,
			"hair_color":           payload.HairColor,
			"skin_type":            payload.SkinType,
			"facing":               payload.Facing,
			"moving":               payload.Moving,
			"movement_destination": movementDestination,
			"visual_target_id":     visualTarget,
			"projection_only":      true,
			"projection_fence":     payload.FencingToken,
			"projection_version":   payload.Version,
		},
	}
}

func compareRegionProjectionOrder(meta remotePlayerProjectionMeta, fence int64, version int64) int {
	if fence < meta.FencingToken || (fence == meta.FencingToken && version < meta.Version) {
		return -1
	}
	if fence == meta.FencingToken && version == meta.Version {
		return 0
	}
	return 1
}

func (runtime *attachedRuntime) applyRegionPlayerProjection(payload regionPlayerProjectionPayload, now time.Time) (map[string]any, regionProjectionApplyResult) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.remotePlayerProjections == nil {
		runtime.remotePlayerProjections = map[string]remotePlayerProjectionMeta{}
	}
	if existingMeta, exists := runtime.remotePlayerProjections[payload.CharacterID]; exists {
		switch compareRegionProjectionOrder(existingMeta, payload.FencingToken, payload.Version) {
		case -1:
			return nil, regionProjectionStale
		case 0:
			return nil, regionProjectionDuplicate
		}
	}
	meta := remotePlayerProjectionMeta{
		FencingToken: payload.FencingToken,
		Version:      payload.Version,
		LastSeenAt:   now,
		Visible:      payload.Action == regionProjectionActionUpsert,
	}
	runtime.remotePlayerProjections[payload.CharacterID] = meta
	if payload.Action == regionProjectionActionDespawn {
		entity, exists := runtime.knownEntities[payload.CharacterID]
		if !exists || entity.EntityType != "player" {
			return nil, regionProjectionDespawned
		}
		delete(runtime.knownEntities, payload.CharacterID)
		if runtime.targetID == payload.CharacterID {
			runtime.targetID = ""
		}
		runtime.regionRevision++
		return entityDisappearMessage(runtime.regionRevision, payload.CharacterID, regionProjectionDisappear), regionProjectionDespawned
	}

	entity := payload.runtimeEntity()
	existing, exists := runtime.knownEntities[payload.CharacterID]
	runtime.knownEntities[payload.CharacterID] = cloneRuntimeEntity(entity)
	if !exists || existing.EntityType != "player" {
		runtime.regionRevision++
		return entityAppearMessage(runtime.regionRevision, cloneRuntimeEntity(entity)), regionProjectionApplied
	}
	existingPatch := playerPresencePatchFromEntity(existing)
	nextPatch := playerPresencePatchFromEntity(entity)
	if reflect.DeepEqual(existingPatch, nextPatch) {
		return nil, regionProjectionApplied
	}
	runtime.revision++
	return deltaMessage(runtime.revision, "", 0, nil, []map[string]any{nextPatch}, nil), regionProjectionApplied
}

func (runtime *attachedRuntime) expireRegionPlayerProjections(now time.Time, ttl time.Duration) []map[string]any {
	if runtime == nil || ttl <= 0 {
		return nil
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	messages := make([]map[string]any, 0)
	for characterID, meta := range runtime.remotePlayerProjections {
		if !meta.Visible || now.Before(meta.LastSeenAt.Add(ttl)) {
			continue
		}
		meta.Visible = false
		runtime.remotePlayerProjections[characterID] = meta
		entity, exists := runtime.knownEntities[characterID]
		if !exists || entity.EntityType != "player" {
			continue
		}
		delete(runtime.knownEntities, characterID)
		if runtime.targetID == characterID {
			runtime.targetID = ""
		}
		runtime.regionRevision++
		messages = append(messages, entityDisappearMessage(runtime.regionRevision, characterID, regionProjectionExpired))
	}
	return messages
}

func (s *Server) expireAttachedRegionPlayerProjections(attached *attachedSession, now time.Time) (int, bool) {
	if s == nil || attached == nil {
		return 0, true
	}
	expiredCount := 0
	if !attached.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
		messages := runtime.expireRegionPlayerProjections(now, s.config.RegionProjectionTTL)
		expiredCount = len(messages)
		return messages
	}) {
		return 0, false
	}
	for index := 0; index < expiredCount; index++ {
		s.recordRegionProjectionEvent("expired", nil, nil, "ttl_elapsed")
	}
	return expiredCount, true
}

func (s *Server) deliverRegionPlayerProjection(ctx context.Context, event *GameplayEvent) error {
	var payload regionPlayerProjectionPayload
	if event == nil || json.Unmarshal(event.Payload, &payload) != nil || validateRegionPlayerProjectionPayload(payload) != nil {
		return errors.New("invalid_payload")
	}
	if payload.RegionID != event.TargetRegionID || payload.SourceServerInstanceID == s.config.ServerInstanceID || payload.CharacterID == event.TargetCharacterID {
		return errors.New("invalid_payload")
	}
	scope, attached, ownership, err := s.resolveCharacterPresence(ctx, event.TargetCharacterID)
	if err != nil {
		return errors.New("presence_unavailable")
	}
	if scope != characterPresenceLocal || attached == nil || ownership == nil || attached.runtime == nil {
		return errors.New("social.recipient_stale_owner")
	}
	if payload.RecipientFencingToken <= 0 || ownership.SessionID != event.TargetSessionID || ownership.ServerInstanceID != event.TargetServerInstanceID || attached.fencingToken != ownership.FencingToken || attached.fencingToken != payload.RecipientFencingToken || ownership.FencingToken != payload.RecipientFencingToken || attached.runtime.regionIDValue() != event.TargetRegionID {
		return errors.New("social.recipient_stale_owner")
	}
	if payload.Action == regionProjectionActionUpsert {
		sourceOwnership, sourceErr := s.store.GameplaySessions.GetActiveOwnershipByCharacterID(ctx, payload.CharacterID)
		if sourceErr != nil {
			if errors.Is(sourceErr, errRecordNotFound) {
				s.recordRegionProjectionEvent("stale_ignored", event, &payload, "source_owner_expired")
				return nil
			}
			return errors.New("presence_unavailable")
		}
		if sourceOwnership.SessionID != payload.SourceSessionID || sourceOwnership.ServerInstanceID != payload.SourceServerInstanceID || sourceOwnership.FencingToken != payload.FencingToken || sourceOwnership.RegionID != payload.RegionID {
			s.recordRegionProjectionEvent("stale_ignored", event, &payload, "source_owner_changed")
			return nil
		}
	}

	var applyResult regionProjectionApplyResult
	if !attached.dispatchAll(func(runtime *attachedRuntime) []map[string]any {
		message, result := runtime.applyRegionPlayerProjection(payload, time.Now())
		applyResult = result
		if message == nil {
			return nil
		}
		return []map[string]any{message}
	}) {
		return errors.New("projection.socket_delivery_failed")
	}
	s.recordRegionProjectionEvent(string(applyResult), event, &payload, "")
	return nil
}

func isRegionPlayerProjectionEvent(event *GameplayEvent) bool {
	return event != nil && event.Type == regionPlayerProjectionEventType
}

func (s *Server) recordRegionProjectionEvent(result string, event *GameplayEvent, payload *regionPlayerProjectionPayload, reasonCode string) {
	if s == nil || s.observer == nil {
		return
	}
	s.observer.incCounter("l2bg_region_projection_events_total", "Total cross-instance regional player projection lifecycle events.", map[string]string{
		"result": result,
	}, 1)
	fields := map[string]any{
		"result":             result,
		"server_instance_id": s.config.ServerInstanceID,
	}
	if event != nil {
		fields["event_id"] = event.ID
		fields["target_server_instance_id"] = event.TargetServerInstanceID
		fields["target_region_id"] = event.TargetRegionID
	}
	if payload != nil {
		fields["source_character_id"] = payload.CharacterID
		fields["fencing_token"] = payload.FencingToken
		fields["projection_version"] = payload.Version
		fields["projection_action"] = payload.Action
	}
	if reasonCode != "" {
		fields["reason_code"] = reasonCode
	}
	level := "info"
	if result == "failed" || result == "dead_letter" {
		level = "error"
	}
	s.observer.log(level, "region_player_projection", fields)
}
