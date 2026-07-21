import {
  getLearnedSkillsForCharacter,
  normalizeHotbarState,
} from '../game/data/templates';
import { isCanonicalBaseClass } from '../game/data/characterClasses';
import type {
  AllianceMemberClanState,
  AllianceState,
  BaseClass,
  ClanState,
  DerivedStats,
  GameState,
  HotbarActionId,
  ItemInstance,
  OwnedPetState,
  PartyState,
  PendingAllianceInviteState,
  PendingClanInviteState,
  PendingPartyInviteState,
  PlayerHotbarSlot,
  PlayerHotbarState,
  PlayerKnownSkill,
} from '../game/domain/types';
import type {
  AllianceInviteSnapshot,
  AllianceSnapshot,
  ClanInviteSnapshot,
  ClanSnapshot,
  CharacterItemRecord,
  CharacterItemSnapshot,
  HotbarSlotSnapshot,
  PartyInviteSnapshot,
  PartySnapshot,
  PetSnapshot,
} from './contracts';

export type OnlineNpcInteraction = {
  npcId: string;
  kind:
    | 'merchant_services'
    | 'warehouse_services'
    | 'wardkeeper_available'
    | 'wardkeeper_active'
    | 'wardkeeper_ready_to_turn_in'
    | 'wardkeeper_completed';
};

const isHotbarActionId = (value: unknown): value is HotbarActionId =>
  value === 'basic_attack' ||
  value === 'pick_up_nearby' ||
  value === 'party_invite' ||
  value === 'party_leave' ||
  value === 'tame_target' ||
  value === 'summon_pet' ||
  value === 'dismiss_pet' ||
  value === 'mount_pet' ||
  value === 'dismount_pet' ||
  value === 'toggle_walk_run';

const isBaseClass = isCanonicalBaseClass;

export const parseMovementMode = (value: unknown): 'run' | 'walk' =>
  value === 'walk' ? 'walk' : 'run';

export const toItemInstanceAttributes = (
  item: CharacterItemRecord,
): ItemInstance['instanceAttributes'] => {
  if (!item.instance_attributes) {
    return undefined;
  }

  const instanceAttributes: NonNullable<ItemInstance['instanceAttributes']> = {};
  if (typeof item.instance_attributes.max_cp === 'number') {
    instanceAttributes.maxCp = item.instance_attributes.max_cp;
  }
  if (typeof item.instance_attributes.max_hp === 'number') {
    instanceAttributes.maxHp = item.instance_attributes.max_hp;
  }
  if (typeof item.instance_attributes.max_mp === 'number') {
    instanceAttributes.maxMp = item.instance_attributes.max_mp;
  }
  if (typeof item.instance_attributes.attack === 'number') {
    instanceAttributes.attack = item.instance_attributes.attack;
  }
  if (typeof item.instance_attributes.defense === 'number') {
    instanceAttributes.defense = item.instance_attributes.defense;
  }
  if (typeof item.instance_attributes.move_speed === 'number') {
    instanceAttributes.moveSpeed = item.instance_attributes.move_speed;
  }

  return Object.keys(instanceAttributes).length > 0 ? instanceAttributes : undefined;
};

export const toAuthoritativeItems = (
  itemState?: CharacterItemSnapshot | null,
): Record<string, ItemInstance> => {
  const items: Record<string, ItemInstance> = {};
  for (const item of [
    ...(itemState?.inventory ?? []),
    ...(itemState?.equipment ?? []),
    ...(itemState?.warehouse ?? []),
  ]) {
    items[item.item_instance_id] = {
      id: item.item_instance_id,
      templateId: item.template_id,
      quantity: item.quantity,
      container: item.container_kind,
      equipSlot: item.equip_slot,
      instanceAttributes: toItemInstanceAttributes(item),
    };
  }
  return items;
};

export const snapshotFromDelta = (
  inventory?: CharacterItemRecord[],
  equipment?: CharacterItemRecord[],
  warehouse?: CharacterItemRecord[],
): CharacterItemSnapshot | null => {
  if (!inventory && !equipment && !warehouse) {
    return null;
  }
  return {
    inventory: inventory ?? [],
    equipment: equipment ?? [],
    warehouse: warehouse ?? [],
  };
};

