package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type characterPresenceScope string

const (
	characterPresenceOffline     characterPresenceScope = "offline"
	characterPresenceLocal       characterPresenceScope = "local"
	characterPresenceRemote      characterPresenceScope = "remote"
	characterPresenceUnavailable characterPresenceScope = "unavailable"
)

func (attached *attachedSession) updateLease(leaseExpiresAt time.Time) {
	if attached == nil {
		return
	}
	attached.leaseMu.Lock()
	attached.leaseExpiresAt = leaseExpiresAt
	attached.leaseMu.Unlock()
}

func (attached *attachedSession) leaseDeadline() time.Time {
	if attached == nil {
		return time.Time{}
	}
	attached.leaseMu.Lock()
	defer attached.leaseMu.Unlock()
	return attached.leaseExpiresAt
}

func runtimeOwnershipAnchor(runtime *attachedRuntime, fallbackRegionID string) (string, runtimePoint) {
	if runtime == nil {
		return fallbackRegionID, runtimePoint{}
	}
	regionID, position := runtime.characterWorldState()
	if regionID == "" {
		regionID = fallbackRegionID
	}
	return regionID, position
}

func (s *Server) renewSessionOwnership(ctx context.Context, session *Session, runtime *attachedRuntime, fallbackRegionID string, observeSuccess bool) error {
	if s == nil || session == nil || session.FencingToken == 0 {
		return nil
	}
	if s.store == nil || s.store.GameplaySessions == nil {
		return errOwnershipStale
	}
	regionID, position := runtimeOwnershipAnchor(runtime, fallbackRegionID)
	ownership, err := s.store.GameplaySessions.RenewOwnership(
		ctx,
		session.CharacterID,
		session.ID,
		s.config.ServerInstanceID,
		session.FencingToken,
		regionID,
		position,
		s.config.SessionLeaseDuration,
		s.config.SessionAttachTokenTTL,
	)
	if err != nil {
		result := "rejected_stale_owner"
		if errors.Is(err, errOwnershipExpired) {
			result = "expired"
		}
		s.recordOwnershipEvent(result, session, nil)
		return err
	}
	if attached := s.attachedSessionBySessionID(session.ID); attached != nil && attached.fencingToken == session.FencingToken {
		attached.updateLease(ownership.LeaseExpiresAt)
		if attached.regionProjectionRegion() != "" && attached.regionProjectionRegion() != ownership.RegionID {
			s.enqueueRegionPlayerProjection(attached, regionProjectionActionUpsert, time.Now())
		}
	}
	if observeSuccess {
		s.recordOwnershipEvent("renewed", session, ownership)
	}
	return nil
}

func (s *Server) refreshSessionOwnershipAnchor(ctx context.Context, session *Session, runtime *attachedRuntime, fallbackRegionID string) error {
	if s == nil || session == nil || runtime == nil || session.FencingToken == 0 {
		return nil
	}
	if s.store == nil || s.store.GameplaySessions == nil {
		return errOwnershipStale
	}
	regionID, position := runtimeOwnershipAnchor(runtime, fallbackRegionID)
	_, err := s.store.GameplaySessions.RefreshOwnershipAnchor(
		ctx,
		session.CharacterID,
		session.ID,
		s.config.ServerInstanceID,
		session.FencingToken,
		regionID,
		position,
	)
	return err
}

func (s *Server) commandOwnershipReject(ctx context.Context, session *Session, runtime *attachedRuntime, command commandEnvelope) map[string]any {
	regionID := ""
	if runtime != nil {
		regionID = runtime.regionIDValue()
	}
	err := s.renewSessionOwnership(ctx, session, runtime, regionID, false)
	if err == nil {
		return nil
	}
	if errors.Is(err, errOwnershipStale) || errors.Is(err, errOwnershipExpired) {
		return rejectMessage(command.CommandID, command.CommandSeq, "session.stale_owner", "Gameplay session ownership is no longer valid on this server instance.")
	}
	s.recordStoreError("gameplay_sessions.renew_ownership", err, errOwnershipStale, errOwnershipExpired)
	return rejectMessage(command.CommandID, command.CommandSeq, "system.persistence_failed", "Unable to validate gameplay session ownership.")
}

