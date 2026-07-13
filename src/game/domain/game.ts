import {
  createInitialState,
  gameTemplates,
  getArchetypeIdForBaseClass,
  getLearnedSkillsForCharacter,
  normalizeHotbarState,
} from '../data/templates';
import type {
  DerivedStats,
  EquipSlot,
  ExchangeOfferTemplate,
  GameCommand,
  GameState,
  ItemInstance,
  ItemTemplate,
  LootDrop,
  MobState,
  OtherPlayerState,
  RegionId,
  SkillTemplate,
  Vec2,
  VendorOfferTemplate,
} from './types';

const LOG_LIMIT = 10;
const FLOATING_TEXT_TTL_MS = 1100;
const VENDOR_INTERACTION_RANGE = 4.5;
const WAREHOUSE_INTERACTION_RANGE = 4.5;
const PLAYER_TRADE_INTERACTION_RANGE = 4.5;
const BASIC_ATTACK_RANGE = 2.2;
const BASIC_ATTACK_COOLDOWN_MS = 750;
const LOOT_PICKUP_RANGE = 4.5;
const LOOT_PICKUP_SEARCH_RANGE = 16;
const IDLE_REGEN_DELAY_MS = 5000;
const IDLE_REGEN_TICK_MS = 1000;
const IDLE_REGEN_PERCENT_PER_TICK = 0.03;

const clamp = (value: number, min: number, max: number): number => Math.max(min, Math.min(max, value));

const distance = (a: Vec2, b: Vec2): number => Math.hypot(a.x - b.x, a.z - b.z);

const normalize = (from: Vec2, to: Vec2): Vec2 => {
  const length = distance(from, to) || 1;
  return {
    x: (to.x - from.x) / length,
    z: (to.z - from.z) / length,
  };
};

const moveTowards = (current: Vec2, target: Vec2, step: number): Vec2 => {
  const total = distance(current, target);
  if (total <= step) {
    return { ...target };
  }
  const dir = normalize(current, target);
  return {
    x: current.x + dir.x * step,
    z: current.z + dir.z * step,
  };
};

const formatItemAttributeValue = (value: number): string =>
  Number.isInteger(value) ? String(value) : value.toFixed(1);

const getMob = (state: GameState, mobId: string | null): MobState | null => {
  if (!mobId) {
    return null;
  }
  const mob = state.mobs[mobId];
  if (!mob || mob.aiState === 'dead') {
    return null;
  }
  return mob;
};

const getSkill = (skillId: string): SkillTemplate | null => gameTemplates.skills[skillId] ?? null;

const getItemTemplate = (templateId: string): ItemTemplate => gameTemplates.itemTemplates[templateId];

const getVendorOffer = (offerId: string): VendorOfferTemplate | null => gameTemplates.vendorOffers[offerId] ?? null;
const getExchangeOffer = (offerId: string): ExchangeOfferTemplate | null => gameTemplates.exchangeOffers[offerId] ?? null;

export const getVendorSellValue = (
  templateId: string,
): {
  currencyTemplateId: string;
  amount: number;
} | null => {
  switch (templateId) {
    case 'ironwood_spear':
      return { currencyTemplateId: 'duskgold', amount: 4 };
    case 'wardkeeper_mantle':
      return { currencyTemplateId: 'duskgold', amount: 3 };
    case 'watcher_gloves':
      return { currencyTemplateId: 'duskgold', amount: 2 };
    case 'pathrunner_boots':
      return { currencyTemplateId: 'duskgold', amount: 2 };
    case 'ruinbound_greaves':
      return { currencyTemplateId: 'duskgold', amount: 4 };
    default:
      return null;
  }
};

const syncPlayerProgressionState = (state: GameState): void => {
  const baseClass = state.player.baseClass ?? 'Fighter';
  state.player.baseClass = baseClass;
  state.player.archetypeId = getArchetypeIdForBaseClass(baseClass);
  state.player.learnedSkills = getLearnedSkillsForCharacter(baseClass, state.player.level);
  state.player.hotbar = normalizeHotbarState(state.player.hotbar, baseClass);
};

const normalizeState = (state: GameState): void => {
  state.otherPlayers ??= {};
  state.companions ??= {};
  state.party ??= null;
  state.partyInvites ??= [];
  state.incomingTradeOffer ??= null;
  state.outgoingTradeOffer ??= null;
  state.pendingPath ??= [];
  state.authoritativePath ??= [];
  state.player.cooldowns ??= {};
  state.player.skillAvailability ??= {};
  state.player.queuedSkill ??= null;
  state.player.queuedBasicAttackTargetId ??= null;
  state.player.queuedLootId ??= null;
  if (!Array.isArray(state.player.learnedSkills) || state.player.learnedSkills.length === 0) {
    syncPlayerProgressionState(state);
  } else {
    state.player.baseClass ??= 'Fighter';
    state.player.archetypeId ||= getArchetypeIdForBaseClass(state.player.baseClass);
  }
  state.player.hotbar = normalizeHotbarState(state.player.hotbar, state.player.baseClass ?? 'Fighter');
  state.player.pets ??= [];
  state.player.activePetId ??= null;
  state.player.mountedPetId ??= null;
  state.player.cp ??= getDerivedStats(state).maxCp;
  state.player.stationarySinceMs ??= state.timeMs;
  state.player.lastIdleRegenAtMs ??= state.player.stationarySinceMs;
};

const getKnownSkill = (state: GameState, skillId: string) =>
  state.player.learnedSkills.find((skill) => skill.skillId === skillId) ?? null;

const passiveSkillBonuses = (state: GameState): DerivedStats => {
  const bonuses: DerivedStats = {
    maxCp: 0,
    maxHp: 0,
    maxMp: 0,
    attack: 0,
    defense: 0,
    moveSpeed: 0,
  };

  for (const skill of state.player.learnedSkills) {
    if (skill.category !== 'passive') {
      continue;
    }
    switch (skill.skillId) {
      case 'iron_will':
        bonuses.maxHp += 8;
        bonuses.defense += 3;
        break;
      case 'arcane_focus':
        bonuses.maxMp += 12;
        bonuses.attack += 2;
        break;
      default:
        break;
    }
  }

  return bonuses;
};

const getNextId = (state: GameState, prefix: string): string => {
  const next = `${prefix}_${state.nextId}`;
  state.nextId += 1;
  return next;
};

const pushLog = (state: GameState, text: string, tone: GameState['logs'][number]['tone']): void => {
  state.logs.unshift({
    id: getNextId(state, 'log'),
    text,
    tone,
  });
  state.logs = state.logs.slice(0, LOG_LIMIT);
};

const pushFloatingText = (state: GameState, text: string, color: string, position: Vec2, entityId?: string): void => {
  state.floatingTexts.push({
    id: getNextId(state, 'float'),
    text,
    color,
    position: { ...position },
    entityId,
    ttlMs: FLOATING_TEXT_TTL_MS,
  });
};

const getEquippedItem = (state: GameState, slot: EquipSlot): ItemInstance | null =>
  Object.values(state.items).find((item) => item.container === 'equipment' && item.equipSlot === slot) ?? null;

const hasItemTemplate = (state: GameState, templateId: string): boolean =>
  Object.values(state.items).some((item) => item.templateId === templateId);

const addItemToInventory = (state: GameState, templateId: string, quantity: number): ItemInstance => {
  const template = getItemTemplate(templateId);
  if (template.stackable) {
    const existing = Object.values(state.items).find(
      (item) => item.templateId === templateId && item.container === 'inventory',
    );
    if (existing) {
      existing.quantity += quantity;
      return existing;
    }
  }

  const instance: ItemInstance = {
    id: getNextId(state, 'item'),
    templateId,
    quantity,
    container: 'inventory',
  };
  state.items[instance.id] = instance;
  return instance;
};

