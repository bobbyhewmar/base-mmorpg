import type { BaseClass, CharacterRace, CharacterSex, CharacterSummary, CreateCharacterRequest } from '../online/contracts';
import { getBaseClassCreationLabel, getBaseClassDefinition } from '../game/data/characterClasses';
import { gameTemplates, getArchetypeIdForBaseClass } from '../game/data/templates';
import { ApiClientError, GameplaySessionClient, OnlineApiClient } from '../online/client';
import { preGameReducer, initialPreGameContext, type PreGameContext } from './preGameMachine';
import { WorldRuntime } from '../runtime/worldRuntime';
import { normalizeCanonicalHairColor, resolveCharacterCreationOptions } from './characterCreationOptions';
import { resolveLoginFailureEvent, resolveRegisterSuccessEvent } from './authFlow';
import { OnlineReadModel } from '../online/readModel';
import type { EquipSlot, GameState, HotbarActionId, PlayerHotbarState } from '../game/domain/types';
import { CharacterLobbyScene } from './characterLobbyScene';
import { CharacterCreationScene } from './characterCreationScene';

const API_BASE_URL = import.meta.env.VITE_L2BG_API_BASE_URL ?? 'http://localhost:8080';

type FormValues = Record<string, FormDataEntryValue>;

declare global {
  interface Window {
    __l2bgE2E?: {
      getSnapshot: () => {
        mode: PreGameContext['mode'];
        phase: PreGameContext['phase'];
        error: string | null;
        selectedCharacterId: string | null;
        runtimeMounted: boolean;
        onlineState: ReturnType<OnlineReadModel['getStateInfo']> | null;
      };
      getWorldState: () => GameState | null;
      sendMoveIntent: (point: { x: number; z: number }) => void;
      sendSelectTarget: (targetId: string) => void;
      sendClearTarget: () => void;
      sendInteractNpc: (npcId: string, actionId?: 'accept_task' | 'turn_in_task') => void;
      sendUseSkill: (skillId: string) => void;
      sendBasicAttack: () => void;
      sendPickUpLoot: (lootId: string) => void;
      sendUseItem: (itemId: string) => void;
      sendEquipItem: (itemId: string) => void;
      sendUnequipItem: (slot: EquipSlot) => void;
      sendSplitItemStack: (itemId: string, quantity: number) => void;
      sendMergeItemStacks: (sourceItemId: string, targetItemId: string) => void;
      sendBuyItem: (offerId: string, quantity: number) => void;
      sendExchangeItem: (offerId: string, quantity: number) => void;
      sendOfferTradeItem: (targetCharacterId: string, itemId: string, quantity: number) => void;
      sendAcceptTradeOffer: (offerId: string) => void;
      sendDeclineTradeOffer: (offerId: string) => void;
      sendInvitePartyMember: (targetCharacterId?: string) => void;
      sendAcceptPartyInvite: (inviteId: string) => void;
      sendDeclinePartyInvite: (inviteId: string) => void;
      sendLeaveParty: () => void;
      sendKickPartyMember: (targetCharacterId: string) => void;
      sendCreateClan: (name: string) => void;
      sendInviteClanMember: () => void;
      sendAcceptClanInvite: (inviteId: string) => void;
      sendDeclineClanInvite: (inviteId: string) => void;
      sendLeaveClan: () => void;
      sendKickClanMember: (targetCharacterId: string) => void;
      sendDissolveClan: () => void;
      sendCreateAlliance: (name: string) => void;
      sendInviteAllianceClan: () => void;
      sendAcceptAllianceInvite: (inviteId: string) => void;
      sendDeclineAllianceInvite: (inviteId: string) => void;
      sendLeaveAlliance: () => void;
      sendExpelAllianceClan: (targetClanId: string) => void;
      sendDissolveAlliance: () => void;
      sendChatMessage: (
        channel: 'region' | 'party' | 'alliance' | 'whisper',
        text: string,
        targetCharacterName?: string,
      ) => void;
      sendSellItem: (itemId: string, quantity: number) => void;
      sendDepositItem: (itemId: string, quantity: number) => void;
      sendWithdrawItem: (itemId: string, quantity: number) => void;
    };
  }
}

const readFormValues = (form: HTMLFormElement): FormValues => Object.fromEntries(new FormData(form).entries());

const escapeHtml = (value: string): string =>
  value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');

type AuthFieldConfig = {
  label: string;
  name: string;
  type: 'text' | 'password';
  autocomplete: string;
};

type GameButtonConfig = {
  label: string;
  type?: 'submit' | 'button';
  clickAction?: string;
  disabled?: boolean;
};

type GameWorldConfig = {
  id: string;
  no: string;
  name: string;
  traffic: 'Light' | 'Normal' | 'Heavy';
  ping: number;
  type: string;
};

const GAME_WORLDS: readonly GameWorldConfig[] = [
  {
    id: 'detona-500x',
    no: '01',
    name: 'Warriors of Bartz',
    traffic: 'Light',
    ping: 0,
    type: '',
  },
];

type CreationCycleOption = 'race' | 'base_class' | 'sex' | 'hair_style' | 'skin_type';

type CreationFieldConfig = {
  label: string;
  value: string;
  option: CreationCycleOption;
  enabled: boolean;
};

type AuthScreenConfig = {
  screenLabel: string;
  action: 'login' | 'register';
  variant: 'login' | 'register';
  title: string;
  error: string;
  fields: AuthFieldConfig[];
  actions: GameButtonConfig[];
  sideMenuActive: 'login' | 'register';
};

export class ClientApp {
  private state = initialPreGameContext();
  private readonly api = new OnlineApiClient(API_BASE_URL);
  private readonly root: HTMLDivElement;
  private readonly statusBar: HTMLDivElement;
  private readonly content: HTMLDivElement;
  private runtime: WorldRuntime | null = null;
  private sessionClient: GameplaySessionClient | null = null;
  private onlineReadModel: OnlineReadModel | null = null;
  private onlineSessionEpoch = 0;
  private characterLobbyScene: CharacterLobbyScene | null = null;
  private characterCreationScene: CharacterCreationScene | null = null;
  private createNameDraft = '';

  constructor(container: HTMLDivElement) {
    this.root = container;
    this.root.className = 'client-app';
    this.statusBar = document.createElement('div');
    this.statusBar.className = 'app-status';
    this.content = document.createElement('div');
    this.content.className = 'app-content';
    this.root.replaceChildren(this.statusBar, this.content);
    this.content.addEventListener('submit', this.handleSubmit.bind(this));
    this.root.addEventListener('click', this.handleClick.bind(this));
    this.content.addEventListener('change', this.handleChange.bind(this));
    this.content.addEventListener('input', this.handleInput.bind(this));
    this.installE2EBridge();
    this.render();
  }

  private setState(next: PreGameContext): void {
    this.state = next;
    this.render();
  }

  private transition(event: Parameters<typeof preGameReducer>[1]): void {
    this.setState(preGameReducer(this.state, event));
  }