export const parseAuthoritativeStats = (value: unknown): DerivedStats | null => {
  if (!value || typeof value !== 'object') {
    return null;
  }
  const candidate = value as Record<string, unknown>;
  if (
    typeof candidate.max_hp !== 'number' ||
    typeof candidate.max_mp !== 'number' ||
    typeof candidate.attack !== 'number' ||
    typeof candidate.defense !== 'number' ||
    typeof candidate.move_speed !== 'number'
  ) {
    return null;
  }
  return {
    maxCp:
      typeof candidate.max_cp === 'number'
        ? candidate.max_cp
        : Math.max(1, Math.round(candidate.max_hp * 0.65)),
    maxHp: candidate.max_hp,
    maxMp: candidate.max_mp,
    attack: candidate.attack,
    defense: candidate.defense,
    moveSpeed: candidate.move_speed,
  };
};

export const parseAuthoritativeHP = (value: unknown): number | null =>
  typeof value === 'number' ? value : null;

export const parseAuthoritativeMP = (value: unknown): number | null =>
  typeof value === 'number' ? value : null;

export const parseAuthoritativeCP = (value: unknown): number | null =>
  typeof value === 'number' ? value : null;

export const parseAuthoritativeXP = (value: unknown): number | null =>
  typeof value === 'number' ? value : null;

export const parseAuthoritativeLevel = (value: unknown): number | null =>
  typeof value === 'number' ? value : null;

export const parseAuthoritativeDead = (value: unknown): boolean | null =>
  typeof value === 'boolean' ? value : null;

export const parseKnownSkills = (
  value: unknown,
  baseClass: BaseClass,
  level: number,
): PlayerKnownSkill[] => {
  if (!Array.isArray(value)) {
    return getLearnedSkillsForCharacter(baseClass, level);
  }
  return value
    .map((entry) => {
      if (!entry || typeof entry !== 'object') {
        return null;
      }
      const candidate = entry as Record<string, unknown>;
      if (
        typeof candidate.skill_id !== 'string' ||
        (candidate.category !== 'active' && candidate.category !== 'passive') ||
        typeof candidate.unlock_level !== 'number'
      ) {
        return null;
      }
      return {
        skillId: candidate.skill_id,
        category: candidate.category,
        unlockLevel: candidate.unlock_level,
      } satisfies PlayerKnownSkill;
    })
    .filter((entry): entry is PlayerKnownSkill => entry !== null);
};

export const parseHotbarState = (value: unknown, baseClass: BaseClass): PlayerHotbarState => {
  if (!value || typeof value !== 'object') {
    return normalizeHotbarState(undefined, baseClass);
  }
  const candidate = value as Record<string, unknown>;
  const slots: PlayerHotbarSlot[] = [];
  if (Array.isArray(candidate.slots)) {
    for (const entry of candidate.slots) {
      if (!entry || typeof entry !== 'object') {
        continue;
      }
      const slot = entry as Record<string, unknown>;
      if (typeof slot.slot_index !== 'number') {
        continue;
      }
      if (slot.entry_type === 'skill' && typeof slot.skill_id === 'string') {
        slots.push({
          slotIndex: slot.slot_index,
          entryType: 'skill',
          skillId: slot.skill_id,
          itemId: null,
          actionId: null,
        });
        continue;
      }
      if (slot.entry_type === 'item' && typeof slot.item_instance_id === 'string') {
        slots.push({
          slotIndex: slot.slot_index,
          entryType: 'item',
          skillId: null,
          itemId: slot.item_instance_id,
          actionId: null,
        });
        continue;
      }
      if (slot.entry_type === 'action' && isHotbarActionId(slot.action_id)) {
        slots.push({
          slotIndex: slot.slot_index,
          entryType: 'action',
          skillId: null,
          itemId: null,
          actionId: slot.action_id,
        });
        continue;
      }
      slots.push({
        slotIndex: slot.slot_index,
        entryType: null,
        skillId: null,
        itemId: null,
        actionId: null,
      });
    }
  }

  return normalizeHotbarState(
    {
      openBarCount: typeof candidate.open_bar_count === 'number' ? candidate.open_bar_count : 1,
      slots,
    },
    baseClass,
  );
};

export const cloneHotbarState = (state: PlayerHotbarState): PlayerHotbarState => ({
  openBarCount: state.openBarCount,
  slots: state.slots.map((slot) => ({ ...slot })),
});

