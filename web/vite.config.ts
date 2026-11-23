import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// The Go server serves dist/ from embed.FS and the API from the same origin.
// In dev, /api and /healthz go to a running `eunomia serve`.
export default defineConfig({
  plugins: [svelte()],
  // emptyOutDir clears stale hashed assets; public/.gitkeep is copied back
  // into dist so the Go embed still compiles on a fresh clone. No source
  // maps: dist is embedded in the binary.
  build: { outDir: 'dist', emptyOutDir: true, sourcemap: false },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:8080',
      '/healthz': 'http://127.0.0.1:8080',
    },
  },
});
