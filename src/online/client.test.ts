import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { GameplaySessionClient } from './client';

type SocketEventType = 'open' | 'message' | 'close' | 'error';

class MockWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  static instances: MockWebSocket[] = [];

  readonly sentPayloads: string[] = [];
  readyState = MockWebSocket.CONNECTING;
  private readonly listeners: Record<SocketEventType, Array<(event?: unknown) => void>> = {
    open: [],
    message: [],
    close: [],
    error: [],
  };

  constructor(readonly url: string) {
    MockWebSocket.instances.push(this);
  }

  addEventListener(type: SocketEventType, listener: (event?: unknown) => void): void {
    this.listeners[type].push(listener);
  }

  send(payload: string): void {
    this.sentPayloads.push(payload);
  }

  close(): void {
    this.readyState = MockWebSocket.CLOSED;
  }

  emitOpen(): void {
    this.readyState = MockWebSocket.OPEN;
    for (const listener of this.listeners.open) {
      listener();
    }
  }

  emitMessage(payload: unknown): void {
    for (const listener of this.listeners.message) {
      listener({ data: JSON.stringify(payload) });
    }
  }

  emitClose(): void {
    this.readyState = MockWebSocket.CLOSED;
    for (const listener of this.listeners.close) {
      listener();
    }
  }

  emitError(): void {
    for (const listener of this.listeners.error) {
      listener();
    }
  }

  static reset(): void {
    MockWebSocket.instances = [];
  }
}

const regionContext = (regionId: string) => ({
  kind: 'region_context' as const,
  emitted_at_ms: Date.now(),
  region_revision: 1,
  region_id: regionId,
  geodata_version: 'clean_plain_1024_geo_v1',
  next_command_seq: 1,
  self_position: { x: -8, z: 0 },
  known_entities: [],
});

describe('GameplaySessionClient', () => {
  const originalWebSocket = globalThis.WebSocket;

  beforeEach(() => {
    MockWebSocket.reset();
    globalThis.WebSocket = MockWebSocket as unknown as typeof WebSocket;
  });

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket;
  });

  it('ignores stale close events from a superseded attach attempt', async () => {
    const client = new GameplaySessionClient('ws://example.test/v1/gameplay/ws');
    const closeHandler = vi.fn();
    client.setCloseHandler(closeHandler);

    const firstAttach = client.attachSession('sess_1', 'attach_1');
    const firstSocket = MockWebSocket.instances[0];
    firstSocket.emitOpen();

    const secondAttach = client.attachSession('sess_2', 'attach_2');
    const secondSocket = MockWebSocket.instances[1];
    secondSocket.emitOpen();

    firstSocket.emitClose();
    expect(closeHandler).not.toHaveBeenCalled();

    secondSocket.emitMessage(regionContext('stonecross_plaza'));

    await expect(firstAttach).rejects.toThrow('Gameplay WebSocket closed before region_context.');
    await expect(secondAttach).resolves.toMatchObject({ kind: 'region_context', region_id: 'stonecross_plaza' });
    expect(closeHandler).not.toHaveBeenCalled();
  });

  it('suppresses close callbacks for intentional client shutdown', async () => {
    const client = new GameplaySessionClient('ws://example.test/v1/gameplay/ws');
    const closeHandler = vi.fn();
    client.setCloseHandler(closeHandler);

    const attach = client.attachSession('sess_1', 'attach_1');
    const socket = MockWebSocket.instances[0];
    socket.emitOpen();

    client.close();
    socket.emitClose();

    await expect(attach).rejects.toThrow('Gameplay WebSocket closed before region_context.');
    expect(closeHandler).not.toHaveBeenCalled();
  });
});
