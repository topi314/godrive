<template>
  <BrowserView :base-path="shareBase" :share-id="id" />
</template>

<script setup lang="ts">
import BrowserView from '~/components/BrowserView.vue'

const route = useRoute()
const id = computed(() => String(route.params.id || ''))
const rest = computed(() => {
  const p = route.params.path
  if (!p) return ''
  return Array.isArray(p) ? p.join('/') : String(p)
})
const shareBase = computed(() => '/s/' + id.value + (rest.value ? '/' + rest.value : ''))
</script>
