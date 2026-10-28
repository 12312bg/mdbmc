<script setup lang="ts">
import logo from '@/assets/logo.png'

// plate puts the logo on a white tile so the blue mark stays visible on
// dark backgrounds: 'always', or only in the dark theme.
withDefaults(defineProps<{ size?: number; plate?: 'always' | 'dark' }>(), { size: 32 })
</script>

<template>
  <span
    class="app-logo"
    :class="plate && `plate-${plate}`"
    :style="{ width: `${size}px`, height: `${size}px`, '--plate-pad': `${Math.round(size * 0.14)}px` }"
  >
    <img :src="logo" alt="" draggable="false" />
  </span>
</template>

<style>
.app-logo {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  border-radius: 22%;
}
.app-logo img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.app-logo.plate-always,
:root[data-theme='dark'] .app-logo.plate-dark {
  padding: var(--plate-pad);
  background: #fff;
}
</style>
