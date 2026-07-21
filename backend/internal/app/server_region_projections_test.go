package app

import (
	"context"
	"encoding/json"
	"errors"
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
	if _, err := fixture.storeB.GameplaySessions.RenewOwnership(context.Background(), otherCharacter.ID, otherOwned.Session.ID, "instance-b", otherOwned.Session.FencingToken, "gate_road", runtimePoint{}, time.Minute, 5*time.Minute); err != nil {
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
	assertMetricLine(t, fixture.serverB.observer.renderPrometheus(), `l2bg_region_projection_delivery_delay_seconds_count 1`)

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

func TestRegionPlayerProjectionFiltersOutFarRemoteOwnerships(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "region_projection_interest")
	farCharacter, farSession := createOwnershipTestCharacterAndSession(t, fixture.storeA, "region_projection_far", "region_projection_far_session")
	farOwned, err := fixture.storeB.GameplaySessions.AcquireOwnership(context.Background(), farSession.ID, farSession.AttachToken, "instance-b", time.Minute, 5*time.Minute)
	if err != nil {
		t.Fatalf("AcquireOwnership(far recipient) error=%v", err)
	}
	if _, err := fixture.storeB.GameplaySessions.RefreshOwnershipAnchor(context.Background(), farCharacter.ID, farOwned.Session.ID, "instance-b", farOwned.Session.FencingToken, "dawn_plaza", runtimePoint{X: fixture.serverA.config.RegionProjectionInterestRadius + 200, Z: 0}); err != nil {
		t.Fatalf("RefreshOwnershipAnchor(far recipient) error=%v", err)
	}
	actorAttached := fixture.serverA.attachedSessionBySessionID(fixture.actor.session.ID)
	if actorAttached == nil {
		t.Fatal("actor attachment missing")
	}

	requests := actorAttached.nextRegionProjectionRequests(regionProjectionActionUpsert, time.Now())
	if len(requests) != 1 {
		t.Fatalf("interest-filter projection requests=%d", len(requests))
	}
	if produced := fixture.serverA.publishRegionProjectionRequest(context.Background(), requests[0]); produced != 1 {
		t.Fatalf("interest-filter projection produced=%d", produced)
	}

	nearKey := "region-player-projection/" + fixture.actorCharacter.ID + "/" + formatInt64(fixture.actor.session.FencingToken) + "/1/" + fixture.targetCharacter.ID + "/" + formatInt64(fixture.target.session.FencingToken)
	if _, err := fixture.storeA.GameplayEvents.GetByIdempotencyKey(context.Background(), nearKey); err != nil {
		t.Fatalf("near recipient missing projection row: %v", err)
	}
	farKey := "region-player-projection/" + fixture.actorCharacter.ID + "/" + formatInt64(fixture.actor.session.FencingToken) + "/1/" + farCharacter.ID + "/" + formatInt64(farOwned.Session.FencingToken)
	if _, err := fixture.storeA.GameplayEvents.GetByIdempotencyKey(context.Background(), farKey); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("far recipient should not receive projection row, err=%v", err)
	}
	assertMetricLine(t, fixture.serverA.observer.renderPrometheus(), `l2bg_region_projection_events_total{result="out_of_interest"} 1`)
	assertMetricLine(t, fixture.serverA.observer.renderPrometheus(), `l2bg_region_projection_fanout_total{result="candidates_before_filtering"} 2`)
	assertMetricLine(t, fixture.serverA.observer.renderPrometheus(), `l2bg_region_projection_fanout_total{result="eligible_after_filtering"} 1`)
	assertMetricLine(t, fixture.serverA.observer.renderPrometheus(), `l2bg_region_projection_fanout_total{reason="outside_interest_entry",result="filtered_out"} 1`)
	assertMetricLine(t, fixture.serverA.observer.renderPrometheus(), `l2bg_region_projection_fanout_total{result="projection_rows_produced"} 1`)
}

