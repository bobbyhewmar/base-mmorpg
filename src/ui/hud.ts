import {
  findKnownSkill,
  gameTemplates,
  HOTBAR_ROW_SIZE,
  getVisibleHotbarRows,
} from '../game/data/templates';
import {
  getDerivedStats,
  getItemAttributeSummary,
  getEquippedBySlot,
  getInventoryItems,
  getItemLabel,
  getNearbyTradePlayers,
  getNearbyWarehouse,
  getNearbyVendor,
  getRegionIdForPoint,
  getTargetMob,
  getTemplate,
  getVendorSellValue,
  type GameStore,
} from '../game/domain/game';
import type {
  EquipSlot,
  GameState,
  HotbarActionId,
  ItemInstance,
  ItemTemplate,
  PlayerHotbarSlot,
  PlayerHotbarState,
  PlayerKnownSkill,
  SkillTemplate,
  DerivedStats,
} from '../game/domain/types';

type HudControls = {
  save: () => void;
  load: () => void;
  reset: () => void;
};

type HudOptions = {
  interactive?: boolean;
  showPersistenceControls?: boolean;
  modeLabel?: string;
  onUseSkill?: (skillId: string) => void;
  onUseHotbarAction?: (actionId: HotbarActionId) => void;
  onUseItem?: (itemId: string) => void;
  onEquipItem?: (itemId: string) => void;
  onUnequipItem?: (slot: EquipSlot) => void;
  onSplitItemStack?: (itemId: string, quantity: number) => void;
  onMergeItemStacks?: (sourceItemId: string, targetItemId: string) => void;
  onBuyVendorOffer?: (offerId: string, quantity: number) => void;
  onExchangeVendorOffer?: (offerId: string, quantity: number) => void;
  onOfferTradeItem?: (targetCharacterId: string, itemId: string, quantity: number) => void;
  onAcceptTradeOffer?: (offerId: string) => void;
  onDeclineTradeOffer?: (offerId: string) => void;
  onInvitePartyMember?: (targetCharacterId?: string) => void;
  onAcceptPartyInvite?: (inviteId: string) => void;
  onDeclinePartyInvite?: (inviteId: string) => void;
  onLeaveParty?: () => void;
  onKickPartyMember?: (targetCharacterId: string) => void;
  onCreateClan?: (name: string) => void;
  onInviteClanMember?: () => void;
  onAcceptClanInvite?: (inviteId: string) => void;
  onDeclineClanInvite?: (inviteId: string) => void;
  onLeaveClan?: () => void;
  onKickClanMember?: (targetCharacterId: string) => void;
  onDissolveClan?: () => void;
  onCreateAlliance?: (name: string) => void;
  onInviteAllianceClan?: () => void;
  onAcceptAllianceInvite?: (inviteId: string) => void;
  onDeclineAllianceInvite?: (inviteId: string) => void;
  onLeaveAlliance?: () => void;
  onExpelAllianceClan?: (targetClanId: string) => void;
  onDissolveAlliance?: () => void;
  onSellVendorItem?: (itemId: string, quantity: number) => void;
  onDepositWarehouseItem?: (itemId: string, quantity: number) => void;
  onWithdrawWarehouseItem?: (itemId: string, quantity: number) => void;
  onHotbarChange?: (hotbar: PlayerHotbarState) => void;
  onInteractNpc?: (npcId: string, actionId?: 'accept_task' | 'turn_in_task') => void;
  onCloseDialog?: () => void;
  onSendChatMessage?: (
    channel: 'region' | 'party' | 'alliance' | 'whisper',
    text: string,
    targetCharacterName?: string,
  ) => boolean;
};

export type ChatLogFilter = 'all' | 'region' | 'party' | 'alliance' | 'whisper';
type ChatComposeChannel = 'region' | 'party' | 'alliance' | 'whisper';

type HudPanelId =
  | 'player'
  | 'party'
  | 'target'
  | 'hotbar'
  | 'skill-book'
  | 'log'
  | 'inventory'
  | 'dialog'
  | 'quick-menu'
  | 'world-map'
  | 'system-menu'
  | 'system-placeholder';
type HotbarOpenBarCount = 1 | 2 | 3;
type SkillBookTab = 'active' | 'passive';
export type CharacterPanelId = 'status' | 'skills' | 'actions' | 'clan' | 'quests';
export type SystemPlaceholderId = 'community' | 'macro' | 'help' | 'petition' | 'options' | 'restart';

type HudDragState = {
  panelId: HudPanelId;
  offsetX: number;
  offsetY: number;
  width: number;
  height: number;
};

const MINIMAP_WORLD_SIZE = 1024;
const MINIMAP_HALF_WORLD_SIZE = MINIMAP_WORLD_SIZE / 2;
const THREE_RAD_TO_DEG = 180 / Math.PI;

const clampNumber = (value: number, min: number, max: number): number => Math.max(min, Math.min(max, value));

const minimapPercentForAxis = (value: number): number =>
  clampNumber(((value + MINIMAP_HALF_WORLD_SIZE) / MINIMAP_WORLD_SIZE) * 100, 0, 100);

export const getPlayerFeedbackState = (
  state: GameState,
): {
  label: string;
  message: string;
  tone: 'success' | 'warning' | 'danger';
  overlay: boolean;
} | null => {
  if (state.player.deadUntilMs) {
    return {
      label: 'Dead',
      message: 'Awaiting authoritative return.',
      tone: 'danger',
      overlay: true,
    };
  }

  const latestLog = state.logs[0];
  if (!latestLog || latestLog.tone === 'neutral') {
    return null;
  }

  return {
    label:
      latestLog.tone === 'success'
        ? 'Recovered'
        : latestLog.tone === 'warning'
          ? 'Blocked'
          : 'Danger',
    message: latestLog.text,
    tone: latestLog.tone,
    overlay: false,
  };
};

export const getSkillButtonState = (
  state: GameState,
  skillId: string,
  requiresTarget: boolean,
): {
  visualRemainingMs: number;
  authorityState: 'ready' | 'cooling' | 'cooldown_elapsed_waiting_authority';
  disabled: boolean;
} => {
  const skillTemplate = gameTemplates.skills[skillId];
  if (!skillTemplate) {
    return {
      visualRemainingMs: 0,
      authorityState: 'ready',
      disabled: true,
    };
  }
  const skillProjection = state.player.skillAvailability[skillId];
  const visualRemainingMs = skillProjection?.visualRemainingMs ?? (state.player.cooldowns[skillId] ?? 0);
  const authorityState =
    skillProjection?.authorityState ?? ((state.player.cooldowns[skillId] ?? 0) > 0 ? 'cooling' : 'ready');
  return {
    visualRemainingMs,
    authorityState,
    disabled:
      (skillProjection?.requestBlocked ?? ((state.player.cooldowns[skillId] ?? 0) > 0)) ||
      state.player.mp < skillTemplate.mpCost ||
      (requiresTarget && !Boolean(getTargetMob(state))) ||
      Boolean(state.player.deadUntilMs),
  };
};

const xpGoalForLevel = (level: number): number => 70 + (Math.max(level, 1) - 1) * 50;

const HOTBAR_KEY_LABELS = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '0', '-', '='];
const SKILL_BOOK_GRID_SLOT_COUNT = 48;
const INVENTORY_GRID_SLOT_COUNT = 30;
const INVENTORY_CAPACITY = 80;
const FUTURE_WEIGHT_PERCENT = '1.90%';
const CHARACTER_PANEL_NAV: Array<{
  id: CharacterPanelId;
  label: string;
  shortcut: string;
  icon: string;
  tint: string;
}> = [
  { id: 'status', label: 'Status', shortcut: 'Alt+T', icon: 'ST', tint: '#a98f5f' },
  { id: 'skills', label: 'Skills', shortcut: 'Alt+K', icon: 'SK', tint: '#b69a42' },
  { id: 'actions', label: 'Actions', shortcut: 'Alt+C', icon: 'AC', tint: '#9c7768' },
  { id: 'clan', label: 'Clan', shortcut: 'Alt+N', icon: 'CL', tint: '#a94f4f' },
  { id: 'quests', label: 'Quests', shortcut: 'Alt+U', icon: 'QT', tint: '#8ea2a1' },
];

type HotbarShortcutPayload =
  | { entryType: 'skill'; skillId: string }
  | { entryType: 'item'; itemId: string }
  | { entryType: 'action'; actionId: HotbarActionId };

type HotbarShortcutOverride = HotbarShortcutPayload | null;

type HotbarActionTemplate = {
  id: HotbarActionId;
  name: string;
  description: string;
  iconKey: string;
  iconTint: string;
};

const HOTBAR_ACTIONS: Record<HotbarActionId, HotbarActionTemplate> = {
  basic_attack: {
    id: 'basic_attack',
    name: 'Attack',
    description: 'Move into melee range and perform a basic attack against the current target.',
    iconKey: 'AT',
    iconTint: '#7f9fb8',
  },
  pick_up_nearby: {
    id: 'pick_up_nearby',
    name: 'Pick Up',
    description: 'Pick up the nearest loot already close to the character.',
    iconKey: 'PU',
    iconTint: '#d3b55b',
  },
  toggle_walk_run: {
    id: 'toggle_walk_run',
    name: 'Walk/Run',
    description: 'Toggle the local movement animation between run and walk.',
    iconKey: 'WR',
    iconTint: '#8fb5e8',
  },
  party_invite: {
    id: 'party_invite',
    name: 'Party Invite',
    description: 'Send an authoritative party invite to the current player target.',
    iconKey: 'PI',
    iconTint: '#91b58a',
  },
  party_leave: {
    id: 'party_leave',
    name: 'Party Leave',
    description: 'Leave the current party through the authoritative backend command flow.',
    iconKey: 'PL',
    iconTint: '#b88a7d',
  },
  tame_target: {
    id: 'tame_target',
    name: 'Tame',
    description: 'Attempt to tame the current target if it is a valid nearby companion source.',
    iconKey: 'TM',
    iconTint: '#87c09a',
  },
  summon_pet: {
    id: 'summon_pet',
    name: 'Summon',
    description: 'Summon the owned companion through an authoritative command.',
    iconKey: 'SM',
    iconTint: '#86bfe0',
  },
  dismiss_pet: {
    id: 'dismiss_pet',
    name: 'Dismiss',
    description: 'Dismiss the active companion without losing ownership.',
    iconKey: 'DS',
    iconTint: '#a7a0b9',
  },
  mount_pet: {
    id: 'mount_pet',
    name: 'Mount',
    description: 'Mount the active eligible companion through backend authority.',
    iconKey: 'MT',
    iconTint: '#d7a468',
  },
  dismount_pet: {
    id: 'dismount_pet',
    name: 'Dismount',
    description: 'Dismount the current companion and restore normal movement speed.',
    iconKey: 'DM',
    iconTint: '#d08372',
  },
};

const hotbarPayloadToSlot = (slotIndex: number, payload: HotbarShortcutPayload): PlayerHotbarSlot => {
  if (payload.entryType === 'skill') {
    return {
      slotIndex,
      entryType: 'skill',
      skillId: payload.skillId,
      itemId: null,
      actionId: null,
    };
  }
  if (payload.entryType === 'item') {
    return {
      slotIndex,
      entryType: 'item',
      skillId: null,
      itemId: payload.itemId,
      actionId: null,
    };
  }
  return {
    slotIndex,
    entryType: 'action',
    skillId: null,
    itemId: null,
    actionId: payload.actionId,
  };
};

const emptyHotbarSlot = (slotIndex: number): PlayerHotbarSlot => ({
  slotIndex,
  entryType: null,
  skillId: null,
  itemId: null,
  actionId: null,
});

const renderHotbarShortcutDragAttributes = (slot: PlayerHotbarSlot): string => {
  if (!slot.entryType) {
    return '';
  }
  const shortcutId =
    slot.entryType === 'skill' ? slot.skillId : slot.entryType === 'item' ? slot.itemId : slot.actionId;
  if (!shortcutId) {
    return '';
  }
  return `draggable="true" data-hotbar-shortcut-slot="${slot.slotIndex}" data-hotbar-entry-type="${slot.entryType}" data-hotbar-shortcut-id="${shortcutId}"`;
};

const EQUIPMENT_SLOT_LAYOUT: Array<{ slot: EquipSlot; label: string }> = [
  { slot: 'weapon', label: 'Weapon' },
  { slot: 'chest', label: 'Chest' },
  { slot: 'gloves', label: 'Gloves' },
  { slot: 'boots', label: 'Boots' },
];

const normalizeHotbarOpenBarCount = (value: number): HotbarOpenBarCount => {
  if (value >= 3) {
    return 3;
  }
  if (value >= 2) {
    return 2;
  }
  return 1;
};

const clampPercent = (current: number, max: number): number => {
  if (max <= 0) {
    return 0;
  }
  return Math.max(0, Math.min(100, (current / max) * 100));
};

const skillRequiresTarget = (skill: SkillTemplate): boolean =>
  skill.targetType === 'single_target_enemy' || skill.targetType === 'target_centered_aoe';

const buildSkillTooltip = (skill: SkillTemplate, knownSkill: PlayerKnownSkill | null): string => {
  const details =
    skill.category === 'active'
      ? `Classe ${skill.baseClass} â€¢ Lv ${skill.unlockLevel}\nMP ${skill.mpCost} â€¢ Cooldown ${(skill.cooldownMs / 1000).toFixed(1)}s`
      : `Classe ${skill.baseClass} â€¢ Lv ${skill.unlockLevel}\nPassiva`;
  const learned = knownSkill ? 'Aprendida' : `Desbloqueia no Lv ${skill.unlockLevel}`;
  return `${skill.name}\n${details}\n${learned}\n${skill.description}`;
};

type HotbarSlotView = {
  keyLabel: string;
  entryType: PlayerHotbarSlot['entryType'];
  skillId: string | null;
  itemId: string | null;
  actionId: HotbarActionId | null;
  template: SkillTemplate | null;
  knownSkill: PlayerKnownSkill | null;
  item: ItemInstance | null;
  itemTemplate: ItemTemplate | null;
  action: HotbarActionTemplate | null;
  disabled: boolean;
  cooldownLabel: string;
  cooldownMaskPercent: number;
  syncPending: boolean;
};

type TargetHudView = {
  name: string;
  subtitle: string;
  hpPercent: number | null;
  hpLabel: string | null;
  tone: 'mob' | 'player' | 'npc';
};

export type CombatStatusHudView = {
  label: 'Neutral' | 'PvP' | 'PK';
  detail: string;
  variant: 'neutral' | 'pvp' | 'pk';
};

export const getPlayerCombatHudView = (state: GameState, nowMs: number): CombatStatusHudView => {
  if (state.player.karma > 0) {
    return {
      label: 'PK',
      detail: `Karma ${state.player.karma}`,
      variant: 'pk',
    };
  }
  if (state.player.pvpFlagged) {
    if (typeof state.player.pvpFlagUntilMs === 'number') {
      const remainingSeconds = Math.max(0, Math.ceil((state.player.pvpFlagUntilMs - nowMs) / 1000));
      return {
        label: 'PvP',
        detail: `Flag ${remainingSeconds}s`,
        variant: 'pvp',
      };
    }
    return {
      label: 'PvP',
      detail: 'Flag active',
      variant: 'pvp',
    };
  }
  return {
    label: 'Neutral',
    detail: 'No current flag',
    variant: 'neutral',
  };
};

const getTargetHudView = (state: GameState): TargetHudView | null => {
  if (!state.targetId) {
    return null;
  }

  const mob = state.mobs[state.targetId];
  if (mob) {
    const template = gameTemplates.mobTemplates[mob.templateId];
    if (!template) {
      return null;
    }
    return {
      name: template.name,
      subtitle: `Lv ${template.level} Mob - ATK ${template.attack} - DEF ${template.defense}`,
      hpPercent: clampPercent(mob.hp, template.maxHp),
      hpLabel: `HP ${Math.round(mob.hp)}/${template.maxHp}`,
      tone: 'mob',
    };
  }

  const otherPlayer = state.otherPlayers[state.targetId];
  if (otherPlayer) {
    const combatState =
      otherPlayer.karma > 0
        ? `PK · Karma ${otherPlayer.karma}`
        : otherPlayer.pvpFlagged
          ? typeof otherPlayer.pvpFlagUntilMs === 'number'
            ? `PvP · Flag ${Math.max(0, Math.ceil((otherPlayer.pvpFlagUntilMs - state.timeMs) / 1000))}s`
            : 'PvP flagged'
          : 'Neutral';
    return {
      name: otherPlayer.name,
      subtitle: `Player - Lv ${otherPlayer.level} · ${combatState}`,
      hpPercent: otherPlayer.dead ? 0 : Math.max(0, Math.min(100, otherPlayer.hp)),
      hpLabel: otherPlayer.dead ? 'Dead' : `HP ${Math.round(otherPlayer.hp)}`,
      tone: 'player',
    };
  }

  const npc = state.npcs[state.targetId];
  if (npc) {
    return {
      name: npc.name,
      subtitle: npc.title,
      hpPercent: null,
      hpLabel: null,
      tone: 'npc',
    };
  }

  return null;
};

