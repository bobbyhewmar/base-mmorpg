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
  value === 'party_invite' ||
  value === 'party_leave' ||
  value === 'tame_target' ||
  value === 'summon_pet' ||
  value === 'dismiss_pet' ||
  value === 'mount_pet' ||
  value === 'dismount_pet' ||
  value === 'toggle_walk_run';

const classContent: Record<BaseClass, ClassContent> = {
  Fighter: {
    archetypeId: 'dusk_vanguard',
    defaultHotbarSkills: ['crescent_strike'],
    learnedSkills: [
      { skillId: 'crescent_strike', category: 'active', unlockLevel: 1 },
      { skillId: 'iron_will', category: 'passive', unlockLevel: 1 },
      { skillId: 'grave_bloom', category: 'active', unlockLevel: 2 },
    ],
  },
  Mage: {
    archetypeId: 'ashen_oracle',
    defaultHotbarSkills: ['ember_shot'],
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

export const createMob = (id: string, templateId: string, x: number, z: number, maxHp: number): MobState => ({
  id,
  templateId,
  personality: gameTemplates.mobTemplates[templateId].personality,
  position: { x, z },
  spawnPoint: { x, z },
  hp: maxHp,
  aiState: 'idle',
  attackCooldownMs: 0,
  respawnAtMs: null,
});

export const createNpc = (id: string, name: string, title: string, x: number, z: number): NpcState => ({
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
      baseMoveSpeed: 3.225,
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
      baseMoveSpeed: 3.075,
      cpGrowth: 9,
      hpGrowth: 18,
      mpGrowth: 7,
      attackGrowth: 4,
      defenseGrowth: 2,
    },
    wild_stalker: {
      id: 'wild_stalker',
      name: 'Wild Stalker',
      title: 'Roadside Tracker',
      baseCp: 68,
      baseHp: 108,
      baseMp: 68,
      baseAttack: 16,
      baseDefense: 7,
      baseMoveSpeed: 3.3,
      cpGrowth: 10,
      hpGrowth: 16,
      mpGrowth: 10,
      attackGrowth: 4,
      defenseGrowth: 2,
    },
    void_reaver: {
      id: 'void_reaver',
      name: 'Void Reaver',
      title: 'Gravetide Adept',
      baseCp: 72,
      baseHp: 112,
      baseMp: 78,
      baseAttack: 15,
      baseDefense: 8,
      baseMoveSpeed: 3.15,
      cpGrowth: 11,
      hpGrowth: 17,
      mpGrowth: 13,
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
    novice_oak_staff: {
      id: 'novice_oak_staff',
      name: 'Novice Oak Staff',
      description: 'A plain focus staff carved for first-circle mystics.',
      kind: 'weapon',
      stackable: false,
      equipSlot: 'weapon',
      statBonuses: {
        maxMp: 14,
        attack: 8,
      },
      appearance: {
        weaponModel: 'staff',
        tint: '#8cc3df',
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
    moonthread_robe: {
      id: 'moonthread_robe',
      name: 'Moonthread Robe',
      description: 'A light robe sewn with pale thread to steady novice casting.',
      kind: 'armor',
      stackable: false,
      equipSlot: 'chest',
      statBonuses: {
        maxMp: 22,
        defense: 4,
      },
      appearance: {
        chestModel: 'robe',
        tint: '#596b9d',
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
    runesewn_gloves: {
      id: 'runesewn_gloves',
      name: 'Runesewn Gloves',
      description: 'Soft gloves stitched with tiny focus marks for novice channeling.',
      kind: 'armor',
      stackable: false,
      equipSlot: 'gloves',
      statBonuses: {
        maxMp: 8,
        attack: 3,
      },
      appearance: {
        tint: '#6578af',
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
        moveSpeed: 0.15,
      },
      appearance: {
        tint: '#6f8d63',
      },
    },
    whisperstep_boots: {
      id: 'whisperstep_boots',
      name: 'Whisperstep Boots',
      description: 'Quiet mystic boots made for keeping spell distance without heavy plates.',
      kind: 'armor',
      stackable: false,
      equipSlot: 'boots',
      statBonuses: {
        defense: 1,
        moveSpeed: 0.13,
      },
      appearance: {
        tint: '#556f91',
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
        moveSpeed: 0.225,
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
      level: 3,
      personality: 'passive',
      tint: '#8e6f8f',
      maxHp: 54,
      attack: 8,
      defense: 3,
      moveSpeed: 4.05,
      aggroRadius: 8.5,
      attackRange: 2.1,
      attackIntervalMs: 1400,
      xpReward: 22,
      currencyDrop: 4,
    },
    ruin_stalker: {
      id: 'ruin_stalker',
      name: 'Ruin Stalker',
      level: 12,
      personality: 'aggressive',
      tint: '#c45a4c',
      maxHp: 84,
      attack: 12,
      defense: 5,
      moveSpeed: 4.65,
      aggroRadius: 10.5,
      attackRange: 2.4,
      attackIntervalMs: 1250,
      xpReward: 34,
      currencyDrop: 6,
      guaranteedEquipmentTemplateId: 'ironwood_spear',
    },
    gloom_wisp: {
      id: 'gloom_wisp',
      name: 'Gloom Wisp',
      level: 8,
      personality: 'aggressive',
      tint: '#6b89b8',
      maxHp: 68,
      attack: 10,
      defense: 4,
      moveSpeed: 4.35,
      aggroRadius: 9.5,
      attackRange: 2.2,
      attackIntervalMs: 1320,
      xpReward: 28,
      currencyDrop: 5,
    },
    stonebound_raider: {
      id: 'stonebound_raider',
      name: 'Stonebound Raider',
      level: 20,
      personality: 'aggressive',
      tint: '#a77b56',
      maxHp: 96,
      attack: 15,
      defense: 7,
      moveSpeed: 4.43,
      aggroRadius: 11,
      attackRange: 2.4,
      attackIntervalMs: 1200,
      xpReward: 48,
      currencyDrop: 8,
    },
    ashen_howler: {
      id: 'ashen_howler',
      name: 'Ashen Howler',
      level: 33,
      personality: 'aggressive',
      tint: '#6c6f77',
      maxHp: 132,
      attack: 21,
      defense: 10,
      moveSpeed: 4.95,
      aggroRadius: 12,
      attackRange: 2.5,
      attackIntervalMs: 1120,
      xpReward: 76,
      currencyDrop: 12,
    },
    gravewarden: {
      id: 'gravewarden',
      name: 'Gravewarden',
      level: 39,
      personality: 'aggressive',
      tint: '#43524d',
      maxHp: 176,
      attack: 28,
      defense: 14,
      moveSpeed: 3.9,
      aggroRadius: 13,
      attackRange: 2.8,
      attackIntervalMs: 1050,
      xpReward: 112,
      currencyDrop: 18,
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
    merchant_staff_offer: {
      id: 'merchant_staff_offer',
      npcId: 'npc_merchant',
      templateId: 'novice_oak_staff',
      quantity: 1,
      priceCurrencyTemplateId: 'duskgold',
      priceAmount: 8,
    },
    merchant_robe_offer: {
      id: 'merchant_robe_offer',
      npcId: 'npc_merchant',
      templateId: 'moonthread_robe',
      quantity: 1,
      priceCurrencyTemplateId: 'duskgold',
      priceAmount: 10,
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
    merchant_whisperstep_boots_exchange: {
      id: 'merchant_whisperstep_boots_exchange',
      npcId: 'npc_merchant',
      templateId: 'whisperstep_boots',
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
      hairColor: '#6b4e37',
      skinType: 0,
      archetypeId: getArchetypeIdForBaseClass(baseClass),
      level,
      xp: 0,
      cp: 80,
      hp: 122,
      mp: 58,
      pvpFlagged: false,
      pvpFlagUntilMs: null,
      pvpKills: 0,
      pkCount: 0,
      karma: 0,
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
    mobs: {},
    loot: {},
    items,
    npcs: {},
    quest: createQuest(),
    dialog: null,
    party: null,
    partyInvites: [],
    clan: null,
    clanInvites: [],
    alliance: null,
    allianceInvites: [],
    incomingTradeOffer: null,
    outgoingTradeOffer: null,
    logs: [{ id: 'log_1', text: 'You arrive in a clean 1024x1024 prototype region.', tone: 'neutral' }],
    floatingTexts: [],
    equipmentAwardsGranted: [],
    lastAutoSaveAtMs: 0,
  };
};
