import { describe, expect, it } from 'vitest';

import { initialPreGameContext, preGameReducer } from './preGameMachine';

describe('pre-game state machine', () => {
  it('starts directly in the online login flow', () => {
    const next = initialPreGameContext();

    expect(next.mode).toBe('online');
    expect(next.phase).toBe('login');
  });

  it('moves to character list after auth success', () => {
    const pending = initialPreGameContext();
    const next = preGameReducer(pending, {
      type: 'auth_succeeded',
      accessToken: 'access_abc',
      accountId: 'acc_123',
      characters: [
        {
          character_id: 'char_1',
          name: 'Arden',
          race: 'Human',
          base_class: 'Fighter',
          sex: 'Female',
          hair_style: 1,
          hair_color: '#8f5fd3',
          skin_type: 2,
          level: 1,
          last_region_id: 'stonecross_plaza',
          is_enterable: true,
        },
      ],
      catalog: {
        races: [
          {
            race: 'Human',
            enabled: true,
            base_classes: ['Fighter', 'Mage'],
            sex_options: ['Male', 'Female'],
            appearance_options: {
              hair_styles: [0, 1, 2],
              hair_color_default: '#6b4e37',
              skin_types: [0, 1, 2],
            },
          },
        ],
      },
    });

    expect(next.phase).toBe('character_list');
    expect(next.selectedCharacterId).toBe('char_1');
    expect(next.catalog?.races).toHaveLength(1);
  });

  it('opens character creation with the first catalog-backed template selected', () => {
    const authed = preGameReducer(initialPreGameContext(), {
      type: 'auth_succeeded',
      accessToken: 'access_abc',
      accountId: 'acc_123',
      characters: [],
      catalog: {
        races: [
          {
            race: 'Human',
            enabled: true,
            base_classes: ['Fighter', 'Mage'],
            sex_options: ['Male', 'Female'],
            appearance_options: {
              hair_styles: [0, 1, 2],
              hair_color_default: '#6b4e37',
              skin_types: [0, 1, 2],
            },
          },
          {
            race: 'Elf',
            enabled: true,
            base_classes: ['Mage', 'Fighter'],
            sex_options: ['Female', 'Male'],
            appearance_options: {
              hair_styles: [1, 2],
              hair_color_default: '#c5d4dc',
              skin_types: [2],
            },
          },
        ],
      },
    });

    const create = preGameReducer(authed, { type: 'open_create_character' });
    expect(create).toMatchObject({
      phase: 'character_create',
      createRace: 'Human',
      createBaseClass: 'Fighter',
      createSex: 'Male',
      createHairStyle: 0,
      createHairColor: '#6b4e37',
      createSkinType: 0,
    });

    const elf = preGameReducer(create, { type: 'set_create_race', race: 'Elf' });
    expect(elf).toMatchObject({
      createRace: 'Elf',
      createBaseClass: 'Mage',
      createSex: 'Female',
      createHairStyle: 1,
      createHairColor: '#c5d4dc',
      createSkinType: 2,
    });
  });

  it('moves register flow into pending verification when required', () => {
    const register = preGameReducer(initialPreGameContext(), { type: 'open_register' });
    const next = preGameReducer(register, {
      type: 'register_requires_verification',
      login: 'pending-user@example.com',
    });

    expect(next.phase).toBe('pending_verification');
    expect(next.verificationLogin).toBe('pending-user@example.com');
  });

  it('reaches online_ready only after attach success', () => {
    const authed = preGameReducer(initialPreGameContext(), {
      type: 'auth_succeeded',
      accessToken: 'access_abc',
      accountId: 'acc_123',
      characters: [],
      catalog: { races: [] },
    });
    const entering = preGameReducer(authed, {
      type: 'enter_world_succeeded',
      characterId: 'char_1',
      bootstrap: {
        sessionId: 'sess_1',
        attachToken: 'attach_1',
        wsUrl: 'ws://localhost:8080/v1/gameplay/ws',
      },
    });
    const next = preGameReducer(entering, {
      type: 'attach_succeeded',
      regionContext: {
        kind: 'region_context',
        emitted_at_ms: Date.now(),
        region_revision: 1,
        region_id: 'stonecross_plaza',
        geodata_version: 'clean_plain_1024_geo_v1',
        next_command_seq: 1,
        self_position: { x: -8, z: 0 },
        known_entities: [],
      },
    });

    expect(entering.phase).toBe('attaching');
    expect(next.phase).toBe('online_ready');
    expect(next.regionContext?.region_id).toBe('stonecross_plaza');
  });

  it('returns to character list when the active online session closes', () => {
    const ready = preGameReducer(
      preGameReducer(
        preGameReducer(initialPreGameContext(), {
          type: 'auth_succeeded',
          accessToken: 'access_abc',
          accountId: 'acc_123',
          characters: [
            {
              character_id: 'char_1',
              name: 'Arden',
              race: 'Human',
              base_class: 'Fighter',
              sex: 'Female',
              hair_style: 1,
              hair_color: '#8f5fd3',
              skin_type: 2,
              level: 1,
              last_region_id: 'stonecross_plaza',
              is_enterable: true,
            },
          ],
          catalog: { races: [] },
        }),
        {
          type: 'enter_world_succeeded',
          characterId: 'char_1',
          bootstrap: {
            sessionId: 'sess_1',
            attachToken: 'attach_1',
            wsUrl: 'ws://localhost:8080/v1/gameplay/ws',
          },
        },
      ),
      {
        type: 'attach_succeeded',
        regionContext: {
          kind: 'region_context',
          emitted_at_ms: Date.now(),
          region_revision: 1,
          region_id: 'stonecross_plaza',
          geodata_version: 'clean_plain_1024_geo_v1',
          next_command_seq: 1,
          self_position: { x: -8, z: 0 },
          known_entities: [],
        },
      },
    );

    const next = preGameReducer(ready, {
      type: 'online_session_closed',
      message: 'Gameplay session closed. Re-enter the world to continue.',
    });

    expect(next.phase).toBe('character_list');
    expect(next.error).toBe('Gameplay session closed. Re-enter the world to continue.');
    expect(next.accessToken).toBe('access_abc');
    expect(next.sessionBootstrap).toBeNull();
    expect(next.regionContext).toBeNull();
  });
});
