import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// En desarrollo (npm run dev) la API la sirve el backend Go en 127.0.0.1:7801.
export default defineConfig({
  plugins: [react()],
  server: { proxy: { '/api': 'http://127.0.0.1:7801' } },
  build: { outDir: 'dist', emptyOutDir: true },
});
