<script setup lang="ts">
// Horizontal magnitude bars in one hue: label column, bar from a shared
// baseline (square there, 4px rounded tip), value at the tip, tooltip per bar.
import { computed } from 'vue'
import { NTooltip } from 'naive-ui'
import type { CountItem } from '@/api'

const props = defineProps<{ items: CountItem[]; max?: number; unit?: string; limit?: number }>()

const shown = computed(() => {
  const items = props.items
  const limit = props.limit ?? 8
  if (items.length <= limit) return items
  const head = items.slice(0, limit - 1)
  const rest = items.slice(limit - 1).reduce((s, x) => s + x.count, 0)
  return [...head, { key: '__other', label: '其他', count: rest }]
})
const top = computed(() => props.max ?? Math.max(1, ...shown.value.map((i) => i.count)))
</script>

<template>
  <div class="bars">
    <div v-for="i in shown" :key="i.key" class="row">
      <div class="label secondary" :title="i.label">{{ i.label }}</div>
      <n-tooltip placement="top">
        <template #trigger>
          <div class="track">
            <div class="fill" :style="{ width: `${(i.count / top) * 100}%` }" />
            <span class="value">{{ i.count }}</span>
          </div>
        </template>
        {{ i.label }}：{{ i.count }} {{ unit ?? '台' }}
      </n-tooltip>
    </div>
    <div v-if="!items.length" class="muted empty">暂无数据</div>
  </div>
</template>

<style scoped>
.bars {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.row {
  display: grid;
  grid-template-columns: 112px 1fr;
  align-items: center;
  gap: 12px;
  font-size: 13px;
}
.label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.track {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 22px;
  cursor: default;
}
.fill {
  height: 12px;
  min-width: 3px;
  background: var(--accent);
  border-radius: 0 4px 4px 0;
}
.value {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--text-1);
}
.empty {
  font-size: 13px;
  padding: 8px 0;
}
</style>
