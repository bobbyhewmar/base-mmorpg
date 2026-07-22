import { execFileSync } from 'node:child_process';
import { expect, test, type Page } from '@playwright/test';

const frontendAURL = process.env.L2BG_MULTI_FRONTEND_A_URL ?? 'http://localhost:15173';
const frontendBURL = process.env.L2BG_MULTI_FRONTEND_B_URL ?? 'http://localhost:15174';
const backendAURL = process.env.L2BG_MULTI_BACKEND_A_URL ?? 'http://localhost:18081';
const backendBURL = process.env.L2BG_MULTI_BACKEND_B_URL ?? 'http://localhost:18082';
const scenarioStartedAt = process.env.L2BG_MULTI_SCENARIO_STARTED_AT ?? new Date().toISOString();

type CharacterIdentity = {
  characterId: string;
  characterName: string;
  login: string;
  password: string;
};

const compose = (...args: string[]): string =>
  execFileSync('docker', ['compose', '--profile', 'multi-backend', ...args], {
    cwd: process.cwd(),
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  }).trim();

const queryDatabase = (sql: string): string =>
  execFileSync(
    'docker',
    ['compose', 'exec', '-T', 'postgres', 'psql', '-U', 'l2bg', '-d', 'l2bg', '-t', '-A', '-F', '|', '-c', sql],
    {
      cwd: process.cwd(),
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'],
    },
  ).trim();

const waitForPhase = async (page: Page, phase: string): Promise<void> => {
  await page.waitForFunction(
    (expected) => (window as any).__l2bgE2E?.getSnapshot?.()?.phase === expected,
    phase,
    { timeout: 30_000 },
  );
};

const bootstrapCharacter = async (page: Page, frontendURL: string, suffix: string): Promise<CharacterIdentity> => {
  const login = `multi.${suffix}@test`;
  const password = 'hunter1234';
  const characterName = `MB${suffix}`.slice(0, 24);

  await page.goto(frontendURL);
  await page.click('[data-click-action="open-register"]');
  await page.fill('form[data-action="register"] input[name="login"]', login);
  await page.fill('form[data-action="register"] input[name="email"]', `${suffix}@example.com`);
  await page.fill('form[data-action="register"] input[name="display_name"]', `Multi ${suffix}`);
  await page.fill('form[data-action="register"] input[name="password"]', password);
  await page.fill('form[data-action="register"] input[name="password_confirm"]', password);
  await page.click('form[data-action="register"] button[type="submit"]');
  await expect(page.locator('form[data-action="login"]')).toBeVisible();

  await page.fill('form[data-action="login"] input[name="login"]', login);
  await page.fill('form[data-action="login"] input[name="password"]', password);
  await page.click('form[data-action="login"] button[type="submit"]');
  await expect(page.locator('[data-click-action="open-create-character"]')).toBeVisible();
  await page.click('[data-click-action="open-create-character"]');
  await page.fill('form[data-action="create-character"] input[name="name"]', characterName);
  await page.click('form[data-action="create-character"] button[type="submit"]');
  await expect(page.locator('.character-card', { hasText: characterName })).toBeVisible();
  await page.click(`.character-card:has-text("${characterName}")`);
  await page.click('[data-click-action="enter-world"]');
  await waitForPhase(page, 'online_ready');

  const characterId = await page.evaluate(() => (window as any).__l2bgE2E?.getSnapshot?.()?.selectedCharacterId ?? '');
  expect(characterId).not.toBe('');
  return { characterId, characterName, login, password };
};

const loginExistingCharacter = async (page: Page, frontendURL: string, identity: CharacterIdentity): Promise<void> => {
  await page.goto(frontendURL);
  await page.fill('form[data-action="login"] input[name="login"]', identity.login);
  await page.fill('form[data-action="login"] input[name="password"]', identity.password);
  await page.click('form[data-action="login"] button[type="submit"]');
  await expect(page.locator('.character-card', { hasText: identity.characterName })).toBeVisible();
  await page.click(`.character-card:has-text("${identity.characterName}")`);
  await page.click('[data-click-action="enter-world"]');
  await waitForPhase(page, 'online_ready');
};

const waitForRemotePlayer = async (page: Page, characterId: string, visible: boolean): Promise<void> => {
  await page.waitForFunction(
    ({ characterId, visible }) =>
      Boolean((window as any).__l2bgE2E?.getWorldState?.()?.otherPlayers?.[characterId]) === visible,
    { characterId, visible },
    { timeout: 30_000 },
  );
};

const waitForLog = async (page: Page, text: string): Promise<void> => {
  await page.waitForFunction(
    (expected) =>
      ((window as any).__l2bgE2E?.getWorldState?.()?.logs ?? []).some((entry: any) =>
        String(entry.text ?? '').includes(expected),
      ),
    text,
    { timeout: 30_000 },
  );
};

const hasLog = async (page: Page, text: string): Promise<boolean> =>
  page.evaluate((expected) =>
    ((window as any).__l2bgE2E?.getWorldState?.()?.logs ?? []).some((entry: any) =>
      String(entry.text ?? '').includes(expected),
    ),
  text);

