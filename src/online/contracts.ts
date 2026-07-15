export type AccountState = 'active' | 'pending_verification' | 'locked';

export type BaseClass = 'Fighter' | 'Mage';

export type CharacterRace = 'Human' | 'Elf' | 'Dark Elf' | 'Orc' | 'Dwarf';

export type CharacterSex = 'Male' | 'Female';

export interface CharacterAppearanceOptions {
  hair_styles: number[];
  hair_color_default: string;
  skin_types: number[];
}

export interface ApiErrorResponse {
  reason_code: string;
  message: string;
}

export interface RegisterRequest {
  login: string;
  password: string;
  display_name: string;
}

export interface RegisterResponse {
  account_id: string;
  registration_state: 'created_pending_verification' | 'created_active';
  next_step: 'login_or_verify' | 'login';
}

export interface LoginRequest {
  login: string;
  password: string;
}

export interface LoginResponse {
  account_id: string;
  access_token: string;
  expires_at_ms: number;
  account_state: AccountState;
}

export interface CharacterSummary {
  character_id: string;
  name: string;
  race: CharacterRace;
  base_class: BaseClass;
  sex: CharacterSex;
  hair_style: number;
  hair_color: string;
  skin_type: number;
  level: number;
  last_region_id: string;
  is_enterable: boolean;
}

export interface CharactersResponse {
  characters: CharacterSummary[];
}

export interface CharacterCatalogRace {
  race: CharacterRace;
  enabled: boolean;
  base_classes: BaseClass[];
  sex_options: CharacterSex[];
  appearance_options: CharacterAppearanceOptions;
}

export interface CharacterCatalogResponse {
  races: CharacterCatalogRace[];
}

export interface CreateCharacterRequest {
  race: CharacterRace;
  base_class: BaseClass;
  sex: CharacterSex;
  hair_style: number;
  hair_color: string;
  skin_type: number;
  name: string;
}

export interface CreateCharacterResponse {
  character: CharacterSummary;
  characters: CharacterSummary[];
}

export interface WorldEnterRequest {
  character_id: string;
}

export interface WorldEnterResponse {
  session_id: string;
  character_id: string;
  attach_token: string;
  attach_expires_at_ms: number;
  self_state?: SelfStateSnapshot;
  item_state: CharacterItemSnapshot;
  ws_url: string;
}

export interface AttachSessionMessage {
  kind: 'attach_session';
  session_id: string;
  attach_token: string;
}

export interface MoveIntentPayload {
  point: {
    x: number;
    z: number;
  };
}

export interface SelectTargetPayload {
  target_id: string;
}

export interface UseSkillPayload {
  skill_id: string;
  target_id?: string;
}

export interface PickUpLootPayload {
  loot_id: string;
}

export interface InteractNpcPayload {
  npc_id: string;
  action_id?: 'accept_task' | 'turn_in_task';
}

export interface EquipItemPayload {
  item_instance_id: string;
}

export interface UseItemPayload {
  item_instance_id: string;
}

export interface TameMobPayload {
  target_id: string;
}

export interface UnequipItemPayload {
  equip_slot: 'weapon' | 'chest' | 'gloves' | 'boots';
}

export interface SplitItemStackPayload {
  item_instance_id: string;
  quantity: number;
}

export interface MergeItemStacksPayload {
  source_item_instance_id: string;
  target_item_instance_id: string;
}

export interface BuyItemPayload {
  vendor_offer_id: string;
  quantity: number;
}

export interface ExchangeItemPayload {
  exchange_offer_id: string;
  quantity: number;
}

export interface DepositItemPayload {
  item_instance_id: string;
  quantity: number;
}

export interface WithdrawItemPayload {
  item_instance_id: string;
  quantity: number;
}

export interface SellItemPayload {
  item_instance_id: string;
  quantity: number;
}

export interface OfferTradeItemPayload {
  target_character_id: string;
  item_instance_id: string;
  quantity: number;
}

export interface TradeOfferDecisionPayload {
  trade_offer_id: string;
}

export interface PartyInvitePayload {
  target_character_id?: string;
}

export interface CreateClanPayload {
  name: string;
}

export interface ClanInvitePayload {
  target_character_id?: string;
}

export interface PartyInviteDecisionPayload {
  invite_id: string;
}

export type ChatChannel = 'region' | 'party' | 'whisper';

export interface SendChatMessagePayload {
  channel: ChatChannel;
  text: string;
  target_character_name?: string;
}

export interface SetHotbarStatePayload {
  open_bar_count: number;
  slots: HotbarSlotSnapshot[];
}