  private async handleSubmit(event: Event): Promise<void> {
    const target = event.target;
    if (!(target instanceof HTMLFormElement)) {
      return;
    }

    event.preventDefault();
    const action = target.dataset.action;
    const values = readFormValues(target);

    if (action === 'register') {
      this.transition({ type: 'auth_pending' });
      try {
        const loginValue = String(values.login ?? '');
        const response = await this.api.register({
          login: loginValue,
          password: String(values.password ?? ''),
          display_name: String(values.display_name ?? ''),
        });
        this.transition(resolveRegisterSuccessEvent(response, loginValue));
      } catch (error) {
        this.transition({ type: 'auth_failed', message: this.toUserMessage(error) });
      }
      return;
    }

    if (action === 'login') {
      this.transition({ type: 'auth_pending' });
      try {
        const login = await this.api.login({
          login: String(values.login ?? ''),
          password: String(values.password ?? ''),
        });
        const [characters, catalog] = await Promise.all([
          this.api.getCharacters(login.access_token),
          this.api.getCatalog(login.access_token),
        ]);
        this.transition({
          type: 'auth_succeeded',
          accessToken: login.access_token,
          accountId: login.account_id,
          characters: characters.characters,
          catalog,
        });
      } catch (error) {
        this.transition(
          resolveLoginFailureEvent(error, String(values.login ?? ''), this.toUserMessage(error)),
        );
      }
      return;
    }

    if (action === 'create-character') {
      if (!this.state.accessToken) {
        this.transition({ type: 'operation_failed', message: 'You must log in before creating a character.' });
        return;
      }

      try {
        const race = String(values.race ?? '');
        const baseClass = String(values.base_class ?? '');
        const sex = String(values.sex ?? '');
        const hairStyleValue = String(values.hair_style ?? '');
        const hairColor = normalizeCanonicalHairColor(String(values.hair_color ?? ''));
        const skinTypeValue = String(values.skin_type ?? '');
        const hairStyle = Number(hairStyleValue);
        const skinType = Number(skinTypeValue);
        if (
          !race ||
          !baseClass ||
          !sex ||
          hairStyleValue === '' ||
          !hairColor ||
          skinTypeValue === '' ||
          !Number.isInteger(hairStyle) ||
          !Number.isInteger(skinType)
        ) {
          this.transition({
            type: 'operation_failed',
            message: 'Select race, class, gender, hairstyle, hair color, and skin type before creating a character.',
          });
          return;
        }
        const request: CreateCharacterRequest = {
          race: race as CreateCharacterRequest['race'],
          base_class: baseClass as CreateCharacterRequest['base_class'],
          sex: sex as CreateCharacterRequest['sex'],
          hair_style: hairStyle,
          hair_color: hairColor,
          skin_type: skinType,
          name: String(values.name ?? ''),
        };
        const response = await this.api.createCharacter(this.state.accessToken, request);
        this.createNameDraft = '';
        this.transition({ type: 'characters_updated', characters: response.characters });
      } catch (error) {
        this.transition({ type: 'operation_failed', message: this.toUserMessage(error) });
      }
    }
  }

  private handleChange(event: Event): void {
    const target = event.target;
    if (!(target instanceof HTMLSelectElement)) {
      return;
    }

    if (target.name === 'race') {
      this.transition({ type: 'set_create_race', race: target.value as CharacterSummary['race'] });
      return;
    }

    if (target.name === 'character_lobby_select' && target.value) {
      this.transition({ type: 'select_character', characterId: target.value });
    }
  }

  private handleInput(event: Event): void {
    const target = event.target;
    if (!(target instanceof HTMLInputElement)) {
      return;
    }
    if (target.name === 'name' && target.closest('form[data-action="create-character"]')) {
      this.createNameDraft = target.value;
      const form = target.closest<HTMLFormElement>('form[data-action="create-character"]');
      if (form) {
        this.syncCharacterCreateSubmitState(form);
      }
      return;
    }
    if (target.name === 'hair_color' && target.closest('form[data-action="create-character"]')) {
      const hairColor = normalizeCanonicalHairColor(target.value);
      if (hairColor) {
        this.transition({ type: 'set_create_hair_color', hairColor });
      }
    }
  }

  private syncCharacterCreateSubmitState(form: HTMLFormElement): void {
    const submitButton = form.querySelector<HTMLButtonElement>('button[type="submit"]');
    if (!submitButton) {
      return;
    }
    const options = resolveCharacterCreationOptions(
      this.state.catalog,
      this.state.createRace,
      this.state.createBaseClass,
      this.state.createSex,
      this.state.createHairStyle,
      this.state.createSkinType,
      this.state.createHairColor,
    );
    const canCreate = Boolean(
      this.createNameDraft.trim() &&
        options.selectedRace &&
        options.selectedBaseClass &&
        options.selectedSex &&
        options.selectedHairStyle !== null &&
        options.selectedHairColor !== null &&
        options.selectedSkinType !== null,
    );
    submitButton.disabled = !canCreate;
    if (canCreate) {
      submitButton.removeAttribute('aria-disabled');
      return;
    }
    submitButton.setAttribute('aria-disabled', 'true');
  }

  private async handleClick(event: MouseEvent): Promise<void> {
    const target = event.target as HTMLElement | null;
    if (!target) {
      return;
    }

    const actionElement = target.closest<HTMLElement>('[data-click-action]');
    if (!actionElement) {
      return;
    }
    const action = actionElement.dataset.clickAction;
    if (!action) {
      return;
    }

    switch (action) {
      case 'choose-local':
        this.transition({ type: 'choose_local' });
        this.mountLocalRuntime();
        return;
      case 'choose-online':
        this.runtime?.destroy();
        this.runtime = null;
        this.onlineReadModel = null;
        this.transition({ type: 'choose_online' });
        return;
      case 'open-login':
        this.transition({ type: 'open_login' });
        return;
      case 'open-register':
        this.transition({ type: 'open_register' });
        return;
      case 'open-recovery':
        this.transition({ type: 'open_recovery' });
        return;
      case 'accept-eula':
        this.transition({ type: 'accept_eula' });
        return;
      case 'reject-eula':
        this.transition({ type: 'reject_eula' });
        return;
      case 'select-world': {
        const worldId = actionElement.dataset.worldId;
        if (worldId) {
          this.transition({ type: 'select_world', worldId });
        }
        return;
      }
      case 'confirm-world-selection':
        this.transition({ type: 'confirm_world_selection' });
        return;
      case 'cancel-world-selection':
        this.transition({ type: 'open_login' });
        return;
      case 'open-create-character':
        this.createNameDraft = '';
        this.transition({ type: 'open_create_character' });
        return;
      case 'cycle-create-option':
        this.cycleCreateOption(
          actionElement.dataset.creationOption as CreationCycleOption,
          Number(actionElement.dataset.direction ?? 1),
        );
        return;
      case 'back-to-characters':
        this.transition({ type: 'characters_updated', characters: this.state.characters });
        return;
      case 'reset-online':
        this.resetOnlineSessionState();
        this.transition({ type: 'reset_online' });
        return;
      case 'select-character': {
        const characterId = target.closest<HTMLElement>('[data-character-id]')?.dataset.characterId;
        if (characterId) {
          this.transition({ type: 'select_character', characterId });
        }
        return;
      }
      case 'enter-world':
        await this.enterSelectedCharacter();
        return;
      default:
        return;
    }
  }

  private cycleCreateOption(option: CreationCycleOption, direction: number): void {
    const step = direction >= 0 ? 1 : -1;
    const options = resolveCharacterCreationOptions(
      this.state.catalog,
      this.state.createRace,
      this.state.createBaseClass,
      this.state.createSex,
      this.state.createHairStyle,
      this.state.createSkinType,
      this.state.createHairColor,
    );

    if (option === 'race') {
      const race = this.cycleValue(options.raceOptions.map((entry) => entry.race), options.selectedRace, step);
      if (race) {
        this.transition({ type: 'set_create_race', race });
      }
      return;
    }

    if (option === 'base_class') {
      const baseClass = this.cycleValue(options.baseClassOptions, options.selectedBaseClass, step);
      if (baseClass) {
        this.transition({ type: 'set_create_base_class', baseClass });
      }
      return;
    }

    if (option === 'sex') {
      const sex = this.cycleValue(options.sexOptions, options.selectedSex, step);
      if (sex) {
        this.transition({ type: 'set_create_sex', sex });
      }
      return;
    }

    const optionValues =
      option === 'hair_style'
        ? options.hairStyleOptions
        : options.skinTypeOptions;
    const current =
      option === 'hair_style'
        ? options.selectedHairStyle
        : options.selectedSkinType;
    const value = this.cycleValue(optionValues, current, step);
    if (value === null) {
      return;
    }
    this.transition({ type: 'set_create_appearance', field: option, value });
  }

  private cycleValue<T>(values: T[], current: T | null, step: number): T | null {
    if (values.length === 0) {
      return null;
    }
    const currentIndex = current === null ? -1 : values.findIndex((value) => value === current);
    if (currentIndex === -1) {
      return step >= 0 ? values[0] : values[values.length - 1];
    }
    return values[(currentIndex + step + values.length) % values.length];
  }

