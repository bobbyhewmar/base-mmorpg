import { afterEach, describe, expect, it, vi } from 'vitest';

import type { CharacterItemSnapshot, CharacterSummary, RegionContextMessage, SelfStateSnapshot } from './contracts';
import { OnlineReadModel } from './readModel';

const character: CharacterSummary = {
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
};

const regionContext: RegionContextMessage = {
  kind: 'region_context',
  emitted_at_ms: Date.now(),
  region_revision: 1,
  region_id: 'stonecross_plaza',
  geodata_version: 'clean_plain_1024_geo_v1',
  self_position: { x: -8, z: 0 },
  known_entities: [
    {
      entity_id: 'mob_1',
      entity_type: 'mob',
      template_id: 'mireling',
      position: { x: 34, z: 10 },
      state: { hp: 54, alive: true },
    },
  ],
};

const questRegionContext: RegionContextMessage = {
  ...regionContext,
  known_entities: [
    {
      entity_id: 'npc_wardkeeper',
      entity_type: 'npc',
      template_id: 'wardkeeper',
      position: { x: -9, z: 0 },
      state: {},
    },
    ...regionContext.known_entities,
  ],
};

const partyRegionContext: RegionContextMessage = {
  ...regionContext,
  known_entities: [
    ...regionContext.known_entities,
    {
      entity_id: 'char_2',
      entity_type: 'player',
      template_id: 'player_character',
      position: { x: -6, z: 1 },
      state: {
        name: 'Selene',
        race: 'Elf',
        base_class: 'Mage',
        sex: 'Female',
        hair_style: 1,
        hair_color: '#c5a46a',
        skin_type: 2,
        level: 4,
        hp: 98,
        dead: false,
        facing: 0,
      },
    },
  ],
};

const initialItemState: CharacterItemSnapshot = {
  inventory: [
    {
      item_instance_id: 'item_duskgold_start',
      template_id: 'duskgold',
      quantity: 12,
      container_kind: 'inventory',
    },
  ],
  equipment: [],
};

const initialSelfState: SelfStateSnapshot = {
  level: 1,
  xp: 0,
  hp: 122,
  mp: 58,
  dead: false,
  stats: {
    max_hp: 122,
    max_mp: 58,
    attack: 17,
    defense: 9,
    move_speed: 3.225,
  },
};

const advanceProjectionFrames = (model: OnlineReadModel, totalMs: number, stepMs = 16): void => {
  let remaining = totalMs;
  while (remaining > 0) {
    const nextStep = Math.min(stepMs, remaining);
    vi.advanceTimersByTime(nextStep);
    model.snapshot;
    remaining -= nextStep;
  }
};

