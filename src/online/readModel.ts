import {
  createInitialState,
  gameTemplates,
  getArchetypeIdForBaseClass,
  getLearnedSkillsForCharacter,
  normalizeHotbarState,
} from '../game/data/templates';
import type {
  AppearanceOptionIndex,
  BaseClass,
  CharacterRace,
  CharacterSex,
  CompanionState,
  DerivedStats,
  GameState,
  ItemInstance,
  LootDrop,
  MobState,
  NpcState,
  OwnedPetState,
  OtherPlayerState,
  PendingPartyInviteState,
  PendingTradeOfferState,
  PlayerHotbarState,
  PlayerKnownSkill,
  SkillAvailabilityProjection,
  SkillAvailabilityState,
  Vec2,
} from '../game/domain/types';
import type {
  AckMessage,
  ChatChannel,
  ChatMessageServerMessage,
  CharacterSummary,
  CharacterItemSnapshot,
  DeltaMessage,
  EntityAppearMessage,
  EntityDisappearMessage,
  GameplayCommandEnvelope,
  GameplayServerMessage,
  PartyNoticeMessage,
  PositionCorrectionMessage,
  RegionContextMessage,
  SelfStateSnapshot,
  TradeNoticeMessage,
} from './contracts';
import { formatChatMessage, formatRejectMessage } from './readModelFormatting';
import {
  ONLINE_PREDICTION_AUTH_PATH_LEASH_DISTANCE,
  ONLINE_PREDICTION_LEASH_SOFT_ZONE_RATIO,
  ONLINE_PREDICTION_PENDING_LEASH_DISTANCE,
  ONLINE_RECONCILIATION_EXTREME_SNAP_DISTANCE,
  ONLINE_RECONCILIATION_SETTLE_EPSILON,
  ONLINE_RECONCILIATION_TARGET_EPSILON,
  REMOTE_PLAYER_INTERPOLATION_MS,
  REMOTE_PLAYER_SETTLE_EPSILON,
  REMOTE_PLAYER_SNAP_DISTANCE,
  type ProjectedPathMode,
  cloneVecPath,
  closestPointOnSegment,
  distance,
  lerpAngle,
  lerpPoint,
  moveTowards,
  shortestAngleDelta,
} from './readModelMovement';
import {
  activePetIdFromRoster,
  cloneHotbarState,
  cloneOwnedPets,
  clonePartyInvites,
  clonePartyState,
  cloneQuestState,
  mountedPetIdFromRoster,
  parseAuthoritativeCP,
  parseAuthoritativeDead,
  parseAuthoritativeHP,
  parseAuthoritativeLevel,
  parseAuthoritativeMP,
  parseAuthoritativePath,
  parseAuthoritativeStats,
  parseAuthoritativeXP,
  parseHotbarState,
  parseKnownSkills,
  parseNpcInteractionSnapshot,
  parseOwnedPets,
  parsePartyInvites,
  parsePartySnapshot,
  parseQuestSnapshot,
  snapshotFromDelta,
  toAuthoritativeItems,
  toHotbarSlotSnapshot,
  type OnlineNpcInteraction,
} from './readModelParsers';

type PendingCommandStatus = 'sent' | 'acked' | 'rejected' | 'applied';

export interface PendingCommand {
  commandId: string;
  commandSeq: number;
  type: GameplayCommandEnvelope['type'];
  status: PendingCommandStatus;
  reasonCode?: string;
}

export type DesyncState = 'none' | 'revision_gap' | 'region_revision_gap';

type OnlineEntityState = {
  entityId: string;
  entityType: 'npc' | 'mob' | 'loot' | 'player' | 'pet';
  templateId: string;
  position: Vec2;
  state: Record<string, unknown>;
};

type CooldownAuthorityEntry = {
  authorityState: SkillAvailabilityState;
  authoritativeEndsAtMs: number | null;
};

type OnlineFloatingText = {
  id: string;
  text: string;
  color: string;
  position: Vec2;
  entityId?: string;
  expiresAtMs: number;
};

type RemotePlayerProjection = {
  fromPosition: Vec2;
  toPosition: Vec2;
  renderPosition: Vec2;
  fromFacing: number;
  toFacing: number;
  renderFacing: number;
  startedAtMs: number;
  endsAtMs: number;
};

type OnlineLogEntry = GameState['logs'][number];

const FLOATING_TEXT_TTL_MS = 1100;
const LOOT_PICKUP_SEARCH_RANGE = 16;
const PET_FOLLOW_DISTANCE = 1.8;
const PET_FOLLOW_SIDE_OFFSET = 0.75;
const CHAT_MESSAGE_MAX_LENGTH = 240;

const makeCommandId = (commandSeq: number): string => `cmd_${Date.now()}_${commandSeq}`;

const isBaseClass = (value: unknown): value is BaseClass => value === 'Fighter' || value === 'Mage';

const CHARACTER_RACES: readonly CharacterRace[] = ['Human', 'Elf', 'Dark Elf', 'Orc', 'Dwarf'];
const CHARACTER_SEXES: readonly CharacterSex[] = ['Male', 'Female'];

const requireBaseClass = (value: unknown, field: string): BaseClass => {
  if (isBaseClass(value)) {
    return value;
  }
  throw new Error(`Missing canonical ${field}.`);
};

const requireRace = (value: unknown, field: string): CharacterRace => {
  if (CHARACTER_RACES.includes(value as CharacterRace)) {
    return value as CharacterRace;
  }
  throw new Error(`Missing canonical ${field}.`);
};

const requireSex = (value: unknown, field: string): CharacterSex => {
  if (CHARACTER_SEXES.includes(value as CharacterSex)) {
    return value as CharacterSex;
  }
  throw new Error(`Missing canonical ${field}.`);
};

const requireAppearanceIndex = (value: unknown, field: string): AppearanceOptionIndex => {
  if (value === 0 || value === 1 || value === 2) {
    return value;
  }
  throw new Error(`Missing canonical ${field}.`);
};

const npcTemplate = (templateId: string): Pick<NpcState, 'name' | 'title'> => {
  if (templateId === 'wardkeeper') {
    return {
      name: 'Selka',
      title: 'Wardkeeper of the Plaza',
    };
  }
  if (templateId === 'merchant') {
    return {
      name: 'Ilya',
      title: 'Provisioner of the Plaza',
    };
  }
  if (templateId === 'warehouse_keeper') {
    return {
      name: 'Rhea',
      title: 'Vaultkeeper of the Plaza',
    };
  }
  return {
    name: templateId,
    title: 'NPC',
  };
};

const otherPlayerSnapshot = (
  entity: OnlineEntityState,
  visualPosition: Vec2 = entity.position,
  visualFacing: number = typeof entity.state.facing === 'number' ? entity.state.facing : 0,
): OtherPlayerState => {
  const baseClass = requireBaseClass(entity.state.base_class, 'other player base_class');
  return {
    id: entity.entityId,
    name: typeof entity.state.name === 'string' ? entity.state.name : entity.entityId,
    race: requireRace(entity.state.race, 'other player race'),
    baseClass,
    sex: requireSex(entity.state.sex, 'other player sex'),
    hairStyle: requireAppearanceIndex(entity.state.hair_style, 'other player hair_style'),
    hairColor: requireAppearanceIndex(entity.state.hair_color, 'other player hair_color'),
    face: requireAppearanceIndex(entity.state.face, 'other player face'),
    archetypeId: getArchetypeIdForBaseClass(baseClass),
    level: typeof entity.state.level === 'number' ? entity.state.level : 1,
    hp: typeof entity.state.hp === 'number' ? entity.state.hp : 1,
    dead: entity.state.dead === true,
    position: { ...visualPosition },
    facing: visualFacing,
    mountedPetId: typeof entity.state.mounted_pet_id === 'string' ? entity.state.mounted_pet_id : null,
  };
};

const toEntityMap = (regionContext: RegionContextMessage): Map<string, OnlineEntityState> => {
  const entities = new Map<string, OnlineEntityState>();
  for (const entity of regionContext.known_entities) {
    entities.set(entity.entity_id, {
      entityId: entity.entity_id,
      entityType: entity.entity_type,
      templateId: entity.template_id,
      position: { ...entity.position },
      state: { ...entity.state },
    });
  }
  return entities;
};

const clonePendingTradeOffer = (offer: PendingTradeOfferState | null): PendingTradeOfferState | null =>
  offer
    ? {
        offerId: offer.offerId,
        direction: offer.direction,
        counterpartyCharacterId: offer.counterpartyCharacterId,
        counterpartyName: offer.counterpartyName,
        itemTemplateId: offer.itemTemplateId,
        quantity: offer.quantity,
      }
    : null;

