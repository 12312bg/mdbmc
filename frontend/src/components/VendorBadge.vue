<script setup lang="ts">
// A neutral monogram chip per vendor. Vendor identity is text, not color, so
// it never competes with status colors.
import { computed } from 'vue'

const props = defineProps<{ vendor: string; name?: string; size?: 'small' | 'medium' }>()

const mono: Record<string, string> = {
  supermicro: 'SM', dell: 'DL', hpe: 'HP', lenovo: 'LN', huawei: 'HW', inspur: 'IS', h3c: 'H3', generic: 'BM',
}
const text = computed(() => mono[props.vendor] ?? '?')
const label = computed(() => props.name || (props.vendor ? '通用' : '未识别'))
</script>

<template>
  <span class="vendor" :class="size">
    <span class="mono-chip">{{ text }}</span>
    <span class="label">{{ label }}</span>
  </span>
</template>

<style scoped>
.vendor {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.mono-chip {
  flex: none;
  width: 24px;
  height: 20px;
  border-radius: 4px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 10.5px;
  font-weight: 700;
  letter-spacing: 0.02em;
  color: var(--text-2);
  background: var(--surface-2);
  border: 1px solid var(--border);
}
.label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.medium .mono-chip {
  width: 30px;
  height: 24px;
  font-size: 12px;
}
</style>
