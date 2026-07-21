import { describe, expect, it } from 'vitest';

import { createInitialState, createMob, gameTemplates } from '../game/data/templates';
import type { GameState } from '../game/domain/types';
import {
  chatFilterMatches,
  getPlayerFeedbackState,
  getSkillButtonState,
  Hud,
  renderAllianceInviteModal,
  renderClanInviteModal,
  renderClanPanel,
  renderPartyInviteModal,
  shouldBlockSkillDispatch,
} from './hud';

type HudSnapshotAccessor = {
  createSnapshot: (state: GameState) => string;
};

type HudPartyToggleHarness = HudSnapshotAccessor & {
  togglePartyPanel: () => void;
};

const seedHudMob = (state: GameState): void => {
  state.mobs.mob_1 = createMob('mob_1', 'mireling', -108, 0, gameTemplates.mobTemplates.mireling.maxHp);
};

describe('hud skill gating', () => {
  it('keeps the sync state visible without disabling the button after the cooldown end timestamp passes', () => {
    const state = createInitialState();
    seedHudMob(state);
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
    seedHudMob(state);
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
    expect(chatFilterMatches('alliance', 'alliance')).toBe(true);
    expect(chatFilterMatches('alliance', 'party')).toBe(false);
    expect(chatFilterMatches('whisper', 'whisper')).toBe(true);
    expect(chatFilterMatches('whisper', 'region')).toBe(false);
  });

  it('keeps the party window closed by default and toggles it on demand', () => {
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

    let updateCount = 0;
    const hud = Object.assign(Object.create(Hud.prototype), {
      activeCharacterPanel: null,
      activeChatFilter: 'all',
      chatDraft: '',
      chatFocusField: null,
      hotbarShortcutOverrides: new Map(),
      inventoryOpen: false,
      lastSnapshot: '',
      partyWindowOpen: false,
      skillBookTab: 'active',
      store: { getState: () => state },
      update: () => {
        updateCount += 1;
      },
      visibleHotbarRowCount: null,
      whisperTargetDraft: '',
    }) as HudPartyToggleHarness;

    expect(JSON.parse(hud.createSnapshot(state)).partyWindowOpen).toBe(false);
    hud.togglePartyPanel();
    expect(updateCount).toBe(1);
    expect(JSON.parse(hud.createSnapshot(state)).partyWindowOpen).toBe(true);

    hud.togglePartyPanel();
    expect(updateCount).toBe(2);
    expect(JSON.parse(hud.createSnapshot(state)).partyWindowOpen).toBe(false);
  });

  it('renders the dedicated invite modal and disables accept after authoritative expiry time passes', () => {
    const state = createInitialState();
    state.partyInvites = [
      {
        inviteId: 'invite_1',
        partyId: 'party_remote',
        inviterCharacterId: 'char_2',
        inviterName: 'Selene',
        expiresAtMs: 15_000,
      },
    ];

    const activeMarkup = renderPartyInviteModal(state, 10_000);
    const expiredMarkup = renderPartyInviteModal(state, 15_001);

    expect(activeMarkup).toContain('data-party-invite-modal');
    expect(activeMarkup).toContain('Accept');
    expect(activeMarkup).not.toContain('disabled aria-disabled="true"');
    expect(expiredMarkup).toContain('Invitation expired.');
    expect(expiredMarkup).toContain('disabled aria-disabled="true"');
  });

  it('renders clan panel states for no clan, leader and regular member affordances', () => {
    const state = createInitialState();
    const emptyMarkup = renderClanPanel(state, 'Nightfall', '');
    expect(emptyMarkup).toContain('No Clan');
    expect(emptyMarkup).toContain('Create Clan');
    expect(emptyMarkup).toContain('value="Nightfall"');

    state.clan = {
      clanId: 'clan_1',
      name: 'Nightfall',
      leaderCharacterId: state.player.id,
      members: [
        {
          characterId: state.player.id,
          name: state.player.name,
          level: state.player.level,
          baseClass: state.player.baseClass,
          online: true,
          isLeader: true,
        },
        {
          characterId: 'char_2',
          name: 'Selene',
          level: 4,
          baseClass: 'Mage',
          online: true,
          isLeader: false,
        },
      ],
    };
    const leaderMarkup = renderClanPanel(state, '', '');
    expect(leaderMarkup).toContain('Name');
    expect(leaderMarkup).toContain('Lv');
    expect(leaderMarkup).toContain('Cls');
    expect(leaderMarkup).toContain('Status');
    expect(leaderMarkup).toContain('Nightfall');
    expect(leaderMarkup).toContain('Invite');
    expect(leaderMarkup).toContain('Clan Info');
    expect(leaderMarkup).toContain('data-clan-kick="char_2"');
    expect(leaderMarkup).toContain('data-clan-leave');
    expect(leaderMarkup).toContain('data-clan-info-toggle');
    expect(leaderMarkup).not.toContain('data-clan-dissolve');
    expect(leaderMarkup).toContain('Leave</button>');
    expect(leaderMarkup).toContain('disabled aria-disabled="true"');
    expect(leaderMarkup).toContain('Title</button>');
    expect(leaderMarkup).toContain('Privileges</button>');

    state.clan = {
      ...state.clan,
      leaderCharacterId: 'char_2',
      members: state.clan.members.map((member) =>
        member.characterId === state.player.id ? { ...member, isLeader: false } : { ...member, isLeader: true },
      ),
    };
    const memberMarkup = renderClanPanel(state, '', '');
    expect(memberMarkup).toContain('data-clan-leave');
    expect(memberMarkup).toContain('data-clan-invite');
    expect(memberMarkup).not.toContain('data-clan-dissolve');

    const leaderInfoMarkup = renderClanPanel(
      {
        ...state,
        clan: {
          clanId: 'clan_1',
          name: 'Nightfall',
          leaderCharacterId: state.player.id,
          members: [
            {
              characterId: state.player.id,
              name: state.player.name,
              level: state.player.level,
              baseClass: state.player.baseClass,
              online: true,
              isLeader: true,
            },
          ],
        },
      },
      '',
      '',
      true,
    );
    expect(leaderInfoMarkup).toContain('data-clan-info-surface');
    expect(leaderInfoMarkup).toContain('data-clan-dissolve');
    expect(leaderInfoMarkup).toContain('Dissolve Clan');

    const memberInfoMarkup = renderClanPanel(state, '', '', true);
    expect(memberInfoMarkup).toContain('data-clan-info-surface');
    expect(memberInfoMarkup).not.toContain('data-clan-dissolve');
  });

  it('keeps disabled clan buttons visible and exposes dissolve only inside Clan Info for leaders', () => {
    const state = createInitialState();
    state.clan = {
      clanId: 'clan_1',
      name: 'Nightfall',
      leaderCharacterId: state.player.id,
      members: [
        {
          characterId: state.player.id,
          name: state.player.name,
          level: state.player.level,
          baseClass: state.player.baseClass,
          online: true,
          isLeader: true,
        },
      ],
    };
    const defaultMarkup = renderClanPanel(state, '', '');
    const infoMarkup = renderClanPanel(state, '', '', true);

    expect(defaultMarkup).toContain('data-clan-info-toggle');
    expect(defaultMarkup).toContain('disabled aria-disabled="true"');
    expect(defaultMarkup).not.toContain('data-clan-dissolve');
    expect(infoMarkup).toContain('data-clan-info-surface');
    expect(infoMarkup).toContain('data-clan-dissolve');
  });

  it('renders the dedicated clan invite modal above the hotbar and disables accept after authoritative expiry time passes', () => {
    const state = createInitialState();
    state.clanInvites = [
      {
        inviteId: 'clan_invite_1',
        clanId: 'clan_1',
        clanName: 'Nightfall',
        inviterCharacterId: 'char_2',
        inviterName: 'Selene',
        expiresAtMs: 15_000,
      },
    ];

    const activeMarkup = renderClanInviteModal(state, 10_000);
    const expiredMarkup = renderClanInviteModal(state, 15_001);

    expect(activeMarkup).toContain('data-clan-invite-modal');
    expect(activeMarkup).toContain('Nightfall');
    expect(activeMarkup).toContain('Selene invites you to join.');
    expect(activeMarkup).not.toContain('disabled aria-disabled="true"');
    expect(expiredMarkup).toContain('Invitation expired.');
    expect(expiredMarkup).toContain('disabled aria-disabled="true"');
  });

  it('renders alliance affordances inside the clan panel from authoritative state only', () => {
    const state = createInitialState();
    state.clan = {
      clanId: 'clan_1',
      name: 'Nightfall',
      leaderCharacterId: state.player.id,
      members: [
        {
          characterId: state.player.id,
          name: state.player.name,
          level: state.player.level,
          baseClass: state.player.baseClass,
          online: true,
          isLeader: true,
        },
      ],
    };

    const noAllianceMarkup = renderClanPanel(state, '', 'Eclipse');
    expect(noAllianceMarkup).toContain('No Alliance');
    expect(noAllianceMarkup).toContain('Create Alliance');
    expect(noAllianceMarkup).toContain('value="Eclipse"');

    state.alliance = {
      allianceId: 'alliance_1',
      name: 'Eclipse',
      leaderClanId: 'clan_1',
      leaderClanName: 'Nightfall',
      clanCap: 3,
      members: [
        {
          clanId: 'clan_1',
          name: 'Nightfall',
          leaderCharacterId: state.player.id,
          leaderName: state.player.name,
          memberCount: 1,
          isLeaderClan: true,
        },
        {
          clanId: 'clan_2',
          name: 'Dawnbreak',
          leaderCharacterId: 'char_2',
          leaderName: 'Selene',
          memberCount: 2,
          isLeaderClan: false,
        },
      ],
    };

    const allianceMarkup = renderClanPanel(state, '', '');
    expect(allianceMarkup).toContain('Alliance');
    expect(allianceMarkup).toContain('Eclipse');
    expect(allianceMarkup).toContain('Invite Clan');
    expect(allianceMarkup).toContain('data-alliance-expel="clan_2"');
    expect(allianceMarkup).toContain('data-alliance-dissolve');
  });

  it('renders the dedicated alliance invite modal and disables accept after authoritative expiry time passes', () => {
    const state = createInitialState();
    state.allianceInvites = [
      {
        inviteId: 'alliance_invite_1',
        allianceId: 'alliance_1',
        allianceName: 'Eclipse',
        inviterCharacterId: 'char_2',
        inviterName: 'Selene',
        inviterClanId: 'clan_2',
        inviterClanName: 'Dawnbreak',
        targetClanId: 'clan_1',
        expiresAtMs: 15_000,
      },
    ];

    const activeMarkup = renderAllianceInviteModal(state, 10_000);
    const expiredMarkup = renderAllianceInviteModal(state, 15_001);

    expect(activeMarkup).toContain('data-alliance-invite-modal');
    expect(activeMarkup).toContain('Eclipse');
    expect(activeMarkup).toContain('Selene invites your clan to join.');
    expect(activeMarkup).not.toContain('disabled aria-disabled="true"');
    expect(expiredMarkup).toContain('Invitation expired.');
    expect(expiredMarkup).toContain('disabled aria-disabled="true"');
  });
});