export interface EmptyCommandPayload {
  [key: string]: never;
}

export interface MoveIntentCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'move_intent';
  payload: MoveIntentPayload;
}

export interface SelectTargetCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'select_target';
  payload: SelectTargetPayload;
}

export interface ClearTargetCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'clear_target';
  payload: EmptyCommandPayload;
}

export interface UseSkillCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'use_skill';
  payload: UseSkillPayload;
}

export interface BasicAttackCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'basic_attack';
  payload: SelectTargetPayload;
}

export interface PickUpLootCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'pick_up_loot';
  payload: PickUpLootPayload;
}

export interface InteractNpcCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'interact_npc';
  payload: InteractNpcPayload;
}

export interface EquipItemCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'equip_item';
  payload: EquipItemPayload;
}

export interface UseItemCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'use_item';
  payload: UseItemPayload;
}

export interface TameMobCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'tame_mob';
  payload: TameMobPayload;
}

export interface SummonPetCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'summon_pet';
  payload: EmptyCommandPayload;
}

export interface DismissPetCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'dismiss_pet';
  payload: EmptyCommandPayload;
}

export interface MountPetCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'mount_pet';
  payload: EmptyCommandPayload;
}

export interface DismountPetCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'dismount_pet';
  payload: EmptyCommandPayload;
}

export interface UnequipItemCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'unequip_item';
  payload: UnequipItemPayload;
}

export interface SplitItemStackCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'split_item_stack';
  payload: SplitItemStackPayload;
}

export interface MergeItemStacksCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'merge_item_stacks';
  payload: MergeItemStacksPayload;
}

export interface BuyItemCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'buy_item';
  payload: BuyItemPayload;
}

export interface ExchangeItemCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'exchange_item';
  payload: ExchangeItemPayload;
}

export interface DepositItemCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'deposit_item';
  payload: DepositItemPayload;
}

export interface WithdrawItemCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'withdraw_item';
  payload: WithdrawItemPayload;
}

export interface SellItemCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'sell_item';
  payload: SellItemPayload;
}

export interface OfferTradeItemCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'offer_trade_item';
  payload: OfferTradeItemPayload;
}

export interface AcceptTradeOfferCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'accept_trade_offer';
  payload: TradeOfferDecisionPayload;
}

export interface DeclineTradeOfferCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'decline_trade_offer';
  payload: TradeOfferDecisionPayload;
}

export interface InvitePartyMemberCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'invite_party_member';
  payload: PartyInvitePayload;
}

export interface AcceptPartyInviteCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'accept_party_invite';
  payload: PartyInviteDecisionPayload;
}

export interface DeclinePartyInviteCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'decline_party_invite';
  payload: PartyInviteDecisionPayload;
}

export interface LeavePartyCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'leave_party';
  payload: EmptyCommandPayload;
}

export interface KickPartyMemberCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'kick_party_member';
  payload: PartyInvitePayload;
}

export interface CreateClanCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'create_clan';
  payload: CreateClanPayload;
}

export interface InviteClanMemberCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'invite_clan_member';
  payload: ClanInvitePayload;
}

export interface AcceptClanInviteCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'accept_clan_invite';
  payload: PartyInviteDecisionPayload;
}

export interface DeclineClanInviteCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'decline_clan_invite';
  payload: PartyInviteDecisionPayload;
}

export interface LeaveClanCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'leave_clan';
  payload: EmptyCommandPayload;
}

export interface KickClanMemberCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'kick_clan_member';
  payload: ClanInvitePayload;
}

export interface DissolveClanCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'dissolve_clan';
  payload: EmptyCommandPayload;
}

export interface SendChatMessageCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'send_chat_message';
  payload: SendChatMessagePayload;
}

export interface SetHotbarStateCommand {
  protocol_version: 1;
  command_id: string;
  command_seq: number;
  client_sent_at_ms: number;
  type: 'set_hotbar_state';
  payload: SetHotbarStatePayload;
}

