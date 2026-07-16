package app

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func projectionMessagesOfKind(messages []map[string]any, kind string) int {
	count := 0
	for _, message := range messages {
		if message["kind"] == kind {
			count++
		}
	}
	return count
}

func TestRegionPlayerProjectionCrossesInstancesAndRemainsProjectionOnly(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "region_projection")
	otherCharacter, otherSession := createOwnershipTestCharacterAndSession(t, fixture.storeA, "region_projection_other", "region_projection_other_session")
	otherOwned, err := fixture.storeB.GameplaySessions.AcquireOwnership(context.Background(), otherSession.ID, otherSession.AttachToken, "instance-b", time.Minute, 5*time.Minute)
	if err != nil {
		t.Fatalf("AcquireOwnership(other region) error=%v", err)
	}
	if _, err := fixture.storeB.GameplaySessions.RenewOwnership(context.Background(), otherCharacter.ID, otherOwned.Session.ID, "instance-b", otherOwned.Session.FencingToken, "gate_road", time.Minute, 5*time.Minute); err != nil {
		t.Fatalf("RenewOwnership(other region) error=%v", err)
	}
	actorAttached := fixture.serverA.attachedSessionBySessionID(fixture.actor.session.ID)
	if actorAttached == nil {
		t.Fatal("actor attachment missing")
	}

	requests := actorAttached.nextRegionProjectionRequests(regionProjectionActionUpsert, time.Now())
	if len(requests) != 1 {
		t.Fatalf("initial projection requests=%d", len(requests))
	}
	if produced := fixture.serverA.publishRegionProjectionRequest(context.Background(), requests[0]); produced != 1 {
		t.Fatalf("initial projection produced=%d", produced)
	}
	if produced := fixture.serverA.publishRegionProjectionRequest(context.Background(), requests[0]); produced != 0 {
		t.Fatalf("identical projection replay produced=%d", produced)
	}
	if fixture.backend.nextGameplayEventID != 1 {
		t.Fatalf("identical projection replay event count=%d", fixture.backend.nextGameplayEventID)
	}
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/projection-initial"); claimed != 1 {
		t.Fatalf("initial projection claimed=%d", claimed)
	}

	fixture.target.runtime.mu.Lock()
	projected, known := fixture.target.runtime.knownEntities[fixture.actorCharacter.ID]
	fixture.target.runtime.mu.Unlock()
	if !known || projected.EntityType != "player" || projected.State["projection_only"] != true || projected.State["projection_fence"] != fixture.actor.session.FencingToken {
		t.Fatalf("remote projection=%+v known=%v", projected, known)
	}
	if projectionMessagesOfKind(fixture.targetMessageSnapshot(), "entity_appear") != 1 {
		t.Fatalf("initial projection messages=%+v", fixture.targetMessageSnapshot())
	}
	assertMetricLine(t, fixture.serverA.observer.renderPrometheus(), `l2bg_region_projection_events_total{result="projection_produced"} 1`)
	assertMetricLine(t, fixture.serverA.observer.renderPrometheus(), `l2bg_region_projection_events_total{result="duplicate"} 1`)
	assertMetricLine(t, fixture.serverB.observer.renderPrometheus(), `l2bg_region_projection_events_total{result="projection_consumed"} 1`)

	fixture.actor.runtime.mu.Lock()
	fixture.actor.runtime.position = runtimePoint{X: 12, Z: -7}
	fixture.actor.runtime.facing = 1.25
	fixture.actor.runtime.activeMovement = &runtimeMovementState{
		AcceptedDestination: runtimePoint{X: 18, Z: -9},
		Waypoints:           []runtimePoint{{X: 18, Z: -9}},
		LastAdvancedAt:      time.Now(),
	}
	fixture.actor.runtime.mu.Unlock()
	fixture.serverA.fanOutPresenceState(fixture.actor.session.ID, fixture.actor.runtime)
	var movementRequest regionProjectionPublishRequest
	select {
	case movementRequest = <-fixture.serverA.regionProjectionQueue:
	default:
		t.Fatal("movement did not enqueue a regional projection")
	}
	if produced := fixture.serverA.publishRegionProjectionRequest(context.Background(), movementRequest); produced != 1 {
		t.Fatalf("movement projection produced=%d", produced)
	}
	movementEvent, err := fixture.storeA.GameplayEvents.GetByIdempotencyKey(
		context.Background(),
		"region-player-projection/"+fixture.actorCharacter.ID+"/"+formatInt64(fixture.actor.session.FencingToken)+"/2/"+fixture.targetCharacter.ID+"/"+formatInt64(fixture.target.session.FencingToken),
	)
	if err != nil {
		t.Fatalf("movement projection event error=%v", err)
	}
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/projection-move"); claimed != 1 {
		t.Fatalf("movement projection claimed=%d", claimed)
	}
	fixture.target.runtime.mu.Lock()
	projected = fixture.target.runtime.knownEntities[fixture.actorCharacter.ID]
	fixture.target.runtime.mu.Unlock()
	if projected.Position != (runtimePoint{X: 12, Z: -7}) || projected.State["moving"] != true || projected.State["projection_version"] != int64(2) {
		t.Fatalf("moved projection=%+v", projected)
	}
	if destination, ok := projected.State["movement_destination"].(runtimePoint); !ok || destination != (runtimePoint{X: 18, Z: -9}) {
		t.Fatalf("movement destination=%+v", projected.State["movement_destination"])
	}

	restartedConsumer := NewServerWithConfig(":0", "", fixture.storeB, ServerConfig{ServerInstanceID: "instance-b"})
	messageCount := len(fixture.targetMessageSnapshot())
	if err := restartedConsumer.deliverGameplayEvent(context.Background(), movementEvent, "instance-b/restarted-projection"); err != nil {
		t.Fatalf("logical restart redelivery error=%v", err)
	}
	if len(fixture.targetMessageSnapshot()) != messageCount {
		t.Fatalf("logical restart duplicated projection messages=%+v", fixture.targetMessageSnapshot())
	}

	selectCommand := commandEnvelope{
		ProtocolVersion: 1,
		CommandID:       "projection_target_remote",
		CommandSeq:      1,
		Type:            "select_target",
		Payload:         mustMarshalCommandPayload(t, map[string]any{"target_id": fixture.actorCharacter.ID}),
	}
	outbound, _ := fixture.serverB.processGameplayCommandWithDedup(context.Background(), fixture.target.session, fixture.target.runtime, selectCommand)
	if reason := extractRejectReason(outbound); reason != "presence.target_remote" {
		t.Fatalf("projected remote target reason=%q outbound=%+v", reason, outbound)
	}
}

