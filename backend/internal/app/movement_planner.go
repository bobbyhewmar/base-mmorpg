package app

import (
	"context"
	"math"
	"sort"
)

const (
	defaultMovementActorRadius      = 0.75
	defaultMovementGridCellSize     = 1.0
	defaultMovementPathVisitBudget  = 6000
	defaultMovementLineSampleStride = 0.35
)

type movementPlanStatus string

const (
	movementPlanStatusAccepted movementPlanStatus = "accepted"
	movementPlanStatusRejected movementPlanStatus = "rejected"
	movementPlanStatusCanceled movementPlanStatus = "canceled"
)

type movementProfile struct {
	ActorRadius float64
}

type movementPlan struct {
	GeodataVersion      string
	AcceptedDestination runtimePoint
	Waypoints           []runtimePoint
}

type movementResolution struct {
	Status           movementPlanStatus
	Plan             movementPlan
	ReasonCode       string
	CorrectionReason string
}

type movementPlanner interface {
	GeodataVersion(regionID string) string
	Resolve(ctx context.Context, regionID string, start runtimePoint, destination runtimePoint, profile movementProfile) movementResolution
}

type regionGeodata struct {
	RegionID    string
	Version     string
	Bounds      movementBounds
	CellSize    float64
	PathBudget  int
	Obstacles   []movementObstacle
}

type movementBounds struct {
	MinX float64
	MaxX float64
	MinZ float64
	MaxZ float64
}

type movementObstacle struct {
	Kind    string
	CenterX float64
	CenterZ float64
	Radius  float64
	MinX    float64
	MaxX    float64
	MinZ    float64
	MaxZ    float64
}

type staticRegionMovementPlanner struct {
	regions map[string]regionGeodata
}

type movementGridCoord struct {
	X int
	Z int
}

type movementGrid struct {
	bounds     movementBounds
	cellSize   float64
	width      int
	height     int
	blocked    []bool
	actorRadius float64
}

var defaultMovementPlanner movementPlanner = newStaticRegionMovementPlanner()

func newStaticRegionMovementPlanner() movementPlanner {
	return &staticRegionMovementPlanner{
		regions: map[string]regionGeodata{
			"dawn_plaza": {
				RegionID:   "dawn_plaza",
				Version:    "dawn_plaza_geo_v1",
				Bounds:     movementBounds{MinX: -18, MaxX: 97, MinZ: -16, MaxZ: 16},
				CellSize:   defaultMovementGridCellSize,
				PathBudget: defaultMovementPathVisitBudget,
				Obstacles: []movementObstacle{
					circleObstacle(-6, -6, 2.2),
					circleObstacle(18, -4.8, 1.35),
					circleObstacle(18, 4.8, 1.35),
					rectObstacle(-12.4, -7.6, 6.2, 10.2),
					circleObstacle(62, -10, 1.4),
					circleObstacle(69, 10, 1.4),
					circleObstacle(82, -11, 1.4),
					circleObstacle(89, 8, 1.4),
				},
			},
		},
	}
}

func circleObstacle(centerX, centerZ, radius float64) movementObstacle {
	return movementObstacle{
		Kind:    "circle",
		CenterX: centerX,
		CenterZ: centerZ,
		Radius:  radius,
	}
}

func rectObstacle(minX, maxX, minZ, maxZ float64) movementObstacle {
	return movementObstacle{
		Kind: "rect",
		MinX: minX,
		MaxX: maxX,
		MinZ: minZ,
		MaxZ: maxZ,
	}
}

func (planner *staticRegionMovementPlanner) GeodataVersion(regionID string) string {
	region, ok := planner.regions[regionID]
	if !ok {
		return "unavailable"
	}
	return region.Version
}

