<script setup lang="ts">
defineProps<{ size?: number; inverse?: boolean }>()
</script>

<template>
  <svg :width="size ?? 32" :height="size ?? 32" viewBox="0 0 32 32" aria-hidden="true">
    <rect x="1" y="1" width="30" height="30" rx="8" fill="#2a78d6" />
    <rect x="7" y="8" width="18" height="5" rx="1.5" fill="#fff" opacity="0.95" />
    <rect x="7" y="15" width="18" height="5" rx="1.5" fill="#fff" opacity="0.75" />
    <rect x="7" y="22" width="18" height="3" rx="1.5" fill="#fff" opacity="0.55" />
    <circle cx="22" cy="10.5" r="1.3" fill="#0ca30c" />
    <circle cx="22" cy="17.5" r="1.3" fill="#0ca30c" />
  </svg>
</template>
