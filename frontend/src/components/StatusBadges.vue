<script setup lang="ts">
// Power and health indicators: always a shape/icon plus a label, never color alone.
import { computed } from 'vue'
import { NIcon } from 'naive-ui'
import { CheckmarkCircle, Warning, CloseCircle, HelpCircle } from '@vicons/ionicons5'
import type { Health, PowerState } from '@/api'
import { healthLabel, powerLabel } from '@/utils/format'

const props = defineProps<{
  power?: PowerState
  health?: Health
  reachable?: boolean
  checked?: boolean
  compact?: boolean
}>()

const powerColor = computed(() => {
  if (props.reachable === false && props.checked) return 'var(--status-unknown)'
  return props.power === 'on' ? 'var(--power-on)' : props.power === 'off' ? 'var(--power-off)' : 'var(--status-unknown)'
})
const powerText = computed(() => {
  if (props.reachable === false && props.checked) return '无法连接'
  return powerLabel[props.power ?? 'unknown']
})

const healthIcon = computed(() => ({ ok: CheckmarkCircle, warning: Warning, critical: CloseCircle, unknown: HelpCircle })[props.health ?? 'unknown'])
const healthColor = computed(() => ({
  ok: 'var(--status-good)', warning: 'var(--status-warning)', critical: 'var(--status-critical)', unknown: 'var(--status-unknown)',
})[props.health ?? 'unknown'])
</script>

<template>
  <span v-if="power !== undefined" class="badge" :class="{ offline: reachable === false && checked }">
    <span class="power-ring" :style="{ borderColor: powerColor, background: power === 'on' && reachable !== false ? powerColor : 'transparent' }" />
    <span>{{ powerText }}</span>
  </span>
  <span v-else-if="health !== undefined" class="badge">
    <n-icon :component="healthIcon" :color="healthColor" :size="15" />
    <span v-if="!compact">{{ healthLabel[health] }}</span>
  </span>
</template>

<style scoped>
.badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
  color: var(--text-1);
}
.badge.offline {
  color: var(--text-3);
}
.power-ring {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  border: 2px solid;
  flex: none;
}
</style>
