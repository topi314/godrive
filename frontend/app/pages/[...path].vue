<template>
  <BrowserView :base-path="basePath" />
</template>

<script setup lang="ts">
import BrowserView from '~/components/BrowserView.vue'

const route = useRoute()
const basePath = computed(() => {
  const p = route.params.path
  if (!p) return '/'
  const joined = Array.isArray(p) ? p.join('/') : String(p)
  return '/' + joined.replace(/^\/+/, '')
})
</script>
