import { spawn } from 'node:child_process';
import process from 'node:process';
import { setTimeout as delay } from 'node:timers/promises';

const frontendURL = process.env.L2BG_E2E_BASE_URL ?? 'http://localhost:5173';
const backendHealthURL = process.env.L2BG_E2E_BACKEND_HEALTH_URL ?? 'http://localhost:8080/healthz';
const keepServices = process.env.L2BG_E2E_KEEP_SERVICES === '1';

process.env.L2BG_AUTH_RATE_LIMIT_MAX_ATTEMPTS ??= '24';
process.env.L2BG_AUTH_RATE_LIMIT_WINDOW ??= '1m';
process.env.L2BG_ATTACH_RATE_LIMIT_MAX_ATTEMPTS ??= '24';
process.env.L2BG_ATTACH_RATE_LIMIT_WINDOW ??= '1m';

const runCommand = (command, args, options = {}) =>
  new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      stdio: 'inherit',
      shell: process.platform === 'win32',
      cwd: process.cwd(),
      env: process.env,
      ...options,
    });

    child.on('exit', (code) => {
      if (code === 0 || options.allowFailure) {
        resolve();
        return;
      }
      reject(new Error(`${command} ${args.join(' ')} exited with code ${code ?? 'unknown'}`));
    });
    child.on('error', reject);
  });

const waitForHttp = async (url, timeoutMs) => {
  const deadline = Date.now() + timeoutMs;
  let lastError = null;

  while (Date.now() < deadline) {
    try {
      const response = await fetch(url, { method: 'GET' });
      if (response.ok) {
        return;
      }
      lastError = new Error(`HTTP ${response.status} from ${url}`);
    } catch (error) {
      lastError = error;
    }

    await delay(1500);
  }

  throw new Error(`Timed out waiting for ${url}: ${lastError instanceof Error ? lastError.message : String(lastError)}`);
};

const main = async () => {
  console.log('[e2e:docker] Starting Docker Compose stack...');
  await runCommand('docker', ['compose', 'up', '-d', '--build']);

  try {
    console.log(`[e2e:docker] Waiting for frontend at ${frontendURL} ...`);
    await waitForHttp(frontendURL, 180_000);
    console.log(`[e2e:docker] Waiting for backend health at ${backendHealthURL} ...`);
    await waitForHttp(backendHealthURL, 120_000);

    console.log('[e2e:docker] Running Playwright suite...');
    await runCommand('npx', ['playwright', 'test']);
  } finally {
    if (!keepServices) {
      console.log('[e2e:docker] Stopping Docker Compose stack...');
      await runCommand('docker', ['compose', 'down'], { allowFailure: true });
    } else {
      console.log('[e2e:docker] Keeping Docker Compose stack running because L2BG_E2E_KEEP_SERVICES=1.');
    }
  }
};

main().catch((error) => {
  console.error('[e2e:docker] Failed:', error);
  process.exitCode = 1;
});
