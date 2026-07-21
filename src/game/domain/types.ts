export type EntityId = string;

export type Vec2 = {
  x: number;
  z: number;
};

export type RegionId = 'stonecross_plaza' | 'dawn_plaza' | 'gate_road' | 'gloam_field' | 'ruin_hollow';

export type BaseClass = 'Fighter' | 'Mage';

export type CharacterRace = 'Human' | 'Elf' | 'Dark Elf' | 'Orc' | 'Dwarf';

export type CharacterSex = 'Male' | 'Female';

export type AppearanceOptionIndex = 0 | 1 | 2;

export type SkillCategory = 'active' | 'passive';

export type SkillTargetType = 'single_target_enemy' | 'target_centered_aoe' | 'passive';

export type ItemKind = 'currency' | 'weapon' | 'armor' | 'quest' | 'material' | 'consumable';

export type ItemContainer = 'inventory' | 'equipment' | 'warehouse' | 'ground';

export type EquipSlot = 'weapon' | 'chest' | 'gloves' | 'boots';

export type QuestStatus = 'available' | 'active' | 'ready_to_turn_in' | 'completed';
export type PetKind = 'pet' | 'mount' | 'pet_mount';

export interface SkillTemplate {
  id: string;
  name: string;
  description: string;
  category: SkillCategory;
  baseClass: BaseClass;
  unlockLevel: number;
  iconKey: string;
  iconTint: string;
  targetType: SkillTargetType;
  castTimeMs: number;
  cooldownMs: number;
  mpCost: number;
  range: number;
  radius?: number;
  power: number;
  maxTargets: number;
}

export interface ItemTemplate {
  id: string;
  name: string;
  description: string;
  kind: ItemKind;
  stackable: boolean;
  equipSlot?: EquipSlot;
  statBonuses?: Partial<DerivedStats>;
  appearance?: {
    weaponModel?: 'none' | 'spear' | 'staff';
    chestModel?: 'none' | 'mantle' | 'robe';
    tint?: string;
  };
}

export interface ItemInstance {
  id: string;
  templateId: string;
  quantity: number;
  container: ItemContainer;
  equipSlot?: EquipSlot;
  instanceAttributes?: Partial<DerivedStats>;
}

export interface ArchetypeTemplate {
  id: string;
  name: string;
  title: string;
  baseCp: number;
  baseHp: number;
  baseMp: number;
  baseAttack: number;
  baseDefense: number;
  baseMoveSpeed: number;
  cpGrowth: number;
  hpGrowth: number;
  mpGrowth: number;
  attackGrowth: number;
  defenseGrowth: number;
}

export interface DerivedStats {
  maxCp: number;
  maxHp: number;
  maxMp: number;
  attack: number;
  defense: number;
  moveSpeed: number;
}

export interface ActiveCast {
  skillId: string;
  targetId: EntityId;
  remainingMs: number;
  totalMs: number;
}

export interface QueuedSkillCast {
  skillId: string;
  targetId: EntityId;
}

export type SkillAvailabilityState = 'ready' | 'cooling' | 'cooldown_elapsed_waiting_authority';

export interface SkillAvailabilityProjection {
  visualRemainingMs: number;
  requestBlocked: boolean;
  authorityState: SkillAvailabilityState;
}

export interface PlayerKnownSkill {
  skillId: string;
  category: SkillCategory;
  unlockLevel: number;
}

export type HotbarEntryType = 'skill' | 'item' | 'action';

export type HotbarActionId =
  | 'basic_attack'
  | 'pick_up_nearby'
  | 'party_invite'
  | 'party_leave'
  | 'tame_target'
  | 'summon_pet'
  | 'dismiss_pet'
  | 'mount_pet'
  | 'dismount_pet'
  | 'toggle_walk_run';

export interface OwnedPetState {
  petInstanceId: string;
  petTemplateId: string;
  name: string;
  kind: PetKind;
  visualTemplateId: string;
  mountEligible: boolean;
  summoned: boolean;
  mounted: boolean;
}

export interface CompanionState {
  id: EntityId;
  ownerId: EntityId;
  ownerName: string;
  petTemplateId: string;
  visualTemplateId: string;
  name: string;
  kind: PetKind;
  mountEligible: boolean;
  mounted: boolean;
  position: Vec2;
  mountedByCharacterId: EntityId | null;
  followOwnerId: EntityId | null;
}