func TestRegionPlayerProjectionMovementCorridorExpandsEligibility(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "region_projection_corridor")
	actorAttached := fixture.serverA.attachedSessionBySessionID(fixture.actor.session.ID)
	if actorAttached == nil {
		t.Fatal("actor attachment missing")
	}
	entryRadius := regionProjectionInterestEnterRadius(fixture.serverA.config.RegionProjectionInterestRadius)
	targetAnchor := runtimePoint{X: entryRadius + 12, Z: 0}
	if _, err := fixture.storeB.GameplaySessions.RefreshOwnershipAnchor(context.Background(), fixture.targetCharacter.ID, fixture.target.session.ID, "instance-b", fixture.target.session.FencingToken, "dawn_plaza", targetAnchor); err != nil {
		t.Fatalf("RefreshOwnershipAnchor(target) error=%v", err)
	}
	fixture.actor.runtime.mu.Lock()
	fixture.actor.runtime.position = runtimePoint{}
	fixture.actor.runtime.activeMovement = &runtimeMovementState{
		AcceptedDestination: runtimePoint{X: targetAnchor.X + 20, Z: 0},
		Waypoints:           []runtimePoint{{X: targetAnchor.X + 20, Z: 0}},
		LastAdvancedAt:      time.Now(),
	}
	fixture.actor.runtime.mu.Unlock()

	requests := actorAttached.nextRegionProjectionRequests(regionProjectionActionUpsert, time.Now())
	if len(requests) != 1 {
		t.Fatalf("movement corridor requests=%d", len(requests))
	}
	if produced := fixture.serverA.publishRegionProjectionRequest(context.Background(), requests[0]); produced != 1 {
		t.Fatalf("movement corridor produced=%d", produced)
	}

	key := "region-player-projection/" + fixture.actorCharacter.ID + "/" + formatInt64(fixture.actor.session.FencingToken) + "/1/" + fixture.targetCharacter.ID + "/" + formatInt64(fixture.target.session.FencingToken)
	event, err := fixture.storeA.GameplayEvents.GetByIdempotencyKey(context.Background(), key)
	if err != nil {
		t.Fatalf("movement corridor event error=%v", err)
	}
	if event.ProjectionAction != regionProjectionActionUpsert {
		t.Fatalf("movement corridor projection action=%q", event.ProjectionAction)
	}
	assertMetricLine(t, fixture.serverA.observer.renderPrometheus(), `l2bg_region_projection_fanout_total{result="eligible_after_filtering"} 1`)
	assertMetricLine(t, fixture.serverA.observer.renderPrometheus(), `l2bg_region_projection_fanout_total{result="projection_rows_produced"} 1`)
}

