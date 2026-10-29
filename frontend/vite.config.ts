import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    rolldownOptions: {
      output: {
        // Pages are split per route (see App.tsx). The libraries and the
        // translations change less often than the app, so they get their own
        // long-cached files instead of one large entry script.
        codeSplitting: {
          groups: [
            { name: 'vendor', test: /node_modules\/(react|react-dom|react-router|scheduler|@tanstack|i18next|react-i18next)\// },
            { name: 'translations', test: /src\/(i18n|\w+Translations)\.ts$/ },
          ],
        },
      },
    },
  },
  server: {
    host: '0.0.0.0',
    port: 5173,
    proxy: {
      '/api': {
        target: process.env.VITE_API_PROXY_TARGET || 'http://localhost:8080',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
    },
  },
})