const grantInventoryTemplate = (state: GameState, templateId: string, quantity: number): void => {
  const template = getItemTemplate(templateId);
  if (template.stackable) {
    addItemToInventory(state, templateId, quantity);
    return;
  }

  for (let count = 0; count < quantity; count += 1) {
    addItemToInventory(state, templateId, 1);
  }
};

const getInventoryTemplateQuantity = (state: GameState, templateId: string): number =>
  Object.values(state.items)
    .filter((item) => item.container === 'inventory' && item.templateId === templateId)
    .reduce((total, item) => total + item.quantity, 0);

const consumeInventoryTemplateQuantity = (state: GameState, templateId: string, quantity: number): boolean => {
  let remaining = quantity;
  const candidateStacks = Object.values(state.items)
    .filter((item) => item.container === 'inventory' && item.templateId === templateId)
    .sort((left, right) => left.quantity - right.quantity);

  const available = candidateStacks.reduce((total, item) => total + item.quantity, 0);
  if (available < quantity) {
    return false;
  }

  for (const item of candidateStacks) {
    if (remaining <= 0) {
      break;
    }
    if (item.quantity <= remaining) {
      remaining -= item.quantity;
      delete state.items[item.id];
      continue;
    }
    item.quantity -= remaining;
    remaining = 0;
  }

  return true;
};

const transferItemBetweenContainers = (
  state: GameState,
  itemId: string,
  sourceContainer: ItemInstance['container'],
  targetContainer: ItemInstance['container'],
  quantity: number,
): { item: ItemInstance; movedQuantity: number } | null => {
  const item = state.items[itemId];
  if (!item || item.container !== sourceContainer || quantity <= 0) {
    return null;
  }

  const template = getItemTemplate(item.templateId);
  if (!template.stackable) {
    if (quantity !== 1 || item.quantity !== 1) {
      return null;
    }
    item.container = targetContainer;
    item.equipSlot = undefined;
    return { item, movedQuantity: 1 };
  }

  if (quantity > item.quantity) {
    return null;
  }

  const targetStack = Object.values(state.items).find(
    (candidate) =>
      candidate.id !== item.id &&
      candidate.container === targetContainer &&
      candidate.templateId === item.templateId &&
      getItemTemplate(candidate.templateId).stackable,
  );

  if (quantity === item.quantity) {
    if (targetStack) {
      targetStack.quantity += quantity;
      delete state.items[item.id];
      return { item: targetStack, movedQuantity: quantity };
    }
    item.container = targetContainer;
    item.equipSlot = undefined;
    return { item, movedQuantity: quantity };
  }

  item.quantity -= quantity;
  if (targetStack) {
    targetStack.quantity += quantity;
    return { item: targetStack, movedQuantity: quantity };
  }

  const movedItem: ItemInstance = {
    id: getNextId(state, 'item'),
    templateId: item.templateId,
    quantity,
    container: targetContainer,
    instanceAttributes: item.instanceAttributes ? { ...item.instanceAttributes } : undefined,
  };
  state.items[movedItem.id] = movedItem;
  return { item: movedItem, movedQuantity: quantity };
};

const clampPoolsToDerived = (state: GameState): void => {
  const stats = getDerivedStats(state);
  state.player.cp = clamp(state.player.cp ?? stats.maxCp, 0, stats.maxCp);
  state.player.hp = clamp(state.player.hp, 0, stats.maxHp);
  state.player.mp = clamp(state.player.mp, 0, stats.maxMp);
};

const idleRegenAmount = (maxValue: number, ticks: number): number => {
  if (maxValue <= 0 || ticks <= 0) {
    return 0;
  }
  return Math.max(1, Math.ceil(maxValue * IDLE_REGEN_PERCENT_PER_TICK)) * ticks;
};

const resetIdleRegenClock = (state: GameState): void => {
  state.player.stationarySinceMs = state.timeMs;
  state.player.lastIdleRegenAtMs = state.timeMs;
};

const applyIdleRegen = (state: GameState): void => {
  if (state.player.deadUntilMs || state.player.moveTarget) {
    resetIdleRegenClock(state);
    return;
  }

  if (typeof state.player.stationarySinceMs !== 'number') {
    resetIdleRegenClock(state);
  }

  const regenStartMs = state.player.stationarySinceMs + IDLE_REGEN_DELAY_MS;
  const lastRegenAtMs = Math.max(state.player.lastIdleRegenAtMs ?? state.player.stationarySinceMs, regenStartMs);
  const elapsedRegenMs = state.timeMs - lastRegenAtMs;
  const ticks = Math.floor(elapsedRegenMs / IDLE_REGEN_TICK_MS);
  if (ticks <= 0) {
    return;
  }

  const stats = getDerivedStats(state);
  state.player.cp = clamp(state.player.cp + idleRegenAmount(stats.maxCp, ticks), 0, stats.maxCp);
  state.player.hp = clamp(state.player.hp + idleRegenAmount(stats.maxHp, ticks), 0, stats.maxHp);
  state.player.mp = clamp(state.player.mp + idleRegenAmount(stats.maxMp, ticks), 0, stats.maxMp);
  state.player.lastIdleRegenAtMs = lastRegenAtMs + ticks * IDLE_REGEN_TICK_MS;
};

const getXpThreshold = (level: number): number => 70 + (level - 1) * 50;

const awardXp = (state: GameState, amount: number): void => {
  const previousKnownSkillIds = new Set(state.player.learnedSkills.map((skill) => skill.skillId));
  state.player.xp += amount;
  let leveled = false;
  while (state.player.xp >= getXpThreshold(state.player.level) && state.player.level < 5) {
    state.player.xp -= getXpThreshold(state.player.level);
    state.player.level += 1;
    leveled = true;
  }
  if (leveled) {
    syncPlayerProgressionState(state);
    const stats = getDerivedStats(state);
    state.player.cp = stats.maxCp;
    state.player.hp = stats.maxHp;
    state.player.mp = stats.maxMp;
    pushLog(state, `You reach level ${state.player.level}. Vitality surges back to full.`, 'success');
    for (const skill of state.player.learnedSkills) {
      if (previousKnownSkillIds.has(skill.skillId)) {
        continue;
      }
      const template = getSkill(skill.skillId);
      if (template) {
        pushLog(state, `You learn ${template.name}.`, 'success');
      }
    }
  }
};

const enterRespawnState = (state: GameState): void => {
  const stats = getDerivedStats(state);
  state.player.deadUntilMs = state.timeMs + 2500;
  state.player.moveTarget = null;
  state.player.cast = null;
  state.player.queuedSkill = null;
  state.player.queuedBasicAttackTargetId = null;
  state.player.queuedLootId = null;
  state.destinationMarker = null;
  state.targetId = null;
  state.player.hp = 0;
  state.player.cp = clamp(state.player.cp, 0, stats.maxCp);
  state.player.mp = clamp(state.player.mp, 0, stats.maxMp);
  resetIdleRegenClock(state);
  pushLog(state, 'You fall in battle and retreat toward Dawn Plaza.', 'danger');
};

const damagePlayer = (state: GameState, sourceName: string, amount: number): void => {
  if (state.player.deadUntilMs) {
    return;
  }
  state.player.hp = clamp(state.player.hp - amount, 0, getDerivedStats(state).maxHp);
  pushFloatingText(state, `-${amount}`, '#ff8d7b', state.player.position, state.player.id);
  if (state.player.hp <= 0) {
    enterRespawnState(state);
  } else {
    pushLog(state, `${sourceName} hits you for ${amount}.`, 'danger');
  }
};

