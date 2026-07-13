import type {
  BaseClass,
  CharacterCatalogResponse,
  CharacterRace,
  CharacterSex,
  CharacterSummary,
  RegionContextMessage,
} from '../online/contracts';

export type AppMode = 'local' | 'online';

export type PreGamePhase =
  | 'mode_select'
  | 'register'
  | 'login'
  | 'pending_verification'
  | 'recovery_entry'
  | 'loading_account'
  | 'character_list'
  | 'character_create'
  | 'entering_world'
  | 'attaching'
  | 'online_ready'
  | 'local_ready';

export interface SessionBootstrap {
  sessionId: string;
  attachToken: string;
  wsUrl: string;
}

export interface PreGameContext {
  phase: PreGamePhase;
  mode: AppMode | null;
  authScreen: 'login' | 'register';
  verificationLogin: string | null;
  accessToken: string | null;
  accountId: string | null;
  characters: CharacterSummary[];
  catalog: CharacterCatalogResponse | null;
  createRace: CharacterRace | null;
  createBaseClass: BaseClass | null;
  createSex: CharacterSex | null;
  createHairStyle: number | null;
  createHairColor: number | null;
  createFace: number | null;
  selectedCharacterId: string | null;
  sessionBootstrap: SessionBootstrap | null;
  regionContext: RegionContextMessage | null;
  error: string | null;
}

export type PreGameEvent =
  | { type: 'choose_local' }
  | { type: 'choose_online' }
  | { type: 'open_login' }
  | { type: 'open_register' }
  | { type: 'open_recovery' }
  | { type: 'auth_pending' }
  | { type: 'register_requires_verification'; login: string }
  | {
      type: 'auth_succeeded';
      accessToken: string;
      accountId: string;
      characters: CharacterSummary[];
      catalog: CharacterCatalogResponse;
    }
  | { type: 'auth_failed'; message: string }
  | { type: 'open_create_character' }
  | { type: 'set_create_race'; race: CharacterRace }
  | { type: 'set_create_base_class'; baseClass: BaseClass }
  | { type: 'set_create_sex'; sex: CharacterSex }
  | { type: 'set_create_appearance'; field: 'hair_style' | 'hair_color' | 'face'; value: number }
  | { type: 'characters_updated'; characters: CharacterSummary[]; catalog?: CharacterCatalogResponse }
  | { type: 'select_character'; characterId: string }
  | { type: 'enter_world_pending' }
  | { type: 'enter_world_succeeded'; bootstrap: SessionBootstrap; characterId: string }
  | { type: 'attach_pending' }
  | { type: 'attach_succeeded'; regionContext: RegionContextMessage }
  | { type: 'online_session_closed'; message: string }
  | { type: 'operation_failed'; message: string }
  | { type: 'reset_online' };

export const initialPreGameContext = (): PreGameContext => ({
  phase: 'login',
  mode: 'online',
  authScreen: 'login',
  verificationLogin: null,
  accessToken: null,
  accountId: null,
  characters: [],
  catalog: null,
  createRace: null,
  createBaseClass: null,
  createSex: null,
  createHairStyle: null,
  createHairColor: null,
  createFace: null,
  selectedCharacterId: null,
  sessionBootstrap: null,
  regionContext: null,
  error: null,
});

