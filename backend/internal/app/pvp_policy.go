package app

type pvpSafeArea struct {
	MinX float64
	MaxX float64
	MinZ float64
	MaxZ float64
}

type pvpRegionPolicy struct {
	OpenPvP   bool
	SafeAreas []pvpSafeArea
}

var canonicalPvPRegionPolicies = map[string]pvpRegionPolicy{
	startingRegionID: {
		OpenPvP: true,
		SafeAreas: []pvpSafeArea{{
			MinX: -12,
			MaxX: -4,
			MinZ: -4,
			MaxZ: 4,
		}},
	},
	"dawn_plaza": {
		OpenPvP: true,
		SafeAreas: []pvpSafeArea{{
			MinX: -12,
			MaxX: -4,
			MinZ: -4,
			MaxZ: 4,
		}},
	},
}

func pvpPolicyReason(regionID string, actorPosition runtimePoint, targetPosition runtimePoint) string {
	policy, exists := canonicalPvPRegionPolicies[regionID]
	if !exists || !policy.OpenPvP {
		return "pvp.region_restricted"
	}
	for _, safeArea := range policy.SafeAreas {
		if safeArea.contains(actorPosition) || safeArea.contains(targetPosition) {
			return "pvp.safe_zone"
		}
	}
	return ""
}

func (area pvpSafeArea) contains(point runtimePoint) bool {
	return point.X >= area.MinX && point.X <= area.MaxX && point.Z >= area.MinZ && point.Z <= area.MaxZ
}