const addLootEntity = (state: GameState, templateId: string, quantity: number, position: Vec2): void => {
  const instance: ItemInstance = {
    id: getNextId(state, 'item'),
    templateId,
    quantity,
    container: 'ground',
  };
  state.items[instance.id] = instance;
  const dropId = getNextId(state, 'loot');
  state.loot[dropId] = {
    id: dropId,
    itemInstanceId: instance.id,
    position: { ...position },
    label: getItemTemplate(templateId).name,
  };
};

const handleQuestKill = (state: GameState, mob: MobState): void => {
  if (state.quest.status !== 'active') {
    return;
  }
  if (mob.templateId !== 'mireling') {
    return;
  }
  state.quest.progress += 1;
  if (state.quest.progress >= state.quest.goal) {
    state.quest.status = 'ready_to_turn_in';
    pushLog(state, 'The wardkeeper asked for 3 Mirelings. Return to Selka for your mantle.', 'success');
    return;
  }
  pushLog(
    state,
    `Keeper of the Gate progress: ${state.quest.progress}/${state.quest.goal} Mirelings defeated.`,
    'neutral',
  );
};

const handleMobDeath = (state: GameState, mob: MobState): void => {
  const mobTemplate = gameTemplates.mobTemplates[mob.templateId];
  mob.aiState = 'dead';
  mob.hp = 0;
  mob.attackCooldownMs = 0;
  mob.respawnAtMs = state.timeMs + 12000;

  addLootEntity(state, 'duskgold', mobTemplate.currencyDrop, mob.position);

  if (
    mobTemplate.guaranteedEquipmentTemplateId &&
    !state.equipmentAwardsGranted.includes(mobTemplate.guaranteedEquipmentTemplateId) &&
    !hasItemTemplate(state, mobTemplate.guaranteedEquipmentTemplateId)
  ) {
    addLootEntity(state, mobTemplate.guaranteedEquipmentTemplateId, 1, { x: mob.position.x + 1.2, z: mob.position.z });
    state.equipmentAwardsGranted.push(mobTemplate.guaranteedEquipmentTemplateId);
  }

  awardXp(state, mobTemplate.xpReward);
  handleQuestKill(state, mob);
  pushLog(state, `${mobTemplate.name} falls and drops loot.`, 'success');
  pushFloatingText(state, 'Down', '#ffdc7c', mob.position, mob.id);
};

const dealDamageToMob = (state: GameState, mob: MobState, damage: number): void => {
  mob.hp = clamp(mob.hp - damage, 0, gameTemplates.mobTemplates[mob.templateId].maxHp);
  mob.aiState = 'aggro';
  pushFloatingText(state, `${damage}`, '#ffd98a', mob.position, mob.id);
  if (mob.hp <= 0) {
    handleMobDeath(state, mob);
  }
};

const collectAoeTargets = (state: GameState, center: Vec2, radius: number, maxTargets: number): MobState[] =>
  Object.values(state.mobs)
    .filter((mob) => mob.aiState !== 'dead' && distance(center, mob.position) <= radius)
    .sort((left, right) => distance(center, left.position) - distance(center, right.position))
    .slice(0, maxTargets);

const resolveSkill = (state: GameState, skill: SkillTemplate, targetId: string): void => {
  if (skill.category !== 'active' || skill.targetType === 'passive') {
    return;
  }
  const target = getMob(state, targetId);
  if (!target) {
    pushLog(state, `${skill.name} fizzles because the target is gone.`, 'warning');
    return;
  }

  if (distance(state.player.position, target.position) > skill.range) {
    pushLog(state, `${skill.name} fails because the target is out of range.`, 'warning');
    return;
  }

  const playerStats = getDerivedStats(state);
  if (skill.targetType === 'single_target_enemy') {
    const damage = Math.max(8, Math.round(playerStats.attack + skill.power - gameTemplates.mobTemplates[target.templateId].defense * 0.4));
    dealDamageToMob(state, target, damage);
    pushLog(state, `${skill.name} lands on ${gameTemplates.mobTemplates[target.templateId].name}.`, 'success');
    return;
  }

  const targets = collectAoeTargets(state, target.position, skill.radius ?? 0, skill.maxTargets);
  if (targets.length === 0) {
    pushLog(state, `${skill.name} finds no valid enemies around the target.`, 'warning');
    return;
  }

  const damageBudget = Math.max(18, playerStats.attack + skill.power);
  const splitBase = Math.round(damageBudget / targets.length);
  for (const mob of targets) {
    const damage = Math.max(7, splitBase - Math.round(gameTemplates.mobTemplates[mob.templateId].defense * 0.4));
    dealDamageToMob(state, mob, damage);
  }
  pushLog(state, `${skill.name} blooms around the target and hits ${targets.length} enemies.`, 'success');
};

const openNpcDialog = (state: GameState): void => {
  if (state.quest.status === 'available') {
    state.dialog = {
      npcId: 'npc_wardkeeper',
      title: 'Selka, Wardkeeper of the Plaza',
      body: 'The gate road is drawing Mirelings again. Clear three of them and I will fit you with a proper mantle.',
      actionLabel: 'Accept Task',
      actionId: 'accept_task',
    };
    return;
  }

  if (state.quest.status === 'active') {
    state.dialog = {
      npcId: 'npc_wardkeeper',
      title: 'Selka, Wardkeeper of the Plaza',
      body: `You have driven back ${state.quest.progress}/${state.quest.goal} Mirelings. Finish the cull beyond the gate.`,
    };
    return;
  }

  if (state.quest.status === 'ready_to_turn_in') {
    state.dialog = {
      npcId: 'npc_wardkeeper',
      title: 'Selka, Wardkeeper of the Plaza',
      body: 'The road is clear enough for now. Take this mantle and keep your footing in the field.',
      actionLabel: 'Receive Reward',
      actionId: 'turn_in_task',
    };
    return;
  }

  state.dialog = {
    npcId: 'npc_wardkeeper',
    title: 'Selka, Wardkeeper of the Plaza',
    body: 'You have done enough for one watch. Deeper ruins still hide stronger prey if you want a better weapon.',
  };
};

const interactWithNpc = (state: GameState, actionId?: 'accept_task' | 'turn_in_task'): void => {
  if (actionId === 'accept_task' && state.quest.status === 'available') {
    state.quest.status = 'active';
    state.dialog = null;
    pushLog(state, 'Selka marks the Mireling cull on your tracker.', 'success');
    return;
  }

  if (actionId === 'turn_in_task' && state.quest.status === 'ready_to_turn_in') {
    state.quest.status = 'completed';
    addItemToInventory(state, 'wardkeeper_mantle', 1);
    state.dialog = null;
    pushLog(state, 'Selka rewards you with the Wardkeeper Mantle.', 'success');
    return;
  }

  openNpcDialog(state);
};

const applyMoveToPoint = (state: GameState, point: Vec2): void => {
  if (state.player.deadUntilMs) {
    return;
  }
  if (state.player.cast) {
    state.player.cast = null;
    pushLog(state, 'You break your cast and move.', 'warning');
  }
  state.player.queuedSkill = null;
  state.player.queuedBasicAttackTargetId = null;
  state.player.queuedLootId = null;
  state.dialog = null;
  state.player.moveTarget = { ...point };
  state.destinationMarker = { ...point };
};

const applySelectTarget = (state: GameState, targetId: string | null): void => {
  const mob = getMob(state, targetId);
  state.targetId = mob ? mob.id : null;
  if (mob) {
    if (state.player.queuedSkill && state.player.queuedSkill.targetId !== mob.id) {
      state.player.queuedSkill = null;
    }
    if (state.player.queuedBasicAttackTargetId && state.player.queuedBasicAttackTargetId !== mob.id) {
      state.player.queuedBasicAttackTargetId = null;
    }
    pushLog(state, `${gameTemplates.mobTemplates[mob.templateId].name} is now your target.`, 'neutral');
  } else {
    state.player.queuedSkill = null;
    state.player.queuedBasicAttackTargetId = null;
  }
};

