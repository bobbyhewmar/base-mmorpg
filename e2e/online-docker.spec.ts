import { expect, test, type Page } from '@playwright/test';

type BridgeSnapshot = {
  mode: 'local' | 'online';
  phase:
    | 'mode_select'
    | 'login'
    | 'register'
    | 'loading_account'
    | 'pending_verification'
    | 'recovery_entry'
    | 'character_list'
    | 'character_create'
    | 'entering_world'
    | 'attaching'
    | 'local_ready'
    | 'online_ready';
  error: string | null;
  selectedCharacterId: string | null;
  runtimeMounted: boolean;
  onlineState: {
    lastRevision: number;
    lastRegionRevision: number;
    nextCommandSeq: number;
    pendingCommands: Array<{ commandId: string; type: string; status: string }>;
    commandFlowBlocked: boolean;
    desyncState: string;
  } | null;
};

type WorldState = {
  player: {
    position: { x: number; z: number };
	cp: number;
	hp: number;
	mp: number;
	pvpFlagged: boolean;
	pvpKills: number;
	pkCount: number;
	karma: number;
  };
  destinationMarker: { x: number; z: number } | null;
  pendingPath: Array<{ x: number; z: number }>;
  authoritativePath: Array<{ x: number; z: number }>;
  incomingTradeOffer: {
    offerId: string;
    counterpartyCharacterId: string;
    counterpartyName: string;
    itemTemplateId: string;
    quantity: number;
  } | null;
  outgoingTradeOffer: {
    offerId: string;
    counterpartyCharacterId: string;
    counterpartyName: string;
    itemTemplateId: string;
    quantity: number;
  } | null;
	otherPlayers: Record<string, { id: string; name: string; position: { x: number; z: number }; facing: number; cp: number; hp: number; dead: boolean; pvpFlagged: boolean; karma: number }>;
  targetId: string | null;
  clan: {
    clanId: string;
    name: string;
    leaderCharacterId: string;
    members: Array<{ characterId: string; name: string; isLeader: boolean; online: boolean }>;
  } | null;
  clanInvites: Array<{ inviteId: string; clanId: string; clanName: string; expiresAtMs: number }>;
  logs: Array<{ id: string; text: string; tone: string }>;
  mobs: Record<string, { id: string; position: { x: number; z: number }; hp: number; aiState: string }>;
  loot: Record<string, { id: string; position: { x: number; z: number }; label: string }>;
  items: Record<string, { id: string; templateId: string; quantity: number; container: string; equipSlot?: 'weapon' | 'chest' | 'gloves' | 'boots' }>;
} | null;

const getBridgeSnapshot = async (page: Page): Promise<BridgeSnapshot> => {
  const snapshot = await page.evaluate(() => (window as any).__l2bgE2E?.getSnapshot?.() ?? null);
  expect(snapshot).not.toBeNull();
  return snapshot as BridgeSnapshot;
};

const getWorldState = async (page: Page): Promise<WorldState> => {
  return (await page.evaluate(() => (window as any).__l2bgE2E?.getWorldState?.() ?? null)) as WorldState;
};

const waitForBridgePhase = async (page: Page, phase: BridgeSnapshot['phase']): Promise<void> => {
  await page.waitForFunction(
    (expectedPhase) => (window as any).__l2bgE2E?.getSnapshot?.()?.phase === expectedPhase,
    phase,
  );
};

const waitForAppliedCommand = async (page: Page, commandType: string): Promise<void> => {
  await page.waitForFunction(
    (expectedType) => {
      const commands = (window as any).__l2bgE2E?.getSnapshot?.()?.onlineState?.pendingCommands ?? [];
      return commands.some((command: any) => command.type === expectedType && command.status === 'applied');
    },
    commandType,
	{ timeout: 15_000 },
  );
};

const waitForPlayerDistance = async (page: Page, target: { x: number; z: number }, maxDistance: number): Promise<void> => {
  await page.waitForFunction(
    ({ target, maxDistance }) => {
      const state = (window as any).__l2bgE2E?.getWorldState?.();
      if (!state) {
        return false;
      }
      const dx = state.player.position.x - target.x;
      const dz = state.player.position.z - target.z;
      return Math.hypot(dx, dz) <= maxDistance;
    },
    { target, maxDistance },
  );
};

const waitForItemContainer = async (
  page: Page,
  templateId: string,
  container: 'inventory' | 'equipment' | 'warehouse',
): Promise<void> => {
  await page.waitForFunction(
    ({ templateId, container }) => {
      const state = (window as any).__l2bgE2E?.getWorldState?.();
      if (!state) {
        return false;
      }
      return Object.values(state.items).some(
        (item: any) => item.templateId === templateId && item.container === container,
      );
    },
    { templateId, container },
  );
};

const waitForMobDamageOrLoot = async (page: Page, mobId: string, previousHP: number): Promise<void> => {
  await page.waitForFunction(
    ({ mobId, previousHP }) => {
      const state = (window as any).__l2bgE2E?.getWorldState?.();
      if (!state) {
        return false;
      }
      const lootCount = Object.keys(state.loot ?? {}).length;
      const mob = state.mobs?.[mobId];
      if (lootCount > 0) {
        return true;
      }
      return Boolean(mob) && mob.hp < previousHP;
    },
    { mobId, previousHP },
  );
};

