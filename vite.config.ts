import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '');
  const proxyTarget = env.L2BG_VITE_PROXY_TARGET;

  return {
    server: {
      host: '0.0.0.0',
      port: 5173,
      proxy: proxyTarget
        ? {
            '/api': {
              target: proxyTarget,
              changeOrigin: true,
              ws: true,
              rewrite: (path) => path.replace(/^\/api/, ''),
            },
          }
        : undefined,
    },
    test: {
      exclude: ['e2e/**', 'playwright.config.ts', 'node_modules/**', 'dist/**'],
    },
  };
});