func TestRegionPlayerProjectionBuildsOrderedRegionTransition(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "region_projection_transition")
	actorAttached := fixture.serverA.attachedSessionBySessionID(fixture.actor.session.ID)
	initial := actorAttached.nextRegionProjectionRequests(regionProjectionActionUpsert, time.Now())
	if len(initial) != 1 || initial[0].Payload.RegionID != "dawn_plaza" || initial[0].Payload.Version != 1 {
		t.Fatalf("initial projection requests=%+v", initial)
	}
	fixture.actor.runtime.mu.Lock()
	fixture.actor.runtime.regionID = "gate_road"
	fixture.actor.runtime.mu.Unlock()
	transition := actorAttached.nextRegionProjectionRequests(regionProjectionActionUpsert, time.Now())
	if len(transition) != 2 {
		t.Fatalf("region transition requests=%+v", transition)
	}
	if transition[0].Payload.Action != regionProjectionActionDespawn || transition[0].Payload.RegionID != "dawn_plaza" || transition[0].Payload.Version != 2 {
		t.Fatalf("region transition despawn=%+v", transition[0].Payload)
	}
	if transition[1].Payload.Action != regionProjectionActionUpsert || transition[1].Payload.RegionID != "gate_road" || transition[1].Payload.Version != 3 {
		t.Fatalf("region transition upsert=%+v", transition[1].Payload)
	}
}