const beginSkillCast = (state: GameState, skill: SkillTemplate, target: MobState): boolean => {
  if ((state.player.cooldowns[skill.id] ?? 0) > 0) {
    pushLog(state, `${skill.name} is still recovering.`, 'warning');
    return false;
  }
  if (state.player.mp < skill.mpCost) {
    pushLog(state, `Not enough MP for ${skill.name}.`, 'warning');
    return false;
  }

  state.player.mp -= skill.mpCost;
  state.player.cooldowns[skill.id] = skill.cooldownMs;
  state.player.cast = {
    skillId: skill.id,
    targetId: target.id,
    remainingMs: skill.castTimeMs,
    totalMs: skill.castTimeMs,
  };
  state.player.queuedSkill = null;
  state.player.queuedBasicAttackTargetId = null;
  state.player.queuedLootId = null;
  state.player.moveTarget = null;
  state.destinationMarker = null;
  state.player.facing = Math.atan2(target.position.z - state.player.position.z, target.position.x - state.player.position.x);
  pushLog(state, `Casting ${skill.name}...`, 'neutral');
  return true;
};

const queueSkillApproach = (state: GameState, skill: SkillTemplate, target: MobState): void => {
  state.player.queuedSkill = {
    skillId: skill.id,
    targetId: target.id,
  };
  state.player.queuedBasicAttackTargetId = null;
  state.player.queuedLootId = null;
  state.player.moveTarget = { ...target.position };
  state.destinationMarker = { ...target.position };
  state.player.facing = Math.atan2(target.position.z - state.player.position.z, target.position.x - state.player.position.x);
  pushLog(state, `Moving into range for ${skill.name}.`, 'neutral');
};

const applyUseSkill = (state: GameState, skillId: string): void => {
  if (state.player.deadUntilMs) {
    return;
  }
  const skill = getSkill(skillId);
  if (!skill) {
    return;
  }
  const knownSkill = getKnownSkill(state, skillId);
  if (!knownSkill) {
    pushLog(state, `${skill.name} is not learned yet.`, 'warning');
    return;
  }
  if (knownSkill.category !== 'active' || skill.category !== 'active' || skill.targetType === 'passive') {
    pushLog(state, `${skill.name} is passive and cannot be activated.`, 'warning');
    return;
  }
  if (state.player.cast) {
    pushLog(state, 'You are already channeling a skill.', 'warning');
    return;
  }
  if ((state.player.cooldowns[skillId] ?? 0) > 0) {
    pushLog(state, `${skill.name} is still recovering.`, 'warning');
    return;
  }
  const target = getMob(state, state.targetId);
  if (!target) {
    pushLog(state, `${skill.name} requires a valid target.`, 'warning');
    return;
  }
  if (state.player.mp < skill.mpCost) {
    pushLog(state, `Not enough MP for ${skill.name}.`, 'warning');
    return;
  }
  if (distance(state.player.position, target.position) > skill.range) {
    queueSkillApproach(state, skill, target);
    return;
  }

  beginSkillCast(state, skill, target);
};

const updateQueuedSkillApproach = (state: GameState): void => {
  const queuedSkill = state.player.queuedSkill;
  if (!queuedSkill || state.player.deadUntilMs || state.player.cast) {
    return;
  }

  const skill = getSkill(queuedSkill.skillId);
  const target = getMob(state, queuedSkill.targetId);
  if (!skill || !target) {
    state.player.queuedSkill = null;
    state.player.moveTarget = null;
    state.destinationMarker = null;
    pushLog(state, `${skill?.name ?? 'Skill'} approach canceled because the target is gone.`, 'warning');
    return;
  }

  if ((state.player.cooldowns[skill.id] ?? 0) > 0) {
    state.player.queuedSkill = null;
    state.player.moveTarget = null;
    state.destinationMarker = null;
    pushLog(state, `${skill.name} is still recovering.`, 'warning');
    return;
  }

  if (state.player.mp < skill.mpCost) {
    state.player.queuedSkill = null;
    state.player.moveTarget = null;
    state.destinationMarker = null;
    pushLog(state, `Not enough MP for ${skill.name}.`, 'warning');
    return;
  }

  if (distance(state.player.position, target.position) <= skill.range) {
    beginSkillCast(state, skill, target);
    return;
  }

  state.player.moveTarget = { ...target.position };
  state.destinationMarker = { ...target.position };
  state.player.facing = Math.atan2(target.position.z - state.player.position.z, target.position.x - state.player.position.x);
};

const executeBasicAttack = (state: GameState, target: MobState): void => {
  const cooldownRemaining = state.player.cooldowns.basic_attack ?? 0;
  if (cooldownRemaining > 0) {
    pushLog(state, 'Basic attack is still recovering.', 'warning');
    return;
  }

  const playerStats = getDerivedStats(state);
  const mobTemplate = gameTemplates.mobTemplates[target.templateId];
  const damage = Math.max(3, Math.round(playerStats.attack - mobTemplate.defense * 0.35));
  state.player.cooldowns.basic_attack = BASIC_ATTACK_COOLDOWN_MS;
  state.player.queuedBasicAttackTargetId = null;
  state.player.queuedSkill = null;
  state.player.queuedLootId = null;
  state.player.moveTarget = null;
  state.destinationMarker = null;
  state.player.facing = Math.atan2(target.position.z - state.player.position.z, target.position.x - state.player.position.x);
  dealDamageToMob(state, target, damage);
  pushLog(state, `You strike ${mobTemplate.name} with a basic attack.`, 'success');
};

const queueBasicAttackApproach = (state: GameState, target: MobState): void => {
  state.player.queuedBasicAttackTargetId = target.id;
  state.player.queuedSkill = null;
  state.player.queuedLootId = null;
  state.player.moveTarget = { ...target.position };
  state.destinationMarker = { ...target.position };
  state.player.facing = Math.atan2(target.position.z - state.player.position.z, target.position.x - state.player.position.x);
  pushLog(state, `Moving into range for a basic attack.`, 'neutral');
};

const applyBasicAttack = (state: GameState): void => {
  if (state.player.deadUntilMs) {
    return;
  }
  if (state.player.cast) {
    pushLog(state, 'You are already channeling a skill.', 'warning');
    return;
  }
  const target = getMob(state, state.targetId);
  if (!target) {
    pushLog(state, 'Basic attack requires a valid target.', 'warning');
    return;
  }
  if ((state.player.cooldowns.basic_attack ?? 0) > 0) {
    pushLog(state, 'Basic attack is still recovering.', 'warning');
    return;
  }
  if (distance(state.player.position, target.position) > BASIC_ATTACK_RANGE) {
    queueBasicAttackApproach(state, target);
    return;
  }
  executeBasicAttack(state, target);
};

const updateQueuedBasicAttackApproach = (state: GameState): void => {
  const targetId = state.player.queuedBasicAttackTargetId;
  if (!targetId || state.player.deadUntilMs || state.player.cast) {
    return;
  }

  const target = getMob(state, targetId);
  if (!target) {
    state.player.queuedBasicAttackTargetId = null;
    state.player.moveTarget = null;
    state.destinationMarker = null;
    pushLog(state, 'Basic attack approach canceled because the target is gone.', 'warning');
    return;
  }

  if ((state.player.cooldowns.basic_attack ?? 0) > 0) {
    state.player.queuedBasicAttackTargetId = null;
    state.player.moveTarget = null;
    state.destinationMarker = null;
    pushLog(state, 'Basic attack is still recovering.', 'warning');
    return;
  }

  if (distance(state.player.position, target.position) <= BASIC_ATTACK_RANGE) {
    executeBasicAttack(state, target);
    return;
  }

  state.player.moveTarget = { ...target.position };
  state.destinationMarker = { ...target.position };
  state.player.facing = Math.atan2(target.position.z - state.player.position.z, target.position.x - state.player.position.x);
};

