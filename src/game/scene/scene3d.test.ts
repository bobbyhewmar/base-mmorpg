import { describe, expect, it } from 'vitest';

import { createInitialState } from '../data/templates';
import {
  getPlayerNameplateColor,
  getVisiblePlayerNameplates,
  NEUTRAL_NAMEPLATE_COLOR,
  PK_NAMEPLATE_COLOR,
  PVP_NAMEPLATE_COLOR,
} from './scene3d';

describe('scene3d player nameplates', () => {
  it('projects authoritative nameplate colors for neutral, pvp, and pk states', () => {
    expect(getPlayerNameplateColor({ karma: 0, pvpFlagged: false })).toBe(NEUTRAL_NAMEPLATE_COLOR);
    expect(getPlayerNameplateColor({ karma: 0, pvpFlagged: true })).toBe(PVP_NAMEPLATE_COLOR);
    expect(getPlayerNameplateColor({ karma: 25, pvpFlagged: true })).toBe(PK_NAMEPLATE_COLOR);
  });

  it('returns nameplates for the local player and visible other players only', () => {
    const state = createInitialState();
    state.player.name = 'LocalHero';
    state.player.pvpFlagged = true;
    state.player.pvpFlagUntilMs = Date.now() + 5_000;
    state.otherPlayers.char_remote = {
      id: 'char_remote',
      name: 'RemoteRogue',
      race: 'Human',
      baseClass: 'Fighter',
      sex: 'Male',
      hairStyle: 0,
      hairColor: '#6b4e37',
      skinType: 0,
      archetypeId: 'human_fighter_male',
      level: 12,
      cp: 80,
      hp: 100,
      dead: false,
      pvpFlagged: false,
      pvpFlagUntilMs: null,
      pvpKills: 0,
      pkCount: 2,
      karma: 120,
      position: { x: 4, z: 6 },
      facing: 0,
      mountedPetId: null,
    };

    const nameplates = getVisiblePlayerNameplates(state);

    expect(nameplates).toEqual([
      {
        id: state.player.id,
        name: 'LocalHero',
        color: PVP_NAMEPLATE_COLOR,
        position: state.player.position,
      },
      {
        id: 'char_remote',
        name: 'RemoteRogue',
        color: PK_NAMEPLATE_COLOR,
        position: { x: 4, z: 6 },
      },
    ]);
  });

  it('does not derive the local nameplate color from target selection or other local-only context', () => {
    const state = createInitialState();
    state.targetId = 'char_remote';
    state.otherPlayers.char_remote = {
      id: 'char_remote',
      name: 'Enemy',
      race: 'Human',
      baseClass: 'Fighter',
      sex: 'Male',
      hairStyle: 0,
      hairColor: '#6b4e37',
      skinType: 0,
      archetypeId: 'human_fighter_male',
      level: 10,
      cp: 100,
      hp: 100,
      dead: false,
      pvpFlagged: true,
      pvpFlagUntilMs: Date.now() + 8_000,
      pvpKills: 4,
      pkCount: 0,
      karma: 0,
      position: { x: 1, z: 2 },
      facing: 0,
      mountedPetId: null,
    };

    const localNameplate = getVisiblePlayerNameplates(state).find((entry) => entry.id === state.player.id);

    expect(localNameplate).toEqual({
      id: state.player.id,
      name: state.player.name,
      color: NEUTRAL_NAMEPLATE_COLOR,
      position: state.player.position,
    });
  });
});