export interface PlayerHotbarSlot {
  slotIndex: number;
  entryType: HotbarEntryType | null;
  skillId: string | null;
  itemId?: string | null;
  actionId?: HotbarActionId | null;
}

export interface PlayerHotbarState {
  openBarCount: number;
  slots: PlayerHotbarSlot[];
}

export interface PlayerState {
  id: EntityId;
  name: string;
  race: CharacterRace;
  baseClass: BaseClass;
  sex: CharacterSex;
  hairStyle: AppearanceOptionIndex;
  hairColor: string;
  skinType: AppearanceOptionIndex;
  archetypeId: string;
  level: number;
  xp: number;
  cp: number;
  hp: number;
  mp: number;
  pvpFlagged: boolean;
  pvpFlagUntilMs: number | null;
  pvpKills: number;
  pkCount: number;
  karma: number;
  authoritativeStats?: DerivedStats | null;
  position: Vec2;
  facing: number;
  movementMode?: 'run' | 'walk';
  moveTarget: Vec2 | null;
  stationarySinceMs: number;
  lastIdleRegenAtMs: number;
  cast: ActiveCast | null;
  queuedSkill: QueuedSkillCast | null;
  queuedBasicAttackTargetId: EntityId | null;
  queuedLootId: EntityId | null;
  cooldowns: Record<string, number>;
  skillAvailability: Record<string, SkillAvailabilityProjection>;
  learnedSkills: PlayerKnownSkill[];
  hotbar: PlayerHotbarState;
  pets: OwnedPetState[];
  activePetId: EntityId | null;
  mountedPetId: EntityId | null;
  deadUntilMs: number | null;
}

export interface MobTemplate {
  id: string;
  name: string;
  level: number;
  personality: MobPersonality;
  tint: string;
  maxHp: number;
  attack: number;
  defense: number;
  moveSpeed: number;
  aggroRadius: number;
  attackRange: number;
  attackIntervalMs: number;
  xpReward: number;
  currencyDrop: number;
  guaranteedEquipmentTemplateId?: string;
}

export type MobPersonality = 'aggressive' | 'passive';

export interface MobState {
  id: EntityId;
  templateId: string;
  personality: MobPersonality;
  position: Vec2;
  spawnPoint: Vec2;
  hp: number;
  aiState: 'idle' | 'aggro' | 'dead';
  attackCooldownMs: number;
  respawnAtMs: number | null;
}

export interface LootDrop {
  id: EntityId;
  itemInstanceId: string;
  position: Vec2;
  label: string;
}

export interface NpcState {
  id: EntityId;
  name: string;
  title: string;
  position: Vec2;
}

export interface OtherPlayerState {
  id: EntityId;
  name: string;
  race: CharacterRace;
  baseClass: BaseClass;
  sex: CharacterSex;
  hairStyle: AppearanceOptionIndex;
  hairColor: string;
  skinType: AppearanceOptionIndex;
  archetypeId: string;
  level: number;
  cp: number;
  hp: number;
  dead: boolean;
  pvpFlagged: boolean;
  pvpFlagUntilMs: number | null;
  pvpKills: number;
  pkCount: number;
  karma: number;
  position: Vec2;
  facing: number;
  movementMode?: 'run' | 'walk';
  mountedPetId: EntityId | null;
}

export interface PendingTradeOfferState {
  offerId: string;
  direction: 'incoming' | 'outgoing';
  counterpartyCharacterId: string;
  counterpartyName: string;
  itemTemplateId: string;
  quantity: number;
}

export interface PartyMemberState {
  characterId: string;
  name: string;
  level: number;
  baseClass: BaseClass;
  hp: number;
  mp: number;
  online: boolean;
  isLeader: boolean;
}

export interface PartyState {
  partyId: string;
  leaderCharacterId: string;
  members: PartyMemberState[];
}

export interface PendingPartyInviteState {
  inviteId: string;
  partyId: string;
  inviterCharacterId: string;
  inviterName: string;
  expiresAtMs: number;
}

export interface ClanMemberState {
  characterId: string;
  name: string;
  level: number;
  baseClass: BaseClass;
  online: boolean;
  isLeader: boolean;
}

export interface ClanState {
  clanId: string;
  name: string;
  leaderCharacterId: string;
  members: ClanMemberState[];
}

export interface PendingClanInviteState {
  inviteId: string;
  clanId: string;
  clanName: string;
  inviterCharacterId: string;
  inviterName: string;
  expiresAtMs: number;
}

export interface AllianceMemberClanState {
  clanId: string;
  name: string;
  leaderCharacterId: string;
  leaderName: string;
  memberCount: number;
  isLeaderClan: boolean;
}

