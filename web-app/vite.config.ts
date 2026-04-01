import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
    plugins: [react()],
    test: {
        environment: 'jsdom',
        setupFiles: './src/test/setup.ts',
    },
    server: {
        port: 5173,
        proxy: {
            '/api': {
                target: 'http://localhost:8080', // Default backend port
                changeOrigin: true,
            }
        }
    }
})
