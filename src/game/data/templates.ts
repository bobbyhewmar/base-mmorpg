import type {
  BaseClass,
  GameState,
  GameTemplates,
  HotbarActionId,
  ItemInstance,
  MobState,
  NpcState,
  PlayerHotbarSlot,
  PlayerHotbarState,
  PlayerKnownSkill,
  PlayerState,
  QuestState,
  SkillCategory,
} from '../domain/types';

type ClassSkillGrant = {
  skillId: string;
  category: SkillCategory;
  unlockLevel: number;
};

type ClassContent = {
  archetypeId: string;
  defaultHotbarSkills: string[];
  learnedSkills: ClassSkillGrant[];
};

const HOTBAR_SLOT_COUNT = 36;
export const HOTBAR_ROW_SIZE = 12;
export const HOTBAR_MAX_OPEN_BARS = 3;

const isHotbarActionId = (value: unknown): value is HotbarActionId =>
  value === 'basic_attack' ||
  value === 'pick_up_nearby' ||
  value === 'tame_target' ||
  value === 'summon_pet' ||
  value === 'dismiss_pet' ||
  value === 'mount_pet' ||
  value === 'dismount_pet';

const classContent: Record<BaseClass, ClassContent> = {
  Fighter: {
    archetypeId: 'dusk_vanguard',
    defaultHotbarSkills: ['crescent_strike', 'grave_bloom'],
    learnedSkills: [
      { skillId: 'crescent_strike', category: 'active', unlockLevel: 1 },
      { skillId: 'iron_will', category: 'passive', unlockLevel: 1 },
      { skillId: 'grave_bloom', category: 'active', unlockLevel: 2 },
    ],
  },
  Mage: {
    archetypeId: 'ashen_oracle',
    defaultHotbarSkills: ['ember_shot', 'astral_burst'],
    learnedSkills: [
      { skillId: 'ember_shot', category: 'active', unlockLevel: 1 },
      { skillId: 'arcane_focus', category: 'passive', unlockLevel: 1 },
      { skillId: 'astral_burst', category: 'active', unlockLevel: 2 },
    ],
  },
};

export const getArchetypeIdForBaseClass = (baseClass: BaseClass): string =>
  classContent[baseClass]?.archetypeId ?? classContent.Fighter.archetypeId;

export const getLearnedSkillsForCharacter = (baseClass: BaseClass, level: number): PlayerKnownSkill[] =>
  [...(classContent[baseClass]?.learnedSkills ?? classContent.Fighter.learnedSkills)]
    .filter((skill) => Math.max(level, 1) >= skill.unlockLevel)
    .map((skill) => ({
      skillId: skill.skillId,
      category: skill.category,
      unlockLevel: skill.unlockLevel,
    }))
    .sort((left, right) => {
      if (left.category === right.category) {
        if (left.unlockLevel === right.unlockLevel) {
          return left.skillId.localeCompare(right.skillId);
        }
        return left.unlockLevel - right.unlockLevel;
      }
      return left.category.localeCompare(right.category);
    });

export const createDefaultHotbarState = (baseClass: BaseClass): PlayerHotbarState => ({
  openBarCount: 1,
  slots: Array.from({ length: HOTBAR_SLOT_COUNT }, (_, slotIndex) => {
    const skillId = classContent[baseClass]?.defaultHotbarSkills[slotIndex] ?? null;
    return {
      slotIndex,
      entryType: skillId ? 'skill' : null,
      skillId,
      itemId: null,
      actionId: null,
    } satisfies PlayerHotbarSlot;
  }),
});