  private async enterSelectedCharacter(): Promise<void> {
    if (!this.state.accessToken || !this.state.selectedCharacterId) {
      this.transition({ type: 'operation_failed', message: 'Select a character before entering the world.' });
      return;
    }

    let sessionClient: GameplaySessionClient | null = null;
    let sessionEpoch: number | null = null;
    this.transition({ type: 'enter_world_pending' });
    try {
      const worldEnter = await this.api.enterWorld(this.state.accessToken, {
        character_id: this.state.selectedCharacterId,
      });
      this.transition({
        type: 'enter_world_succeeded',
        characterId: this.state.selectedCharacterId,
        bootstrap: {
          sessionId: worldEnter.session_id,
          attachToken: worldEnter.attach_token,
          wsUrl: worldEnter.ws_url,
        },
      });

      const wsUrl = worldEnter.ws_url.startsWith('http')
        ? worldEnter.ws_url.replace('http://', 'ws://').replace('https://', 'wss://')
        : worldEnter.ws_url;
      sessionEpoch = ++this.onlineSessionEpoch;
      const previousClient = this.sessionClient;
      this.sessionClient = null;
      previousClient?.close();
      this.clearMountedOnlineState();
      sessionClient = new GameplaySessionClient(wsUrl);
      this.sessionClient = sessionClient;
      sessionClient.setMessageHandler((message) => {
        this.handleOnlineMessage(sessionClient as GameplaySessionClient, sessionEpoch as number, message);
      });
      sessionClient.setCloseHandler(() => {
        this.handleOnlineClose(sessionClient as GameplaySessionClient, sessionEpoch as number);
      });
      this.transition({ type: 'attach_pending' });
      const regionContext = await sessionClient.attachSession(worldEnter.session_id, worldEnter.attach_token);
      if (this.sessionClient !== sessionClient || this.onlineSessionEpoch !== sessionEpoch) {
        sessionClient.close();
        return;
      }
      this.onlineReadModel = new OnlineReadModel(
        regionContext,
        this.selectedCharacter(),
        worldEnter.item_state,
        worldEnter.self_state,
      );
      this.transition({ type: 'attach_succeeded', regionContext });
      this.mountOnlineRuntime();
    } catch (error) {
      if (sessionClient && sessionEpoch !== null && (this.sessionClient !== sessionClient || this.onlineSessionEpoch !== sessionEpoch)) {
        return;
      }
      if (this.sessionClient) {
        this.sessionClient.close();
        this.sessionClient = null;
      }
      this.clearMountedOnlineState();
      this.transition({ type: 'operation_failed', message: this.toUserMessage(error) });
    }
  }

  private mountLocalRuntime(): void {
    this.resetOnlineSessionState();
    this.runtime = WorldRuntime.fromLocalSave(this.content);
    this.renderStatus();
  }

  private mountOnlineRuntime(): void {
    if (!this.onlineReadModel) {
      return;
    }
    const snapshot = this.onlineReadModel.snapshot;
    this.runtime?.destroy();
    this.runtime = WorldRuntime.fromOnlineAuthoritative(this.content, snapshot, {
      onMoveIntent: (point) => {
        this.sendMoveIntent(point);
      },
      onSelectTarget: (targetId) => {
        this.sendSelectTarget(targetId);
      },
      onClearTarget: () => {
        this.sendClearTarget();
      },
      onInteractNpc: (npcId, actionId) => {
        this.sendInteractNpc(npcId, actionId);
      },
      onCloseDialog: () => {
        this.dismissOnlineDialog();
      },
      onUseSkill: (skillId) => {
        this.sendUseSkill(skillId);
      },
      onUseHotbarAction: (actionId) => {
        this.sendHotbarAction(actionId);
      },
      onUseItem: (itemId) => {
        this.sendUseItem(itemId);
      },
      onPickUpLoot: (lootId) => {
        this.sendPickUpLoot(lootId);
      },
      onEquipItem: (itemId) => {
        this.sendEquipItem(itemId);
      },
      onUnequipItem: (slot) => {
        this.sendUnequipItem(slot);
      },
      onSplitItemStack: (itemId, quantity) => {
        this.sendSplitItemStack(itemId, quantity);
      },
      onMergeItemStacks: (sourceItemId, targetItemId) => {
        this.sendMergeItemStacks(sourceItemId, targetItemId);
      },
      onBuyVendorOffer: (offerId, quantity) => {
        this.sendBuyItem(offerId, quantity);
      },
      onExchangeVendorOffer: (offerId, quantity) => {
        this.sendExchangeItem(offerId, quantity);
      },
      onOfferTradeItem: (targetCharacterId, itemId, quantity) => {
        this.sendOfferTradeItem(targetCharacterId, itemId, quantity);
      },
      onAcceptTradeOffer: (offerId) => {
        this.sendAcceptTradeOffer(offerId);
      },
      onDeclineTradeOffer: (offerId) => {
        this.sendDeclineTradeOffer(offerId);
      },
      onInvitePartyMember: () => {
        this.sendInvitePartyMember();
      },
      onAcceptPartyInvite: (inviteId) => {
        this.sendAcceptPartyInvite(inviteId);
      },
      onDeclinePartyInvite: (inviteId) => {
        this.sendDeclinePartyInvite(inviteId);
      },
      onLeaveParty: () => {
        this.sendLeaveParty();
      },
      onKickPartyMember: (targetCharacterId) => {
        this.sendKickPartyMember(targetCharacterId);
      },
      onCreateClan: (name) => {
        this.sendCreateClan(name);
      },
      onInviteClanMember: () => {
        this.sendInviteClanMember();
      },
      onAcceptClanInvite: (inviteId) => {
        this.sendAcceptClanInvite(inviteId);
      },
      onDeclineClanInvite: (inviteId) => {
        this.sendDeclineClanInvite(inviteId);
      },
      onLeaveClan: () => {
        this.sendLeaveClan();
      },
      onKickClanMember: (targetCharacterId) => {
        this.sendKickClanMember(targetCharacterId);
      },
      onDissolveClan: () => {
        this.sendDissolveClan();
      },
      onCreateAlliance: (name) => {
        this.sendCreateAlliance(name);
      },
      onInviteAllianceClan: () => {
        this.sendInviteAllianceClan();
      },
      onAcceptAllianceInvite: (inviteId) => {
        this.sendAcceptAllianceInvite(inviteId);
      },
      onDeclineAllianceInvite: (inviteId) => {
        this.sendDeclineAllianceInvite(inviteId);
      },
      onLeaveAlliance: () => {
        this.sendLeaveAlliance();
      },
      onExpelAllianceClan: (targetClanId) => {
        this.sendExpelAllianceClan(targetClanId);
      },
      onDissolveAlliance: () => {
        this.sendDissolveAlliance();
      },
      onSendChatMessage: (channel, text, targetCharacterName) => {
        return this.sendChatMessage(channel, text, targetCharacterName);
      },
      onSellVendorItem: (itemId, quantity) => {
        this.sendSellItem(itemId, quantity);
      },
      onDepositWarehouseItem: (itemId, quantity) => {
        this.sendDepositItem(itemId, quantity);
      },
      onWithdrawWarehouseItem: (itemId, quantity) => {
        this.sendWithdrawItem(itemId, quantity);
      },
      onHotbarChange: (hotbar) => {
        this.sendHotbarState(hotbar);
      },
      stateProvider: () => this.onlineReadModel?.snapshot ?? snapshot,
    });
    this.renderStatus();
  }

  private refreshOnlineRuntime(): void {
    if (!this.onlineReadModel || !this.runtime) {
      return;
    }
    this.runtime.replaceState(this.onlineReadModel.snapshot);
    this.renderStatus();
  }