func TestRegionPlayerProjectionRetainsRelevantRecipientAndDespawnsOnInterestLoss(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "region_projection_hysteresis")
	actorAttached := fixture.serverA.attachedSessionBySessionID(fixture.actor.session.ID)
	if actorAttached == nil {
		t.Fatal("actor attachment missing")
	}
	radius := fixture.serverA.config.RegionProjectionInterestRadius
	entryRadius := regionProjectionInterestEnterRadius(radius)
	initialAnchor := runtimePoint{X: entryRadius - 5, Z: 0}
	if _, err := fixture.storeB.GameplaySessions.RefreshOwnershipAnchor(context.Background(), fixture.targetCharacter.ID, fixture.target.session.ID, "instance-b", fixture.target.session.FencingToken, "dawn_plaza", initialAnchor); err != nil {
		t.Fatalf("RefreshOwnershipAnchor(initial) error=%v", err)
	}

	if produced := fixture.serverA.publishAttachedRegionProjectionNow(context.Background(), actorAttached, regionProjectionActionUpsert, time.Now()); produced != 1 {
		t.Fatalf("initial projection produced=%d", produced)
	}
	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/projection-hysteresis-initial"); claimed != 1 {
		t.Fatalf("initial hysteresis claim=%d", claimed)
	}

	retainedAnchor := runtimePoint{X: entryRadius + 8, Z: 0}
	if _, err := fixture.storeB.GameplaySessions.RefreshOwnershipAnchor(context.Background(), fixture.targetCharacter.ID, fixture.target.session.ID, "instance-b", fixture.target.session.FencingToken, "dawn_plaza", retainedAnchor); err != nil {
		t.Fatalf("RefreshOwnershipAnchor(retained) error=%v", err)
	}
	if produced := fixture.serverA.publishAttachedRegionProjectionNow(context.Background(), actorAttached, regionProjectionActionUpsert, time.Now()); produced != 1 {
		t.Fatalf("retained projection produced=%d", produced)
	}
	retainedKey := "region-player-projection/" + fixture.actorCharacter.ID + "/" + formatInt64(fixture.actor.session.FencingToken) + "/2/" + fixture.targetCharacter.ID + "/" + formatInt64(fixture.target.session.FencingToken)
	retainedEvent, err := fixture.storeA.GameplayEvents.GetByIdempotencyKey(context.Background(), retainedKey)
	if err != nil || retainedEvent.ProjectionAction != regionProjectionActionUpsert {
		t.Fatalf("retained projection event=%+v err=%v", retainedEvent, err)
	}

	lostAnchor := runtimePoint{X: radius + 40, Z: 0}
	if _, err := fixture.storeB.GameplaySessions.RefreshOwnershipAnchor(context.Background(), fixture.targetCharacter.ID, fixture.target.session.ID, "instance-b", fixture.target.session.FencingToken, "dawn_plaza", lostAnchor); err != nil {
		t.Fatalf("RefreshOwnershipAnchor(lost) error=%v", err)
	}
	if produced := fixture.serverA.publishAttachedRegionProjectionNow(context.Background(), actorAttached, regionProjectionActionUpsert, time.Now()); produced != 1 {
		t.Fatalf("lost-interest projection produced=%d", produced)
	}
	lostKey := "region-player-projection/" + fixture.actorCharacter.ID + "/" + formatInt64(fixture.actor.session.FencingToken) + "/3/" + fixture.targetCharacter.ID + "/" + formatInt64(fixture.target.session.FencingToken)
	lostEvent, err := fixture.storeA.GameplayEvents.GetByIdempotencyKey(context.Background(), lostKey)
	if err != nil {
		t.Fatalf("lost-interest event error=%v", err)
	}
	if lostEvent.ProjectionAction != regionProjectionActionDespawn {
		t.Fatalf("lost-interest action=%q", lostEvent.ProjectionAction)
	}
	retainedEvent, err = fixture.storeA.GameplayEvents.GetByIdempotencyKey(context.Background(), retainedKey)
	if err != nil {
		t.Fatalf("retained event reload error=%v", err)
	}
	if retainedEvent.SupersededAt.IsZero() || retainedEvent.SupersededByEventID != lostEvent.ID {
		t.Fatalf("lost-interest despawn did not supersede prior upsert: retained=%+v lost=%+v", retainedEvent, lostEvent)
	}

	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/projection-hysteresis-lost"); claimed != 1 {
		t.Fatalf("lost-interest claim=%d", claimed)
	}
	fixture.target.runtime.mu.Lock()
	_, known := fixture.target.runtime.knownEntities[fixture.actorCharacter.ID]
	meta := fixture.target.runtime.remotePlayerProjections[fixture.actorCharacter.ID]
	fixture.target.runtime.mu.Unlock()
	if known || meta.Visible || meta.Version != lostEvent.ProjectionVersion {
		t.Fatalf("lost-interest despawn missed visibility cleanup known=%v meta=%+v", known, meta)
	}
	if message, result := fixture.target.runtime.applyRegionPlayerProjection(requestsPayloadForProjectionTest(t, fixture, regionProjectionActionUpsert, 2, runtimePoint{X: 0, Z: 0}), time.Now()); message != nil || result != regionProjectionStale {
		t.Fatalf("superseded upsert resurrected after lost-interest despawn message=%+v result=%q", message, result)
	}
	assertMetricLine(t, fixture.serverA.observer.renderPrometheus(), `l2bg_region_projection_fanout_total{reason="outside_refined_interest",result="filtered_out"} 1`)
	assertMetricLine(t, fixture.serverA.observer.renderPrometheus(), `l2bg_region_projection_events_total{result="projection_row_superseded"} 1`)
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

