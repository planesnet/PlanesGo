import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// En desarrollo (npm run dev) la API la sirve el backend Go en 127.0.0.1:7801.
export default defineConfig({
  plugins: [react()],
  // El backend solo acepta Host y Origin locales en su puerto (defensa contra DNS rebinding).
  server: { proxy: { '/api': { target: 'http://127.0.0.1:7801', changeOrigin: true, headers: { origin: 'http://127.0.0.1:7801' } } } },
  build: { outDir: 'dist', emptyOutDir: true },
});