export type GameplayCommandEnvelope =
  | MoveIntentCommand
  | SelectTargetCommand
  | ClearTargetCommand
  | UseSkillCommand
  | BasicAttackCommand
  | PickUpLootCommand
  | InteractNpcCommand
  | EquipItemCommand
  | UseItemCommand
  | TameMobCommand
  | SummonPetCommand
  | DismissPetCommand
  | MountPetCommand
  | DismountPetCommand
  | UnequipItemCommand
  | SplitItemStackCommand
  | MergeItemStacksCommand
  | BuyItemCommand
  | ExchangeItemCommand
  | DepositItemCommand
  | WithdrawItemCommand
  | SellItemCommand
  | OfferTradeItemCommand
  | AcceptTradeOfferCommand
  | DeclineTradeOfferCommand
  | InvitePartyMemberCommand
  | AcceptPartyInviteCommand
  | DeclinePartyInviteCommand
  | LeavePartyCommand
  | KickPartyMemberCommand
  | CreateClanCommand
  | InviteClanMemberCommand
  | AcceptClanInviteCommand
  | DeclineClanInviteCommand
  | LeaveClanCommand
  | KickClanMemberCommand
  | DissolveClanCommand
  | SendChatMessageCommand
  | SetHotbarStateCommand;

export interface ServerMessageBase {
  kind: string;
  emitted_at_ms: number;
}

export interface AckMessage extends ServerMessageBase {
  kind: 'ack';
  command_id: string;
  command_seq: number;
  status: 'received';
}

export interface RejectMessage extends ServerMessageBase {
  kind: 'reject';
  reason_code: string;
  message: string;
  command_id?: string;
  command_seq?: number;
}

export interface DeltaMessage extends ServerMessageBase {
  kind: 'delta';
  revision: number;
  applies_to_command_id: string;
  applies_to_command_seq: number;
  self?: Record<string, unknown>;
  entities?: Array<Record<string, unknown>>;
  inventory?: CharacterItemRecord[];
  equipment?: CharacterItemRecord[];
  warehouse?: CharacterItemRecord[];
  region?: Record<string, unknown>;
}

export interface CharacterItemRecord {
  item_instance_id: string;
  template_id: string;
  quantity: number;
  container_kind: 'inventory' | 'equipment' | 'warehouse';
  equip_slot?: 'weapon' | 'chest' | 'gloves' | 'boots';
  instance_attributes?: Partial<AuthoritativePlayerStatsRecord>;
}

export interface CharacterItemSnapshot {
  inventory: CharacterItemRecord[];
  equipment: CharacterItemRecord[];
  warehouse?: CharacterItemRecord[];
}

export interface AuthoritativePlayerStatsRecord {
  max_cp?: number;
  max_hp: number;
  max_mp: number;
  attack: number;
  defense: number;
  move_speed: number;
}

export interface KnownSkillSnapshot {
  skill_id: string;
  category: 'active' | 'passive';
  unlock_level: number;
}

export interface HotbarSlotSnapshot {
  slot_index: number;
  entry_type?: 'skill' | 'item' | 'action';
  skill_id?: string;
  item_instance_id?: string;
  action_id?:
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
}

export interface HotbarSnapshot {
  open_bar_count: number;
  slots: HotbarSlotSnapshot[];
}

export interface QuestSnapshot {
  id: string;
  title: string;
  description: string;
  status: 'available' | 'active' | 'ready_to_turn_in' | 'completed';
  progress: number;
  goal: number;
}

export interface NpcInteractionSnapshot {
  npc_id: string;
  kind:
    | 'merchant_services'
    | 'warehouse_services'
    | 'wardkeeper_available'
    | 'wardkeeper_active'
    | 'wardkeeper_ready_to_turn_in'
    | 'wardkeeper_completed';
}

export interface PetSnapshot {
  pet_instance_id: string;
  pet_template_id: string;
  name: string;
  kind: 'pet' | 'mount' | 'pet_mount';
  visual_template_id: string;
  mount_eligible: boolean;
  summoned: boolean;
  mounted: boolean;
}

export interface PartyMemberSnapshot {
  character_id: string;
  name: string;
  level: number;
  base_class: BaseClass;
  hp: number;
  mp: number;
  online: boolean;
  is_leader: boolean;
}

export interface PartySnapshot {
  party_id: string;
  leader_character_id: string;
  members: PartyMemberSnapshot[];
}

export interface PartyInviteSnapshot {
  invite_id: string;
  party_id: string;
  inviter_character_id: string;
  inviter_name: string;
  expires_at_ms: number;
}

export interface ClanMemberSnapshot {
  character_id: string;
  name: string;
  level: number;
  base_class: BaseClass;
  online: boolean;
  is_leader: boolean;
}

export interface ClanSnapshot {
  clan_id: string;
  name: string;
  leader_character_id: string;
  members: ClanMemberSnapshot[];
}

export interface ClanInviteSnapshot {
  invite_id: string;
  clan_id: string;
  clan_name: string;
  inviter_character_id: string;
  inviter_name: string;
  expires_at_ms: number;
}

