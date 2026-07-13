import type {
  ApiErrorResponse,
  AttachSessionMessage,
  CharacterCatalogResponse,
  CharactersResponse,
  CreateCharacterRequest,
  CreateCharacterResponse,
  GameplayCommandEnvelope,
  GameplayServerMessage,
  LoginRequest,
  LoginResponse,
  RegionContextMessage,
  RegisterRequest,
  RegisterResponse,
  WorldEnterRequest,
  WorldEnterResponse,
} from './contracts';
import { isApiErrorResponse } from './contracts';

export class ApiClientError extends Error {
  readonly reasonCode: string;
  readonly status: number;

  constructor(message: string, reasonCode: string, status: number) {
    super(message);
    this.name = 'ApiClientError';
    this.reasonCode = reasonCode;
    this.status = status;
  }
}

const parseJsonResponse = async <T>(response: Response): Promise<T> => {
  const payload = (await response.json()) as T | ApiErrorResponse;
  if (!response.ok) {
    if (isApiErrorResponse(payload)) {
      throw new ApiClientError(payload.message, payload.reason_code, response.status);
    }
    throw new ApiClientError('Unexpected API error.', 'internal.unexpected_error', response.status);
  }
  return payload as T;
};

export class OnlineApiClient {
  constructor(private readonly baseUrl: string) {}

  async register(request: RegisterRequest): Promise<RegisterResponse> {
    return this.post<RegisterResponse>('/v1/auth/register', request);
  }

  async login(request: LoginRequest): Promise<LoginResponse> {
    return this.post<LoginResponse>('/v1/auth/login', request);
  }

  async getCharacters(accessToken: string): Promise<CharactersResponse> {
    return this.get<CharactersResponse>('/v1/characters', accessToken);
  }

  async getCatalog(accessToken: string): Promise<CharacterCatalogResponse> {
    return this.get<CharacterCatalogResponse>('/v1/characters/catalog', accessToken);
  }

  async createCharacter(accessToken: string, request: CreateCharacterRequest): Promise<CreateCharacterResponse> {
    return this.post<CreateCharacterResponse>('/v1/characters', request, accessToken);
  }

  async enterWorld(accessToken: string, request: WorldEnterRequest): Promise<WorldEnterResponse> {
    return this.post<WorldEnterResponse>('/v1/world/enter', request, accessToken);
  }

  private async get<T>(path: string, accessToken?: string): Promise<T> {
    const response = await fetch(`${this.baseUrl}${path}`, {
      headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : {},
    });
    return parseJsonResponse<T>(response);
  }

  private async post<T>(path: string, body: unknown, accessToken?: string): Promise<T> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };
    if (accessToken) {
      headers.Authorization = `Bearer ${accessToken}`;
    }
    const response = await fetch(`${this.baseUrl}${path}`, {
      method: 'POST',
      headers,
      body: JSON.stringify(body),
    });
    return parseJsonResponse<T>(response);
  }
}

const isServerMessage = (value: unknown): value is GameplayServerMessage => {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const candidate = value as Partial<GameplayServerMessage>;
  return typeof candidate.kind === 'string' && typeof candidate.emitted_at_ms === 'number';
};

export class GameplaySessionClient {
  private socket: WebSocket | null = null;
  private onMessage: ((message: GameplayServerMessage) => void) | null = null;
  private onClose: (() => void) | null = null;
  private socketGeneration = 0;
  private pendingAttachGeneration = 0;
  private pendingAttachReject: ((error: Error) => void) | null = null;

  constructor(private readonly wsUrl: string) {}

  setMessageHandler(handler: (message: GameplayServerMessage) => void): void {
    this.onMessage = handler;
  }

  setCloseHandler(handler: () => void): void {
    this.onClose = handler;
  }

  sendCommand(command: GameplayCommandEnvelope): void {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
      throw new Error('Gameplay WebSocket is not ready.');
    }
    this.socket.send(JSON.stringify(command));
  }

  async attachSession(sessionId: string, attachToken: string): Promise<RegionContextMessage> {
    const generation = ++this.socketGeneration;
    const previousSocket = this.socket;
    this.rejectPendingAttach(new Error('Gameplay WebSocket closed before region_context.'));
    this.socket = new WebSocket(this.wsUrl);
    previousSocket?.close();

    return new Promise<RegionContextMessage>((resolve, reject) => {
      let resolved = false;
      const socket = this.socket;
      if (!socket) {
        reject(new Error('WebSocket was not created.'));
        return;
      }
      this.pendingAttachGeneration = generation;
      this.pendingAttachReject = reject;
      const isActiveSocket = (): boolean => this.socket === socket && this.socketGeneration === generation;
      const clearPendingAttachIfActive = (): void => {
        if (this.pendingAttachGeneration !== generation) {
          return;
        }
        this.pendingAttachGeneration = 0;
        this.pendingAttachReject = null;
      };
      const rejectIfActive = (error: Error): void => {
        if (!resolved && isActiveSocket()) {
          clearPendingAttachIfActive();
          reject(error);
        }
      };

      socket.addEventListener('open', () => {
        if (!isActiveSocket()) {
          return;
        }
        const message: AttachSessionMessage = {
          kind: 'attach_session',
          session_id: sessionId,
          attach_token: attachToken,
        };
        socket.send(JSON.stringify(message));
      });

      socket.addEventListener('message', (event) => {
        if (!isActiveSocket()) {
          return;
        }
        let payload: unknown;
        try {
          payload = JSON.parse(String(event.data)) as unknown;
        } catch {
          rejectIfActive(new Error('Server sent invalid JSON.'));
          socket.close();
          return;
        }

        if (!isServerMessage(payload)) {
          rejectIfActive(new Error('Server sent an invalid message envelope.'));
          socket.close();
          return;
        }

        this.onMessage?.(payload);

        if (payload.kind === 'reject' && !resolved) {
          rejectIfActive(new ApiClientError(payload.message, payload.reason_code, 400));
          socket.close();
          return;
        }

        if (payload.kind === 'region_context' && !resolved) {
          resolved = true;
          clearPendingAttachIfActive();
          resolve(payload);
        }
      });

      socket.addEventListener('close', () => {
        if (!isActiveSocket()) {
          return;
        }
        this.socket = null;
        this.onClose?.();
        if (!resolved) {
          clearPendingAttachIfActive();
          reject(new Error('Gameplay WebSocket closed before region_context.'));
        }
      });

      socket.addEventListener('error', () => {
        rejectIfActive(new Error('Gameplay WebSocket connection failed.'));
      });
    });
  }

  close(): void {
    this.rejectPendingAttach(new Error('Gameplay WebSocket closed before region_context.'));
    this.socketGeneration += 1;
    const socket = this.socket;
    this.socket = null;
    socket?.close();
  }

  private rejectPendingAttach(error: Error): void {
    const pendingAttachReject = this.pendingAttachReject;
    this.pendingAttachGeneration = 0;
    this.pendingAttachReject = null;
    pendingAttachReject?.(error);
  }
}