const collectLoot = (state: GameState, lootId: string): boolean => {
  const loot = state.loot[lootId];
  if (!loot) {
    return false;
  }

  const instance = state.items[loot.itemInstanceId];
  if (!instance) {
    delete state.loot[lootId];
    return false;
  }

  const template = getItemTemplate(instance.templateId);
  const pickedQuantity = instance.quantity;
  if (template.stackable) {
    const targetStack = Object.values(state.items).find(
      (item) =>
        item.id !== instance.id &&
        item.container === 'inventory' &&
        item.templateId === instance.templateId &&
        getItemTemplate(item.templateId).stackable,
    );
    if (targetStack) {
      targetStack.quantity += instance.quantity;
      delete state.items[instance.id];
    } else {
      instance.container = 'inventory';
    }
  } else {
    instance.container = 'inventory';
  }

  delete state.loot[lootId];
  state.player.queuedLootId = null;
  state.player.moveTarget = null;
  state.destinationMarker = null;
  pushLog(state, `Picked up ${template.name}${pickedQuantity > 1 ? ` x${pickedQuantity}` : ''}.`, 'success');
  return true;
};

const queueLootPickup = (state: GameState, loot: LootDrop): void => {
  state.player.queuedLootId = loot.id;
  state.player.queuedSkill = null;
  state.player.queuedBasicAttackTargetId = null;
  state.player.moveTarget = { ...loot.position };
  state.destinationMarker = { ...loot.position };
  state.player.facing = Math.atan2(loot.position.z - state.player.position.z, loot.position.x - state.player.position.x);
  pushLog(state, `Moving to pick up ${loot.label}.`, 'neutral');
};

const updateQueuedLootPickup = (state: GameState): void => {
  const lootId = state.player.queuedLootId;
  if (!lootId || state.player.deadUntilMs || state.player.cast) {
    return;
  }

  const loot = state.loot[lootId];
  if (!loot) {
    state.player.queuedLootId = null;
    state.player.moveTarget = null;
    state.destinationMarker = null;
    return;
  }

  if (distance(state.player.position, loot.position) <= LOOT_PICKUP_RANGE) {
    collectLoot(state, lootId);
    return;
  }

  state.player.moveTarget = { ...loot.position };
  state.destinationMarker = { ...loot.position };
  state.player.facing = Math.atan2(loot.position.z - state.player.position.z, loot.position.x - state.player.position.x);
};

const applyPickUpLoot = (state: GameState, lootId: string): void => {
  if (state.player.deadUntilMs) {
    return;
  }
  const loot = state.loot[lootId];
  if (!loot) {
    return;
  }
  if (distance(state.player.position, loot.position) > LOOT_PICKUP_RANGE) {
    queueLootPickup(state, loot);
    return;
  }

  collectLoot(state, lootId);
};

const applyPickUpNearbyLoot = (state: GameState): void => {
  if (state.player.deadUntilMs) {
    return;
  }

  const nearestLoot = Object.values(state.loot)
    .map((loot) => ({
      loot,
      distance: distance(state.player.position, loot.position),
    }))
    .filter((entry) => entry.distance <= LOOT_PICKUP_SEARCH_RANGE)
    .sort((left, right) => left.distance - right.distance)[0];
  if (!nearestLoot) {
    pushLog(state, 'No nearby loot to pick up.', 'warning');
    return;
  }

  applyPickUpLoot(state, nearestLoot.loot.id);
};

const applyUseItem = (state: GameState, itemId: string): void => {
  if (state.player.deadUntilMs) {
    return;
  }

  const item = state.items[itemId];
  if (!item || item.container !== 'inventory') {
    pushLog(state, 'Consumable use failed: item is no longer in the inventory.', 'warning');
    return;
  }

  const template = getItemTemplate(item.templateId);
  if (template.kind !== 'consumable') {
    pushLog(state, `${template.name} cannot be used as a consumable.`, 'warning');
    return;
  }

  switch (template.id) {
    case 'healing_potion': {
      const stats = getDerivedStats(state);
      if (state.player.hp >= stats.maxHp) {
        pushLog(state, 'Healing Potion would have no effect right now.', 'warning');
        return;
      }
      const recovered = Math.min(45, stats.maxHp - state.player.hp);
      state.player.hp += recovered;
      if (item.quantity <= 1) {
        delete state.items[item.id];
      } else {
        item.quantity -= 1;
      }
      pushLog(state, `Used Healing Potion and recovered ${recovered} HP.`, 'success');
      return;
    }
    default:
      pushLog(state, `${template.name} cannot be used as a consumable.`, 'warning');
      return;
  }
};

const applyEquipItem = (state: GameState, itemId: string): void => {
  const item = state.items[itemId];
  if (!item || item.container !== 'inventory') {
    return;
  }
  const template = getItemTemplate(item.templateId);
  if (!template.equipSlot) {
    pushLog(state, `${template.name} cannot be equipped.`, 'warning');
    return;
  }
  const current = getEquippedItem(state, template.equipSlot);
  if (current) {
    current.container = 'inventory';
    current.equipSlot = undefined;
  }
  item.container = 'equipment';
  item.equipSlot = template.equipSlot;
  clampPoolsToDerived(state);
  pushLog(state, `Equipped ${template.name}.`, 'success');
};

const applyUnequipItem = (state: GameState, slot: EquipSlot): void => {
  const current = getEquippedItem(state, slot);
  if (!current) {
    return;
  }
  current.container = 'inventory';
  current.equipSlot = undefined;
  clampPoolsToDerived(state);
  pushLog(state, `Unequipped ${getItemTemplate(current.templateId).name}.`, 'neutral');
};

const applySplitItemStack = (state: GameState, itemId: string, quantity: number): void => {
  const item = state.items[itemId];
  if (!item || item.container !== 'inventory') {
    pushLog(state, 'Stack split failed: item is no longer in the inventory.', 'warning');
    return;
  }
  const template = getItemTemplate(item.templateId);
  if (!template.stackable) {
    pushLog(state, `${template.name} cannot be split.`, 'warning');
    return;
  }
  if (quantity <= 0 || item.quantity <= quantity) {
    pushLog(state, `Split amount is invalid for ${template.name}.`, 'warning');
    return;
  }

  item.quantity -= quantity;
  const splitItem: ItemInstance = {
    id: getNextId(state, 'item'),
    templateId: item.templateId,
    quantity,
    container: 'inventory',
    instanceAttributes: item.instanceAttributes ? { ...item.instanceAttributes } : undefined,
  };
  state.items[splitItem.id] = splitItem;
  pushLog(state, `Split ${template.name} into ${item.quantity} and ${quantity}.`, 'success');
};

const applyMergeItemStacks = (state: GameState, sourceItemId: string, targetItemId: string): void => {
  if (sourceItemId === targetItemId) {
    pushLog(state, 'Stack merge failed: source and target are identical.', 'warning');
    return;
  }
  const source = state.items[sourceItemId];
  const target = state.items[targetItemId];
  if (!source || !target || source.container !== 'inventory' || target.container !== 'inventory') {
    pushLog(state, 'Stack merge failed: one of the stacks is no longer in the inventory.', 'warning');
    return;
  }
  if (source.templateId !== target.templateId) {
    pushLog(state, 'Stack merge failed: items do not share the same template.', 'warning');
    return;
  }
  const template = getItemTemplate(source.templateId);
  if (!template.stackable) {
    pushLog(state, `${template.name} cannot be merged as a stack.`, 'warning');
    return;
  }

  target.quantity += source.quantity;
  delete state.items[sourceItemId];
  pushLog(state, `Merged ${template.name} stacks into ${target.quantity}.`, 'success');
};