export const parseOwnedPets = (value: unknown): OwnedPetState[] => {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((entry) => {
      if (!entry || typeof entry !== 'object') {
        return null;
      }
      const candidate = entry as PetSnapshot;
      if (
        typeof candidate.pet_instance_id !== 'string' ||
        typeof candidate.pet_template_id !== 'string' ||
        typeof candidate.name !== 'string' ||
        typeof candidate.visual_template_id !== 'string' ||
        typeof candidate.mount_eligible !== 'boolean' ||
        typeof candidate.summoned !== 'boolean' ||
        typeof candidate.mounted !== 'boolean'
      ) {
        return null;
      }
      if (
        candidate.kind !== 'pet' &&
        candidate.kind !== 'mount' &&
        candidate.kind !== 'pet_mount'
      ) {
        return null;
      }
      return {
        petInstanceId: candidate.pet_instance_id,
        petTemplateId: candidate.pet_template_id,
        name: candidate.name,
        kind: candidate.kind,
        visualTemplateId: candidate.visual_template_id,
        mountEligible: candidate.mount_eligible,
        summoned: candidate.summoned,
        mounted: candidate.mounted,
      } satisfies OwnedPetState;
    })
    .filter((entry): entry is OwnedPetState => entry !== null);
};

export const cloneOwnedPets = (pets: OwnedPetState[]): OwnedPetState[] =>
  pets.map((pet) => ({ ...pet }));

export const activePetIdFromRoster = (pets: OwnedPetState[]): string | null =>
  pets.find((pet) => pet.summoned)?.petInstanceId ?? null;

export const mountedPetIdFromRoster = (pets: OwnedPetState[]): string | null =>
  pets.find((pet) => pet.mounted)?.petInstanceId ?? null;

export const toHotbarSlotSnapshot = (slot: PlayerHotbarSlot): HotbarSlotSnapshot => {
  if (slot.entryType === 'skill' && slot.skillId) {
    return {
      slot_index: slot.slotIndex,
      entry_type: 'skill',
      skill_id: slot.skillId,
    };
  }
  if (slot.entryType === 'item' && slot.itemId) {
    return {
      slot_index: slot.slotIndex,
      entry_type: 'item',
      item_instance_id: slot.itemId,
    };
  }
  if (slot.entryType === 'action' && slot.actionId) {
    return {
      slot_index: slot.slotIndex,
      entry_type: 'action',
      action_id: slot.actionId,
    };
  }
  return {
    slot_index: slot.slotIndex,
  };
};

export const parsePartySnapshot = (value: unknown): PartyState | null => {
  if (!value || typeof value !== 'object') {
    return null;
  }
  const candidate = value as PartySnapshot;
  if (
    typeof candidate.party_id !== 'string' ||
    typeof candidate.leader_character_id !== 'string' ||
    !Array.isArray(candidate.members)
  ) {
    return null;
  }

  const members = candidate.members
    .map((member) => {
      if (
        !member ||
        typeof member.character_id !== 'string' ||
        typeof member.name !== 'string' ||
        typeof member.level !== 'number' ||
        !isBaseClass(member.base_class) ||
        typeof member.hp !== 'number' ||
        typeof member.mp !== 'number' ||
        typeof member.online !== 'boolean' ||
        typeof member.is_leader !== 'boolean'
      ) {
        return null;
      }
      return {
        characterId: member.character_id,
        name: member.name,
        level: member.level,
        baseClass: member.base_class,
        hp: member.hp,
        mp: member.mp,
        online: member.online,
        isLeader: member.is_leader,
      } satisfies NonNullable<PartyState['members']>[number];
    })
    .filter(
      (member): member is NonNullable<PartyState['members']>[number] => member !== null,
    );

  return {
    partyId: candidate.party_id,
    leaderCharacterId: candidate.leader_character_id,
    members,
  };
};

export const clonePartyState = (party: PartyState | null): PartyState | null =>
  party
    ? {
        partyId: party.partyId,
        leaderCharacterId: party.leaderCharacterId,
        members: party.members.map((member) => ({ ...member })),
      }
    : null;

export const parsePartyInvites = (value: unknown): PendingPartyInviteState[] => {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((entry) => {
      if (!entry || typeof entry !== 'object') {
        return null;
      }
      const candidate = entry as PartyInviteSnapshot;
      if (
        typeof candidate.invite_id !== 'string' ||
        typeof candidate.party_id !== 'string' ||
        typeof candidate.inviter_character_id !== 'string' ||
        typeof candidate.inviter_name !== 'string' ||
        typeof candidate.expires_at_ms !== 'number'
      ) {
        return null;
      }
      return {
        inviteId: candidate.invite_id,
        partyId: candidate.party_id,
        inviterCharacterId: candidate.inviter_character_id,
        inviterName: candidate.inviter_name,
        expiresAtMs: candidate.expires_at_ms,
      } satisfies PendingPartyInviteState;
    })
    .filter((invite): invite is PendingPartyInviteState => invite !== null);
};

