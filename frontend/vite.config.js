import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  server: {
    port: 5173,
    proxy: { '/api': { target: process.env.API_URL || 'http://localhost:8080', changeOrigin: true } }
  },
  build: { outDir: 'dist', sourcemap: false, chunkSizeWarningLimit: 800 }
});
