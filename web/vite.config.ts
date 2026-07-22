import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The build output (dist/) is embedded into the Go binary via go:embed.
// In dev, Vite serves the SPA and proxies /api to the Go server on :8080.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