export const preGameReducer = (state: PreGameContext, event: PreGameEvent): PreGameContext => {
  switch (event.type) {
    case 'choose_local':
      return {
        ...initialPreGameContext(),
        mode: 'local',
        phase: 'local_ready',
      };
    case 'choose_online':
      return {
        ...initialPreGameContext(),
        mode: 'online',
        authScreen: 'login',
        phase: 'login',
      };
    case 'open_login':
      return {
        ...state,
        mode: 'online',
        authScreen: 'login',
        phase: 'login',
        verificationLogin: state.verificationLogin,
        error: null,
      };
    case 'open_register':
      return {
        ...state,
        mode: 'online',
        authScreen: 'register',
        phase: 'register',
        verificationLogin: null,
        error: null,
      };
    case 'open_recovery':
      return {
        ...state,
        mode: 'online',
        phase: 'recovery_entry',
        error: null,
      };
    case 'auth_pending':
      return {
        ...state,
        phase: 'loading_account',
        error: null,
      };
    case 'register_requires_verification':
      return {
        ...state,
        phase: 'pending_verification',
        verificationLogin: event.login,
        error: null,
      };
    case 'auth_succeeded':
      return {
        ...state,
        phase: 'character_list',
        mode: 'online',
        accessToken: event.accessToken,
        accountId: event.accountId,
        characters: event.characters,
        catalog: event.catalog,
        createRace: null,
        createBaseClass: null,
        createSex: null,
        createHairStyle: null,
        createHairColor: null,
        createFace: null,
        selectedCharacterId: event.characters[0]?.character_id ?? null,
        verificationLogin: null,
        error: null,
      };
    case 'auth_failed':
      return {
        ...state,
        phase: state.authScreen,
        error: event.message,
      };
    case 'open_create_character':
      return {
        ...state,
        phase: 'character_create',
        createRace: null,
        createBaseClass: null,
        createSex: null,
        createHairStyle: null,
        createHairColor: null,
        createFace: null,
        error: null,
      };
    case 'set_create_race':
      return {
        ...state,
        phase: 'character_create',
        createRace: event.race,
        createBaseClass: null,
        createSex: null,
        createHairStyle: null,
        createHairColor: null,
        createFace: null,
        error: null,
      };
    case 'set_create_base_class':
      return {
        ...state,
        phase: 'character_create',
        createBaseClass: event.baseClass,
        error: null,
      };
    case 'set_create_sex':
      return {
        ...state,
        phase: 'character_create',
        createSex: event.sex,
        error: null,
      };
    case 'set_create_appearance':
      return {
        ...state,
        phase: 'character_create',
        createHairStyle: event.field === 'hair_style' ? event.value : state.createHairStyle,
        createHairColor: event.field === 'hair_color' ? event.value : state.createHairColor,
        createFace: event.field === 'face' ? event.value : state.createFace,
        error: null,
      };
    case 'characters_updated':
      return {
        ...state,
        phase: 'character_list',
        characters: event.characters,
        catalog: event.catalog ?? state.catalog,
        createRace: null,
        createBaseClass: null,
        createSex: null,
        createHairStyle: null,
        createHairColor: null,
        createFace: null,
        selectedCharacterId: event.characters[0]?.character_id ?? null,
        error: null,
      };
    case 'select_character':
      return {
        ...state,
        selectedCharacterId: event.characterId,
        error: null,
      };
    case 'enter_world_pending':
      return {
        ...state,
        phase: 'entering_world',
        error: null,
      };
    case 'enter_world_succeeded':
      return {
        ...state,
        phase: 'attaching',
        selectedCharacterId: event.characterId,
        sessionBootstrap: event.bootstrap,
        error: null,
      };
    case 'attach_pending':
      return {
        ...state,
        phase: 'attaching',
        error: null,
      };
    case 'attach_succeeded':
      return {
        ...state,
        phase: 'online_ready',
        regionContext: event.regionContext,
        error: null,
      };
    case 'online_session_closed':
      if (!state.accessToken) {
        return {
          ...initialPreGameContext(),
          mode: 'online',
          authScreen: 'login',
          phase: 'login',
          error: event.message,
        };
      }
      return {
        ...state,
        phase: 'character_list',
        sessionBootstrap: null,
        regionContext: null,
        error: event.message,
      };
    case 'operation_failed':
      return {
        ...state,
        phase:
          state.phase === 'attaching' || state.phase === 'entering_world' ? 'character_list' : state.phase,
        error: event.message,
      };
    case 'reset_online':
      return {
        ...initialPreGameContext(),
        mode: 'online',
        authScreen: 'login',
        phase: 'login',
      };
    default:
      return state;
  }
};
