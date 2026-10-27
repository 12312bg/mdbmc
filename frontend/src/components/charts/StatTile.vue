<script setup lang="ts">
defineProps<{ label: string; value: number | string; hint?: string; tone?: 'default' | 'warning' | 'critical' | 'good' }>()
</script>

<template>
  <div class="tile card">
    <div class="label">{{ label }}</div>
    <div class="value">
      <span v-if="tone && tone !== 'default'" class="status-dot" :style="{ background: `var(--status-${tone})` }" />
      {{ value }}
    </div>
    <div v-if="hint" class="hint">{{ hint }}</div>
  </div>
</template>

<style scoped>
.tile {
  padding: 16px 18px;
  min-width: 0;
}
.label {
  font-size: 13px;
  color: var(--text-2);
}
.value {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 28px;
  font-weight: 600;
  line-height: 40px;
  margin-top: 4px;
  font-variant-numeric: tabular-nums;
}
.value .status-dot {
  width: 10px;
  height: 10px;
}
.hint {
  font-size: 12px;
  color: var(--text-3);
  margin-top: 2px;
}
</style>
