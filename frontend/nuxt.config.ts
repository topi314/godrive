// https://nuxt.com/docs/api/configuration/nuxt-config
const backend = process.env.GODRIVE_API || 'http://localhost:8090'

function pathOnly(url: string) {
  return (url.split('?')[0] || '').split('#')[0] || ''
}

function looksLikeFile(url: string) {
  const path = pathOnly(url)
  const base = path.split('/').pop() || ''
  // e.g. test.mp3 — not ".env" style hidden names without a real extension
  return base.includes('.') && !base.startsWith('.')
}

// Served from Nuxt `public/` (and embedded in production). Do not proxy to Go in dev.
const nuxtPublicExact = new Set([
  '/favicon.ico',
  '/favicon.png',
  '/favicon-light.png',
  '/robots.txt',
])

function shouldProxyToGo(url: string, accept: string, method = 'GET') {
  const path = pathOnly(url)
  if (nuxtPublicExact.has(path)) return false
  const m = method.toUpperCase()
  // Mutations always hit Go on the public path (original API style).
  if (m !== 'GET' && m !== 'HEAD') return true
  if (
    path.startsWith('/api') ||
    url.includes('dl=1') ||
    url.includes('download=1') ||
    accept.includes('application/json') ||
    looksLikeFile(url)
  ) {
    return true
  }
  return false
}

export default defineNuxtConfig({
  compatibilityDate: '2025-07-15',
  devtools: { enabled: false },
  ssr: false,
  devServer: {
    port: 3000,
    host: 'localhost',
  },
  runtimeConfig: {
    public: {
      apiBase: process.env.NUXT_PUBLIC_API_BASE || '',
    },
  },
  app: {
    head: {
      title: 'godrive',
      htmlAttrs: {
        class: 'dark',
      },
      link: [
        { rel: 'icon', type: 'image/png', href: '/favicon.png' },
        { rel: 'preconnect', href: 'https://fonts.googleapis.com' },
        { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' },
        {
          rel: 'stylesheet',
          href: 'https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600;700&display=swap',
        },
      ],
    },
  },
  css: ['~/assets/css/main.css'],
  nitro: {
    preset: 'static',
  },
  routeRules: {
    '/api/**': { proxy: `${backend}/api/**` },
  },
  vite: {
    server: {
      proxy: {
        '/api': { target: backend, changeOrigin: true },
        // Share JSON + file bytes; leave HTML directory navigations to the Nuxt SPA.
        '/s': {
          target: backend,
          changeOrigin: true,
          bypass(req) {
            const url = req.url || ''
            const accept = String(req.headers.accept || '')
            if (shouldProxyToGo(url, accept, req.method)) return
            return url
          },
        },
        // File bytes / downloads / JSON listings → Go; SPA directory browsing stays on Nuxt.
        '^/(?!_nuxt/|@|node_modules/|__nuxt|\\.nuxt).*': {
          target: backend,
          changeOrigin: true,
          bypass(req) {
            const url = req.url || ''
            const accept = String(req.headers.accept || '')
            if (shouldProxyToGo(url, accept, req.method)) return
            return url
          },
        },
      },
    },
  },
})