const getHotbarSlotView = (state: GameState, slot: PlayerHotbarSlot): HotbarSlotView => {
  const skillId = slot.entryType === 'skill' ? slot.skillId : null;
  const template = skillId ? gameTemplates.skills[skillId] ?? null : null;
  const knownSkill = skillId ? findKnownSkill(state.player.learnedSkills, skillId) : null;
  const activeBoundSkillId = knownSkill?.category === 'active' ? skillId : null;
  const itemId = slot.entryType === 'item' ? (slot.itemId ?? null) : null;
  const item = itemId ? state.items[itemId] ?? null : null;
  const itemTemplate = item ? getTemplate(item.templateId) : null;
  const actionId = slot.entryType === 'action' ? (slot.actionId ?? null) : null;
  const action = actionId ? HOTBAR_ACTIONS[actionId] ?? null : null;
  const skillState =
    activeBoundSkillId && template ? getSkillButtonState(state, activeBoundSkillId, skillRequiresTarget(template)) : null;
  const cooldown = skillState ? Math.ceil(skillState.visualRemainingMs / 100) / 10 : 0;
  const cooldownMaskPercent =
    skillState && template && template.cooldownMs > 0
      ? Math.max(0, Math.min(100, (skillState.visualRemainingMs / template.cooldownMs) * 100))
      : 0;

  return {
    keyLabel: HOTBAR_KEY_LABELS[slot.slotIndex] ?? String(slot.slotIndex + 1),
    entryType: slot.entryType,
    skillId,
    itemId,
    actionId,
    template,
    knownSkill,
    item,
    itemTemplate,
    action,
    disabled:
      slot.entryType === 'skill'
        ? !activeBoundSkillId || !skillState || skillState.disabled
        : slot.entryType === 'item'
          ? !item || !itemTemplate || item.container !== 'inventory'
          : slot.entryType === 'action'
            ? !action
            : true,
    cooldownLabel:
      skillState?.authorityState === 'cooldown_elapsed_waiting_authority'
        ? 'SYNC'
        : cooldown > 0
          ? `${cooldown.toFixed(1)}s`
          : '',
    cooldownMaskPercent,
    syncPending: skillState?.authorityState === 'cooldown_elapsed_waiting_authority',
  };
};

const renderHotbarSlot = (state: GameState, slot: PlayerHotbarSlot): string => {
  const view = getHotbarSlotView(state, slot);
  const template = view.template;
  const dragAttributes = renderHotbarShortcutDragAttributes(slot);
  if (view.entryType === 'item') {
    const item = view.item;
    const itemTemplate = view.itemTemplate;
    if (!item || !itemTemplate) {
      return `
        <div class="skill-btn empty locked" data-hotbar-slot="${slot.slotIndex}" data-hotbar-drop="true" ${dragAttributes} aria-disabled="true" title="Item unavailable\nAlt+click to remove">
          <span class="skill-key">${view.keyLabel}</span>
          <span class="skill-slot-state">MISS</span>
        </div>
      `;
    }
    const actionHint = itemTemplate.equipSlot ? 'Click to equip' : 'Item shortcut';
    return `
      <button
        class="skill-btn hotbar-item-btn ${view.disabled ? 'disabled' : ''}"
        data-hotbar-slot="${slot.slotIndex}"
        data-hotbar-drop="true"
        data-hotbar-item="${item.id}"
        ${dragAttributes}
        ${view.disabled ? 'aria-disabled="true"' : ''}
        title="${getItemLabel(item)}\n${itemTemplate.kind}${itemTemplate.equipSlot ? ` - ${itemTemplate.equipSlot}` : ''}\n${actionHint}\nAlt+click to remove"
      >
        <span class="skill-key">${view.keyLabel}</span>
        <span class="hotbar-item-icon">${renderInventoryIconArt(itemTemplate, item)}</span>
        ${!itemTemplate.equipSlot ? '<span class="skill-slot-state">ITEM</span>' : ''}
      </button>
    `;
  }

  if (view.entryType === 'action') {
    const action = view.action;
    if (!action) {
      return `
        <div class="skill-btn empty locked" data-hotbar-slot="${slot.slotIndex}" data-hotbar-drop="true" ${dragAttributes} aria-disabled="true" title="Action unavailable\nAlt+click to remove">
          <span class="skill-key">${view.keyLabel}</span>
          <span class="skill-slot-state">MISS</span>
        </div>
      `;
    }
    return `
      <button
        class="skill-btn hotbar-action-btn"
        data-hotbar-slot="${slot.slotIndex}"
        data-hotbar-drop="true"
        data-hotbar-action="${action.id}"
        ${dragAttributes}
        title="${action.name}\n${action.description}\nClick to use\nAlt+click to remove"
      >
        <span class="skill-key">${view.keyLabel}</span>
        <span class="skill-icon" style="--skill-icon-tint:${action.iconTint}">${action.iconKey}</span>
      </button>
    `;
  }

  if (!template) {
    return `
      <div class="skill-btn empty" data-hotbar-slot="${slot.slotIndex}" data-hotbar-drop="true" aria-disabled="true">
        <span class="skill-key">${view.keyLabel}</span>
      </div>
    `;
  }

  const knownSkill = view.knownSkill;
  const isLocked = !knownSkill;
  const isPassive = knownSkill?.category === 'passive' || template.category === 'passive';
  if (isLocked || isPassive) {
    return `
      <div
        class="skill-btn empty ${isLocked ? 'locked' : 'passive'}"
        data-hotbar-slot="${slot.slotIndex}"
        data-hotbar-drop="true"
        ${dragAttributes}
        aria-disabled="true"
        title="${buildSkillTooltip(template, knownSkill)}\nAlt+click to remove"
      >
        <span class="skill-key">${view.keyLabel}</span>
        <span class="skill-slot-state">${isLocked ? `Lv ${template.unlockLevel}` : 'PASS'}</span>
      </div>
    `;
  }
  const buttonAttributes = view.skillId
    ? `data-skill="${view.skillId}" data-skill-disabled="${view.disabled ? 'true' : 'false'}" ${view.disabled ? 'aria-disabled="true"' : ''}`
    : 'aria-disabled="true" data-skill-disabled="true"';

  return `
    <button
      class="skill-btn ${view.disabled ? 'disabled' : ''}"
      data-hotbar-slot="${slot.slotIndex}"
      data-hotbar-drop="true"
      ${dragAttributes}
      ${buttonAttributes}
      title="${buildSkillTooltip(template, knownSkill)}\nAlt+click to remove"
    >
      <span class="skill-key">${view.keyLabel}</span>
      <span class="skill-icon" style="--skill-icon-tint:${template.iconTint}">${template.iconKey}</span>
      ${view.cooldownMaskPercent > 0 ? `<span class="skill-cooldown-mask" style="height:${view.cooldownMaskPercent}%"></span>` : ''}
      ${view.cooldownLabel ? `<span class="cooldown ${view.syncPending ? 'pending' : ''}">${view.cooldownLabel}</span>` : ''}
    </button>
  `;
};

const renderEmptySkillBookSlot = (): string => '<div class="lineage-skill-slot empty" aria-hidden="true"></div>';

const renderSkillBookIconSlot = (
  knownSkill: PlayerKnownSkill,
  options: { draggable: boolean; compact?: boolean },
): string => {
  const template = gameTemplates.skills[knownSkill.skillId];
  if (!template) {
    return renderEmptySkillBookSlot();
  }
  const canDrag = options.draggable && knownSkill.category === 'active';
  const dragAttributes = canDrag ? `draggable="true" data-skill-book-skill="${knownSkill.skillId}"` : '';
  const usageLabel = knownSkill.category === 'passive' ? 'Passive' : 'Drag to shortcut bar';
  return `
    <div
      class="lineage-skill-slot filled ${knownSkill.category} ${canDrag ? 'draggable' : ''} ${options.compact ? 'compact' : ''}"
      tabindex="0"
      ${dragAttributes}
      aria-label="${template.name}"
      title="${buildSkillTooltip(template, knownSkill)}"
    >
      <span class="skill-icon" style="--skill-icon-tint:${template.iconTint}">${template.iconKey}</span>
      <span class="lineage-skill-tooltip">
        <strong>${template.name}</strong>
        <small>Lv ${knownSkill.unlockLevel} - ${template.mpCost > 0 ? `MP ${template.mpCost}` : 'No MP cost'}</small>
        <em>${usageLabel}</em>
      </span>
    </div>
  `;
};

const escapeHudText = (value: string): string =>
  value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');

const composeChatChannelForFilter = (filter: ChatLogFilter): ChatComposeChannel =>
  filter === 'party' || filter === 'alliance' || filter === 'whisper' ? filter : 'region';

export const chatFilterMatches = (filter: ChatLogFilter, channel: GameState['logs'][number]['channel']): boolean => {
  if (filter === 'all') {
    return true;
  }
  if (filter === 'region') {
    return channel === 'region';
  }
  return channel === filter;
};

const renderSkillBookGrid = (skills: PlayerKnownSkill[], draggable: boolean): string => {
  const knownSlots = skills
    .slice(0, SKILL_BOOK_GRID_SLOT_COUNT)
    .map((knownSkill) => renderSkillBookIconSlot(knownSkill, { draggable }));
  const emptySlots = Array.from(
    { length: Math.max(0, SKILL_BOOK_GRID_SLOT_COUNT - knownSlots.length) },
    renderEmptySkillBookSlot,
  );
  return [...knownSlots, ...emptySlots].join('');
};

const renderCharacterPanelNav = (activePanel: CharacterPanelId): string =>
  CHARACTER_PANEL_NAV.map(
    (entry) => `
      <button
        type="button"
        class="lineage-panel-nav-button ${entry.id === activePanel ? 'active' : ''}"
        data-character-panel="${entry.id}"
        data-no-drag
        title="${entry.label} (${entry.shortcut})"
        aria-label="${entry.label} (${entry.shortcut})"
      >
        <span class="lineage-panel-nav-icon" style="--panel-icon-tint:${entry.tint}">${entry.icon}</span>
      </button>
    `,
  ).join('');

const renderStatusPanel = (state: GameState, stats: DerivedStats, cp: number, maxCp: number): string => `
  <section class="lineage-character-panel status-panel">
    <div class="lineage-status-identity">
      <strong>${state.player.name}</strong>
      <span>${state.player.baseClass}</span>
    </div>
    <div class="lineage-status-bars">
      <div><span>LV</span><strong>${state.player.level}</strong></div>
      <div><span>CP</span><strong>${Math.round(cp)}/${maxCp}</strong></div>
      <div><span>HP</span><strong>${Math.round(state.player.hp)}/${stats.maxHp}</strong></div>
      <div><span>MP</span><strong>${Math.round(state.player.mp)}/${stats.maxMp}</strong></div>
      <div><span>Exp</span><strong>${((state.player.xp / xpGoalForLevel(state.player.level)) * 100).toFixed(2)}%</strong></div>
      <div><span>PvP</span><strong>${state.player.pvpKills}</strong></div>
      <div><span>PK</span><strong>${state.player.pkCount}</strong></div>
      <div><span>Karma</span><strong>${state.player.karma}</strong></div>
    </div>
    <div class="lineage-status-section">
      <h4>Combat</h4>
      <div class="lineage-status-grid">
        <span>P. Atk.</span><strong>${stats.attack}</strong>
        <span>P. Def.</span><strong>${stats.defense}</strong>
        <span>Speed</span><strong>${stats.moveSpeed}</strong>
        <span>Accuracy</span><strong>--</strong>
        <span>Crit. Rate</span><strong>--</strong>
        <span>Casting Spd.</span><strong>--</strong>
      </div>
    </div>
    <div class="lineage-status-section">
      <h4>Basic</h4>
      <div class="lineage-status-basic">
        <span>STR <strong>--</strong></span>
        <span>DEX <strong>--</strong></span>
        <span>CON <strong>--</strong></span>
        <span>INT <strong>--</strong></span>
        <span>WIT <strong>--</strong></span>
        <span>MEN <strong>--</strong></span>
      </div>
    </div>
  </section>
`;

const renderActionSlot = (actionId: HotbarActionId): string => {
  const action = HOTBAR_ACTIONS[actionId];
  return `
    <div
      class="lineage-action-slot filled"
      draggable="true"
      data-action-hotbar-action="${action.id}"
      data-hotbar-action="${action.id}"
      title="${action.name}\n${action.description}\nDrag to the shortcut bar"
      aria-label="${action.name}"
    >
      <span class="skill-icon" style="--skill-icon-tint:${action.iconTint}">${action.iconKey}</span>
    </div>
  `;
};

const renderEmptyActionSlot = (title: string): string => `
  <div class="lineage-action-slot empty" title="${title}">
    <span></span>
  </div>
`;

const renderActionsPanel = (): string => `
  <section class="lineage-character-panel actions-panel">
    ${(['Basic', 'Party', 'Social'] as const)
      .map(
        (group) => `
          <div class="lineage-actions-section">
            <h4>${group}</h4>
            <div class="lineage-actions-grid">
              ${
                group === 'Basic'
                  ? [
                      renderActionSlot('basic_attack'),
                      renderActionSlot('pick_up_nearby'),
                      renderActionSlot('toggle_walk_run'),
                      renderActionSlot('tame_target'),
                      renderActionSlot('summon_pet'),
                      renderActionSlot('dismiss_pet'),
                      renderActionSlot('mount_pet'),
                      renderActionSlot('dismount_pet'),
                      ...Array.from({ length: 11 }, (_, index) => renderEmptyActionSlot(`Basic action ${index + 8}`)),
                    ].join('')
                  : group === 'Party'
                    ? [
                        renderActionSlot('party_invite'),
                        renderActionSlot('party_leave'),
                        ...Array.from({ length: 10 }, (_, index) => renderEmptyActionSlot(`Party action ${index + 3}`)),
                      ].join('')
                    : Array.from({ length: 12 }, (_, index) => renderEmptyActionSlot(`${group} action ${index + 1}`)).join('')
              }
            </div>
          </div>
        `,
      )
      .join('')}
  </section>
`;

type ClanPanelGridAction = {
  label: string;
  action?: string;
  disabled?: boolean;
  pressed?: boolean;
};

const CLAN_PANEL_GRID_ACTIONS = (isLeader: boolean, clanInfoOpen: boolean): ClanPanelGridAction[] => [
  { label: 'Title', disabled: true },
  { label: 'Privileges', disabled: true },
  { label: 'Community', disabled: true },
  { label: 'Clan Info', action: 'data-clan-info-toggle', pressed: clanInfoOpen },
  { label: 'Penalty', disabled: true },
  { label: 'Leave', action: 'data-clan-leave', disabled: isLeader },
  { label: 'War Info', disabled: true },
  { label: 'Declare War', disabled: true },
  { label: 'End War', disabled: true },
  { label: 'Invite', action: 'data-clan-invite', disabled: !isLeader },
  { label: 'Edit Privileges', disabled: true },
  { label: 'Edit Crest', disabled: true },
];

const renderClanGridButton = ({ label, action, disabled = false, pressed = false }: ClanPanelGridAction): string => `
  <button
    type="button"
    class="lineage-clan-grid-button ${pressed ? 'active' : ''}"
    ${action ? `${action}=""` : ''}
    ${disabled ? 'disabled aria-disabled="true"' : ''}
    data-no-drag
  >${label}</button>
`;

const renderAllianceMembers = (state: GameState): string => {
  const alliance = state.alliance;
  if (!alliance) {
    return '';
  }
  const clan = state.clan;
  const canManageAlliance = Boolean(clan && state.player.id === clan.leaderCharacterId && alliance.leaderClanId === clan.clanId);
  return `
    <div class="lineage-clan-alliance-list">
      ${alliance.members
        .map(
          (member) => `
            <div class="lineage-clan-alliance-row ${member.isLeaderClan ? 'leader' : ''}">
              <div class="lineage-clan-alliance-copy">
                <strong>${escapeHudText(member.name)}${member.isLeaderClan ? ' *' : ''}</strong>
                <span>Leader ${escapeHudText(member.leaderName)} · Members ${member.memberCount}</span>
              </div>
              ${
                canManageAlliance && !member.isLeaderClan
                  ? `<button type="button" class="lineage-clan-inline-action" data-alliance-expel="${member.clanId}" data-no-drag>Expel</button>`
                  : ''
              }
            </div>
          `,
        )
        .join('')}
    </div>
  `;
};

