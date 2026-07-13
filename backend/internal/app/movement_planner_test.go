package app

import (
	"context"
	"testing"
)

func TestStaticRegionMovementPlannerRoutesAroundCircularBlocker(t *testing.T) {
	planner := &staticRegionMovementPlanner{
		regions: map[string]regionGeodata{
			"obstacle_field": {
				RegionID:   "obstacle_field",
				Version:    "obstacle_field_geo_v1",
				Bounds:     movementBounds{MinX: -8, MaxX: 8, MinZ: -8, MaxZ: 8},
				CellSize:   1,
				PathBudget: 500,
				Obstacles: []movementObstacle{
					circleObstacle(0, 0, 2),
				},
			},
		},
	}

	resolution := planner.Resolve(context.Background(), "obstacle_field", runtimePoint{X: -6, Z: 0}, runtimePoint{X: 6, Z: 0}, movementProfile{
		ActorRadius: defaultMovementActorRadius,
	})

	if resolution.Status != movementPlanStatusAccepted {
		t.Fatalf("expected accepted movement resolution, got %+v", resolution)
	}
	if len(resolution.Plan.Waypoints) <= 1 {
		t.Fatalf("expected alternate route around blocker, got %+v", resolution.Plan.Waypoints)
	}
	if resolution.Plan.AcceptedDestination != (runtimePoint{X: 6, Z: 0}) {
		t.Fatalf("expected original destination to remain accepted, got %+v", resolution.Plan.AcceptedDestination)
	}
	for _, waypoint := range resolution.Plan.Waypoints {
		if mathAbs(waypoint.X) <= 2 && mathAbs(waypoint.Z) <= 2 {
			t.Fatalf("expected path to avoid the blocker footprint, got %+v", resolution.Plan.Waypoints)
		}
	}
}

func TestStaticRegionMovementPlannerRejectsDestinationOutsideBounds(t *testing.T) {
	planner := &staticRegionMovementPlanner{
		regions: map[string]regionGeodata{
			"small_field": {
				RegionID:   "small_field",
				Version:    "small_field_geo_v1",
				Bounds:     movementBounds{MinX: -4, MaxX: 4, MinZ: -4, MaxZ: 4},
				CellSize:   1,
				PathBudget: 200,
			},
		},
	}

	resolution := planner.Resolve(context.Background(), "small_field", runtimePoint{X: 0, Z: 0}, runtimePoint{X: 9, Z: 0}, movementProfile{
		ActorRadius: defaultMovementActorRadius,
	})

	if resolution.Status != movementPlanStatusRejected {
		t.Fatalf("expected rejected movement resolution, got %+v", resolution)
	}
	if resolution.ReasonCode != "movement.destination_out_of_bounds" {
		t.Fatalf("expected movement.destination_out_of_bounds, got %s", resolution.ReasonCode)
	}
}

func TestStaticRegionMovementPlannerRejectsUnreachableDestinationBehindWall(t *testing.T) {
	planner := &staticRegionMovementPlanner{
		regions: map[string]regionGeodata{
			"sealed_field": {
				RegionID:   "sealed_field",
				Version:    "sealed_field_geo_v1",
				Bounds:     movementBounds{MinX: -6, MaxX: 6, MinZ: -3, MaxZ: 3},
				CellSize:   1,
				PathBudget: 200,
				Obstacles: []movementObstacle{
					rectObstacle(-0.5, 0.5, -3, 3),
				},
			},
		},
	}

	resolution := planner.Resolve(context.Background(), "sealed_field", runtimePoint{X: -4, Z: 0}, runtimePoint{X: 4, Z: 0}, movementProfile{
		ActorRadius: defaultMovementActorRadius,
	})

	if resolution.Status != movementPlanStatusRejected {
		t.Fatalf("expected rejected movement resolution, got %+v", resolution)
	}
	if resolution.ReasonCode != "movement.path_unreachable" {
		t.Fatalf("expected movement.path_unreachable, got %s", resolution.ReasonCode)
	}
}

func TestStaticRegionMovementPlannerCancelsWithContext(t *testing.T) {
	planner := &staticRegionMovementPlanner{
		regions: map[string]regionGeodata{
			"cancel_field": {
				RegionID:   "cancel_field",
				Version:    "cancel_field_geo_v1",
				Bounds:     movementBounds{MinX: -20, MaxX: 20, MinZ: -20, MaxZ: 20},
				CellSize:   1,
				PathBudget: 4000,
				Obstacles: []movementObstacle{
					rectObstacle(-1, 1, -20, 20),
				},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolution := planner.Resolve(ctx, "cancel_field", runtimePoint{X: -18, Z: 0}, runtimePoint{X: 18, Z: 0}, movementProfile{
		ActorRadius: defaultMovementActorRadius,
	})

	if resolution.Status != movementPlanStatusCanceled {
		t.Fatalf("expected canceled movement resolution, got %+v", resolution)
	}
}

func mathAbs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