describe('online read model', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('applies delta only when revision advances monotonically', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'));

    const model = new OnlineReadModel(regionContext, character, initialItemState);
    const command = model.createMoveIntent({ x: 10, z: 4 });
    expect(command).not.toBeNull();
    if (!command) {
      return;
    }

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: command.command_id,
      applies_to_command_seq: command.command_seq,
      self: {
        position: { x: 10, z: 4 },
      },
    });

    advanceProjectionFrames(model, 3000);

    expect(model.snapshot.player.position).toEqual({ x: 10, z: 4 });
    expect(model.getStateInfo().lastRevision).toBe(1);
  });

  it('projects authoritative party state and pending invites from self state and deltas', () => {
    const model = new OnlineReadModel(partyRegionContext, character, initialItemState, {
      ...initialSelfState,
      party: {
        party_id: 'party_1',
        leader_character_id: 'char_1',
        members: [
          {
            character_id: 'char_1',
            name: 'Arden',
            level: 1,
            base_class: 'Fighter',
            hp: 122,
            mp: 58,
            online: true,
            is_leader: true,
          },
        ],
      },
      party_invites: [
        {
          invite_id: 'invite_1',
          party_id: 'party_2',
          inviter_character_id: 'char_2',
          inviter_name: 'Selene',
          expires_at_ms: Date.now() + 30000,
        },
      ],
    });

    expect(model.snapshot.party).toEqual({
      partyId: 'party_1',
      leaderCharacterId: 'char_1',
      members: [
        {
          characterId: 'char_1',
          name: 'Arden',
          level: 1,
          baseClass: 'Fighter',
          hp: 122,
          mp: 58,
          online: true,
          isLeader: true,
        },
      ],
    });
    expect(model.snapshot.partyInvites).toEqual([
      {
        inviteId: 'invite_1',
        partyId: 'party_2',
        inviterCharacterId: 'char_2',
        inviterName: 'Selene',
        expiresAtMs: expect.any(Number),
      },
    ]);

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: '',
      applies_to_command_seq: 0,
      self: {
        party: {
          party_id: 'party_1',
          leader_character_id: 'char_1',
          members: [
            {
              character_id: 'char_1',
              name: 'Arden',
              level: 1,
              base_class: 'Fighter',
              hp: 122,
              mp: 58,
              online: true,
              is_leader: true,
            },
            {
              character_id: 'char_2',
              name: 'Selene',
              level: 4,
              base_class: 'Mage',
              hp: 98,
              mp: 64,
              online: true,
              is_leader: false,
            },
          ],
        },
        party_invites: [],
      },
    });

    expect(model.snapshot.party?.members).toHaveLength(2);
    expect(model.snapshot.partyInvites).toEqual([]);
  });

  it('projects authoritative clan state and pending clan invites from self state and deltas only', () => {
    const model = new OnlineReadModel(partyRegionContext, character, initialItemState, {
      ...initialSelfState,
      clan: {
        clan_id: 'clan_1',
        name: 'Nightfall',
        leader_character_id: 'char_1',
        members: [
          {
            character_id: 'char_1',
            name: 'Arden',
            level: 1,
            base_class: 'Fighter',
            online: true,
            is_leader: true,
          },
        ],
      },
      clan_invites: [
        {
          invite_id: 'clan_invite_1',
          clan_id: 'clan_2',
          clan_name: 'Moonrise',
          inviter_character_id: 'char_2',
          inviter_name: 'Selene',
          expires_at_ms: Date.now() + 30_000,
        },
      ],
    });

    expect(model.snapshot.clan).toEqual({
      clanId: 'clan_1',
      name: 'Nightfall',
      leaderCharacterId: 'char_1',
      members: [
        {
          characterId: 'char_1',
          name: 'Arden',
          level: 1,
          baseClass: 'Fighter',
          online: true,
          isLeader: true,
        },
      ],
    });
    expect(model.snapshot.clanInvites).toEqual([
      {
        inviteId: 'clan_invite_1',
        clanId: 'clan_2',
        clanName: 'Moonrise',
        inviterCharacterId: 'char_2',
        inviterName: 'Selene',
        expiresAtMs: expect.any(Number),
      },
    ]);

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: '',
      applies_to_command_seq: 0,
      self: {
        clan: {
          clan_id: 'clan_1',
          name: 'Nightfall',
          leader_character_id: 'char_1',
          members: [
            {
              character_id: 'char_1',
              name: 'Arden',
              level: 1,
              base_class: 'Fighter',
              online: true,
              is_leader: true,
            },
            {
              character_id: 'char_2',
              name: 'Selene',
              level: 4,
              base_class: 'Mage',
              online: true,
              is_leader: false,
            },
          ],
        },
        clan_invites: [],
      },
    });

    expect(model.snapshot.clan?.members).toHaveLength(2);
    expect(model.snapshot.clanInvites).toEqual([]);
  });

  it('starts moving immediately and shows a pending path preview before any delta arrives', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'));

    const model = new OnlineReadModel(regionContext, character, initialItemState);
    const command = model.createMoveIntent({ x: 10, z: 0 });
    expect(command).not.toBeNull();
    expect(model.snapshot.player.moveTarget).toEqual({ x: 10, z: 0 });
    expect(model.snapshot.destinationMarker).toEqual({ x: 10, z: 0 });

    vi.advanceTimersByTime(200);

    const snapshot = model.snapshot;
    expect(snapshot.player.position.x).toBeGreaterThan(-8);
    expect(snapshot.player.position.x).toBeLessThan(10);
    expect(snapshot.player.position.z).toBe(0);
    expect(snapshot.player.moveTarget).toEqual({ x: 10, z: 0 });
    expect(snapshot.pendingPath).toEqual([
      { x: -8, z: 0 },
      { x: 10, z: 0 },
    ]);
  });

  it('creates party commands from authoritative state and logs party notices', () => {
    const model = new OnlineReadModel(partyRegionContext, character, initialItemState, {
      ...initialSelfState,
      party_invites: [
        {
          invite_id: 'invite_accept_1',
          party_id: 'party_remote',
          inviter_character_id: 'char_2',
          inviter_name: 'Selene',
          expires_at_ms: Date.now() + 30000,
        },
      ],
    });

    (model as any).targetId = 'char_2';

    const inviteCommand = model.createInvitePartyMember();
    expect(inviteCommand?.type).toBe('invite_party_member');
    expect(inviteCommand?.payload).toEqual({});

    const acceptCommand = model.createAcceptPartyInvite('invite_accept_1');
    expect(acceptCommand?.type).toBe('accept_party_invite');

    const slashInviteCommand = model.createPartySlashCommand('/invite');
    expect(slashInviteCommand?.type).toBe('invite_party_member');

    const leaveModel = new OnlineReadModel(partyRegionContext, character, initialItemState, {
      ...initialSelfState,
      party: {
        party_id: 'party_1',
        leader_character_id: character.character_id,
        members: [
          {
            character_id: character.character_id,
            name: character.name,
            level: character.level,
            base_class: character.base_class,
            hp: 100,
            mp: 50,
            online: true,
            is_leader: true,
          },
        ],
      },
    });
    const slashLeaveCommand = leaveModel.createPartySlashCommand('/leave');
    expect(slashLeaveCommand?.type).toBe('leave_party');

    expect(model.createPartySlashCommand('/invite Selene')).toBeNull();
    expect(model.snapshot.logs[0].text).toBe('Party invite failed: /invite currently uses the current player target only.');

    model.applyMessage({
      kind: 'party_notice',
      emitted_at_ms: Date.now(),
      status: 'member_joined',
      party_id: 'party_1',
      message: 'Selene joined the party.',
      command_id: inviteCommand?.command_id,
    });

    expect(model.snapshot.logs[0]).toMatchObject({
      id: expect.stringContaining('log_'),
      text: 'Selene joined the party.',
      tone: 'success',
    });
  });

  it('creates clan commands from authoritative state and keeps local clan truth immutable on semantic reject', () => {
    const model = new OnlineReadModel(partyRegionContext, character, initialItemState, {
      ...initialSelfState,
      clan: {
        clan_id: 'clan_1',
        name: 'Nightfall',
        leader_character_id: character.character_id,
        members: [
          {
            character_id: character.character_id,
            name: character.name,
            level: character.level,
            base_class: character.base_class,
            online: true,
            is_leader: true,
          },
        ],
      },
      clan_invites: [
        {
          invite_id: 'clan_invite_accept_1',
          clan_id: 'clan_remote',
          clan_name: 'Moonrise',
          inviter_character_id: 'char_2',
          inviter_name: 'Selene',
          expires_at_ms: Date.now() + 30_000,
        },
      ],
    });

    (model as any).targetId = 'char_2';

    const inviteCommand = model.createInviteClanMember();
    expect(inviteCommand?.type).toBe('invite_clan_member');
    expect(inviteCommand?.payload).toEqual({});

    const acceptCommand = model.createAcceptClanInvite('clan_invite_accept_1');
    expect(acceptCommand?.type).toBe('accept_clan_invite');

    model.applyMessage({
      kind: 'clan_notice',
      emitted_at_ms: Date.now(),
      status: 'member_joined',
      clan_id: 'clan_1',
      message: 'Selene joined the clan.',
      command_id: inviteCommand?.command_id,
    });
    expect(model.snapshot.logs[0]).toMatchObject({
      text: 'Selene joined the clan.',
      tone: 'success',
      channel: 'system',
    });

    const beforeRejectClan = model.snapshot.clan;
    model.applyMessage({
      kind: 'reject',
      emitted_at_ms: Date.now(),
      command_id: inviteCommand?.command_id,
      reason_code: 'clan.target_already_in_clan',
      message: 'Referenced player is already in a clan.',
    });
    expect(model.snapshot.clan).toEqual(beforeRejectClan);
    expect(model.snapshot.logs[0].text).toBe('Clan invite failed: player is already in a clan.');
  });

  it('allows local player target selection for clan and party affordances without waiting for a server target delta', () => {
    const model = new OnlineReadModel(regionContext, character, undefined, {
      ...initialSelfState,
      clan: {
        clan_id: 'clan_1',
        name: 'Dawnwatch',
        leader_character_id: 'char_1',
        members: [
          {
            character_id: 'char_1',
            name: 'Arden',
            level: 1,
            base_class: 'Fighter',
            online: true,
            is_leader: true,
          },
        ],
      },
    });

    model.applyMessage({
      kind: 'entity_appear',
      emitted_at_ms: Date.now(),
      region_revision: 2,
      entity: {
        entity_id: 'char_other',
        entity_type: 'player',
        template_id: 'player_character',
        position: { x: -4, z: 2 },
        state: {
          name: 'Selene',
          level: 4,
          race: 'Elf',
          base_class: 'Mage',
          sex: 'Female',
          hair_style: 2,
          hair_color: '#3366cc',
          skin_type: 1,
          hp: 118,
          dead: false,
          facing: 0.5,
        },
      },
    });

    const selectPlayer = model.createSelectTarget('char_other');
    expect(selectPlayer).not.toBeNull();
    expect(model.snapshot.targetId).toBeNull();
    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: selectPlayer!.command_id,
      applies_to_command_seq: selectPlayer!.command_seq,
      self: {
        target_id: 'char_other',
      },
    });
    expect(model.snapshot.targetId).toBe('char_other');

    const partyInvite = model.createInvitePartyMember();
    expect(partyInvite?.type).toBe('invite_party_member');
    expect(partyInvite?.payload).toEqual({});

    const clanInvite = model.createInviteClanMember();
    expect(clanInvite?.type).toBe('invite_clan_member');
    expect(clanInvite?.payload).toEqual({});
  });

  it('keeps clan membership unchanged after ack or notice and applies it only from a correlated authoritative delta', () => {
    const model = new OnlineReadModel(partyRegionContext, character, initialItemState, {
      ...initialSelfState,
      clan: null,
      clan_invites: [
        {
          invite_id: 'clan_invite_authoritative_1',
          clan_id: 'clan_authoritative_1',
          clan_name: 'Dawnwatch',
          inviter_character_id: 'char_2',
          inviter_name: 'Selene',
          expires_at_ms: Date.now() + 30_000,
        },
      ],
    });

    const command = model.createAcceptClanInvite('clan_invite_authoritative_1');
    expect(command).not.toBeNull();
    model.applyMessage({
      kind: 'ack',
      emitted_at_ms: Date.now(),
      command_id: command!.command_id,
      command_seq: command!.command_seq,
      status: 'received',
    });
    expect(model.getStateInfo().pendingCommands.find((entry) => entry.commandId === command!.command_id)?.status).toBe(
      'acked',
    );
    expect(model.snapshot.clan).toBeNull();
    expect(model.snapshot.clanInvites).toHaveLength(1);

    model.applyMessage({
      kind: 'clan_notice',
      emitted_at_ms: Date.now(),
      status: 'invite_accepted',
      clan_id: 'clan_authoritative_1',
      invite_id: 'clan_invite_authoritative_1',
      message: 'You join Dawnwatch.',
    });
    expect(model.snapshot.clan).toBeNull();
    expect(model.snapshot.clanInvites).toHaveLength(1);

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: command!.command_id,
      applies_to_command_seq: command!.command_seq,
      self: {
        clan: {
          clan_id: 'clan_authoritative_1',
          name: 'Dawnwatch',
          leader_character_id: 'char_2',
          members: [
            {
              character_id: 'char_2',
              name: 'Selene',
              level: 4,
              base_class: 'Mage',
              online: true,
              is_leader: true,
            },
            {
              character_id: character.character_id,
              name: character.name,
              level: character.level,
              base_class: character.base_class,
              online: true,
              is_leader: false,
            },
          ],
        },
        clan_invites: [],
      },
    });

    expect(model.getStateInfo().pendingCommands.find((entry) => entry.commandId === command!.command_id)?.status).toBe(
      'applied',
    );
    expect(model.snapshot.clan?.clanId).toBe('clan_authoritative_1');
    expect(model.snapshot.clan?.members).toHaveLength(2);
    expect(model.snapshot.clanInvites).toEqual([]);
  });

  it('creates authoritative chat envelopes and projects region plus whisper messages into logs', () => {
    const model = new OnlineReadModel(partyRegionContext, character, initialItemState, {
      ...initialSelfState,
      party: {
        party_id: 'party_1',
        leader_character_id: 'char_1',
        members: [
          {
            character_id: 'char_1',
            name: 'Arden',
            level: 1,
            base_class: 'Fighter',
            hp: 122,
            mp: 58,
            online: true,
            is_leader: true,
          },
          {
            character_id: 'char_2',
            name: 'Selene',
            level: 4,
            base_class: 'Mage',
            hp: 98,
            mp: 64,
            online: true,
            is_leader: false,
          },
        ],
      },
    });

    const regionCommand = model.createSendChatMessage('region', '  Hello   there  ');
    expect(regionCommand?.type).toBe('send_chat_message');
    expect(regionCommand?.payload).toEqual({
      channel: 'region',
      text: 'Hello   there',
    });

    model.applyMessage({
      kind: 'chat_message',
      emitted_at_ms: Date.now(),
      command_id: regionCommand?.command_id,
      command_seq: regionCommand?.command_seq,
      channel: 'region',
      sender_character_id: 'char_1',
      sender_name: 'Arden',
      region_id: 'stonecross_plaza',
      text: 'Hello there',
    });

    expect(model.snapshot.logs[0]).toMatchObject({
      text: '[Region] Arden: Hello there',
      channel: 'region',
    });

    const whisperCommand = model.createSendChatMessage('whisper', 'Meet at gate.', 'Selene');
    expect(whisperCommand?.type).toBe('send_chat_message');
    expect(whisperCommand?.payload).toEqual({
      channel: 'whisper',
      text: 'Meet at gate.',
      target_character_name: 'Selene',
    });

    model.applyMessage({
      kind: 'chat_message',
      emitted_at_ms: Date.now(),
      command_id: whisperCommand?.command_id,
      command_seq: whisperCommand?.command_seq,
      channel: 'whisper',
      sender_character_id: 'char_1',
      sender_name: 'Arden',
      target_character_id: 'char_2',
      target_character_name: 'Selene',
      text: 'Meet at gate.',
    });

    expect(model.snapshot.logs[0]).toMatchObject({
      text: '[Whisper -> Selene] Meet at gate.',
      channel: 'whisper',
    });
  });

  it('deduplicates at-least-once remote chat and social notices by authoritative event id', () => {
    const model = new OnlineReadModel(partyRegionContext, character, initialItemState, initialSelfState);
    const command = model.createSendChatMessage('whisper', 'Remote hello.', 'Selene');
    const logsBeforeAck = model.snapshot.logs.length;
    model.applyMessage({
      kind: 'ack',
      emitted_at_ms: Date.now(),
      command_id: command!.command_id,
      command_seq: command!.command_seq,
      status: 'received',
    });
    expect(model.snapshot.logs).toHaveLength(logsBeforeAck);
    const remoteWhisper = {
      kind: 'chat_message' as const,
      event_id: 42,
      emitted_at_ms: Date.now(),
      channel: 'whisper' as const,
      sender_character_id: 'char_2',
      sender_name: 'Selene',
      target_character_id: character.character_id,
      target_character_name: character.name,
      text: 'Remote hello.',
    };

    expect(model.applyMessage(remoteWhisper)).toEqual({ changed: true });
    const afterFirstWhisper = model.snapshot.logs.length;
    expect(model.applyMessage(remoteWhisper)).toEqual({ changed: false });
    expect(model.snapshot.logs).toHaveLength(afterFirstWhisper);

    const remotePartyNotice = {
      kind: 'party_notice' as const,
      event_id: 43,
      emitted_at_ms: Date.now(),
      status: 'invite_received' as const,
      party_id: 'party_remote',
      invite_id: 'invite_remote',
      message: 'Selene invited you to a party.',
    };
    expect(model.applyMessage(remotePartyNotice)).toEqual({ changed: true });
    const afterFirstNotice = model.snapshot.logs.length;
    expect(model.applyMessage(remotePartyNotice)).toEqual({ changed: false });
    expect(model.snapshot.logs).toHaveLength(afterFirstNotice);
  });

  it('allows region chat while dead and keeps party or whisper prerequisites authoritative', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState, {
      ...initialSelfState,
      hp: 0,
      dead: true,
    });

    const regionEnvelope = model.createSendChatMessage('region', 'Still here.');
    expect(regionEnvelope).not.toBeNull();
    expect(regionEnvelope?.payload).toMatchObject({
      channel: 'region',
      text: 'Still here.',
    });

    expect(model.createSendChatMessage('party', 'Hello party.')).toBeNull();
    expect(model.snapshot.logs[0].text).toBe('Party chat failed: character is not currently in a party.');

    expect(model.createSendChatMessage('whisper', 'Hello whisper.', '   ')).toBeNull();
    expect(model.snapshot.logs[0].text).toBe('Whisper failed: choose an online target character name.');
  });

  it('keeps local prediction inside the leash while waiting for the server route', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'));

    const model = new OnlineReadModel(regionContext, character, initialItemState);
    model.createMoveIntent({ x: 10, z: 0 });

    advanceProjectionFrames(model, 4000);

    const snapshot = model.snapshot;
    expect(snapshot.player.position.x).toBeGreaterThan(-8);
    expect(snapshot.player.position.x).toBeLessThanOrEqual(-5.5);
    expect(snapshot.pendingPath).toEqual([
      { x: -8, z: 0 },
      { x: 10, z: 0 },
    ]);
  });

  it('keeps visual movement smooth after authoritative move delta instead of teleporting immediately', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'));

    const model = new OnlineReadModel(regionContext, character);
    const command = model.createMoveIntent({ x: 4, z: 0 });
    expect(command).not.toBeNull();
    if (!command) {
      return;
    }

    vi.advanceTimersByTime(80);
    const beforeDelta = model.snapshot.player.position.x;

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: command.command_id,
      applies_to_command_seq: command.command_seq,
      self: {
        position: { x: 4, z: 0 },
      },
    });

    const rightAfterDelta = model.snapshot.player.position.x;
    expect(rightAfterDelta).toBe(beforeDelta);
    expect(rightAfterDelta).toBeLessThan(4);

    advanceProjectionFrames(model, 120);
    expect(model.snapshot.player.position.x).toBeGreaterThan(rightAfterDelta);

    advanceProjectionFrames(model, 4000);
    expect(model.snapshot.player.position).toEqual({ x: 4, z: 0 });
    expect(model.snapshot.player.moveTarget).toEqual({ x: 4, z: 0 });
  });

  it('swaps the pending preview for the authoritative path and blends onto it without snapping back', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'));

    const model = new OnlineReadModel(regionContext, character);
    const command = model.createMoveIntent({ x: -4, z: -11 });
    expect(command).not.toBeNull();

    advanceProjectionFrames(model, 180);
    const beforeDelta = model.snapshot.player.position;

    const pendingSnapshot = model.snapshot;
    expect(pendingSnapshot.pendingPath).toEqual([
      { x: -8, z: 0 },
      { x: -4, z: -11 },
    ]);
    expect(pendingSnapshot.authoritativePath).toEqual([]);

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: command?.command_id ?? 'cmd_1',
      applies_to_command_seq: command?.command_seq ?? 1,
      self: {
        position: { x: -8, z: 0 },
        geodata_version: 'clean_plain_1024_geo_v1',
        authoritative_path: [
          { x: -8, z: 0 },
          { x: -8, z: -4 },
          { x: -4, z: -11 },
        ],
      },
    });

    const authoritativeSnapshot = model.snapshot;
    expect(authoritativeSnapshot.pendingPath).toEqual([]);
    expect(authoritativeSnapshot.authoritativePath).toEqual([
      { x: -8, z: 0 },
      { x: -8, z: -4 },
      { x: -4, z: -11 },
    ]);
    expect(authoritativeSnapshot.destinationMarker).toEqual({ x: -4, z: -11 });
    expect(
      Math.hypot(
        authoritativeSnapshot.player.position.x - beforeDelta.x,
        authoritativeSnapshot.player.position.z - beforeDelta.z,
      ),
    ).toBeLessThan(0.35);

    advanceProjectionFrames(model, 320);
    expect(model.snapshot.player.position.z).toBeLessThan(beforeDelta.z);
  });

  it('reconciles position_correction smoothly instead of teleporting for ordinary corrections', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'));

    const model = new OnlineReadModel(regionContext, character);
    const command = model.createMoveIntent({ x: 10, z: 0 });
    expect(command).not.toBeNull();

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: command?.command_id ?? 'cmd_1',
      applies_to_command_seq: command?.command_seq ?? 1,
      self: {
        position: { x: 4, z: 0 },
      },
    });

    advanceProjectionFrames(model, 220);
    const beforeCorrection = model.snapshot.player.position.x;
    expect(beforeCorrection).toBeGreaterThan(-8);

    model.applyMessage({
      kind: 'position_correction',
      emitted_at_ms: Date.now(),
      applies_to_command_seq: command?.command_seq ?? 1,
      position: { x: 6, z: 0 },
      facing: 0,
      reason: 'authoritative_reconciliation',
    });

    const rightAfterCorrection = model.snapshot.player.position.x;
    expect(rightAfterCorrection).toBe(beforeCorrection);
    expect(rightAfterCorrection).toBeLessThan(6);

    advanceProjectionFrames(model, 600);
    const settled = model.snapshot.player.position.x;
    expect(settled).toBeGreaterThan(rightAfterCorrection);
    expect(settled).toBeLessThanOrEqual(6);

    advanceProjectionFrames(model, 4200);
    expect(model.snapshot.player.position).toEqual({ x: 6, z: 0 });
  });

  it('snaps visually only for extreme authoritative corrections', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'));

    const model = new OnlineReadModel(regionContext, character);
    model.createMoveIntent({ x: -2, z: 0 });
    advanceProjectionFrames(model, 100);
    const beforeCorrection = model.snapshot.player.position.x;

    model.applyMessage({
      kind: 'position_correction',
      emitted_at_ms: Date.now(),
      applies_to_command_seq: 1,
      position: { x: 18, z: 0 },
      facing: 0,
      reason: 'authoritative_reconciliation',
    });

    const snapshot = model.snapshot;
    expect(snapshot.player.position).toEqual({ x: 18, z: 0 });
    expect(snapshot.player.moveTarget).toBeNull();
    expect(beforeCorrection).toBeLessThan(18);
  });

  it('ignores delayed or duplicated deltas', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'));

    const model = new OnlineReadModel(regionContext, character);
    const command = model.createMoveIntent({ x: 10, z: 4 });
    expect(command).not.toBeNull();
    if (!command) {
      return;
    }

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: command.command_id,
      applies_to_command_seq: command.command_seq,
      self: {
        position: { x: 10, z: 4 },
      },
    });
    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: command.command_id,
      applies_to_command_seq: command.command_seq,
      self: {
        position: { x: 55, z: 12 },
      },
    });

    advanceProjectionFrames(model, 3000);

    expect(model.snapshot.player.position).toEqual({ x: 10, z: 4 });
    expect(model.getStateInfo().lastRevision).toBe(1);
  });

  it('blocks new commands after revision gap', () => {
    const model = new OnlineReadModel(regionContext, character);

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 2,
      applies_to_command_id: 'cmd_gap',
      applies_to_command_seq: 1,
      self: {
        position: { x: 10, z: 4 },
      },
    });

    const command = model.createMoveIntent({ x: 5, z: 5 });

    expect(command).toBeNull();
    expect(model.getStateInfo().commandFlowBlocked).toBe(true);
    expect(model.getStateInfo().desyncState).toBe('revision_gap');
  });

  it('blocks new commands after region revision gap', () => {
    const model = new OnlineReadModel(regionContext, character);

    model.applyMessage({
      kind: 'entity_appear',
      emitted_at_ms: Date.now(),
      region_revision: 3,
      entity: {
        entity_id: 'mob_3',
        entity_type: 'mob',
        template_id: 'mireling',
        position: { x: 10, z: 4 },
        state: { hp: 54, alive: true },
      },
    });

    const command = model.createSelectTarget('mob_1');

    expect(command).toBeNull();
    expect(model.getStateInfo().commandFlowBlocked).toBe(true);
    expect(model.getStateInfo().desyncState).toBe('region_revision_gap');
  });

  it('interpolates other players between authoritative snapshots and removes them on disappear', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'));

    const model = new OnlineReadModel(regionContext, character);

    model.applyMessage({
      kind: 'entity_appear',
      emitted_at_ms: Date.now(),
      region_revision: 2,
      entity: {
        entity_id: 'char_other',
        entity_type: 'player',
        template_id: 'player_character',
        position: { x: -4, z: 2 },
        state: {
          name: 'Selene',
          level: 4,
          race: 'Elf',
          base_class: 'Mage',
          sex: 'Female',
          hair_style: 2,
          hair_color: '#d8bfd8',
          skin_type: 1,
          hp: 118,
          dead: false,
          facing: 0.5,
        },
      },
    });

    expect(model.snapshot.otherPlayers.char_other).toEqual({
      id: 'char_other',
      name: 'Selene',
      race: 'Elf',
      baseClass: 'Mage',
      sex: 'Female',
      hairStyle: 2,
      hairColor: '#d8bfd8',
      skinType: 1,
      archetypeId: 'ashen_oracle',
      level: 4,
      cp: 0,
      hp: 118,
      dead: false,
      pvpFlagged: false,
      pvpFlagUntilMs: null,
      pvpKills: 0,
      pkCount: 0,
      karma: 0,
      position: { x: -4, z: 2 },
      facing: 0.5,
      mountedPetId: null,
    });

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: '',
      applies_to_command_seq: 0,
      entities: [
        {
          entity_id: 'char_other',
          position: { x: 4, z: -1 },
          facing: 1.2,
          hp: 96,
          dead: false,
          level: 4,
        },
      ],
    });

    const rightAfterDelta = model.snapshot.otherPlayers.char_other;
    expect(rightAfterDelta.position).toEqual({ x: -4, z: 2 });
    expect(rightAfterDelta.facing).toBe(0.5);
    expect(rightAfterDelta.hp).toBe(96);
    expect(rightAfterDelta.level).toBe(4);
    expect(rightAfterDelta.dead).toBe(false);

    vi.advanceTimersByTime(125);

    const midway = model.snapshot.otherPlayers.char_other;
    expect(midway.position.x).toBeGreaterThan(-4);
    expect(midway.position.x).toBeLessThan(4);
    expect(midway.position.z).toBeLessThan(2);
    expect(midway.position.z).toBeGreaterThan(-1);
    expect(midway.facing).toBeGreaterThan(0.5);
    expect(midway.facing).toBeLessThan(1.2);
    expect(midway.hp).toBe(96);

    vi.advanceTimersByTime(200);

    const settled = model.snapshot.otherPlayers.char_other;
    expect(settled.position).toEqual({ x: 4, z: -1 });
    expect(settled.facing).toBe(1.2);
    expect(settled.hp).toBe(96);

    model.applyMessage({
      kind: 'entity_disappear',
      emitted_at_ms: Date.now(),
      region_revision: 3,
      entity_id: 'char_other',
      reason: 'removed',
    });

    expect(model.snapshot.otherPlayers.char_other).toBeUndefined();
  });

  it('snaps remote player interpolation for extreme position divergence', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'));

    const model = new OnlineReadModel(regionContext, character);

    model.applyMessage({
      kind: 'entity_appear',
      emitted_at_ms: Date.now(),
      region_revision: 2,
      entity: {
        entity_id: 'char_far',
        entity_type: 'player',
        template_id: 'player_character',
        position: { x: -4, z: 2 },
        state: {
          name: 'Astra',
          level: 7,
          race: 'Orc',
          base_class: 'Fighter',
          sex: 'Male',
          hair_style: 1,
          hair_color: '#44372d',
          skin_type: 0,
          hp: 134,
          dead: false,
          facing: 0.2,
        },
      },
    });

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: '',
      applies_to_command_seq: 0,
      entities: [
        {
          entity_id: 'char_far',
          position: { x: 26, z: -18 },
          facing: 2.4,
          hp: 120,
          dead: false,
        },
      ],
    });

    const snapshot = model.snapshot.otherPlayers.char_far;
    expect(snapshot.position).toEqual({ x: 26, z: -18 });
    expect(snapshot.facing).toBe(2.4);
    expect(snapshot.hp).toBe(120);
  });

  it('hydrates authoritative known skills and hotbar for the selected class', () => {
    const mageCharacter: CharacterSummary = {
      ...character,
      base_class: 'Mage',
      level: 2,
    };
    const model = new OnlineReadModel(regionContext, mageCharacter, initialItemState, {
      ...initialSelfState,
      level: 2,
      hp: 110,
      mp: 104,
      known_skills: [
        { skill_id: 'ember_shot', category: 'active', unlock_level: 1 },
        { skill_id: 'astral_burst', category: 'active', unlock_level: 2 },
        { skill_id: 'arcane_focus', category: 'passive', unlock_level: 1 },
      ],
      hotbar: {
        open_bar_count: 2,
        slots: [
          { slot_index: 0, entry_type: 'skill', skill_id: 'ember_shot' },
          { slot_index: 1, entry_type: 'skill', skill_id: 'astral_burst' },
        ],
      },
    });

    const snapshot = model.snapshot;

    expect(snapshot.player.baseClass).toBe('Mage');
    expect(snapshot.player.archetypeId).toBe('ashen_oracle');
    expect(snapshot.player.learnedSkills.map((skill) => skill.skillId)).toEqual([
      'ember_shot',
      'astral_burst',
      'arcane_focus',
    ]);
    expect(snapshot.player.hotbar.openBarCount).toBe(2);
    expect(snapshot.player.hotbar.slots[0]?.skillId).toBe('ember_shot');
    expect(snapshot.player.hotbar.slots[1]?.skillId).toBe('astral_burst');
  });

  it('seeds only starter learned skills in the default hotbar projection', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState, initialSelfState);

    expect(model.snapshot.player.hotbar.slots[0]).toMatchObject({
      entryType: 'skill',
      skillId: 'crescent_strike',
    });
    expect(model.snapshot.player.hotbar.slots[1]).toMatchObject({
      entryType: null,
      skillId: null,
    });
  });

  it('projects authoritative quest state and wardkeeper dialog from self snapshots', () => {
    const model = new OnlineReadModel(questRegionContext, character, initialItemState, {
      ...initialSelfState,
      quest: {
        id: 'keeper_request',
        title: 'Keeper of the Gate',
        description: 'Defeat 3 Mirelings beyond the gate, then return to the wardkeeper.',
        status: 'active',
        progress: 1,
        goal: 3,
      },
      npc_interaction: {
        npc_id: 'npc_wardkeeper',
        kind: 'wardkeeper_active',
      },
    });

    const snapshot = model.snapshot;

    expect(snapshot.quest).toEqual({
      id: 'keeper_request',
      title: 'Keeper of the Gate',
      description: 'Defeat 3 Mirelings beyond the gate, then return to the wardkeeper.',
      status: 'active',
      progress: 1,
      goal: 3,
    });
    expect(snapshot.dialog).toEqual({
      npcId: 'npc_wardkeeper',
      title: 'Selka, Wardkeeper of the Plaza',
      body: 'You have driven back 1/3 Mirelings. Finish the cull beyond the gate.',
    });
  });

  it('creates interact_npc envelopes and allows local dialog dismissal without mutating quest authority', () => {
    const model = new OnlineReadModel(questRegionContext, character, initialItemState, {
      ...initialSelfState,
      quest: {
        id: 'keeper_request',
        title: 'Keeper of the Gate',
        description: 'Defeat 3 Mirelings beyond the gate, then return to the wardkeeper.',
        status: 'available',
        progress: 0,
        goal: 3,
      },
      npc_interaction: {
        npc_id: 'npc_wardkeeper',
        kind: 'wardkeeper_available',
      },
    });

    const command = model.createInteractNpc('npc_wardkeeper', 'accept_task');

    expect(command?.type).toBe('interact_npc');
    expect(command?.payload).toEqual({
      npc_id: 'npc_wardkeeper',
      action_id: 'accept_task',
    });
    expect(model.snapshot.dialog?.actionId).toBe('accept_task');

    model.dismissNpcInteraction();

    expect(model.snapshot.dialog).toBeNull();
    expect(model.snapshot.quest.status).toBe('available');
  });

  it('creates use_item envelopes for consumables and only applies healing after authoritative delta', () => {
    const potionItemState: CharacterItemSnapshot = {
      inventory: [
        {
          item_instance_id: 'item_healing_potion_start',
          template_id: 'healing_potion',
          quantity: 3,
          container_kind: 'inventory',
        },
      ],
      equipment: [],
    };
    const model = new OnlineReadModel(regionContext, character, potionItemState, {
      ...initialSelfState,
      hp: 77,
    });

    const command = model.createUseItem('item_healing_potion_start');

    expect(command?.type).toBe('use_item');
    expect(command?.payload).toEqual({
      item_instance_id: 'item_healing_potion_start',
    });
    expect(model.snapshot.player.hp).toBe(77);
    expect(model.snapshot.items.item_healing_potion_start.quantity).toBe(3);

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: command?.command_id ?? 'cmd_use_item_1',
      applies_to_command_seq: command?.command_seq ?? 1,
      inventory: [
        {
          item_instance_id: 'item_healing_potion_start',
          template_id: 'healing_potion',
          quantity: 2,
          container_kind: 'inventory',
        },
      ],
      equipment: [],
      self: {
        hp: 122,
        stats: {
          max_hp: 122,
          max_mp: 58,
          attack: 17,
          defense: 9,
          move_speed: 3.225,
        },
      },
    });

    expect(model.snapshot.player.hp).toBe(122);
    expect(model.snapshot.items.item_healing_potion_start.quantity).toBe(2);
  });

  it('builds authoritative multitype hotbar state commands', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState, initialSelfState);

    const command = model.createSetHotbarState({
      openBarCount: 2,
      slots: [
        { slotIndex: 0, entryType: 'action', skillId: null, itemId: null, actionId: 'basic_attack' },
        { slotIndex: 1, entryType: 'item', skillId: null, itemId: 'item_duskgold_start', actionId: null },
        { slotIndex: 2, entryType: 'skill', skillId: 'crescent_strike', itemId: null, actionId: null },
        { slotIndex: 3, entryType: null, skillId: null, itemId: null, actionId: null },
      ],
    });

    expect(command).not.toBeNull();
    if (!command) {
      return;
    }
    expect(command.type).toBe('set_hotbar_state');
    if (command.type !== 'set_hotbar_state') {
      throw new Error(`unexpected command type ${command.type}`);
    }
    expect(command.payload.open_bar_count).toBe(2);
    expect(command.payload.slots).toHaveLength(36);
    expect(command.payload.slots[0]).toEqual({
      slot_index: 0,
      entry_type: 'action',
      action_id: 'basic_attack',
    });
    expect(command.payload.slots[1]).toEqual({
      slot_index: 1,
      entry_type: 'item',
      item_instance_id: 'item_duskgold_start',
    });
    expect(command.payload.slots[2]).toEqual({
      slot_index: 2,
      entry_type: 'skill',
      skill_id: 'crescent_strike',
    });
    expect(command.payload.slots[3]).toEqual({
      slot_index: 3,
    });
    expect(model.snapshot.player.hotbar.openBarCount).toBe(2);
    expect(model.snapshot.player.hotbar.slots[1]?.itemId).toBe('item_duskgold_start');
  });

  it('preserves authoritative item instance attributes from the initial item snapshot', () => {
    const attributeItemState: CharacterItemSnapshot = {
      inventory: [
        {
          item_instance_id: 'item_gloves_inventory',
          template_id: 'watcher_gloves',
          quantity: 1,
          container_kind: 'inventory',
          instance_attributes: {
            attack: 1,
            defense: 1,
          },
        },
      ],
      equipment: [],
    };
    const model = new OnlineReadModel(regionContext, character, attributeItemState, initialSelfState);

    expect(model.snapshot.items.item_gloves_inventory.instanceAttributes).toEqual({
      attack: 1,
      defense: 1,
    });
  });

  it('blocks passive or unlearned skills before building an online command', () => {
    const model = new OnlineReadModel(regionContext, character);

    expect(model.createUseSkill('iron_will')).toBeNull();
    expect(model.createUseSkill('grave_bloom')).toBeNull();
    expect(model.snapshot.logs[0]?.text).toContain('not learned');
  });

  it('builds a clear target command for the Escape target reset shortcut', () => {
    const model = new OnlineReadModel(regionContext, character);

    const command = model.createClearTarget();

    expect(command).not.toBeNull();
    if (!command) {
      return;
    }
    expect(command.type).toBe('clear_target');
    expect(command.payload).toEqual({});
    const pendingCommands = model.getStateInfo().pendingCommands;
    expect(pendingCommands[pendingCommands.length - 1]).toMatchObject({
      commandId: command.command_id,
      commandSeq: command.command_seq,
      type: 'clear_target',
      status: 'sent',
    });
  });

  it('reflects cooldown only after authoritative delta arrives', () => {
    const model = new OnlineReadModel(regionContext, character);
    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: 'cmd_target',
      applies_to_command_seq: 1,
      self: {
        target_id: 'mob_1',
      },
    });

    const command = model.createUseSkill('crescent_strike');
    expect(command).not.toBeNull();
    expect(model.snapshot.player.cooldowns.crescent_strike ?? 0).toBe(0);
    if (!command) {
      return;
    }

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 2,
      applies_to_command_id: command.command_id,
      applies_to_command_seq: command.command_seq,
      self: {
        target_id: 'mob_1',
        cooldowns: {
          crescent_strike: 900,
        },
      },
      entities: [
        {
          entity_id: 'mob_1',
          hp: 36,
          alive: true,
        },
      ],
    });

    expect(model.snapshot.player.cooldowns.crescent_strike).toBeGreaterThan(0);
    expect(model.snapshot.mobs.mob_1.hp).toBe(36);
  });

  it('hydrates cooldown projection from initial authoritative self state', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'));

    const model = new OnlineReadModel(regionContext, character, initialItemState, {
      ...initialSelfState,
      cooldowns: {
        grave_bloom: 4500,
      },
    });

    expect(model.snapshot.player.cooldowns.grave_bloom).toBeGreaterThan(0);
    expect(model.createUseSkill('grave_bloom')).toBeNull();

    vi.advanceTimersByTime(4600);

    expect(model.snapshot.player.cooldowns.grave_bloom ?? 0).toBe(0);
    expect(model.snapshot.player.skillAvailability.grave_bloom.authorityState).toBe(
      'cooldown_elapsed_waiting_authority',
    );
  });

  it('reflects range reject without mutating hp or cooldown projection', () => {
    const model = new OnlineReadModel(regionContext, character);
    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: 'cmd_target',
      applies_to_command_seq: 1,
      self: {
        target_id: 'mob_1',
      },
    });

    const command = model.createUseSkill('crescent_strike');
    expect(command).not.toBeNull();
    if (!command) {
      return;
    }

    model.applyMessage({
      kind: 'ack',
      emitted_at_ms: Date.now(),
      command_id: command.command_id,
      command_seq: command.command_seq,
      status: 'received',
    });
    model.applyMessage({
      kind: 'reject',
      emitted_at_ms: Date.now(),
      command_id: command.command_id,
      command_seq: command.command_seq,
      reason_code: 'combat.target_out_of_range',
      message: 'Referenced target is outside skill range.',
    });

    const snapshot = model.snapshot;
    expect(snapshot.player.cooldowns.crescent_strike ?? 0).toBe(0);
    expect(snapshot.mobs.mob_1.hp).toBe(54);
    expect(snapshot.logs[0].text).toBe('Skill failed: target is out of range.');
    expect(model.getStateInfo().pendingCommands.find((entry) => entry.commandId === command.command_id)?.status).toBe(
      'rejected',
    );
  });

  it('keeps ack plus reject semantics for range failure', () => {
    const model = new OnlineReadModel(regionContext, character);
    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: 'cmd_target',
      applies_to_command_seq: 1,
      self: {
        target_id: 'mob_1',
      },
    });

    const command = model.createUseSkill('crescent_strike');
    expect(command).not.toBeNull();
    if (!command) {
      return;
    }

    model.applyMessage({
      kind: 'ack',
      emitted_at_ms: Date.now(),
      command_id: command.command_id,
      command_seq: command.command_seq,
      status: 'received',
    });

    const pendingAfterAck = model.getStateInfo().pendingCommands.find((entry) => entry.commandId === command.command_id);
    expect(pendingAfterAck?.status).toBe('acked');

    model.applyMessage({
      kind: 'reject',
      emitted_at_ms: Date.now(),
      command_id: command.command_id,
      command_seq: command.command_seq,
      reason_code: 'combat.target_out_of_range',
      message: 'Referenced target is outside skill range.',
    });

    const pendingAfterReject = model.getStateInfo().pendingCommands.find((entry) => entry.commandId === command.command_id);
    expect(pendingAfterReject?.status).toBe('rejected');
    expect(pendingAfterReject?.reasonCode).toBe('combat.target_out_of_range');
  });

  it('re-enables skill usage once the authoritative cooldown end timestamp has elapsed locally', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'));

    const model = new OnlineReadModel(regionContext, character);
    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: 'cmd_target',
      applies_to_command_seq: 1,
      self: {
        target_id: 'mob_1',
        cooldowns: {
          crescent_strike: 100,
        },
      },
    });

    vi.advanceTimersByTime(120);

    const snapshot = model.snapshot;
    expect(snapshot.player.cooldowns.crescent_strike ?? 0).toBe(0);
    expect(snapshot.player.skillAvailability.crescent_strike.authorityState).toBe(
      'cooldown_elapsed_waiting_authority',
    );
    expect(snapshot.player.skillAvailability.crescent_strike.requestBlocked).toBe(false);
    expect(model.createUseSkill('crescent_strike')).not.toBeNull();
  });

  it('upgrades the projected authority state back to ready after a fresh cooldown snapshot arrives', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'));

    const model = new OnlineReadModel(regionContext, character);
    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: 'cmd_target',
      applies_to_command_seq: 1,
      self: {
        target_id: 'mob_1',
        cooldowns: {
          crescent_strike: 100,
        },
      },
    });

    vi.advanceTimersByTime(120);
    expect(model.snapshot.player.skillAvailability.crescent_strike.authorityState).toBe(
      'cooldown_elapsed_waiting_authority',
    );
    expect(model.snapshot.player.skillAvailability.crescent_strike.requestBlocked).toBe(false);

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 2,
      applies_to_command_id: 'cmd_ready',
      applies_to_command_seq: 2,
      self: {
        target_id: 'mob_1',
        cooldowns: {},
      },
    });

    expect(model.snapshot.player.skillAvailability.crescent_strike.authorityState).toBe('ready');
    expect(model.snapshot.player.skillAvailability.crescent_strike.requestBlocked).toBe(false);
  });

  it('reflects mob death from delta entities and clears the projected target', () => {
    const model = new OnlineReadModel(regionContext, character);
    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: 'cmd_target',
      applies_to_command_seq: 1,
      self: {
        target_id: 'mob_1',
      },
    });

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 2,
      applies_to_command_id: 'cmd_kill',
      applies_to_command_seq: 2,
      self: {
        target_id: null,
        cooldowns: {},
      },
      entities: [
        {
          entity_id: 'mob_1',
          hp: 0,
          alive: false,
        },
      ],
    });

    const snapshot = model.snapshot;
    expect(snapshot.mobs.mob_1.aiState).toBe('dead');
    expect(snapshot.targetId).toBeNull();
    expect(snapshot.floatingTexts).toHaveLength(1);
    expect(snapshot.floatingTexts[0].text).toBe('-54');
  });

  it('does not create use_skill when the projected target is already dead', () => {
    const model = new OnlineReadModel(regionContext, character);
    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: 'cmd_target',
      applies_to_command_seq: 1,
      self: {
        target_id: 'mob_1',
      },
      entities: [
        {
          entity_id: 'mob_1',
          hp: 0,
          alive: false,
        },
      ],
    });

    expect(model.createUseSkill('crescent_strike')).toBeNull();
    expect(model.snapshot.targetId).toBeNull();
  });

  it('creates basic attack and skill commands for a living player target without projecting damage locally', () => {
    const model = new OnlineReadModel(partyRegionContext, character, initialItemState, initialSelfState);
    const select = model.createSelectTarget('char_2');
    expect(select).not.toBeNull();
    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: select!.command_id,
      applies_to_command_seq: select!.command_seq,
      self: { target_id: 'char_2' },
    });

    const before = model.snapshot.otherPlayers.char_2;
    const attack = model.createBasicAttack();
    const skill = model.createUseSkill('crescent_strike');
    expect(attack?.payload).toEqual({ target_id: 'char_2' });
    expect(skill?.payload).toEqual({ skill_id: 'crescent_strike', target_id: 'char_2' });
    expect(model.snapshot.otherPlayers.char_2).toEqual(before);
  });

  it('projects PvP, PK, karma, CP and death only from authoritative snapshot and delta', () => {
    const flagUntil = Date.now() + 30_000;
    const model = new OnlineReadModel(partyRegionContext, character, initialItemState, {
      ...initialSelfState,
      pvp_flagged: true,
      pvp_flag_until_ms: flagUntil,
      pvp_kills: 2,
      pk_count: 1,
      karma: 100,
    });
    expect(model.snapshot.player).toMatchObject({
      pvpFlagged: true,
      pvpFlagUntilMs: flagUntil,
      pvpKills: 2,
      pkCount: 1,
      karma: 100,
    });

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: '',
      applies_to_command_seq: 0,
      self: {
        cp: 0,
        hp: 0,
        dead: true,
        pvp_flagged: false,
        pvp_flag_until_ms: null,
        pvp_kills: 3,
        pk_count: 2,
        karma: 200,
      },
      entities: [
        {
          entity_id: 'char_2',
          cp: 12,
          hp: 50,
          dead: false,
          pvp_flagged: true,
          pvp_flag_until_ms: flagUntil,
          pvp_kills: 4,
          pk_count: 3,
          karma: 300,
        },
      ],
    });

    expect(model.snapshot.player).toMatchObject({
      cp: 0,
      hp: 0,
      pvpFlagged: false,
      pvpFlagUntilMs: null,
      pvpKills: 3,
      pkCount: 2,
      karma: 200,
    });
    expect(model.snapshot.otherPlayers.char_2).toMatchObject({
      cp: 12,
      hp: 50,
      pvpFlagged: true,
      pvpFlagUntilMs: flagUntil,
      pvpKills: 4,
      pkCount: 3,
      karma: 300,
    });
  });

  it('removes entity from the projected known-set on entity_disappear and clears target', () => {
    const model = new OnlineReadModel(regionContext, character);
    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: 'cmd_target',
      applies_to_command_seq: 1,
      self: {
        target_id: 'mob_1',
      },
    });

    model.applyMessage({
      kind: 'entity_disappear',
      emitted_at_ms: Date.now(),
      region_revision: 2,
      entity_id: 'mob_1',
      reason: 'defeated_despawn',
    });

    const snapshot = model.snapshot;
    expect(snapshot.mobs.mob_1).toBeUndefined();
    expect(snapshot.targetId).toBeNull();
  });

  it('reflects respawn through entity_appear after despawn', () => {
    const model = new OnlineReadModel(regionContext, character);
    model.applyMessage({
      kind: 'entity_disappear',
      emitted_at_ms: Date.now(),
      region_revision: 2,
      entity_id: 'mob_1',
      reason: 'defeated_despawn',
    });

    model.applyMessage({
      kind: 'entity_appear',
      emitted_at_ms: Date.now(),
      region_revision: 3,
      entity: {
        entity_id: 'mob_1',
        entity_type: 'mob',
        template_id: 'mireling',
        position: { x: 34, z: 10 },
        state: { hp: 54, alive: true },
      },
    });

    const snapshot = model.snapshot;
    expect(snapshot.mobs.mob_1).toBeDefined();
    expect(snapshot.mobs.mob_1.aiState).toBe('idle');
    expect(snapshot.mobs.mob_1.hp).toBe(54);
  });

  it('reflects loot appear and disappear through authoritative region messages', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState);
    expect(model.snapshot.items.item_duskgold_start.quantity).toBe(12);

    model.applyMessage({
      kind: 'entity_appear',
      emitted_at_ms: Date.now(),
      region_revision: 2,
      entity: {
        entity_id: 'loot_1',
        entity_type: 'loot',
        template_id: 'duskgold',
        position: { x: -6, z: 0 },
        state: { quantity: 4 },
      },
    });

    expect(model.snapshot.loot.loot_1).toBeDefined();
    expect(model.snapshot.loot.loot_1.label).toBe('Duskgold');

    model.applyMessage({
      kind: 'entity_disappear',
      emitted_at_ms: Date.now(),
      region_revision: 3,
      entity_id: 'loot_1',
      reason: 'picked_up',
    });

    expect(model.snapshot.loot.loot_1).toBeUndefined();
    expect(model.snapshot.items.item_duskgold_start.quantity).toBe(12);
  });

  it('updates projected inventory only after authoritative pickup confirmation arrives', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState);
    model.applyMessage({
      kind: 'entity_appear',
      emitted_at_ms: Date.now(),
      region_revision: 2,
      entity: {
        entity_id: 'loot_1',
        entity_type: 'loot',
        template_id: 'duskgold',
        position: { x: -6, z: 0 },
        state: { quantity: 4 },
      },
    });

    const command = model.createPickUpLoot('loot_1');
    expect(command).not.toBeNull();
    expect(model.snapshot.items.item_duskgold_start.quantity).toBe(12);
    if (!command) {
      return;
    }

    model.applyMessage({
      kind: 'ack',
      emitted_at_ms: Date.now(),
      command_id: command.command_id,
      command_seq: command.command_seq,
      status: 'received',
    });
    expect(model.snapshot.items.item_duskgold_start.quantity).toBe(12);

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: command.command_id,
      applies_to_command_seq: command.command_seq,
      self: {
        cooldowns: {},
      },
      inventory: [
        {
          item_instance_id: 'item_duskgold_start',
          template_id: 'duskgold',
          quantity: 16,
          container_kind: 'inventory',
        },
      ],
      equipment: [],
    });

    expect(model.snapshot.items.item_duskgold_start.quantity).toBe(16);
  });

  it('treats visually adjacent loot as reachable for direct pickup', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState);
    model.applyMessage({
      kind: 'entity_appear',
      emitted_at_ms: Date.now(),
      region_revision: 2,
      entity: {
        entity_id: 'loot_adjacent',
        entity_type: 'loot',
        template_id: 'duskgold',
        position: { x: -3.8, z: 0 },
        state: { quantity: 4 },
      },
    });

    const command = model.createPickUpLoot('loot_adjacent');

    expect(command?.type).toBe('pick_up_loot');
  });

  it('targets the nearest visible loot when using the pickup shortcut outside reach', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState);
    model.applyMessage({
      kind: 'entity_appear',
      emitted_at_ms: Date.now(),
      region_revision: 2,
      entity: {
        entity_id: 'loot_visible',
        entity_type: 'loot',
        template_id: 'duskgold',
        position: { x: 2, z: 0 },
        state: { quantity: 4 },
      },
    });

    const command = model.createPickUpNearbyLoot();

    expect(command?.type).toBe('pick_up_loot');
    if (command?.type === 'pick_up_loot') {
      expect(command.payload.loot_id).toBe('loot_visible');
    }
  });

  it('does not mutate projected inventory on loot contention reject', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState);
    model.applyMessage({
      kind: 'entity_appear',
      emitted_at_ms: Date.now(),
      region_revision: 2,
      entity: {
        entity_id: 'loot_1',
        entity_type: 'loot',
        template_id: 'duskgold',
        position: { x: -6, z: 0 },
        state: { quantity: 4 },
      },
    });

    const command = model.createPickUpLoot('loot_1');
    expect(command).not.toBeNull();
    if (!command) {
      return;
    }

    model.applyMessage({
      kind: 'ack',
      emitted_at_ms: Date.now(),
      command_id: command.command_id,
      command_seq: command.command_seq,
      status: 'received',
    });
    model.applyMessage({
      kind: 'reject',
      emitted_at_ms: Date.now(),
      command_id: command.command_id,
      command_seq: command.command_seq,
      reason_code: 'loot.already_collected',
      message: 'Referenced loot was already collected by another actor.',
    });

    expect(model.snapshot.items.item_duskgold_start.quantity).toBe(12);
    expect(model.snapshot.logs[0].text).toBe('Loot failed: item was already collected.');
  });

  it('does not mutate projected inventory on party-loot eligibility reject', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState);
    model.applyMessage({
      kind: 'entity_appear',
      emitted_at_ms: Date.now(),
      region_revision: 2,
      entity: {
        entity_id: 'loot_party_1',
        entity_type: 'loot',
        template_id: 'duskgold',
        position: { x: -6, z: 0 },
        state: { quantity: 4, party_id: 'party_1', eligible_character_ids: ['char_other'] },
      },
    });

    const command = model.createPickUpLoot('loot_party_1');
    expect(command).not.toBeNull();
    if (!command) {
      return;
    }

    model.applyMessage({
      kind: 'ack',
      emitted_at_ms: Date.now(),
      command_id: command.command_id,
      command_seq: command.command_seq,
      status: 'received',
    });
    model.applyMessage({
      kind: 'reject',
      emitted_at_ms: Date.now(),
      command_id: command.command_id,
      command_seq: command.command_seq,
      reason_code: 'loot.party_ineligible',
      message: 'Referenced loot is reserved for a different party reward scope.',
    });

    expect(model.snapshot.items.item_duskgold_start.quantity).toBe(12);
    expect(model.snapshot.logs[0].text).toBe('Loot failed: item is reserved for another eligible party member.');
  });

  it('updates projected equipment only after authoritative equip delta arrives', () => {
    const equipableItemState: CharacterItemSnapshot = {
      inventory: [
        {
          item_instance_id: 'item_weapon_inventory',
          template_id: 'ironwood_spear',
          quantity: 1,
          container_kind: 'inventory',
        },
      ],
      equipment: [],
    };
    const model = new OnlineReadModel(regionContext, character, equipableItemState, initialSelfState);

    const command = model.createEquipItem('item_weapon_inventory');
    expect(command?.type).toBe('equip_item');
    expect(model.snapshot.items.item_weapon_inventory.container).toBe('inventory');

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: command?.command_id ?? 'cmd_1',
      applies_to_command_seq: command?.command_seq ?? 1,
      inventory: [],
      equipment: [
        {
          item_instance_id: 'item_weapon_inventory',
          template_id: 'ironwood_spear',
          quantity: 1,
          container_kind: 'equipment',
          equip_slot: 'weapon',
        },
      ],
      self: {
        stats: {
          max_hp: 122,
          max_mp: 58,
          attack: 27,
          defense: 9,
          move_speed: 3.225,
        },
      },
    });

    expect(model.snapshot.items.item_weapon_inventory.container).toBe('equipment');
    expect(model.snapshot.items.item_weapon_inventory.equipSlot).toBe('weapon');
    expect(model.snapshot.player.authoritativeStats?.attack).toBe(27);
  });

  it('preserves instance attributes when authoritative equipment moves from inventory into a slot', () => {
    const glovesItemState: CharacterItemSnapshot = {
      inventory: [
        {
          item_instance_id: 'item_gloves_inventory',
          template_id: 'watcher_gloves',
          quantity: 1,
          container_kind: 'inventory',
          instance_attributes: {
            attack: 1,
            defense: 1,
          },
        },
      ],
      equipment: [],
    };
    const model = new OnlineReadModel(regionContext, character, glovesItemState, initialSelfState);

    const command = model.createEquipItem('item_gloves_inventory');
    expect(command?.type).toBe('equip_item');

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: command?.command_id ?? 'cmd_gloves_1',
      applies_to_command_seq: command?.command_seq ?? 1,
      inventory: [],
      equipment: [
        {
          item_instance_id: 'item_gloves_inventory',
          template_id: 'watcher_gloves',
          quantity: 1,
          container_kind: 'equipment',
          equip_slot: 'gloves',
          instance_attributes: {
            attack: 1,
            defense: 1,
          },
        },
      ],
      self: {
        stats: {
          max_hp: 150,
          max_mp: 58,
          attack: 32,
          defense: 20,
          move_speed: 3.225,
        },
      },
    });

    expect(model.snapshot.items.item_gloves_inventory.container).toBe('equipment');
    expect(model.snapshot.items.item_gloves_inventory.equipSlot).toBe('gloves');
    expect(model.snapshot.items.item_gloves_inventory.instanceAttributes).toEqual({
      attack: 1,
      defense: 1,
    });
    expect(model.snapshot.player.authoritativeStats).toMatchObject({
      attack: 32,
      defense: 20,
    });
  });

  it('updates projected player hp only after authoritative self delta arrives', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState, initialSelfState);

    expect(model.snapshot.player.hp).toBe(122);

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: 'cmd_1',
      applies_to_command_seq: 1,
      self: {
        hp: 120,
        dead: false,
        stats: {
          max_hp: 122,
          max_mp: 58,
          attack: 17,
          defense: 9,
          move_speed: 3.225,
        },
      },
      entities: [
        {
          entity_id: 'mob_1',
          hp: 36,
          alive: true,
        },
      ],
    });

    expect(model.snapshot.player.hp).toBe(120);
    expect(model.snapshot.player.authoritativeStats?.defense).toBe(9);
  });

  it('projects authoritative mp, xp, and level progression from self deltas', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState, initialSelfState);

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: 'cmd_progression',
      applies_to_command_seq: 1,
      self: {
        level: 2,
        xp: 12,
        hp: 140,
        mp: 65,
        dead: false,
        stats: {
          max_hp: 140,
          max_mp: 65,
          attack: 21,
          defense: 11,
          move_speed: 3.225,
        },
      },
    });

    const snapshot = model.snapshot;
    expect(snapshot.player.level).toBe(2);
    expect(snapshot.player.xp).toBe(12);
    expect(snapshot.player.hp).toBe(140);
    expect(snapshot.player.mp).toBe(65);
    expect(snapshot.logs[0].text).toBe('You reach level 2. Vitality surges back to full.');
  });

  it('reflects authoritative death state and blocks incompatible commands locally', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState, initialSelfState);

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: 'cmd_1',
      applies_to_command_seq: 1,
      self: {
        hp: 0,
        dead: true,
        stats: {
          max_hp: 122,
          max_mp: 58,
          attack: 17,
          defense: 9,
          move_speed: 3.225,
        },
      },
    });

    expect(model.snapshot.player.hp).toBe(0);
    expect(model.snapshot.player.deadUntilMs).not.toBeNull();
    expect(model.createMoveIntent({ x: 0, z: 0 })).toBeNull();
    expect(model.snapshot.logs[0].text).toBe('Actor is currently dead.');
  });

  it('reflects authoritative respawn state only after delta arrives', () => {
    const deadSelfState: SelfStateSnapshot = {
      hp: 0,
      dead: true,
      stats: {
        max_hp: 122,
        max_mp: 58,
        attack: 17,
        defense: 9,
        move_speed: 3.225,
      },
    };
    const model = new OnlineReadModel(regionContext, character, initialItemState, deadSelfState);

    expect(model.snapshot.player.hp).toBe(0);
    expect(model.snapshot.player.deadUntilMs).not.toBeNull();

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: '',
      applies_to_command_seq: 0,
      self: {
        hp: 122,
        dead: false,
        position: { x: -8, z: 0 },
        stats: {
          max_hp: 122,
          max_mp: 58,
          attack: 17,
          defense: 9,
          move_speed: 3.225,
        },
      },
    });

    expect(model.snapshot.player.hp).toBe(122);
    expect(model.snapshot.player.deadUntilMs).toBeNull();
  });

  it('does not mutate projected equipment on semantic equip reject', () => {
    const equipableItemState: CharacterItemSnapshot = {
      inventory: [
        {
          item_instance_id: 'item_weapon_inventory',
          template_id: 'ironwood_spear',
          quantity: 1,
          container_kind: 'inventory',
        },
      ],
      equipment: [],
    };
    const model = new OnlineReadModel(regionContext, character, equipableItemState, initialSelfState);

    const command = model.createEquipItem('item_weapon_inventory');
    expect(command?.type).toBe('equip_item');

    model.applyMessage({
      kind: 'reject',
      emitted_at_ms: Date.now(),
      command_id: command?.command_id ?? 'cmd_1',
      command_seq: command?.command_seq ?? 1,
      reason_code: 'inventory.item_not_equippable',
      message: 'Referenced item cannot be equipped.',
    });

    expect(model.snapshot.items.item_weapon_inventory.container).toBe('inventory');
    expect(model.snapshot.logs[0].text).toBe('Equipment failed: item cannot be equipped.');
    expect(model.snapshot.player.authoritativeStats?.attack).toBe(17);
  });

  it('rejects invalid stack split quantities before emitting a command', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState, initialSelfState);

    const command = model.createSplitItemStack('item_duskgold_start', 12);

    expect(command).toBeNull();
    expect(model.snapshot.items.item_duskgold_start.quantity).toBe(12);
    expect(model.snapshot.logs[0].text).toBe('Stack split failed: quantity is not valid for that stack.');
  });

  it('reconciles authoritative split and merge stack deltas without optimistic inventory mutation', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState, initialSelfState);

    const splitCommand = model.createSplitItemStack('item_duskgold_start', 1);
    expect(splitCommand?.type).toBe('split_item_stack');
    expect(model.snapshot.items.item_duskgold_start.quantity).toBe(12);

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: splitCommand?.command_id ?? 'cmd_split_1',
      applies_to_command_seq: splitCommand?.command_seq ?? 1,
      inventory: [
        {
          item_instance_id: 'item_duskgold_start',
          template_id: 'duskgold',
          quantity: 11,
          container_kind: 'inventory',
        },
        {
          item_instance_id: 'item_duskgold_split_1',
          template_id: 'duskgold',
          quantity: 1,
          container_kind: 'inventory',
        },
      ],
      equipment: [],
    });

    expect(model.snapshot.items.item_duskgold_start.quantity).toBe(11);
    expect(model.snapshot.items.item_duskgold_split_1.quantity).toBe(1);

    const mergeCommand = model.createMergeItemStacks('item_duskgold_split_1', 'item_duskgold_start');
    expect(mergeCommand?.type).toBe('merge_item_stacks');
    expect(model.snapshot.items.item_duskgold_split_1.quantity).toBe(1);

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 2,
      applies_to_command_id: mergeCommand?.command_id ?? 'cmd_merge_1',
      applies_to_command_seq: mergeCommand?.command_seq ?? 2,
      inventory: [
        {
          item_instance_id: 'item_duskgold_start',
          template_id: 'duskgold',
          quantity: 12,
          container_kind: 'inventory',
        },
      ],
      equipment: [],
    });

    expect(model.snapshot.items.item_duskgold_start.quantity).toBe(12);
    expect(model.snapshot.items.item_duskgold_split_1).toBeUndefined();
  });

  it('creates buy_item envelopes with only offer id and quantity for vendor purchases', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState, initialSelfState);

    const command = model.createBuyItem('merchant_spear_offer', 1);

    expect(command).not.toBeNull();
    expect(command?.type).toBe('buy_item');
    expect(command?.payload).toEqual({
      vendor_offer_id: 'merchant_spear_offer',
      quantity: 1,
    });
  });

  it('creates exchange_item envelopes with only offer id and quantity for fixed exchanges', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState, initialSelfState);

    const command = model.createExchangeItem('merchant_mantle_exchange', 1);

    expect(command).not.toBeNull();
    expect(command?.type).toBe('exchange_item');
    expect(command?.payload).toEqual({
      exchange_offer_id: 'merchant_mantle_exchange',
      quantity: 1,
    });
  });

  it('creates player trade envelopes using only target id, item id, and quantity', () => {
    const tradeRegionContext: RegionContextMessage = {
      ...regionContext,
      known_entities: [
        ...regionContext.known_entities,
        {
          entity_id: 'char_peer',
          entity_type: 'player',
          template_id: 'player_character',
          position: { x: -7, z: 0 },
          state: {
            name: 'Peer',
            level: 1,
            race: 'Dark Elf',
            base_class: 'Mage',
            sex: 'Female',
            hair_style: 2,
            hair_color: '#f4f4ff',
            skin_type: 2,
            hp: 92,
            dead: false,
            facing: 0,
          },
        },
      ],
    };
    const model = new OnlineReadModel(tradeRegionContext, character, initialItemState, initialSelfState);

    const offerCommand = model.createOfferTradeItem('char_peer', 'item_duskgold_start', 1);

    expect(offerCommand?.type).toBe('offer_trade_item');
    expect(offerCommand?.payload).toEqual({
      target_character_id: 'char_peer',
      item_instance_id: 'item_duskgold_start',
      quantity: 1,
    });
  });

  it('creates deposit_item and withdraw_item envelopes using only item id and quantity', () => {
    const warehouseItemState: CharacterItemSnapshot = {
      inventory: [
        {
          item_instance_id: 'item_duskgold_start',
          template_id: 'duskgold',
          quantity: 12,
          container_kind: 'inventory',
        },
      ],
      equipment: [],
      warehouse: [
        {
          item_instance_id: 'item_duskgold_warehouse',
          template_id: 'duskgold',
          quantity: 2,
          container_kind: 'warehouse',
        },
      ],
    };
    const model = new OnlineReadModel(regionContext, character, warehouseItemState, initialSelfState);

    const depositCommand = model.createDepositItem('item_duskgold_start', 1);
    const withdrawCommand = model.createWithdrawItem('item_duskgold_warehouse', 1);

    expect(depositCommand?.type).toBe('deposit_item');
    expect(depositCommand?.payload).toEqual({
      item_instance_id: 'item_duskgold_start',
      quantity: 1,
    });
    expect(withdrawCommand?.type).toBe('withdraw_item');
    expect(withdrawCommand?.payload).toEqual({
      item_instance_id: 'item_duskgold_warehouse',
      quantity: 1,
    });
    expect(model.snapshot.items.item_duskgold_warehouse.container).toBe('warehouse');
  });

  it('creates sell_item envelopes using only item id and quantity for vendor sales', () => {
    const sellableItemState: CharacterItemSnapshot = {
      inventory: [
        {
          item_instance_id: 'item_weapon_inventory',
          template_id: 'ironwood_spear',
          quantity: 1,
          container_kind: 'inventory',
        },
      ],
      equipment: [],
    };
    const model = new OnlineReadModel(regionContext, character, sellableItemState, initialSelfState);

    const command = model.createSellItem('item_weapon_inventory', 1);

    expect(command?.type).toBe('sell_item');
    expect(command?.payload).toEqual({
      item_instance_id: 'item_weapon_inventory',
      quantity: 1,
    });
  });

  it('maps exchange rejects into stable player-facing messages', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState, initialSelfState);

    const command = model.createExchangeItem('merchant_mantle_exchange', 1);
    expect(command?.type).toBe('exchange_item');

    model.applyMessage({
      kind: 'reject',
      emitted_at_ms: Date.now(),
      command_id: command?.command_id ?? 'cmd_exchange_1',
      command_seq: command?.command_seq ?? 1,
      reason_code: 'economy.exchange_insufficient_materials',
      message: 'Actor lacks the required materials for this exchange.',
    });

    expect(model.snapshot.logs[0].text).toBe('Exchange failed: required materials are insufficient.');
  });

  it('projects incoming and resolved player trade notices into the snapshot', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState, initialSelfState);

    model.applyMessage({
      kind: 'trade_notice',
      emitted_at_ms: Date.now(),
      status: 'pending',
      direction: 'incoming',
      offer_id: 'trade_1',
      counterparty_character_id: 'char_peer',
      counterparty_name: 'Peer',
      item_template_id: 'duskgold',
      quantity: 1,
      message: 'Peer sent you a trade offer.',
    });

    expect(model.snapshot.incomingTradeOffer).toEqual({
      offerId: 'trade_1',
      direction: 'incoming',
      counterpartyCharacterId: 'char_peer',
      counterpartyName: 'Peer',
      itemTemplateId: 'duskgold',
      quantity: 1,
    });

    const acceptCommand = model.createAcceptTradeOffer('trade_1');
    expect(acceptCommand?.type).toBe('accept_trade_offer');
    expect(acceptCommand?.payload).toEqual({ trade_offer_id: 'trade_1' });

    model.applyMessage({
      kind: 'trade_notice',
      emitted_at_ms: Date.now(),
      status: 'accepted',
      direction: 'incoming',
      offer_id: 'trade_1',
      counterparty_character_id: 'char_peer',
      counterparty_name: 'Peer',
      item_template_id: 'duskgold',
      quantity: 1,
      message: 'Trade accepted.',
    });

    expect(model.snapshot.incomingTradeOffer).toBeNull();
    expect(model.snapshot.logs[0].text).toBe('Trade accepted.');
  });

  it('maps player-trade rejects into stable player-facing messages', () => {
    const model = new OnlineReadModel(regionContext, character, initialItemState, initialSelfState);

    model.applyMessage({
      kind: 'reject',
      emitted_at_ms: Date.now(),
      command_id: 'cmd_trade_1',
      command_seq: 1,
      reason_code: 'trade.target_busy',
      message: 'Referenced player already has a pending trade offer.',
    });

    expect(model.snapshot.logs[0].text).toBe('Trade offer failed: that player already has a pending trade.');
  });

  it('updates projected defense only after authoritative equipment delta arrives', () => {
    const chestItemState: CharacterItemSnapshot = {
      inventory: [
        {
          item_instance_id: 'item_chest_inventory',
          template_id: 'wardkeeper_mantle',
          quantity: 1,
          container_kind: 'inventory',
        },
      ],
      equipment: [],
    };
    const model = new OnlineReadModel(regionContext, character, chestItemState, initialSelfState);

    const command = model.createEquipItem('item_chest_inventory');
    expect(command?.type).toBe('equip_item');
    expect(model.snapshot.player.authoritativeStats?.defense).toBe(9);

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: command?.command_id ?? 'cmd_1',
      applies_to_command_seq: command?.command_seq ?? 1,
      inventory: [],
      equipment: [
        {
          item_instance_id: 'item_chest_inventory',
          template_id: 'wardkeeper_mantle',
          quantity: 1,
          container_kind: 'equipment',
          equip_slot: 'chest',
        },
      ],
      self: {
        stats: {
          max_hp: 142,
          max_mp: 58,
          attack: 17,
          defense: 15,
          move_speed: 3.225,
        },
      },
    });

    expect(model.snapshot.player.authoritativeStats?.maxHp).toBe(142);
    expect(model.snapshot.player.authoritativeStats?.defense).toBe(15);
  });

  it('projects owned pets and companion entities from authoritative self and region snapshots', () => {
    const petRegionContext: RegionContextMessage = {
      ...regionContext,
      known_entities: [
        ...regionContext.known_entities,
        {
          entity_id: 'pet_1',
          entity_type: 'pet',
          template_id: 'mireling',
          position: { x: -8, z: 0 },
          state: {
            name: 'Mireling Strider',
            owner_id: 'char_1',
            owner_name: 'Arden',
            pet_template_id: 'mireling_strider',
            visual_template: 'mireling',
            kind: 'pet_mount',
            mount_eligible: true,
            mounted: false,
            follow_owner_id: 'char_1',
          },
        },
      ],
    };
    const petSelfState: SelfStateSnapshot = {
      ...initialSelfState,
      pets: [
        {
          pet_instance_id: 'pet_1',
          pet_template_id: 'mireling_strider',
          name: 'Mireling Strider',
          kind: 'pet_mount',
          visual_template_id: 'mireling',
          mount_eligible: true,
          summoned: true,
          mounted: false,
        },
      ],
    };

    const model = new OnlineReadModel(petRegionContext, character, initialItemState, petSelfState);
    const snapshot = model.snapshot;

    expect(snapshot.player.pets).toEqual([
      {
        petInstanceId: 'pet_1',
        petTemplateId: 'mireling_strider',
        name: 'Mireling Strider',
        kind: 'pet_mount',
        visualTemplateId: 'mireling',
        mountEligible: true,
        summoned: true,
        mounted: false,
      },
    ]);
    expect(snapshot.player.activePetId).toBe('pet_1');
    expect(snapshot.player.mountedPetId).toBeNull();
    expect(snapshot.companions.pet_1.ownerId).toBe('char_1');
    expect(snapshot.companions.pet_1.visualTemplateId).toBe('mireling');
    expect(snapshot.companions.pet_1.position.x).toBeCloseTo(-9.8, 3);
    expect(snapshot.companions.pet_1.position.z).toBeCloseTo(-0.75, 3);
  });

  it('creates authoritative tame, summon, dismiss, mount, and dismount commands for pets', () => {
    const unsummonedPetState: SelfStateSnapshot = {
      ...initialSelfState,
      pets: [
        {
          pet_instance_id: 'pet_1',
          pet_template_id: 'mireling_strider',
          name: 'Mireling Strider',
          kind: 'pet_mount',
          visual_template_id: 'mireling',
          mount_eligible: true,
          summoned: false,
          mounted: false,
        },
      ],
    };
    const model = new OnlineReadModel(regionContext, character, initialItemState, unsummonedPetState);

    const tame = model.createTameMob('mob_1');
    expect(tame?.type).toBe('tame_mob');
    expect(tame?.payload).toEqual({ target_id: 'mob_1' });

    const summon = model.createSummonPet();
    expect(summon?.type).toBe('summon_pet');
    expect(summon?.payload).toEqual({});

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: summon?.command_id ?? 'cmd_pet_summon',
      applies_to_command_seq: summon?.command_seq ?? 1,
      self: {
        pets: [
          {
            pet_instance_id: 'pet_1',
            pet_template_id: 'mireling_strider',
            name: 'Mireling Strider',
            kind: 'pet_mount',
            visual_template_id: 'mireling',
            mount_eligible: true,
            summoned: true,
            mounted: false,
          },
        ],
      },
    });

    const mount = model.createMountPet();
    expect(mount?.type).toBe('mount_pet');
    expect(mount?.payload).toEqual({});

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 2,
      applies_to_command_id: mount?.command_id ?? 'cmd_pet_mount',
      applies_to_command_seq: mount?.command_seq ?? 2,
      self: {
        pets: [
          {
            pet_instance_id: 'pet_1',
            pet_template_id: 'mireling_strider',
            name: 'Mireling Strider',
            kind: 'pet_mount',
            visual_template_id: 'mireling',
            mount_eligible: true,
            summoned: true,
            mounted: true,
          },
        ],
        stats: {
          max_hp: 122,
          max_mp: 58,
          attack: 17,
          defense: 9,
          move_speed: 4.05,
        },
      },
    });

    const dismount = model.createDismountPet();
    expect(dismount?.type).toBe('dismount_pet');
    expect(dismount?.payload).toEqual({});

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 3,
      applies_to_command_id: dismount?.command_id ?? 'cmd_pet_dismount',
      applies_to_command_seq: dismount?.command_seq ?? 3,
      self: {
        pets: [
          {
            pet_instance_id: 'pet_1',
            pet_template_id: 'mireling_strider',
            name: 'Mireling Strider',
            kind: 'pet_mount',
            visual_template_id: 'mireling',
            mount_eligible: true,
            summoned: true,
            mounted: false,
          },
        ],
        stats: {
          max_hp: 122,
          max_mp: 58,
          attack: 17,
          defense: 9,
          move_speed: 3.225,
        },
      },
    });

    const dismiss = model.createDismissPet();
    expect(dismiss?.type).toBe('dismiss_pet');
    expect(dismiss?.payload).toEqual({});
  });

  it('logs pet summon and mount transitions and projects mounted move speed authoritatively', () => {
    const initialPetState: SelfStateSnapshot = {
      ...initialSelfState,
      pets: [
        {
          pet_instance_id: 'pet_1',
          pet_template_id: 'mireling_strider',
          name: 'Mireling Strider',
          kind: 'pet_mount',
          visual_template_id: 'mireling',
          mount_eligible: true,
          summoned: false,
          mounted: false,
        },
      ],
    };
    const model = new OnlineReadModel(regionContext, character, initialItemState, initialPetState);

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 1,
      applies_to_command_id: 'cmd_pet_state_1',
      applies_to_command_seq: 1,
      self: {
        pets: [
          {
            pet_instance_id: 'pet_1',
            pet_template_id: 'mireling_strider',
            name: 'Mireling Strider',
            kind: 'pet_mount',
            visual_template_id: 'mireling',
            mount_eligible: true,
            summoned: true,
            mounted: false,
          },
        ],
      },
    });

    expect(model.snapshot.logs[0].text).toBe('Mireling Strider answers the summon.');

    model.applyMessage({
      kind: 'delta',
      emitted_at_ms: Date.now(),
      revision: 2,
      applies_to_command_id: 'cmd_pet_state_2',
      applies_to_command_seq: 2,
      self: {
        pets: [
          {
            pet_instance_id: 'pet_1',
            pet_template_id: 'mireling_strider',
            name: 'Mireling Strider',
            kind: 'pet_mount',
            visual_template_id: 'mireling',
            mount_eligible: true,
            summoned: true,
            mounted: true,
          },
        ],
        stats: {
          max_hp: 122,
          max_mp: 58,
          attack: 17,
          defense: 9,
          move_speed: 4.05,
        },
      },
    });

    expect(model.snapshot.logs[0].text).toBe('You mount Mireling Strider.');
    expect(model.snapshot.player.mountedPetId).toBe('pet_1');
    expect(model.snapshot.player.authoritativeStats?.moveSpeed).toBe(4.05);
  });
});