const renderAllianceSection = (
  state: GameState,
  allianceNameDraft: string,
): string => {
  const clan = state.clan;
  if (!clan) {
    return '';
  }
  const alliance = state.alliance;
  const isClanLeader = clan.leaderCharacterId === state.player.id;
  const isAllianceLeaderClan = Boolean(alliance && alliance.leaderClanId === clan.clanId);
  if (!alliance) {
    return `
      <section class="lineage-clan-alliance-surface">
        <div class="lineage-clan-info-surface-header">
          <strong>Alliance</strong>
          <span>No Alliance</span>
        </div>
        <div class="lineage-panel-empty-copy compact">
          <span>${isClanLeader ? 'Create an alliance from your current clan.' : 'Only the current clan leader can create an alliance.'}</span>
        </div>
        ${
          isClanLeader
            ? `
              <form class="lineage-clan-create-form" data-alliance-create-form style="display:flex;flex-direction:column;gap:8px;">
                <input
                  type="text"
                  name="alliance_name"
                  value="${escapeHudText(allianceNameDraft)}"
                  maxlength="16"
                  placeholder="Alliance name"
                  autocomplete="off"
                  data-alliance-name
                  data-no-drag
                />
                <button type="submit" data-alliance-create-submit data-no-drag>Create Alliance</button>
              </form>
            `
            : ''
        }
      </section>
    `;
  }
  return `
    <section class="lineage-clan-alliance-surface">
      <div class="lineage-clan-info-surface-header">
        <strong>Alliance</strong>
        <span>${escapeHudText(alliance.name)}</span>
      </div>
      <div class="lineage-clan-info-grid">
        <div><span>Leader Clan</span><strong>${escapeHudText(alliance.leaderClanName)}</strong></div>
        <div><span>Member Clans</span><strong>${alliance.members.length}/${alliance.clanCap}</strong></div>
      </div>
      ${renderAllianceMembers(state)}
      <div class="lineage-clan-info-actions">
        ${
          isAllianceLeaderClan
            ? `<button type="button" data-alliance-invite data-no-drag>Invite Clan</button>`
            : ''
        }
        ${
          isClanLeader && !isAllianceLeaderClan
            ? `<button type="button" data-alliance-leave data-no-drag>Leave Alliance</button>`
            : ''
        }
        ${
          isAllianceLeaderClan
            ? `<button type="button" data-alliance-dissolve data-no-drag ${alliance.members.length === 1 ? '' : 'disabled aria-disabled="true"'}>Dissolve Alliance</button>`
            : ''
        }
      </div>
    </section>
  `;
};

export const renderClanPanel = (
  state: GameState,
  clanNameDraft: string,
  allianceNameDraft: string,
  clanInfoOpen = false,
): string => {
  const clan = state.clan;
  if (!clan) {
    return `
      <section class="lineage-character-panel clan-panel">
        <div class="lineage-panel-empty-copy">
          <strong>No Clan</strong>
          <span>Found a minimal clan from ALT+N. Name must stay short and authoritative.</span>
        </div>
        <form class="lineage-clan-create-form" data-clan-create-form style="display:flex;flex-direction:column;gap:8px;">
          <input
            type="text"
            name="clan_name"
            value="${escapeHudText(clanNameDraft)}"
            maxlength="16"
            placeholder="Clan name"
            autocomplete="off"
            data-clan-name
            data-no-drag
          />
          <button type="submit" data-clan-create-submit data-no-drag>Create Clan</button>
        </form>
      </section>
    `;
  }

  const isLeader = clan.leaderCharacterId === state.player.id;
  const leaderName = clan.members.find((member) => member.isLeader)?.name ?? 'Unknown';
  const gridButtons = CLAN_PANEL_GRID_ACTIONS(isLeader, clanInfoOpen).map(renderClanGridButton).join('');
  return `
    <section class="lineage-character-panel clan-panel">
      <div class="lineage-clan-summary-block">
        <div class="lineage-clan-summary-heading">
          <strong>Clan</strong>
          <span>${escapeHudText(clan.name)}</span>
        </div>
        <div class="lineage-clan-summary-meta">
          <div class="lineage-clan-summary-field">
            <span>Leader</span>
            <strong>${escapeHudText(leaderName)}</strong>
          </div>
          <div class="lineage-clan-summary-field">
            <span>Base</span>
            <strong class="placeholder">None</strong>
          </div>
          <div class="lineage-clan-summary-field member-count">
            <span>Members</span>
            <strong>${clan.members.length}</strong>
          </div>
        </div>
      </div>
      <div class="lineage-clan-roster-shell">
        <div class="lineage-clan-roster-header" role="row">
          <span>Name</span>
          <span>Lv</span>
          <span>Cls</span>
          <span>Status</span>
        </div>
        <div class="lineage-clan-roster-body">
          ${clan.members
            .map(
              (member) => `
                <div class="lineage-clan-roster-row ${member.online ? 'online' : 'offline'}" role="row">
                  <div class="lineage-clan-name-cell">
                    <strong>${escapeHudText(member.name)}</strong>
                    ${member.isLeader ? '<span class="lineage-clan-member-tag">Leader</span>' : ''}
                  </div>
                  <span class="lineage-clan-stat-cell">${Number.isFinite(member.level) ? member.level : '--'}</span>
                  <span class="lineage-clan-stat-cell">${member.baseClass ? escapeHudText(member.baseClass) : '--'}</span>
                  <div class="lineage-clan-status-cell">
                    <span class="lineage-clan-status ${member.online ? 'online' : 'offline'}">${member.online ? 'Online' : 'Offline'}</span>
                    ${
                      isLeader && member.characterId !== state.player.id
                        ? `<button type="button" class="lineage-clan-inline-action" data-clan-kick="${member.characterId}" data-no-drag>Kick</button>`
                        : ''
                    }
                  </div>
                </div>
              `,
            )
            .join('')}
        </div>
      </div>
      ${
        clanInfoOpen
          ? `
            <section class="lineage-clan-info-surface" data-clan-info-surface>
              <div class="lineage-clan-info-surface-header">
                <strong>Clan Info</strong>
                <button type="button" class="lineage-clan-inline-action" data-clan-info-close data-no-drag>Close</button>
              </div>
              <div class="lineage-clan-info-grid">
                <div><span>Clan</span><strong>${escapeHudText(clan.name)}</strong></div>
                <div><span>Leader</span><strong>${escapeHudText(leaderName)}</strong></div>
                <div><span>Members</span><strong>${clan.members.length}</strong></div>
                <div><span>Base</span><strong class="placeholder">None</strong></div>
              </div>
              ${
                isLeader
                  ? `
                    <div class="lineage-clan-info-actions">
                      <button type="button" data-clan-dissolve data-no-drag>Dissolve Clan</button>
                    </div>
                  `
                  : ''
              }
            </section>
          `
          : ''
      }
      ${renderAllianceSection(state, allianceNameDraft)}
      <div class="lineage-clan-action-grid">
        ${gridButtons}
      </div>
    </section>
  `;
};

const renderQuestPanel = (state: GameState): string => `
  <section class="lineage-character-panel quest-panel">
    <div class="lineage-quest-header">
      <span>Accepted Quest</span>
      <strong>(${state.quest.status === 'completed' ? 0 : 1}/25)</strong>
    </div>
    <div class="lineage-quest-list">
      ${
        state.quest.status === 'completed'
          ? '<div class="lineage-panel-empty-copy"><span>No accepted quests.</span></div>'
          : `
            <button type="button" data-no-drag class="lineage-quest-entry">
              <strong>${state.quest.title}</strong>
              <span>${state.quest.progress}/${state.quest.goal}</span>
            </button>
          `
      }
    </div>
    <div class="lineage-quest-footer">
      <label><input type="checkbox" checked data-no-drag /> Location</label>
      <button type="button" data-no-drag>Abort</button>
    </div>
  </section>
`;

const PARTY_INVITE_VISUAL_TTL_MS = 10_000;

export const renderPartyInviteModal = (state: GameState, nowMs: number): string => {
  const invite = state.partyInvites[0];
  if (!invite) {
    return '';
  }
  const remainingMs = Math.max(0, invite.expiresAtMs - nowMs);
  const expired = remainingMs <= 0;
  const fillPercent = Math.max(0, Math.min(100, (remainingMs / PARTY_INVITE_VISUAL_TTL_MS) * 100));
  return `
    <section
      class="frame-panel classic-window lineage-party-invite-modal"
      data-party-invite-modal
      style="position:absolute;left:50%;bottom:104px;transform:translateX(-50%);width:280px;z-index:18;"
    >
      <div class="lineage-party-invite-timer" aria-hidden="true" style="height:4px;background:rgba(28,60,24,0.85);">
        <div style="height:100%;width:${fillPercent.toFixed(2)}%;background:linear-gradient(90deg,#63c46b,#2f8a39);"></div>
      </div>
      <div class="hud-window-title classic-title compact">
        <span>Party Invitation</span>
      </div>
      <div class="lineage-party-invite-body" style="padding:10px 12px 12px;display:flex;flex-direction:column;gap:8px;">
        <div class="lineage-party-invite-copy">
          <strong>${escapeHudText(invite.inviterName)}</strong>
          <span>${expired ? 'Invitation expired.' : 'Invites you to join a party.'}</span>
        </div>
        <div class="lineage-party-inline-actions">
          <button type="button" data-party-accept="${invite.inviteId}" data-no-drag ${expired ? 'disabled aria-disabled="true"' : ''}>Accept</button>
          <button type="button" data-party-decline="${invite.inviteId}" data-no-drag>Cancel</button>
        </div>
      </div>
    </section>
  `;
};

export const renderClanInviteModal = (state: GameState, nowMs: number): string => {
  const invite = state.clanInvites[0];
  if (!invite) {
    return '';
  }
  const remainingMs = Math.max(0, invite.expiresAtMs - nowMs);
  const expired = remainingMs <= 0;
  const fillPercent = Math.max(0, Math.min(100, (remainingMs / PARTY_INVITE_VISUAL_TTL_MS) * 100));
  const bottomOffset = state.partyInvites.length > 0 ? 198 : 104;
  return `
    <section
      class="frame-panel classic-window lineage-clan-invite-modal"
      data-clan-invite-modal
      style="position:absolute;left:50%;bottom:${bottomOffset}px;transform:translateX(-50%);width:300px;z-index:18;"
    >
      <div class="lineage-party-invite-timer" aria-hidden="true" style="height:4px;background:rgba(28,60,24,0.85);">
        <div style="height:100%;width:${fillPercent.toFixed(2)}%;background:linear-gradient(90deg,#63c46b,#2f8a39);"></div>
      </div>
      <div class="hud-window-title classic-title compact">
        <span>Clan Invitation</span>
      </div>
      <div class="lineage-party-invite-body" style="padding:10px 12px 12px;display:flex;flex-direction:column;gap:8px;">
        <div class="lineage-party-invite-copy">
          <strong>${escapeHudText(invite.clanName)}</strong>
          <span>${expired ? 'Invitation expired.' : `${escapeHudText(invite.inviterName)} invites you to join.`}</span>
        </div>
        <div class="lineage-party-inline-actions">
          <button type="button" data-clan-accept="${invite.inviteId}" data-no-drag ${expired ? 'disabled aria-disabled="true"' : ''}>Accept</button>
          <button type="button" data-clan-decline="${invite.inviteId}" data-no-drag>Cancel</button>
        </div>
      </div>
    </section>
  `;
};

export const renderAllianceInviteModal = (state: GameState, nowMs: number): string => {
  const invite = state.allianceInvites[0];
  if (!invite) {
    return '';
  }
  const remainingMs = Math.max(0, invite.expiresAtMs - nowMs);
  const expired = remainingMs <= 0;
  const fillPercent = Math.max(0, Math.min(100, (remainingMs / PARTY_INVITE_VISUAL_TTL_MS) * 100));
  const modalStack = state.partyInvites.length + state.clanInvites.length;
  const bottomOffset = 104 + modalStack * 94;
  return `
    <section
      class="frame-panel classic-window lineage-alliance-invite-modal"
      data-alliance-invite-modal
      style="position:absolute;left:50%;bottom:${bottomOffset}px;transform:translateX(-50%);width:316px;z-index:18;"
    >
      <div class="lineage-party-invite-timer" aria-hidden="true" style="height:4px;background:rgba(28,60,24,0.85);">
        <div style="height:100%;width:${fillPercent.toFixed(2)}%;background:linear-gradient(90deg,#63c46b,#2f8a39);"></div>
      </div>
      <div class="hud-window-title classic-title compact">
        <span>Alliance Invitation</span>
      </div>
      <div class="lineage-party-invite-body" style="padding:10px 12px 12px;display:flex;flex-direction:column;gap:8px;">
        <div class="lineage-party-invite-copy">
          <strong>${escapeHudText(invite.allianceName)}</strong>
          <span>${expired ? 'Invitation expired.' : `${escapeHudText(invite.inviterName)} invites your clan to join.`}</span>
        </div>
        <div class="lineage-party-inline-actions">
          <button type="button" data-alliance-accept="${invite.inviteId}" data-no-drag ${expired ? 'disabled aria-disabled="true"' : ''}>Accept</button>
          <button type="button" data-alliance-decline="${invite.inviteId}" data-no-drag>Cancel</button>
        </div>
      </div>
    </section>
  `;
};

const renderPartyPanel = (state: GameState): string => {
  const party = state.party;
  const isLeader = party?.leaderCharacterId === state.player.id;

  if (!party) {
    return `
      <section class="lineage-character-panel party-panel">
        <div class="lineage-panel-empty-copy">
          <strong>No Party</strong>
          <span>Use ALT+C or /invite with the current player target to send a party invite.</span>
        </div>
      </section>
    `;
  }

  return `
    <section class="lineage-character-panel party-panel">
      <div class="lineage-party-summary">
        <strong>${isLeader ? 'Party Leader' : 'Party Member'}</strong>
        <span>${party.members.length} member${party.members.length === 1 ? '' : 's'}</span>
      </div>
      <div class="lineage-party-roster">
        ${party.members
          .map(
            (member) => `
              <div class="lineage-party-row ${member.online ? 'online' : 'offline'}">
                <div class="lineage-party-member-copy">
                  <strong>${member.name}${member.isLeader ? ' *' : ''}</strong>
                  <span>Lv ${member.level} ${member.baseClass} · HP ${member.hp} · MP ${member.mp}</span>
                </div>
                <div class="lineage-party-member-meta">
                  <span>${member.online ? 'Online' : 'Offline'}</span>
                  ${
                    isLeader && member.characterId !== state.player.id
                      ? `<button type="button" data-party-kick="${member.characterId}" data-no-drag>Kick</button>`
                      : ''
                  }
                </div>
              </div>
            `,
          )
          .join('')}
      </div>
      <div class="lineage-party-footer">
        <button type="button" data-party-leave data-no-drag>Leave</button>
      </div>
    </section>
  `;
};

const renderCharacterPanelBody = (
  panelId: CharacterPanelId,
  state: GameState,
  stats: DerivedStats,
  cp: number,
  maxCp: number,
  selectedSkillBookSkills: PlayerKnownSkill[],
  skillBookTab: SkillBookTab,
  clanNameDraft: string,
  allianceNameDraft: string,
  clanInfoOpen: boolean,
): string => {
  if (panelId === 'status') {
    return renderStatusPanel(state, stats, cp, maxCp);
  }
  if (panelId === 'actions') {
    return renderActionsPanel();
  }
  if (panelId === 'clan') {
    return renderClanPanel(state, clanNameDraft, allianceNameDraft, clanInfoOpen);
  }
  if (panelId === 'quests') {
    return renderQuestPanel(state);
  }
  return `
    <div class="lineage-skill-tabs">
      <button
        type="button"
        class="${skillBookTab === 'active' ? 'active' : ''}"
        data-skill-book-tab="active"
        data-no-drag
      >Active</button>
      <button
        type="button"
        class="${skillBookTab === 'passive' ? 'active' : ''}"
        data-skill-book-tab="passive"
        data-no-drag
      >Passive</button>
    </div>
    <div class="lineage-skill-grid" aria-label="${skillBookTab === 'active' ? 'Active skills' : 'Passive skills'}">
      ${renderSkillBookGrid(selectedSkillBookSkills, skillBookTab === 'active')}
    </div>
  `;
};

const getInventoryIconCode = (template: ItemTemplate): string => {
  if (template.id === 'duskgold') {
    return 'DG';
  }
  if (template.appearance?.weaponModel === 'staff') {
    return 'ST';
  }
  if (template.appearance?.chestModel === 'robe') {
    return 'RB';
  }
  if (template.equipSlot === 'weapon') {
    return 'WP';
  }
  if (template.equipSlot === 'chest') {
    return 'AR';
  }
  if (template.equipSlot === 'gloves') {
    return 'GL';
  }
  if (template.equipSlot === 'boots') {
    return 'BT';
  }
  if (template.kind === 'material') {
    return 'MT';
  }
  if (template.kind === 'quest') {
    return 'QS';
  }
  if (template.kind === 'consumable') {
    return 'HP';
  }
  return 'IT';
};

const getInventoryIconTone = (template: ItemTemplate): string => {
  if (template.kind === 'currency') {
    return 'currency';
  }
  if (template.kind === 'weapon') {
    return 'weapon';
  }
  if (template.kind === 'armor') {
    return 'armor';
  }
  if (template.kind === 'material') {
    return 'material';
  }
  if (template.kind === 'quest') {
    return 'quest';
  }
  if (template.kind === 'consumable') {
    return 'generic';
  }
  return 'generic';
};

const getInventoryIconTint = (template: ItemTemplate): string => {
  if (template.appearance?.tint) {
    return template.appearance.tint;
  }
  if (template.kind === 'currency') {
    return '#d9b94f';
  }
  if (template.kind === 'weapon') {
    return '#a95f42';
  }
  if (template.kind === 'armor') {
    return '#6f8796';
  }
  if (template.kind === 'material') {
    return '#6e8d69';
  }
  if (template.kind === 'quest') {
    return '#9a72b8';
  }
  if (template.kind === 'consumable') {
    return '#b85858';
  }
  return '#7e8798';
};

const renderInventoryIconArt = (template: ItemTemplate, item: ItemInstance | null): string => {
  const quantityBadge =
    item && template.stackable && item.quantity > 1 ? `<span class="lineage-item-quantity">${item.quantity}</span>` : '';
  return `
    <span
      class="lineage-item-icon-art ${getInventoryIconTone(template)}"
      style="--inventory-icon-tint:${getInventoryIconTint(template)}"
      aria-hidden="true"
    >
      <span>${getInventoryIconCode(template)}</span>
    </span>
    ${quantityBadge}
  `;
};

