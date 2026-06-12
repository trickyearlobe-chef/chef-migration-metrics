/// <reference types="vitest" />
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
  },
  build: {
    outDir: 'dist',
    sourcemap: true,
    // Single-bundle build is intentional: this is an internal admin tool served
    // over VDI, load times are a non-issue, and route-level code splitting would be
    // extra config to maintain (and re-tune) on every refactor. Raise the warning
    // threshold so the build stays quiet until the bundle grows substantially; the
    // size itself is not a concern. Revisit (React.lazy route splitting) only if a
    // real load-time problem appears.
    chunkSizeWarningLimit: 1500,
  },
  server: {
    port: 3000,
    // Proxy API and WebSocket requests to the Go backend during development.
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/api/v1/ws': {
        target: 'ws://localhost:8080',
        ws: true,
      },
    },
  },
})