const applyBuyVendorOffer = (state: GameState, offerId: string, quantity: number): void => {
  const offer = getVendorOffer(offerId);
  if (!offer || quantity <= 0) {
    pushLog(state, 'Vendor purchase failed: offer is not available.', 'warning');
    return;
  }

  const vendor = state.npcs[offer.npcId];
  if (!vendor || distance(state.player.position, vendor.position) > VENDOR_INTERACTION_RANGE) {
    pushLog(state, `${vendor?.name ?? 'Vendor'} is too far away to trade.`, 'warning');
    return;
  }

  const totalCost = offer.priceAmount * quantity;
  const currencyLabel = getItemTemplate(offer.priceCurrencyTemplateId).name;
  const availableCurrency = getInventoryTemplateQuantity(state, offer.priceCurrencyTemplateId);
  if (availableCurrency < totalCost) {
    pushLog(state, `Vendor purchase failed: ${currencyLabel} is insufficient.`, 'warning');
    return;
  }

  if (!consumeInventoryTemplateQuantity(state, offer.priceCurrencyTemplateId, totalCost)) {
    pushLog(state, `Vendor purchase failed: ${currencyLabel} is insufficient.`, 'warning');
    return;
  }

  grantInventoryTemplate(state, offer.templateId, offer.quantity * quantity);
  pushLog(
    state,
    `Bought ${getItemTemplate(offer.templateId).name} for ${totalCost} ${currencyLabel}.`,
    'success',
  );
};

const applyExchangeVendorOffer = (state: GameState, offerId: string, quantity: number): void => {
  const offer = getExchangeOffer(offerId);
  if (!offer || quantity <= 0) {
    pushLog(state, 'Exchange failed: offer is not available.', 'warning');
    return;
  }

  const vendor = state.npcs[offer.npcId];
  if (!vendor || distance(state.player.position, vendor.position) > VENDOR_INTERACTION_RANGE) {
    pushLog(state, `${vendor?.name ?? 'Vendor'} is too far away to exchange items.`, 'warning');
    return;
  }

  const totalCost = offer.costAmount * quantity;
  const costLabel = getItemTemplate(offer.costTemplateId).name;
  const availableCost = getInventoryTemplateQuantity(state, offer.costTemplateId);
  if (availableCost < totalCost) {
    pushLog(state, `Exchange failed: ${costLabel} is insufficient.`, 'warning');
    return;
  }

  if (!consumeInventoryTemplateQuantity(state, offer.costTemplateId, totalCost)) {
    pushLog(state, `Exchange failed: ${costLabel} is insufficient.`, 'warning');
    return;
  }

  grantInventoryTemplate(state, offer.templateId, offer.quantity * quantity);
  pushLog(
    state,
    `Exchanged ${totalCost} ${costLabel} for ${getItemTemplate(offer.templateId).name}.`,
    'success',
  );
};

const applySellVendorItem = (state: GameState, itemId: string, quantity: number): void => {
  const vendor = state.npcs.npc_merchant;
  if (!vendor || distance(state.player.position, vendor.position) > VENDOR_INTERACTION_RANGE) {
    pushLog(state, `${vendor?.name ?? 'Vendor'} is too far away to trade.`, 'warning');
    return;
  }

  const item = state.items[itemId];
  if (!item || item.container !== 'inventory') {
    pushLog(state, 'Vendor sale failed: item is no longer in the inventory.', 'warning');
    return;
  }

  const template = getItemTemplate(item.templateId);
  const sellValue = getVendorSellValue(item.templateId);
  if (!sellValue) {
    pushLog(state, `Vendor sale failed: ${template.name} cannot be sold here.`, 'warning');
    return;
  }
  if (quantity <= 0) {
    pushLog(state, `Vendor sale failed: quantity is invalid for ${template.name}.`, 'warning');
    return;
  }
  if (!template.stackable) {
    if (quantity !== 1 || item.quantity !== 1) {
      pushLog(state, `Vendor sale failed: quantity is invalid for ${template.name}.`, 'warning');
      return;
    }
    delete state.items[item.id];
  } else {
    if (quantity > item.quantity) {
      pushLog(state, `Vendor sale failed: quantity is invalid for ${template.name}.`, 'warning');
      return;
    }
    if (quantity === item.quantity) {
      delete state.items[item.id];
    } else {
      item.quantity -= quantity;
    }
  }

  const totalValue = sellValue.amount * quantity;
  grantInventoryTemplate(state, sellValue.currencyTemplateId, totalValue);
  pushLog(
    state,
    `Sold ${template.name}${template.stackable ? ` x${quantity}` : ''} for ${totalValue} ${getItemTemplate(sellValue.currencyTemplateId).name}.`,
    'success',
  );
};

const applyDepositWarehouseItem = (state: GameState, itemId: string, quantity: number): void => {
  const warehouseKeeper = state.npcs.npc_warehouse_keeper;
  if (!warehouseKeeper || distance(state.player.position, warehouseKeeper.position) > WAREHOUSE_INTERACTION_RANGE) {
    pushLog(state, `${warehouseKeeper?.name ?? 'Warehouse Keeper'} is too far away to store items.`, 'warning');
    return;
  }

  const item = state.items[itemId];
  if (!item || item.container !== 'inventory') {
    pushLog(state, 'Warehouse deposit failed: item is no longer in the inventory.', 'warning');
    return;
  }

  const transfer = transferItemBetweenContainers(state, itemId, 'inventory', 'warehouse', quantity);
  if (!transfer) {
    pushLog(state, 'Warehouse deposit failed: quantity is invalid for that item.', 'warning');
    return;
  }

  const template = getItemTemplate(item.templateId);
  pushLog(
    state,
    `Stored ${template.name}${template.stackable ? ` x${transfer.movedQuantity}` : ''} in the warehouse.`,
    'success',
  );
};

const applyWithdrawWarehouseItem = (state: GameState, itemId: string, quantity: number): void => {
  const warehouseKeeper = state.npcs.npc_warehouse_keeper;
  if (!warehouseKeeper || distance(state.player.position, warehouseKeeper.position) > WAREHOUSE_INTERACTION_RANGE) {
    pushLog(state, `${warehouseKeeper?.name ?? 'Warehouse Keeper'} is too far away to release storage.`, 'warning');
    return;
  }

  const item = state.items[itemId];
  if (!item || item.container !== 'warehouse') {
    pushLog(state, 'Warehouse withdraw failed: item is no longer stored.', 'warning');
    return;
  }

  const transfer = transferItemBetweenContainers(state, itemId, 'warehouse', 'inventory', quantity);
  if (!transfer) {
    pushLog(state, 'Warehouse withdraw failed: quantity is invalid for that item.', 'warning');
    return;
  }

  const template = getItemTemplate(item.templateId);
  pushLog(
    state,
    `Withdrew ${template.name}${template.stackable ? ` x${transfer.movedQuantity}` : ''} from the warehouse.`,
    'success',
  );
};

const applyInteractNpc = (state: GameState, npcId: string, actionId?: 'accept_task' | 'turn_in_task'): void => {
  const npc = state.npcs[npcId];
  if (!npc) {
    return;
  }
  if (distance(state.player.position, npc.position) > 4.5) {
    pushLog(state, `${npc.name} is too far away to hear you.`, 'warning');
    return;
  }
  interactWithNpc(state, actionId);
};

const applyCloseDialog = (state: GameState): void => {
  state.dialog = null;
};

