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
	regionProjectionQueueSize       = 256
	regionProjectionDisappear       = "remote_projection_despawn"
	regionProjectionExpired         = "remote_projection_expired"
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
		select {
		case s.regionProjectionQueue <- request:
		default:
			s.recordRegionProjectionEvent("failed", nil, &request.Payload, "publisher_queue_full")
		}
	}
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
	encoded, err := json.Marshal(payload)
	if err != nil {
		s.recordRegionProjectionEvent("failed", nil, &payload, "invalid_payload")
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
		event := &GameplayEvent{
			IdempotencyKey: fmt.Sprintf(
				"region-player-projection/%s/%d/%d/%s/%d",
				payload.CharacterID,
				payload.FencingToken,
				payload.Version,
				recipient.CharacterID,
				recipient.FencingToken,
			),
			Type:                   regionPlayerProjectionEventType,
			Payload:                encoded,
			TargetServerInstanceID: recipient.ServerInstanceID,
			TargetRegionID:         payload.RegionID,
			TargetSessionID:        recipient.SessionID,
			TargetCharacterID:      recipient.CharacterID,
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
		}
		s.recordGameplayEvent(map[bool]string{true: "produced", false: "duplicate"}[created], event, "")
		s.recordRegionProjectionEvent(result, event, &payload, "")
	}
	return produced
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
	if ownership.SessionID != event.TargetSessionID || ownership.ServerInstanceID != event.TargetServerInstanceID || attached.fencingToken != ownership.FencingToken || attached.runtime.regionIDValue() != event.TargetRegionID {
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
