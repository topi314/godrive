<template>
  <div class="perm-legend-block">
    <ul class="perm-legend" aria-label="Permission legend">
      <li
        v-for="b in PERM_FLAGS"
        :key="b.bit"
        class="perm-legend-item"
        :class="'perm-' + b.short.toLowerCase()"
      >
        <strong>{{ b.short }}</strong> {{ b.label }}
      </li>
    </ul>
    <ul v-if="marks" class="perm-legend perm-legend-marks" aria-label="Permission mark legend">
      <li><strong class="mark-allow">+</strong> Allow</li>
      <li><strong class="mark-deny">−</strong> Deny</li>
      <li><strong class="mark-none">·</strong> None</li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { PERM_FLAGS } from '~/composables/permBits'

defineProps<{ marks?: boolean }>()
</script>