func TestRegionProjectionQueuePressureCoalescesLatestStateWithinBound(t *testing.T) {
	server := NewServerWithConfig(":0", "", newMemoryStore(), ServerConfig{
		ServerInstanceID:          "queue-pressure-instance",
		RegionProjectionQueueSize: 1,
	})
	request := regionProjectionPublishRequest{Payload: regionPlayerProjectionPayload{
		Action:                 regionProjectionActionUpsert,
		CharacterID:            "queue-character",
		DisplayName:            "Queue Character",
		RegionID:               "dawn_plaza",
		SourceSessionID:        "queue-session",
		SourceServerInstanceID: "queue-pressure-instance",
		FencingToken:           1,
		Version:                1,
	}}

	server.enqueueRegionProjectionRequest(request)
	request.Payload.Version = 2
	server.enqueueRegionProjectionRequest(request)
	request.Payload.Version = 3
	server.enqueueRegionProjectionRequest(request)

	if len(server.regionProjectionQueue) != 1 {
		t.Fatalf("queue depth=%d", len(server.regionProjectionQueue))
	}
	coalesced, ok := server.takeCoalescedRegionProjection()
	if !ok || coalesced.Payload.Version != 3 {
		t.Fatalf("coalesced request=%+v ok=%v", coalesced, ok)
	}
	metrics := server.observer.renderPrometheus()
	assertMetricLine(t, metrics, `l2bg_region_projection_queue_events_total{result="enqueued"} 1`)
	assertMetricLine(t, metrics, `l2bg_region_projection_queue_events_total{result="coalesced"} 2`)
	assertMetricLine(t, metrics, `l2bg_region_projection_events_total{result="projection_queue_pressure"} 2`)
}

