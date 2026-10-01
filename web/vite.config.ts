import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/chat': { target: 'http://localhost:3001', changeOrigin: true },
      '/events': { target: 'http://localhost:3001', changeOrigin: true },
      '/provision': { target: 'http://localhost:3001', changeOrigin: true },
      '/admin': { target: 'http://localhost:3001', changeOrigin: true },
      '/metrics': { target: 'http://localhost:3001', changeOrigin: true },
      '/analytics': { target: 'http://localhost:3001', changeOrigin: true }
    }
  }
});