func (planner *staticRegionMovementPlanner) Resolve(ctx context.Context, regionID string, start runtimePoint, destination runtimePoint, profile movementProfile) movementResolution {
	if ctx != nil && ctx.Err() != nil {
		return movementResolution{Status: movementPlanStatusCanceled}
	}

	region, ok := planner.regions[regionID]
	if !ok {
		return movementResolution{
			Status:           movementPlanStatusRejected,
			ReasonCode:       "movement.geodata_unavailable",
			CorrectionReason: "geodata_mismatch",
		}
	}

	actorRadius := profile.ActorRadius
	if actorRadius <= 0 {
		actorRadius = defaultMovementActorRadius
	}

	if !region.walkablePoint(start, actorRadius) {
		return movementResolution{
			Status:           movementPlanStatusRejected,
			ReasonCode:       "movement.geodata_mismatch",
			CorrectionReason: "geodata_mismatch",
		}
	}
	if !region.inBounds(destination, actorRadius) {
		return movementResolution{
			Status:           movementPlanStatusRejected,
			ReasonCode:       "movement.destination_out_of_bounds",
			CorrectionReason: "path_unreachable",
		}
	}
	if !region.walkablePoint(destination, actorRadius) {
		return movementResolution{
			Status:           movementPlanStatusRejected,
			ReasonCode:       "movement.destination_blocked",
			CorrectionReason: "path_blocked",
		}
	}
	if lineWalkable(start, destination, region, actorRadius) {
		return movementResolution{
			Status: movementPlanStatusAccepted,
			Plan: movementPlan{
				GeodataVersion:      region.Version,
				AcceptedDestination: destination,
				Waypoints:           []runtimePoint{destination},
			},
		}
	}

	grid := newMovementGrid(region, actorRadius)
	startCoord, ok := grid.nearestWalkableCoord(start)
	if !ok {
		return movementResolution{
			Status:           movementPlanStatusRejected,
			ReasonCode:       "movement.geodata_mismatch",
			CorrectionReason: "geodata_mismatch",
		}
	}
	destinationCoord, ok := grid.nearestWalkableCoord(destination)
	if !ok {
		return movementResolution{
			Status:           movementPlanStatusRejected,
			ReasonCode:       "movement.path_unreachable",
			CorrectionReason: "path_unreachable",
		}
	}

	coords, reasonCode, canceled := grid.findPath(ctx, startCoord, destinationCoord, region.PathBudget)
	if canceled {
		return movementResolution{Status: movementPlanStatusCanceled}
	}
	if reasonCode != "" {
		correctionReason := "path_unreachable"
		if reasonCode == "movement.path_budget_exceeded" {
			correctionReason = "path_blocked"
		}
		return movementResolution{
			Status:           movementPlanStatusRejected,
			ReasonCode:       reasonCode,
			CorrectionReason: correctionReason,
		}
	}

	waypoints := make([]runtimePoint, 0, len(coords))
	for _, coord := range coords[1:] {
		waypoints = append(waypoints, grid.coordPoint(coord))
	}
	if len(waypoints) == 0 || distance(waypoints[len(waypoints)-1], destination) > 0.001 {
		lastPoint := start
		if len(waypoints) > 0 {
			lastPoint = waypoints[len(waypoints)-1]
		}
		if lineWalkable(lastPoint, destination, region, actorRadius) {
			waypoints = append(waypoints, destination)
		}
	}
	if len(waypoints) == 0 {
		waypoints = append(waypoints, destination)
	}

	return movementResolution{
		Status: movementPlanStatusAccepted,
		Plan: movementPlan{
			GeodataVersion:      region.Version,
			AcceptedDestination: waypoints[len(waypoints)-1],
			Waypoints:           waypoints,
		},
	}
}

func (region regionGeodata) inBounds(point runtimePoint, actorRadius float64) bool {
	return point.X >= region.Bounds.MinX+actorRadius &&
		point.X <= region.Bounds.MaxX-actorRadius &&
		point.Z >= region.Bounds.MinZ+actorRadius &&
		point.Z <= region.Bounds.MaxZ-actorRadius
}

func (region regionGeodata) walkablePoint(point runtimePoint, actorRadius float64) bool {
	if !region.inBounds(point, actorRadius) {
		return false
	}
	for _, obstacle := range region.Obstacles {
		if obstacle.blocks(point, actorRadius) {
			return false
		}
	}
	return true
}

func (obstacle movementObstacle) blocks(point runtimePoint, actorRadius float64) bool {
	switch obstacle.Kind {
	case "circle":
		return math.Hypot(point.X-obstacle.CenterX, point.Z-obstacle.CenterZ) <= obstacle.Radius+actorRadius
	case "rect":
		return point.X >= obstacle.MinX-actorRadius &&
			point.X <= obstacle.MaxX+actorRadius &&
			point.Z >= obstacle.MinZ-actorRadius &&
			point.Z <= obstacle.MaxZ+actorRadius
	default:
		return false
	}
}