export const getDerivedStats = (state: GameState): DerivedStats => {
  if (state.player.authoritativeStats) {
    return { ...state.player.authoritativeStats };
  }

  const archetype = gameTemplates.archetypes[state.player.archetypeId];
  const levelOffset = state.player.level - 1;
  const stats: DerivedStats = {
    maxCp: archetype.baseCp + archetype.cpGrowth * levelOffset,
    maxHp: archetype.baseHp + archetype.hpGrowth * levelOffset,
    maxMp: archetype.baseMp + archetype.mpGrowth * levelOffset,
    attack: archetype.baseAttack + archetype.attackGrowth * levelOffset,
    defense: archetype.baseDefense + archetype.defenseGrowth * levelOffset,
    moveSpeed: archetype.baseMoveSpeed,
  };
  const passiveBonuses = passiveSkillBonuses(state);
  stats.maxCp += passiveBonuses.maxCp;
  stats.maxHp += passiveBonuses.maxHp;
  stats.maxMp += passiveBonuses.maxMp;
  stats.attack += passiveBonuses.attack;
  stats.defense += passiveBonuses.defense;
  stats.moveSpeed += passiveBonuses.moveSpeed;

  for (const item of Object.values(state.items)) {
    if (item.container !== 'equipment') {
      continue;
    }
    const bonus = getItemTemplate(item.templateId).statBonuses;
    if (bonus) {
      stats.maxCp += bonus.maxCp ?? 0;
      stats.maxHp += bonus.maxHp ?? 0;
      stats.maxMp += bonus.maxMp ?? 0;
      stats.attack += bonus.attack ?? 0;
      stats.defense += bonus.defense ?? 0;
      stats.moveSpeed += bonus.moveSpeed ?? 0;
    }
    stats.maxCp += item.instanceAttributes?.maxCp ?? 0;
    stats.maxHp += item.instanceAttributes?.maxHp ?? 0;
    stats.maxMp += item.instanceAttributes?.maxMp ?? 0;
    stats.attack += item.instanceAttributes?.attack ?? 0;
    stats.defense += item.instanceAttributes?.defense ?? 0;
    stats.moveSpeed += item.instanceAttributes?.moveSpeed ?? 0;
  }

  return stats;
};

export const getInventoryItems = (state: GameState): ItemInstance[] =>
  Object.values(state.items)
    .filter((item) => item.container === 'inventory')
    .sort((left, right) => {
      const leftTemplate = getItemTemplate(left.templateId);
      const rightTemplate = getItemTemplate(right.templateId);
      return leftTemplate.name.localeCompare(rightTemplate.name);
    });

export const getWarehouseItems = (state: GameState): ItemInstance[] =>
  Object.values(state.items)
    .filter((item) => item.container === 'warehouse')
    .sort((left, right) => {
      const leftTemplate = getItemTemplate(left.templateId);
      const rightTemplate = getItemTemplate(right.templateId);
      return leftTemplate.name.localeCompare(rightTemplate.name);
    });

export const getRegionIdForPoint = (point: Vec2): RegionId => {
  if (point.x < 18) {
    return 'dawn_plaza';
  }
  if (point.x < 32) {
    return 'gate_road';
  }
  if (point.x < 70) {
    return 'gloam_field';
  }
  return 'ruin_hollow';
};

export const regionLabels: Record<RegionId, string> = {
  dawn_plaza: 'Dawn Plaza',
  gate_road: 'Gate Road',
  gloam_field: 'Gloam Field',
  ruin_hollow: 'Ruin Hollow',
};

export const getTargetMob = (state: GameState): MobState | null => getMob(state, state.targetId);

export const getEquippedBySlot = (state: GameState): Record<EquipSlot, ItemInstance | null> => ({
  weapon: getEquippedItem(state, 'weapon'),
  chest: getEquippedItem(state, 'chest'),
  gloves: getEquippedItem(state, 'gloves'),
  boots: getEquippedItem(state, 'boots'),
});

export const getItemLabel = (item: ItemInstance): string => {
  const template = getItemTemplate(item.templateId);
  return template.stackable ? `${template.name} x${item.quantity}` : template.name;
};

export const getItemAttributeSummary = (item: ItemInstance): string | null => {
  if (!item.instanceAttributes) {
    return null;
  }

  const parts: string[] = [];
  if (item.instanceAttributes.maxCp) {
    parts.push(`CP +${formatItemAttributeValue(item.instanceAttributes.maxCp)}`);
  }
  if (item.instanceAttributes.maxHp) {
    parts.push(`HP +${formatItemAttributeValue(item.instanceAttributes.maxHp)}`);
  }
  if (item.instanceAttributes.maxMp) {
    parts.push(`MP +${formatItemAttributeValue(item.instanceAttributes.maxMp)}`);
  }
  if (item.instanceAttributes.attack) {
    parts.push(`ATK +${formatItemAttributeValue(item.instanceAttributes.attack)}`);
  }
  if (item.instanceAttributes.defense) {
    parts.push(`DEF +${formatItemAttributeValue(item.instanceAttributes.defense)}`);
  }
  if (item.instanceAttributes.moveSpeed) {
    parts.push(`MOVE +${formatItemAttributeValue(item.instanceAttributes.moveSpeed)}`);
  }

  return parts.length > 0 ? parts.join(' | ') : null;
};

export const getNearbyWarehouse = (
  state: GameState,
): {
  npcId: string;
  name: string;
  title: string;
  items: ItemInstance[];
} | null => {
  const warehouseKeeper = state.npcs.npc_warehouse_keeper;
  if (!warehouseKeeper || distance(state.player.position, warehouseKeeper.position) > WAREHOUSE_INTERACTION_RANGE) {
    return null;
  }

  return {
    npcId: warehouseKeeper.id,
    name: warehouseKeeper.name,
    title: warehouseKeeper.title,
    items: getWarehouseItems(state),
  };
};

export const getNearbyVendor = (
  state: GameState,
): {
  npcId: string;
  name: string;
  title: string;
  offers: VendorOfferTemplate[];
  exchangeOffers: ExchangeOfferTemplate[];
  currencyQuantity: number;
} | null => {
  const nearbyVendors = Object.values(state.npcs)
    .map((npc) => ({
      npc,
      offers: Object.values(gameTemplates.vendorOffers)
        .filter((offer) => offer.npcId === npc.id)
        .sort((left, right) => left.priceAmount - right.priceAmount),
      exchangeOffers: Object.values(gameTemplates.exchangeOffers)
        .filter((offer) => offer.npcId === npc.id)
        .sort((left, right) => left.costAmount - right.costAmount),
      distance: distance(state.player.position, npc.position),
    }))
    .filter((entry) => (entry.offers.length > 0 || entry.exchangeOffers.length > 0) && entry.distance <= VENDOR_INTERACTION_RANGE)
    .sort((left, right) => left.distance - right.distance);

  const entry = nearbyVendors[0];
  if (!entry) {
    return null;
  }

  const priceCurrencyTemplateId = entry.offers[0]?.priceCurrencyTemplateId ?? 'duskgold';
  return {
    npcId: entry.npc.id,
    name: entry.npc.name,
    title: entry.npc.title,
    offers: entry.offers,
    exchangeOffers: entry.exchangeOffers,
    currencyQuantity: getInventoryTemplateQuantity(state, priceCurrencyTemplateId),
  };
};

export const getNearbyTradePlayers = (state: GameState): OtherPlayerState[] =>
  Object.values(state.otherPlayers)
    .filter((otherPlayer) => distance(state.player.position, otherPlayer.position) <= PLAYER_TRADE_INTERACTION_RANGE)
    .sort((left, right) => distance(state.player.position, left.position) - distance(state.player.position, right.position));

export const getTemplate = getItemTemplate;