const projectNpcDialog = (
  interaction: OnlineNpcInteraction | null,
  quest: GameState['quest'] | null,
): GameState['dialog'] => {
  if (!interaction) {
    return null;
  }

  switch (interaction.kind) {
    case 'merchant_services':
      return {
        npcId: interaction.npcId,
        title: 'Ilya, Provisioner of the Plaza',
        body: 'My stock and exchanges are open while you remain within trading distance.',
      };
    case 'warehouse_services':
      return {
        npcId: interaction.npcId,
        title: 'Rhea, Vaultkeeper of the Plaza',
        body: 'Your warehouse is available while you remain within storage range.',
      };
    case 'wardkeeper_available':
      return {
        npcId: interaction.npcId,
        title: 'Selka, Wardkeeper of the Plaza',
        body: 'The gate road is drawing Mirelings again. Clear three of them and I will fit you with a proper mantle.',
        actionLabel: 'Accept Task',
        actionId: 'accept_task',
      };
    case 'wardkeeper_active':
      return {
        npcId: interaction.npcId,
        title: 'Selka, Wardkeeper of the Plaza',
        body: `You have driven back ${quest?.progress ?? 0}/${quest?.goal ?? 3} Mirelings. Finish the cull beyond the gate.`,
      };
    case 'wardkeeper_ready_to_turn_in':
      return {
        npcId: interaction.npcId,
        title: 'Selka, Wardkeeper of the Plaza',
        body: 'The road is clear enough for now. Take this mantle and keep your footing in the field.',
        actionLabel: 'Receive Reward',
        actionId: 'turn_in_task',
      };
    case 'wardkeeper_completed':
      return {
        npcId: interaction.npcId,
        title: 'Selka, Wardkeeper of the Plaza',
        body: 'You have done enough for one watch. Deeper ruins still hide stronger prey if you want a better weapon.',
      };
    default:
      return null;
  }
};

const projectCompanionPosition = (
  ownerPosition: Vec2,
  ownerFacing: number,
  mounted: boolean,
): Vec2 => {
  if (mounted) {
    return { ...ownerPosition };
  }
  return {
    x: ownerPosition.x - Math.cos(ownerFacing) * PET_FOLLOW_DISTANCE + Math.sin(ownerFacing) * PET_FOLLOW_SIDE_OFFSET,
    z: ownerPosition.z - Math.sin(ownerFacing) * PET_FOLLOW_DISTANCE - Math.cos(ownerFacing) * PET_FOLLOW_SIDE_OFFSET,
  };
};

export class OnlineReadModel {
  private readonly character: CharacterSummary;
  private readonly entities = new Map<string, OnlineEntityState>();
  private readonly pendingCommands = new Map<string, PendingCommand>();
  private lastAppliedCorrectionSeq = 0;
  private nextCommandSeq = 1;
  private lastRevision = 0;
  private lastRegionRevision = 0;
  private regionId: string;
  private geodataVersion: string;
  private authoritativePlayerPosition: Vec2;
  private projectedPlayerPosition: Vec2;
  private projectedMoveTarget: Vec2 | null = null;
  private projectedPathQueue: Vec2[] = [];
  private projectedPathMode: ProjectedPathMode = 'none';
  private projectedFacing = 0;
  private lastProjectionAtMs = Date.now();
  private pendingPathPreview: Vec2[] = [];
  private authoritativePathPreview: Vec2[] = [];
  private targetId: string | null = null;
  private readonly cooldownAuthority = new Map<string, CooldownAuthorityEntry>();
  private authoritativeItems: Record<string, ItemInstance>;
  private authoritativeStats: DerivedStats | null;
  private authoritativeCP: number | null;
  private authoritativeHP: number | null;
  private authoritativeMP: number | null;
  private authoritativeXP: number | null;
  private authoritativeLevel: number | null;
  private authoritativeBaseClass: BaseClass;
  private authoritativeKnownSkills: PlayerKnownSkill[];
  private authoritativeHotbar: PlayerHotbarState;
  private authoritativePets: OwnedPetState[];
  private authoritativeDead = false;
  private authoritativeQuest: GameState['quest'] | null;
  private authoritativeParty: GameState['party'] | null;
  private authoritativePartyInvites: PendingPartyInviteState[] = [];
  private activeNpcInteraction: OnlineNpcInteraction | null;
  private incomingTradeOffer: PendingTradeOfferState | null = null;
  private outgoingTradeOffer: PendingTradeOfferState | null = null;
  private floatingTexts: OnlineFloatingText[] = [];
  private readonly remotePlayerProjections = new Map<string, RemotePlayerProjection>();
  private logs: OnlineLogEntry[] = [];
  private desyncState: DesyncState = 'none';

  constructor(
    regionContext: RegionContextMessage,
    character: CharacterSummary | null,
    itemState?: CharacterItemSnapshot | null,
    selfState?: SelfStateSnapshot | null,
  ) {
    if (!character) {
      throw new Error('Online read model requires an authoritative character summary.');
    }
    this.character = character;
    this.regionId = regionContext.region_id;
    this.geodataVersion = regionContext.geodata_version;
    this.authoritativePlayerPosition = { ...regionContext.self_position };
    this.projectedPlayerPosition = { ...regionContext.self_position };
    this.lastRegionRevision = regionContext.region_revision;
    this.authoritativeItems = toAuthoritativeItems(itemState);
    this.authoritativeStats = parseAuthoritativeStats(selfState?.stats);
    this.authoritativeCP = parseAuthoritativeCP(selfState?.cp);
    this.authoritativeHP = parseAuthoritativeHP(selfState?.hp);
    this.authoritativeMP = parseAuthoritativeMP(selfState?.mp);
    this.authoritativeXP = parseAuthoritativeXP(selfState?.xp);
    this.authoritativeLevel = parseAuthoritativeLevel(selfState?.level);
    this.authoritativeBaseClass = character.base_class;
    const currentLevel = this.authoritativeLevel ?? character.level;
    this.authoritativeKnownSkills = parseKnownSkills(selfState?.known_skills, this.authoritativeBaseClass, currentLevel);
    this.authoritativeHotbar = parseHotbarState(selfState?.hotbar, this.authoritativeBaseClass);
    this.authoritativePets = parseOwnedPets(selfState?.pets);
    this.authoritativeDead = parseAuthoritativeDead(selfState?.dead) ?? false;
    this.authoritativeQuest = parseQuestSnapshot(selfState?.quest);
    this.authoritativeParty = parsePartySnapshot(selfState?.party);
    this.authoritativePartyInvites = parsePartyInvites(selfState?.party_invites);
    this.activeNpcInteraction = parseNpcInteractionSnapshot(selfState?.npc_interaction);
    if (selfState?.cooldowns && typeof selfState.cooldowns === 'object') {
      this.syncCooldownSnapshot(selfState.cooldowns, Date.now());
    }
    const projectionNowMs = Date.now();
    for (const [entityId, entity] of toEntityMap(regionContext)) {
      this.entities.set(entityId, entity);
      if (entity.entityType === 'player') {
        this.seedRemotePlayerProjection(entity, projectionNowMs);
      }
    }
    this.pushLog(`Online attach confirmed. Region context loaded for ${regionContext.region_id}.`, 'success');
  }

