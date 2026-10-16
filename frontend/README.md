# godrive frontend

[Nuxt](https://nuxt.com) SPA (`ssr: false`). In development, `npm run dev` proxies API traffic to the [Go](https://go.dev) server.

```bash
npm install
npm run dev
```

For production embedding:

```bash
NUXT_PUBLIC_API_BASE= npm run generate
# copy .output/public → dist/ (see repo README / Dockerfile)
```