export const normalizeHotbarState = (
  hotbar: Partial<PlayerHotbarState> | null | undefined,
  baseClass: BaseClass,
): PlayerHotbarState => {
  const normalized = createDefaultHotbarState(baseClass);
  if (!hotbar) {
    return normalized;
  }

  if (
    typeof hotbar.openBarCount === 'number' &&
    hotbar.openBarCount >= 1 &&
    hotbar.openBarCount <= HOTBAR_MAX_OPEN_BARS
  ) {
    normalized.openBarCount = hotbar.openBarCount;
  }

  const slots = Array.isArray(hotbar.slots) ? hotbar.slots : [];
  for (const slot of slots) {
    if (!slot || typeof slot.slotIndex !== 'number' || slot.slotIndex < 0 || slot.slotIndex >= HOTBAR_SLOT_COUNT) {
      continue;
    }
    if (slot.entryType === 'skill' && typeof slot.skillId === 'string') {
      normalized.slots[slot.slotIndex] = {
        slotIndex: slot.slotIndex,
        entryType: 'skill',
        skillId: slot.skillId,
        itemId: null,
        actionId: null,
      };
      continue;
    }

    if (slot.entryType === 'item' && typeof slot.itemId === 'string') {
      normalized.slots[slot.slotIndex] = {
        slotIndex: slot.slotIndex,
        entryType: 'item',
        skillId: null,
        itemId: slot.itemId,
        actionId: null,
      };
      continue;
    }

    if (slot.entryType === 'action' && isHotbarActionId(slot.actionId)) {
      normalized.slots[slot.slotIndex] = {
        slotIndex: slot.slotIndex,
        entryType: 'action',
        skillId: null,
        itemId: null,
        actionId: slot.actionId,
      };
      continue;
    }

    normalized.slots[slot.slotIndex] = {
      slotIndex: slot.slotIndex,
      entryType: null,
      skillId: null,
      itemId: null,
      actionId: null,
    };
  }

  return {
    openBarCount: normalized.openBarCount,
    slots: [...normalized.slots].sort((left, right) => left.slotIndex - right.slotIndex),
  };
};

export const findKnownSkill = (
  learnedSkills: PlayerKnownSkill[],
  skillId: string,
): PlayerKnownSkill | null => learnedSkills.find((skill) => skill.skillId === skillId) ?? null;

export const getPlayerHotbarBoundSkillId = (
  player: Pick<PlayerState, 'hotbar' | 'learnedSkills'>,
  slotIndex: number,
): string | null => {
  const slot = player.hotbar.slots.find((entry) => entry.slotIndex === slotIndex);
  if (!slot || slot.entryType !== 'skill' || !slot.skillId) {
    return null;
  }
  const knownSkill = findKnownSkill(player.learnedSkills, slot.skillId);
  if (!knownSkill || knownSkill.category !== 'active') {
    return null;
  }
  return slot.skillId;
};

export const getVisibleHotbarRows = (hotbar: PlayerHotbarState): PlayerHotbarSlot[][] => {
  const openBarCount =
    typeof hotbar.openBarCount === 'number' && hotbar.openBarCount >= 1 && hotbar.openBarCount <= HOTBAR_MAX_OPEN_BARS
      ? hotbar.openBarCount
      : 1;
  const slots = [...hotbar.slots].sort((left, right) => left.slotIndex - right.slotIndex);
  const rows: PlayerHotbarSlot[][] = [];
  for (let rowIndex = 0; rowIndex < openBarCount; rowIndex++) {
    const rowStart = rowIndex * HOTBAR_ROW_SIZE;
    rows.push(slots.slice(rowStart, rowStart + HOTBAR_ROW_SIZE));
  }
  return rows;
};

const createItemInstance = (
  id: string,
  templateId: string,
  quantity: number,
  container: ItemInstance['container'],
  equipSlot?: ItemInstance['equipSlot'],
  instanceAttributes?: ItemInstance['instanceAttributes'],
): ItemInstance => ({
  id,
  templateId,
  quantity,
  container,
  equipSlot,
  instanceAttributes,
});

const createMob = (id: string, templateId: string, x: number, z: number, maxHp: number): MobState => ({
  id,
  templateId,
  position: { x, z },
  spawnPoint: { x, z },
  hp: maxHp,
  aiState: 'idle',
  attackCooldownMs: 0,
  respawnAtMs: null,
});

const createNpc = (id: string, name: string, title: string, x: number, z: number): NpcState => ({
  id,
  name,
  title,
  position: { x, z },
});

const createQuest = (): QuestState => ({
  id: 'keeper_request',
  title: 'Keeper of the Gate',
  description: 'Defeat 3 Mirelings beyond the gate, then return to the wardkeeper.',
  status: 'available',
  progress: 0,
  goal: 3,
});

