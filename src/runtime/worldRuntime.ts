import {
  createInitialState,
  gameTemplates,
  getArchetypeIdForBaseClass,
  getLearnedSkillsForCharacter,
  normalizeHotbarState,
} from '../game/data/templates';
import { GameStore } from '../game/domain/game';
import type {
  AppearanceOptionIndex,
  BaseClass,
  CharacterRace,
  CharacterSex,
  EquipSlot,
  GameState,
  HotbarActionId,
  LootDrop,
  MobState,
  NpcState,
  OtherPlayerState,
  PlayerHotbarState,
} from '../game/domain/types';
import { localSaveAdapter } from '../game/platform/localSave';
import { Scene3D } from '../game/scene/scene3d';
import type { CharacterSummary, RegionContextMessage } from '../online/contracts';
import { Hud, type CharacterPanelId } from '../ui/hud';

declare global {
  interface Window {
    __mvpDebug?: {
      store: GameStore;
      scene: Scene3D;
    };
  }
}

type RuntimeMode = 'local' | 'online_authoritative';

const HOTBAR_KEY_BINDINGS = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '0', '-', '='];
const CHARACTER_PANEL_KEY_BINDINGS: Record<string, CharacterPanelId> = {
  t: 'status',
  k: 'skills',
  c: 'actions',
  n: 'clan',
  u: 'quests',
};

type WorldRuntimeOptions = {
  mode: RuntimeMode;
  initialState: GameState;
  onMoveIntent?: (point: { x: number; z: number }) => void;
  onSelectTarget?: (targetId: string) => void;
  onInteractNpc?: (npcId: string, actionId?: 'accept_task' | 'turn_in_task') => void;
  onCloseDialog?: () => void;
  onUseSkill?: (skillId: string) => void;
  onUseHotbarAction?: (actionId: HotbarActionId) => void;
  onUseItem?: (itemId: string) => void;
  onPickUpLoot?: (lootId: string) => void;
  onEquipItem?: (itemId: string) => void;
  onUnequipItem?: (slot: EquipSlot) => void;
  onSplitItemStack?: (itemId: string, quantity: number) => void;
  onMergeItemStacks?: (sourceItemId: string, targetItemId: string) => void;
  onBuyVendorOffer?: (offerId: string, quantity: number) => void;
  onExchangeVendorOffer?: (offerId: string, quantity: number) => void;
  onOfferTradeItem?: (targetCharacterId: string, itemId: string, quantity: number) => void;
  onAcceptTradeOffer?: (offerId: string) => void;
  onDeclineTradeOffer?: (offerId: string) => void;
  onInvitePartyMember?: (targetCharacterId: string) => void;
  onAcceptPartyInvite?: (inviteId: string) => void;
  onDeclinePartyInvite?: (inviteId: string) => void;
  onLeaveParty?: () => void;
  onKickPartyMember?: (targetCharacterId: string) => void;
  onSellVendorItem?: (itemId: string, quantity: number) => void;
  onDepositWarehouseItem?: (itemId: string, quantity: number) => void;
  onWithdrawWarehouseItem?: (itemId: string, quantity: number) => void;
  onHotbarChange?: (hotbar: PlayerHotbarState) => void;
  onSendChatMessage?: (
    channel: 'region' | 'party' | 'whisper',
    text: string,
    targetCharacterName?: string,
  ) => boolean;
  stateProvider?: () => GameState;
};

const NPC_TEMPLATES: Record<string, Pick<NpcState, 'name' | 'title'>> = {
  wardkeeper: {
    name: 'Selka',
    title: 'Wardkeeper of the Plaza',
  },
  merchant: {
    name: 'Ilya',
    title: 'Provisioner of the Plaza',
  },
  warehouse_keeper: {
    name: 'Rhea',
    title: 'Vaultkeeper of the Plaza',
  },
};