export const clonePartyInvites = (
  invites: PendingPartyInviteState[],
): PendingPartyInviteState[] => invites.map((invite) => ({ ...invite }));

export const parseClanSnapshot = (value: unknown): ClanState | null => {
  if (!value || typeof value !== 'object') {
    return null;
  }
  const candidate = value as ClanSnapshot;
  if (
    typeof candidate.clan_id !== 'string' ||
    typeof candidate.name !== 'string' ||
    typeof candidate.leader_character_id !== 'string' ||
    !Array.isArray(candidate.members)
  ) {
    return null;
  }

  const members = candidate.members
    .map((member) => {
      if (
        !member ||
        typeof member.character_id !== 'string' ||
        typeof member.name !== 'string' ||
        typeof member.level !== 'number' ||
        !isBaseClass(member.base_class) ||
        typeof member.online !== 'boolean' ||
        typeof member.is_leader !== 'boolean'
      ) {
        return null;
      }
      return {
        characterId: member.character_id,
        name: member.name,
        level: member.level,
        baseClass: member.base_class,
        online: member.online,
        isLeader: member.is_leader,
      } satisfies NonNullable<ClanState['members']>[number];
    })
    .filter(
      (member): member is NonNullable<ClanState['members']>[number] => member !== null,
    );

  return {
    clanId: candidate.clan_id,
    name: candidate.name,
    leaderCharacterId: candidate.leader_character_id,
    members,
  };
};

export const cloneClanState = (clan: ClanState | null): ClanState | null =>
  clan
    ? {
        clanId: clan.clanId,
        name: clan.name,
        leaderCharacterId: clan.leaderCharacterId,
        members: clan.members.map((member) => ({ ...member })),
      }
    : null;

export const parseClanInvites = (value: unknown): PendingClanInviteState[] => {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((entry) => {
      if (!entry || typeof entry !== 'object') {
        return null;
      }
      const candidate = entry as ClanInviteSnapshot;
      if (
        typeof candidate.invite_id !== 'string' ||
        typeof candidate.clan_id !== 'string' ||
        typeof candidate.clan_name !== 'string' ||
        typeof candidate.inviter_character_id !== 'string' ||
        typeof candidate.inviter_name !== 'string' ||
        typeof candidate.expires_at_ms !== 'number'
      ) {
        return null;
      }
      return {
        inviteId: candidate.invite_id,
        clanId: candidate.clan_id,
        clanName: candidate.clan_name,
        inviterCharacterId: candidate.inviter_character_id,
        inviterName: candidate.inviter_name,
        expiresAtMs: candidate.expires_at_ms,
      } satisfies PendingClanInviteState;
    })
    .filter((invite): invite is PendingClanInviteState => invite !== null);
};

export const cloneClanInvites = (
  invites: PendingClanInviteState[],
): PendingClanInviteState[] => invites.map((invite) => ({ ...invite }));

export const parseAllianceSnapshot = (value: unknown): AllianceState | null => {
  if (!value || typeof value !== 'object') {
    return null;
  }
  const candidate = value as AllianceSnapshot;
  if (
    typeof candidate.alliance_id !== 'string' ||
    typeof candidate.name !== 'string' ||
    typeof candidate.leader_clan_id !== 'string' ||
    typeof candidate.leader_clan_name !== 'string' ||
    typeof candidate.clan_cap !== 'number' ||
    !Array.isArray(candidate.members)
  ) {
    return null;
  }

  const members = candidate.members
    .map((member) => {
      if (
        !member ||
        typeof member.clan_id !== 'string' ||
        typeof member.name !== 'string' ||
        typeof member.leader_character_id !== 'string' ||
        typeof member.leader_name !== 'string' ||
        typeof member.member_count !== 'number' ||
        typeof member.is_leader_clan !== 'boolean'
      ) {
        return null;
      }
      return {
        clanId: member.clan_id,
        name: member.name,
        leaderCharacterId: member.leader_character_id,
        leaderName: member.leader_name,
        memberCount: member.member_count,
        isLeaderClan: member.is_leader_clan,
      } satisfies AllianceMemberClanState;
    })
    .filter((member): member is AllianceMemberClanState => member !== null);

  return {
    allianceId: candidate.alliance_id,
    name: candidate.name,
    leaderClanId: candidate.leader_clan_id,
    leaderClanName: candidate.leader_clan_name,
    clanCap: candidate.clan_cap,
    members,
  };
};