func newMovementGrid(region regionGeodata, actorRadius float64) movementGrid {
	cellSize := region.CellSize
	if cellSize <= 0 {
		cellSize = defaultMovementGridCellSize
	}
	width := int(math.Round((region.Bounds.MaxX-region.Bounds.MinX)/cellSize)) + 1
	height := int(math.Round((region.Bounds.MaxZ-region.Bounds.MinZ)/cellSize)) + 1
	grid := movementGrid{
		bounds:      region.Bounds,
		cellSize:    cellSize,
		width:       width,
		height:      height,
		blocked:     make([]bool, width*height),
		actorRadius: actorRadius,
	}
	for z := 0; z < height; z++ {
		for x := 0; x < width; x++ {
			point := grid.coordPoint(movementGridCoord{X: x, Z: z})
			if !region.walkablePoint(point, actorRadius) {
				grid.blocked[grid.index(x, z)] = true
			}
		}
	}
	return grid
}

func (grid movementGrid) index(x, z int) int {
	return z*grid.width + x
}

func (grid movementGrid) coordPoint(coord movementGridCoord) runtimePoint {
	return runtimePoint{
		X: grid.bounds.MinX + float64(coord.X)*grid.cellSize,
		Z: grid.bounds.MinZ + float64(coord.Z)*grid.cellSize,
	}
}

func (grid movementGrid) pointToCoord(point runtimePoint) (movementGridCoord, bool) {
	if point.X < grid.bounds.MinX || point.X > grid.bounds.MaxX || point.Z < grid.bounds.MinZ || point.Z > grid.bounds.MaxZ {
		return movementGridCoord{}, false
	}
	x := int(math.Round((point.X - grid.bounds.MinX) / grid.cellSize))
	z := int(math.Round((point.Z - grid.bounds.MinZ) / grid.cellSize))
	if x < 0 || x >= grid.width || z < 0 || z >= grid.height {
		return movementGridCoord{}, false
	}
	return movementGridCoord{X: x, Z: z}, true
}

func (grid movementGrid) walkableCoord(coord movementGridCoord) bool {
	if coord.X < 0 || coord.X >= grid.width || coord.Z < 0 || coord.Z >= grid.height {
		return false
	}
	return !grid.blocked[grid.index(coord.X, coord.Z)]
}

func (grid movementGrid) nearestWalkableCoord(point runtimePoint) (movementGridCoord, bool) {
	start, ok := grid.pointToCoord(point)
	if !ok {
		return movementGridCoord{}, false
	}
	if grid.walkableCoord(start) {
		return start, true
	}

	candidates := make([]movementGridCoord, 0, 32)
	for radius := 1; radius <= 4; radius++ {
		candidates = candidates[:0]
		for dz := -radius; dz <= radius; dz++ {
			for dx := -radius; dx <= radius; dx++ {
				if maxAbs(dx, dz) != radius {
					continue
				}
				coord := movementGridCoord{X: start.X + dx, Z: start.Z + dz}
				if !grid.walkableCoord(coord) {
					continue
				}
				candidates = append(candidates, coord)
			}
		}
		sort.Slice(candidates, func(i, j int) bool {
			leftDistance := math.Hypot(float64(candidates[i].X-start.X), float64(candidates[i].Z-start.Z))
			rightDistance := math.Hypot(float64(candidates[j].X-start.X), float64(candidates[j].Z-start.Z))
			if leftDistance == rightDistance {
				if candidates[i].Z == candidates[j].Z {
					return candidates[i].X < candidates[j].X
				}
				return candidates[i].Z < candidates[j].Z
			}
			return leftDistance < rightDistance
		})
		if len(candidates) > 0 {
			return candidates[0], true
		}
	}
	return movementGridCoord{}, false
}

