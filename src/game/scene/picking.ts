export type ClickTarget = {
  kind: 'mob' | 'loot' | 'npc' | 'player';
  id: string;
};

export type ClickHit = {
  target: ClickTarget;
  distance: number;
};

type ClickSelectionOptions = {
  isTargetActive?: (target: ClickTarget) => boolean;
};

const LOOT_OVERLAP_DISTANCE_EPSILON = 2;

export const selectClickTarget = (hits: ClickHit[], options: ClickSelectionOptions = {}): ClickTarget | null => {
  if (hits.length === 0) {
    return null;
  }

  const uniqueHits: ClickHit[] = [];
  const seenTargets = new Set<string>();
  for (const hit of hits) {
    if (options.isTargetActive && !options.isTargetActive(hit.target)) {
      continue;
    }
    const key = `${hit.target.kind}:${hit.target.id}`;
    if (seenTargets.has(key)) {
      continue;
    }
    seenTargets.add(key);
    uniqueHits.push(hit);
  }

  const primaryHit = uniqueHits[0];
  if (!primaryHit) {
    return null;
  }
  if (primaryHit.target.kind === 'loot') {
    return primaryHit.target;
  }

  // Loot should remain clickable when a respawned mob overlaps the same screen space.
  const overlappingLoot = uniqueHits.find(
    (hit) =>
      hit.target.kind === 'loot' &&
      hit.distance <= primaryHit.distance + LOOT_OVERLAP_DISTANCE_EPSILON,
  );
  if (overlappingLoot) {
    return overlappingLoot.target;
  }

  return primaryHit.target;
};