export const gameTemplates: GameTemplates = {
  archetypes: {
    dusk_vanguard: {
      id: 'dusk_vanguard',
      name: 'Dusk Vanguard',
      title: 'Gatebound Initiate',
      baseCp: 80,
      baseHp: 122,
      baseMp: 58,
      baseAttack: 17,
      baseDefense: 9,
      baseMoveSpeed: 8.6,
      cpGrowth: 12,
      hpGrowth: 18,
      mpGrowth: 7,
      attackGrowth: 4,
      defenseGrowth: 2,
    },
    ashen_oracle: {
      id: 'ashen_oracle',
      name: 'Ashen Oracle',
      title: 'Ashen Scholar',
      baseCp: 55,
      baseHp: 92,
      baseMp: 92,
      baseAttack: 13,
      baseDefense: 6,
      baseMoveSpeed: 8.2,
      cpGrowth: 9,
      hpGrowth: 18,
      mpGrowth: 7,
      attackGrowth: 4,
      defenseGrowth: 2,
    },
  },
  skills: {
    crescent_strike: {
      id: 'crescent_strike',
      name: 'Crescent Strike',
      description: 'A quick target-locked cut against one enemy.',
      category: 'active',
      baseClass: 'Fighter',
      unlockLevel: 1,
      iconKey: 'CS',
      iconTint: '#d58f62',
      targetType: 'single_target_enemy',
      castTimeMs: 180,
      cooldownMs: 900,
      mpCost: 6,
      range: 8,
      power: 18,
      maxTargets: 1,
    },
    grave_bloom: {
      id: 'grave_bloom',
      name: 'Grave Bloom',
      description: 'A slower pulse around the current target that splits its damage.',
      category: 'active',
      baseClass: 'Fighter',
      unlockLevel: 2,
      iconKey: 'GB',
      iconTint: '#8c728f',
      targetType: 'target_centered_aoe',
      castTimeMs: 900,
      cooldownMs: 4500,
      mpCost: 14,
      range: 9,
      radius: 7,
      power: 40,
      maxTargets: 4,
    },
    iron_will: {
      id: 'iron_will',
      name: 'Iron Will',
      description: 'A passive martial discipline that hardens the body and steadies the guard.',
      category: 'passive',
      baseClass: 'Fighter',
      unlockLevel: 1,
      iconKey: 'IW',
      iconTint: '#6f86b8',
      targetType: 'passive',
      castTimeMs: 0,
      cooldownMs: 0,
      mpCost: 0,
      range: 0,
      power: 0,
      maxTargets: 0,
    },
    ember_shot: {
      id: 'ember_shot',
      name: 'Ember Shot',
      description: 'A focused ember bolt that snaps toward one enemy.',
      category: 'active',
      baseClass: 'Mage',
      unlockLevel: 1,
      iconKey: 'ES',
      iconTint: '#de7d51',
      targetType: 'single_target_enemy',
      castTimeMs: 180,
      cooldownMs: 800,
      mpCost: 7,
      range: 10,
      power: 20,
      maxTargets: 1,
    },
    astral_burst: {
      id: 'astral_burst',
      name: 'Astral Burst',
      description: 'A delayed astral surge around the chosen target that divides its force.',
      category: 'active',
      baseClass: 'Mage',
      unlockLevel: 2,
      iconKey: 'AB',
      iconTint: '#7d73d6',
      targetType: 'target_centered_aoe',
      castTimeMs: 900,
      cooldownMs: 4200,
      mpCost: 15,
      range: 10,
      radius: 7,
      power: 38,
      maxTargets: 4,
    },
    arcane_focus: {
      id: 'arcane_focus',
      name: 'Arcane Focus',
      description: 'A passive discipline that sharpens magical reserve and refines spell force.',
      category: 'passive',
      baseClass: 'Mage',
      unlockLevel: 1,
      iconKey: 'AF',
      iconTint: '#57a6c7',
      targetType: 'passive',
      castTimeMs: 0,
      cooldownMs: 0,
      mpCost: 0,
      range: 0,
      power: 0,
      maxTargets: 0,
    },
  },
  itemTemplates: {
    duskgold: {
      id: 'duskgold',
      name: 'Duskgold',
      description: 'A stamped coin used by the plaza merchants.',
      kind: 'currency',
      stackable: true,
    },
    healing_potion: {
      id: 'healing_potion',
      name: 'Healing Potion',
      description: 'A restorative draught that recovers 45 HP when consumed.',
      kind: 'consumable',
      stackable: true,
    },
    ironwood_spear: {
      id: 'ironwood_spear',
      name: 'Ironwood Spear',
      description: 'A long ash spear salvaged from the ruin pocket.',
      kind: 'weapon',
      stackable: false,
      equipSlot: 'weapon',
      statBonuses: {
        attack: 10,
      },
      appearance: {
        weaponModel: 'spear',
        tint: '#cbb98d',
      },
    },
    wardkeeper_mantle: {
      id: 'wardkeeper_mantle',
      name: 'Wardkeeper Mantle',
      description: 'A heavy mantle stitched for defenders of the gate road.',
      kind: 'armor',
      stackable: false,
      equipSlot: 'chest',
      statBonuses: {
        defense: 6,
        maxHp: 20,
      },
      appearance: {
        chestModel: 'mantle',
        tint: '#5f8ccf',
      },
    },
    watcher_gloves: {
      id: 'watcher_gloves',
      name: 'Watcher Gloves',
      description: 'Reinforced gloves worn by plaza sentries to steady fast strikes.',
      kind: 'armor',
      stackable: false,
      equipSlot: 'gloves',
      statBonuses: {
        attack: 4,
        defense: 1,
      },
      appearance: {
        tint: '#8d7a61',
      },
    },
    pathrunner_boots: {
      id: 'pathrunner_boots',
      name: 'Pathrunner Boots',
      description: 'Flexible boots cut for long roads and sudden sidesteps.',
      kind: 'armor',
      stackable: false,
      equipSlot: 'boots',
      statBonuses: {
        defense: 1,
        moveSpeed: 0.4,
      },
      appearance: {
        tint: '#6f8d63',
      },
    },
    ruin_shard: {
      id: 'ruin_shard',
      name: 'Ruin Shard',
      description: 'Jagged salvage pulled from the broken gate ruins and favored by local traders.',
      kind: 'material',
      stackable: true,
    },
    ruinbound_greaves: {
      id: 'ruinbound_greaves',
      name: 'Ruinbound Greaves',
      description: 'Greaves reinforced with shard-laced plates for steadier footing in the ruins.',
      kind: 'armor',
      stackable: false,
      equipSlot: 'boots',
      statBonuses: {
        defense: 2,
        moveSpeed: 0.6,
      },
      appearance: {
        tint: '#6a8191',
      },
    },
  },
  mobTemplates: {
    mireling: {
      id: 'mireling',
      name: 'Mireling',
      tint: '#8e6f8f',
      maxHp: 54,
      attack: 8,
      defense: 3,
      moveSpeed: 5.4,
      aggroRadius: 8.5,
      attackRange: 2.1,
      attackIntervalMs: 1400,
      xpReward: 22,
      currencyDrop: 4,
    },
    ruin_stalker: {
      id: 'ruin_stalker',
      name: 'Ruin Stalker',
      tint: '#c45a4c',
      maxHp: 84,
      attack: 12,
      defense: 5,
      moveSpeed: 6.2,
      aggroRadius: 10.5,
      attackRange: 2.4,
      attackIntervalMs: 1250,
      xpReward: 34,
      currencyDrop: 6,
      guaranteedEquipmentTemplateId: 'ironwood_spear',
    },
  },
  vendorOffers: {
    merchant_spear_offer: {
      id: 'merchant_spear_offer',
      npcId: 'npc_merchant',
      templateId: 'ironwood_spear',
      quantity: 1,
      priceCurrencyTemplateId: 'duskgold',
      priceAmount: 8,
    },
    merchant_ruin_shard_bundle: {
      id: 'merchant_ruin_shard_bundle',
      npcId: 'npc_merchant',
      templateId: 'ruin_shard',
      quantity: 4,
      priceCurrencyTemplateId: 'duskgold',
      priceAmount: 4,
    },
  },
  exchangeOffers: {
    merchant_mantle_exchange: {
      id: 'merchant_mantle_exchange',
      npcId: 'npc_merchant',
      templateId: 'wardkeeper_mantle',
      quantity: 1,
      costTemplateId: 'duskgold',
      costAmount: 10,
    },
    merchant_ruinbound_greaves_exchange: {
      id: 'merchant_ruinbound_greaves_exchange',
      npcId: 'npc_merchant',
      templateId: 'ruinbound_greaves',
      quantity: 1,
      costTemplateId: 'ruin_shard',
      costAmount: 6,
    },
  },
};

