import { spawn } from 'node:child_process';
import process from 'node:process';
import { setTimeout as delay } from 'node:timers/promises';

const keepServices = process.env.L2BG_MULTI_KEEP_SERVICES === '1';
const endpoints = [
  process.env.L2BG_MULTI_FRONTEND_A_URL ?? 'http://localhost:15173',
  process.env.L2BG_MULTI_FRONTEND_B_URL ?? 'http://localhost:15174',
  `${process.env.L2BG_MULTI_BACKEND_A_URL ?? 'http://localhost:18081'}/healthz`,
  `${process.env.L2BG_MULTI_BACKEND_B_URL ?? 'http://localhost:18082'}/healthz`,
];

process.env.L2BG_MULTI_SCENARIO_STARTED_AT = new Date().toISOString();

const run = (command, args, options = {}) =>
  new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd: process.cwd(),
      env: process.env,
      stdio: 'inherit',
      shell: process.platform === 'win32',
      ...options,
    });
    child.on('exit', (code) => {
      if (code === 0 || options.allowFailure) {
        resolve();
      } else {
        reject(new Error(`${command} ${args.join(' ')} exited with ${code ?? 'unknown'}`));
      }
    });
    child.on('error', reject);
  });

const waitForHTTP = async (url, timeoutMs = 180_000) => {
  const deadline = Date.now() + timeoutMs;
  let lastError;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(5000) });
      if (response.ok) {
        return;
      }
      lastError = new Error(`HTTP ${response.status}`);
    } catch (error) {
      lastError = error;
    }
    await delay(1000);
  }
  throw new Error(`Timed out waiting for ${url}: ${lastError instanceof Error ? lastError.message : String(lastError)}`);
};

const profileArgs = ['compose', '--profile', 'multi-backend'];
const services = ['frontend-a', 'frontend-b', 'backend-a', 'backend-b'];

const main = async () => {
  console.log('[multi-backend] Building and starting isolated Compose profile...');
  try {
    await run('docker', [...profileArgs, 'build', 'backend-a', 'frontend-a']);
    await run('docker', [...profileArgs, 'up', '-d', 'backend-a', 'backend-b', 'frontend-a', 'frontend-b']);
    for (const endpoint of endpoints) {
      console.log(`[multi-backend] Waiting for ${endpoint} ...`);
      await waitForHTTP(endpoint);
    }
    await run('npx', ['playwright', 'test', 'e2e/multi-backend-fanout.spec.ts']);
  } finally {
    if (keepServices) {
      console.log('[multi-backend] Services kept running because L2BG_MULTI_KEEP_SERVICES=1.');
    } else {
      console.log('[multi-backend] Stopping only the isolated multi-backend services...');
      await run('docker', [...profileArgs, 'stop', ...services], { allowFailure: true });
      await run('docker', [...profileArgs, 'rm', '-f', ...services], { allowFailure: true });
    }
  }
};

main().catch((error) => {
  console.error('[multi-backend] Failed:', error);
  process.exitCode = 1;
});
