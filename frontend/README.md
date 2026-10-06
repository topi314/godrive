# godrive frontend

Nuxt SPA (`ssr: false`). In development, `npm run dev` proxies API traffic to the Go server.

```bash
npm install
npm run dev
```

For production embedding:

```bash
NUXT_PUBLIC_API_BASE= npm run generate
# copy .output/public → dist/ (see repo README / Dockerfile)
```
