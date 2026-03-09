import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

function createProxyOptions(proxyTarget: string, proxyAPIKey: string) {
  return {
    target: proxyTarget,
    changeOrigin: true,
    configure(proxy: { on: (event: 'proxyReq', listener: (proxyReq: { setHeader(name: string, value: string): void }) => void) => void }) {
      if (!proxyAPIKey) {
        return;
      }
      proxy.on('proxyReq', (proxyReq) => {
        proxyReq.setHeader('Authorization', `Bearer ${proxyAPIKey}`);
      });
    },
  };
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const proxyTarget = env.SIMICLAW_WEB_PROXY_TARGET?.trim() || 'http://127.0.0.1:8080';
  const proxyAPIKey = env.SIMICLAW_WEB_PROXY_API_KEY?.trim() || '';

  return {
    plugins: [react(), tailwindcss()],
    server: {
      host: '127.0.0.1',
      port: 5173,
      proxy: {
        '/v1': createProxyOptions(proxyTarget, proxyAPIKey),
        '/healthz': createProxyOptions(proxyTarget, proxyAPIKey),
        '/readyz': createProxyOptions(proxyTarget, proxyAPIKey),
      },
    },
  };
});
