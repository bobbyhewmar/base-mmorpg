import { describe, expect, it } from 'vitest';
import { selectClickTarget, type ClickHit } from './picking';

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
});
