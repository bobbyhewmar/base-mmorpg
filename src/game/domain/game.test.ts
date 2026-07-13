import { describe, expect, it } from 'vitest';

import { createInitialState, getLearnedSkillsForCharacter } from '../data/templates';
import { GameStore, getDerivedStats, getItemAttributeSummary } from './game';

describe('domain combat rules', () => {
  it('regenerates CP, HP, and MP by 3 percent per second after standing still for 5 seconds', () => {
    const state = createInitialState();
    state.player.cp = 0;
    state.player.hp = 40;
    state.player.mp = 10;
    const store = new GameStore(state);

    store.tick(5000);

    expect(store.getState().player.cp).toBe(0);
    expect(store.getState().player.hp).toBe(40);
    expect(store.getState().player.mp).toBe(10);

    store.tick(1000);

    expect(store.getState().player.cp).toBe(3);
    expect(store.getState().player.hp).toBe(44);
    expect(store.getState().player.mp).toBe(12);
  });

  it('resets idle regeneration when movement happens', () => {
    const state = createInitialState();
    state.player.cp = 0;
    state.player.hp = 40;
    state.player.mp = 10;
    const store = new GameStore(state);

    store.tick(4000);
    store.dispatch({ type: 'moveToPoint', point: { x: -7, z: 0 } });
    store.tick(250);
    store.tick(5000);

    expect(store.getState().player.cp).toBe(0);
    expect(store.getState().player.hp).toBe(40);
    expect(store.getState().player.mp).toBe(10);
  });

  it('requires a target before hostile skills can begin casting', () => {
    const state = createInitialState();
    state.player.level = 2;
    state.player.learnedSkills = getLearnedSkillsForCharacter(state.player.baseClass, state.player.level);
    const store = new GameStore(state);

    store.dispatch({ type: 'useSkill', skillId: 'grave_bloom' });

    expect(store.getState().player.cast).toBeNull();
    expect(store.getState().player.cooldowns.grave_bloom ?? 0).toBe(0);
  });

  it('walks into skill range before starting the cast against a selected target', () => {
    const state = createInitialState();
    state.player.position = { x: 20, z: 10 };
    state.mobs.mob_1.position = { x: 34, z: 10 };
    state.targetId = 'mob_1';
    const store = new GameStore(state);

    store.dispatch({ type: 'useSkill', skillId: 'crescent_strike' });

    expect(store.getState().player.cast).toBeNull();
    expect(store.getState().player.queuedSkill).toEqual({ skillId: 'crescent_strike', targetId: 'mob_1' });
    expect(store.getState().logs[0]?.text).toContain('Moving into range');

    store.tick(700);

    expect(store.getState().player.cast?.skillId).toBe('crescent_strike');
    expect(store.getState().player.queuedSkill).toBeNull();
    expect(store.getState().player.mp).toBe(52);
    expect(store.getState().player.cooldowns.crescent_strike).toBe(900);
  });

  it('walks into melee range before resolving a basic attack against the selected target', () => {
    const state = createInitialState();
    state.player.position = { x: 20, z: 10 };
    state.mobs.mob_1.position = { x: 26, z: 10 };
    state.targetId = 'mob_1';
    const store = new GameStore(state);

    store.dispatch({ type: 'basicAttack' });

    expect(store.getState().player.queuedBasicAttackTargetId).toBe('mob_1');
    expect(store.getState().mobs.mob_1.hp).toBe(54);
    expect(store.getState().logs[0]?.text).toContain('Moving into range');

    store.tick(450);

    expect(store.getState().player.queuedBasicAttackTargetId).toBeNull();
    expect(store.getState().player.cooldowns.basic_attack).toBe(750);
    expect(store.getState().mobs.mob_1.hp).toBeLessThan(54);
    expect(store.getState().logs[0]?.text).toContain('basic attack');
  });

  it('splits target-centered AoE damage across nearby enemies', () => {
    const state = createInitialState();
    state.player.level = 2;
    state.player.learnedSkills = getLearnedSkillsForCharacter(state.player.baseClass, state.player.level);
    state.player.position = { x: 42, z: 0 };
    state.mobs.mob_1.position = { x: 45, z: 0 };
    state.mobs.mob_2.position = { x: 46.5, z: 1 };
    state.mobs.mob_3.position = { x: 47, z: -1 };
    state.mobs.mob_4.position = { x: 65, z: 12 };

    const store = new GameStore(state);
    store.dispatch({ type: 'selectTarget', targetId: 'mob_1' });
    store.dispatch({ type: 'useSkill', skillId: 'grave_bloom' });
    store.tick(950);

    expect(store.getState().mobs.mob_1.hp).toBeLessThan(54);
    expect(store.getState().mobs.mob_2.hp).toBeLessThan(54);
    expect(store.getState().mobs.mob_3.hp).toBeLessThan(54);
    expect(store.getState().mobs.mob_4.hp).toBe(54);
  });

  it('rejects active skills that are not learned at the current level', () => {
    const state = createInitialState();
    state.targetId = 'mob_1';
    const store = new GameStore(state);

    store.dispatch({ type: 'useSkill', skillId: 'grave_bloom' });

    expect(store.getState().player.cast).toBeNull();
    expect(store.getState().logs[0]?.text).toContain('not learned');
  });

  it('rejects passive skills as activatable commands', () => {
    const state = createInitialState();
    state.targetId = 'mob_1';
    const store = new GameStore(state);

    store.dispatch({ type: 'useSkill', skillId: 'iron_will' });

    expect(store.getState().player.cast).toBeNull();
    expect(store.getState().logs[0]?.text).toContain('passive');
  });
});