func (s *Server) resolveCharacterPresence(ctx context.Context, characterID string) (characterPresenceScope, *attachedSession, *SessionOwnership, error) {
	if s == nil || characterID == "" || s.store == nil || s.store.GameplaySessions == nil {
		return characterPresenceOffline, nil, nil, nil
	}
	local := s.attachedSessionByCharacterID(characterID)
	if local != nil && local.fencingToken == 0 {
		return characterPresenceLocal, local, nil, nil
	}
	ownership, err := s.store.GameplaySessions.GetActiveOwnershipByCharacterID(ctx, characterID)
	if err != nil {
		if errors.Is(err, errRecordNotFound) {
			return characterPresenceOffline, nil, nil, nil
		}
		return characterPresenceUnavailable, nil, nil, err
	}
	if ownership.ServerInstanceID != s.config.ServerInstanceID {
		return characterPresenceRemote, nil, ownership, nil
	}
	if local == nil || local.sessionID != ownership.SessionID || local.fencingToken != ownership.FencingToken {
		return characterPresenceUnavailable, nil, ownership, nil
	}
	return characterPresenceLocal, local, ownership, nil
}

func (s *Server) processTargetCommand(ctx context.Context, session *Session, runtime *attachedRuntime, command commandEnvelope) ([]map[string]any, *GameplayEvent) {
	if runtime == nil {
		return []map[string]any{rejectMessage(command.CommandID, command.CommandSeq, "internal.unexpected_error", "Target pipeline is unavailable.")}, nil
	}
	if command.Type != "select_target" {
		return runtime.processCommand(command), nil
	}
	var payload struct {
		TargetID string `json:"target_id"`
	}
	if err := json.Unmarshal(command.Payload, &payload); err != nil || payload.TargetID == "" {
		return runtime.processCommand(command), nil
	}
	runtime.mu.Lock()
	entity, known := runtime.knownEntities[payload.TargetID]
	runtime.mu.Unlock()
	if !known || entity.EntityType != "player" {
		return runtime.processCommand(command), nil
	}

	scope, _, ownership, err := s.resolveCharacterPresence(ctx, payload.TargetID)
	if err != nil {
		return runtime.rejectPlayerTargetForPresence(command, "system.persistence_failed", "Unable to resolve authoritative player presence."), nil
	}
	switch scope {
	case characterPresenceRemote:
		outbound := runtime.rejectPlayerTargetForPresence(command, "presence.target_remote", "Referenced player is online on another server instance and is not locally interactable.")
		if extractRejectReason(outbound) != "presence.target_remote" {
			return outbound, nil
		}
		event, buildErr := buildRemoteTargetNoticeEvent(session, runtime.characterID, payload.TargetID, s.config.ServerInstanceID, ownership, command)
		if buildErr != nil {
			return outbound, nil
		}
		return outbound, event
	case characterPresenceOffline, characterPresenceUnavailable:
		return runtime.rejectPlayerTargetForPresence(command, "presence.target_offline", "Referenced player is no longer online in the authoritative presence registry."), nil
	default:
		return runtime.processCommand(command), nil
	}
}

func (runtime *attachedRuntime) rejectPlayerTargetForPresence(command commandEnvelope, reasonCode string, message string) []map[string]any {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.advanceMovementLocked(time.Now())
	parsed, reject := runtime.preValidate(command)
	if reject != nil {
		return []map[string]any{reject}
	}
	runtime.expectedCommandSeq++
	outbound := []map[string]any{ackMessage(command.CommandID, command.CommandSeq)}
	entity, known := runtime.knownEntities[parsed.targetID]
	if !known {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "world.entity_not_known", "Referenced entity is not in the current known-set."))
	}
	if entity.EntityType != "player" {
		return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, "world.entity_not_interactable", "Referenced entity is not a player."))
	}
	return append(outbound, rejectMessage(command.CommandID, command.CommandSeq, reasonCode, message))
}

func (s *Server) recordOwnershipEvent(result string, session *Session, ownership *SessionOwnership) {
	if s == nil || s.observer == nil {
		return
	}
	if result == "" {
		result = "unknown"
	}
	fields := map[string]any{
		"result":             result,
		"server_instance_id": s.config.ServerInstanceID,
	}
	if session != nil {
		fields["session_id"] = session.ID
		fields["character_id"] = session.CharacterID
		fields["fencing_token"] = session.FencingToken
	}
	if ownership != nil {
		fields["session_id"] = ownership.SessionID
		fields["character_id"] = ownership.CharacterID
		fields["owner_server_instance_id"] = ownership.ServerInstanceID
		fields["fencing_token"] = ownership.FencingToken
		fields["region_id"] = ownership.RegionID
		fields["lease_expires_at"] = ownership.LeaseExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	s.observer.incCounter("l2bg_session_ownership_events_total", "Total durable gameplay session ownership events.", map[string]string{
		"result": result,
	}, 1)
	s.observer.log("info", "session_ownership", fields)
}