export const createInitialState = (): GameState => {
  const baseClass: BaseClass = 'Fighter';
  const level = 1;
  const items: Record<string, ItemInstance> = {
    item_duskgold_start: createItemInstance('item_duskgold_start', 'duskgold', 12, 'inventory'),
    item_healing_potion_start: createItemInstance('item_healing_potion_start', 'healing_potion', 3, 'inventory'),
    item_gloves_start: createItemInstance('item_gloves_start', 'watcher_gloves', 1, 'inventory', undefined, {
      attack: 1,
      defense: 1,
    }),
    item_boots_start: createItemInstance('item_boots_start', 'pathrunner_boots', 1, 'inventory'),
  };

  return {
    timeMs: 0,
    nextId: 100,
    targetId: null,
    destinationMarker: null,
    pendingPath: [],
    authoritativePath: [],
    player: {
      id: 'player',
      name: 'Arden',
      race: 'Human',
      baseClass,
      sex: 'Male',
      hairStyle: 0,
      hairColor: 0,
      face: 0,
      archetypeId: getArchetypeIdForBaseClass(baseClass),
      level,
      xp: 0,
      cp: 80,
      hp: 122,
      mp: 58,
      position: { x: -8, z: 0 },
      facing: 0,
      moveTarget: null,
      stationarySinceMs: 0,
      lastIdleRegenAtMs: 0,
      cast: null,
      queuedSkill: null,
      queuedBasicAttackTargetId: null,
      queuedLootId: null,
      cooldowns: {},
      skillAvailability: {},
      learnedSkills: getLearnedSkillsForCharacter(baseClass, level),
      hotbar: createDefaultHotbarState(baseClass),
      pets: [],
      activePetId: null,
      mountedPetId: null,
      deadUntilMs: null,
    },
    otherPlayers: {},
    companions: {},
    mobs: {
      mob_1: createMob('mob_1', 'mireling', 34, 10, gameTemplates.mobTemplates.mireling.maxHp),
      mob_2: createMob('mob_2', 'mireling', 42, -7, gameTemplates.mobTemplates.mireling.maxHp),
      mob_3: createMob('mob_3', 'mireling', 49, 13, gameTemplates.mobTemplates.mireling.maxHp),
      mob_4: createMob('mob_4', 'mireling', 56, -12, gameTemplates.mobTemplates.mireling.maxHp),
      mob_5: createMob('mob_5', 'ruin_stalker', 77, -4, gameTemplates.mobTemplates.ruin_stalker.maxHp),
      mob_6: createMob('mob_6', 'ruin_stalker', 87, 9, gameTemplates.mobTemplates.ruin_stalker.maxHp),
    },
    loot: {},
    items,
    npcs: {
      npc_wardkeeper: createNpc('npc_wardkeeper', 'Selka', 'Wardkeeper of the Plaza', -2, 10),
      npc_merchant: createNpc('npc_merchant', 'Ilya', 'Provisioner of the Plaza', -10, 8),
      npc_warehouse_keeper: createNpc('npc_warehouse_keeper', 'Rhea', 'Vaultkeeper of the Plaza', -13, 4),
    },
    quest: createQuest(),
    dialog: null,
    party: null,
    partyInvites: [],
    incomingTradeOffer: null,
    outgoingTradeOffer: null,
    logs: [
      { id: 'log_1', text: 'You arrive in Dawn Plaza. Cross the gate road to find prey.', tone: 'neutral' },
      { id: 'log_2', text: 'Click terrain to move. Use the hotbar for learned skills and press ALT+K for the skill book.', tone: 'neutral' },
    ],
    floatingTexts: [],
    equipmentAwardsGranted: [],
    lastAutoSaveAtMs: 0,
  };
};
