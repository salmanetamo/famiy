import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

// Dev server: the browser talks only to this origin. /api/* is proxied to the Go API with the
// /api prefix removed (the API has no such prefix). `changeOrigin` stays OFF on purpose: the API's
// Origin check compares the Origin header's host with the request Host, so Host must remain
// the dev server's (localhost:5173), the same as the browser's Origin. The API sends no CORS headers.
const apiTarget = process.env.API_PROXY_TARGET ?? 'http://localhost:8080';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': {
        target: apiTarget,
        changeOrigin: false,
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test-setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
  },
});
