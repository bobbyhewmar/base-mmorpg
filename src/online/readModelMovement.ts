import type { Vec2 } from '../game/domain/types';

export type ProjectedPathMode = 'none' | 'pending' | 'authoritative' | 'correction';

export const ONLINE_RECONCILIATION_EXTREME_SNAP_DISTANCE = 16;
export const ONLINE_PREDICTION_PENDING_LEASH_DISTANCE = 2.4;
export const ONLINE_PREDICTION_AUTH_PATH_LEASH_DISTANCE = 5.2;
export const ONLINE_PREDICTION_LEASH_SOFT_ZONE_RATIO = 0.72;
export const REMOTE_PLAYER_INTERPOLATION_MS = 250;
export const REMOTE_PLAYER_SNAP_DISTANCE = 12;
export const REMOTE_PLAYER_SETTLE_EPSILON = 0.05;
export const ONLINE_RECONCILIATION_TARGET_EPSILON = 0.35;
export const ONLINE_RECONCILIATION_SETTLE_EPSILON = 0.08;

export const distance = (left: Vec2, right: Vec2): number =>
  Math.hypot(left.x - right.x, left.z - right.z);

export const moveTowards = (current: Vec2, target: Vec2, step: number): Vec2 => {
  const total = distance(current, target);
  if (total <= step) {
    return { ...target };
  }
  return {
    x: current.x + ((target.x - current.x) / total) * step,
    z: current.z + ((target.z - current.z) / total) * step,
  };
};

export const lerpPoint = (start: Vec2, destination: Vec2, ratio: number): Vec2 => ({
  x: start.x + (destination.x - start.x) * ratio,
  z: start.z + (destination.z - start.z) * ratio,
});

export const shortestAngleDelta = (from: number, to: number): number =>
  Math.atan2(Math.sin(to - from), Math.cos(to - from));

export const lerpAngle = (from: number, to: number, ratio: number): number =>
  from + shortestAngleDelta(from, to) * ratio;

export const closestPointOnSegment = (
  point: Vec2,
  start: Vec2,
  destination: Vec2,
): { point: Vec2; ratio: number } => {
  const dx = destination.x - start.x;
  const dz = destination.z - start.z;
  const lengthSquared = dx * dx + dz * dz;
  if (lengthSquared <= 0.000001) {
    return { point: { ...start }, ratio: 0 };
  }
  const projectedRatio =
    ((point.x - start.x) * dx + (point.z - start.z) * dz) / lengthSquared;
  const ratio = Math.max(0, Math.min(1, projectedRatio));
  return {
    point: {
      x: start.x + dx * ratio,
      z: start.z + dz * ratio,
    },
    ratio,
  };
};

export const cloneVecPath = (points: Vec2[]): Vec2[] =>
  points.map((point) => ({ ...point }));
