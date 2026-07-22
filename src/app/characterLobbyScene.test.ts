import { describe, expect, it, vi } from 'vitest';
import * as THREE from 'three';

import { triggerSelectedLobbyCharacterInteraction } from './characterLobbyScene';

describe('character lobby scene', () => {
  it('uses model interaction trigger on click for already-selected character after walk-in', () => {
    const group = new THREE.Group();
    const triggerModelInteraction = vi.fn(() => true);

    const started = triggerSelectedLobbyCharacterInteraction(
      {
        group,
        startedAtMs: 100,
      },
      100 + 1500,
      triggerModelInteraction,
    );

    expect(started).toBe(true);
    expect(triggerModelInteraction).toHaveBeenCalledWith(group);
  });

  it('does not trigger selected-character interaction before walk-to-center is complete', () => {
    const group = new THREE.Group();
    const triggerModelInteraction = vi.fn(() => true);

    const started = triggerSelectedLobbyCharacterInteraction(
      {
        group,
        startedAtMs: 100,
      },
      100 + 200,
      triggerModelInteraction,
    );

    expect(started).toBe(false);
    expect(triggerModelInteraction).not.toHaveBeenCalled();
  });
});