func TestRegionProjectionQueuePressureDropsOnlyBeyondBoundedSpill(t *testing.T) {
	server := NewServerWithConfig(":0", "", newMemoryStore(), ServerConfig{
		ServerInstanceID:          "queue-drop-instance",
		RegionProjectionQueueSize: 1,
	})
	server.enqueueRegionProjectionRequest(regionProjectionPublishRequest{Payload: regionPlayerProjectionPayload{
		CharacterID: "queue-head",
		RegionID:    "dawn_plaza",
		Version:     1,
	}})
	for index := 0; index <= server.regionProjectionLimit; index++ {
		server.enqueueRegionProjectionRequest(regionProjectionPublishRequest{Payload: regionPlayerProjectionPayload{
			CharacterID: fmt.Sprintf("spill-%d", index),
			RegionID:    "dawn_plaza",
			Version:     1,
		}})
	}

	if len(server.regionProjectionSpill) != server.regionProjectionLimit {
		t.Fatalf("spill depth=%d limit=%d", len(server.regionProjectionSpill), server.regionProjectionLimit)
	}
	assertMetricLine(t, server.observer.renderPrometheus(), `l2bg_region_projection_queue_events_total{result="dropped"} 1`)
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

func TestRegionPlayerProjectionBacklogSupersedesOlderUndeliveredVersion(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "region_projection_backlog_supersede")
	actorAttached := fixture.serverA.attachedSessionBySessionID(fixture.actor.session.ID)
	if actorAttached == nil {
		t.Fatal("actor attachment missing")
	}

	if produced := fixture.serverA.publishAttachedRegionProjectionNow(context.Background(), actorAttached, regionProjectionActionUpsert, time.Now()); produced != 1 {
		t.Fatalf("initial projection produced=%d", produced)
	}
	fixture.actor.runtime.mu.Lock()
	fixture.actor.runtime.position = runtimePoint{X: 44, Z: -11}
	fixture.actor.runtime.facing = 2.5
	fixture.actor.runtime.mu.Unlock()
	if produced := fixture.serverA.publishAttachedRegionProjectionNow(context.Background(), actorAttached, regionProjectionActionUpsert, time.Now()); produced != 1 {
		t.Fatalf("newer backlog projection produced=%d", produced)
	}

	firstKey := "region-player-projection/" + fixture.actorCharacter.ID + "/" + formatInt64(fixture.actor.session.FencingToken) + "/1/" + fixture.targetCharacter.ID + "/" + formatInt64(fixture.target.session.FencingToken)
	secondKey := "region-player-projection/" + fixture.actorCharacter.ID + "/" + formatInt64(fixture.actor.session.FencingToken) + "/2/" + fixture.targetCharacter.ID + "/" + formatInt64(fixture.target.session.FencingToken)
	firstEvent, err := fixture.storeA.GameplayEvents.GetByIdempotencyKey(context.Background(), firstKey)
	if err != nil {
		t.Fatalf("first backlog event error=%v", err)
	}
	secondEvent, err := fixture.storeA.GameplayEvents.GetByIdempotencyKey(context.Background(), secondKey)
	if err != nil {
		t.Fatalf("second backlog event error=%v", err)
	}
	if firstEvent.SupersededAt.IsZero() || firstEvent.SupersededByEventID != secondEvent.ID {
		t.Fatalf("older backlog event was not superseded: first=%+v newer=%+v", firstEvent, secondEvent)
	}

	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/projection-backlog-supersede"); claimed != 1 {
		t.Fatalf("backlog dispatch claimed=%d", claimed)
	}
	fixture.target.runtime.mu.Lock()
	projected := fixture.target.runtime.knownEntities[fixture.actorCharacter.ID]
	fixture.target.runtime.mu.Unlock()
	if projected.Position != (runtimePoint{X: 44, Z: -11}) || projected.State["projection_version"] != int64(2) {
		t.Fatalf("backlog supersession projected=%+v", projected)
	}

	fixture.serverB.cleanupDeliveredGameplayEvents(context.Background(), time.Now().Add(time.Second))
	if _, err := fixture.storeA.GameplayEvents.GetByIdempotencyKey(context.Background(), firstKey); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("superseded backlog event survived compaction: %v", err)
	}
	assertMetricLine(t, fixture.serverA.observer.renderPrometheus(), `l2bg_region_projection_events_total{result="projection_row_superseded"} 1`)
	assertMetricLine(t, fixture.serverB.observer.renderPrometheus(), `l2bg_region_projection_events_total{result="compacted_obsolete"} 1`)
}