export interface SelfStateSnapshot {
  level?: number;
  xp?: number;
  cp?: number;
  hp?: number;
  mp?: number;
  dead?: boolean;
  pvp_flagged?: boolean;
  pvp_flag_until_ms?: number | null;
  pvp_kills?: number;
  pk_count?: number;
  karma?: number;
  cooldowns?: Record<string, number>;
  stats: AuthoritativePlayerStatsRecord;
  known_skills?: KnownSkillSnapshot[];
  hotbar?: HotbarSnapshot;
  pets?: PetSnapshot[];
  quest?: QuestSnapshot;
  party?: PartySnapshot | null;
  party_invites?: PartyInviteSnapshot[];
  clan?: ClanSnapshot | null;
  clan_invites?: ClanInviteSnapshot[];
  npc_interaction?: NpcInteractionSnapshot | null;
}

export type KnownEntityType = 'npc' | 'mob' | 'loot' | 'player' | 'pet';

export interface RegionEntity {
  entity_id: string;
  entity_type: KnownEntityType;
  template_id: string;
  position: {
    x: number;
    z: number;
  };
  state: Record<string, unknown>;
}

export interface RegionContextMessage extends ServerMessageBase {
  kind: 'region_context';
  region_revision: number;
  region_id: string;
  geodata_version: string;
  self_position: {
    x: number;
    z: number;
  };
  known_entities: RegionEntity[];
}

export interface EntityAppearMessage extends ServerMessageBase {
  kind: 'entity_appear';
  region_revision: number;
  entity: RegionEntity;
}

export interface EntityDisappearMessage extends ServerMessageBase {
  kind: 'entity_disappear';
  region_revision: number;
  entity_id: string;
  reason: string;
}

export interface PositionCorrectionMessage extends ServerMessageBase {
  kind: 'position_correction';
  applies_to_command_seq: number;
  position: {
    x: number;
    z: number;
  };
  facing: number;
  reason: string;
}

export interface TradeNoticeMessage extends ServerMessageBase {
  kind: 'trade_notice';
  command_id?: string;
  command_seq?: number;
  status: 'pending' | 'accepted' | 'declined' | 'cancelled';
  direction: 'incoming' | 'outgoing';
  offer_id: string;
  counterparty_character_id: string;
  counterparty_name: string;
  item_template_id: string;
  quantity: number;
  message: string;
}

export interface PartyNoticeMessage extends ServerMessageBase {
  kind: 'party_notice';
  command_id?: string;
  command_seq?: number;
  status:
    | 'invite_sent'
    | 'invite_received'
    | 'invite_accepted'
    | 'invite_declined'
    | 'invite_expired'
    | 'member_joined'
    | 'member_left'
    | 'member_kicked'
    | 'leader_transferred'
    | 'party_dissolved';
  party_id?: string;
  invite_id?: string;
  actor_character_id?: string;
  actor_name?: string;
  target_character_id?: string;
  target_name?: string;
  message: string;
}

export interface ClanNoticeMessage extends ServerMessageBase {
  kind: 'clan_notice';
  command_id?: string;
  command_seq?: number;
  status:
    | 'created'
    | 'invite_sent'
    | 'invite_received'
    | 'invite_accepted'
    | 'invite_declined'
    | 'invite_expired'
    | 'member_joined'
    | 'member_left'
    | 'member_kicked'
    | 'clan_dissolved';
  clan_id?: string;
  invite_id?: string;
  actor_character_id?: string;
  actor_name?: string;
  target_character_id?: string;
  target_name?: string;
  message: string;
}

export interface ChatMessageServerMessage extends ServerMessageBase {
  kind: 'chat_message';
  command_id?: string;
  command_seq?: number;
  channel: ChatChannel;
  sender_character_id: string;
  sender_name: string;
  target_character_id?: string;
  target_character_name?: string;
  region_id?: string;
  text: string;
}

export type GameplayServerMessage =
  | AckMessage
  | RejectMessage
  | DeltaMessage
  | RegionContextMessage
  | EntityAppearMessage
  | EntityDisappearMessage
  | PositionCorrectionMessage
  | TradeNoticeMessage
  | PartyNoticeMessage
  | ClanNoticeMessage
  | ChatMessageServerMessage;

export const isApiErrorResponse = (value: unknown): value is ApiErrorResponse => {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const candidate = value as Partial<ApiErrorResponse>;
  return typeof candidate.reason_code === 'string' && typeof candidate.message === 'string';
};