export class GameStore {
  private state: GameState;

  constructor(initialState?: GameState) {
    this.state = initialState ?? createInitialState();
    normalizeState(this.state);
    clampPoolsToDerived(this.state);
  }

  getState(): GameState {
    return this.state;
  }

  replaceState(nextState: GameState): void {
    this.state = nextState;
    normalizeState(this.state);
    clampPoolsToDerived(this.state);
  }

  dispatch(command: GameCommand): void {
    switch (command.type) {
      case 'moveToPoint':
        applyMoveToPoint(this.state, command.point);
        break;
      case 'selectTarget':
        applySelectTarget(this.state, command.targetId);
        break;
      case 'useSkill':
        applyUseSkill(this.state, command.skillId);
        break;
      case 'basicAttack':
        applyBasicAttack(this.state);
        break;
      case 'pickUpLoot':
        applyPickUpLoot(this.state, command.lootId);
        break;
      case 'pickUpNearbyLoot':
        applyPickUpNearbyLoot(this.state);
        break;
      case 'useItem':
        applyUseItem(this.state, command.itemId);
        break;
      case 'equipItem':
        applyEquipItem(this.state, command.itemId);
        break;
      case 'unequipItem':
        applyUnequipItem(this.state, command.slot);
        break;
      case 'splitItemStack':
        applySplitItemStack(this.state, command.itemId, command.quantity);
        break;
      case 'mergeItemStacks':
        applyMergeItemStacks(this.state, command.sourceItemId, command.targetItemId);
        break;
      case 'buyVendorOffer':
        applyBuyVendorOffer(this.state, command.offerId, command.quantity);
        break;
      case 'exchangeVendorOffer':
        applyExchangeVendorOffer(this.state, command.offerId, command.quantity);
        break;
      case 'sellVendorItem':
        applySellVendorItem(this.state, command.itemId, command.quantity);
        break;
      case 'depositWarehouseItem':
        applyDepositWarehouseItem(this.state, command.itemId, command.quantity);
        break;
      case 'withdrawWarehouseItem':
        applyWithdrawWarehouseItem(this.state, command.itemId, command.quantity);
        break;
      case 'interactNpc':
        applyInteractNpc(this.state, command.npcId, command.actionId);
        break;
      case 'closeDialog':
        applyCloseDialog(this.state);
        break;
      default:
        break;
    }
  }

  tick(deltaMs: number): void {
    const state = this.state;
    state.timeMs += deltaMs;

    for (const key of Object.keys(state.player.cooldowns)) {
      state.player.cooldowns[key] = Math.max(0, state.player.cooldowns[key] - deltaMs);
    }

    state.floatingTexts = state.floatingTexts
      .map((entry) => ({
        ...entry,
        ttlMs: entry.ttlMs - deltaMs,
      }))
      .filter((entry) => entry.ttlMs > 0);

    if (state.player.deadUntilMs && state.timeMs >= state.player.deadUntilMs) {
      const stats = getDerivedStats(state);
      state.player.deadUntilMs = null;
      state.player.position = { x: -8, z: 0 };
      state.player.cp = stats.maxCp;
      state.player.hp = stats.maxHp;
      state.player.mp = stats.maxMp;
      state.player.facing = 0;
      resetIdleRegenClock(state);
      pushLog(state, 'You regroup in Dawn Plaza.', 'success');
    }

    if (!state.player.deadUntilMs) {
      if (state.player.cast) {
        state.player.cast.remainingMs -= deltaMs;
        if (state.player.cast.remainingMs <= 0) {
          const cast = state.player.cast;
          state.player.cast = null;
          const skill = getSkill(cast.skillId);
          if (skill) {
            resolveSkill(state, skill, cast.targetId);
          }
        }
      } else {
        updateQueuedSkillApproach(state);
        updateQueuedBasicAttackApproach(state);
        updateQueuedLootPickup(state);
        if (!state.player.cast && state.player.moveTarget) {
          const stats = getDerivedStats(state);
          const previousPosition = { ...state.player.position };
          const next = moveTowards(state.player.position, state.player.moveTarget, (stats.moveSpeed * deltaMs) / 1000);
          state.player.facing = Math.atan2(state.player.moveTarget.z - state.player.position.z, state.player.moveTarget.x - state.player.position.x);
          state.player.position = next;
          if (distance(previousPosition, state.player.position) > 0.001) {
            resetIdleRegenClock(state);
          }
          if (distance(state.player.position, state.player.moveTarget) < 0.2) {
            state.player.moveTarget = null;
            state.destinationMarker = null;
          }
        }
        if (!state.player.cast) {
          updateQueuedSkillApproach(state);
          updateQueuedBasicAttackApproach(state);
          updateQueuedLootPickup(state);
        }
      }
    }

    applyIdleRegen(state);

    for (const mob of Object.values(state.mobs)) {
      const template = gameTemplates.mobTemplates[mob.templateId];
      mob.attackCooldownMs = Math.max(0, mob.attackCooldownMs - deltaMs);

      if (mob.aiState === 'dead') {
        if (mob.respawnAtMs && state.timeMs >= mob.respawnAtMs) {
          mob.aiState = 'idle';
          mob.hp = template.maxHp;
          mob.position = { ...mob.spawnPoint };
          mob.respawnAtMs = null;
          pushLog(state, `${template.name} returns to the field.`, 'neutral');
        }
        continue;
      }

      if (state.player.deadUntilMs) {
        if (distance(mob.position, mob.spawnPoint) > 0.1) {
          mob.position = moveTowards(mob.position, mob.spawnPoint, (template.moveSpeed * deltaMs) / 1000);
        }
        mob.aiState = 'idle';
        continue;
      }

      const playerDistance = distance(mob.position, state.player.position);
      if (playerDistance <= template.aggroRadius || mob.aiState === 'aggro') {
        mob.aiState = 'aggro';
      }

      if (mob.aiState === 'aggro') {
        if (playerDistance > 15 && distance(mob.position, mob.spawnPoint) > 0.1) {
          mob.position = moveTowards(mob.position, mob.spawnPoint, (template.moveSpeed * deltaMs) / 1000);
          if (distance(mob.position, mob.spawnPoint) < 0.2) {
            mob.aiState = 'idle';
          }
          continue;
        }

        if (playerDistance > template.attackRange) {
          mob.position = moveTowards(mob.position, state.player.position, (template.moveSpeed * deltaMs) / 1000);
        } else if (mob.attackCooldownMs <= 0) {
          const playerStats = getDerivedStats(state);
          const damage = Math.max(4, Math.round(template.attack - playerStats.defense * 0.25));
          mob.attackCooldownMs = template.attackIntervalMs;
          damagePlayer(state, template.name, damage);
        }
      } else if (distance(mob.position, mob.spawnPoint) > 0.1) {
        mob.position = moveTowards(mob.position, mob.spawnPoint, (template.moveSpeed * deltaMs) / 1000);
      }
    }

    if (state.targetId && !getMob(state, state.targetId)) {
      state.targetId = null;
    }
    if (state.player.queuedSkill && !getMob(state, state.player.queuedSkill.targetId)) {
      state.player.queuedSkill = null;
      state.player.moveTarget = null;
      state.destinationMarker = null;
    }
    if (state.player.queuedBasicAttackTargetId && !getMob(state, state.player.queuedBasicAttackTargetId)) {
      state.player.queuedBasicAttackTargetId = null;
      state.player.moveTarget = null;
      state.destinationMarker = null;
    }
    if (state.player.queuedLootId && !state.loot[state.player.queuedLootId]) {
      state.player.queuedLootId = null;
      state.player.moveTarget = null;
      state.destinationMarker = null;
    }

    clampPoolsToDerived(state);
  }
}