const waitForHealth = async (url: string): Promise<void> => {
  await expect
    .poll(async () => {
      try {
        return (await fetch(`${url}/healthz`, { signal: AbortSignal.timeout(5000) })).ok;
      } catch {
        return false;
      }
    }, { timeout: 60_000 })
    .toBe(true);
};

const generateMovementLoad = async (pageA: Page, pageB: Page, count: number): Promise<void> => {
  const [originA, originB] = await Promise.all([
    pageA.evaluate(() => (window as any).__l2bgE2E.getWorldState().player.position),
    pageB.evaluate(() => (window as any).__l2bgE2E.getWorldState().player.position),
  ]);
  for (let index = 0; index < count; index += 1) {
    const sign = index % 2 === 0 ? 1 : -1;
    await Promise.all([
      pageA.evaluate(
        ({ origin, sign, index }) =>
          (window as any).__l2bgE2E.sendMoveIntent({ x: origin.x + sign * (1.5 + (index % 3) * 0.2), z: origin.z + sign * 0.8 }),
        { origin: originA, sign, index },
      ),
      pageB.evaluate(
        ({ origin, sign, index }) =>
          (window as any).__l2bgE2E.sendMoveIntent({ x: origin.x - sign * (1.2 + (index % 3) * 0.2), z: origin.z + sign * 0.9 }),
        { origin: originB, sign, index },
      ),
    ]);
    await pageA.waitForTimeout(25);
  }
};