const waitForVisible = async (page: Page, selector: string, timeoutMs = 2500): Promise<boolean> => {
  try {
    await page.locator(selector).waitFor({ state: 'visible', timeout: timeoutMs });
    return true;
  } catch {
    return false;
  }
};

const pageHasAuthRateLimit = async (page: Page): Promise<boolean> => {
  const text = await page.locator('body').textContent();
  return text?.includes('auth.rate_limited') ?? false;
};

const submitAuthActionWithRetry = async (
  page: Page,
  submitSelector: string,
  successSelector: string,
  attempts = 5,
): Promise<void> => {
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    await page.click(submitSelector);
    if (await waitForVisible(page, successSelector)) {
      return;
    }
    if (!(await pageHasAuthRateLimit(page))) {
      break;
    }
    await page.waitForTimeout(1200 * (attempt + 1));
  }

  await expect(page.locator(successSelector)).toBeVisible();
};

const bootstrapOnlineCharacter = async (
  page: Page,
  suffix: string,
): Promise<{ characterId: string; characterName: string; login: string; password: string }> => {
  const login = `e2e.${suffix}@test`;
  const password = 'hunter1234';
  const characterName = `E2E${suffix}`.slice(0, 24);

  await page.goto('/');
  await page.click('[data-click-action="open-register"]');

  await page.fill('form[data-action="register"] input[name="login"]', login);
  await page.fill('form[data-action="register"] input[name="display_name"]', `E2E ${suffix}`);
  await page.fill('form[data-action="register"] input[name="password"]', password);
  await submitAuthActionWithRetry(
    page,
    'form[data-action="register"] button[type="submit"]',
    'form[data-action="login"]',
  );

  await page.fill('form[data-action="login"] input[name="login"]', login);
  await page.fill('form[data-action="login"] input[name="password"]', password);
  await submitAuthActionWithRetry(
    page,
    'form[data-action="login"] button[type="submit"]',
    '[data-click-action="open-create-character"]',
  );
  await page.click('[data-click-action="open-create-character"]');

  await expect(page.locator('form[data-action="create-character"] input[name="race"]')).toHaveValue('Human');
  await expect(page.locator('form[data-action="create-character"] input[name="base_class"]')).toHaveValue('Fighter');
  await expect(page.locator('form[data-action="create-character"] input[name="sex"]')).toHaveValue('Male');
  await page.fill('form[data-action="create-character"] input[name="name"]', characterName);
  await page.click('form[data-action="create-character"] button[type="submit"]');

  await expect(page.locator('.character-card', { hasText: characterName })).toBeVisible();
  await page.click(`.character-card:has-text("${characterName}")`);
  await page.click('[data-click-action="enter-world"]');
  await waitForBridgePhase(page, 'online_ready');

  const snapshot = await getBridgeSnapshot(page);
  expect(snapshot.selectedCharacterId).not.toBeNull();
  return {
    characterId: snapshot.selectedCharacterId!,
    characterName,
    login,
    password,
  };
};

const createAndEnterCharacterForExistingAccount = async (
  page: Page,
  login: string,
  password: string,
  suffix: string,
): Promise<{ characterId: string; characterName: string }> => {
  const characterName = `E2E${suffix}`.slice(0, 24);

  await page.goto('/');
  await page.fill('form[data-action="login"] input[name="login"]', login);
  await page.fill('form[data-action="login"] input[name="password"]', password);
  await submitAuthActionWithRetry(
    page,
    'form[data-action="login"] button[type="submit"]',
    '[data-click-action="open-create-character"]',
  );
  await page.click('[data-click-action="open-create-character"]');

  await expect(page.locator('form[data-action="create-character"] input[name="race"]')).toHaveValue('Human');
  await expect(page.locator('form[data-action="create-character"] input[name="base_class"]')).toHaveValue('Fighter');
  await expect(page.locator('form[data-action="create-character"] input[name="sex"]')).toHaveValue('Male');
  await page.fill('form[data-action="create-character"] input[name="name"]', characterName);
  await page.click('form[data-action="create-character"] button[type="submit"]');

  await expect(page.locator('.character-card', { hasText: characterName })).toBeVisible();
  await page.click(`.character-card:has-text("${characterName}")`);
  await page.click('[data-click-action="enter-world"]');
  await waitForBridgePhase(page, 'online_ready');

  const snapshot = await getBridgeSnapshot(page);
  expect(snapshot.selectedCharacterId).not.toBeNull();
  return {
    characterId: snapshot.selectedCharacterId!,
    characterName,
  };
};

const loginAndEnterExistingCharacter = async (
  page: Page,
  login: string,
  password: string,
  characterName: string,
): Promise<void> => {
  await page.goto('/');
  await page.fill('form[data-action="login"] input[name="login"]', login);
  await page.fill('form[data-action="login"] input[name="password"]', password);
  await submitAuthActionWithRetry(
    page,
    'form[data-action="login"] button[type="submit"]',
    '[data-click-action="open-create-character"]',
  );
  await expect(page.locator('.character-card', { hasText: characterName })).toBeVisible();
  await page.click(`.character-card:has-text("${characterName}")`);
  await page.click('[data-click-action="enter-world"]');
  await waitForBridgePhase(page, 'online_ready');
};