  get snapshot(): GameState {
    const nowMs = Date.now();
    this.advanceProjectedMovement(nowMs);
    this.advanceRemotePlayerProjections(nowMs);
    const state = createInitialState();
    state.timeMs = nowMs;
    this.clearInvalidTargetIfNeeded();
    state.player.id = this.character.character_id;
    state.player.name = this.character.name;
    state.player.race = this.character.race;
    state.player.baseClass = this.authoritativeBaseClass;
    state.player.sex = this.character.sex;
    state.player.hairStyle = requireAppearanceIndex(this.character.hair_style, 'character hair_style');
    state.player.hairColor = requireAppearanceIndex(this.character.hair_color, 'character hair_color');
    state.player.face = requireAppearanceIndex(this.character.face, 'character face');
    state.player.archetypeId = getArchetypeIdForBaseClass(this.authoritativeBaseClass);
    state.player.level = this.authoritativeLevel ?? this.character.level;
    state.player.xp = this.authoritativeXP ?? state.player.xp;
    state.player.position = { ...this.projectedPlayerPosition };
    state.player.facing = this.projectedFacing;
    state.player.moveTarget = this.currentPathDestination();
    state.player.cast = null;
    state.player.learnedSkills = this.authoritativeKnownSkills.map((skill) => ({ ...skill }));
    state.player.hotbar = cloneHotbarState(this.authoritativeHotbar);
    state.player.pets = cloneOwnedPets(this.authoritativePets);
    state.player.activePetId = activePetIdFromRoster(this.authoritativePets);
    state.player.mountedPetId = mountedPetIdFromRoster(this.authoritativePets);
    state.player.deadUntilMs = this.authoritativeDead ? Number.MAX_SAFE_INTEGER : null;
    state.player.authoritativeStats = this.authoritativeStats ? { ...this.authoritativeStats } : null;
    if (this.authoritativeCP !== null) {
      const maxCp = state.player.authoritativeStats?.maxCp ?? state.player.cp;
      state.player.cp = Math.max(0, Math.min(this.authoritativeCP, maxCp));
    }
    if (this.authoritativeHP !== null) {
      const maxHp = state.player.authoritativeStats?.maxHp ?? state.player.hp;
      state.player.hp = Math.max(0, Math.min(this.authoritativeHP, maxHp));
    }
    if (this.authoritativeMP !== null) {
      const maxMp = state.player.authoritativeStats?.maxMp ?? state.player.mp;
      state.player.mp = Math.max(0, Math.min(this.authoritativeMP, maxMp));
    }
    const cooldownProjection = this.projectCooldowns();
    state.player.cooldowns = Object.fromEntries(
      Object.entries(cooldownProjection).map(([skillId, projection]) => [skillId, projection.visualRemainingMs]),
    );
    state.player.skillAvailability = cooldownProjection;
    state.targetId = this.targetId;
    state.destinationMarker = this.currentPathDestination();
    state.pendingPath = cloneVecPath(this.pendingPathPreview);
    state.authoritativePath = cloneVecPath(this.authoritativePathPreview);
    state.logs = [...this.logs];
    state.quest =
      this.authoritativeQuest
        ? cloneQuestState(this.authoritativeQuest)
        : {
            id: 'online_bootstrap',
            title: 'Online Bootstrap',
            description: this.isCommandFlowBlocked()
              ? this.desyncState === 'revision_gap'
                ? 'Online read model detected a revision gap. Command flow is blocked until explicit online reset.'
                : 'Online read model detected a region revision gap. Command flow is blocked until explicit online reset.'
              : `Quest authority is pending while region ${this.regionId} (${this.geodataVersion}) remains attached.`,
            status: 'available',
            progress: 0,
            goal: 1,
          };
    state.dialog = projectNpcDialog(this.activeNpcInteraction, this.authoritativeQuest);
    state.party = clonePartyState(this.authoritativeParty);
    state.partyInvites = clonePartyInvites(this.authoritativePartyInvites);
    state.incomingTradeOffer = clonePendingTradeOffer(this.incomingTradeOffer);
    state.outgoingTradeOffer = clonePendingTradeOffer(this.outgoingTradeOffer);
    state.floatingTexts = this.projectFloatingTexts();
    state.npcs = {};
    state.otherPlayers = {};
    state.companions = {};
    state.mobs = {};
    state.loot = {};
    state.items = structuredClone(this.authoritativeItems);

    for (const entity of this.entities.values()) {
      if (entity.entityType === 'npc') {
        const npc = npcTemplate(entity.templateId);
        state.npcs[entity.entityId] = {
          id: entity.entityId,
          name: npc.name,
          title: npc.title,
          position: { ...entity.position },
        };
        continue;
      }

      if (entity.entityType === 'mob') {
        const hp = typeof entity.state.hp === 'number' ? entity.state.hp : 1;
        state.mobs[entity.entityId] = {
          id: entity.entityId,
          templateId: entity.templateId,
          position: { ...entity.position },
          spawnPoint: { ...entity.position },
          hp,
          aiState: entity.state.alive === false ? 'dead' : 'idle',
          attackCooldownMs: 0,
          respawnAtMs: null,
        } satisfies MobState;
        continue;
      }

      if (entity.entityType === 'player') {
        const projection = this.remotePlayerProjections.get(entity.entityId);
        state.otherPlayers[entity.entityId] = otherPlayerSnapshot(
          entity,
          projection?.renderPosition ?? entity.position,
          projection?.renderFacing ?? (typeof entity.state.facing === 'number' ? entity.state.facing : 0),
        );
        continue;
      }

      if (entity.entityType === 'pet') {
        continue;
      }

      state.loot[entity.entityId] = {
        id: entity.entityId,
        itemInstanceId: entity.entityId,
        position: { ...entity.position },
        label: gameTemplates.itemTemplates[entity.templateId]?.name ?? entity.templateId,
      } satisfies LootDrop;
    }

    for (const entity of this.entities.values()) {
      if (entity.entityType !== 'pet') {
        continue;
      }
      const ownerId = typeof entity.state.owner_id === 'string' ? entity.state.owner_id : '';
      const ownerName = typeof entity.state.owner_name === 'string' ? entity.state.owner_name : ownerId;
      const petTemplateId =
        typeof entity.state.pet_template_id === 'string' ? entity.state.pet_template_id : entity.entityId;
      const visualTemplateId =
        typeof entity.state.visual_template === 'string' ? entity.state.visual_template : entity.templateId;
      const ownerPosition =
        ownerId === state.player.id
          ? state.player.position
          : state.otherPlayers[ownerId]?.position ?? entity.position;
      const ownerFacing =
        ownerId === state.player.id
          ? state.player.facing
          : state.otherPlayers[ownerId]?.facing ?? (typeof entity.state.facing === 'number' ? entity.state.facing : 0);
      const mounted = entity.state.mounted === true;
      state.companions[entity.entityId] = {
        id: entity.entityId,
        ownerId,
        ownerName,
        petTemplateId,
        visualTemplateId,
        name: typeof entity.state.name === 'string' ? entity.state.name : entity.entityId,
        kind:
          entity.state.kind === 'pet' || entity.state.kind === 'mount' || entity.state.kind === 'pet_mount'
            ? entity.state.kind
            : 'pet',
        mountEligible: entity.state.mount_eligible === true,
        mounted,
        position: projectCompanionPosition(ownerPosition, ownerFacing, mounted),
        mountedByCharacterId:
          typeof entity.state.mounted_by_character_id === 'string' ? entity.state.mounted_by_character_id : null,
        followOwnerId: typeof entity.state.follow_owner_id === 'string' ? entity.state.follow_owner_id : null,
      } satisfies CompanionState;
    }

    return state;
  }

  getStateInfo(): {
    lastRevision: number;
    lastRegionRevision: number;
    nextCommandSeq: number;
    pendingCommands: PendingCommand[];
    commandFlowBlocked: boolean;
    desyncState: DesyncState;
  } {
    return {
      lastRevision: this.lastRevision,
      lastRegionRevision: this.lastRegionRevision,
      nextCommandSeq: this.nextCommandSeq,
      pendingCommands: [...this.pendingCommands.values()],
      commandFlowBlocked: this.isCommandFlowBlocked(),
      desyncState: this.desyncState,
    };
  }

  isCommandFlowBlocked(): boolean {
    return this.desyncState !== 'none';
  }

  createMoveIntent(point: Vec2): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'move_intent',
      payload: {
        point: { ...point },
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    this.pendingPathPreview = [{ ...this.projectedPlayerPosition }, { ...envelope.payload.point }];
    this.authoritativePathPreview = [];
    this.beginProjectedPath([envelope.payload.point], 'pending');
    return envelope;
  }