const CHARACTER_RACES: readonly CharacterRace[] = ['Human', 'Elf', 'Dark Elf', 'Orc', 'Dwarf'];
const CHARACTER_SEXES: readonly CharacterSex[] = ['Male', 'Female'];

const requireBaseClass = (value: unknown, field: string): BaseClass => {
  if (value === 'Fighter' || value === 'Mage') {
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

const createOtherPlayerState = (
  entity: RegionContextMessage['known_entities'][number],
): OtherPlayerState => {
  const baseClass = requireBaseClass(entity.state.base_class, 'other player base_class');
  return {
    baseClass,
    race: requireRace(entity.state.race, 'other player race'),
    sex: requireSex(entity.state.sex, 'other player sex'),
    hairStyle: requireAppearanceIndex(entity.state.hair_style, 'other player hair_style'),
    hairColor: requireAppearanceIndex(entity.state.hair_color, 'other player hair_color'),
    face: requireAppearanceIndex(entity.state.face, 'other player face'),
    id: entity.entity_id,
    name: typeof entity.state.name === 'string' ? entity.state.name : entity.entity_id,
    archetypeId: getArchetypeIdForBaseClass(baseClass),
    level: typeof entity.state.level === 'number' ? entity.state.level : 1,
    hp: typeof entity.state.hp === 'number' ? entity.state.hp : 1,
    dead: entity.state.dead === true,
    position: { ...entity.position },
    facing: typeof entity.state.facing === 'number' ? entity.state.facing : 0,
    mountedPetId: typeof entity.state.mounted_pet_id === 'string' ? entity.state.mounted_pet_id : null,
  };
};

const createOnlineBootstrapState = (
  regionContext: RegionContextMessage,
  character: CharacterSummary | null,
): GameState => {
  if (!character) {
    throw new Error('Online bootstrap requires an authoritative character summary.');
  }
  const state = createInitialState();
  const baseClass = character.base_class;
  state.player.id = character.character_id;
  state.player.name = character.name;
  state.player.race = character.race;
  state.player.baseClass = baseClass;
  state.player.sex = character.sex;
  state.player.hairStyle = requireAppearanceIndex(character.hair_style, 'character hair_style');
  state.player.hairColor = requireAppearanceIndex(character.hair_color, 'character hair_color');
  state.player.face = requireAppearanceIndex(character.face, 'character face');
  state.player.archetypeId = getArchetypeIdForBaseClass(baseClass);
  state.player.level = character.level;
  state.player.learnedSkills = getLearnedSkillsForCharacter(baseClass, state.player.level);
  state.player.hotbar = normalizeHotbarState(state.player.hotbar, baseClass);
  state.player.position = { ...regionContext.self_position };
  state.player.moveTarget = null;
  state.player.cast = null;
  state.player.cooldowns = {};
  state.destinationMarker = null;
  state.targetId = null;
  state.logs = [
    {
      id: 'log_online_attach',
      text: `Online attach confirmed. Region context loaded for ${regionContext.region_id}.`,
      tone: 'success',
    },
  ];
  state.quest = {
    id: 'online_bootstrap',
    title: 'Online Bootstrap',
    description: 'Fase 1.1 is attached. Gameplay authority remains server-owned.',
    status: 'completed',
    progress: 1,
    goal: 1,
  };
  state.dialog = null;
  state.floatingTexts = [];
  state.otherPlayers = {};
  state.mobs = {};
  state.loot = {};
  state.npcs = {};

  for (const entity of regionContext.known_entities) {
    if (entity.entity_type === 'npc') {
      const fallback = NPC_TEMPLATES[entity.template_id] ?? {
        name: entity.template_id,
        title: 'NPC',
      };
      state.npcs[entity.entity_id] = {
        id: entity.entity_id,
        name: fallback.name,
        title: fallback.title,
        position: { ...entity.position },
      };
      continue;
    }

    if (entity.entity_type === 'mob') {
      const hp = typeof entity.state.hp === 'number' ? entity.state.hp : 1;
      state.mobs[entity.entity_id] = {
        id: entity.entity_id,
        templateId: entity.template_id,
        position: { ...entity.position },
        spawnPoint: { ...entity.position },
        hp,
        aiState: entity.state.alive === false ? 'dead' : 'idle',
        attackCooldownMs: 0,
        respawnAtMs: null,
      } satisfies MobState;
      continue;
    }

    if (entity.entity_type === 'player') {
      state.otherPlayers[entity.entity_id] = createOtherPlayerState(entity);
      continue;
    }

    const lootDrop: LootDrop = {
      id: entity.entity_id,
      itemInstanceId: `online_stub_item_${entity.entity_id}`,
      position: { ...entity.position },
      label: entity.template_id,
    };
    state.loot[entity.entity_id] = lootDrop;
  }

  return state;
};

export class WorldRuntime {
  private readonly shell: HTMLDivElement;
  private readonly store: GameStore;
  private readonly scene: Scene3D;
  private readonly hud: Hud;
  private readonly mode: RuntimeMode;
  private readonly onInteractNpc?: (npcId: string, actionId?: 'accept_task' | 'turn_in_task') => void;
  private readonly onCloseDialog?: () => void;
  private readonly onUseSkill?: (skillId: string) => void;
  private readonly onUseHotbarAction?: (actionId: HotbarActionId) => void;
  private readonly onUseItem?: (itemId: string) => void;
  private readonly onEquipItem?: (itemId: string) => void;
  private readonly stateProvider?: () => GameState;
  private frameHandle = 0;
  private lastFrame = performance.now();
  private readonly handleKeyDownBound = this.handleKeyDown.bind(this);
  private readonly handleBeforeUnloadBound = this.handleBeforeUnload.bind(this);
  private readonly handleContextMenuBound = this.handleContextMenu.bind(this);

  constructor(container: HTMLElement, options: WorldRuntimeOptions) {
    this.mode = options.mode;
    this.onInteractNpc = options.onInteractNpc;
    this.onCloseDialog = options.onCloseDialog;
    this.onUseSkill = options.onUseSkill;
    this.onUseHotbarAction = options.onUseHotbarAction;
    this.onUseItem = options.onUseItem;
    this.onEquipItem = options.onEquipItem;
    this.stateProvider = options.stateProvider;
    this.shell = document.createElement('div');
    this.shell.className = 'game-shell';
    container.replaceChildren(this.shell);

    this.store = new GameStore(options.initialState);
    this.scene = new Scene3D(this.shell, this.store, {
      interactive: true,
      onMoveIntent: options.onMoveIntent,
      onSelectTarget: options.onSelectTarget,
      onInteractNpc: options.onInteractNpc,
      onPickUpLoot: options.onPickUpLoot,
    });
    this.hud = new Hud(
      this.shell,
      this.store,
      {
        save: () => {
          localSaveAdapter.save(this.store.getState());
        },
        load: () => {
          const loaded = localSaveAdapter.load();
          if (loaded) {
            this.store.replaceState(loaded);
          }
        },
        reset: () => {
          localSaveAdapter.clear();
          this.store.replaceState(createInitialState());
        },
      },
      {
        interactive: this.mode === 'local',
        showPersistenceControls: this.mode === 'local',
        modeLabel: this.mode === 'local' ? 'Local Prototype' : 'Online Authoritative',
        onInteractNpc: options.onInteractNpc,
        onCloseDialog: options.onCloseDialog,
        onUseSkill: options.onUseSkill,
        onUseHotbarAction: (actionId) => this.activateHotbarAction(actionId),
        onUseItem: options.onUseItem,
        onEquipItem: options.onEquipItem,
        onUnequipItem: options.onUnequipItem,
        onSplitItemStack: options.onSplitItemStack,
        onMergeItemStacks: options.onMergeItemStacks,
        onBuyVendorOffer: options.onBuyVendorOffer,
        onExchangeVendorOffer: options.onExchangeVendorOffer,
        onOfferTradeItem: options.onOfferTradeItem,
        onAcceptTradeOffer: options.onAcceptTradeOffer,
        onDeclineTradeOffer: options.onDeclineTradeOffer,
        onInvitePartyMember: options.onInvitePartyMember,
        onAcceptPartyInvite: options.onAcceptPartyInvite,
        onDeclinePartyInvite: options.onDeclinePartyInvite,
        onLeaveParty: options.onLeaveParty,
        onKickPartyMember: options.onKickPartyMember,
        onSellVendorItem: options.onSellVendorItem,
        onDepositWarehouseItem: options.onDepositWarehouseItem,
        onWithdrawWarehouseItem: options.onWithdrawWarehouseItem,
        onHotbarChange: options.onHotbarChange,
        onSendChatMessage: options.onSendChatMessage,
      },
    );

    window.__mvpDebug = { store: this.store, scene: this.scene };
    this.hud.update(this.store.getState());
    this.shell.addEventListener('contextmenu', this.handleContextMenuBound);
    if (
      this.mode === 'local' ||
      this.onUseSkill ||
      this.onUseHotbarAction ||
      this.onUseItem ||
      this.onCloseDialog ||
      options.onSendChatMessage
    ) {
      window.addEventListener('keydown', this.handleKeyDownBound);
    }
    if (this.mode === 'local') {
      window.addEventListener('beforeunload', this.handleBeforeUnloadBound);
    }
    this.start();
  }

  static fromLocalSave(container: HTMLElement): WorldRuntime {
    return new WorldRuntime(container, {
      mode: 'local',
      initialState: localSaveAdapter.load() ?? createInitialState(),
    });
  }

  static fromRegionContext(
    container: HTMLElement,
    regionContext: RegionContextMessage,
    character: CharacterSummary | null,
  ): WorldRuntime {
    return new WorldRuntime(container, {
      mode: 'online_authoritative',
      initialState: createOnlineBootstrapState(regionContext, character),
    });
  }

  static fromOnlineAuthoritative(
    container: HTMLElement,
    initialState: GameState,
    handlers: {
      onMoveIntent: (point: { x: number; z: number }) => void;
      onSelectTarget: (targetId: string) => void;
      onInteractNpc: (npcId: string, actionId?: 'accept_task' | 'turn_in_task') => void;
      onCloseDialog: () => void;
      onUseSkill: (skillId: string) => void;
      onUseHotbarAction: (actionId: HotbarActionId) => void;
      onUseItem: (itemId: string) => void;
      onPickUpLoot: (lootId: string) => void;
      onEquipItem: (itemId: string) => void;
      onUnequipItem: (slot: EquipSlot) => void;
      onSplitItemStack: (itemId: string, quantity: number) => void;
      onMergeItemStacks: (sourceItemId: string, targetItemId: string) => void;
      onBuyVendorOffer: (offerId: string, quantity: number) => void;
      onExchangeVendorOffer: (offerId: string, quantity: number) => void;
      onOfferTradeItem: (targetCharacterId: string, itemId: string, quantity: number) => void;
      onAcceptTradeOffer: (offerId: string) => void;
      onDeclineTradeOffer: (offerId: string) => void;
      onInvitePartyMember: (targetCharacterId: string) => void;
      onAcceptPartyInvite: (inviteId: string) => void;
      onDeclinePartyInvite: (inviteId: string) => void;
      onLeaveParty: () => void;
      onKickPartyMember: (targetCharacterId: string) => void;
      onSellVendorItem: (itemId: string, quantity: number) => void;
      onDepositWarehouseItem: (itemId: string, quantity: number) => void;
      onWithdrawWarehouseItem: (itemId: string, quantity: number) => void;
      onHotbarChange: (hotbar: PlayerHotbarState) => void;
      onSendChatMessage: (
        channel: 'region' | 'party' | 'whisper',
        text: string,
        targetCharacterName?: string,
      ) => boolean;
      stateProvider: () => GameState;
    },
  ): WorldRuntime {
    return new WorldRuntime(container, {
      mode: 'online_authoritative',
      initialState,
      onMoveIntent: handlers.onMoveIntent,
      onSelectTarget: handlers.onSelectTarget,
      onInteractNpc: handlers.onInteractNpc,
      onCloseDialog: handlers.onCloseDialog,
      onUseSkill: handlers.onUseSkill,
      onUseHotbarAction: handlers.onUseHotbarAction,
      onUseItem: handlers.onUseItem,
      onPickUpLoot: handlers.onPickUpLoot,
      onEquipItem: handlers.onEquipItem,
      onUnequipItem: handlers.onUnequipItem,
      onSplitItemStack: handlers.onSplitItemStack,
      onMergeItemStacks: handlers.onMergeItemStacks,
      onBuyVendorOffer: handlers.onBuyVendorOffer,
      onExchangeVendorOffer: handlers.onExchangeVendorOffer,
      onOfferTradeItem: handlers.onOfferTradeItem,
      onAcceptTradeOffer: handlers.onAcceptTradeOffer,
      onDeclineTradeOffer: handlers.onDeclineTradeOffer,
      onInvitePartyMember: handlers.onInvitePartyMember,
      onAcceptPartyInvite: handlers.onAcceptPartyInvite,
      onDeclinePartyInvite: handlers.onDeclinePartyInvite,
      onLeaveParty: handlers.onLeaveParty,
      onKickPartyMember: handlers.onKickPartyMember,
      onSellVendorItem: handlers.onSellVendorItem,
      onDepositWarehouseItem: handlers.onDepositWarehouseItem,
      onWithdrawWarehouseItem: handlers.onWithdrawWarehouseItem,
      onHotbarChange: handlers.onHotbarChange,
      onSendChatMessage: handlers.onSendChatMessage,
      stateProvider: handlers.stateProvider,
    });
  }

  replaceState(state: GameState): void {
    this.store.replaceState(state);
    this.hud.update(this.store.getState());
  }

  private start(): void {
    const frame = (now: number): void => {
      const deltaMs = Math.min(now - this.lastFrame, 32);
      this.lastFrame = now;

      if (this.mode === 'local') {
        this.store.tick(deltaMs);
        const state = this.store.getState();
        if (state.timeMs - state.lastAutoSaveAtMs > 10000) {
          localSaveAdapter.save(state);
          state.lastAutoSaveAtMs = state.timeMs;
        }
      } else if (this.stateProvider) {
        this.store.replaceState(this.stateProvider());
      }

      const state = this.store.getState();
      this.scene.update(state);
      this.scene.render();
      this.hud.update(state);
      this.frameHandle = requestAnimationFrame(frame);
    };

    this.frameHandle = requestAnimationFrame(frame);
  }

  private handleKeyDown(event: KeyboardEvent): void {
    if (event.key === 'Enter') {
      if (this.hud.isChatInputFocused()) {
        event.preventDefault();
        this.hud.submitChatInput();
        return;
      }
      event.preventDefault();
      this.hud.focusChatInput();
      return;
    }

    if (event.key === 'Escape' && this.hud.isChatInputFocused()) {
      event.preventDefault();
      this.hud.cancelChatInput();
      return;
    }

    if (this.hud.isChatInputFocused()) {
      return;
    }

    if (event.altKey) {
      const key = event.key.toLowerCase();
      const characterPanelId = CHARACTER_PANEL_KEY_BINDINGS[key];
      if (characterPanelId) {
        event.preventDefault();
        this.hud.toggleCharacterPanel(characterPanelId);
        return;
      }
    }

    if (event.altKey && event.key.toLowerCase() === 'v') {
      event.preventDefault();
      this.hud.toggleInventory();
      return;
    }

    if (event.altKey && event.key.toLowerCase() === 'p') {
      event.preventDefault();
      this.hud.togglePartyPanel();
      return;
    }

    const hotbarIndex = HOTBAR_KEY_BINDINGS.indexOf(event.key);
    if (hotbarIndex >= 0) {
      const state = this.store.getState();
      const slot = state.player.hotbar.slots.find((entry) => entry.slotIndex === hotbarIndex);
      if (!slot?.entryType) {
        return;
      }
      if (slot.entryType === 'skill' && slot.skillId) {
        if (this.mode === 'local') {
          this.store.dispatch({ type: 'useSkill', skillId: slot.skillId });
        } else {
          this.onUseSkill?.(slot.skillId);
        }
        return;
      }
      if (slot.entryType === 'item' && slot.itemId) {
        const item = state.items[slot.itemId];
        const itemTemplate = item ? gameTemplates.itemTemplates[item.templateId] : null;
        if (!itemTemplate) {
          return;
        }
        if (itemTemplate.kind === 'consumable') {
          if (this.mode === 'local') {
            this.store.dispatch({ type: 'useItem', itemId: slot.itemId });
          } else {
            this.onUseItem?.(slot.itemId);
          }
          return;
        }
        if (this.mode === 'local') {
          this.store.dispatch({ type: 'equipItem', itemId: slot.itemId });
        } else {
          this.onEquipItem?.(slot.itemId);
        }
        return;
      }
      if (slot.entryType === 'action' && slot.actionId) {
        this.activateHotbarAction(slot.actionId);
      }
      return;
    }

    if (event.key === 'Escape' && this.store.getState().dialog) {
      event.preventDefault();
      if (this.mode === 'local') {
        this.store.dispatch({ type: 'closeDialog' });
      } else {
        this.onCloseDialog?.();
      }
      return;
    }

    if (this.mode !== 'local') {
      return;
    }

    if (event.key === 'Tab') {
      event.preventDefault();
      this.cycleTarget();
      return;
    }
  }

  private handleBeforeUnload(): void {
    if (this.mode !== 'local') {
      return;
    }
    localSaveAdapter.save(this.store.getState());
    this.scene.destroy();
  }

  private handleContextMenu(event: MouseEvent): void {
    event.preventDefault();
  }

  private activateHotbarAction(actionId: HotbarActionId): void {
    if (this.mode === 'local') {
      if (actionId === 'basic_attack') {
        this.store.dispatch({ type: 'basicAttack' });
        return;
      }
      if (actionId === 'pick_up_nearby') {
        this.store.dispatch({ type: 'pickUpNearbyLoot' });
      }
      return;
    }

    this.onUseHotbarAction?.(actionId);
  }

  private cycleTarget(): void {
    const state = this.store.getState();
    const living = Object.values(state.mobs)
      .filter((mob) => mob.aiState !== 'dead')
      .sort((left, right) => {
        const leftDistance = Math.hypot(left.position.x - state.player.position.x, left.position.z - state.player.position.z);
        const rightDistance = Math.hypot(
          right.position.x - state.player.position.x,
          right.position.z - state.player.position.z,
        );
        return leftDistance - rightDistance;
      });

    if (living.length === 0) {
      return;
    }

    const currentIndex = living.findIndex((mob) => mob.id === state.targetId);
    const next = living[(currentIndex + 1 + living.length) % living.length];
    this.store.dispatch({ type: 'selectTarget', targetId: next.id });
  }

  destroy(): void {
    cancelAnimationFrame(this.frameHandle);
    this.shell.removeEventListener('contextmenu', this.handleContextMenuBound);
    if (
      this.mode === 'local' ||
      this.onUseSkill ||
      this.onUseHotbarAction ||
      this.onUseItem ||
      this.onCloseDialog ||
      this.hud
    ) {
      window.removeEventListener('keydown', this.handleKeyDownBound);
    }
    if (this.mode === 'local') {
      window.removeEventListener('beforeunload', this.handleBeforeUnloadBound);
    }
    this.scene.destroy();
  }
}
