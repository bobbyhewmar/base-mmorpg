import { describe, expect, it } from 'vitest';
import * as THREE from 'three';
import { selectClickTarget, type ClickHit } from './picking';
import {
  clampPointToStonecrossPlayableBounds,
  resolveCameraPositionWithGroundGuard,
  STONECROSS_PLAYABLE_BOUNDS,
} from './scene3d';

describe('selectClickTarget', () => {
  it('prefers overlapping loot over duplicated mob mesh hits', () => {
    const hits: ClickHit[] = [
      { target: { kind: 'mob', id: 'mob_1' }, distance: 36.26 },
      { target: { kind: 'loot', id: 'loot_1' }, distance: 37.59 },
      { target: { kind: 'mob', id: 'mob_1' }, distance: 38.33 },
    ];

    expect(selectClickTarget(hits)).toEqual({ kind: 'loot', id: 'loot_1' });
  });

  it('keeps the nearest non-loot target when loot is clearly farther behind', () => {
    const hits: ClickHit[] = [
      { target: { kind: 'mob', id: 'mob_1' }, distance: 20 },
      { target: { kind: 'loot', id: 'loot_1' }, distance: 24.5 },
    ];

    expect(selectClickTarget(hits)).toEqual({ kind: 'mob', id: 'mob_1' });
  });

  it('returns the first unique target when there is no overlapping loot', () => {
    const hits: ClickHit[] = [
      { target: { kind: 'npc', id: 'npc_gatekeeper' }, distance: 12 },
      { target: { kind: 'npc', id: 'npc_gatekeeper' }, distance: 12.3 },
      { target: { kind: 'mob', id: 'mob_1' }, distance: 13 },
    ];

    expect(selectClickTarget(hits)).toEqual({ kind: 'npc', id: 'npc_gatekeeper' });
  });

  it('ignores stale inactive targets before choosing the click target', () => {
    const hits: ClickHit[] = [
      { target: { kind: 'mob', id: 'mob_dead' }, distance: 8 },
      { target: { kind: 'mob', id: 'mob_alive' }, distance: 9 },
    ];

    expect(
      selectClickTarget(hits, {
        isTargetActive: (target) => target.id !== 'mob_dead',
      }),
    ).toEqual({ kind: 'mob', id: 'mob_alive' });
  });
});

describe('Stonecross terrain picking bounds', () => {
  it('keeps valid 1024x1024 map destinations instead of clamping to the old city corridor', () => {
    expect(clampPointToStonecrossPlayableBounds({ x: 500, z: 500 })).toEqual({ x: 500, z: 500 });
    expect(clampPointToStonecrossPlayableBounds({ x: -500, z: -500 })).toEqual({ x: -500, z: -500 });
  });

  it('clamps only outside the canonical Stonecross playable bounds', () => {
    expect(clampPointToStonecrossPlayableBounds({ x: 900, z: -900 })).toEqual({
      x: STONECROSS_PLAYABLE_BOUNDS.maxX,
      z: STONECROSS_PLAYABLE_BOUNDS.minZ,
    });
  });
});

describe('camera ground guard', () => {
  it('keeps low orbit camera positions above the ground by moving closer to the player', () => {
    const target = new THREE.Vector3(0, 2.1, 0);
    const desired = new THREE.Vector3(18, -12, 18);
    const guarded = resolveCameraPositionWithGroundGuard(target, desired);

    expect(guarded.y).toBeGreaterThan(0);
    expect(guarded.distanceTo(target)).toBeLessThan(desired.distanceTo(target));
  });
});