func TestRegionPlayerProjectionStaleRecipientDeadLettersWithoutVisualSuccess(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "region_projection_stale_recipient")
	actorAttached := fixture.serverA.attachedSessionBySessionID(fixture.actor.session.ID)
	if produced := fixture.serverA.publishAttachedRegionProjectionNow(context.Background(), actorAttached, regionProjectionActionUpsert, time.Now()); produced != 1 {
		t.Fatalf("projection produced=%d", produced)
	}
	eventKey := "region-player-projection/" + fixture.actorCharacter.ID + "/" + formatInt64(fixture.actor.session.FencingToken) + "/1/" + fixture.targetCharacter.ID + "/" + formatInt64(fixture.target.session.FencingToken)
	fixture.serverB.unregisterAttachedSession(fixture.target.session.ID, fixture.target.session.FencingToken)
	if released, err := fixture.storeB.GameplaySessions.ReleaseOwnership(context.Background(), fixture.targetCharacter.ID, fixture.target.session.ID, "instance-b", fixture.target.session.FencingToken); err != nil || !released {
		t.Fatalf("release stale recipient released=%v error=%v", released, err)
	}
	fixture.serverB.config.GameplayEventRetryDelay = time.Nanosecond
	fixture.serverB.config.GameplayEventMaxRetries = 1
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/projection-stale-recipient"); claimed != 1 {
		t.Fatalf("stale recipient projection claimed=%d", claimed)
	}
	event, err := fixture.storeB.GameplayEvents.GetByIdempotencyKey(context.Background(), eventKey)
	if err != nil || event.DeadLetteredAt.IsZero() || event.LastError != "social.recipient_stale_owner" {
		t.Fatalf("stale recipient event=%+v error=%v", event, err)
	}
	fixture.target.runtime.mu.Lock()
	_, projected := fixture.target.runtime.knownEntities[fixture.actorCharacter.ID]
	fixture.target.runtime.mu.Unlock()
	if projected || projectionMessagesOfKind(fixture.targetMessageSnapshot(), "entity_appear") != 0 {
		t.Fatalf("stale recipient received visual success messages=%+v", fixture.targetMessageSnapshot())
	}
	assertMetricLine(t, fixture.serverB.observer.renderPrometheus(), `l2bg_region_projection_events_total{result="dead_letter"} 1`)
}