func (grid movementGrid) findPath(ctx context.Context, start, destination movementGridCoord, budget int) ([]movementGridCoord, string, bool) {
	if start == destination {
		return []movementGridCoord{start}, "", false
	}
	if budget <= 0 {
		budget = defaultMovementPathVisitBudget
	}

	openSet := map[movementGridCoord]struct{}{start: {}}
	cameFrom := map[movementGridCoord]movementGridCoord{}
	gScore := map[movementGridCoord]float64{start: 0}
	visited := 0

	for len(openSet) > 0 {
		if ctx != nil && ctx.Err() != nil {
			return nil, "", true
		}
		current := selectBestOpenCoord(openSet, gScore, destination)
		if current == destination {
			return reconstructCoordPath(cameFrom, current), "", false
		}

		delete(openSet, current)
		visited++
		if visited > budget {
			return nil, "movement.path_budget_exceeded", false
		}

		for _, neighbor := range grid.neighbors(current) {
			stepCost := movementStepCost(current, neighbor)
			tentativeScore := gScore[current] + stepCost
			existingScore, seen := gScore[neighbor]
			if seen && tentativeScore >= existingScore {
				continue
			}
			cameFrom[neighbor] = current
			gScore[neighbor] = tentativeScore
			openSet[neighbor] = struct{}{}
		}
	}

	return nil, "movement.path_unreachable", false
}

func (grid movementGrid) neighbors(coord movementGridCoord) []movementGridCoord {
	directions := []movementGridCoord{
		{X: 1, Z: 0},
		{X: 0, Z: 1},
		{X: -1, Z: 0},
		{X: 0, Z: -1},
		{X: 1, Z: 1},
		{X: -1, Z: 1},
		{X: -1, Z: -1},
		{X: 1, Z: -1},
	}

	result := make([]movementGridCoord, 0, len(directions))
	for _, direction := range directions {
		next := movementGridCoord{X: coord.X + direction.X, Z: coord.Z + direction.Z}
		if !grid.walkableCoord(next) {
			continue
		}
		if direction.X != 0 && direction.Z != 0 {
			orthogonalA := movementGridCoord{X: coord.X + direction.X, Z: coord.Z}
			orthogonalB := movementGridCoord{X: coord.X, Z: coord.Z + direction.Z}
			if !grid.walkableCoord(orthogonalA) || !grid.walkableCoord(orthogonalB) {
				continue
			}
		}
		result = append(result, next)
	}
	return result
}

func reconstructCoordPath(cameFrom map[movementGridCoord]movementGridCoord, current movementGridCoord) []movementGridCoord {
	path := []movementGridCoord{current}
	for {
		parent, ok := cameFrom[current]
		if !ok {
			break
		}
		current = parent
		path = append(path, current)
	}
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	return path
}

func selectBestOpenCoord(openSet map[movementGridCoord]struct{}, gScore map[movementGridCoord]float64, destination movementGridCoord) movementGridCoord {
	var best movementGridCoord
	bestSet := false
	bestF := 0.0
	bestH := 0.0
	for candidate := range openSet {
		g := gScore[candidate]
		h := movementHeuristic(candidate, destination)
		f := g + h
		if !bestSet ||
			f < bestF ||
			(f == bestF && h < bestH) ||
			(f == bestF && h == bestH && candidate.Z < best.Z) ||
			(f == bestF && h == bestH && candidate.Z == best.Z && candidate.X < best.X) {
			best = candidate
			bestF = f
			bestH = h
			bestSet = true
		}
	}
	return best
}

func movementHeuristic(left, right movementGridCoord) float64 {
	return math.Hypot(float64(right.X-left.X), float64(right.Z-left.Z))
}

func movementStepCost(left, right movementGridCoord) float64 {
	if left.X != right.X && left.Z != right.Z {
		return math.Sqrt2
	}
	return 1
}

func lineWalkable(start, destination runtimePoint, region regionGeodata, actorRadius float64) bool {
	length := distance(start, destination)
	if length <= 0.001 {
		return region.walkablePoint(destination, actorRadius)
	}
	steps := int(math.Ceil(length / defaultMovementLineSampleStride))
	for index := 0; index <= steps; index++ {
		ratio := float64(index) / float64(steps)
		point := runtimePoint{
			X: start.X + (destination.X-start.X)*ratio,
			Z: start.Z + (destination.Z-start.Z)*ratio,
		}
		if !region.walkablePoint(point, actorRadius) {
			return false
		}
	}
	return true
}

func maxAbs(left, right int) int {
	leftAbs := int(math.Abs(float64(left)))
	rightAbs := int(math.Abs(float64(right)))
	if leftAbs > rightAbs {
		return leftAbs
	}
	return rightAbs
}
