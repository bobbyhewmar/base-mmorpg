import type { GameState } from '../domain/types';

const SAVE_KEY = 'compact-mmorpg-mvp-save';

export const localSaveAdapter = {
  load(): GameState | null {
    const raw = window.localStorage.getItem(SAVE_KEY);
    if (!raw) {
      return null;
    }

    try {
      return JSON.parse(raw) as GameState;
    } catch {
      return null;
    }
  },

  save(state: GameState): void {
    window.localStorage.setItem(SAVE_KEY, JSON.stringify(state));
  },

  clear(): void {
    window.localStorage.removeItem(SAVE_KEY);
  },
};
