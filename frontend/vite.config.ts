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
            // Exam and routine texts are about a third of all translations;
            // their own file keeps each chunk under the 500 kB warning.
            { name: 'translations-exams', test: /src\/(exam\w*|marks|result|e7|routine\w*|marksheetAdmit)Translations\.ts$/, priority: 2 },
            { name: 'translations', test: /src\/(i18n|\w+Translations)\.ts$/, priority: 1 },
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
