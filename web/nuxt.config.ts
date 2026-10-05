// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  compatibilityDate: '2025-07-15',
  devtools: { enabled: false },
  ssr: false,
  app: {
    head: {
      title: 'godrive',
      link: [
        { rel: 'preconnect', href: 'https://fonts.googleapis.com' },
        { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' },
        {
          rel: 'stylesheet',
          href: 'https://fonts.googleapis.com/css2?family=Fraunces:opsz,wght@9..144,600;9..144,700&family=IBM+Plex+Sans:wght@400;500;600&display=swap',
        },
      ],
    },
  },
  css: ['~/assets/css/main.css'],
  nitro: {
    output: {
      publicDir: '../public',
    },
  },
  vite: {
    server: {
      proxy: {
        '/api': 'http://127.0.0.1:8080',
        '/s': 'http://127.0.0.1:8080',
        '/version': 'http://127.0.0.1:8080',
        '/ping': 'http://127.0.0.1:8080',
      },
    },
  },
})