  private sendMoveIntent(point: { x: number; z: number }): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createMoveIntent(point);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.refreshOnlineRuntime();
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendSelectTarget(targetId: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createSelectTarget(targetId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendClearTarget(): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createClearTarget();
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendInteractNpc(npcId: string, actionId?: 'accept_task' | 'turn_in_task'): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createInteractNpc(npcId, actionId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private dismissOnlineDialog(): void {
    if (!this.onlineReadModel) {
      return;
    }
    this.onlineReadModel.dismissNpcInteraction();
    this.refreshOnlineRuntime();
  }

  private sendUseSkill(skillId: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createUseSkill(skillId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendUseItem(itemId: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createUseItem(itemId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendPickUpLoot(lootId: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createPickUpLoot(lootId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendHotbarAction(actionId: HotbarActionId): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const pickupLootId = actionId === 'pick_up_nearby' ? this.onlineReadModel.findNearestLootId() : null;
    let command = null;
    switch (actionId) {
      case 'basic_attack':
        command = this.onlineReadModel.createBasicAttack();
        break;
      case 'pick_up_nearby':
        command = pickupLootId
          ? this.onlineReadModel.createPickUpLoot(pickupLootId)
          : this.onlineReadModel.createPickUpNearbyLoot();
        break;
      case 'party_invite':
        command = this.onlineReadModel.createInvitePartyMember();
        break;
      case 'party_leave':
        command = this.onlineReadModel.createLeaveParty();
        break;
      case 'tame_target':
        command = this.onlineReadModel.createTameMob();
        break;
      case 'summon_pet':
        command = this.onlineReadModel.createSummonPet();
        break;
      case 'dismiss_pet':
        command = this.onlineReadModel.createDismissPet();
        break;
      case 'mount_pet':
        command = this.onlineReadModel.createMountPet();
        break;
      case 'dismount_pet':
        command = this.onlineReadModel.createDismountPet();
        break;
      case 'toggle_walk_run':
        return;
      default:
        command = null;
        break;
    }
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendEquipItem(itemId: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createEquipItem(itemId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendUnequipItem(slot: EquipSlot): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createUnequipItem(slot);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendSplitItemStack(itemId: string, quantity: number): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createSplitItemStack(itemId, quantity);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendMergeItemStacks(sourceItemId: string, targetItemId: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createMergeItemStacks(sourceItemId, targetItemId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendBuyItem(offerId: string, quantity: number): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createBuyItem(offerId, quantity);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendExchangeItem(offerId: string, quantity: number): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createExchangeItem(offerId, quantity);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendDepositItem(itemId: string, quantity: number): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createDepositItem(itemId, quantity);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendOfferTradeItem(targetCharacterId: string, itemId: string, quantity: number): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createOfferTradeItem(targetCharacterId, itemId, quantity);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendAcceptTradeOffer(offerId: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createAcceptTradeOffer(offerId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendDeclineTradeOffer(offerId: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createDeclineTradeOffer(offerId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendInvitePartyMember(targetCharacterId?: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createInvitePartyMember(targetCharacterId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendAcceptPartyInvite(inviteId: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createAcceptPartyInvite(inviteId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendDeclinePartyInvite(inviteId: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createDeclinePartyInvite(inviteId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendLeaveParty(): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createLeaveParty();
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendKickPartyMember(targetCharacterId: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createKickPartyMember(targetCharacterId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendCreateClan(name: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createClan(name);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendInviteClanMember(): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createInviteClanMember();
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendAcceptClanInvite(inviteId: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createAcceptClanInvite(inviteId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendDeclineClanInvite(inviteId: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createDeclineClanInvite(inviteId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendLeaveClan(): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createLeaveClan();
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendKickClanMember(targetCharacterId: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createKickClanMember(targetCharacterId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendDissolveClan(): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createDissolveClan();
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendCreateAlliance(name: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createAlliance(name);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendInviteAllianceClan(): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createInviteAllianceClan();
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendAcceptAllianceInvite(inviteId: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createAcceptAllianceInvite(inviteId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendDeclineAllianceInvite(inviteId: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createDeclineAllianceInvite(inviteId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendLeaveAlliance(): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createLeaveAlliance();
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendExpelAllianceClan(targetClanId: string): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createExpelAllianceClan(targetClanId);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendDissolveAlliance(): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createDissolveAlliance();
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendChatMessage(
    channel: 'region' | 'party' | 'alliance' | 'whisper',
    text: string,
    targetCharacterName?: string,
  ): boolean {
    if (!this.onlineReadModel || !this.sessionClient) {
      return false;
    }
    const slashCommand = this.onlineReadModel.createPartySlashCommand(text);
    if (slashCommand !== undefined) {
      if (!slashCommand) {
        this.refreshOnlineRuntime();
        return false;
      }
      this.sessionClient.sendCommand(slashCommand);
      this.renderStatus();
      return true;
    }
    const command = this.onlineReadModel.createSendChatMessage(channel, text, targetCharacterName);
    if (!command) {
      this.refreshOnlineRuntime();
      return false;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
    return true;
  }

  private sendSellItem(itemId: string, quantity: number): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createSellItem(itemId, quantity);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendWithdrawItem(itemId: string, quantity: number): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createWithdrawItem(itemId, quantity);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private sendHotbarState(hotbar: PlayerHotbarState): void {
    if (!this.onlineReadModel || !this.sessionClient) {
      return;
    }
    const command = this.onlineReadModel.createSetHotbarState(hotbar);
    if (!command) {
      this.refreshOnlineRuntime();
      return;
    }
    this.sessionClient.sendCommand(command);
    this.renderStatus();
  }

  private handleOnlineMessage(
    sessionClient: GameplaySessionClient,
    sessionEpoch: number,
    message: Parameters<OnlineReadModel['applyMessage']>[0],
  ): void {
    if (this.sessionClient !== sessionClient || this.onlineSessionEpoch !== sessionEpoch) {
      return;
    }
    if (!this.onlineReadModel) {
      if (message.kind === 'reject') {
        this.transition({ type: 'operation_failed', message: message.message });
      }
      return;
    }

    const result = this.onlineReadModel.applyMessage(message);
    if (message.kind === 'reject' && !message.command_id) {
      this.transition({ type: 'operation_failed', message: message.message });
    }
    if (result.changed) {
      this.refreshOnlineRuntime();
    } else {
      this.renderStatus();
    }
  }

  private handleOnlineClose(sessionClient: GameplaySessionClient, sessionEpoch: number): void {
    if (this.sessionClient !== sessionClient || this.onlineSessionEpoch !== sessionEpoch) {
      return;
    }
    this.onlineSessionEpoch += 1;
    this.sessionClient = null;
    this.clearMountedOnlineState();
    this.transition({
      type: 'online_session_closed',
      message: 'Gameplay session closed. Re-enter the world to continue.',
    });
  }

  private resetOnlineSessionState(): void {
    this.onlineSessionEpoch += 1;
    const sessionClient = this.sessionClient;
    this.sessionClient = null;
    sessionClient?.close();
    this.clearMountedOnlineState();
  }

  private clearMountedOnlineState(): void {
    this.onlineReadModel = null;
    this.runtime?.destroy();
    this.runtime = null;
  }

  private selectedCharacter(): CharacterSummary | null {
    return this.state.characters.find((character) => character.character_id === this.state.selectedCharacterId) ?? null;
  }

  private toUserMessage(error: unknown): string {
    if (error instanceof ApiClientError) {
      if (error.reasonCode === 'character.name_unavailable') {
        return 'Character name is already reserved.';
      }
      return `${error.reasonCode}: ${error.message}`;
    }
    if (error instanceof Error) {
      return error.message;
    }
    return 'Unexpected error.';
  }

  private render(): void {
    const runtimeReady = this.state.phase === 'local_ready' || this.state.phase === 'online_ready';
    this.root.classList.toggle('client-app--pregame', !runtimeReady);
    this.root.classList.toggle('client-app--runtime', runtimeReady);
    this.renderStatus();
    if (runtimeReady) {
      this.destroyCharacterLobbyScene();
      this.destroyCharacterCreationScene();
      return;
    }

    this.destroyCharacterLobbyScene();
    this.destroyCharacterCreationScene();
    this.content.innerHTML = this.renderPreGame();
    if (this.state.phase === 'character_list') {
      this.mountCharacterLobbyScene();
    }
    if (this.state.phase === 'character_create') {
      this.mountCharacterCreationScene();
    }
  }

  private renderStatus(): void {
    const onlineState = this.onlineReadModel?.getStateInfo() ?? null;
    const modeLabel =
      this.state.phase === 'local_ready'
        ? 'Local Prototype'
        : this.state.phase === 'online_ready'
          ? onlineState?.commandFlowBlocked
            ? 'Online Desynced'
            : 'Online Command Flow'
          : this.state.mode === 'online'
            ? 'Online Bootstrap'
            : 'Pre-Game';
    this.statusBar.innerHTML = `
      <div>
        <strong>L2 BOARD GAME</strong>
        <span class="app-status-chip">${modeLabel}</span>
        ${
          onlineState
            ? `<span class="app-status-meta">rev ${onlineState.lastRevision} • region ${onlineState.lastRegionRevision} • next seq ${onlineState.nextCommandSeq} • pending ${onlineState.pendingCommands.filter((command) => command.status === 'sent' || command.status === 'acked').length}${onlineState.commandFlowBlocked ? ` • blocked ${onlineState.desyncState}` : ''}</span>`
            : ''
        }
      </div>
      <div class="app-status-actions">
        ${this.state.mode === 'online' || this.state.phase === 'online_ready' ? `<button data-click-action="reset-online">${onlineState?.commandFlowBlocked ? 'Rebootstrap Online' : 'Reset Online'}</button>` : ''}
      </div>
    `;
  }

  private renderPreGame(): string {
    const error = this.state.error ? `<div class="pregame-error">${escapeHtml(this.state.error)}</div>` : '';

    switch (this.state.phase) {
      case 'mode_select':
        return this.renderLoginScreen(error);
      case 'register':
        return this.renderRegisterScreen(error);
      case 'loading_account':
      case 'entering_world':
      case 'attaching':
        return `
          <section class="pregame-screen center">
            <div class="pregame-card status-card">
              <h2>${this.state.phase === 'loading_account' ? 'Authenticating' : this.state.phase === 'entering_world' ? 'Requesting World Entry' : 'Attaching Session'}</h2>
              <p>${this.state.phase === 'attaching' ? 'Waiting for authoritative region_context before the world mounts and command flow opens.' : 'Please wait while the backend confirms the next step.'}</p>
              ${error}
            </div>
          </section>
        `;
      case 'pending_verification':
        return `
          <section class="pregame-screen center">
            <div class="pregame-card status-card">
              <h2>Verification Pending</h2>
              <p>The backend requires account verification before login can continue.</p>
              <p class="muted-copy">Account: ${escapeHtml(this.state.verificationLogin ?? 'unknown')}</p>
              ${error}
              <div class="pregame-actions">
                <button type="button" data-click-action="open-login">Back to Login</button>
                <button type="button" class="secondary" data-click-action="open-recovery">Open Recovery Hook</button>
              </div>
            </div>
          </section>
        `;
      case 'eula_review':
        return this.renderEulaScreen(error);
      case 'server_select':
        return this.renderServerSelectScreen(error);
      case 'recovery_entry':
        return `
          <section class="pregame-screen center">
            <div class="pregame-card status-card">
              <h2>Recovery Hook</h2>
              <p>This explicit hook exists for the future recovery flow required by the accepted docs.</p>
              <p class="muted-copy">No recovery endpoint is implemented in this slice. The client now exposes the hook instead of collapsing into a generic error state.</p>
              ${error}
              <div class="pregame-actions">
                <button type="button" data-click-action="open-login">Back to Login</button>
                <button type="button" class="secondary" data-click-action="open-register">Back to Registration</button>
              </div>
            </div>
          </section>
        `;
      case 'character_create':
        return this.renderCharacterCreationScreen(error);
      case 'character_list':
        return this.renderCharacterLobbyScreen(error);
      case 'login':
      default:
        return this.renderLoginScreen(error);
    }
  }

  private renderLoginScreen(error: string): string {
    return this.renderAuthScreen({
      screenLabel: 'Game login',
      action: 'login',
      variant: 'login',
      title: 'Log In',
      error,
      fields: [
        { label: 'ID', name: 'login', type: 'text', autocomplete: 'username' },
        { label: 'PWD', name: 'password', type: 'password', autocomplete: 'current-password' },
      ],
      actions: [
        { label: 'Log In', type: 'submit' },
        { label: 'Exit', clickAction: 'reset-online' },
      ],
      sideMenuActive: 'login',
    });
  }

  private renderEulaScreen(error: string): string {
    return `
      <section class="pregame-screen classic-prelogin-screen eula-screen" aria-label="User agreement">
        <article class="classic-eula-window">
          <div class="classic-scroll-rail" aria-hidden="true"><span></span><i></i></div>
          <div class="classic-eula-content">
            <p>Game User Agreement (the Agreement)</p>
            <p>Last Modified January 2007</p>
            <h3>1. TERMS OF AGREEMENT</h3>
            <p>
              This user agreement defines the rules for accessing this game world. By selecting Agree, you confirm
              that gameplay, account access, character progression, chat, trades and social systems are governed by
              server authority and by the operational policies of this project.
            </p>
            <p>
              You agree not to exploit bugs, automate gameplay, abuse other players, duplicate items, evade PvP/PK
              consequences or attempt to bypass authoritative session ownership. Access may be limited while systems
              are under active development.
            </p>
            <p>
              The agreement is intentionally presented before world selection so every account follows the same
              canonical entry flow: login, agreement, world selection and character selection.
            </p>
          </div>
          ${error}
          <div class="classic-eula-actions">
            <button class="game-menu-button" type="button" data-click-action="accept-eula">Agree</button>
            <button class="game-menu-button" type="button" data-click-action="reject-eula">Disagree</button>
          </div>
        </article>
      </section>
    `;
  }

  private renderServerSelectScreen(error: string): string {
    const selectedWorldId = this.state.selectedWorldId ?? GAME_WORLDS[0]?.id ?? '';
    return `
      <section class="pregame-screen classic-prelogin-screen server-select-screen" aria-label="Server selection">
        <div class="classic-server-stack">
          <section class="classic-server-window" aria-label="World list">
            <div class="classic-server-table">
              <div class="classic-server-header">
                <span>No</span><span>Name</span><span>Traffic</span><span>Ping</span><span>Type</span>
              </div>
              <div class="classic-server-rows">
                ${GAME_WORLDS.map(
                  (world) => `
                    <button
                      type="button"
                      class="classic-server-row ${world.id === selectedWorldId ? 'selected' : ''}"
                      data-click-action="select-world"
                      data-world-id="${escapeHtml(world.id)}"
                    >
                      <span>${escapeHtml(world.no)}</span>
                      <strong>${escapeHtml(world.name)}</strong>
                      <span>${escapeHtml(world.traffic)}</span>
                      <span>${world.ping}</span>
                      <span>${escapeHtml(world.type)}</span>
                    </button>
                  `,
                ).join('')}
                ${Array.from({ length: 10 }, () => '<div class="classic-server-row empty" aria-hidden="true"></div>').join('')}
              </div>
            </div>
            ${error}
            <div class="classic-server-actions">
              <button class="game-menu-button" type="button" disabled aria-disabled="true">Quality</button>
              <button class="game-menu-button" type="button" data-click-action="confirm-world-selection" ${selectedWorldId ? '' : 'disabled aria-disabled="true"'}>OK</button>
              <button class="game-menu-button" type="button" data-click-action="cancel-world-selection">Cancel</button>
            </div>
          </section>
          <section class="classic-server-help" aria-label="Server Selection Help">
            <div class="classic-help-title">Server Selection Help</div>
            <a>Server Concepts</a>
            <a>Server Status</a>
            <hr />
          </section>
        </div>
      </section>
    `;
  }

  private mountCharacterLobbyScene(): void {
    const host = this.content.querySelector<HTMLElement>('[data-character-lobby-scene]');
    if (!host) {
      return;
    }
    this.characterLobbyScene = new CharacterLobbyScene(
      host,
      this.state.characters,
      this.state.selectedCharacterId,
      (characterId) => {
        this.transition({ type: 'select_character', characterId });
      },
    );
  }

  private destroyCharacterLobbyScene(): void {
    this.characterLobbyScene?.destroy();
    this.characterLobbyScene = null;
  }

  private mountCharacterCreationScene(): void {
    const host = this.content.querySelector<HTMLElement>('[data-character-creation-scene]');
    if (!host) {
      return;
    }
    const options = resolveCharacterCreationOptions(
      this.state.catalog,
      this.state.createRace,
      this.state.createBaseClass,
      this.state.createSex,
      this.state.createHairStyle,
      this.state.createSkinType,
      this.state.createHairColor,
    );
    this.characterCreationScene = new CharacterCreationScene(host, {
      race: options.selectedRace,
      baseClass: options.selectedBaseClass,
      sex: options.selectedSex,
      hairStyle: options.selectedHairStyle,
      hairColor: options.selectedHairColor,
      skinType: options.selectedSkinType,
      baseClassOptions: options.baseClassOptions,
      sexOptions: options.sexOptions,
    });
  }

  private destroyCharacterCreationScene(): void {
    this.characterCreationScene?.destroy();
    this.characterCreationScene = null;
  }

  private renderCharacterCreationScreen(error: string): string {
    const options = resolveCharacterCreationOptions(
      this.state.catalog,
      this.state.createRace,
      this.state.createBaseClass,
      this.state.createSex,
      this.state.createHairStyle,
      this.state.createSkinType,
      this.state.createHairColor,
    );
    const selectedRace = options.selectedRace;
    const selectedBaseClass = options.selectedBaseClass;
    const selectedSex = options.selectedSex;
    const sceneClass = selectedRace ? this.creationRaceSlug(selectedRace) : 'default';
    const canCreate = Boolean(
      selectedRace &&
        selectedBaseClass &&
        selectedSex &&
        options.selectedHairStyle !== null &&
        options.selectedHairColor !== null &&
        options.selectedSkinType !== null &&
        this.createNameDraft.trim(),
    );
    const fields: CreationFieldConfig[] = [
      {
        label: 'Race',
        value: selectedRace ?? '',
        option: 'race',
        enabled: options.raceOptions.length > 0,
      },
      {
        label: 'Class',
        value: this.creationBaseClassLabel(selectedBaseClass),
        option: 'base_class',
        enabled: Boolean(selectedRace && options.baseClassOptions.length > 0),
      },
      {
        label: 'Gender',
        value: selectedSex ?? '',
        option: 'sex',
        enabled: Boolean(selectedRace && options.sexOptions.length > 0),
      },
      {
        label: 'Hairstyle',
        value: selectedRace && options.selectedHairStyle !== null ? this.appearanceLabel(options.selectedHairStyle) : '',
        option: 'hair_style',
        enabled: Boolean(selectedRace && options.hairStyleOptions.length > 0),
      },
      {
        label: 'Skin Type',
        value: selectedRace && options.selectedSkinType !== null ? this.skinTypeLabel(options.selectedSkinType) : '',
        option: 'skin_type',
        enabled: Boolean(selectedRace && options.skinTypeOptions.length > 0),
      },
    ];

    return `
      <section class="pregame-screen character-create-screen character-create-screen--${sceneClass}" aria-label="Character creation">
        <div class="character-create-scene" data-character-creation-scene></div>
        <div class="character-create-vignette"></div>
        <form class="character-create-form" data-action="create-character">
          <input type="hidden" name="race" value="${escapeHtml(selectedRace ?? '')}" />
          <input type="hidden" name="base_class" value="${escapeHtml(selectedBaseClass ?? '')}" />
          <input type="hidden" name="sex" value="${escapeHtml(selectedSex ?? '')}" />
          <input type="hidden" name="hair_style" value="${escapeHtml(options.selectedHairStyle === null ? '' : String(options.selectedHairStyle))}" />
          <input type="hidden" name="hair_color" value="${escapeHtml(options.selectedHairColor ?? '')}" />
          <input type="hidden" name="skin_type" value="${escapeHtml(options.selectedSkinType === null ? '' : String(options.selectedSkinType))}" />
          <aside class="character-create-panel character-create-controls">
            <label class="character-create-field character-create-field--name">
              <span>Name</span>
              <input class="game-text-input" name="name" type="text" value="${escapeHtml(this.createNameDraft)}" maxlength="24" autocomplete="off" required />
            </label>
            ${fields
              .map(
                (field) =>
                  `${this.renderCreationField(field)}${
                    field.option === 'hair_style' ? this.renderCreationHairColorField(options.selectedHairColor) : ''
                  }`,
              )
              .join('')}
          </aside>
          ${this.renderCreationDescription(selectedRace, selectedBaseClass, selectedSex)}
          ${error ? `<div class="character-create-error">${error}</div>` : ''}
          ${this.renderCreationQuickIcons({
            baseClassEnabled: Boolean(selectedRace && options.baseClassOptions.length > 0),
            sexEnabled: Boolean(selectedRace && options.sexOptions.length > 0),
            appearanceEnabled: Boolean(
              selectedRace &&
                (options.hairStyleOptions.length > 0 || options.skinTypeOptions.length > 0),
            ),
          })}
          <nav class="character-create-actions" aria-label="Character creation actions">
            <button class="game-menu-button" type="submit" ${canCreate ? '' : 'disabled aria-disabled="true"'}>Create Character</button>
            <button class="game-menu-button" type="button" data-click-action="back-to-characters">Previous</button>
          </nav>
        </form>
      </section>
    `;
  }

  private renderCreationField(field: CreationFieldConfig): string {
    const disabled = field.enabled ? '' : ' disabled aria-disabled="true"';
    return `
      <div class="character-create-field">
        <span>${escapeHtml(field.label)}</span>
        <div class="character-create-picker">
          <output>${escapeHtml(field.value)}</output>
          <button class="character-create-arrow" type="button" data-click-action="cycle-create-option" data-creation-option="${field.option}" data-direction="-1"${disabled}>&lt;</button>
          <button class="character-create-arrow" type="button" data-click-action="cycle-create-option" data-creation-option="${field.option}" data-direction="1"${disabled}>&gt;</button>
        </div>
      </div>
    `;
  }

  private renderCreationHairColorField(hairColor: string | null): string {
    const value = normalizeCanonicalHairColor(hairColor);
    if (!value) {
      return `
        <label class="character-create-field character-create-field--hair-color">
          <span>Hair Color</span>
          <span class="character-create-color-control character-create-color-control--invalid">
            <output>Invalid</output>
          </span>
        </label>
      `;
    }
    return `
      <label class="character-create-field character-create-field--hair-color">
        <span>Hair Color</span>
        <span class="character-create-color-control">
          <input type="color" name="hair_color" value="${escapeHtml(value)}" aria-label="Hair Color" />
          <output>${escapeHtml(value.toUpperCase())}</output>
        </span>
      </label>
    `;
  }

  private renderCreationDescription(
    race: CharacterRace | null,
    baseClass: BaseClass | null,
    sex: CharacterSex | null,
  ): string {
    const description = race && baseClass && sex ? this.creationDescription(race, baseClass, sex) : null;
    if (!description) {
      return `
        <aside class="character-create-panel character-create-description character-create-description--empty" aria-label="Class description">
          <div></div><div></div><div></div><div></div>
        </aside>
      `;
    }
    return `
      <aside class="character-create-panel character-create-description" aria-label="Class description">
        <p>${escapeHtml(description.quote)}</p>
        <p>${escapeHtml(description.body)}</p>
      </aside>
    `;
  }

  private renderCreationQuickIcons(config: {
    baseClassEnabled: boolean;
    sexEnabled: boolean;
    appearanceEnabled: boolean;
  }): string {
    if (!config.baseClassEnabled && !config.sexEnabled && !config.appearanceEnabled) {
      return '';
    }
    const classDisabled = config.baseClassEnabled ? '' : ' disabled aria-disabled="true"';
    const sexDisabled = config.sexEnabled ? '' : ' disabled aria-disabled="true"';
    const appearanceDisabled = config.appearanceEnabled ? '' : ' disabled aria-disabled="true"';
    return `
      <div class="character-create-quick-icons" aria-label="Quick creation selectors">
        <button type="button" data-click-action="cycle-create-option" data-creation-option="base_class" data-direction="1"${classDisabled}><span class="creation-icon creation-icon--class"></span></button>
        <button type="button" data-click-action="cycle-create-option" data-creation-option="sex" data-direction="1"${sexDisabled}><span class="creation-icon creation-icon--gender"></span></button>
        <button type="button" data-click-action="cycle-create-option" data-creation-option="hair_style" data-direction="1"${appearanceDisabled}><span class="creation-icon creation-icon--style"></span></button>
      </div>
    `;
  }

  private creationRaceSlug(race: CharacterRace): string {
    return race.toLowerCase().replace(/\s+/g, '-');
  }

  private creationBaseClassLabel(baseClass: BaseClass | null): string {
    if (!baseClass) {
      return '';
    }
    return getBaseClassCreationLabel(baseClass);
  }

  private appearanceLabel(value: number): string {
    return `Type ${String.fromCharCode(65 + value)}`;
  }

  private skinTypeLabel(value: number): string {
    return `Type ${String.fromCharCode(65 + value)}`;
  }

  private creationDescription(
    race: CharacterRace,
    baseClass: BaseClass,
    sex: CharacterSex,
  ): { quote: string; body: string } {
    const classLabel = this.creationBaseClassLabel(baseClass);
    const raceCopy: Record<CharacterRace, string> = {
      Human: 'Human adventurers are versatile, disciplined, and trusted to stand at the center of every conflict.',
      Elf: 'Elves move with calm precision, using grace, distance, and old forest magic to outlast their enemies.',
      'Dark Elf': 'Dark Elves draw strength from shadowed rites, striking with cold focus before the enemy can answer.',
      Orc: 'Orcs are direct, proud, and overwhelming, built to break enemy lines through raw will and force.',
      Dwarf: 'Dwarves carry mountain stubbornness into battle, turning craft, resilience, and timing into survival.',
    };
    const classCopy = getBaseClassDefinition(baseClass).description;
    return {
      quote: `${sex} ${race} ${classLabel}`,
      body: `${raceCopy[race]} ${classCopy}`,
    };
  }

  private renderCharacterLobbyScreen(error: string): string {
    const selected = this.selectedCharacter();
    return `
      <section class="pregame-screen character-lobby-screen" aria-label="Character selection lobby">
        <div class="character-lobby-scene" data-character-lobby-scene></div>
        <div class="character-lobby-vignette"></div>
        ${this.renderCharacterLobbyInfoPanel(selected)}
        ${this.renderCharacterLobbyQuickSelect()}
        ${
          error
            ? `<div class="character-lobby-error">${error}</div>`
            : ''
        }
        ${
          this.state.characters.length === 0
            ? '<div class="character-lobby-empty">No characters yet. Use Create to prepare your first character.</div>'
            : ''
        }
        <div class="character-lobby-start-panel">
          <button class="game-menu-button character-lobby-start" data-click-action="enter-world" ${!this.state.selectedCharacterId ? 'disabled' : ''}>Start</button>
        </div>
        <nav class="character-lobby-menu" aria-label="Character menu">
          <button class="game-menu-button" data-click-action="open-create-character">Create</button>
          <button class="game-menu-button" type="button" disabled aria-disabled="true" title="Delete flow is not implemented yet.">Delete</button>
          <button class="game-menu-button" data-click-action="reset-online">Re-Login</button>
        </nav>
      </section>
    `;
  }

  private renderCharacterLobbyInfoPanel(selected: CharacterSummary | null): string {
    const stats = selected ? this.lobbyStatsForCharacter(selected) : null;
    const selectOptions =
      this.state.characters.length > 0
        ? this.state.characters
            .map(
              (character) =>
                `<option value="${character.character_id}" ${character.character_id === this.state.selectedCharacterId ? 'selected' : ''}>${escapeHtml(character.name)}</option>`,
            )
            .join('')
        : '<option value="">No Character</option>';

    return `
      <aside class="character-lobby-info">
        <label class="character-lobby-name-row">
          <span>Name</span>
          <select name="character_lobby_select" ${this.state.characters.length === 0 ? 'disabled' : ''}>
            ${selectOptions}
          </select>
        </label>
        <div class="character-lobby-class-row">
          <span>Lv ${selected?.level ?? 0}</span>
          <strong>${selected ? `${escapeHtml(selected.race)} ${escapeHtml(selected.base_class)}` : 'No Character'}</strong>
        </div>
        ${this.renderLobbyStatRow('HP', stats?.hp ?? 0, stats?.maxHp ?? 1, 'hp')}
        ${this.renderLobbyStatRow('MP', stats?.mp ?? 0, stats?.maxMp ?? 1, 'mp')}
        <div class="character-lobby-two-stat-row">
          <span>SP</span><strong>0</strong>
        </div>
        <div class="character-lobby-two-stat-row">
          <span>Karma</span><strong>0</strong>
        </div>
        <div class="character-lobby-exp-row">
          <span>Exp</span><strong>0.00%</strong>
        </div>
      </aside>
    `;
  }

  private renderLobbyStatRow(label: string, value: number, maxValue: number, tone: 'hp' | 'mp'): string {
    const width = Math.max(0, Math.min(100, (value / Math.max(maxValue, 1)) * 100));
    return `
      <div class="character-lobby-stat-row ${tone}">
        <span>${label}</span>
        <div class="character-lobby-stat-bar">
          <i style="width:${width}%"></i>
          <strong>${value}/${maxValue}</strong>
        </div>
      </div>
    `;
  }

  private renderCharacterLobbyQuickSelect(): string {
    if (this.state.characters.length === 0) {
      return '';
    }
    return `
      <div class="character-lobby-quick-select" aria-label="Quick character select">
        ${this.state.characters.map((character) => this.renderCharacterCard(character)).join('')}
      </div>
    `;
  }

  private lobbyStatsForCharacter(character: CharacterSummary): {
    hp: number;
    maxHp: number;
    mp: number;
    maxMp: number;
  } {
    const level = Math.max(character.level, 1);
    const archetype = gameTemplates.archetypes[getArchetypeIdForBaseClass(character.base_class)];
    const maxHp = archetype.baseHp + (level - 1) * archetype.hpGrowth;
    const maxMp = archetype.baseMp + (level - 1) * archetype.mpGrowth;
    return {
      hp: maxHp,
      maxHp,
      mp: maxMp,
      maxMp,
    };
  }

  private renderRegisterScreen(error: string): string {
    return this.renderAuthScreen({
      screenLabel: 'New account',
      action: 'register',
      variant: 'register',
      title: 'New Account',
      error,
      fields: [
        { label: 'ID', name: 'login', type: 'text', autocomplete: 'username' },
        { label: 'NAME', name: 'display_name', type: 'text', autocomplete: 'nickname' },
        { label: 'PWD', name: 'password', type: 'password', autocomplete: 'new-password' },
      ],
      actions: [
        { label: 'Create', type: 'submit' },
        { label: 'Back', clickAction: 'open-login' },
      ],
      sideMenuActive: 'register',
    });
  }

  private renderAuthScreen(config: AuthScreenConfig): string {
    return `
      <section class="pregame-screen auth-screen" aria-label="${escapeHtml(config.screenLabel)}">
        <form class="auth-panel auth-panel--${config.variant}" data-action="${config.action}">
          <div class="auth-panel-title">${escapeHtml(config.title)}</div>
          ${config.error}
          ${config.fields.map((field) => this.renderAuthField(field)).join('')}
          <div class="auth-actions">
            ${config.actions.map((button) => this.renderGameMenuButton(button)).join('')}
          </div>
        </form>
        ${this.renderAuthSideMenu(config.sideMenuActive)}
      </section>
    `;
  }

  private renderAuthField(field: AuthFieldConfig): string {
    return `
      <label class="auth-field">
        <span>${escapeHtml(field.label)}</span>
        <input class="game-text-input" name="${field.name}" type="${field.type}" autocomplete="${field.autocomplete}" required />
      </label>
    `;
  }

  private renderGameMenuButton(button: GameButtonConfig): string {
    const type = button.type ?? 'button';
    const clickAction = button.clickAction ? ` data-click-action="${button.clickAction}"` : '';
    const disabled = button.disabled ? ' disabled aria-disabled="true"' : '';
    return `<button class="game-menu-button" type="${type}"${clickAction}${disabled}>${escapeHtml(button.label)}</button>`;
  }

  private renderAuthSideMenu(active: 'login' | 'register'): string {
    const primaryAction =
      active === 'register'
        ? { label: 'Log In', clickAction: 'open-login' }
        : { label: 'New Account', clickAction: 'open-register' };
    const buttons: GameButtonConfig[] = [
      primaryAction,
      { label: 'Lost Account', clickAction: 'open-recovery' },
      { label: 'Options', disabled: true },
      { label: 'Production Team', disabled: true },
      { label: 'Replay', disabled: true },
    ];
    return `
      <nav class="auth-side-menu" aria-label="Account menu">
        ${buttons.map((button) => this.renderGameMenuButton(button)).join('')}
      </nav>
    `;
  }

  private renderSidebar(): string {
    return `
      <aside class="pregame-card sidebar-card">
        <h3>Account Session</h3>
        <ul class="pregame-list">
          <li>Online mode starts at auth and waits for region_context.</li>
          <li>Character entry remains explicit and server-authoritative.</li>
          <li>No automatic fallback from online to local rules.</li>
        </ul>
        <div class="pregame-actions stacked">
          <button class="secondary" data-click-action="reset-online">Reset Online Flow</button>
        </div>
      </aside>
    `;
  }

  private renderCharacterCard(character: CharacterSummary): string {
    const selected = character.character_id === this.state.selectedCharacterId;
    return `
      <button class="character-card character-lobby-roster-button ${selected ? 'selected' : ''}" data-click-action="select-character" data-character-id="${character.character_id}" aria-pressed="${selected}">
        <strong>${escapeHtml(character.name)}</strong>
        <span>${escapeHtml(character.race)} ${escapeHtml(character.base_class)}</span>
        <span>${escapeHtml(character.sex)} • Lv ${character.level}</span>
        <span>${escapeHtml(character.last_region_id)}</span>
      </button>
    `;
  }

  private renderCatalogInputs(): string {
    const options = resolveCharacterCreationOptions(
      this.state.catalog,
      this.state.createRace,
      this.state.createBaseClass,
      this.state.createSex,
      this.state.createHairStyle,
      this.state.createSkinType,
      this.state.createHairColor,
    );
    const raceOptions = options.raceOptions
      .map(
        (race) =>
          `<option value="${race.race}" ${race.race === options.selectedRace ? 'selected' : ''}>${escapeHtml(race.race)}</option>`,
      )
      .join('');
    const classOptions = options.baseClassOptions
      .map((baseClass) => `<option value="${baseClass}">${escapeHtml(baseClass)}</option>`)
      .join('');
    const sexOptions = options.sexOptions
      .map((sex) => `<option value="${sex}">${escapeHtml(sex)}</option>`)
      .join('');

    return `
      <label>Race<select name="race">${raceOptions}</select></label>
      <label>Base Class<select name="base_class">${classOptions}</select></label>
      <label>Sex<select name="sex">${sexOptions}</select></label>
      <label>Hair Color<input name="hair_color" type="color" value="${escapeHtml(options.selectedHairColor ?? '#000000')}" /></label>
      <label>Name<input name="name" type="text" maxlength="24" required /></label>
    `;
  }

  private installE2EBridge(): void {
    window.__l2bgE2E = {
      getSnapshot: () => ({
        mode: this.state.mode,
        phase: this.state.phase,
        error: this.state.error,
        selectedCharacterId: this.state.selectedCharacterId,
        runtimeMounted: Boolean(this.runtime),
        onlineState: this.onlineReadModel?.getStateInfo() ?? null,
      }),
      getWorldState: () => window.__mvpDebug?.store.getState() ?? null,
      sendMoveIntent: (point) => {
        this.sendMoveIntent(point);
      },
      sendSelectTarget: (targetId) => {
        this.sendSelectTarget(targetId);
      },
      sendClearTarget: () => {
        this.sendClearTarget();
      },
      sendInteractNpc: (npcId, actionId) => {
        this.sendInteractNpc(npcId, actionId);
      },
      sendUseSkill: (skillId) => {
        this.sendUseSkill(skillId);
      },
      sendBasicAttack: () => {
        this.sendHotbarAction('basic_attack');
      },
      sendPickUpLoot: (lootId) => {
        this.sendPickUpLoot(lootId);
      },
      sendUseItem: (itemId) => {
        this.sendUseItem(itemId);
      },
      sendEquipItem: (itemId) => {
        this.sendEquipItem(itemId);
      },
      sendUnequipItem: (slot) => {
        this.sendUnequipItem(slot);
      },
      sendSplitItemStack: (itemId, quantity) => {
        this.sendSplitItemStack(itemId, quantity);
      },
      sendMergeItemStacks: (sourceItemId, targetItemId) => {
        this.sendMergeItemStacks(sourceItemId, targetItemId);
      },
      sendBuyItem: (offerId, quantity) => {
        this.sendBuyItem(offerId, quantity);
      },
      sendExchangeItem: (offerId, quantity) => {
        this.sendExchangeItem(offerId, quantity);
      },
      sendOfferTradeItem: (targetCharacterId, itemId, quantity) => {
        this.sendOfferTradeItem(targetCharacterId, itemId, quantity);
      },
      sendAcceptTradeOffer: (offerId) => {
        this.sendAcceptTradeOffer(offerId);
      },
      sendDeclineTradeOffer: (offerId) => {
        this.sendDeclineTradeOffer(offerId);
      },
      sendInvitePartyMember: (targetCharacterId) => {
        this.sendInvitePartyMember(targetCharacterId);
      },
      sendAcceptPartyInvite: (inviteId) => {
        this.sendAcceptPartyInvite(inviteId);
      },
      sendDeclinePartyInvite: (inviteId) => {
        this.sendDeclinePartyInvite(inviteId);
      },
      sendLeaveParty: () => {
        this.sendLeaveParty();
      },
      sendKickPartyMember: (targetCharacterId) => {
        this.sendKickPartyMember(targetCharacterId);
      },
      sendCreateClan: (name) => {
        this.sendCreateClan(name);
      },
      sendInviteClanMember: () => {
        this.sendInviteClanMember();
      },
      sendAcceptClanInvite: (inviteId) => {
        this.sendAcceptClanInvite(inviteId);
      },
      sendDeclineClanInvite: (inviteId) => {
        this.sendDeclineClanInvite(inviteId);
      },
      sendLeaveClan: () => {
        this.sendLeaveClan();
      },
      sendKickClanMember: (targetCharacterId) => {
        this.sendKickClanMember(targetCharacterId);
      },
      sendDissolveClan: () => {
        this.sendDissolveClan();
      },
      sendCreateAlliance: (name) => {
        this.sendCreateAlliance(name);
      },
      sendInviteAllianceClan: () => {
        this.sendInviteAllianceClan();
      },
      sendAcceptAllianceInvite: (inviteId) => {
        this.sendAcceptAllianceInvite(inviteId);
      },
      sendDeclineAllianceInvite: (inviteId) => {
        this.sendDeclineAllianceInvite(inviteId);
      },
      sendLeaveAlliance: () => {
        this.sendLeaveAlliance();
      },
      sendExpelAllianceClan: (targetClanId) => {
        this.sendExpelAllianceClan(targetClanId);
      },
      sendDissolveAlliance: () => {
        this.sendDissolveAlliance();
      },
      sendChatMessage: (channel, text, targetCharacterName) => {
        this.sendChatMessage(channel, text, targetCharacterName);
      },
      sendSellItem: (itemId, quantity) => {
        this.sendSellItem(itemId, quantity);
      },
      sendDepositItem: (itemId, quantity) => {
        this.sendDepositItem(itemId, quantity);
      },
      sendWithdrawItem: (itemId, quantity) => {
        this.sendWithdrawItem(itemId, quantity);
      },
    };
  }
}