describe('inventory and equipment rules', () => {
  it('walks to clicked loot and picks it up when it becomes reachable', () => {
    const state = createInitialState();
    state.player.position = { x: 0, z: 0 };
    state.items.item_loot_far = {
      id: 'item_loot_far',
      templateId: 'duskgold',
      quantity: 4,
      container: 'ground',
    };
    state.loot.loot_far = {
      id: 'loot_far',
      itemInstanceId: 'item_loot_far',
      position: { x: 7, z: 0 },
      label: 'Duskgold',
    };
    const store = new GameStore(state);

    store.dispatch({ type: 'pickUpLoot', lootId: 'loot_far' });

    expect(store.getState().player.queuedLootId).toBe('loot_far');
    expect(store.getState().items.item_loot_far.container).toBe('ground');
    expect(store.getState().logs[0]?.text).toContain('Moving to pick up');

    store.tick(600);

    expect(store.getState().player.queuedLootId).toBeNull();
    expect(store.getState().loot.loot_far).toBeUndefined();
    expect(store.getState().items.item_loot_far).toBeUndefined();
    expect(store.getState().items.item_duskgold_start.quantity).toBe(16);
  });

  it('picks up the nearest reachable loot through the pickup shortcut', () => {
    const state = createInitialState();
    state.player.position = { x: 0, z: 0 };
    state.items.item_near_loot = {
      id: 'item_near_loot',
      templateId: 'duskgold',
      quantity: 2,
      container: 'ground',
    };
    state.items.item_far_loot = {
      id: 'item_far_loot',
      templateId: 'duskgold',
      quantity: 5,
      container: 'ground',
    };
    state.loot.loot_near = {
      id: 'loot_near',
      itemInstanceId: 'item_near_loot',
      position: { x: 2.5, z: 0 },
      label: 'Duskgold',
    };
    state.loot.loot_far = {
      id: 'loot_far',
      itemInstanceId: 'item_far_loot',
      position: { x: 8, z: 0 },
      label: 'Duskgold',
    };
    const store = new GameStore(state);

    store.dispatch({ type: 'pickUpNearbyLoot' });

    expect(store.getState().loot.loot_near).toBeUndefined();
    expect(store.getState().loot.loot_far).toBeDefined();
    expect(store.getState().items.item_duskgold_start.quantity).toBe(14);
  });

  it('walks to nearby untargeted loot when using the pickup shortcut outside immediate reach', () => {
    const state = createInitialState();
    state.player.position = { x: 0, z: 0 };
    state.targetId = null;
    state.items.item_nearby_loot = {
      id: 'item_nearby_loot',
      templateId: 'duskgold',
      quantity: 3,
      container: 'ground',
    };
    state.loot.loot_nearby = {
      id: 'loot_nearby',
      itemInstanceId: 'item_nearby_loot',
      position: { x: 7, z: 0 },
      label: 'Duskgold',
    };
    const store = new GameStore(state);

    store.dispatch({ type: 'pickUpNearbyLoot' });

    expect(store.getState().targetId).toBeNull();
    expect(store.getState().player.queuedLootId).toBe('loot_nearby');
    expect(store.getState().logs[0]?.text).toContain('Moving to pick up');

    store.tick(600);

    expect(store.getState().player.queuedLootId).toBeNull();
    expect(store.getState().loot.loot_nearby).toBeUndefined();
    expect(store.getState().items.item_nearby_loot).toBeUndefined();
    expect(store.getState().items.item_duskgold_start.quantity).toBe(15);
  });

  it('uses a consumable shortcut item and only mutates local inventory and HP through item truth', () => {
    const state = createInitialState();
    state.player.hp = 70;
    const store = new GameStore(state);

    store.dispatch({ type: 'useItem', itemId: 'item_healing_potion_start' });

    expect(store.getState().player.hp).toBe(115);
    expect(store.getState().items.item_healing_potion_start.quantity).toBe(2);
    expect(store.getState().logs[0]?.text).toContain('Healing Potion');
  });

  it('equipping a weapon changes placement truth and derived stats', () => {
    const state = createInitialState();
    state.items.item_weapon = {
      id: 'item_weapon',
      templateId: 'ironwood_spear',
      quantity: 1,
      container: 'inventory',
    };

    const store = new GameStore(state);
    const before = getDerivedStats(store.getState()).attack;

    store.dispatch({ type: 'equipItem', itemId: 'item_weapon' });

    const after = getDerivedStats(store.getState()).attack;
    expect(store.getState().items.item_weapon.container).toBe('equipment');
    expect(store.getState().items.item_weapon.equipSlot).toBe('weapon');
    expect(after).toBeGreaterThan(before);
  });

  it('equipping boots changes placement truth and derived move speed', () => {
    const store = new GameStore(createInitialState());
    const before = getDerivedStats(store.getState()).moveSpeed;

    store.dispatch({ type: 'equipItem', itemId: 'item_boots_start' });

    const after = getDerivedStats(store.getState()).moveSpeed;
    expect(store.getState().items.item_boots_start.container).toBe('equipment');
    expect(store.getState().items.item_boots_start.equipSlot).toBe('boots');
    expect(after).toBeGreaterThan(before);
  });

  it('equipping starter gloves applies deterministic instance attributes on top of template bonuses', () => {
    const state = createInitialState();
    state.items.item_weapon = {
      id: 'item_weapon',
      templateId: 'ironwood_spear',
      quantity: 1,
      container: 'equipment',
      equipSlot: 'weapon',
    };
    state.items.item_chest = {
      id: 'item_chest',
      templateId: 'wardkeeper_mantle',
      quantity: 1,
      container: 'equipment',
      equipSlot: 'chest',
    };

    const store = new GameStore(state);
    expect(getItemAttributeSummary(store.getState().items.item_gloves_start)).toBe('ATK +1 | DEF +1');
    expect(getDerivedStats(store.getState())).toMatchObject({
      attack: 27,
      defense: 18,
    });

    store.dispatch({ type: 'equipItem', itemId: 'item_gloves_start' });

    expect(store.getState().items.item_gloves_start.container).toBe('equipment');
    expect(store.getState().items.item_gloves_start.equipSlot).toBe('gloves');
    expect(getDerivedStats(store.getState())).toMatchObject({
      attack: 32,
      defense: 20,
    });
  });

  it('splits a stackable inventory item into a new stack', () => {
    const store = new GameStore(createInitialState());

    store.dispatch({ type: 'splitItemStack', itemId: 'item_duskgold_start', quantity: 1 });

    const state = store.getState();
    const duskgoldStacks = Object.values(state.items).filter((item) => item.templateId === 'duskgold');
    expect(state.items.item_duskgold_start.quantity).toBe(11);
    expect(duskgoldStacks).toHaveLength(2);
    expect(duskgoldStacks.some((item) => item.id !== 'item_duskgold_start' && item.quantity === 1)).toBe(true);
    expect(state.logs[0]?.text).toContain('Split Duskgold into 11 and 1.');
  });

  it('merges matching inventory stacks back into one stack', () => {
    const store = new GameStore(createInitialState());
    store.dispatch({ type: 'splitItemStack', itemId: 'item_duskgold_start', quantity: 1 });

    const splitItem = Object.values(store.getState().items).find(
      (item) => item.templateId === 'duskgold' && item.id !== 'item_duskgold_start',
    );
    expect(splitItem).toBeDefined();
    if (!splitItem) {
      return;
    }

    store.dispatch({ type: 'mergeItemStacks', sourceItemId: splitItem.id, targetItemId: 'item_duskgold_start' });

    const state = store.getState();
    const duskgoldStacks = Object.values(state.items).filter((item) => item.templateId === 'duskgold');
    expect(duskgoldStacks).toHaveLength(1);
    expect(state.items.item_duskgold_start.quantity).toBe(12);
    expect(state.items[splitItem.id]).toBeUndefined();
    expect(state.logs[0]?.text).toContain('Merged Duskgold stacks into 12.');
  });

  it('buys a vendor offer without trusting any client-side price payload', () => {
    const state = createInitialState();
    state.player.position = { x: -10, z: 8 };
    const store = new GameStore(state);

    store.dispatch({ type: 'buyVendorOffer', offerId: 'merchant_spear_offer', quantity: 1 });

    const inventorySpear = Object.values(store.getState().items).find(
      (item) => item.templateId === 'ironwood_spear' && item.container === 'inventory',
    );
    expect(inventorySpear).toBeDefined();
    expect(store.getState().items.item_duskgold_start.quantity).toBe(4);
    expect(store.getState().logs[0]?.text).toContain('Bought Ironwood Spear for 8 Duskgold.');
  });

  it('exchanges a fixed merchant offer without trusting any client-side material valuation', () => {
    const state = createInitialState();
    state.player.position = { x: -10, z: 8 };
    const store = new GameStore(state);

    store.dispatch({ type: 'exchangeVendorOffer', offerId: 'merchant_mantle_exchange', quantity: 1 });

    const inventoryMantle = Object.values(store.getState().items).find(
      (item) => item.templateId === 'wardkeeper_mantle' && item.container === 'inventory',
    );
    expect(inventoryMantle).toBeDefined();
    expect(store.getState().items.item_duskgold_start.quantity).toBe(2);
    expect(store.getState().logs[0]?.text).toContain('Exchanged 10 Duskgold for Wardkeeper Mantle.');
  });

  it('sells an inventory item to the nearby vendor with backend-owned valuation semantics', () => {
    const state = createInitialState();
    state.player.position = { x: -10, z: 8 };
    state.items.item_weapon_inventory = {
      id: 'item_weapon_inventory',
      templateId: 'ironwood_spear',
      quantity: 1,
      container: 'inventory',
    };
    const store = new GameStore(state);

    store.dispatch({ type: 'sellVendorItem', itemId: 'item_weapon_inventory', quantity: 1 });

    expect(store.getState().items.item_weapon_inventory).toBeUndefined();
    expect(store.getState().items.item_duskgold_start.quantity).toBe(16);
    expect(store.getState().logs[0]?.text).toContain('Sold Ironwood Spear for 4 Duskgold.');
  });

  it('moves stackable items into and out of the warehouse through container truth', () => {
    const state = createInitialState();
    state.player.position = { x: -13, z: 4 };
    const store = new GameStore(state);

    store.dispatch({ type: 'depositWarehouseItem', itemId: 'item_duskgold_start', quantity: 2 });

    const warehouseStack = Object.values(store.getState().items).find(
      (item) => item.templateId === 'duskgold' && item.container === 'warehouse',
    );
    expect(warehouseStack).toBeDefined();
    expect(store.getState().items.item_duskgold_start.quantity).toBe(10);

    if (!warehouseStack) {
      return;
    }

    store.dispatch({ type: 'withdrawWarehouseItem', itemId: warehouseStack.id, quantity: 1 });

    expect(store.getState().items.item_duskgold_start.quantity).toBe(11);
    const remainingWarehouseStack = Object.values(store.getState().items).find(
      (item) => item.templateId === 'duskgold' && item.container === 'warehouse',
    );
    expect(remainingWarehouseStack?.quantity).toBe(1);
    expect(store.getState().logs[0]?.text).toContain('Withdrew Duskgold x1 from the warehouse.');
  });
});
