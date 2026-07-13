import { describe, expect, it } from 'vitest';

import { ClientApp } from './clientApp';
import { initialPreGameContext } from './preGameMachine';

class FakeMouseEvent {
  readonly type: string;
  readonly bubbles: boolean;
  target: FakeElement | null = null;

  constructor(type: string, init?: { bubbles?: boolean }) {
    this.type = type;
    this.bubbles = init?.bubbles ?? false;
  }
}

class FakeElement {
  className = '';
  children: FakeElement[] = [];
  parent: FakeElement | null = null;
  private listeners = new Map<string, Array<(event: FakeMouseEvent) => void>>();
  private attributes = new Map<string, string>();
  private textContentValue = '';
  private innerHtmlValue = '';
  readonly dataset: Record<string, string> = {};
  readonly classList = {
    toggle: () => {},
  };

  constructor(private readonly tagName: string) {}

  set innerHTML(value: string) {
    this.innerHtmlValue = value;
    this.children = [];
    const actionMatches = [...value.matchAll(/<button[^>]*data-click-action="([^"]+)"[^>]*>(.*?)<\/button>/g)];
    for (const match of actionMatches) {
      const button = new FakeElement('button');
      button.setAttribute('data-click-action', match[1]);
      button.textContent = match[2].replace(/<[^>]+>/g, '').trim();
      this.appendChild(button);
    }
  }

  get innerHTML(): string {
    return this.innerHtmlValue;
  }

  set textContent(value: string) {
    this.textContentValue = value;
  }

  get textContent(): string {
    return this.textContentValue;
  }

  addEventListener(type: string, listener: (event: FakeMouseEvent) => void): void {
    const listeners = this.listeners.get(type) ?? [];
    listeners.push(listener);
    this.listeners.set(type, listeners);
  }

  dispatchEvent(event: FakeMouseEvent): boolean {
    event.target = event.target ?? this;
    for (const listener of this.listeners.get(event.type) ?? []) {
      listener(event);
    }
    if (event.bubbles && this.parent) {
      this.parent.dispatchEvent(event);
    }
    return true;
  }

  appendChild(child: FakeElement): void {
    child.parent = this;
    this.children.push(child);
  }

  replaceChildren(...children: FakeElement[]): void {
    this.children = [];
    for (const child of children) {
      this.appendChild(child);
    }
  }

  setAttribute(name: string, value: string): void {
    this.attributes.set(name, value);
    if (name.startsWith('data-')) {
      const datasetKey = name
        .slice(5)
        .replace(/-([a-z])/g, (_, letter: string) => letter.toUpperCase());
      this.dataset[datasetKey] = value;
    }
  }

  getAttribute(name: string): string | null {
    return this.attributes.get(name) ?? null;
  }

  querySelector<T = FakeElement>(selector: string): T | null {
    for (const child of this.children) {
      if (child.matches(selector)) {
        return child as T;
      }
      const nested = child.querySelector<T>(selector);
      if (nested) {
        return nested;
      }
    }
    return null;
  }

  closest<T = FakeElement>(selector: string): T | null {
    let current: FakeElement | null = this;
    while (current) {
      if (current.matches(selector)) {
        return current as T;
      }
      current = current.parent;
    }
    return null;
  }

  private matches(selector: string): boolean {
    if (selector === '[data-click-action]') {
      return this.attributes.has('data-click-action');
    }
    const actionMatch = selector.match(/^button\[data-click-action="([^"]+)"\]$/);
    if (actionMatch) {
      return this.tagName === 'button' && this.attributes.get('data-click-action') === actionMatch[1];
    }
    return false;
  }
}

const installFakeDom = (): FakeElement => {
  const body = new FakeElement('body');
  const documentStub = {
    body,
    createElement: () => new FakeElement('div'),
  };
  Object.assign(globalThis, {
    document: documentStub,
    window: {},
    MouseEvent: FakeMouseEvent,
  });
  return body;
};

describe('ClientApp', () => {
  it('resets online flow from the runtime status bar button', () => {
    const body = installFakeDom();
    const host = new FakeElement('div');
    body.appendChild(host);

    const app = new ClientApp(host as unknown as HTMLDivElement) as any;
    app.state = {
      ...initialPreGameContext(),
      phase: 'online_ready',
      mode: 'online',
      accessToken: 'access_abc',
      accountId: 'acc_123',
      selectedCharacterId: 'char_1',
    };
    app.onlineReadModel = {
      getStateInfo: () => ({
        lastRevision: 7,
        lastRegionRevision: 3,
        nextCommandSeq: 2,
        pendingCommands: [],
        commandFlowBlocked: false,
        desyncState: 'none',
      }),
    };

    app.renderStatus();

    const resetButton = host.querySelector<FakeElement>('button[data-click-action="reset-online"]');
    expect(resetButton).not.toBeNull();

    resetButton?.dispatchEvent(new FakeMouseEvent('click', { bubbles: true }));

    expect((globalThis as any).window.__l2bgE2E?.getSnapshot()).toMatchObject({
      mode: 'online',
      phase: 'login',
      runtimeMounted: false,
      onlineState: null,
    });
  });
});