const renderItemTooltip = (
  template: ItemTemplate,
  item: ItemInstance | null,
  attributeSummary: string | null,
  actions = '',
): string => `
  <div class="lineage-item-tooltip" role="tooltip">
    <strong>${item ? getItemLabel(item) : template.name}</strong>
    <small>${template.kind}${template.equipSlot ? ` - ${template.equipSlot}` : ''}</small>
    <p>${template.description}</p>
    ${attributeSummary ? `<em>${attributeSummary}</em>` : ''}
    ${item && template.stackable ? `<span>Quantity: ${item.quantity}</span>` : ''}
    ${actions ? `<div class="lineage-item-actions">${actions}</div>` : ''}
  </div>
`;

const renderEmptyInventorySlot = (label = 'Empty slot'): string => `
  <div class="lineage-inventory-slot empty" aria-label="${label}"></div>
`;

const renderInventoryGridSlot = (item: ItemInstance, actionHtml: string): string => {
  const template = getTemplate(item.templateId);
  const attributeSummary = getItemAttributeSummary(item);
  return `
    <div
      class="lineage-inventory-slot filled draggable"
      tabindex="0"
      draggable="true"
      data-inventory-hotbar-item="${item.id}"
      aria-label="${getItemLabel(item)}"
    >
      ${renderInventoryIconArt(template, item)}
      ${renderItemTooltip(template, item, attributeSummary, actionHtml)}
    </div>
  `;
};

const renderEquipmentSlot = (slot: EquipSlot, label: string, item: ItemInstance | null): string => {
  if (!item) {
    return `
      <div class="lineage-equipment-slot empty" tabindex="0" aria-label="${label} empty">
        <span class="lineage-equipment-placeholder">${label.slice(0, 2).toUpperCase()}</span>
      </div>
    `;
  }

  const template = getTemplate(item.templateId);
  const attributeSummary = getItemAttributeSummary(item);
  return `
    <div class="lineage-equipment-slot filled" tabindex="0" aria-label="${getItemLabel(item)}">
      ${renderInventoryIconArt(template, item)}
      ${renderItemTooltip(
        template,
        item,
        attributeSummary,
        `<button class="inventory-mini-action" data-slot="${slot}">Unequip</button>`,
      )}
    </div>
  `;
};

export const shouldBlockSkillDispatch = (
  skillButton: Pick<HTMLElement, 'dataset'> & Partial<Pick<HTMLButtonElement, 'disabled'>>,
): boolean => {
  return Boolean(skillButton.disabled) || skillButton.dataset?.skillDisabled === 'true';
};

export class Hud {
  private readonly root: HTMLDivElement;
  private readonly store: GameStore;
  private readonly controls: HudControls;
  private readonly interactive: boolean;
  private readonly showPersistenceControls: boolean;
  private readonly modeLabel: string | null;
  private readonly onUseSkill?: (skillId: string) => void;
  private readonly onUseHotbarAction?: (actionId: HotbarActionId) => void;
  private readonly onUseItem?: (itemId: string) => void;
  private readonly onEquipItem?: (itemId: string) => void;
  private readonly onUnequipItem?: (slot: EquipSlot) => void;
  private readonly onSplitItemStack?: (itemId: string, quantity: number) => void;
  private readonly onMergeItemStacks?: (sourceItemId: string, targetItemId: string) => void;
  private readonly onBuyVendorOffer?: (offerId: string, quantity: number) => void;
  private readonly onExchangeVendorOffer?: (offerId: string, quantity: number) => void;
  private readonly onOfferTradeItem?: (targetCharacterId: string, itemId: string, quantity: number) => void;
  private readonly onAcceptTradeOffer?: (offerId: string) => void;
  private readonly onDeclineTradeOffer?: (offerId: string) => void;
  private readonly onInvitePartyMember?: (targetCharacterId?: string) => void;
  private readonly onAcceptPartyInvite?: (inviteId: string) => void;
  private readonly onDeclinePartyInvite?: (inviteId: string) => void;
  private readonly onLeaveParty?: () => void;
  private readonly onKickPartyMember?: (targetCharacterId: string) => void;
  private readonly onCreateClan?: (name: string) => void;
  private readonly onInviteClanMember?: () => void;
  private readonly onAcceptClanInvite?: (inviteId: string) => void;
  private readonly onDeclineClanInvite?: (inviteId: string) => void;
  private readonly onLeaveClan?: () => void;
  private readonly onKickClanMember?: (targetCharacterId: string) => void;
  private readonly onDissolveClan?: () => void;
  private readonly onCreateAlliance?: (name: string) => void;
  private readonly onInviteAllianceClan?: () => void;
  private readonly onAcceptAllianceInvite?: (inviteId: string) => void;
  private readonly onDeclineAllianceInvite?: (inviteId: string) => void;
  private readonly onLeaveAlliance?: () => void;
  private readonly onExpelAllianceClan?: (targetClanId: string) => void;
  private readonly onDissolveAlliance?: () => void;
  private readonly onSellVendorItem?: (itemId: string, quantity: number) => void;
  private readonly onDepositWarehouseItem?: (itemId: string, quantity: number) => void;
  private readonly onWithdrawWarehouseItem?: (itemId: string, quantity: number) => void;
  private readonly onHotbarChange?: (hotbar: PlayerHotbarState) => void;
  private readonly onInteractNpc?: (npcId: string, actionId?: 'accept_task' | 'turn_in_task') => void;
  private readonly onCloseDialog?: () => void;
  private readonly onSendChatMessage?: (
    channel: 'region' | 'party' | 'alliance' | 'whisper',
    text: string,
    targetCharacterName?: string,
  ) => boolean;
  private lastSnapshot = '';
  private activeCharacterPanel: CharacterPanelId | null = null;
  private skillBookTab: SkillBookTab = 'active';
  private inventoryOpen = false;
  private mapOpen = false;
  private systemMenuOpen = false;
  private exitConfirmOpen = false;
  private systemPlaceholderOpen: SystemPlaceholderId | null = null;
  private partyWindowOpen = false;
  private clanInfoOpen = false;
  private activeChatFilter: ChatLogFilter = 'all';
  private chatDraft = '';
  private whisperTargetDraft = '';
  private clanNameDraft = '';
  private allianceNameDraft = '';
  private chatFocusField: 'target' | 'text' | null = null;
  private cameraYaw = 0;
  private visibleHotbarRowCount: HotbarOpenBarCount | null = null;
  private draggedHotbarEntry: HotbarShortcutPayload | null = null;
  private draggedHotbarSourceSlotIndex: number | null = null;
  private hotbarDropHandled = false;
  private dragGhostElement: HTMLElement | null = null;
  private readonly hotbarShortcutOverrides = new Map<number, HotbarShortcutOverride>();
  private readonly panelPositions = new Map<HudPanelId, { x: number; y: number }>();
  private dragState: HudDragState | null = null;
  private readonly handlePointerDownBound = this.handlePointerDown.bind(this);
  private readonly handlePointerMoveBound = this.handlePointerMove.bind(this);
  private readonly handlePointerUpBound = this.handlePointerUp.bind(this);
  private readonly handleDragStartBound = this.handleDragStart.bind(this);
  private readonly handleDragOverBound = this.handleDragOver.bind(this);
  private readonly handleDropBound = this.handleDrop.bind(this);
  private readonly handleDragEndBound = this.handleDragEnd.bind(this);
  private readonly handleSkillPointerMoveBound = this.handleSkillPointerMove.bind(this);
  private readonly handleSkillPointerUpBound = this.handleSkillPointerUp.bind(this);
  private readonly handleClickReleaseBound = this.releaseHudInteractionLock.bind(this);
  private readonly handleSubmitBound = this.handleSubmit.bind(this);
  private readonly handleInputBound = this.handleInput.bind(this);
  private readonly handleFocusInBound = this.handleFocusIn.bind(this);
  private hudInteractionLocked = false;
  private pendingHudState: GameState | null = null;
  private hudInteractionReleaseTimerId: ReturnType<typeof setTimeout> | null = null;
  private inviteCountdownIntervalId: ReturnType<typeof setInterval> | null = null;

  constructor(container: HTMLElement, store: GameStore, controls: HudControls, options?: HudOptions) {
    this.store = store;
    this.controls = controls;
    this.interactive = options?.interactive ?? true;
    this.showPersistenceControls = options?.showPersistenceControls ?? true;
    this.modeLabel = options?.modeLabel ?? null;
    this.onUseSkill = options?.onUseSkill;
    this.onUseHotbarAction = options?.onUseHotbarAction;
    this.onUseItem = options?.onUseItem;
    this.onEquipItem = options?.onEquipItem;
    this.onUnequipItem = options?.onUnequipItem;
    this.onSplitItemStack = options?.onSplitItemStack;
    this.onMergeItemStacks = options?.onMergeItemStacks;
    this.onBuyVendorOffer = options?.onBuyVendorOffer;
    this.onExchangeVendorOffer = options?.onExchangeVendorOffer;
    this.onOfferTradeItem = options?.onOfferTradeItem;
    this.onAcceptTradeOffer = options?.onAcceptTradeOffer;
    this.onDeclineTradeOffer = options?.onDeclineTradeOffer;
    this.onInvitePartyMember = options?.onInvitePartyMember;
    this.onAcceptPartyInvite = options?.onAcceptPartyInvite;
    this.onDeclinePartyInvite = options?.onDeclinePartyInvite;
    this.onLeaveParty = options?.onLeaveParty;
    this.onKickPartyMember = options?.onKickPartyMember;
    this.onCreateClan = options?.onCreateClan;
    this.onInviteClanMember = options?.onInviteClanMember;
    this.onAcceptClanInvite = options?.onAcceptClanInvite;
    this.onDeclineClanInvite = options?.onDeclineClanInvite;
    this.onLeaveClan = options?.onLeaveClan;
    this.onKickClanMember = options?.onKickClanMember;
    this.onDissolveClan = options?.onDissolveClan;
    this.onCreateAlliance = options?.onCreateAlliance;
    this.onInviteAllianceClan = options?.onInviteAllianceClan;
    this.onAcceptAllianceInvite = options?.onAcceptAllianceInvite;
    this.onDeclineAllianceInvite = options?.onDeclineAllianceInvite;
    this.onLeaveAlliance = options?.onLeaveAlliance;
    this.onExpelAllianceClan = options?.onExpelAllianceClan;
    this.onDissolveAlliance = options?.onDissolveAlliance;
    this.onSellVendorItem = options?.onSellVendorItem;
    this.onDepositWarehouseItem = options?.onDepositWarehouseItem;
    this.onWithdrawWarehouseItem = options?.onWithdrawWarehouseItem;
    this.onHotbarChange = options?.onHotbarChange;
    this.onInteractNpc = options?.onInteractNpc;
    this.onCloseDialog = options?.onCloseDialog;
    this.onSendChatMessage = options?.onSendChatMessage;
    this.root = document.createElement('div');
    this.root.className = 'hud-root';
    if (
      this.interactive ||
      this.onUseSkill ||
      this.onUseHotbarAction ||
      this.onUseItem ||
      this.onEquipItem ||
      this.onUnequipItem ||
      this.onSplitItemStack ||
      this.onMergeItemStacks ||
      this.onBuyVendorOffer ||
      this.onExchangeVendorOffer ||
      this.onOfferTradeItem ||
      this.onAcceptTradeOffer ||
      this.onDeclineTradeOffer ||
      this.onInvitePartyMember ||
      this.onAcceptPartyInvite ||
      this.onDeclinePartyInvite ||
      this.onLeaveParty ||
      this.onKickPartyMember ||
      this.onCreateClan ||
      this.onInviteClanMember ||
      this.onAcceptClanInvite ||
      this.onDeclineClanInvite ||
      this.onLeaveClan ||
      this.onKickClanMember ||
      this.onDissolveClan ||
      this.onSellVendorItem ||
      this.onDepositWarehouseItem ||
      this.onWithdrawWarehouseItem ||
      this.onHotbarChange ||
      this.onInteractNpc ||
      this.onCloseDialog ||
      this.onSendChatMessage
    ) {
      this.root.addEventListener('click', this.handleClick.bind(this));
      this.root.addEventListener('click', this.handleClickReleaseBound);
    }
    if (this.onSendChatMessage || this.onCreateClan) {
      this.root.addEventListener('submit', this.handleSubmitBound);
      this.root.addEventListener('input', this.handleInputBound);
    }
    if (this.onSendChatMessage) {
      this.root.addEventListener('focusin', this.handleFocusInBound);
    }
    this.root.addEventListener('pointerdown', this.handlePointerDownBound);
    this.root.addEventListener('dragstart', this.handleDragStartBound);
    this.root.addEventListener('dragover', this.handleDragOverBound);
    this.root.addEventListener('drop', this.handleDropBound);
    this.root.addEventListener('dragend', this.handleDragEndBound);
    container.appendChild(this.root);
  }

