<script setup lang="ts">
// Part-to-whole bar: segments separated by a 2px surface gap, 4px rounded
// outer ends, a legend that always names every class (icon/swatch + label +
// count), and a hover tooltip per segment.
import { computed } from 'vue'
import { NTooltip, NIcon } from 'naive-ui'
import type { Segment } from './types'

const props = defineProps<{ segments: Segment[]; total?: number; height?: number }>()

const sum = computed(() => props.total ?? props.segments.reduce((s, x) => s + x.value, 0))
const visible = computed(() => props.segments.filter((s) => s.value > 0))
const pct = (v: number) => (sum.value ? Math.round((v / sum.value) * 1000) / 10 : 0)
</script>

<template>
  <div class="stacked">
    <div class="bar" :style="{ height: `${height ?? 14}px` }" role="img" :aria-label="segments.map((s) => `${s.label} ${s.value}`).join('，')">
      <template v-if="sum > 0">
        <n-tooltip v-for="s in visible" :key="s.key" placement="top">
          <template #trigger>
            <div class="seg" :style="{ flexGrow: s.value, background: s.color }" />
          </template>
          {{ s.label }}：{{ s.value }} 台（{{ pct(s.value) }}%）
        </n-tooltip>
      </template>
      <div v-else class="seg empty" />
    </div>
    <div class="legend">
      <div v-for="s in segments" :key="s.key" class="item">
        <n-icon v-if="s.icon" :component="s.icon" :color="s.color" :size="14" />
        <span v-else class="swatch" :style="{ background: s.color }" />
        <span class="secondary">{{ s.label }}</span>
        <span class="count">{{ s.value }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.bar {
  display: flex;
  gap: 2px;
  width: 100%;
}
.seg {
  min-width: 4px;
  height: 100%;
  cursor: default;
}
.seg:first-child {
  border-radius: 4px 0 0 4px;
}
.seg:last-child {
  border-radius: 0 4px 4px 0;
}
.seg:only-child {
  border-radius: 4px;
}
.seg.empty {
  flex: 1;
  background: var(--track);
}
.legend {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 18px;
  margin-top: 12px;
  font-size: 13px;
}
.item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.swatch {
  width: 10px;
  height: 10px;
  border-radius: 2px;
}
.count {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
</style>