func TestRegionPlayerProjectionIgnoresOldEventsAndExpiresOrDespawns(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "region_projection_order")
	actorAttached := fixture.serverA.attachedSessionBySessionID(fixture.actor.session.ID)
	if produced := fixture.serverA.publishAttachedRegionProjectionNow(context.Background(), actorAttached, regionProjectionActionUpsert, time.Now()); produced != 1 {
		t.Fatalf("initial projection produced=%d", produced)
	}
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/projection-order-initial"); claimed != 1 {
		t.Fatalf("initial projection claimed=%d", claimed)
	}

	fixture.actor.runtime.mu.Lock()
	fixture.actor.runtime.position = runtimePoint{X: 9, Z: 3}
	fixture.actor.runtime.mu.Unlock()
	if produced := fixture.serverA.publishAttachedRegionProjectionNow(context.Background(), actorAttached, regionProjectionActionUpsert, time.Now()); produced != 1 {
		t.Fatalf("newer projection produced=%d", produced)
	}
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/projection-order-newer"); claimed != 1 {
		t.Fatalf("newer projection claimed=%d", claimed)
	}

	stalePayload := requestsPayloadForProjectionTest(t, fixture, regionProjectionActionUpsert, 1, runtimePoint{X: -30, Z: -30})
	staleEvent := projectionEventForTest(t, fixture, "region-projection-stale-order", stalePayload)
	if created, err := fixture.storeA.GameplayEvents.Create(context.Background(), staleEvent); err != nil || !created {
		t.Fatalf("stale event create=%v error=%v", created, err)
	}
	messageCount := len(fixture.targetMessageSnapshot())
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/projection-order-stale"); claimed != 1 {
		t.Fatalf("stale projection claimed=%d", claimed)
	}
	fixture.target.runtime.mu.Lock()
	projected := fixture.target.runtime.knownEntities[fixture.actorCharacter.ID]
	fixture.target.runtime.mu.Unlock()
	if projected.Position != (runtimePoint{X: 9, Z: 3}) || len(fixture.targetMessageSnapshot()) != messageCount {
		t.Fatalf("stale projection overwrote state=%+v messages=%+v", projected, fixture.targetMessageSnapshot())
	}

	targetAttached := fixture.serverB.attachedSessionBySessionID(fixture.target.session.ID)
	expiredCount, delivered := fixture.serverB.expireAttachedRegionPlayerProjections(targetAttached, time.Now().Add(fixture.serverB.config.RegionProjectionTTL+time.Second))
	if !delivered || expiredCount != 1 {
		t.Fatalf("expired projection count=%d delivered=%v", expiredCount, delivered)
	}
	expiredMessages := fixture.targetMessageSnapshot()
	if expiredMessages[len(expiredMessages)-1]["kind"] != "entity_disappear" || expiredMessages[len(expiredMessages)-1]["reason"] != regionProjectionExpired {
		t.Fatalf("expired projection messages=%+v", expiredMessages)
	}
	assertMetricLine(t, fixture.serverB.observer.renderPrometheus(), `l2bg_region_projection_events_total{result="stale_ignored"} 1`)
	assertMetricLine(t, fixture.serverB.observer.renderPrometheus(), `l2bg_region_projection_events_total{result="expired"} 1`)
	fixture.target.runtime.mu.Lock()
	_, knownAfterTTL := fixture.target.runtime.knownEntities[fixture.actorCharacter.ID]
	fixture.target.runtime.mu.Unlock()
	if knownAfterTTL {
		t.Fatal("TTL left stale projection in known-set")
	}
	if message, result := fixture.target.runtime.applyRegionPlayerProjection(stalePayload, time.Now()); message != nil || result != regionProjectionStale {
		t.Fatalf("old event after TTL message=%+v result=%q", message, result)
	}

	if produced := fixture.serverA.publishAttachedRegionProjectionNow(context.Background(), actorAttached, regionProjectionActionUpsert, time.Now()); produced != 1 {
		t.Fatalf("heartbeat after TTL produced=%d", produced)
	}
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/projection-after-ttl"); claimed != 1 {
		t.Fatalf("heartbeat after TTL claimed=%d", claimed)
	}
	if produced := fixture.serverA.publishAttachedRegionProjectionNow(context.Background(), actorAttached, regionProjectionActionDespawn, time.Now()); produced != 1 {
		t.Fatalf("despawn produced=%d", produced)
	}
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/projection-despawn"); claimed != 1 {
		t.Fatalf("despawn claimed=%d", claimed)
	}
	fixture.target.runtime.mu.Lock()
	_, knownAfterDespawn := fixture.target.runtime.knownEntities[fixture.actorCharacter.ID]
	fixture.target.runtime.mu.Unlock()
	if knownAfterDespawn {
		t.Fatal("despawn left projection in known-set")
	}
	lastMessages := fixture.targetMessageSnapshot()
	if lastMessages[len(lastMessages)-1]["kind"] != "entity_disappear" || lastMessages[len(lastMessages)-1]["reason"] != regionProjectionDisappear {
		t.Fatalf("despawn messages=%+v", lastMessages)
	}
}

func requestsPayloadForProjectionTest(t *testing.T, fixture *crossInstanceSocialFixture, action string, version int64, position runtimePoint) regionPlayerProjectionPayload {
	t.Helper()
	payload := fixture.actor.runtime.regionProjectionPayload()
	payload.Action = action
	payload.Position = position
	payload.SourceSessionID = fixture.actor.session.ID
	payload.SourceServerInstanceID = "instance-a"
	payload.FencingToken = fixture.actor.session.FencingToken
	payload.Version = version
	return payload
}

func projectionEventForTest(t *testing.T, fixture *crossInstanceSocialFixture, key string, payload regionPlayerProjectionPayload) *GameplayEvent {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal projection payload error=%v", err)
	}
	return &GameplayEvent{
		IdempotencyKey:         key,
		Type:                   regionPlayerProjectionEventType,
		Payload:                encoded,
		TargetServerInstanceID: "instance-b",
		TargetRegionID:         payload.RegionID,
		TargetSessionID:        fixture.target.session.ID,
		TargetCharacterID:      fixture.targetCharacter.ID,
	}
}

func formatInt64(value int64) string {
	return fmt.Sprintf("%d", value)
}