export const cloneAllianceState = (alliance: AllianceState | null): AllianceState | null =>
  alliance
    ? {
        allianceId: alliance.allianceId,
        name: alliance.name,
        leaderClanId: alliance.leaderClanId,
        leaderClanName: alliance.leaderClanName,
        clanCap: alliance.clanCap,
        members: alliance.members.map((member) => ({ ...member })),
      }
    : null;

export const parseAllianceInvites = (value: unknown): PendingAllianceInviteState[] => {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((entry) => {
      if (!entry || typeof entry !== 'object') {
        return null;
      }
      const candidate = entry as AllianceInviteSnapshot;
      if (
        typeof candidate.invite_id !== 'string' ||
        typeof candidate.alliance_id !== 'string' ||
        typeof candidate.alliance_name !== 'string' ||
        typeof candidate.inviter_character_id !== 'string' ||
        typeof candidate.inviter_name !== 'string' ||
        typeof candidate.inviter_clan_id !== 'string' ||
        typeof candidate.inviter_clan_name !== 'string' ||
        typeof candidate.target_clan_id !== 'string' ||
        typeof candidate.expires_at_ms !== 'number'
      ) {
        return null;
      }
      return {
        inviteId: candidate.invite_id,
        allianceId: candidate.alliance_id,
        allianceName: candidate.alliance_name,
        inviterCharacterId: candidate.inviter_character_id,
        inviterName: candidate.inviter_name,
        inviterClanId: candidate.inviter_clan_id,
        inviterClanName: candidate.inviter_clan_name,
        targetClanId: candidate.target_clan_id,
        expiresAtMs: candidate.expires_at_ms,
      } satisfies PendingAllianceInviteState;
    })
    .filter((invite): invite is PendingAllianceInviteState => invite !== null);
};

export const cloneAllianceInvites = (
  invites: PendingAllianceInviteState[],
): PendingAllianceInviteState[] => invites.map((invite) => ({ ...invite }));

export const parseAuthoritativePath = (
  value: unknown,
): { x: number; z: number }[] | null => {
  if (!Array.isArray(value)) {
    return null;
  }
  const points: { x: number; z: number }[] = [];
  for (const entry of value) {
    if (!entry || typeof entry !== 'object') {
      continue;
    }
    const candidate = entry as Partial<{ x: number; z: number }>;
    if (typeof candidate.x !== 'number' || typeof candidate.z !== 'number') {
      continue;
    }
    points.push({ x: candidate.x, z: candidate.z });
  }
  return points;
};

export const parseQuestSnapshot = (value: unknown): GameState['quest'] | null => {
  if (!value || typeof value !== 'object') {
    return null;
  }
  const candidate = value as Record<string, unknown>;
  if (
    typeof candidate.id !== 'string' ||
    typeof candidate.title !== 'string' ||
    typeof candidate.description !== 'string' ||
    typeof candidate.progress !== 'number' ||
    typeof candidate.goal !== 'number'
  ) {
    return null;
  }
  const status = candidate.status;
  if (
    status !== 'available' &&
    status !== 'active' &&
    status !== 'ready_to_turn_in' &&
    status !== 'completed'
  ) {
    return null;
  }
  return {
    id: candidate.id,
    title: candidate.title,
    description: candidate.description,
    status,
    progress: Math.max(0, candidate.progress),
    goal: Math.max(1, candidate.goal),
  };
};

export const parseNpcInteractionSnapshot = (
  value: unknown,
): OnlineNpcInteraction | null => {
  if (!value || typeof value !== 'object') {
    return null;
  }
  const candidate = value as Record<string, unknown>;
  if (typeof candidate.npc_id !== 'string') {
    return null;
  }
  const kind = candidate.kind;
  switch (kind) {
    case 'merchant_services':
    case 'warehouse_services':
    case 'wardkeeper_available':
    case 'wardkeeper_active':
    case 'wardkeeper_ready_to_turn_in':
    case 'wardkeeper_completed':
      return {
        npcId: candidate.npc_id,
        kind,
      };
    default:
      return null;
  }
};

export const cloneQuestState = (quest: GameState['quest']): GameState['quest'] => ({
  ...quest,
});