  createSelectTarget(targetId: string): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'select_target',
      payload: {
        target_id: targetId,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createUseSkill(skillId: string): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    const projection = this.projectCooldowns()[skillId];
    if (projection && projection.requestBlocked) {
      this.pushLog(
        projection.authorityState === 'cooldown_elapsed_waiting_authority'
          ? `${skillId} is waiting for a new authoritative cooldown snapshot.`
          : `${skillId} is still on cooldown.`,
        'warning',
      );
      return null;
    }

    const skillTemplate = gameTemplates.skills[skillId];
    const knownSkill = this.authoritativeKnownSkills.find((skill) => skill.skillId === skillId) ?? null;
    if (!knownSkill) {
      this.pushLog(`${skillTemplate?.name ?? skillId} is not learned yet.`, 'warning');
      return null;
    }
    if (!skillTemplate || knownSkill.category !== 'active' || skillTemplate.category !== 'active') {
      this.pushLog(`${skillTemplate?.name ?? skillId} is passive and cannot be activated.`, 'warning');
      return null;
    }
    const requiresLivingTarget =
      skillTemplate?.targetType === 'single_target_enemy' || skillTemplate?.targetType === 'target_centered_aoe';
    const resolvedTarget = requiresLivingTarget ? this.getLivingMobTarget(this.targetId) : null;
    if (requiresLivingTarget && !resolvedTarget) {
      this.clearInvalidTargetIfNeeded();
      this.pushLog(`${skillId} requires a living target.`, 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const payload: { skill_id: string; target_id?: string } = {
      skill_id: skillId,
    };
    if (resolvedTarget) {
      payload.target_id = resolvedTarget.entityId;
    }

    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'use_skill',
      payload,
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createBasicAttack(): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    const resolvedTarget = this.getLivingMobTarget(this.targetId);
    if (!resolvedTarget) {
      this.clearInvalidTargetIfNeeded();
      this.pushLog('Basic attack requires a living target.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'basic_attack',
      payload: {
        target_id: resolvedTarget.entityId,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createTameMob(targetId = this.targetId): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    const resolvedTarget = this.getLivingMobTarget(targetId);
    if (!resolvedTarget) {
      this.clearInvalidTargetIfNeeded();
      this.pushLog('Taming requires a living target.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'tame_mob',
      payload: {
        target_id: resolvedTarget.entityId,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createPickUpLoot(lootId: string): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    const loot = this.entities.get(lootId);
    if (!loot || loot.entityType !== 'loot') {
      this.pushLog('Loot is no longer known in the current region.', 'warning');
      return null;
    }
    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'pick_up_loot',
      payload: {
        loot_id: lootId,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  findNearestLootId(maxDistance = LOOT_PICKUP_SEARCH_RANGE): string | null {
    const nearestLoot = [...this.entities.values()]
      .filter((entity) => entity.entityType === 'loot')
      .map((entity) => ({
        entity,
        distance: distance(this.projectedPlayerPosition, entity.position),
      }))
      .filter((entry) => entry.distance <= maxDistance)
      .sort((left, right) => left.distance - right.distance)[0];

    return nearestLoot?.entity.entityId ?? null;
  }

  createPickUpNearbyLoot(): GameplayCommandEnvelope | null {
    const nearestLootId = this.findNearestLootId();
    if (!nearestLootId) {
      this.pushLog('No nearby loot to pick up.', 'warning');
      return null;
    }
    return this.createPickUpLoot(nearestLootId);
  }

  createSummonPet(): GameplayCommandEnvelope | null {
    const pet = this.primaryOwnedPet();
    if (!pet) {
      this.pushLog('Character does not own a companion yet.', 'warning');
      return null;
    }
    if (pet.summoned) {
      this.pushLog(`${pet.name} is already summoned.`, 'warning');
      return null;
    }
    return this.createEmptyPetCommand('summon_pet');
  }

  createDismissPet(): GameplayCommandEnvelope | null {
    const pet = this.primaryOwnedPet();
    if (!pet) {
      this.pushLog('Character does not own a companion yet.', 'warning');
      return null;
    }
    if (pet.mounted) {
      this.pushLog('Dismount before dismissing the companion.', 'warning');
      return null;
    }
    if (!pet.summoned) {
      this.pushLog(`${pet.name} is not currently summoned.`, 'warning');
      return null;
    }
    return this.createEmptyPetCommand('dismiss_pet');
  }

  createMountPet(): GameplayCommandEnvelope | null {
    const pet = this.primaryOwnedPet();
    if (!pet) {
      this.pushLog('Character does not own a companion yet.', 'warning');
      return null;
    }
    if (!pet.mountEligible) {
      this.pushLog(`${pet.name} cannot be mounted in this slice.`, 'warning');
      return null;
    }
    if (!pet.summoned) {
      this.pushLog(`Summon ${pet.name} before mounting.`, 'warning');
      return null;
    }
    if (pet.mounted) {
      this.pushLog(`You are already mounted on ${pet.name}.`, 'warning');
      return null;
    }
    return this.createEmptyPetCommand('mount_pet');
  }

  createDismountPet(): GameplayCommandEnvelope | null {
    const pet = this.mountedOwnedPet();
    if (!pet) {
      this.pushLog('Character is not currently mounted.', 'warning');
      return null;
    }
    return this.createEmptyPetCommand('dismount_pet');
  }

  private createEmptyPetCommand(
    type: 'summon_pet' | 'dismiss_pet' | 'mount_pet' | 'dismount_pet',
  ): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type,
      payload: {},
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createInteractNpc(npcId: string, actionId?: 'accept_task' | 'turn_in_task'): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    const npc = this.entities.get(npcId);
    if (!npc || npc.entityType !== 'npc') {
      this.pushLog('NPC is no longer known in the current region.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'interact_npc',
      payload: {
        npc_id: npcId,
        ...(actionId ? { action_id: actionId } : {}),
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createSetHotbarState(hotbar: PlayerHotbarState): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    const normalized = normalizeHotbarState(hotbar, this.authoritativeBaseClass);
    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'set_hotbar_state',
      payload: {
        open_bar_count: normalized.openBarCount,
        slots: normalized.slots.map(toHotbarSlotSnapshot),
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    this.authoritativeHotbar = cloneHotbarState(normalized);
    return envelope;
  }

  createEquipItem(itemId: string): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    const item = this.authoritativeItems[itemId];
    if (!item || item.container !== 'inventory') {
      this.pushLog('Item is no longer available in the inventory.', 'warning');
      return null;
    }

    const template = gameTemplates.itemTemplates[item.templateId];
    if (!template?.equipSlot) {
      this.pushLog('Item cannot be equipped.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'equip_item',
      payload: {
        item_instance_id: itemId,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createUseItem(itemId: string): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    const item = this.authoritativeItems[itemId];
    if (!item || item.container !== 'inventory') {
      this.pushLog('Consumable use failed: item is no longer in the inventory.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'use_item',
      payload: {
        item_instance_id: itemId,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createUnequipItem(slot: 'weapon' | 'chest' | 'gloves' | 'boots'): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    const equippedItem = Object.values(this.authoritativeItems).find(
      (item) => item.container === 'equipment' && item.equipSlot === slot,
    );
    if (!equippedItem) {
      this.pushLog('Equipment slot is already empty.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'unequip_item',
      payload: {
        equip_slot: slot,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  dismissNpcInteraction(): void {
    this.activeNpcInteraction = null;
  }

  createSplitItemStack(itemId: string, quantity: number): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    const item = this.authoritativeItems[itemId];
    if (!item || item.container !== 'inventory') {
      this.pushLog('Stack split failed: item is no longer available in the inventory.', 'warning');
      return null;
    }
    if (quantity <= 0 || item.quantity <= quantity) {
      this.pushLog('Stack split failed: quantity is not valid for that stack.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'split_item_stack',
      payload: {
        item_instance_id: itemId,
        quantity,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createMergeItemStacks(sourceItemId: string, targetItemId: string): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    if (sourceItemId === targetItemId) {
      this.pushLog('Stack merge failed: source and target are identical.', 'warning');
      return null;
    }
    const source = this.authoritativeItems[sourceItemId];
    const target = this.authoritativeItems[targetItemId];
    if (!source || !target || source.container !== 'inventory' || target.container !== 'inventory') {
      this.pushLog('Stack merge failed: one of the stacks is no longer in the inventory.', 'warning');
      return null;
    }
    if (source.templateId !== target.templateId) {
      this.pushLog('Stack merge failed: items do not share the same template.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'merge_item_stacks',
      payload: {
        source_item_instance_id: sourceItemId,
        target_item_instance_id: targetItemId,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createBuyItem(offerId: string, quantity: number): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    if (!gameTemplates.vendorOffers[offerId] || quantity <= 0) {
      this.pushLog('Vendor purchase failed: offer is not available.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'buy_item',
      payload: {
        vendor_offer_id: offerId,
        quantity,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createExchangeItem(offerId: string, quantity: number): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    if (!gameTemplates.exchangeOffers[offerId] || quantity <= 0) {
      this.pushLog('Exchange failed: offer is not available.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'exchange_item',
      payload: {
        exchange_offer_id: offerId,
        quantity,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createDepositItem(itemId: string, quantity: number): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    const item = this.authoritativeItems[itemId];
    if (!item || item.container !== 'inventory' || quantity <= 0) {
      this.pushLog('Warehouse deposit failed: item is no longer in the inventory.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'deposit_item',
      payload: {
        item_instance_id: itemId,
        quantity,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createWithdrawItem(itemId: string, quantity: number): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    const item = this.authoritativeItems[itemId];
    if (!item || item.container !== 'warehouse' || quantity <= 0) {
      this.pushLog('Warehouse withdraw failed: item is no longer stored.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'withdraw_item',
      payload: {
        item_instance_id: itemId,
        quantity,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createSellItem(itemId: string, quantity: number): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    const item = this.authoritativeItems[itemId];
    if (!item || item.container !== 'inventory' || quantity <= 0) {
      this.pushLog('Vendor sale failed: item is no longer in the inventory.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'sell_item',
      payload: {
        item_instance_id: itemId,
        quantity,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createOfferTradeItem(targetCharacterId: string, itemId: string, quantity: number): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    if (this.incomingTradeOffer || this.outgoingTradeOffer) {
      this.pushLog('Trade offer failed: there is already a pending player trade.', 'warning');
      return null;
    }

    const item = this.authoritativeItems[itemId];
    const target = this.entities.get(targetCharacterId);
    if (!item || item.container !== 'inventory') {
      this.pushLog('Trade offer failed: item is no longer in the inventory.', 'warning');
      return null;
    }
    if (!target || target.entityType !== 'player') {
      this.pushLog('Trade offer failed: player is no longer nearby.', 'warning');
      return null;
    }
    if (quantity <= 0 || (!gameTemplates.itemTemplates[item.templateId]?.stackable && quantity !== 1) || quantity > item.quantity) {
      this.pushLog('Trade offer failed: quantity is invalid for the selected item.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'offer_trade_item',
      payload: {
        target_character_id: targetCharacterId,
        item_instance_id: itemId,
        quantity,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createAcceptTradeOffer(offerId: string): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    if (!this.incomingTradeOffer || this.incomingTradeOffer.offerId !== offerId) {
      this.pushLog('Trade accept failed: offer is no longer available.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'accept_trade_offer',
      payload: {
        trade_offer_id: offerId,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createDeclineTradeOffer(offerId: string): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    if (!this.incomingTradeOffer || this.incomingTradeOffer.offerId !== offerId) {
      this.pushLog('Trade decline failed: offer is no longer available.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'decline_trade_offer',
      payload: {
        trade_offer_id: offerId,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createInvitePartyMember(targetCharacterId: string): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    const target = this.entities.get(targetCharacterId);
    if (!target || target.entityType !== 'player') {
      this.pushLog('Party invite failed: player is no longer known in the current region.', 'warning');
      return null;
    }
    if (this.authoritativeParty && this.authoritativeParty.leaderCharacterId !== this.character.character_id) {
      this.pushLog('Party invite failed: only the current leader can invite new members.', 'warning');
      return null;
    }
    if (this.authoritativeParty?.members.some((member) => member.characterId === targetCharacterId)) {
      this.pushLog('Party invite failed: player is already in the party.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'invite_party_member',
      payload: {
        target_character_id: targetCharacterId,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createAcceptPartyInvite(inviteId: string): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    if (!this.authoritativePartyInvites.some((invite) => invite.inviteId === inviteId)) {
      this.pushLog('Party accept failed: invite is no longer available.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'accept_party_invite',
      payload: {
        invite_id: inviteId,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createDeclinePartyInvite(inviteId: string): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    if (!this.authoritativePartyInvites.some((invite) => invite.inviteId === inviteId)) {
      this.pushLog('Party decline failed: invite is no longer available.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'decline_party_invite',
      payload: {
        invite_id: inviteId,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createLeaveParty(): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    if (!this.authoritativeParty) {
      this.pushLog('Party leave failed: character is not currently in a party.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'leave_party',
      payload: {},
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createKickPartyMember(targetCharacterId: string): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    if (this.authoritativeDead) {
      this.pushLog('Actor is currently dead.', 'warning');
      return null;
    }
    if (!this.authoritativeParty) {
      this.pushLog('Party removal failed: character is not currently in a party.', 'warning');
      return null;
    }
    if (this.authoritativeParty.leaderCharacterId !== this.character.character_id) {
      this.pushLog('Party removal failed: only the current leader can remove members.', 'warning');
      return null;
    }
    if (!this.authoritativeParty.members.some((member) => member.characterId === targetCharacterId)) {
      this.pushLog('Party removal failed: player is no longer in the party.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'kick_party_member',
      payload: {
        target_character_id: targetCharacterId,
      },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  createSendChatMessage(
    channel: ChatChannel,
    text: string,
    targetCharacterName?: string,
  ): GameplayCommandEnvelope | null {
    if (this.isCommandFlowBlocked()) {
      this.pushLog('Command flow is blocked. Reset online bootstrap before sending new commands.', 'warning');
      return null;
    }
    const normalizedChannel = channel === 'region' || channel === 'party' || channel === 'whisper'
      ? channel
      : null;
    if (!normalizedChannel) {
      this.pushLog('Chat send failed: channel is not available in this slice.', 'warning');
      return null;
    }

    const normalizedText = text.trim();
    if (!normalizedText) {
      this.pushLog('Chat send failed: message cannot be empty.', 'warning');
      return null;
    }
    if ([...normalizedText].length > CHAT_MESSAGE_MAX_LENGTH) {
      this.pushLog(`Chat send failed: message exceeds ${CHAT_MESSAGE_MAX_LENGTH} characters.`, 'warning');
      return null;
    }
    if (normalizedChannel === 'party' && !this.authoritativeParty) {
      this.pushLog('Party chat failed: character is not currently in a party.', 'warning');
      return null;
    }

    const normalizedTargetName = (targetCharacterName ?? '').trim();
    if (normalizedChannel === 'whisper' && !normalizedTargetName) {
      this.pushLog('Whisper failed: choose an online target character name.', 'warning');
      return null;
    }

    const commandSeq = this.nextCommandSeq++;
    const commandId = makeCommandId(commandSeq);
    const envelope: GameplayCommandEnvelope = {
      protocol_version: 1,
      command_id: commandId,
      command_seq: commandSeq,
      client_sent_at_ms: Date.now(),
      type: 'send_chat_message',
      payload: normalizedChannel === 'whisper'
        ? {
            channel: normalizedChannel,
            text: normalizedText,
            target_character_name: normalizedTargetName,
          }
        : {
            channel: normalizedChannel,
            text: normalizedText,
          },
    };
    this.pendingCommands.set(commandId, {
      commandId,
      commandSeq,
      type: envelope.type,
      status: 'sent',
    });
    return envelope;
  }

  applyMessage(message: GameplayServerMessage): { changed: boolean } {
    switch (message.kind) {
      case 'ack':
        return this.applyAck(message);
      case 'reject':
        return this.applyReject(message.command_id, message.reason_code, message.message);
      case 'delta':
        return this.applyDelta(message);
      case 'entity_appear':
        return this.applyEntityAppear(message);
      case 'entity_disappear':
        return this.applyEntityDisappear(message);
      case 'position_correction':
        return this.applyPositionCorrection(message);
      case 'region_context':
        return this.applyRegionContext(message);
      case 'trade_notice':
        return this.applyTradeNotice(message);
      case 'party_notice':
        return this.applyPartyNotice(message);
      case 'chat_message':
        return this.applyChatMessage(message);
      default:
        return { changed: false };
    }
  }

  private applyAck(message: AckMessage): { changed: boolean } {
    const pending = this.pendingCommands.get(message.command_id);
    if (!pending) {
      return { changed: false };
    }
    pending.status = 'acked';
    return { changed: false };
  }

  private applyReject(commandId: string | undefined, reasonCode: string, message: string): { changed: boolean } {
    if (commandId) {
      const pending = this.pendingCommands.get(commandId);
      if (pending) {
        pending.status = 'rejected';
        pending.reasonCode = reasonCode;
      }
    }
    this.handleMovementReject(reasonCode);
    this.pushLog(formatRejectMessage(reasonCode, message, CHAT_MESSAGE_MAX_LENGTH), 'warning');
    return { changed: true };
  }

  private applyDelta(message: DeltaMessage): { changed: boolean } {
    if (message.revision <= this.lastRevision) {
      return { changed: false };
    }
    if (message.revision > this.lastRevision+1) {
      this.desyncState = 'revision_gap';
      this.pushLog('Online revision gap detected. Command flow is now blocked until explicit reset.', 'danger');
      return { changed: true };
    }

    const previousLevel = this.authoritativeLevel ?? this.character.level;
    const previousQuest = this.authoritativeQuest ? cloneQuestState(this.authoritativeQuest) : null;
    const previousPets = cloneOwnedPets(this.authoritativePets);
    this.lastRevision = message.revision;
    const pending = this.pendingCommands.get(message.applies_to_command_id);
    if (pending) {
      pending.status = 'applied';
    }

    const self = message.self ?? {};
    const maybePosition = (self.position ?? null) as Partial<Vec2> | null;
    const nextPosition =
      maybePosition && typeof maybePosition.x === 'number' && typeof maybePosition.z === 'number'
        ? { x: maybePosition.x, z: maybePosition.z }
        : null;
    if (typeof self.geodata_version === 'string') {
      this.geodataVersion = self.geodata_version;
    }
    const authoritativePath = parseAuthoritativePath(self.authoritative_path);
    if (authoritativePath) {
      this.pendingPathPreview = [];
      this.authoritativePathPreview = cloneVecPath(authoritativePath);
    }
    if (nextPosition) {
      if (authoritativePath) {
        this.applyAuthoritativeMovementUpdate(nextPosition, authoritativePath);
      } else {
        this.reconcileAuthoritativePosition(nextPosition, { source: 'delta' });
      }
    }
    if (typeof self.facing === 'number') {
      this.projectedFacing = self.facing;
    }
    const maybeTargetId = self.target_id;
    if (typeof maybeTargetId === 'string' || maybeTargetId === null) {
      this.targetId = maybeTargetId;
    }
    const maybeCooldowns = self.cooldowns as Record<string, number> | undefined;
    if (maybeCooldowns && typeof maybeCooldowns === 'object') {
      this.syncCooldownSnapshot(maybeCooldowns, message.emitted_at_ms);
    }
    const maybeCP = parseAuthoritativeCP(self.cp);
    if (maybeCP !== null) {
      this.authoritativeCP = maybeCP;
    }
    const maybeHP = parseAuthoritativeHP(self.hp);
    if (maybeHP !== null) {
      this.authoritativeHP = maybeHP;
    }
    const maybeMP = parseAuthoritativeMP(self.mp);
    if (maybeMP !== null) {
      this.authoritativeMP = maybeMP;
    }
    const maybeXP = parseAuthoritativeXP(self.xp);
    if (maybeXP !== null) {
      this.authoritativeXP = maybeXP;
    }
    const maybeLevel = parseAuthoritativeLevel(self.level);
    if (maybeLevel !== null) {
      this.authoritativeLevel = maybeLevel;
    }
    const currentLevel = this.authoritativeLevel ?? this.character.level;
    if (self.known_skills !== undefined) {
      this.authoritativeKnownSkills = parseKnownSkills(self.known_skills, this.authoritativeBaseClass, currentLevel);
    } else if (maybeLevel !== null && maybeLevel !== previousLevel) {
      this.authoritativeKnownSkills = getLearnedSkillsForCharacter(this.authoritativeBaseClass, currentLevel);
    }
    if (self.hotbar !== undefined) {
      this.authoritativeHotbar = parseHotbarState(self.hotbar, this.authoritativeBaseClass);
    }
    if (self.pets !== undefined) {
      this.authoritativePets = parseOwnedPets(self.pets);
    }
    if (self.party !== undefined) {
      this.authoritativeParty = self.party === null ? null : parsePartySnapshot(self.party);
    }
    if (self.party_invites !== undefined) {
      this.authoritativePartyInvites = parsePartyInvites(self.party_invites);
    }
    const maybeDead = parseAuthoritativeDead(self.dead);
    if (maybeDead !== null) {
      const wasDead = this.authoritativeDead;
      this.authoritativeDead = maybeDead;
      if (this.authoritativeDead) {
        this.stopProjectedMovement();
        this.pendingPathPreview = [];
        this.authoritativePathPreview = [];
        if (!wasDead) {
          this.pushLog('You are dead. Command flow is blocked until authoritative return.', 'danger');
        }
      } else if (wasDead) {
        this.pushLog('You return with restored vitality.', 'success');
      }
    }
    const maybeStats = parseAuthoritativeStats(self.stats);
    if (maybeStats) {
      this.authoritativeStats = maybeStats;
      if (this.authoritativeCP !== null) {
        this.authoritativeCP = Math.min(this.authoritativeCP, maybeStats.maxCp);
      }
      if (this.authoritativeHP !== null) {
        this.authoritativeHP = Math.min(this.authoritativeHP, maybeStats.maxHp);
      }
      if (this.authoritativeMP !== null) {
        this.authoritativeMP = Math.min(this.authoritativeMP, maybeStats.maxMp);
      }
    }
    const nextQuest = parseQuestSnapshot(self.quest);
    if (nextQuest) {
      this.authoritativeQuest = nextQuest;
    }
    if (self.npc_interaction !== undefined) {
      this.activeNpcInteraction = parseNpcInteractionSnapshot(self.npc_interaction);
    }
    const itemSnapshot = snapshotFromDelta(message.inventory, message.equipment, message.warehouse);
    if (itemSnapshot) {
      this.authoritativeItems = toAuthoritativeItems(itemSnapshot);
    }
    const maybeEntities = message.entities;
    if (Array.isArray(maybeEntities)) {
      const entityProjectionNowMs = Date.now();
      for (const entityPatch of maybeEntities) {
        const entityId = typeof entityPatch.entity_id === 'string' ? entityPatch.entity_id : null;
        if (!entityId) {
          continue;
        }
        const existing = this.entities.get(entityId);
        if (!existing) {
          continue;
        }
        const maybeEntityPosition = (entityPatch.position ?? null) as Partial<Vec2> | null;
        const nextPosition =
          maybeEntityPosition && typeof maybeEntityPosition.x === 'number' && typeof maybeEntityPosition.z === 'number'
            ? { x: maybeEntityPosition.x, z: maybeEntityPosition.z }
            : existing.position;
        const nextState = { ...existing.state };
        const previousHp = typeof existing.state.hp === 'number' ? existing.state.hp : null;
        const wasAlive = existing.state.alive !== false;
        for (const [key, value] of Object.entries(entityPatch)) {
          if (key === 'entity_id' || key === 'position') {
            continue;
          }
          nextState[key] = value;
        }
        if (typeof entityPatch.hp === 'number') {
          nextState.hp = entityPatch.hp;
        }
        if (typeof entityPatch.alive === 'boolean') {
          nextState.alive = entityPatch.alive;
        }
        const nextHp = typeof nextState.hp === 'number' ? nextState.hp : previousHp;
        if (previousHp !== null && nextHp !== null && nextHp < previousHp) {
          this.pushFloatingText(`-${previousHp - nextHp}`, '#ff8d7b', existing.position, entityId);
        }
        if (wasAlive && nextState.alive === false) {
          this.pushLog(`${this.describeEntity(existing)} is defeated.`, 'success');
          if (this.targetId === entityId) {
            this.targetId = null;
          }
        }
        if (existing.entityType === 'player') {
          this.reconcileRemotePlayerProjection(existing, nextPosition, nextState, entityProjectionNowMs);
        }
        this.entities.set(entityId, {
          ...existing,
          position: nextPosition,
          state: nextState,
        });
      }
    }

    this.clearInvalidTargetIfNeeded();
    this.logQuestChange(previousQuest, this.authoritativeQuest);
    this.logPetRosterChange(previousPets, this.authoritativePets);
    if (previousLevel !== null && this.authoritativeLevel !== null && this.authoritativeLevel > previousLevel) {
      this.pushLog(`You reach level ${this.authoritativeLevel}. Vitality surges back to full.`, 'success');
    }

    return { changed: true };
  }

  private applyRegionContext(message: RegionContextMessage): { changed: boolean } {
    if (message.region_revision < this.lastRegionRevision) {
      return { changed: false };
    }
    this.lastRegionRevision = message.region_revision;
    this.regionId = message.region_id;
    this.geodataVersion = message.geodata_version;
    this.incomingTradeOffer = null;
    this.outgoingTradeOffer = null;
    this.activeNpcInteraction = null;
    this.pendingPathPreview = [];
    this.authoritativePathPreview = [];
    this.setProjectedPositionInstantly(message.self_position);
    this.entities.clear();
    this.remotePlayerProjections.clear();
    const projectionNowMs = Date.now();
    for (const [entityId, entity] of toEntityMap(message)) {
      this.entities.set(entityId, entity);
      if (entity.entityType === 'player') {
        this.seedRemotePlayerProjection(entity, projectionNowMs);
      }
    }
    return { changed: true };
  }

  private applyEntityAppear(message: EntityAppearMessage): { changed: boolean } {
    if (message.region_revision <= this.lastRegionRevision) {
      return { changed: false };
    }
    if (message.region_revision > this.lastRegionRevision+1) {
      this.desyncState = 'region_revision_gap';
      this.pushLog('Region revision gap detected. Command flow is now blocked until explicit reset.', 'danger');
      return { changed: true };
    }
    this.lastRegionRevision = message.region_revision;
    const entity: OnlineEntityState = {
      entityId: message.entity.entity_id,
      entityType: message.entity.entity_type,
      templateId: message.entity.template_id,
      position: { ...message.entity.position },
      state: { ...message.entity.state },
    };
    this.entities.set(message.entity.entity_id, entity);
    if (entity.entityType === 'player') {
      this.seedRemotePlayerProjection(entity);
    }
    return { changed: true };
  }

  private applyEntityDisappear(message: EntityDisappearMessage): { changed: boolean } {
    if (message.region_revision <= this.lastRegionRevision) {
      return { changed: false };
    }
    if (message.region_revision > this.lastRegionRevision+1) {
      this.desyncState = 'region_revision_gap';
      this.pushLog('Region revision gap detected. Command flow is now blocked until explicit reset.', 'danger');
      return { changed: true };
    }
    this.lastRegionRevision = message.region_revision;
    this.entities.delete(message.entity_id);
    this.remotePlayerProjections.delete(message.entity_id);
    if (this.activeNpcInteraction?.npcId === message.entity_id) {
      this.activeNpcInteraction = null;
    }
    if (this.targetId === message.entity_id) {
      this.targetId = null;
    }
    return { changed: true };
  }

  private applyPositionCorrection(message: PositionCorrectionMessage): { changed: boolean } {
    if (message.applies_to_command_seq < this.lastAppliedCorrectionSeq) {
      return { changed: false };
    }
    this.lastAppliedCorrectionSeq = message.applies_to_command_seq;
    this.pendingPathPreview = [];
    this.authoritativePathPreview = [];
    this.projectedFacing = message.facing;
    const hardSnap =
      message.reason === 'path_blocked' ||
      message.reason === 'path_unreachable' ||
      message.reason === 'geodata_mismatch';
    this.reconcileAuthoritativePosition({ ...message.position }, { source: 'correction', hardSnap });
    return { changed: true };
  }

  private applyTradeNotice(message: TradeNoticeMessage): { changed: boolean } {
    if (message.command_id) {
      const pending = this.pendingCommands.get(message.command_id);
      if (pending) {
        pending.status = 'applied';
      }
    }

    const nextOffer: PendingTradeOfferState = {
      offerId: message.offer_id,
      direction: message.direction,
      counterpartyCharacterId: message.counterparty_character_id,
      counterpartyName: message.counterparty_name,
      itemTemplateId: message.item_template_id,
      quantity: message.quantity,
    };

    if (message.status === 'pending') {
      if (message.direction === 'incoming') {
        this.incomingTradeOffer = nextOffer;
      } else {
        this.outgoingTradeOffer = nextOffer;
      }
    } else {
      if (this.incomingTradeOffer?.offerId === message.offer_id) {
        this.incomingTradeOffer = null;
      }
      if (this.outgoingTradeOffer?.offerId === message.offer_id) {
        this.outgoingTradeOffer = null;
      }
    }

    this.pushLog(
      message.message,
      message.status === 'accepted' ? 'success' : message.status === 'pending' ? 'neutral' : 'warning',
      'trade',
    );
    return { changed: true };
  }

  private applyPartyNotice(message: PartyNoticeMessage): { changed: boolean } {
    if (message.command_id) {
      const pending = this.pendingCommands.get(message.command_id);
      if (pending) {
        pending.status = 'applied';
      }
    }

    const tone =
      message.status === 'invite_accepted' || message.status === 'member_joined'
        ? 'success'
        : message.status === 'invite_received' || message.status === 'invite_sent'
          ? 'neutral'
          : 'warning';
    this.pushLog(message.message, tone, 'party');
    return { changed: true };
  }

  private applyChatMessage(message: ChatMessageServerMessage): { changed: boolean } {
    if (message.command_id) {
      const pending = this.pendingCommands.get(message.command_id);
      if (pending) {
        pending.status = 'applied';
      }
    }

    this.pushLog(formatChatMessage(message, this.character.character_id), 'neutral', message.channel);
    return { changed: true };
  }

  private logQuestChange(previousQuest: GameState['quest'] | null, nextQuest: GameState['quest'] | null): void {
    if (!previousQuest || !nextQuest || previousQuest.id !== nextQuest.id) {
      return;
    }

    if (
      nextQuest.status === 'active' &&
      previousQuest.status !== 'active' &&
      previousQuest.status !== 'ready_to_turn_in' &&
      previousQuest.status !== 'completed'
    ) {
      this.pushLog('Selka marks the Mireling cull on your tracker.', 'success');
      return;
    }

    if (nextQuest.progress > previousQuest.progress && nextQuest.status !== 'completed') {
      this.pushLog(
        `${nextQuest.title} progress: ${nextQuest.progress}/${nextQuest.goal} Mirelings defeated.`,
        'success',
      );
    }

    if (nextQuest.status === 'ready_to_turn_in' && previousQuest.status !== 'ready_to_turn_in') {
      this.pushLog('The wardkeeper asked for 3 Mirelings. Return to Selka for your mantle.', 'success');
      return;
    }

    if (nextQuest.status === 'completed' && previousQuest.status !== 'completed') {
      this.pushLog('Selka rewards you with the Wardkeeper Mantle.', 'success');
    }
  }

  private logPetRosterChange(previousPets: OwnedPetState[], nextPets: OwnedPetState[]): void {
    const previousById = new Map(previousPets.map((pet) => [pet.petInstanceId, pet]));
    for (const pet of nextPets) {
      const previous = previousById.get(pet.petInstanceId);
      if (!previous) {
        this.pushLog(`${pet.name} now answers to your call.`, 'success');
        continue;
      }
      if (!previous.mounted && pet.mounted) {
        this.pushLog(`You mount ${pet.name}.`, 'success');
        continue;
      }
      if (previous.mounted && !pet.mounted) {
        this.pushLog(`You dismount ${pet.name}.`, 'success');
        continue;
      }
      if (!previous.summoned && pet.summoned) {
        this.pushLog(`${pet.name} answers the summon.`, 'success');
        continue;
      }
      if (previous.summoned && !pet.summoned) {
        this.pushLog(`${pet.name} returns to rest.`, 'neutral');
      }
    }
  }

  private pushLog(
    text: string,
    tone: GameState['logs'][number]['tone'],
    channel: GameState['logs'][number]['channel'] = 'system',
  ): void {
    this.logs = [
      {
        id: `online_log_${this.logs.length + 1}_${Date.now()}`,
        text,
        tone,
        channel,
      },
      ...this.logs,
    ].slice(0, 10);
  }

  private pushFloatingText(text: string, color: string, position: Vec2, entityId?: string): void {
    this.floatingTexts = [
      ...this.floatingTexts,
      {
        id: `online_float_${Date.now()}_${this.floatingTexts.length + 1}`,
        text,
        color,
        position: { ...position },
        entityId,
        expiresAtMs: Date.now() + FLOATING_TEXT_TTL_MS,
      },
    ].slice(-12);
  }

  private projectCooldowns(): Record<string, SkillAvailabilityProjection> {
    const now = Date.now();
    const cooldowns: Record<string, SkillAvailabilityProjection> = {};
    for (const [skillId, entry] of this.cooldownAuthority.entries()) {
      if (entry.authorityState === 'ready') {
        cooldowns[skillId] = {
          visualRemainingMs: 0,
          requestBlocked: false,
          authorityState: 'ready',
        };
        continue;
      }

      const remaining =
        entry.authoritativeEndsAtMs === null ? 0 : Math.max(0, entry.authoritativeEndsAtMs - now);
      const authorityState: SkillAvailabilityState =
        remaining > 0 ? 'cooling' : 'cooldown_elapsed_waiting_authority';
      cooldowns[skillId] = {
        visualRemainingMs: remaining,
        requestBlocked: remaining > 0,
        authorityState,
      };
      entry.authorityState = authorityState;
    }
    return cooldowns;
  }

  private syncCooldownSnapshot(cooldowns: Record<string, number>, emittedAtMs: number): void {
    const seen = new Set<string>();
    for (const [skillId, remainingMs] of Object.entries(cooldowns)) {
      seen.add(skillId);
      if (typeof remainingMs === 'number' && remainingMs > 0) {
        this.cooldownAuthority.set(skillId, {
          authorityState: 'cooling',
          authoritativeEndsAtMs: emittedAtMs + remainingMs,
        });
        continue;
      }
      this.cooldownAuthority.set(skillId, {
        authorityState: 'ready',
        authoritativeEndsAtMs: null,
      });
    }

    for (const skillId of this.cooldownAuthority.keys()) {
      if (!seen.has(skillId)) {
        this.cooldownAuthority.set(skillId, {
          authorityState: 'ready',
          authoritativeEndsAtMs: null,
        });
      }
    }
  }

  private projectFloatingTexts(): GameState['floatingTexts'] {
    const now = Date.now();
    this.floatingTexts = this.floatingTexts.filter((entry) => entry.expiresAtMs > now);
    return this.floatingTexts.map((entry) => ({
      id: entry.id,
      text: entry.text,
      color: entry.color,
      position: { ...entry.position },
      entityId: entry.entityId,
      ttlMs: entry.expiresAtMs - now,
    }));
  }

  private primaryOwnedPet(): OwnedPetState | null {
    return this.authoritativePets[0] ?? null;
  }

  private mountedOwnedPet(): OwnedPetState | null {
    return this.authoritativePets.find((pet) => pet.mounted) ?? null;
  }

  private getLivingMobTarget(targetId: string | null): OnlineEntityState | null {
    if (!targetId) {
      return null;
    }
    const entity = this.entities.get(targetId);
    if (!entity || entity.entityType !== 'mob' || entity.state.alive === false) {
      return null;
    }
    return entity;
  }

  private clearInvalidTargetIfNeeded(): void {
    if (!this.getLivingMobTarget(this.targetId)) {
      this.targetId = null;
    }
  }

  private projectedMoveSpeed(): number {
    if (this.authoritativeStats?.moveSpeed) {
      return this.authoritativeStats.moveSpeed;
    }
    return gameTemplates.archetypes[getArchetypeIdForBaseClass(this.authoritativeBaseClass)].baseMoveSpeed;
  }

  private applyAuthoritativeMovementUpdate(nextPosition: Vec2, authoritativePath: Vec2[]): void {
    this.advanceProjectedMovement(Date.now());
    this.authoritativePlayerPosition = { ...nextPosition };

    if (authoritativePath.length === 0) {
      this.reconcileAuthoritativePosition(nextPosition, { source: 'delta' });
      return;
    }

    if (distance(this.projectedPlayerPosition, nextPosition) >= ONLINE_RECONCILIATION_EXTREME_SNAP_DISTANCE) {
      this.setProjectedPositionInstantly(nextPosition);
    }

    const blendedPath = this.buildProjectedPathFromAuthoritative(authoritativePath);
    if (blendedPath.length === 0) {
      if (distance(this.projectedPlayerPosition, nextPosition) <= ONLINE_RECONCILIATION_SETTLE_EPSILON) {
        this.projectedPlayerPosition = { ...nextPosition };
        this.stopProjectedMovement();
        return;
      }
      this.beginProjectedPath([nextPosition], 'correction');
      return;
    }

    this.beginProjectedPath(blendedPath, 'authoritative');
  }

  private buildProjectedPathFromAuthoritative(authoritativePath: Vec2[]): Vec2[] {
    if (authoritativePath.length === 0) {
      return [];
    }
    if (authoritativePath.length === 1) {
      return distance(this.projectedPlayerPosition, authoritativePath[0]) <= ONLINE_RECONCILIATION_SETTLE_EPSILON
        ? []
        : [{ ...authoritativePath[0] }];
    }

    let bestPoint = authoritativePath[0];
    let bestDistance = Number.POSITIVE_INFINITY;
    let nextIndex = 1;
    for (let index = 0; index < authoritativePath.length - 1; index += 1) {
      const segment = closestPointOnSegment(this.projectedPlayerPosition, authoritativePath[index], authoritativePath[index + 1]);
      const segmentDistance = distance(this.projectedPlayerPosition, segment.point);
      if (segmentDistance < bestDistance) {
        bestDistance = segmentDistance;
        bestPoint = segment.point;
        nextIndex = segment.ratio >= 1 - 0.0001 ? index + 2 : index + 1;
      }
    }

    const blendedPath: Vec2[] = [];
    if (bestDistance > ONLINE_RECONCILIATION_SETTLE_EPSILON) {
      blendedPath.push(bestPoint);
    }
    for (let index = nextIndex; index < authoritativePath.length; index += 1) {
      const waypoint = authoritativePath[index];
      const previous = blendedPath[blendedPath.length - 1];
      if (previous && distance(previous, waypoint) <= ONLINE_RECONCILIATION_SETTLE_EPSILON) {
        continue;
      }
      if (distance(this.projectedPlayerPosition, waypoint) <= ONLINE_RECONCILIATION_SETTLE_EPSILON && index !== authoritativePath.length - 1) {
        continue;
      }
      blendedPath.push({ ...waypoint });
    }

    return blendedPath;
  }

  private predictionLeashDistance(): number {
    switch (this.projectedPathMode) {
      case 'pending':
        return ONLINE_PREDICTION_PENDING_LEASH_DISTANCE;
      case 'authoritative':
        return ONLINE_PREDICTION_AUTH_PATH_LEASH_DISTANCE;
      default:
        return Number.POSITIVE_INFINITY;
    }
  }

  private applyPredictionLeash(current: Vec2, proposed: Vec2): Vec2 {
    const leashDistance = this.predictionLeashDistance();
    if (!Number.isFinite(leashDistance)) {
      return proposed;
    }

    const currentLead = distance(current, this.authoritativePlayerPosition);
    const proposedLead = distance(proposed, this.authoritativePlayerPosition);
    if (proposedLead <= leashDistance || proposedLead <= currentLead) {
      return proposed;
    }
    if (currentLead >= leashDistance) {
      return current;
    }

    let leashed = proposed;
    const softZoneStart = leashDistance * ONLINE_PREDICTION_LEASH_SOFT_ZONE_RATIO;
    if (currentLead > softZoneStart) {
      const easingRatio = Math.max(0, (leashDistance - currentLead) / Math.max(leashDistance - softZoneStart, 0.0001));
      leashed = moveTowards(current, proposed, distance(current, proposed) * easingRatio);
    }

    const leashedLead = distance(leashed, this.authoritativePlayerPosition);
    if (leashedLead <= leashDistance) {
      return leashed;
    }

    const totalStep = distance(current, leashed);
    if (totalStep <= 0.0001) {
      return current;
    }
    const clampRatio = Math.max(0, Math.min(1, (leashDistance - currentLead) / Math.max(leashedLead - currentLead, 0.0001)));
    return lerpPoint(current, leashed, clampRatio);
  }

  private advanceProjectedMovement(nowMs: number): void {
    const deltaMs = Math.max(0, Math.min(nowMs - this.lastProjectionAtMs, 32));
    this.lastProjectionAtMs = nowMs;

    if (!this.projectedMoveTarget || deltaMs === 0) {
      return;
    }

    let remainingStep = (this.projectedMoveSpeed() * deltaMs) / 1000;
    while (remainingStep > 0 && this.projectedMoveTarget) {
      this.updateProjectedFacing(this.projectedMoveTarget);
      const proposedPosition = moveTowards(this.projectedPlayerPosition, this.projectedMoveTarget, remainingStep);
      const nextPosition = this.applyPredictionLeash(this.projectedPlayerPosition, proposedPosition);
      const advancedDistance = distance(this.projectedPlayerPosition, nextPosition);
      if (advancedDistance <= 0.0001) {
        return;
      }

      this.projectedPlayerPosition = nextPosition;
      remainingStep = Math.max(0, remainingStep - advancedDistance);
      if (distance(this.projectedPlayerPosition, this.projectedMoveTarget) > ONLINE_RECONCILIATION_SETTLE_EPSILON) {
        return;
      }

      this.projectedPlayerPosition = { ...this.projectedMoveTarget };
      this.advanceProjectedPathTarget();
    }
  }

  private reconcileAuthoritativePosition(
    nextPosition: Vec2,
    options: { source: 'delta' | 'correction'; hardSnap?: boolean },
  ): void {
    this.advanceProjectedMovement(Date.now());
    const correctionDistance = distance(this.projectedPlayerPosition, nextPosition);
    const hardSnap = options.hardSnap === true || correctionDistance >= ONLINE_RECONCILIATION_EXTREME_SNAP_DISTANCE;
    if (hardSnap) {
      this.setProjectedPositionInstantly(nextPosition);
      return;
    }

    this.authoritativePlayerPosition = { ...nextPosition };

    if (correctionDistance <= ONLINE_RECONCILIATION_SETTLE_EPSILON) {
      this.projectedPlayerPosition = { ...nextPosition };
      if (!this.projectedMoveTarget || distance(this.projectedMoveTarget, nextPosition) <= ONLINE_RECONCILIATION_SETTLE_EPSILON) {
        this.stopProjectedMovement();
      }
      return;
    }

    if (options.source === 'correction') {
      this.beginProjectedPath([nextPosition], 'correction');
      return;
    }

    const alreadyMovingTowardAcceptedPoint =
      this.projectedMoveTarget !== null &&
      distance(this.projectedMoveTarget, nextPosition) <= ONLINE_RECONCILIATION_TARGET_EPSILON;
    if (this.projectedPathMode === 'pending' || this.projectedPathMode === 'authoritative' || alreadyMovingTowardAcceptedPoint) {
      return;
    }

    this.beginProjectedPath([nextPosition], 'correction');
  }

  private beginProjectedPath(points: Vec2[], mode: ProjectedPathMode): void {
    const nextQueue = points
      .filter((point, index, collection) => {
        const previous = index === 0 ? this.projectedPlayerPosition : collection[index - 1];
        return distance(previous, point) > ONLINE_RECONCILIATION_SETTLE_EPSILON;
      })
      .map((point) => ({ ...point }));
    this.projectedPathQueue = nextQueue;
    this.projectedPathMode = nextQueue.length > 0 ? mode : 'none';
    this.projectedMoveTarget = nextQueue[0] ?? null;
    if (this.projectedMoveTarget) {
      this.updateProjectedFacing(this.projectedMoveTarget);
    }
  }

  private advanceProjectedPathTarget(): void {
    if (this.projectedPathQueue.length > 0) {
      this.projectedPathQueue.shift();
    }
    this.projectedMoveTarget = this.projectedPathQueue[0] ?? null;
    if (!this.projectedMoveTarget) {
      this.projectedPathMode = 'none';
      return;
    }
    this.updateProjectedFacing(this.projectedMoveTarget);
  }

  private stopProjectedMovement(): void {
    this.projectedMoveTarget = null;
    this.projectedPathQueue = [];
    this.projectedPathMode = 'none';
  }

  private setProjectedPositionInstantly(position: Vec2): void {
    this.authoritativePlayerPosition = { ...position };
    this.projectedPlayerPosition = { ...position };
    this.stopProjectedMovement();
    this.lastProjectionAtMs = Date.now();
  }

  private seedRemotePlayerProjection(entity: OnlineEntityState, nowMs = Date.now()): void {
    if (entity.entityType !== 'player') {
      return;
    }
    const facing = typeof entity.state.facing === 'number' ? entity.state.facing : 0;
    this.remotePlayerProjections.set(entity.entityId, {
      fromPosition: { ...entity.position },
      toPosition: { ...entity.position },
      renderPosition: { ...entity.position },
      fromFacing: facing,
      toFacing: facing,
      renderFacing: facing,
      startedAtMs: nowMs,
      endsAtMs: nowMs,
    });
  }

  private advanceRemotePlayerProjections(nowMs: number): void {
    for (const projection of this.remotePlayerProjections.values()) {
      this.advanceRemotePlayerProjection(projection, nowMs);
    }
  }

  private advanceRemotePlayerProjection(projection: RemotePlayerProjection, nowMs: number): void {
    if (projection.endsAtMs <= projection.startedAtMs || nowMs >= projection.endsAtMs) {
      projection.renderPosition = { ...projection.toPosition };
      projection.renderFacing = projection.toFacing;
      return;
    }
    if (nowMs <= projection.startedAtMs) {
      projection.renderPosition = { ...projection.fromPosition };
      projection.renderFacing = projection.fromFacing;
      return;
    }

    const ratio = Math.max(0, Math.min(1, (nowMs - projection.startedAtMs) / (projection.endsAtMs - projection.startedAtMs)));
    projection.renderPosition = lerpPoint(projection.fromPosition, projection.toPosition, ratio);
    projection.renderFacing = lerpAngle(projection.fromFacing, projection.toFacing, ratio);
  }

  private reconcileRemotePlayerProjection(
    existing: OnlineEntityState,
    nextPosition: Vec2,
    nextState: Record<string, unknown>,
    nowMs: number,
  ): void {
    let projection = this.remotePlayerProjections.get(existing.entityId);
    if (!projection) {
      this.seedRemotePlayerProjection(existing, nowMs);
      projection = this.remotePlayerProjections.get(existing.entityId);
      if (!projection) {
        return;
      }
    }
    this.advanceRemotePlayerProjection(projection, nowMs);

    const nextFacing = typeof nextState.facing === 'number' ? nextState.facing : projection.toFacing;
    const visualStartPosition = { ...projection.renderPosition };
    const visualStartFacing = projection.renderFacing;
    const positionChanged = distance(projection.toPosition, nextPosition) > REMOTE_PLAYER_SETTLE_EPSILON;
    const facingChanged = Math.abs(shortestAngleDelta(projection.toFacing, nextFacing)) > 0.001;

    if (!positionChanged && !facingChanged) {
      return;
    }

    if (distance(visualStartPosition, nextPosition) >= REMOTE_PLAYER_SNAP_DISTANCE) {
      projection.fromPosition = { ...nextPosition };
      projection.toPosition = { ...nextPosition };
      projection.renderPosition = { ...nextPosition };
      projection.fromFacing = nextFacing;
      projection.toFacing = nextFacing;
      projection.renderFacing = nextFacing;
      projection.startedAtMs = nowMs;
      projection.endsAtMs = nowMs;
      return;
    }

    projection.fromPosition = visualStartPosition;
    projection.toPosition = { ...nextPosition };
    projection.renderPosition = visualStartPosition;
    projection.fromFacing = visualStartFacing;
    projection.toFacing = nextFacing;
    projection.renderFacing = visualStartFacing;
    projection.startedAtMs = nowMs;
    projection.endsAtMs = nowMs + REMOTE_PLAYER_INTERPOLATION_MS;
    if (
      distance(projection.fromPosition, projection.toPosition) <= REMOTE_PLAYER_SETTLE_EPSILON &&
      Math.abs(shortestAngleDelta(projection.fromFacing, projection.toFacing)) <= 0.001
    ) {
      projection.endsAtMs = nowMs;
      this.advanceRemotePlayerProjection(projection, nowMs);
    }
  }

  private updateProjectedFacing(target: Vec2): void {
    if (distance(this.projectedPlayerPosition, target) < 0.001) {
      return;
    }
    this.projectedFacing = Math.atan2(target.z - this.projectedPlayerPosition.z, target.x - this.projectedPlayerPosition.x);
  }

  private describeEntity(entity: OnlineEntityState): string {
    if (entity.entityType === 'player') {
      return typeof entity.state.name === 'string' ? entity.state.name : entity.entityId;
    }
    if (entity.entityType === 'mob') {
      return gameTemplates.mobTemplates[entity.templateId]?.name ?? entity.templateId;
    }
    return entity.templateId;
  }

  private currentPathDestination(): Vec2 | null {
    const path = this.authoritativePathPreview.length > 0 ? this.authoritativePathPreview : this.pendingPathPreview;
    const destination = path[path.length - 1];
    return destination ? { ...destination } : null;
  }

  private handleMovementReject(reasonCode: string): void {
    if (!reasonCode.startsWith('movement.')) {
      return;
    }
    const rejectedPoint = this.pendingPathPreview[this.pendingPathPreview.length - 1];
    this.pendingPathPreview = [];
    this.authoritativePathPreview = [];
    this.stopProjectedMovement();
    if (!rejectedPoint) {
      return;
    }

    switch (reasonCode) {
      case 'movement.destination_blocked':
        this.pushFloatingText('Blocked', '#ff8b6f', rejectedPoint);
        break;
      case 'movement.destination_out_of_bounds':
        this.pushFloatingText('Out of bounds', '#ffb25f', rejectedPoint);
        break;
      case 'movement.path_unreachable':
      case 'movement.path_budget_exceeded':
        this.pushFloatingText('Unreachable', '#ffd36e', rejectedPoint);
        break;
      case 'movement.geodata_unavailable':
      case 'movement.geodata_mismatch':
        this.pushFloatingText('Route lost', '#ff8b6f', rejectedPoint);
        break;
      default:
        break;
    }
  }
}