export interface AllianceState {
  allianceId: string;
  name: string;
  leaderClanId: string;
  leaderClanName: string;
  clanCap: number;
  members: AllianceMemberClanState[];
}

export interface PendingAllianceInviteState {
  inviteId: string;
  allianceId: string;
  allianceName: string;
  inviterCharacterId: string;
  inviterName: string;
  inviterClanId: string;
  inviterClanName: string;
  targetClanId: string;
  expiresAtMs: number;
}

export interface QuestState {
  id: string;
  title: string;
  description: string;
  status: QuestStatus;
  progress: number;
  goal: number;
}

export type LogChannel = 'system' | 'region' | 'party' | 'alliance' | 'whisper' | 'trade';

export interface LogEntry {
  id: string;
  text: string;
  tone: 'neutral' | 'success' | 'warning' | 'danger';
  channel?: LogChannel;
}

export interface FloatingText {
  id: string;
  text: string;
  color: string;
  position: Vec2;
  entityId?: EntityId;
  ttlMs: number;
}

export interface VendorOfferTemplate {
  id: string;
  npcId: EntityId;
  templateId: string;
  quantity: number;
  priceCurrencyTemplateId: string;
  priceAmount: number;
}

export interface ExchangeOfferTemplate {
  id: string;
  npcId: EntityId;
  templateId: string;
  quantity: number;
  costTemplateId: string;
  costAmount: number;
}

export interface NpcDialogState {
  npcId: EntityId;
  title: string;
  body: string;
  actionLabel?: string;
  actionId?: 'accept_task' | 'turn_in_task';
}

export interface GameTemplates {
  archetypes: Record<string, ArchetypeTemplate>;
  skills: Record<string, SkillTemplate>;
  itemTemplates: Record<string, ItemTemplate>;
  mobTemplates: Record<string, MobTemplate>;
  vendorOffers: Record<string, VendorOfferTemplate>;
  exchangeOffers: Record<string, ExchangeOfferTemplate>;
}

export interface GameState {
  timeMs: number;
  nextId: number;
  targetId: EntityId | null;
  destinationMarker: Vec2 | null;
  pendingPath: Vec2[];
  authoritativePath: Vec2[];
  player: PlayerState;
  otherPlayers: Record<EntityId, OtherPlayerState>;
  companions: Record<EntityId, CompanionState>;
  mobs: Record<EntityId, MobState>;
  loot: Record<EntityId, LootDrop>;
  items: Record<string, ItemInstance>;
  npcs: Record<EntityId, NpcState>;
  quest: QuestState;
  dialog: NpcDialogState | null;
  party: PartyState | null;
  partyInvites: PendingPartyInviteState[];
  clan: ClanState | null;
  clanInvites: PendingClanInviteState[];
  alliance: AllianceState | null;
  allianceInvites: PendingAllianceInviteState[];
  incomingTradeOffer: PendingTradeOfferState | null;
  outgoingTradeOffer: PendingTradeOfferState | null;
  logs: LogEntry[];
  floatingTexts: FloatingText[];
  equipmentAwardsGranted: string[];
  lastAutoSaveAtMs: number;
}

export type GameCommand =
  | { type: 'moveToPoint'; point: Vec2 }
  | { type: 'selectTarget'; targetId: EntityId | null }
  | { type: 'clearTarget' }
  | { type: 'useSkill'; skillId: string }
  | { type: 'basicAttack' }
  | { type: 'pickUpLoot'; lootId: EntityId }
  | { type: 'pickUpNearbyLoot' }
  | { type: 'useItem'; itemId: string }
  | { type: 'equipItem'; itemId: string }
  | { type: 'unequipItem'; slot: EquipSlot }
  | { type: 'splitItemStack'; itemId: string; quantity: number }
  | { type: 'mergeItemStacks'; sourceItemId: string; targetItemId: string }
  | { type: 'buyVendorOffer'; offerId: string; quantity: number }
  | { type: 'exchangeVendorOffer'; offerId: string; quantity: number }
  | { type: 'sellVendorItem'; itemId: string; quantity: number }
  | { type: 'depositWarehouseItem'; itemId: string; quantity: number }
  | { type: 'withdrawWarehouseItem'; itemId: string; quantity: number }
  | { type: 'interactNpc'; npcId: EntityId; actionId?: 'accept_task' | 'turn_in_task' }
  | { type: 'closeDialog' };
