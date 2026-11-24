import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
    plugins: [react()],
    build: {
        rollupOptions: {
            output: {
                // Keep the framework in its own long-lived chunk so page code
                // changes do not invalidate the vendor cache, and split the
                // lazily loaded admin/reseller pages into their own chunks.
                manualChunks(id) {
                    if (id.indexOf('node_modules') !== -1) {
                        if (/node_modules\/(react|react-dom|scheduler)\//.test(id)) {
                            return 'vendor-react';
                        }
                        // react-router-dom and its engine (@remix-run/router, which
                        // declares Pop/Push/Replace itself) go in one chunk.
                        if (id.indexOf('node_modules/react-router') !== -1 || id.indexOf('node_modules/@remix-run/router') !== -1) {
                            return 'vendor-router';
                        }
                        return 'vendor';
                    }
                    return undefined;
                },
            },
        },
    },
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