test('executa o fluxo online ponta a ponta via browser real e Docker Compose', async ({ page }) => {
  test.setTimeout(180_000);

  const apiRequests: string[] = [];
  const websocketURLs: string[] = [];
  const receivedMessageKinds: string[] = [];
  const sentGameplayFrames: string[] = [];

  page.on('request', (request) => {
    if (request.url().includes('/api/')) {
      apiRequests.push(request.url());
    }
  });

  page.on('websocket', (websocket) => {
    websocketURLs.push(websocket.url());
    websocket.on('framereceived', (event) => {
      const payload = typeof event.payload === 'string' ? event.payload : event.payload.toString('utf8');
      try {
        const parsed = JSON.parse(payload) as { kind?: string };
        if (parsed.kind) {
          receivedMessageKinds.push(parsed.kind);
        }
      } catch {
        // ignore non-JSON frames
      }
    });
    websocket.on('framesent', (event) => {
      const payload = typeof event.payload === 'string' ? event.payload : event.payload.toString('utf8');
      if (payload.includes('"protocol_version"')) {
        sentGameplayFrames.push(payload);
      }
    });
  });

  await page.goto('/');

  const directCorsAllowed = await page.evaluate(async () => {
    try {
      const response = await fetch('http://localhost:8080/healthz', { mode: 'cors' });
      return response.ok;
    } catch {
      return false;
    }
  });
  expect(directCorsAllowed).toBe(true);

  const suffix = `${Date.now()}${Math.floor(Math.random() * 1000)}`;
  const login = `e2e.${suffix}@test`;
  const password = 'hunter1234';
  const characterName = `E2E${suffix}`.slice(0, 24);

  await page.click('[data-click-action="open-register"]');

  await page.fill('form[data-action="register"] input[name="login"]', login);
  await page.fill('form[data-action="register"] input[name="display_name"]', `E2E ${suffix}`);
  await page.fill('form[data-action="register"] input[name="password"]', password);
  await submitAuthActionWithRetry(
    page,
    'form[data-action="register"] button[type="submit"]',
    'form[data-action="login"]',
  );

  await page.fill('form[data-action="login"] input[name="login"]', login);
  await page.fill('form[data-action="login"] input[name="password"]', password);
  await submitAuthActionWithRetry(
    page,
    'form[data-action="login"] button[type="submit"]',
    '[data-click-action="open-create-character"]',
  );
  await page.click('[data-click-action="open-create-character"]');

  await expect(page.locator('form[data-action="create-character"] input[name="race"]')).toHaveValue('Human');
  await expect(page.locator('form[data-action="create-character"] input[name="base_class"]')).toHaveValue('Fighter');
  await expect(page.locator('form[data-action="create-character"] input[name="sex"]')).toHaveValue('Male');
  await page.fill('form[data-action="create-character"] input[name="name"]', characterName);
  await page.click('form[data-action="create-character"] button[type="submit"]');

  await expect(page.locator('.character-card', { hasText: characterName })).toBeVisible();
  await page.click(`.character-card:has-text("${characterName}")`);
  await page.click('[data-click-action="enter-world"]');

  await page.waitForFunction(() => {
    const phase = (window as any).__l2bgE2E?.getSnapshot?.()?.phase;
    return phase === 'attaching' || phase === 'online_ready';
  });

  await waitForBridgePhase(page, 'online_ready');
  await expect(page.locator('.game-shell')).toHaveCount(1);

  const readySnapshot = await getBridgeSnapshot(page);
  expect(readySnapshot.runtimeMounted).toBe(true);
  expect(readySnapshot.onlineState?.nextCommandSeq).toBe(1);
  expect(readySnapshot.onlineState?.commandFlowBlocked).toBe(false);
  expect(receivedMessageKinds).toContain('region_context');
  expect(sentGameplayFrames).toHaveLength(0);

  expect(apiRequests.some((url) => /http:\/\/(localhost|127\.0\.0\.1):5173\/api\/v1\/auth\/register/.test(url))).toBe(true);
  expect(apiRequests.some((url) => /http:\/\/(localhost|127\.0\.0\.1):5173\/api\/v1\/auth\/login/.test(url))).toBe(true);
  expect(apiRequests.some((url) => /http:\/\/(localhost|127\.0\.0\.1):5173\/api\/v1\/characters\/catalog/.test(url))).toBe(true);
  expect(apiRequests.some((url) => /http:\/\/(localhost|127\.0\.0\.1):5173\/api\/v1\/world\/enter/.test(url))).toBe(true);
  expect(apiRequests.some((url) => /:8080\/v1\//.test(url))).toBe(false);
  expect(websocketURLs.some((url) => /ws:\/\/(localhost|127\.0\.0\.1):5173\/api\/v1\/gameplay\/ws/.test(url))).toBe(true);

  const initialWorld = await getWorldState(page);
  expect(initialWorld).not.toBeNull();
  await page.evaluate(
    ({ x, z }) => (window as any).__l2bgE2E.sendMoveIntent({ x, z }),
    { x: -4, z: -11 },
  );
  await page.waitForFunction(() => {
    const state = (window as any).__l2bgE2E?.getWorldState?.();
    return Boolean(state) && state.pendingPath.length === 0 && state.authoritativePath.length >= 3;
  });

  const routedPreview = await getWorldState(page);
  expect(routedPreview?.authoritativePath.length ?? 0).toBeGreaterThanOrEqual(3);
  expect(routedPreview?.destinationMarker).toEqual({ x: -4, z: -11 });

  await waitForPlayerDistance(page, { x: -4, z: -11 }, 1.5);

  await page.evaluate(
    ({ x, z }) => (window as any).__l2bgE2E.sendMoveIntent({ x, z }),
    { x: -6, z: -6 },
  );
  await page.waitForFunction(() => {
    const state = (window as any).__l2bgE2E?.getWorldState?.();
    return Boolean(state) && state.logs.some((entry: any) => entry.text.includes('Movement failed: the destination is blocked'));
  });

  const blockedPreview = await getWorldState(page);
  expect(blockedPreview?.pendingPath).toEqual([]);
  const blockedRoute = blockedPreview?.authoritativePath ?? [];
  if (blockedRoute.length > 0) {
    expect(blockedRoute[blockedRoute.length - 1]).not.toEqual({ x: -6, z: -6 });
  }

  const mobEntry = Object.entries(initialWorld!.mobs).find(([, mob]) => mob.aiState !== 'dead');
  expect(mobEntry).toBeTruthy();
  const [mobId, mob] = mobEntry!;

  await page.evaluate(
    ({ x, z }) => (window as any).__l2bgE2E.sendMoveIntent({ x, z }),
    { x: mob.position.x - 2, z: mob.position.z },
  );
  await waitForPlayerDistance(page, { x: mob.position.x, z: mob.position.z }, 7.5);

  await page.evaluate((targetId) => (window as any).__l2bgE2E.sendSelectTarget(targetId), mobId);
  await page.waitForFunction(
    (targetId) => (window as any).__l2bgE2E?.getWorldState?.()?.targetId === targetId,
    mobId,
  );

  const crescentStrikeButton = page.locator('button.skill-btn', { hasText: 'CS' }).first();
  let afterAttackState = await getWorldState(page);
  let lootId: string | null = null;
  for (let attempt = 0; attempt < 4 && !lootId; attempt += 1) {
    const beforeAttackState = await getWorldState(page);
    await expect(crescentStrikeButton).toBeEnabled();
    await crescentStrikeButton.click();
    await waitForMobDamageOrLoot(page, mobId, beforeAttackState?.mobs[mobId]?.hp ?? 0);
    afterAttackState = await getWorldState(page);
    lootId = Object.keys(afterAttackState?.loot ?? {})[0] ?? null;
  }

  expect(lootId).not.toBeNull();

  const lootState = await getWorldState(page);
  const loot = lootState!.loot[lootId!];
  await waitForPlayerDistance(page, loot.position, 3);

  const goldBeforePickup = Object.values(lootState!.items)
    .filter((item) => item.templateId === 'duskgold')
    .reduce((total, item) => total + item.quantity, 0);

  await page.evaluate((currentLootId) => (window as any).__l2bgE2E.sendPickUpLoot(currentLootId), lootId);
  await page.waitForFunction(
    (currentLootId) => {
      const state = (window as any).__l2bgE2E?.getWorldState?.();
      return Boolean(state) && !state.loot[currentLootId];
    },
    lootId,
  );

  const postPickupState = await getWorldState(page);
  const goldAfterPickup = Object.values(postPickupState!.items)
    .filter((item) => item.templateId === 'duskgold')
    .reduce((total, item) => total + item.quantity, 0);
  expect(goldAfterPickup).toBeGreaterThan(goldBeforePickup);

  await page.evaluate(
    ({ x, z }) => (window as any).__l2bgE2E.sendMoveIntent({ x, z }),
    { x: -10, z: 4 },
  );
  await waitForPlayerDistance(page, { x: -10, z: 4 }, 1.5);

  await page.evaluate(() => (window as any).__l2bgE2E.sendBuyItem('merchant_spear_offer', 1));

  await page.waitForFunction(
    (expectedGold) => {
      const state = (window as any).__l2bgE2E?.getWorldState?.();
      if (!state) {
        return false;
      }
      const totalGold = Object.values(state.items)
        .filter((item: any) => item.templateId === 'duskgold')
        .reduce((total: number, item: any) => total + item.quantity, 0);
      const inventorySpears = Object.values(state.items).filter(
        (item: any) => item.templateId === 'ironwood_spear' && item.container === 'inventory',
      ).length;
      return totalGold === expectedGold && inventorySpears === 1;
    },
    goldAfterPickup - 8,
  );

  const purchasedSpearId =
    Object.values((await getWorldState(page))?.items ?? {}).find(
      (item) => item.templateId === 'ironwood_spear' && item.container === 'inventory',
    )?.id ?? null;
  expect(purchasedSpearId).not.toBeNull();

  await page.evaluate(
    ({ x, z }) => (window as any).__l2bgE2E.sendMoveIntent({ x, z }),
    { x: -13, z: 4 },
  );
  await waitForPlayerDistance(page, { x: -13, z: 4 }, 1.5);

  await page.evaluate((itemId) => (window as any).__l2bgE2E.sendDepositItem(itemId, 1), purchasedSpearId);
  await waitForItemContainer(page, 'ironwood_spear', 'warehouse');

  const warehouseSpearId =
    Object.values((await getWorldState(page))?.items ?? {}).find(
      (item) => item.templateId === 'ironwood_spear' && item.container === 'warehouse',
    )?.id ?? null;
  expect(warehouseSpearId).not.toBeNull();

  await page.evaluate((itemId) => (window as any).__l2bgE2E.sendWithdrawItem(itemId, 1), warehouseSpearId);
  await waitForItemContainer(page, 'ironwood_spear', 'inventory');

  const resoldSpearId =
    Object.values((await getWorldState(page))?.items ?? {}).find(
      (item) => item.templateId === 'ironwood_spear' && item.container === 'inventory',
    )?.id ?? null;
  expect(resoldSpearId).not.toBeNull();

  await page.evaluate(
    ({ x, z }) => (window as any).__l2bgE2E.sendMoveIntent({ x, z }),
    { x: -10, z: 4 },
  );
  await waitForPlayerDistance(page, { x: -10, z: 4 }, 1.5);

  await page.evaluate((itemId) => (window as any).__l2bgE2E.sendSellItem(itemId, 1), resoldSpearId);
  await page.waitForFunction(
    (expectedGold) => {
      const state = (window as any).__l2bgE2E?.getWorldState?.();
      if (!state) {
        return false;
      }
      const totalGold = Object.values(state.items)
        .filter((item: any) => item.templateId === 'duskgold')
        .reduce((total: number, item: any) => total + item.quantity, 0);
      const inventorySpears = Object.values(state.items).filter(
        (item: any) => item.templateId === 'ironwood_spear' && item.container === 'inventory',
      ).length;
      return totalGold === expectedGold && inventorySpears === 0;
    },
    goldAfterPickup - 4,
  );

  await page.evaluate(() => (window as any).__l2bgE2E.sendBuyItem('merchant_ruin_shard_bundle', 2));
  await page.waitForFunction(
    (expectedGold) => {
      const state = (window as any).__l2bgE2E?.getWorldState?.();
      if (!state) {
        return false;
      }
      const totalGold = Object.values(state.items)
        .filter((item: any) => item.templateId === 'duskgold')
        .reduce((total: number, item: any) => total + item.quantity, 0);
      const ruinShards = Object.values(state.items)
        .filter((item: any) => item.templateId === 'ruin_shard' && item.container === 'inventory')
        .reduce((total: number, item: any) => total + item.quantity, 0);
      return totalGold === expectedGold && ruinShards === 8;
    },
    goldAfterPickup - 12,
  );

  await page.evaluate(() => (window as any).__l2bgE2E.sendExchangeItem('merchant_ruinbound_greaves_exchange', 1));
  await page.waitForFunction(
    (expectedGold) => {
      const state = (window as any).__l2bgE2E?.getWorldState?.();
      if (!state) {
        return false;
      }
      const totalGold = Object.values(state.items)
        .filter((item: any) => item.templateId === 'duskgold')
        .reduce((total: number, item: any) => total + item.quantity, 0);
      const remainingShards = Object.values(state.items)
        .filter((item: any) => item.templateId === 'ruin_shard' && item.container === 'inventory')
        .reduce((total: number, item: any) => total + item.quantity, 0);
      const inventoryGreaves = Object.values(state.items).filter(
        (item: any) => item.templateId === 'ruinbound_greaves' && item.container === 'inventory',
      ).length;
      return totalGold === expectedGold && remainingShards === 2 && inventoryGreaves === 1;
    },
    goldAfterPickup - 12,
  );

  const ruinShardInventoryItemId =
    Object.values((await getWorldState(page))?.items ?? {}).find(
      (item) => item.templateId === 'ruin_shard' && item.container === 'inventory',
    )?.id ?? null;
  expect(ruinShardInventoryItemId).not.toBeNull();

  await page.evaluate(
    ({ x, z }) => (window as any).__l2bgE2E.sendMoveIntent({ x, z }),
    { x: -13, z: 4 },
  );
  await waitForPlayerDistance(page, { x: -13, z: 4 }, 1.5);

  await page.evaluate((itemId) => (window as any).__l2bgE2E.sendDepositItem(itemId, 1), ruinShardInventoryItemId);
  await page.waitForFunction(() => {
    const state = (window as any).__l2bgE2E?.getWorldState?.();
    if (!state) {
      return false;
    }
    const inventoryShards = Object.values(state.items)
      .filter((item: any) => item.templateId === 'ruin_shard' && item.container === 'inventory')
      .reduce((total: number, item: any) => total + item.quantity, 0);
    const warehouseShards = Object.values(state.items)
      .filter((item: any) => item.templateId === 'ruin_shard' && item.container === 'warehouse')
      .reduce((total: number, item: any) => total + item.quantity, 0);
    return inventoryShards === 1 && warehouseShards === 1;
  });

  const ruinShardWarehouseItemId =
    Object.values((await getWorldState(page))?.items ?? {}).find(
      (item) => item.templateId === 'ruin_shard' && item.container === 'warehouse',
    )?.id ?? null;
  expect(ruinShardWarehouseItemId).not.toBeNull();

  await page.evaluate((itemId) => (window as any).__l2bgE2E.sendWithdrawItem(itemId, 1), ruinShardWarehouseItemId);
  await page.waitForFunction(() => {
    const state = (window as any).__l2bgE2E?.getWorldState?.();
    if (!state) {
      return false;
    }
    const inventoryShards = Object.values(state.items)
      .filter((item: any) => item.templateId === 'ruin_shard' && item.container === 'inventory')
      .reduce((total: number, item: any) => total + item.quantity, 0);
    const warehouseShards = Object.values(state.items)
      .filter((item: any) => item.templateId === 'ruin_shard' && item.container === 'warehouse')
      .reduce((total: number, item: any) => total + item.quantity, 0);
    return inventoryShards === 2 && warehouseShards === 0;
  });

  const greavesInventoryItemId =
    Object.values((await getWorldState(page))?.items ?? {}).find(
      (item) => item.templateId === 'ruinbound_greaves' && item.container === 'inventory',
    )?.id ?? null;
  expect(greavesInventoryItemId).not.toBeNull();

  await page.evaluate((itemId) => (window as any).__l2bgE2E.sendEquipItem(itemId), greavesInventoryItemId);
  await waitForItemContainer(page, 'ruinbound_greaves', 'equipment');
});

test('sincroniza presença multiplayer autoritativa entre duas sessões online', async ({ browser, page }) => {
  test.setTimeout(180_000);

  const suffixBase = `${Date.now()}${Math.floor(Math.random() * 1000)}`;
  const peerPage = await browser.newPage();

  try {
    const source = await bootstrapOnlineCharacter(page, `${suffixBase}a`);
    await page.waitForFunction(
      () => Object.keys((window as any).__l2bgE2E?.getWorldState?.()?.otherPlayers ?? {}).length === 0,
    );

    const peer = await createAndEnterCharacterForExistingAccount(peerPage, source.login, source.password, `${suffixBase}b`);

    await page.waitForFunction(
      (characterId) => Boolean((window as any).__l2bgE2E?.getWorldState?.()?.otherPlayers?.[characterId]),
      peer.characterId,
    );
    await peerPage.waitForFunction(
      (characterId) => Boolean((window as any).__l2bgE2E?.getWorldState?.()?.otherPlayers?.[characterId]),
      source.characterId,
    );

    const observedPresence = await getWorldState(page);
    expect(observedPresence?.otherPlayers[peer.characterId]?.name).toBe(peer.characterName);

    const destination = { x: 12, z: -3 };
    await peerPage.evaluate((point) => (window as any).__l2bgE2E.sendMoveIntent(point), destination);

    await page.waitForFunction(
      ({ characterId, target }) => {
        const otherPlayer = (window as any).__l2bgE2E?.getWorldState?.()?.otherPlayers?.[characterId];
        if (!otherPlayer) {
          return false;
        }
        return Math.hypot(otherPlayer.position.x - target.x, otherPlayer.position.z - target.z) <= 0.25;
      },
      { characterId: peer.characterId, target: destination },
    );

    const movedPresence = await getWorldState(page);
    expect(movedPresence?.otherPlayers[peer.characterId]?.position).toEqual(destination);

    await peerPage.close();

    await page.waitForFunction(
      (characterId) => !((window as any).__l2bgE2E?.getWorldState?.()?.otherPlayers?.[characterId]),
      peer.characterId,
    );
  } finally {
    await peerPage.close().catch(() => undefined);
  }
});

test('executa trade P2P autoritativo entre duas sessÃµes online', async ({ browser, page }) => {
  test.setTimeout(180_000);

  const suffixBase = `${Date.now()}${Math.floor(Math.random() * 1000)}`;
  const peerPage = await browser.newPage();

  try {
    const source = await bootstrapOnlineCharacter(page, `${suffixBase}a`);
    const peer = await createAndEnterCharacterForExistingAccount(peerPage, source.login, source.password, `${suffixBase}b`);

    await page.waitForFunction(
      (characterId) => Boolean((window as any).__l2bgE2E?.getWorldState?.()?.otherPlayers?.[characterId]),
      peer.characterId,
    );
    await peerPage.waitForFunction(
      (characterId) => Boolean((window as any).__l2bgE2E?.getWorldState?.()?.otherPlayers?.[characterId]),
      source.characterId,
    );

    const sourceGoldItemId =
      Object.values((await getWorldState(page))?.items ?? {}).find(
        (item) => item.templateId === 'duskgold' && item.container === 'inventory',
      )?.id ?? null;
    expect(sourceGoldItemId).not.toBeNull();

    await page.evaluate(
      ({ targetCharacterId, itemId }) => (window as any).__l2bgE2E.sendOfferTradeItem(targetCharacterId, itemId, 1),
      { targetCharacterId: peer.characterId, itemId: sourceGoldItemId },
    );

    await peerPage.waitForFunction(() => Boolean((window as any).__l2bgE2E?.getWorldState?.()?.incomingTradeOffer?.offerId));

    const pendingTradeOfferId = (await getWorldState(peerPage))?.incomingTradeOffer?.offerId ?? null;
    expect(pendingTradeOfferId).not.toBeNull();

    await peerPage.evaluate((offerId) => (window as any).__l2bgE2E.sendAcceptTradeOffer(offerId), pendingTradeOfferId);

    await page.waitForFunction(() => {
      const state = (window as any).__l2bgE2E?.getWorldState?.();
      if (!state || state.outgoingTradeOffer) {
        return false;
      }
      const totalGold = Object.values(state.items)
        .filter((item: any) => item.templateId === 'duskgold' && item.container === 'inventory')
        .reduce((sum: number, item: any) => sum + item.quantity, 0);
      return totalGold === 11;
    });
    await peerPage.waitForFunction(() => {
      const state = (window as any).__l2bgE2E?.getWorldState?.();
      if (!state || state.incomingTradeOffer) {
        return false;
      }
      const totalGold = Object.values(state.items)
        .filter((item: any) => item.templateId === 'duskgold' && item.container === 'inventory')
        .reduce((sum: number, item: any) => sum + item.quantity, 0);
      return totalGold === 13;
    });

    const sourceWorld = await getWorldState(page);
    const peerWorld = await getWorldState(peerPage);
    expect(sourceWorld?.outgoingTradeOffer).toBeNull();
    expect(peerWorld?.incomingTradeOffer).toBeNull();
  } finally {
    await peerPage.close().catch(() => undefined);
  }
});

test('hardeniza o lifecycle autoritativo de clan entre dois personagens com reconnect', async ({ browser, page }) => {
  test.setTimeout(240_000);
  const suffixBase = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
  const peerPage = await browser.newPage();

  try {
    const leader = await bootstrapOnlineCharacter(page, `${suffixBase}a`);
    const peer = await bootstrapOnlineCharacter(peerPage, `${suffixBase}b`);
    const clanName = `C${suffixBase.slice(-10)}`;

    await page.waitForFunction(
      (characterId) => Boolean((window as any).__l2bgE2E?.getWorldState?.()?.otherPlayers?.[characterId]),
      peer.characterId,
    );
    await peerPage.waitForFunction(
      (characterId) => Boolean((window as any).__l2bgE2E?.getWorldState?.()?.otherPlayers?.[characterId]),
      leader.characterId,
    );

    await page.evaluate((name) => (window as any).__l2bgE2E.sendCreateClan(name), clanName);
    await page.waitForFunction(
      (expectedName) => (window as any).__l2bgE2E?.getWorldState?.()?.clan?.name === expectedName,
      clanName,
    );
    await waitForAppliedCommand(page, 'create_clan');

    const invitePeer = async (): Promise<string> => {
      await page.evaluate((targetId) => (window as any).__l2bgE2E.sendSelectTarget(targetId), peer.characterId);
      await page.waitForFunction(
        (targetId) => (window as any).__l2bgE2E?.getWorldState?.()?.targetId === targetId,
        peer.characterId,
      );
      await page.evaluate(() => (window as any).__l2bgE2E.sendInviteClanMember());
      await peerPage.waitForFunction(
        (expectedClanName) =>
          ((window as any).__l2bgE2E?.getWorldState?.()?.clanInvites ?? []).some(
            (invite: any) => invite.clanName === expectedClanName,
          ),
        clanName,
      );
      const invite = (await getWorldState(peerPage))?.clanInvites.find((entry) => entry.clanName === clanName);
      expect(invite?.inviteId).toBeTruthy();
      return invite!.inviteId;
    };

    const firstInviteID = await invitePeer();
    await waitForAppliedCommand(page, 'invite_clan_member');
    await peerPage.evaluate((inviteId) => (window as any).__l2bgE2E.sendAcceptClanInvite(inviteId), firstInviteID);
    await peerPage.waitForFunction(
      (expectedClanName) =>
        (window as any).__l2bgE2E?.getWorldState?.()?.clan?.name === expectedClanName &&
        (window as any).__l2bgE2E?.getWorldState?.()?.clan?.members?.length === 2,
      clanName,
    );
    await page.waitForFunction(() => (window as any).__l2bgE2E?.getWorldState?.()?.clan?.members?.length === 2);
    await waitForAppliedCommand(peerPage, 'accept_clan_invite');

    await loginAndEnterExistingCharacter(peerPage, peer.login, peer.password, peer.characterName);
    await peerPage.waitForFunction(
      (expectedClanName) =>
        (window as any).__l2bgE2E?.getWorldState?.()?.clan?.name === expectedClanName &&
        (window as any).__l2bgE2E?.getWorldState?.()?.clan?.members?.length === 2,
      clanName,
    );

    await peerPage.evaluate(() => (window as any).__l2bgE2E.sendLeaveClan());
    await peerPage.waitForFunction(() => (window as any).__l2bgE2E?.getWorldState?.()?.clan === null);
    await page.waitForFunction(() => (window as any).__l2bgE2E?.getWorldState?.()?.clan?.members?.length === 1);
    await waitForAppliedCommand(peerPage, 'leave_clan');

    const declinedInviteID = await invitePeer();
    await peerPage.evaluate((inviteId) => (window as any).__l2bgE2E.sendDeclineClanInvite(inviteId), declinedInviteID);
    await peerPage.waitForFunction(() => (window as any).__l2bgE2E?.getWorldState?.()?.clanInvites?.length === 0);
    await page.waitForFunction(() =>
      ((window as any).__l2bgE2E?.getWorldState?.()?.logs ?? []).some((entry: any) => entry.text.includes('declined the clan invitation')),
    );
    await waitForAppliedCommand(peerPage, 'decline_clan_invite');

    const kickInviteID = await invitePeer();
    await peerPage.evaluate((inviteId) => (window as any).__l2bgE2E.sendAcceptClanInvite(inviteId), kickInviteID);
    await page.waitForFunction(() => (window as any).__l2bgE2E?.getWorldState?.()?.clan?.members?.length === 2);
    await page.evaluate(
      (targetCharacterId) => (window as any).__l2bgE2E.sendKickClanMember(targetCharacterId),
      peer.characterId,
    );
    await peerPage.waitForFunction(() => (window as any).__l2bgE2E?.getWorldState?.()?.clan === null);
    await page.waitForFunction(() => (window as any).__l2bgE2E?.getWorldState?.()?.clan?.members?.length === 1);
    await waitForAppliedCommand(page, 'kick_clan_member');

    await page.evaluate(() => (window as any).__l2bgE2E.sendDissolveClan());
    await page.waitForFunction(() => (window as any).__l2bgE2E?.getWorldState?.()?.clan === null);
    await waitForAppliedCommand(page, 'dissolve_clan');
  } finally {
    await peerPage.close().catch(() => undefined);
  }
});

test('aplica ataque e skill PvP autoritativos entre duas sessões online', async ({ browser, page }) => {
	test.setTimeout(240_000);
	const suffixBase = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
	const peerPage = await browser.newPage();

	try {
	  const [attacker, target] = await Promise.all([
		bootstrapOnlineCharacter(page, `${suffixBase}a`),
		bootstrapOnlineCharacter(peerPage, `${suffixBase}b`),
	  ]);
	  await page.waitForFunction(
		(characterId) => Boolean((window as any).__l2bgE2E?.getWorldState?.()?.otherPlayers?.[characterId]),
		target.characterId,
		{ timeout: 15_000 },
	  );
	  await peerPage.waitForFunction(
		(characterId) => Boolean((window as any).__l2bgE2E?.getWorldState?.()?.otherPlayers?.[characterId]),
		attacker.characterId,
		{ timeout: 15_000 },
	  );

	  const attackerCombatPosition = { x: 0, z: 0 };
	  const targetCombatPosition = { x: 1, z: 0 };
	  await Promise.all([
		page.evaluate((point) => (window as any).__l2bgE2E.sendMoveIntent(point), attackerCombatPosition),
		peerPage.evaluate((point) => (window as any).__l2bgE2E.sendMoveIntent(point), targetCombatPosition),
	  ]);
	  await Promise.all([
		waitForPlayerDistance(page, attackerCombatPosition, 0.25),
		waitForPlayerDistance(peerPage, targetCombatPosition, 0.25),
	  ]);

	  const targetBeforeSelect = await getWorldState(peerPage);
	  await page.evaluate((targetId) => (window as any).__l2bgE2E.sendSelectTarget(targetId), target.characterId);
	  await page.waitForFunction(
		(targetId) => (window as any).__l2bgE2E?.getWorldState?.()?.targetId === targetId,
		target.characterId,
		{ timeout: 15_000 },
	  );
	  expect((await getWorldState(peerPage))?.player.cp).toBe(targetBeforeSelect?.player.cp);
	  expect((await getWorldState(peerPage))?.player.hp).toBe(targetBeforeSelect?.player.hp);

	  await page.evaluate(() => (window as any).__l2bgE2E.sendBasicAttack());
	  await waitForAppliedCommand(page, 'basic_attack');
	  await peerPage.waitForFunction(
		({ previousCP, previousHP }) => {
		  const player = (window as any).__l2bgE2E?.getWorldState?.()?.player;
		  return Boolean(player) && (player.cp < previousCP || player.hp < previousHP);
		},
		{ previousCP: targetBeforeSelect!.player.cp, previousHP: targetBeforeSelect!.player.hp },
		{ timeout: 15_000 },
	  );
	  await page.waitForFunction(
		() => (window as any).__l2bgE2E?.getWorldState?.()?.player?.pvpFlagged === true,
		undefined,
		{ timeout: 15_000 },
	  );

	  await loginAndEnterExistingCharacter(page, attacker.login, attacker.password, attacker.characterName);
	  await page.waitForFunction(
		() => (window as any).__l2bgE2E?.getWorldState?.()?.player?.pvpFlagged === true,
		undefined,
		{ timeout: 15_000 },
	  );
	  await page.waitForFunction(
		(characterId) => Boolean((window as any).__l2bgE2E?.getWorldState?.()?.otherPlayers?.[characterId]),
		target.characterId,
		{ timeout: 15_000 },
	  );
	  await page.evaluate((targetId) => (window as any).__l2bgE2E.sendSelectTarget(targetId), target.characterId);
	  await page.waitForFunction(
		(targetId) => (window as any).__l2bgE2E?.getWorldState?.()?.targetId === targetId,
		target.characterId,
		{ timeout: 15_000 },
	  );

	  const afterBasic = await getWorldState(peerPage);
	  await page.evaluate(() => (window as any).__l2bgE2E.sendUseSkill('crescent_strike'));
	  await waitForAppliedCommand(page, 'use_skill');
	  await peerPage.waitForFunction(
		({ previousCP, previousHP }) => {
		  const player = (window as any).__l2bgE2E?.getWorldState?.()?.player;
		  return Boolean(player) && (player.cp < previousCP || player.hp < previousHP);
		},
		{ previousCP: afterBasic!.player.cp, previousHP: afterBasic!.player.hp },
		{ timeout: 15_000 },
	  );

	  const attackerAfter = await getWorldState(page);
	  const targetAfter = await getWorldState(peerPage);
	  expect(attackerAfter?.player.pvpFlagged).toBe(true);
	  expect(attackerAfter?.player.mp).toBeLessThan(58);
	  expect(targetAfter?.player.cp ?? 0).toBeLessThan(targetBeforeSelect?.player.cp ?? 0);
	} finally {
	  await peerPage.close().catch(() => undefined);
	}
});