func TestRegionPlayerProjectionBacklogDespawnSupersedesOlderUpsert(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "region_projection_backlog_despawn")
	actorAttached := fixture.serverA.attachedSessionBySessionID(fixture.actor.session.ID)
	if actorAttached == nil {
		t.Fatal("actor attachment missing")
	}

	if produced := fixture.serverA.publishAttachedRegionProjectionNow(context.Background(), actorAttached, regionProjectionActionUpsert, time.Now()); produced != 1 {
		t.Fatalf("initial upsert produced=%d", produced)
	}
	if produced := fixture.serverA.publishAttachedRegionProjectionNow(context.Background(), actorAttached, regionProjectionActionDespawn, time.Now()); produced != 1 {
		t.Fatalf("backlog despawn produced=%d", produced)
	}

	upsertKey := "region-player-projection/" + fixture.actorCharacter.ID + "/" + formatInt64(fixture.actor.session.FencingToken) + "/1/" + fixture.targetCharacter.ID + "/" + formatInt64(fixture.target.session.FencingToken)
	despawnKey := "region-player-projection/" + fixture.actorCharacter.ID + "/" + formatInt64(fixture.actor.session.FencingToken) + "/2/" + fixture.targetCharacter.ID + "/" + formatInt64(fixture.target.session.FencingToken)
	upsertEvent, err := fixture.storeA.GameplayEvents.GetByIdempotencyKey(context.Background(), upsertKey)
	if err != nil {
		t.Fatalf("upsert backlog event error=%v", err)
	}
	despawnEvent, err := fixture.storeA.GameplayEvents.GetByIdempotencyKey(context.Background(), despawnKey)
	if err != nil {
		t.Fatalf("despawn backlog event error=%v", err)
	}
	if upsertEvent.SupersededAt.IsZero() || upsertEvent.SupersededByEventID != despawnEvent.ID {
		t.Fatalf("backlog despawn did not supersede prior upsert: upsert=%+v despawn=%+v", upsertEvent, despawnEvent)
	}

	if claimed := fixture.serverB.dispatchGameplayEventsOnce(context.Background(), "instance-b/projection-backlog-despawn"); claimed != 1 {
		t.Fatalf("backlog despawn dispatch claimed=%d", claimed)
	}
	fixture.target.runtime.mu.Lock()
	_, projected := fixture.target.runtime.knownEntities[fixture.actorCharacter.ID]
	meta := fixture.target.runtime.remotePlayerProjections[fixture.actorCharacter.ID]
	fixture.target.runtime.mu.Unlock()
	if projected || meta.Visible || meta.Version != despawnEvent.ProjectionVersion {
		t.Fatalf("backlog despawn resurrected or missed tombstone: projected=%v meta=%+v", projected, meta)
	}
	if projectionMessagesOfKind(fixture.targetMessageSnapshot(), "entity_appear") != 0 {
		t.Fatalf("backlog despawn created visual success messages=%+v", fixture.targetMessageSnapshot())
	}
}

func TestRegionPlayerProjectionClaimedRowSkipsDeliveryAfterSupersession(t *testing.T) {
	fixture := newCrossInstanceSocialFixture(t, "region_projection_claimed_skip")
	actorAttached := fixture.serverA.attachedSessionBySessionID(fixture.actor.session.ID)
	if actorAttached == nil {
		t.Fatal("actor attachment missing")
	}

	if produced := fixture.serverA.publishAttachedRegionProjectionNow(context.Background(), actorAttached, regionProjectionActionUpsert, time.Now()); produced != 1 {
		t.Fatalf("initial projection produced=%d", produced)
	}
	claimed, err := fixture.storeB.GameplayEvents.Claim(context.Background(), "instance-b", "instance-b/claimed-skip", time.Now(), time.Minute, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("Claim() events=%+v error=%v", claimed, err)
	}
	staleEvent := claimed[0]

	fixture.actor.runtime.mu.Lock()
	fixture.actor.runtime.position = runtimePoint{X: 77, Z: 15}
	fixture.actor.runtime.mu.Unlock()
	if produced := fixture.serverA.publishAttachedRegionProjectionNow(context.Background(), actorAttached, regionProjectionActionUpsert, time.Now()); produced != 1 {
		t.Fatalf("newer projection produced=%d", produced)
	}

	if err := fixture.serverB.deliverGameplayEvent(context.Background(), &staleEvent, "instance-b/claimed-skip"); !errors.Is(err, errGameplayEventSuperseded) {
		t.Fatalf("deliver stale claimed event error=%v", err)
	}
	if _, err := fixture.storeB.GameplayReceipts.GetByEventID(context.Background(), staleEvent.ID); !errors.Is(err, errRecordNotFound) {
		t.Fatalf("stale claimed row reserved a receipt unexpectedly: %v", err)
	}
	assertMetricLine(t, fixture.serverB.observer.renderPrometheus(), `l2bg_region_projection_events_total{result="stale_delivery_skipped"} 1`)
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
	payload.RecipientFencingToken = fixture.target.session.FencingToken
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
