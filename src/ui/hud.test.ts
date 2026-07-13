import { describe, expect, it } from 'vitest';

import { createInitialState } from '../game/data/templates';
import type { GameState } from '../game/domain/types';
import { chatFilterMatches, getPlayerFeedbackState, getSkillButtonState, Hud, shouldBlockSkillDispatch } from './hud';

type HudSnapshotAccessor = {
  createSnapshot: (state: GameState) => string;
};

describe('hud skill gating', () => {
  it('keeps the sync state visible without disabling the button after the cooldown end timestamp passes', () => {
    const state = createInitialState();
    state.targetId = 'mob_1';
    state.player.skillAvailability.crescent_strike = {
      visualRemainingMs: 0,
      requestBlocked: false,
      authorityState: 'cooldown_elapsed_waiting_authority',
    };

    const view = getSkillButtonState(state, 'crescent_strike', true);

    expect(view.disabled).toBe(false);
    expect(view.authorityState).toBe('cooldown_elapsed_waiting_authority');
  });

  it('blocks skill dispatch when semantic cooldown gating is already projected', () => {
    expect(
      shouldBlockSkillDispatch({
        disabled: false,
        dataset: {
          skill: 'crescent_strike',
          skillDisabled: 'true',
        },
      }),
    ).toBe(true);
  });

  it('changes the hud snapshot when authority state changes without changing numeric cooldown', () => {
    const state = createInitialState();
    state.targetId = 'mob_1';
    state.player.cooldowns.crescent_strike = 0;
    state.player.skillAvailability.crescent_strike = {
      visualRemainingMs: 0,
      requestBlocked: true,
      authorityState: 'cooldown_elapsed_waiting_authority',
    };

    const waitingSnapshot = (Hud.prototype as unknown as HudSnapshotAccessor).createSnapshot(state);

    state.player.skillAvailability.crescent_strike = {
      visualRemainingMs: 0,
      requestBlocked: false,
      authorityState: 'ready',
    };

    const readySnapshot = (Hud.prototype as unknown as HudSnapshotAccessor).createSnapshot(state);

    expect(waitingSnapshot).not.toBe(readySnapshot);
  });

  it('returns explicit death feedback only when the player is authoritatively dead', () => {
    const state = createInitialState();
    state.player.deadUntilMs = Number.MAX_SAFE_INTEGER;

    expect(getPlayerFeedbackState(state)).toEqual({
      label: 'Dead',
      message: 'Awaiting authoritative return.',
      tone: 'danger',
      overlay: true,
    });
  });

  it('surfaces authoritative recovery feedback through the player status banner', () => {
    const state = createInitialState();
    state.logs.unshift({
      id: 'log_recovered',
      text: 'You return with restored vitality.',
      tone: 'success',
    });

    expect(getPlayerFeedbackState(state)).toEqual({
      label: 'Recovered',
      message: 'You return with restored vitality.',
      tone: 'success',
      overlay: false,
    });
  });

  it('changes the hud snapshot when party roster or invites change', () => {
    const state = createInitialState();
    const emptySnapshot = (Hud.prototype as unknown as HudSnapshotAccessor).createSnapshot(state);

    state.partyInvites = [
      {
        inviteId: 'invite_1',
        partyId: 'party_remote',
        inviterCharacterId: 'char_2',
        inviterName: 'Selene',
        expiresAtMs: Date.now() + 30000,
      },
    ];
    state.party = {
      partyId: 'party_1',
      leaderCharacterId: state.player.id,
      members: [
        {
          characterId: state.player.id,
          name: state.player.name,
          level: state.player.level,
          baseClass: state.player.baseClass,
          hp: state.player.hp,
          mp: state.player.mp,
          online: true,
          isLeader: true,
        },
      ],
    };

    const partySnapshot = (Hud.prototype as unknown as HudSnapshotAccessor).createSnapshot(state);

    expect(partySnapshot).not.toBe(emptySnapshot);
  });

  it('matches region chat filter against the region channel only', () => {
    expect(chatFilterMatches('region', 'local' as never)).toBe(false);
    expect(chatFilterMatches('region', 'region')).toBe(true);
    expect(chatFilterMatches('region', 'party')).toBe(false);
    expect(chatFilterMatches('whisper', 'whisper')).toBe(true);
    expect(chatFilterMatches('whisper', 'region')).toBe(false);
  });

  it('keeps the party window closed by default and toggles it on demand', () => {
    const container = document.createElement('div');
    const state = createInitialState();
    state.party = {
      partyId: 'party_1',
      leaderCharacterId: state.player.id,
      members: [
        {
          characterId: state.player.id,
          name: state.player.name,
          level: state.player.level,
          baseClass: state.player.baseClass,
          hp: state.player.hp,
          mp: state.player.mp,
          online: true,
          isLeader: true,
        },
      ],
    };

    const hud = new Hud(
      container,
      { getState: () => state } as never,
      { save: () => {}, load: () => {}, reset: () => {} },
      { interactive: false },
    );

    hud.update(state);
    expect(container.querySelector('[data-hud-panel="party"]')).toBeNull();

    hud.togglePartyPanel();
    expect(container.querySelector('[data-hud-panel="party"]')).not.toBeNull();

    hud.togglePartyPanel();
    expect(container.querySelector('[data-hud-panel="party"]')).toBeNull();
  });
});
