import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

const backend = 'http://localhost:8080'

export default defineConfig({
  base: '/admin/',
  plugins: [react()],
  // one bundle for an internal tool (React + CodeMirror ≈ 620 kB)
  build: { outDir: 'dist', chunkSizeWarningLimit: 1000 },
  server: {
    // the dev-only mock (VITE_MOCK=1) imports ../contract/*.json
    fs: { allow: ['..'] },
    proxy: {
      '/admin/api': { target: backend, ws: true },
      '/v1': { target: backend, ws: true },
      '/assets': { target: backend },
    },
  },
  test: { environment: 'node', include: ['src/**/*.test.ts'] },
})