  update(state: GameState): void {
    if (this.hudInteractionLocked) {
      this.pendingHudState = state;
      return;
    }
    if (!state.clan) {
      this.clanInfoOpen = false;
    }
    this.syncInviteCountdownLoop(state);
    const snapshot = this.createSnapshot(state);
    if (snapshot === this.lastSnapshot) {
      return;
    }
    this.lastSnapshot = snapshot;

    const stats = getDerivedStats(state);
    const targetView = getTargetHudView(state);
    const inventory = getInventoryItems(state);
    const equipped = getEquippedBySlot(state);
    const castProgress = state.player.cast
      ? Math.max(0, 1 - state.player.cast.remainingMs / Math.max(state.player.cast.totalMs, 1))
      : 0;
    const effectiveHotbar = this.getEffectiveHotbarState(state);
    const visibleHotbarRowCount = effectiveHotbar.openBarCount;
    const hotbarRows = getVisibleHotbarRows(effectiveHotbar);
    const visualHotbarRows = [...hotbarRows].reverse();
    const cp = state.player.cp;
    const maxCp = stats.maxCp;
    const activeSkills = state.player.learnedSkills.filter((skill) => skill.category === 'active');
    const passiveSkills = state.player.learnedSkills.filter((skill) => skill.category === 'passive');
    const selectedSkillBookSkills = this.skillBookTab === 'passive' ? passiveSkills : activeSkills;
    const weaponAttributeSummary = equipped.weapon ? getItemAttributeSummary(equipped.weapon) : null;
    const chestAttributeSummary = equipped.chest ? getItemAttributeSummary(equipped.chest) : null;
    const glovesAttributeSummary = equipped.gloves ? getItemAttributeSummary(equipped.gloves) : null;
    const bootsAttributeSummary = equipped.boots ? getItemAttributeSummary(equipped.boots) : null;
    const nearbyVendor = getNearbyVendor(state);
    const nearbyWarehouse = getNearbyWarehouse(state);
    const nearbyTradePlayers = getNearbyTradePlayers(state).slice(0, 3);
    const nowMs = Date.now();
    const playerCombatView = getPlayerCombatHudView(state, nowMs);
    const composeChannel = composeChatChannelForFilter(this.activeChatFilter);
    const composeSendLabel =
      composeChannel === 'party' ? '#' : composeChannel === 'alliance' ? '&' : composeChannel === 'whisper' ? '@' : '~';
    const composePlaceholder =
      composeChannel === 'party'
        ? 'Party message'
        : composeChannel === 'alliance'
          ? 'Alliance message'
        : composeChannel === 'whisper'
          ? 'Whisper message'
          : 'Region message';
    const visibleLogs = state.logs.filter((entry) => chatFilterMatches(this.activeChatFilter, entry.channel));
    const tradeBlocked = Boolean(state.incomingTradeOffer || state.outgoingTradeOffer);
    const mergeTargetByTemplate = new Map<string, string>();
    const inventoryTemplateQuantities = new Map<string, number>();
    for (const item of inventory) {
      const template = getTemplate(item.templateId);
      inventoryTemplateQuantities.set(item.templateId, (inventoryTemplateQuantities.get(item.templateId) ?? 0) + item.quantity);
      if (!template.stackable || mergeTargetByTemplate.has(item.templateId)) {
        continue;
      }
      mergeTargetByTemplate.set(item.templateId, item.id);
    }
    const currencyQuantity = inventory
      .filter((item) => getTemplate(item.templateId).kind === 'currency')
      .reduce((total, item) => total + item.quantity, 0);
    const formattedCurrency = new Intl.NumberFormat('en-US').format(currencyQuantity);
    const equipmentSlotsHtml = [
      ...EQUIPMENT_SLOT_LAYOUT.map(({ slot, label }) => renderEquipmentSlot(slot, label, equipped[slot] ?? null)),
      ...Array.from({ length: 6 }, (_, index) => renderEmptyInventorySlot(`Empty equipment slot ${index + 1}`)),
    ].join('');
    const buildInventoryItemActions = (item: ItemInstance): string => {
      const template = getTemplate(item.templateId);
      const mergeTargetItemId = mergeTargetByTemplate.get(item.templateId) ?? null;
      const canSplit = template.stackable && item.quantity > 1;
      const canMerge = template.stackable && mergeTargetItemId !== null && mergeTargetItemId !== item.id;
      const sellValue = nearbyVendor ? getVendorSellValue(item.templateId) : null;
      const canDeposit = Boolean(nearbyWarehouse);
      const depositQuantity = template.stackable && item.quantity > 1 ? 1 : item.quantity;
      const sellQuantity = template.stackable && item.quantity > 1 ? 1 : item.quantity;
      const tradeQuantity = template.stackable && item.quantity > 1 ? 1 : item.quantity;
      return [
        template.kind === 'consumable' ? `<button class="inventory-mini-action" data-use-item="${item.id}">Use</button>` : '',
        template.equipSlot ? `<button class="inventory-mini-action" data-item="${item.id}">Equip</button>` : '',
        canSplit ? `<button class="inventory-mini-action" data-split-item="${item.id}" data-split-quantity="1">Split 1</button>` : '',
        canMerge
          ? `<button class="inventory-mini-action" data-merge-source="${item.id}" data-merge-target="${mergeTargetItemId}">Merge</button>`
          : '',
        canDeposit
          ? `<button class="inventory-mini-action" data-deposit-item="${item.id}" data-deposit-quantity="${depositQuantity}">${template.stackable && item.quantity > 1 ? 'Store 1' : 'Store'}</button>`
          : '',
        sellValue
          ? `<button class="inventory-mini-action" data-sell-item="${item.id}" data-sell-quantity="${sellQuantity}">${template.stackable && item.quantity > 1 ? `Sell 1 (${sellValue.amount})` : `Sell (${sellValue.amount})`}</button>`
          : '',
        nearbyTradePlayers.length > 0
          ? nearbyTradePlayers
              .map(
                (otherPlayer) => `
                  <button
                    class="inventory-mini-action"
                    data-offer-trade-item="${item.id}"
                    data-offer-trade-target="${otherPlayer.id}"
                    data-offer-trade-quantity="${tradeQuantity}"
                    ${tradeBlocked ? 'disabled aria-disabled="true"' : ''}
                  >
                    ${template.stackable && item.quantity > 1 ? `Offer 1 to ${otherPlayer.name}` : `Offer to ${otherPlayer.name}`}
                  </button>
                `,
              )
              .join('')
          : '',
      ].join('');
    };
    const inventoryGridSlots = [
      ...inventory
        .slice(0, INVENTORY_GRID_SLOT_COUNT)
        .map((item) => renderInventoryGridSlot(item, buildInventoryItemActions(item))),
      ...Array.from({ length: Math.max(0, INVENTORY_GRID_SLOT_COUNT - inventory.length) }, () => renderEmptyInventorySlot()),
    ].join('');
    const mapMarkerStyles = this.renderMapMarkerStyles(state);

    this.root.innerHTML = `
      ${
        this.modeLabel
          ? `
            <div class="frame-panel mode-chip">
              <div class="small-label">Runtime Mode</div>
              <div class="region-name">${this.modeLabel}</div>
            </div>
          `
          : ''
      }
      <div class="lineage-minimap" aria-label="Minimap">
        <div class="lineage-minimap-map" style="${mapMarkerStyles.mapOffset}">
          <span class="lineage-minimap-road north"></span>
          <span class="lineage-minimap-road east"></span>
          <span class="lineage-minimap-plaza"></span>
          <span class="lineage-minimap-water"></span>
          <span class="lineage-minimap-grove a"></span>
          <span class="lineage-minimap-grove b"></span>
        </div>
        <span class="lineage-minimap-camera" style="${mapMarkerStyles.minimapCamera}"></span>
        <span class="lineage-minimap-player"></span>
      </div>
      <div class="frame-panel player-frame classic-window ${state.player.deadUntilMs ? 'dead' : ''}" data-hud-panel="player"${this.renderPanelStyle('player')}>
        <div class="hud-drag-rail player-drag-rail" data-hud-drag="player" aria-label="Drag character status"></div>
        <div class="classic-player-identity">
          <span>Lv ${state.player.level}</span>
          <strong>${state.player.name}</strong>
        </div>
        <div class="classic-player-combat-row">
          <span class="classic-player-combat-state ${playerCombatView.variant}">${playerCombatView.label}</span>
          <span class="classic-player-combat-detail">${playerCombatView.detail}</span>
        </div>
        <div class="classic-status-rows">
          <div class="classic-status-row cp">
            <span>CP</span>
            <div class="classic-status-bar">
              <i style="width:${clampPercent(cp, maxCp)}%"></i>
              <strong>${Math.round(cp)}/${maxCp}</strong>
            </div>
          </div>
          <div class="classic-status-row hp">
            <span>HP</span>
            <div class="classic-status-bar">
              <i style="width:${clampPercent(state.player.hp, stats.maxHp)}%"></i>
              <strong>${Math.round(state.player.hp)}/${stats.maxHp}</strong>
            </div>
          </div>
          <div class="classic-status-row mp">
            <span>MP</span>
            <div class="classic-status-bar">
              <i style="width:${clampPercent(state.player.mp, stats.maxMp)}%"></i>
              <strong>${Math.round(state.player.mp)}/${stats.maxMp}</strong>
            </div>
          </div>
        </div>
        <div class="classic-xp-row">
          <strong>${((state.player.xp / xpGoalForLevel(state.player.level)) * 100).toFixed(2)}%</strong>
        </div>
      </div>

      ${
        this.partyWindowOpen
          ? `
            <div class="frame-panel classic-window party-frame" data-hud-panel="party"${this.renderPanelStyle('party')}>
              <div class="hud-window-title classic-title compact" data-hud-drag="party">
                <span>Party</span>
                <div class="lineage-window-controls">
                  <button type="button" data-party-close data-no-drag aria-label="Close party window">x</button>
                </div>
              </div>
              ${renderPartyPanel(state)}
            </div>
          `
          : ''
      }
      ${renderPartyInviteModal(state, nowMs)}
      ${renderClanInviteModal(state, nowMs)}
      ${renderAllianceInviteModal(state, nowMs)}

      ${
        targetView
          ? `
            <div class="frame-panel target-frame classic-window ${targetView.tone}" data-hud-panel="target"${this.renderPanelStyle('target')}>
              <div class="hud-window-title classic-title target-title" data-hud-drag="target">
                <span>${targetView.name}</span>
                <i aria-hidden="true">x</i>
              </div>
              ${
                targetView.hpPercent !== null
                  ? `
                    <div class="classic-target-bar">
                      <span style="width:${targetView.hpPercent}%"></span>
                    </div>
                  `
                  : ''
              }
              <div class="target-meta">
                <span>${targetView.subtitle}</span>
                ${targetView.hpLabel ? `<strong>${targetView.hpLabel}</strong>` : ''}
              </div>
            </div>
          `
          : ''
      }

      <div class="panel hotbar-panel" data-hud-panel="hotbar"${this.renderPanelStyle('hotbar')}>
        <div class="hud-drag-rail left" data-hud-drag="hotbar" aria-label="Drag skill hotbar"></div>
        <button
          class="hotbar-row-toggle"
          data-hotbar-row-toggle
          title="${visibleHotbarRowCount >= 3 ? 'Collapse shortcut rows' : 'Add shortcut row'}"
          aria-label="${visibleHotbarRowCount >= 3 ? 'Collapse shortcut rows' : 'Add shortcut row'}"
        >&#9650;</button>
        <div class="hotbar-viewport">
          <div class="hotbar-rows">
            ${visualHotbarRows
              .map(
                (row) => `
                  <div class="hotbar-row">
                    ${row.map((slot) => renderHotbarSlot(state, slot)).join('')}
                  </div>
                `,
              )
              .join('')}
          </div>
        </div>
        ${
          state.player.cast
            ? `
              <div class="classic-cast-strip">
                <span>${gameTemplates.skills[state.player.cast.skillId].name}</span>
                <div class="classic-cast-bar"><i style="width:${castProgress * 100}%"></i></div>
              </div>
            `
            : ''
        }
      </div>

      ${
        this.activeCharacterPanel
          ? `
            <div class="skill-book-panel frame-panel lineage-skill-window ${
              this.activeCharacterPanel === 'clan' ? 'lineage-skill-window--clan' : ''
            }" data-hud-panel="skill-book"${this.renderPanelStyle('skill-book')}>
              <div class="lineage-skill-title hud-window-title" data-hud-drag="skill-book">
                <span>${CHARACTER_PANEL_NAV.find((entry) => entry.id === this.activeCharacterPanel)?.label ?? 'Skills'}</span>
                <div class="lineage-window-controls">
                  <button type="button" data-no-drag aria-label="Minimize character window">-</button>
                  <button type="button" data-character-panel-close data-no-drag aria-label="Close character window">x</button>
                </div>
              </div>
              <div class="lineage-panel-nav" role="tablist" aria-label="Character window navigation">
                ${renderCharacterPanelNav(this.activeCharacterPanel)}
              </div>
              ${renderCharacterPanelBody(
                this.activeCharacterPanel,
                state,
                stats,
                cp,
                maxCp,
                selectedSkillBookSkills,
                this.skillBookTab,
                this.clanNameDraft,
                this.allianceNameDraft,
                this.clanInfoOpen,
              )}
            </div>
          `
          : ''
      }

      <div class="panel log-panel frame-panel classic-window" data-hud-panel="log"${this.renderPanelStyle('log')}>
        <div class="hud-window-title classic-title compact" data-hud-drag="log">
          <span>Chat</span>
        </div>
        <div class="log-list">
          ${visibleLogs
            .map((entry) => `<div class="log-line ${entry.tone}">${escapeHudText(entry.text)}</div>`)
            .join('')}
        </div>
        <div class="chat-tabs" aria-label="Chat channels">
          <button type="button" data-chat-filter="all" class="${this.activeChatFilter === 'all' ? 'active' : ''}">All</button>
          <button type="button" data-chat-filter="region" class="${this.activeChatFilter === 'region' ? 'active' : ''}">Region</button>
          <button type="button" data-chat-filter="party" class="${this.activeChatFilter === 'party' ? 'active' : ''}">#Party</button>
          <button type="button" data-chat-filter="alliance" class="${this.activeChatFilter === 'alliance' ? 'active' : ''}">&amp;Alliance</button>
          <button type="button" data-chat-filter="whisper" class="${this.activeChatFilter === 'whisper' ? 'active' : ''}">@Whisper</button>
          <button type="button" class="disabled" disabled aria-disabled="true">+Trade</button>
        </div>
        ${
          this.onSendChatMessage
            ? `
              <form class="chat-compose" data-chat-compose>
                ${
                  composeChannel === 'whisper'
                    ? `<input class="chat-target-input" data-chat-target name="chat_target" type="text" value="${escapeHudText(this.whisperTargetDraft)}" placeholder="Target" maxlength="24" autocomplete="off" />`
                    : ''
                }
                <input
                  class="chat-text-input"
                  data-chat-text
                  name="chat_text"
                  type="text"
                  value="${escapeHudText(this.chatDraft)}"
                  placeholder="${composePlaceholder}"
                  maxlength="240"
                  autocomplete="off"
                />
                <button type="submit" class="chat-send-button">${composeSendLabel}Say</button>
              </form>
            `
            : ''
        }
      </div>

      <div class="lineage-quick-menu" data-hud-panel="quick-menu"${this.renderPanelStyle('quick-menu')}>
        <div class="hud-drag-rail left" data-hud-drag="quick-menu" aria-label="Drag quick access menu"></div>
        <button type="button" data-quick-menu="status" title="Character Status (Alt+T)" aria-label="Character Status (Alt+T)">
          <span class="lineage-quick-icon lineage-quick-icon--status">ST</span>
        </button>
        <button type="button" data-quick-menu="inventory" title="Inventory (Alt+V)" aria-label="Inventory (Alt+V)">
          <span class="lineage-quick-icon lineage-quick-icon--inventory">IV</span>
        </button>
        <button type="button" data-quick-menu="map" title="Map (Alt+M)" aria-label="Map (Alt+M)">
          <span class="lineage-quick-icon lineage-quick-icon--map">MP</span>
        </button>
        <button type="button" data-quick-menu="system" title="System (Alt+X)" aria-label="System (Alt+X)">
          <span class="lineage-quick-icon lineage-quick-icon--system">SY</span>
        </button>
      </div>

      ${
        this.inventoryOpen
          ? `
            <div class="panel inventory-panel frame-panel lineage-inventory-window" data-hud-panel="inventory"${this.renderPanelStyle('inventory')}>
              <div class="lineage-inventory-title hud-window-title" data-hud-drag="inventory">
                <span>Inventory</span>
                <div class="lineage-window-controls">
                  <button type="button" data-no-drag aria-label="Minimize inventory">-</button>
                  <button type="button" data-inventory-close data-no-drag aria-label="Close inventory">x</button>
                </div>
              </div>
              <div class="lineage-inventory-equipment">
                ${equipmentSlotsHtml}
              </div>
              <div class="lineage-inventory-tabs">
                <button class="active" type="button">Item</button>
                <button type="button">Quest</button>
                <span>(${inventory.length}/${INVENTORY_CAPACITY})</span>
              </div>
              <div class="lineage-inventory-grid">
                ${inventoryGridSlots}
              </div>
              <div class="lineage-inventory-footer">
                <button class="lineage-inventory-round-action" type="button" aria-label="Inventory action">
                  <span></span>
                </button>
                <div class="lineage-inventory-ledger">
                  <div class="lineage-ledger-row">
                    <span class="lineage-ledger-icon coin">DG</span>
                    <strong>${formattedCurrency}</strong>
                  </div>
                  <div class="lineage-ledger-row">
                    <span class="lineage-ledger-icon weight">WT</span>
                    <strong>${FUTURE_WEIGHT_PERCENT}</strong>
                  </div>
                </div>
                <button class="lineage-inventory-trash" type="button" aria-label="Discard item preview"></button>
              </div>
            </div>
          `
          : ''
      }


      ${
        state.dialog
          ? `
            <div class="dialog-backdrop">
              <div class="dialog-card frame-panel" data-hud-panel="dialog"${this.renderPanelStyle('dialog')}>
                <div class="frame-title hud-window-title" data-hud-drag="dialog">
                  <span>${state.dialog.title}</span>
                  <span>NPC</span>
                </div>
                <div class="dialog-copy">${state.dialog.body}</div>
                <div class="dialog-actions">
                  ${
                    state.dialog.actionId && state.dialog.actionLabel
                      ? `<button class="primary" data-dialog-action="${state.dialog.actionId}" data-dialog-npc="${state.dialog.npcId}">${state.dialog.actionLabel}</button>`
                      : ''
                  }
                  <button data-close-dialog>Close</button>
                </div>
              </div>
            </div>
          `
          : ''
      }

      ${
        this.mapOpen
          ? `
            <div class="lineage-map-window frame-panel classic-window" data-hud-panel="world-map"${this.renderPanelStyle('world-map')}>
              <div class="hud-window-title classic-title compact" data-hud-drag="world-map">
                <span>Map</span>
                <div class="lineage-window-controls">
                  <button type="button" data-map-close data-no-drag aria-label="Close map">x</button>
                </div>
              </div>
              <div class="lineage-map-toolbar">
                <button type="button">- Cursed Weapon -</button>
                <button type="button">Find</button>
                <button type="button">World Info.</button>
              </div>
              <div class="lineage-map-canvas" aria-label="Current map" style="${mapMarkerStyles.mapOffset}">
                <span class="lineage-map-road north"></span>
                <span class="lineage-map-road east"></span>
                <span class="lineage-map-water"></span>
                <span class="lineage-map-grove a"></span>
                <span class="lineage-map-grove b"></span>
                <span class="lineage-map-plaza"></span>
                <span class="lineage-map-sun">PM 05 : 15</span>
                <span class="lineage-map-label warehouse">Warehouse</span>
                <span class="lineage-map-label gatekeeper">Gatekeeper</span>
                <span class="lineage-map-label temple">Temple</span>
                <span class="lineage-map-label magic">Magic Shop</span>
                <i class="lineage-map-camera" style="${mapMarkerStyles.mapCamera}" aria-hidden="true"></i>
                <i class="lineage-map-player" style="${mapMarkerStyles.marker}" aria-hidden="true"></i>
              </div>
              <div class="lineage-map-footer">
                <span>Current Position : ${escapeHudText(getRegionIdForPoint(state.player.position))}</span>
                <div>
                  <button type="button">Current Loc.</button>
                  <button type="button">Party Member</button>
                  <button type="button">Target Loc.</button>
                  <button type="button">Enlarge Map</button>
                </div>
              </div>
            </div>
          `
          : ''
      }

      ${
        this.systemMenuOpen
          ? `
            <div class="lineage-system-window frame-panel classic-window" data-hud-panel="system-menu"${this.renderPanelStyle('system-menu')}>
              <div class="hud-window-title classic-title compact">
                <span>System Menu</span>
                <div class="lineage-window-controls">
                  <button type="button" data-system-close data-no-drag aria-label="Close system menu">x</button>
                </div>
              </div>
              <button type="button" data-system-placeholder="community"><span>CM</span>Community(Alt+B)</button>
              <button type="button" data-system-placeholder="macro"><span>MR</span>Macro(Alt+R)</button>
              <button type="button" data-system-placeholder="help"><span>?</span>Help</button>
              <button type="button" data-system-placeholder="petition"><span>PT</span>Petition</button>
              <button type="button" data-system-placeholder="options"><span>OP</span>Options</button>
              <button type="button" data-system-placeholder="restart"><span>RS</span>Restart</button>
              <button type="button" data-exit-game-open><span>EX</span>Exit Game</button>
            </div>
          `
          : ''
      }

      ${
        this.systemPlaceholderOpen
          ? `
            <div class="lineage-system-placeholder frame-panel classic-window" data-hud-panel="system-placeholder"${this.renderPanelStyle('system-placeholder')}>
              <div class="hud-window-title classic-title compact">
                <span>${this.renderSystemPlaceholderTitle(this.systemPlaceholderOpen)}</span>
                <div class="lineage-window-controls">
                  <button type="button" data-system-placeholder-close data-no-drag aria-label="Close window">x</button>
                </div>
              </div>
              <div class="lineage-system-placeholder-body">
                <span class="lineage-system-placeholder-icon">${this.renderSystemPlaceholderIcon(this.systemPlaceholderOpen)}</span>
                <p>${this.renderSystemPlaceholderMessage(this.systemPlaceholderOpen)}</p>
              </div>
              <div class="lineage-system-placeholder-actions">
                <button class="game-menu-button" type="button" data-system-placeholder-close data-no-drag>OK</button>
              </div>
            </div>
          `
          : ''
      }

      ${
        this.exitConfirmOpen
          ? `
            <div class="lineage-exit-confirm">
              <div class="lineage-exit-box">
                <span class="lineage-exit-warning">!</span>
                <p>Do you wish to exit the game?</p>
                <div>
                  <button class="game-menu-button" type="button" data-exit-game-ok>OK</button>
                  <button class="game-menu-button" type="button" data-exit-game-cancel>Cancel</button>
                </div>
              </div>
            </div>
          `
          : ''
      }
    `;
    this.syncChatComposerFocus();
  }

  destroy(): void {
    if (this.inviteCountdownIntervalId !== null) {
      clearInterval(this.inviteCountdownIntervalId);
      this.inviteCountdownIntervalId = null;
    }
  }

  private renderPanelStyle(panelId: HudPanelId): string {
    const position = this.panelPositions.get(panelId);
    if (!position) {
      return '';
    }
    return ` style="left:${position.x}px;top:${position.y}px;right:auto;bottom:auto;transform:none;"`;
  }

  private isHudPanelId(value: string | undefined): value is HudPanelId {
    return (
      value === 'player' ||
      value === 'party' ||
      value === 'target' ||
      value === 'hotbar' ||
      value === 'skill-book' ||
      value === 'log' ||
      value === 'inventory' ||
      value === 'dialog' ||
      value === 'quick-menu' ||
      value === 'world-map' ||
      value === 'system-menu' ||
      value === 'system-placeholder'
    );
  }

  private applyPanelPosition(panelId: HudPanelId): void {
    const panel = this.root.querySelector<HTMLElement>(`[data-hud-panel="${panelId}"]`);
    const position = this.panelPositions.get(panelId);
    if (!panel || !position) {
      return;
    }
    panel.style.left = `${position.x}px`;
    panel.style.top = `${position.y}px`;
    panel.style.right = 'auto';
    panel.style.bottom = 'auto';
    panel.style.transform = 'none';
  }

  private handlePointerDown(event: PointerEvent): void {
    const target = event.target as HTMLElement | null;
    if (event.button === 0 && this.handleImmediateClose(target)) {
      event.preventDefault();
      event.stopPropagation();
      return;
    }
    if (event.button === 0 && target && target !== this.root) {
      this.beginHudInteractionLock(750);
    }

    const skillSlot = target?.closest<HTMLElement>('[data-skill-book-skill]');
    if (event.button === 0 && skillSlot) {
      const skillId = skillSlot.dataset.skillBookSkill;
      const knownSkill = skillId ? findKnownSkill(this.store.getState().player.learnedSkills, skillId) : null;
      if (skillId && knownSkill?.category === 'active') {
        this.beginHudInteractionLock();
        this.draggedHotbarEntry = { entryType: 'skill', skillId };
        this.showHotbarDragGhost(this.draggedHotbarEntry, event.clientX, event.clientY);
        skillSlot.classList.add('is-dragging');
        window.addEventListener('pointermove', this.handleSkillPointerMoveBound);
        window.addEventListener('pointerup', this.handleSkillPointerUpBound, { once: true });
        event.preventDefault();
        event.stopPropagation();
        return;
      }
    }

    const handle = target?.closest<HTMLElement>('[data-hud-drag]');
    if (!target || !handle) {
      return;
    }
    if (target.closest('button, input, select, textarea, [data-no-drag]')) {
      return;
    }

    const panelId = handle.dataset.hudDrag;
    if (!this.isHudPanelId(panelId)) {
      return;
    }
    const panel = this.root.querySelector<HTMLElement>(`[data-hud-panel="${panelId}"]`);
    if (!panel) {
      return;
    }
    this.beginHudInteractionLock();

    const rootRect = this.root.getBoundingClientRect();
    const panelRect = panel.getBoundingClientRect();
    const x = panelRect.left - rootRect.left;
    const y = panelRect.top - rootRect.top;
    this.panelPositions.set(panelId, { x, y });
    this.applyPanelPosition(panelId);
    this.dragState = {
      panelId,
      offsetX: event.clientX - panelRect.left,
      offsetY: event.clientY - panelRect.top,
      width: panelRect.width,
      height: panelRect.height,
    };
    this.root.classList.add('hud-root--dragging');
    event.preventDefault();
    event.stopPropagation();
    window.addEventListener('pointermove', this.handlePointerMoveBound);
    window.addEventListener('pointerup', this.handlePointerUpBound, { once: true });
  }

  private handleImmediateClose(target: HTMLElement | null): boolean {
    if (!target) {
      return false;
    }

    if (target.closest<HTMLElement>('[data-character-panel-close]')) {
      this.activeCharacterPanel = null;
    } else if (target.closest<HTMLElement>('[data-inventory-close]')) {
      this.inventoryOpen = false;
    } else if (target.closest<HTMLElement>('[data-map-close]')) {
      this.mapOpen = false;
    } else if (target.closest<HTMLElement>('[data-system-close]')) {
      this.systemMenuOpen = false;
      this.exitConfirmOpen = false;
      this.systemPlaceholderOpen = null;
    } else if (target.closest<HTMLElement>('[data-system-placeholder-close]')) {
      this.systemPlaceholderOpen = null;
    } else if (target.closest<HTMLElement>('[data-party-close]')) {
      this.partyWindowOpen = false;
    } else if (target.closest<HTMLElement>('[data-clan-info-close]')) {
      this.clanInfoOpen = false;
    } else if (target.closest<HTMLElement>('[data-exit-game-cancel]')) {
      this.exitConfirmOpen = false;
    } else if (target.closest<HTMLElement>('[data-close-dialog]')) {
      if (this.onCloseDialog) {
        this.onCloseDialog();
      } else if (this.interactive) {
        this.store.dispatch({ type: 'closeDialog' });
      }
      return true;
    } else {
      return false;
    }

    this.lastSnapshot = '';
    this.update(this.store.getState());
    return true;
  }

  private handlePointerMove(event: PointerEvent): void {
    if (!this.dragState) {
      return;
    }
    const rootRect = this.root.getBoundingClientRect();
    const maxX = Math.max(0, rootRect.width - this.dragState.width);
    const maxY = Math.max(0, rootRect.height - this.dragState.height);
    const x = Math.max(0, Math.min(maxX, event.clientX - rootRect.left - this.dragState.offsetX));
    const y = Math.max(0, Math.min(maxY, event.clientY - rootRect.top - this.dragState.offsetY));
    this.panelPositions.set(this.dragState.panelId, { x, y });
    this.applyPanelPosition(this.dragState.panelId);
    event.preventDefault();
  }

  private handlePointerUp(): void {
    this.dragState = null;
    this.root.classList.remove('hud-root--dragging');
    window.removeEventListener('pointermove', this.handlePointerMoveBound);
    this.releaseHudInteractionLock();
  }

  private beginHudInteractionLock(autoReleaseMs = 0): void {
    this.hudInteractionLocked = true;
    if (this.hudInteractionReleaseTimerId) {
      clearTimeout(this.hudInteractionReleaseTimerId);
      this.hudInteractionReleaseTimerId = null;
    }
    if (autoReleaseMs > 0) {
      this.hudInteractionReleaseTimerId = setTimeout(() => {
        this.releaseHudInteractionLock();
      }, autoReleaseMs);
    }
  }

  private releaseHudInteractionLock(): void {
    if (this.hudInteractionReleaseTimerId) {
      clearTimeout(this.hudInteractionReleaseTimerId);
      this.hudInteractionReleaseTimerId = null;
    }
    if (!this.hudInteractionLocked) {
      return;
    }
    this.hudInteractionLocked = false;
    const pendingState = this.pendingHudState;
    this.pendingHudState = null;
    if (pendingState) {
      this.lastSnapshot = '';
      this.update(pendingState);
    }
  }

  toggleCharacterPanel(panelId: CharacterPanelId): void {
    this.activeCharacterPanel = this.activeCharacterPanel === panelId ? null : panelId;
    this.lastSnapshot = '';
    this.update(this.store.getState());
  }

  toggleSkillBook(): void {
    this.toggleCharacterPanel('skills');
  }

  toggleInventory(): void {
    this.inventoryOpen = !this.inventoryOpen;
    this.lastSnapshot = '';
    this.update(this.store.getState());
  }

  setCameraYaw(cameraYaw: number): void {
    if (!Number.isFinite(cameraYaw)) {
      return;
    }
    this.cameraYaw = cameraYaw;
  }

  toggleMap(): void {
    this.mapOpen = !this.mapOpen;
    this.lastSnapshot = '';
    this.update(this.store.getState());
  }

  toggleSystemMenu(): void {
    this.systemMenuOpen = !this.systemMenuOpen;
    if (!this.systemMenuOpen) {
      this.exitConfirmOpen = false;
      this.systemPlaceholderOpen = null;
    }
    this.lastSnapshot = '';
    this.update(this.store.getState());
  }

  openSystemMenuPlaceholder(placeholderId: SystemPlaceholderId): void {
    this.systemMenuOpen = true;
    this.systemPlaceholderOpen = placeholderId;
    this.lastSnapshot = '';
    this.update(this.store.getState());
  }

  private renderSystemPlaceholderTitle(placeholderId: SystemPlaceholderId): string {
    switch (placeholderId) {
      case 'community':
        return 'Community';
      case 'macro':
        return 'Macro';
      case 'help':
        return 'Help';
      case 'petition':
        return 'Petition';
      case 'options':
        return 'Options';
      case 'restart':
        return 'Restart';
    }
  }

  private renderSystemPlaceholderIcon(placeholderId: SystemPlaceholderId): string {
    switch (placeholderId) {
      case 'community':
        return 'CM';
      case 'macro':
        return 'MR';
      case 'help':
        return '?';
      case 'petition':
        return 'PT';
      case 'options':
        return 'OP';
      case 'restart':
        return 'RS';
    }
  }

  private renderSystemPlaceholderMessage(placeholderId: SystemPlaceholderId): string {
    switch (placeholderId) {
      case 'community':
        return 'Community board placeholder. This window is reserved for future server news, rankings, market notices and social shortcuts.';
      case 'macro':
        return 'Macro placeholder. Future support will let players organize command sequences without client-side authority.';
      case 'help':
        return 'Help placeholder. This will become the in-game guide for movement, targeting, combat, party, clan and economy flows.';
      case 'petition':
        return 'Petition placeholder. This will become the support request flow when moderation and support tooling are implemented.';
      case 'options':
        return 'Options placeholder. Future client settings will live here, including audio, graphics, camera and interface preferences.';
      case 'restart':
        return 'Restart placeholder. Character restart will require an authoritative logout/session teardown flow before it becomes active.';
    }
  }

  private renderMapMarkerStyles(state: GameState): {
    mapOffset: string;
    marker: string;
    minimapCamera: string;
    mapCamera: string;
  } {
    const xPercent = minimapPercentForAxis(state.player.position.x);
    const zPercent = minimapPercentForAxis(state.player.position.z);
    const mapOffsetX = 50 - xPercent;
    const mapOffsetY = 50 - zPercent;
    const cameraYaw = Number.isFinite(this.cameraYaw) ? this.cameraYaw : 0;
    const cameraDegrees = THREE_RAD_TO_DEG * cameraYaw + 90;
    return {
      mapOffset: `--map-offset-x:${mapOffsetX.toFixed(2)}%;--map-offset-y:${mapOffsetY.toFixed(2)}%;`,
      marker: `left:${xPercent.toFixed(2)}%;top:${zPercent.toFixed(2)}%;`,
      minimapCamera: `transform:translate(-50%, -50%) rotate(${cameraDegrees.toFixed(2)}deg);`,
      mapCamera: `left:${xPercent.toFixed(2)}%;top:${zPercent.toFixed(2)}%;transform:translate(-50%, -50%) rotate(${cameraDegrees.toFixed(2)}deg);`,
    };
  }

  togglePartyPanel(): void {
    this.partyWindowOpen = !this.partyWindowOpen;
    this.lastSnapshot = '';
    this.update(this.store.getState());
  }

  private syncInviteCountdownLoop(state: GameState): void {
    const nowMs = Date.now();
    const hasTrackedInvite =
      state.partyInvites.some((invite) => invite.expiresAtMs > nowMs) ||
      state.clanInvites.some((invite) => invite.expiresAtMs > nowMs) ||
      (state.player.pvpFlagged === true &&
        typeof state.player.pvpFlagUntilMs === 'number' &&
        state.player.pvpFlagUntilMs > nowMs);
    if (hasTrackedInvite) {
      if (this.inviteCountdownIntervalId === null) {
        this.inviteCountdownIntervalId = setInterval(() => {
          this.lastSnapshot = '';
          this.update(this.store.getState());
        }, 250);
      }
      return;
    }
    if (this.inviteCountdownIntervalId !== null) {
      clearInterval(this.inviteCountdownIntervalId);
      this.inviteCountdownIntervalId = null;
    }
  }

  focusChatInput(): void {
    if (!this.onSendChatMessage) {
      return;
    }
    this.chatFocusField = this.activeChatFilter === 'whisper' && !this.whisperTargetDraft ? 'target' : 'text';
    this.lastSnapshot = '';
    this.update(this.store.getState());
  }

  cancelChatInput(): void {
    this.chatFocusField = null;
    const activeElement = this.root.ownerDocument.activeElement;
    if (activeElement instanceof HTMLElement) {
      activeElement.blur();
    }
  }

  isChatInputFocused(): boolean {
    const activeElement = this.root.ownerDocument.activeElement;
    if (!(activeElement instanceof HTMLElement)) {
      return false;
    }
    return Boolean(activeElement.closest('[data-chat-compose]'));
  }

  submitChatInput(): boolean {
    if (!this.onSendChatMessage) {
      return false;
    }
    return this.dispatchChatMessage();
  }

  private getEffectiveHotbarState(state: GameState): PlayerHotbarState {
    const openBarCount = this.visibleHotbarRowCount ?? normalizeHotbarOpenBarCount(state.player.hotbar.openBarCount);
    return {
      openBarCount,
      slots: state.player.hotbar.slots.map((slot) => {
        if (!this.hotbarShortcutOverrides.has(slot.slotIndex)) {
          return { ...slot };
        }
        const overrideEntry = this.hotbarShortcutOverrides.get(slot.slotIndex);
        if (!overrideEntry) {
          return emptyHotbarSlot(slot.slotIndex);
        }
        return hotbarPayloadToSlot(slot.slotIndex, overrideEntry);
      }),
    };
  }

  private notifyHotbarChanged(state: GameState = this.store.getState()): void {
    this.onHotbarChange?.(this.getEffectiveHotbarState(state));
  }

  private toggleHotbarRows(): void {
    const state = this.store.getState();
    const current = this.visibleHotbarRowCount ?? normalizeHotbarOpenBarCount(state.player.hotbar.openBarCount);
    this.visibleHotbarRowCount = current >= 3 ? 1 : normalizeHotbarOpenBarCount(current + 1);
    this.notifyHotbarChanged(state);
    this.lastSnapshot = '';
    this.update(state);
  }

  private bindShortcutToHotbar(slotIndex: number, payload: HotbarShortcutPayload): void {
    const state = this.store.getState();
    if (payload.entryType === 'skill') {
      const knownSkill = findKnownSkill(state.player.learnedSkills, payload.skillId);
      if (!knownSkill || knownSkill.category !== 'active') {
        return;
      }
    }
    if (payload.entryType === 'item') {
      const item = state.items[payload.itemId];
      if (!item || item.container !== 'inventory') {
        return;
      }
    }
    if (payload.entryType === 'action' && !HOTBAR_ACTIONS[payload.actionId]) {
      return;
    }
    this.hotbarShortcutOverrides.set(slotIndex, payload);
    const requiredRows = normalizeHotbarOpenBarCount(Math.floor(slotIndex / HOTBAR_ROW_SIZE) + 1);
    const currentRows = this.visibleHotbarRowCount ?? normalizeHotbarOpenBarCount(state.player.hotbar.openBarCount);
    if (requiredRows > currentRows) {
      this.visibleHotbarRowCount = requiredRows;
    }
    this.notifyHotbarChanged(state);
    this.lastSnapshot = '';
    this.update(state);
  }

  private clearShortcutFromHotbar(slotIndex: number): void {
    if (!Number.isInteger(slotIndex) || slotIndex < 0) {
      return;
    }
    this.hotbarShortcutOverrides.set(slotIndex, null);
    this.notifyHotbarChanged();
    this.lastSnapshot = '';
    this.update(this.store.getState());
  }

  private getHotbarPayloadFromShortcutElement(element: HTMLElement): HotbarShortcutPayload | null {
    const entryType = element.dataset.hotbarEntryType;
    const shortcutId = element.dataset.hotbarShortcutId;
    if (entryType === 'skill' && shortcutId) {
      return { entryType: 'skill', skillId: shortcutId };
    }
    if (entryType === 'item' && shortcutId) {
      return { entryType: 'item', itemId: shortcutId };
    }
    if (entryType === 'action' && shortcutId && HOTBAR_ACTIONS[shortcutId as HotbarActionId]) {
      return { entryType: 'action', actionId: shortcutId as HotbarActionId };
    }
    return null;
  }

  private showHotbarDragGhost(payload: HotbarShortcutPayload, clientX: number, clientY: number): void {
    this.removeSkillDragGhost();
    const ghost = document.createElement('div');
    ghost.className = 'lineage-hotbar-drag-ghost';
    if (payload.entryType === 'skill') {
      const template = gameTemplates.skills[payload.skillId];
      if (!template) {
        return;
      }
      ghost.innerHTML = `<span class="skill-icon" style="--skill-icon-tint:${template.iconTint}">${template.iconKey}</span>`;
    } else if (payload.entryType === 'item') {
      const item = this.store.getState().items[payload.itemId];
      if (!item) {
        return;
      }
      const template = getTemplate(item.templateId);
      ghost.innerHTML = `<span class="hotbar-item-icon">${renderInventoryIconArt(template, item)}</span>`;
    } else {
      const action = HOTBAR_ACTIONS[payload.actionId];
      if (!action) {
        return;
      }
      ghost.innerHTML = `<span class="skill-icon" style="--skill-icon-tint:${action.iconTint}">${action.iconKey}</span>`;
    }
    this.root.appendChild(ghost);
    this.dragGhostElement = ghost;
    this.moveSkillDragGhost(clientX, clientY);
  }

  private moveSkillDragGhost(clientX: number, clientY: number): void {
    if (!this.dragGhostElement) {
      return;
    }
    this.dragGhostElement.style.transform = `translate(${clientX - 16}px, ${clientY - 16}px)`;
  }

  private removeSkillDragGhost(): void {
    this.dragGhostElement?.remove();
    this.dragGhostElement = null;
  }

  private getHotbarDropSlotAtPoint(clientX: number, clientY: number): HTMLElement | null {
    const element = document.elementFromPoint(clientX, clientY) as HTMLElement | null;
    return element?.closest<HTMLElement>('[data-hotbar-slot]') ?? null;
  }

  private markHotbarDropSlot(dropSlot: HTMLElement | null): void {
    this.root.querySelectorAll('.drop-ready').forEach((element) => {
      element.classList.remove('drop-ready');
    });
    dropSlot?.classList.add('drop-ready');
  }

  private handleSkillPointerMove(event: PointerEvent): void {
    if (!this.draggedHotbarEntry) {
      return;
    }
    this.moveSkillDragGhost(event.clientX, event.clientY);
    this.markHotbarDropSlot(this.getHotbarDropSlotAtPoint(event.clientX, event.clientY));
  }

  private handleSkillPointerUp(event: PointerEvent): void {
    const payload = this.draggedHotbarEntry;
    const dropSlot = this.getHotbarDropSlotAtPoint(event.clientX, event.clientY);
    if (!payload || !dropSlot) {
      this.clearSkillDragState();
      return;
    }
    const slotIndex = Number(dropSlot.dataset.hotbarSlot);
    if (Number.isInteger(slotIndex) && slotIndex >= 0) {
      this.bindShortcutToHotbar(slotIndex, payload);
    }
    this.clearSkillDragState();
  }

  private handleDragStart(event: DragEvent): void {
    const target = event.target as HTMLElement | null;
    const hotbarShortcut = target?.closest<HTMLElement>('[data-hotbar-shortcut-slot]');
    const skillSlot = target?.closest<HTMLElement>('[data-skill-book-skill]');
    const skillId = skillSlot?.dataset.skillBookSkill;
    const itemSlot = target?.closest<HTMLElement>('[data-inventory-hotbar-item]');
    const itemId = itemSlot?.dataset.inventoryHotbarItem;
    const actionSlot = target?.closest<HTMLElement>('[data-action-hotbar-action]');
    const actionId = actionSlot?.dataset.actionHotbarAction;
    if (!event.dataTransfer) {
      return;
    }

    let payload: HotbarShortcutPayload | null = null;
    let dragSource: HTMLElement | null = null;
    let sourceSlotIndex: number | null = null;
    if (hotbarShortcut) {
      payload = this.getHotbarPayloadFromShortcutElement(hotbarShortcut);
      const slotIndex = Number(hotbarShortcut.dataset.hotbarShortcutSlot);
      if (!payload || !Number.isInteger(slotIndex) || slotIndex < 0) {
        event.preventDefault();
        return;
      }
      sourceSlotIndex = slotIndex;
      dragSource = hotbarShortcut;
    } else if (skillId) {
      const knownSkill = findKnownSkill(this.store.getState().player.learnedSkills, skillId);
      if (!knownSkill || knownSkill.category !== 'active') {
        event.preventDefault();
        return;
      }
      payload = { entryType: 'skill', skillId };
      dragSource = skillSlot ?? null;
    } else if (itemId) {
      const item = this.store.getState().items[itemId];
      if (!item || item.container !== 'inventory') {
        event.preventDefault();
        return;
      }
      payload = { entryType: 'item', itemId };
      dragSource = itemSlot ?? null;
    } else if (actionId && HOTBAR_ACTIONS[actionId as HotbarActionId]) {
      payload = { entryType: 'action', actionId: actionId as HotbarActionId };
      dragSource = actionSlot ?? null;
    }

    if (!payload) {
      return;
    }
    this.beginHudInteractionLock();
    this.draggedHotbarEntry = payload;
    this.draggedHotbarSourceSlotIndex = sourceSlotIndex;
    this.hotbarDropHandled = false;
    this.showHotbarDragGhost(payload, event.clientX, event.clientY);
    event.dataTransfer.effectAllowed = sourceSlotIndex === null ? 'copy' : 'move';
    event.dataTransfer.setData('application/x-hotbar-shortcut', JSON.stringify(payload));
    const transparentDragImage = document.createElement('canvas');
    transparentDragImage.width = 1;
    transparentDragImage.height = 1;
    event.dataTransfer.setDragImage(transparentDragImage, 0, 0);
    dragSource?.classList.add('is-dragging');
  }

  private handleDragOver(event: DragEvent): void {
    const target = event.target as HTMLElement | null;
    const dropSlot = target?.closest<HTMLElement>('[data-hotbar-slot]');
    if (!this.draggedHotbarEntry) {
      return;
    }
    this.moveSkillDragGhost(event.clientX, event.clientY);
    if (!dropSlot) {
      this.markHotbarDropSlot(null);
      return;
    }
    event.preventDefault();
    if (event.dataTransfer) {
      event.dataTransfer.dropEffect = this.draggedHotbarSourceSlotIndex === null ? 'copy' : 'move';
    }
    this.markHotbarDropSlot(dropSlot);
  }

  private handleDrop(event: DragEvent): void {
    const target = event.target as HTMLElement | null;
    const dropSlot = target?.closest<HTMLElement>('[data-hotbar-slot]');
    const payload = this.draggedHotbarEntry;
    if (!dropSlot || !payload) {
      return;
    }
    const slotIndex = Number(dropSlot.dataset.hotbarSlot);
    if (!Number.isInteger(slotIndex) || slotIndex < 0) {
      return;
    }
    event.preventDefault();
    this.hotbarDropHandled = true;
    if (this.draggedHotbarSourceSlotIndex !== null && this.draggedHotbarSourceSlotIndex !== slotIndex) {
      this.hotbarShortcutOverrides.set(this.draggedHotbarSourceSlotIndex, null);
    }
    this.bindShortcutToHotbar(slotIndex, payload);
    this.clearSkillDragState();
  }

  private handleDragEnd(event: DragEvent): void {
    if (this.draggedHotbarSourceSlotIndex !== null && !this.hotbarDropHandled) {
      const dropSlot = this.getHotbarDropSlotAtPoint(event.clientX, event.clientY);
      if (!dropSlot) {
        this.clearShortcutFromHotbar(this.draggedHotbarSourceSlotIndex);
      }
    }
    this.clearSkillDragState();
  }

  private clearSkillDragState(): void {
    this.draggedHotbarEntry = null;
    this.draggedHotbarSourceSlotIndex = null;
    this.hotbarDropHandled = false;
    this.removeSkillDragGhost();
    window.removeEventListener('pointermove', this.handleSkillPointerMoveBound);
    window.removeEventListener('pointerup', this.handleSkillPointerUpBound);
    this.root.querySelectorAll('.is-dragging, .drop-ready').forEach((element) => {
      element.classList.remove('is-dragging', 'drop-ready');
    });
    this.releaseHudInteractionLock();
  }

  private createSnapshot(state: GameState): string {
    const hotbarShortcutOverrides =
      this.hotbarShortcutOverrides instanceof Map ? [...this.hotbarShortcutOverrides.entries()] : [];
    const partyInviteCountdownBucket =
      state.partyInvites.length > 0
        ? state.partyInvites.map((invite) => Math.max(0, Math.ceil((invite.expiresAtMs - Date.now()) / 250))).join(':')
        : 'none';
    const clanInviteCountdownBucket =
      state.clanInvites.length > 0
        ? state.clanInvites.map((invite) => Math.max(0, Math.ceil((invite.expiresAtMs - Date.now()) / 250))).join(':')
        : 'none';
    const pvpFlagCountdownBucket =
      state.player.pvpFlagged && typeof state.player.pvpFlagUntilMs === 'number'
        ? Math.max(0, Math.ceil((state.player.pvpFlagUntilMs - Date.now()) / 250))
        : 'none';
    return JSON.stringify({
      hp: state.player.hp,
      mp: state.player.mp,
      cp: state.player.cp,
      xp: state.player.xp,
      level: state.player.level,
      targetId: state.targetId,
      targetHp: state.targetId ? state.mobs[state.targetId]?.hp ?? 0 : 0,
      cooldowns: state.player.cooldowns,
      skillAvailability: Object.entries(state.player.skillAvailability).map(([skillId, projection]) => ({
        skillId,
        authorityState: projection.authorityState,
        requestBlocked: projection.requestBlocked,
        visualRemainingMs: projection.visualRemainingMs,
      })),
      cast: state.player.cast,
      logs: state.logs.map((entry) => entry.id),
      deadUntilMs: state.player.deadUntilMs,
      pvpFlagged: state.player.pvpFlagged,
      pvpFlagUntilMs: state.player.pvpFlagUntilMs,
      pvpKills: state.player.pvpKills,
      pkCount: state.player.pkCount,
      karma: state.player.karma,
      pvpFlagCountdownBucket,
      inventory: Object.values(state.items).map(
        (item) =>
          `${item.id}:${item.container}:${item.quantity}:${item.equipSlot ?? ''}:${JSON.stringify(item.instanceAttributes ?? null)}`,
      ),
      authoritativeStats: state.player.authoritativeStats,
      learnedSkills: state.player.learnedSkills,
      hotbar: state.player.hotbar,
      pets: state.player.pets,
      hotbarShortcutOverrides,
      visibleHotbarRowCount: this.visibleHotbarRowCount,
      activeCharacterPanel: this.activeCharacterPanel,
      activeChatFilter: this.activeChatFilter,
      chatDraft: this.chatDraft,
      whisperTargetDraft: this.whisperTargetDraft,
      clanNameDraft: this.clanNameDraft,
      chatFocusField: this.chatFocusField,
      skillBookTab: this.skillBookTab,
      inventoryOpen: this.inventoryOpen,
      mapOpen: this.mapOpen,
      systemMenuOpen: this.systemMenuOpen,
      exitConfirmOpen: this.exitConfirmOpen,
      systemPlaceholderOpen: this.systemPlaceholderOpen,
      cameraYaw: Number((Number.isFinite(this.cameraYaw) ? this.cameraYaw : 0).toFixed(4)),
      minimapX: Number(minimapPercentForAxis(state.player.position.x).toFixed(2)),
      minimapZ: Number(minimapPercentForAxis(state.player.position.z).toFixed(2)),
      partyWindowOpen: this.partyWindowOpen,
      clanInfoOpen: this.clanInfoOpen,
      quest: state.quest,
      dialog: state.dialog,
      party: state.party,
      partyInvites: state.partyInvites,
      clan: state.clan,
      clanInvites: state.clanInvites,
      partyInviteCountdownBucket,
      clanInviteCountdownBucket,
      incomingTradeOffer: state.incomingTradeOffer,
      outgoingTradeOffer: state.outgoingTradeOffer,
      region: getRegionIdForPoint(state.player.position),
    });
  }

  private handleSubmit(event: Event): void {
    const target = event.target;
    if (!(target instanceof HTMLFormElement)) {
      return;
    }
    if (target.matches('[data-clan-create-form]')) {
      event.preventDefault();
      const nextName = this.clanNameDraft.trim();
      if (!nextName || !this.onCreateClan) {
        return;
      }
      this.onCreateClan(nextName);
      return;
    }
    if (target.matches('[data-alliance-create-form]')) {
      event.preventDefault();
      const nextName = this.allianceNameDraft.trim();
      if (!nextName || !this.onCreateAlliance) {
        return;
      }
      this.onCreateAlliance(nextName);
      return;
    }
    if (!target.matches('[data-chat-compose]')) {
      return;
    }
    event.preventDefault();
    this.dispatchChatMessage();
  }

  private handleInput(event: Event): void {
    const target = event.target;
    if (!(target instanceof HTMLInputElement)) {
      return;
    }
    if (target.matches('[data-chat-text]')) {
      this.chatDraft = target.value;
      return;
    }
    if (target.matches('[data-chat-target]')) {
      this.whisperTargetDraft = target.value;
      return;
    }
    if (target.matches('[data-clan-name]')) {
      this.clanNameDraft = target.value;
      return;
    }
    if (target.matches('[data-alliance-name]')) {
      this.allianceNameDraft = target.value;
    }
  }

  private handleFocusIn(event: FocusEvent): void {
    const target = event.target;
    if (!(target instanceof HTMLElement)) {
      return;
    }
    if (target.matches('[data-chat-target]')) {
      this.chatFocusField = 'target';
      return;
    }
    if (target.matches('[data-chat-text]')) {
      this.chatFocusField = 'text';
    }
  }

  private dispatchChatMessage(): boolean {
    if (!this.onSendChatMessage) {
      return false;
    }

    const channel = composeChatChannelForFilter(this.activeChatFilter);
    const sent = this.onSendChatMessage(
      channel,
      this.chatDraft,
      channel === 'whisper' ? this.whisperTargetDraft : undefined,
    );
    if (!sent) {
      return false;
    }

    this.chatDraft = '';
    if (channel === 'whisper') {
      this.whisperTargetDraft = '';
    }
    this.chatFocusField = null;
    this.lastSnapshot = '';
    this.update(this.store.getState());
    return true;
  }

  private syncChatComposerFocus(): void {
    if (!this.chatFocusField || !this.onSendChatMessage) {
      return;
    }
    const selector = this.chatFocusField === 'target' ? '[data-chat-target]' : '[data-chat-text]';
    const input = this.root.querySelector<HTMLInputElement>(selector);
    if (!input) {
      return;
    }
    input.focus();
    const valueLength = input.value.length;
    input.setSelectionRange(valueLength, valueLength);
  }

  private handleClick(event: MouseEvent): void {
    const target = event.target as HTMLElement | null;
    if (!target) {
      return;
    }

    const chatFilterButton = target.closest<HTMLElement>('[data-chat-filter]');
    if (chatFilterButton) {
      const filter = chatFilterButton.dataset.chatFilter;
      if (filter === 'all' || filter === 'region' || filter === 'party' || filter === 'alliance' || filter === 'whisper') {
        this.activeChatFilter = filter;
        this.lastSnapshot = '';
        this.update(this.store.getState());
      }
      return;
    }

    const hotbarRowToggle = target.closest<HTMLElement>('[data-hotbar-row-toggle]');
    if (hotbarRowToggle) {
      this.toggleHotbarRows();
      return;
    }

    const quickMenuButton = target.closest<HTMLElement>('[data-quick-menu]');
    if (quickMenuButton) {
      const action = quickMenuButton.dataset.quickMenu;
      if (action === 'status') {
        this.toggleCharacterPanel('status');
      } else if (action === 'inventory') {
        this.toggleInventory();
      } else if (action === 'map') {
        this.toggleMap();
      } else if (action === 'system') {
        this.toggleSystemMenu();
      }
      return;
    }

    const systemPlaceholderButton = target.closest<HTMLElement>('[data-system-placeholder]');
    if (systemPlaceholderButton) {
      const placeholderId = systemPlaceholderButton.dataset.systemPlaceholder;
      if (
        placeholderId === 'community' ||
        placeholderId === 'macro' ||
        placeholderId === 'help' ||
        placeholderId === 'petition' ||
        placeholderId === 'options' ||
        placeholderId === 'restart'
      ) {
        this.openSystemMenuPlaceholder(placeholderId);
      }
      return;
    }

    if (target.closest<HTMLElement>('[data-system-placeholder-close]')) {
      this.systemPlaceholderOpen = null;
      this.lastSnapshot = '';
      this.update(this.store.getState());
      return;
    }

    const characterPanelButton = target.closest<HTMLElement>('[data-character-panel]');
    if (characterPanelButton) {
      const nextPanel = characterPanelButton.dataset.characterPanel;
      if (
        nextPanel === 'status' ||
        nextPanel === 'skills' ||
        nextPanel === 'actions' ||
        nextPanel === 'clan' ||
        nextPanel === 'quests'
      ) {
        this.activeCharacterPanel = nextPanel;
        this.lastSnapshot = '';
        this.update(this.store.getState());
      }
      return;
    }

    const skillBookTab = target.closest<HTMLElement>('[data-skill-book-tab]');
    if (skillBookTab) {
      const nextTab = skillBookTab.dataset.skillBookTab;
      if (nextTab === 'active' || nextTab === 'passive') {
        this.skillBookTab = nextTab;
        this.lastSnapshot = '';
        this.update(this.store.getState());
      }
      return;
    }

    const hotbarShortcut = target.closest<HTMLElement>('[data-hotbar-shortcut-slot]');
    if (event.altKey && hotbarShortcut) {
      const slotIndex = Number(hotbarShortcut.dataset.hotbarShortcutSlot);
      if (Number.isInteger(slotIndex) && slotIndex >= 0) {
        event.preventDefault();
        this.clearShortcutFromHotbar(slotIndex);
      }
      return;
    }

    const skillButton = target.closest<HTMLElement>('[data-skill]');
    if (skillButton) {
      if (shouldBlockSkillDispatch(skillButton)) {
        return;
      }
      if (this.onUseSkill) {
        this.onUseSkill(skillButton.dataset.skill!);
      } else {
        this.store.dispatch({ type: 'useSkill', skillId: skillButton.dataset.skill! });
      }
      return;
    }

    const hotbarItemButton = target.closest<HTMLElement>('[data-hotbar-item]');
    if (hotbarItemButton) {
      const itemId = hotbarItemButton.dataset.hotbarItem;
      const item = itemId ? this.store.getState().items[itemId] : null;
      const template = item ? getTemplate(item.templateId) : null;
      if (!itemId || !item || item.container !== 'inventory' || !template) {
        return;
      }
      if (template.kind === 'consumable') {
        if (this.onUseItem) {
          this.onUseItem(itemId);
        } else if (this.interactive) {
          this.store.dispatch({ type: 'useItem', itemId });
        }
        return;
      }
      if (!template.equipSlot) {
        return;
      }
      if (this.onEquipItem) {
        this.onEquipItem(itemId);
      } else if (this.interactive) {
        this.store.dispatch({ type: 'equipItem', itemId });
      }
      return;
    }

    const useItemButton = target.closest<HTMLElement>('[data-use-item]');
    if (useItemButton) {
      const itemId = useItemButton.dataset.useItem;
      if (!itemId) {
        return;
      }
      if (this.onUseItem) {
        this.onUseItem(itemId);
      } else if (this.interactive) {
        this.store.dispatch({ type: 'useItem', itemId });
      }
      return;
    }

    const hotbarActionButton = target.closest<HTMLElement>('[data-hotbar-action]');
    if (hotbarActionButton) {
      const actionId = hotbarActionButton.dataset.hotbarAction as HotbarActionId | undefined;
      if (!actionId || !HOTBAR_ACTIONS[actionId]) {
        return;
      }
      if (this.onUseHotbarAction) {
        this.onUseHotbarAction(actionId);
      } else if (this.interactive) {
        if (actionId === 'basic_attack') {
          this.store.dispatch({ type: 'basicAttack' });
        } else if (actionId === 'pick_up_nearby') {
          this.store.dispatch({ type: 'pickUpNearbyLoot' });
        }
      }
      return;
    }

    const splitButton = target.closest<HTMLElement>('[data-split-item]');
    if (splitButton) {
      const itemId = splitButton.dataset.splitItem;
      const quantity = Number(splitButton.dataset.splitQuantity ?? '0');
      if (!itemId || !Number.isFinite(quantity)) {
        return;
      }
      if (this.onSplitItemStack) {
        this.onSplitItemStack(itemId, quantity);
      } else if (this.interactive) {
        this.store.dispatch({ type: 'splitItemStack', itemId, quantity });
      }
      return;
    }

    const mergeButton = target.closest<HTMLElement>('[data-merge-source]');
    if (mergeButton) {
      const sourceItemId = mergeButton.dataset.mergeSource;
      const targetItemId = mergeButton.dataset.mergeTarget;
      if (!sourceItemId || !targetItemId) {
        return;
      }
      if (this.onMergeItemStacks) {
        this.onMergeItemStacks(sourceItemId, targetItemId);
      } else if (this.interactive) {
        this.store.dispatch({ type: 'mergeItemStacks', sourceItemId, targetItemId });
      }
      return;
    }

    const vendorButton = target.closest<HTMLElement>('[data-vendor-offer]');
    if (vendorButton) {
      const offerId = vendorButton.dataset.vendorOffer;
      const quantity = Number(vendorButton.dataset.vendorQuantity ?? '0');
      if (!offerId || !Number.isFinite(quantity) || quantity <= 0) {
        return;
      }
      if (this.onBuyVendorOffer) {
        this.onBuyVendorOffer(offerId, quantity);
      } else if (this.interactive) {
        this.store.dispatch({ type: 'buyVendorOffer', offerId, quantity });
      }
      return;
    }

    const exchangeButton = target.closest<HTMLElement>('[data-exchange-offer]');
    if (exchangeButton) {
      const offerId = exchangeButton.dataset.exchangeOffer;
      const quantity = Number(exchangeButton.dataset.exchangeQuantity ?? '0');
      if (!offerId || !Number.isFinite(quantity) || quantity <= 0) {
        return;
      }
      if (this.onExchangeVendorOffer) {
        this.onExchangeVendorOffer(offerId, quantity);
      } else if (this.interactive) {
        this.store.dispatch({ type: 'exchangeVendorOffer', offerId, quantity });
      }
      return;
    }

    const offerTradeButton = target.closest<HTMLElement>('[data-offer-trade-item]');
    if (offerTradeButton) {
      const itemId = offerTradeButton.dataset.offerTradeItem;
      const targetCharacterId = offerTradeButton.dataset.offerTradeTarget;
      const quantity = Number(offerTradeButton.dataset.offerTradeQuantity ?? '0');
      if (!itemId || !targetCharacterId || !Number.isFinite(quantity) || quantity <= 0 || !this.onOfferTradeItem) {
        return;
      }
      this.onOfferTradeItem(targetCharacterId, itemId, quantity);
      return;
    }

    const acceptTradeButton = target.closest<HTMLElement>('[data-accept-trade-offer]');
    if (acceptTradeButton) {
      const offerId = acceptTradeButton.dataset.acceptTradeOffer;
      if (!offerId || !this.onAcceptTradeOffer) {
        return;
      }
      this.onAcceptTradeOffer(offerId);
      return;
    }

    const declineTradeButton = target.closest<HTMLElement>('[data-decline-trade-offer]');
    if (declineTradeButton) {
      const offerId = declineTradeButton.dataset.declineTradeOffer;
      if (!offerId || !this.onDeclineTradeOffer) {
        return;
      }
      this.onDeclineTradeOffer(offerId);
      return;
    }

    const partyAcceptButton = target.closest<HTMLElement>('[data-party-accept]');
    if (partyAcceptButton) {
      const inviteId = partyAcceptButton.dataset.partyAccept;
      if (!inviteId || !this.onAcceptPartyInvite) {
        return;
      }
      this.onAcceptPartyInvite(inviteId);
      return;
    }

    const partyDeclineButton = target.closest<HTMLElement>('[data-party-decline]');
    if (partyDeclineButton) {
      const inviteId = partyDeclineButton.dataset.partyDecline;
      if (!inviteId || !this.onDeclinePartyInvite) {
        return;
      }
      this.onDeclinePartyInvite(inviteId);
      return;
    }

    const partyLeaveButton = target.closest<HTMLElement>('[data-party-leave]');
    if (partyLeaveButton) {
      if (!this.onLeaveParty) {
        return;
      }
      this.onLeaveParty();
      return;
    }

    const partyKickButton = target.closest<HTMLElement>('[data-party-kick]');
    if (partyKickButton) {
      const targetCharacterId = partyKickButton.dataset.partyKick;
      if (!targetCharacterId || !this.onKickPartyMember) {
        return;
      }
      this.onKickPartyMember(targetCharacterId);
      return;
    }

    const clanAcceptButton = target.closest<HTMLElement>('[data-clan-accept]');
    if (clanAcceptButton) {
      const inviteId = clanAcceptButton.dataset.clanAccept;
      if (!inviteId || !this.onAcceptClanInvite) {
        return;
      }
      this.onAcceptClanInvite(inviteId);
      return;
    }

    const clanDeclineButton = target.closest<HTMLElement>('[data-clan-decline]');
    if (clanDeclineButton) {
      const inviteId = clanDeclineButton.dataset.clanDecline;
      if (!inviteId || !this.onDeclineClanInvite) {
        return;
      }
      this.onDeclineClanInvite(inviteId);
      return;
    }

    if (target.closest<HTMLElement>('[data-clan-invite]')) {
      if (!this.onInviteClanMember) {
        return;
      }
      this.onInviteClanMember();
      return;
    }

    if (target.closest<HTMLElement>('[data-clan-info-toggle]')) {
      this.clanInfoOpen = !this.clanInfoOpen;
      this.lastSnapshot = '';
      this.update(this.store.getState());
      return;
    }

    if (target.closest<HTMLElement>('[data-clan-info-close]')) {
      this.clanInfoOpen = false;
      this.lastSnapshot = '';
      this.update(this.store.getState());
      return;
    }

    if (target.closest<HTMLElement>('[data-clan-leave]')) {
      if (!this.onLeaveClan) {
        return;
      }
      this.onLeaveClan();
      return;
    }

    const clanKickButton = target.closest<HTMLElement>('[data-clan-kick]');
    if (clanKickButton) {
      const targetCharacterId = clanKickButton.dataset.clanKick;
      if (!targetCharacterId || !this.onKickClanMember) {
        return;
      }
      this.onKickClanMember(targetCharacterId);
      return;
    }

    if (target.closest<HTMLElement>('[data-clan-dissolve]')) {
      if (!this.onDissolveClan) {
        return;
      }
      this.onDissolveClan();
      return;
    }

    const allianceAcceptButton = target.closest<HTMLElement>('[data-alliance-accept]');
    if (allianceAcceptButton) {
      const inviteId = allianceAcceptButton.dataset.allianceAccept;
      if (!inviteId || !this.onAcceptAllianceInvite) {
        return;
      }
      this.onAcceptAllianceInvite(inviteId);
      return;
    }

    const allianceDeclineButton = target.closest<HTMLElement>('[data-alliance-decline]');
    if (allianceDeclineButton) {
      const inviteId = allianceDeclineButton.dataset.allianceDecline;
      if (!inviteId || !this.onDeclineAllianceInvite) {
        return;
      }
      this.onDeclineAllianceInvite(inviteId);
      return;
    }

    if (target.closest<HTMLElement>('[data-alliance-invite]')) {
      if (!this.onInviteAllianceClan) {
        return;
      }
      this.onInviteAllianceClan();
      return;
    }

    if (target.closest<HTMLElement>('[data-alliance-leave]')) {
      if (!this.onLeaveAlliance) {
        return;
      }
      this.onLeaveAlliance();
      return;
    }

    const allianceExpelButton = target.closest<HTMLElement>('[data-alliance-expel]');
    if (allianceExpelButton) {
      const targetClanId = allianceExpelButton.dataset.allianceExpel;
      if (!targetClanId || !this.onExpelAllianceClan) {
        return;
      }
      this.onExpelAllianceClan(targetClanId);
      return;
    }

    if (target.closest<HTMLElement>('[data-alliance-dissolve]')) {
      if (!this.onDissolveAlliance) {
        return;
      }
      this.onDissolveAlliance();
      return;
    }

    const depositButton = target.closest<HTMLElement>('[data-deposit-item]');
    if (depositButton) {
      const itemId = depositButton.dataset.depositItem;
      const quantity = Number(depositButton.dataset.depositQuantity ?? '0');
      if (!itemId || !Number.isFinite(quantity) || quantity <= 0) {
        return;
      }
      if (this.onDepositWarehouseItem) {
        this.onDepositWarehouseItem(itemId, quantity);
      } else if (this.interactive) {
        this.store.dispatch({ type: 'depositWarehouseItem', itemId, quantity });
      }
      return;
    }

    const sellButton = target.closest<HTMLElement>('[data-sell-item]');
    if (sellButton) {
      const itemId = sellButton.dataset.sellItem;
      const quantity = Number(sellButton.dataset.sellQuantity ?? '0');
      if (!itemId || !Number.isFinite(quantity) || quantity <= 0) {
        return;
      }
      if (this.onSellVendorItem) {
        this.onSellVendorItem(itemId, quantity);
      } else if (this.interactive) {
        this.store.dispatch({ type: 'sellVendorItem', itemId, quantity });
      }
      return;
    }

    const withdrawButton = target.closest<HTMLElement>('[data-withdraw-item]');
    if (withdrawButton) {
      const itemId = withdrawButton.dataset.withdrawItem;
      const quantity = Number(withdrawButton.dataset.withdrawQuantity ?? '0');
      if (!itemId || !Number.isFinite(quantity) || quantity <= 0) {
        return;
      }
      if (this.onWithdrawWarehouseItem) {
        this.onWithdrawWarehouseItem(itemId, quantity);
      } else if (this.interactive) {
        this.store.dispatch({ type: 'withdrawWarehouseItem', itemId, quantity });
      }
      return;
    }

    const itemButton = target.closest<HTMLElement>('[data-item]');
    if (itemButton) {
      if (this.onEquipItem) {
        this.onEquipItem(itemButton.dataset.item!);
      } else if (this.interactive) {
        this.store.dispatch({ type: 'equipItem', itemId: itemButton.dataset.item! });
      }
      return;
    }

    const slotButton = target.closest<HTMLElement>('[data-slot]');
    if (slotButton) {
      if (this.onUnequipItem) {
        this.onUnequipItem(slotButton.dataset.slot as EquipSlot);
      } else if (this.interactive) {
        this.store.dispatch({ type: 'unequipItem', slot: slotButton.dataset.slot as EquipSlot });
      }
      return;
    }

    if (target.closest<HTMLElement>('[data-skill-book-toggle]')) {
      this.toggleSkillBook();
      return;
    }

    if (target.closest<HTMLElement>('[data-character-panel-close]')) {
      this.activeCharacterPanel = null;
      this.lastSnapshot = '';
      this.update(this.store.getState());
      return;
    }

    if (target.closest<HTMLElement>('[data-inventory-close]')) {
      this.inventoryOpen = false;
      this.lastSnapshot = '';
      this.update(this.store.getState());
      return;
    }

    if (target.closest<HTMLElement>('[data-map-close]')) {
      this.mapOpen = false;
      this.lastSnapshot = '';
      this.update(this.store.getState());
      return;
    }

    if (target.closest<HTMLElement>('[data-system-close]')) {
      this.systemMenuOpen = false;
      this.exitConfirmOpen = false;
      this.systemPlaceholderOpen = null;
      this.lastSnapshot = '';
      this.update(this.store.getState());
      return;
    }

    if (target.closest<HTMLElement>('[data-exit-game-open]')) {
      this.exitConfirmOpen = true;
      this.lastSnapshot = '';
      this.update(this.store.getState());
      return;
    }

    if (target.closest<HTMLElement>('[data-exit-game-cancel]')) {
      this.exitConfirmOpen = false;
      this.lastSnapshot = '';
      this.update(this.store.getState());
      return;
    }

    if (target.closest<HTMLElement>('[data-exit-game-ok]')) {
      window.location.reload();
      return;
    }

    if (target.closest<HTMLElement>('[data-party-close]')) {
      this.partyWindowOpen = false;
      this.lastSnapshot = '';
      this.update(this.store.getState());
      return;
    }

    const dialogButton = target.closest<HTMLElement>('[data-dialog-action]');
    if (dialogButton) {
      const npcId = dialogButton.dataset.dialogNpc;
      const actionId = dialogButton.dataset.dialogAction as 'accept_task' | 'turn_in_task' | undefined;
      if (!npcId) {
        return;
      }
      if (this.onInteractNpc) {
        this.onInteractNpc(npcId, actionId);
      } else if (this.interactive) {
        this.store.dispatch({
          type: 'interactNpc',
          npcId,
          actionId,
        });
      }
      return;
    }

    if (target.closest('[data-close-dialog]')) {
      if (this.onCloseDialog) {
        this.onCloseDialog();
      } else if (this.interactive) {
        this.store.dispatch({ type: 'closeDialog' });
      }
      return;
    }

    if (!this.interactive) {
      return;
    }

    if (target.closest('[data-save]')) {
      this.controls.save();
      return;
    }

    if (target.closest('[data-load]')) {
      this.controls.load();
      return;
    }

    if (target.closest('[data-reset]')) {
      this.controls.reset();
    }
  }
}