test('valida fanout regional real, fault recovery, TTL e métricas entre dois backends', async ({ browser }) => {
  test.setTimeout(600_000);
  const suffix = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
  const contextA = await browser.newContext();
  const contextB = await browser.newContext();
  const pageA = await contextA.newPage();
  const pageB = await contextB.newPage();

  try {
    const [actorA, actorB] = await Promise.all([
      bootstrapCharacter(pageA, frontendAURL, `${suffix}a`),
      bootstrapCharacter(pageB, frontendBURL, `${suffix}b`),
    ]);

    await Promise.all([
      waitForRemotePlayer(pageA, actorB.characterId, true),
      waitForRemotePlayer(pageB, actorA.characterId, true),
    ]);

    const ownershipRows = queryDatabase(`
      SELECT character_id, server_instance_id, fencing_token
      FROM gameplay_session_ownerships
      WHERE character_id IN ('${actorA.characterId}', '${actorB.characterId}')
        AND lease_expires_at > NOW()
      ORDER BY character_id;
    `).split('\n').filter(Boolean);
    expect(ownershipRows).toHaveLength(2);
    expect(ownershipRows.some((row) => row.includes(`${actorA.characterId}|backend-multi-a|`))).toBe(true);
    expect(ownershipRows.some((row) => row.includes(`${actorB.characterId}|backend-multi-b|`))).toBe(true);

    await generateMovementLoad(pageA, pageB, 8);
    await pageA.waitForTimeout(1000);

    const chatAToB = `region-a-${suffix}`;
    const chatBToA = `region-b-${suffix}`;
    await pageA.evaluate((text) => (window as any).__l2bgE2E.sendChatMessage('region', text), chatAToB);
    await Promise.all([waitForLog(pageA, chatAToB), waitForLog(pageB, chatAToB)]);
    await pageB.evaluate((text) => (window as any).__l2bgE2E.sendChatMessage('region', text), chatBToA);
    await Promise.all([waitForLog(pageA, chatBToA), waitForLog(pageB, chatBToA)]);

    compose('stop', 'backend-b');
    await waitForRemotePlayer(pageA, actorB.characterId, false);

    const replayedEventID = queryDatabase(`
      WITH old_event AS (
        SELECT *
        FROM gameplay_event_outbox
        WHERE event_type = 'presence.region_player_projection.v1'
          AND payload_json->>'character_id' = '${actorB.characterId}'
          AND target_character_id = '${actorA.characterId}'
        ORDER BY event_id ASC
        LIMIT 1
      )
      INSERT INTO gameplay_event_outbox (
        idempotency_key, event_type, payload_json, target_server_instance_id,
        target_region_id, target_session_id, target_character_id, created_at, available_at
      )
      SELECT idempotency_key || '/fault-old-${suffix}', event_type, payload_json,
        target_server_instance_id, target_region_id, target_session_id, target_character_id, NOW(), NOW()
      FROM old_event
      RETURNING event_id;
    `);
    expect(replayedEventID).not.toBe('');
    await pageA.waitForTimeout(750);
    await waitForRemotePlayer(pageA, actorB.characterId, false);

    const staleChat = `stale-owner-${suffix}`;
    await pageA.evaluate((text) => (window as any).__l2bgE2E.sendChatMessage('region', text), staleChat);
    await waitForLog(pageA, staleChat);
    const positionA = await pageA.evaluate(() => (window as any).__l2bgE2E.getWorldState().player.position);
    for (let index = 0; index < 8; index += 1) {
      await pageA.evaluate(
        ({ position, index }) =>
          (window as any).__l2bgE2E.sendMoveIntent({ x: position.x + (index % 2 === 0 ? 1.8 : -1.8), z: position.z + 0.6 }),
        { position: positionA, index },
      );
    }

    compose('start', 'backend-b');
    await waitForHealth(backendBURL);
    await pageA.waitForTimeout(2500);
    await loginExistingCharacter(pageB, frontendBURL, actorB);
    await Promise.all([
      waitForRemotePlayer(pageA, actorB.characterId, true),
      waitForRemotePlayer(pageB, actorA.characterId, true),
    ]);
    expect(await hasLog(pageB, staleChat)).toBe(false);

    const ownershipAfterRestart = queryDatabase(`
      SELECT server_instance_id, fencing_token
      FROM gameplay_session_ownerships
      WHERE character_id = '${actorB.characterId}' AND lease_expires_at > NOW();
    `);
    expect(ownershipAfterRestart.startsWith('backend-multi-b|')).toBe(true);
    const initialFenceB = Number(ownershipRows.find((row) => row.startsWith(`${actorB.characterId}|`))?.split('|')[2]);
    const recoveredFenceB = Number(ownershipAfterRestart.split('|')[1]);
    expect(recoveredFenceB).toBeGreaterThan(initialFenceB);

    const recoveryChat = `recovered-${suffix}`;
    await pageB.evaluate((text) => (window as any).__l2bgE2E.sendChatMessage('region', text), recoveryChat);
    await Promise.all([waitForLog(pageA, recoveryChat), waitForLog(pageB, recoveryChat)]);

    await contextB.close();
    await waitForRemotePlayer(pageA, actorB.characterId, false);

    const rawMetrics = queryDatabase(`
      SELECT
        COUNT(*) FILTER (WHERE event_type = 'presence.region_player_projection.v1'),
        COUNT(*) FILTER (WHERE event_type = 'social.chat_message.v1'),
        COALESCE(SUM(retry_count), 0),
        COUNT(*) FILTER (WHERE dead_lettered_at IS NOT NULL),
        ROUND(COALESCE(AVG(EXTRACT(EPOCH FROM (delivered_at - created_at)) * 1000)
          FILTER (WHERE delivered_at IS NOT NULL), 0)::numeric, 3),
        ROUND(COALESCE(MAX(EXTRACT(EPOCH FROM (delivered_at - created_at)) * 1000)
          FILTER (WHERE delivered_at IS NOT NULL), 0)::numeric, 3),
        (SELECT COUNT(*) FROM gameplay_event_receipts receipt
          JOIN gameplay_event_outbox event ON event.event_id = receipt.event_id
          WHERE event.created_at >= '${scenarioStartedAt}'::timestamptz),
        COUNT(*) FILTER (
          WHERE event_type = 'presence.region_player_projection.v1'
            AND payload_json->>'action' = 'despawn'
        )
      FROM gameplay_event_outbox
      WHERE created_at >= '${scenarioStartedAt}'::timestamptz;
    `);
    const [projectionVolume, chatVolume, retryCount, deadLetterCount, averageDelayMs, maximumDelayMs, receiptCount, despawnCount] =
      rawMetrics.split('|').map(Number);
    expect(projectionVolume).toBeGreaterThanOrEqual(6);
    expect(chatVolume).toBeGreaterThanOrEqual(3);
    expect(retryCount).toBeGreaterThan(0);
    expect(deadLetterCount).toBeGreaterThan(0);
    expect(receiptCount).toBeGreaterThan(0);
    expect(despawnCount).toBeGreaterThan(0);
    expect(averageDelayMs).toBeGreaterThanOrEqual(0);
    expect(maximumDelayMs).toBeGreaterThanOrEqual(averageDelayMs);

    const [metricsA, metricsB] = await Promise.all([
      fetch(`${backendAURL}/metrics`, { signal: AbortSignal.timeout(10_000) }).then((response) => response.text()),
      fetch(`${backendBURL}/metrics`, { signal: AbortSignal.timeout(10_000) }).then((response) => response.text()),
    ]);
    expect(metricsA).toContain('l2bg_region_projection_queue_capacity 4');
    expect(metricsB).toContain('l2bg_region_projection_queue_capacity 4');
    expect(`${metricsA}\n${metricsB}`).toContain('l2bg_region_projection_delivery_delay_seconds_count');
    expect(`${metricsA}\n${metricsB}`).toContain('l2bg_region_projection_delivery_delay_seconds_max');

    console.log(
      `[multi-backend] projection_events=${projectionVolume} chat_events=${chatVolume} receipts=${receiptCount} ` +
        `retry_count=${retryCount} dead_letters=${deadLetterCount} despawns=${despawnCount} ` +
        `delivery_delay_avg_ms=${averageDelayMs} delivery_delay_max_ms=${maximumDelayMs}`,
    );
  } finally {
    await contextA.close();
    await contextB.close().catch(() => undefined);
  }
});
