<template>
  <BrowserView :base-path="shareBase" :share-id="id" />
</template>

<script setup lang="ts">
import BrowserView from '~/components/BrowserView.vue'

definePageMeta({
  key: route => 'share:' + String(route.params.id || ''),
})

const route = useRoute()
const router = useRouter()

const id = computed(() => String(route.params.id || ''))

/** Resolve share-relative segments; null means an escape above the share root. */
function resolveShareSegments(raw: string[]): string[] | null {
  const out: string[] = []
  for (const seg of raw) {
    if (!seg || seg === '.') continue
    if (seg === '..') {
      if (out.length === 0) return null
      out.pop()
      continue
    }
    if (seg.includes('\\') || seg.includes('\0')) return null
    out.push(seg)
  }
  return out
}

const restSegments = computed(() => {
  const p = route.params.path
  if (!p) return [] as string[]
  const raw = (Array.isArray(p) ? p : [String(p)])
    .flatMap(s => String(s).split('/'))
    .filter(Boolean)
  return resolveShareSegments(raw) ?? null
})

const rest = computed(() => (restSegments.value ? restSegments.value.join('/') : ''))
const shareBase = computed(() => '/s/' + id.value + (rest.value ? '/' + rest.value : ''))

// /s/{id}/../… (or encoded ..) → bounce back to the share root.
watch(
  [id, () => route.params.path],
  async () => {
    if (!id.value) return
    const p = route.params.path
    if (!p) return
    const raw = (Array.isArray(p) ? p : [String(p)])
      .flatMap(s => String(s).split('/'))
      .filter(Boolean)
    const resolved = resolveShareSegments(raw)
    if (resolved === null) {
      await router.replace('/s/' + id.value)
      return
    }
    const want = resolved.join('/')
    const have = raw.join('/')
    // Normalize ./foo or foo/../bar within the share.
    if (want !== have) {
      await router.replace('/s/' + id.value + (want ? '/' + want : ''))
    }
  },
  { immediate: true },
)
</script>
