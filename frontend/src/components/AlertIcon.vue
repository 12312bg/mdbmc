<script setup lang="ts">
// An alert's severity as an icon on a tinted disc: a cross for critical, a
// warning sign for warnings, a check for recoveries.
import { computed } from 'vue'
import { NIcon } from 'naive-ui'
import { CloseCircle, Warning, CheckmarkCircle } from '@vicons/ionicons5'
import type { AlertSeverity } from '@/api'
import { alertSeverityLabel } from '@/utils/format'

const props = defineProps<{ severity: AlertSeverity; size?: number }>()

const icon = computed(() => ({ critical: CloseCircle, warning: Warning, ok: CheckmarkCircle })[props.severity])
const color = computed(() => ({ critical: 'var(--status-critical)', warning: 'var(--status-warning)', ok: 'var(--status-good)' })[props.severity])
</script>

<template>
  <span class="alert-icon" :style="{ '--c': color, '--s': `${size ?? 28}px` }" :title="alertSeverityLabel[severity]">
    <n-icon :component="icon" :color="color" :size="Math.round((size ?? 28) * 0.6)" />
  </span>
</template>

<style scoped>
.alert-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: none;
  width: var(--s);
  height: var(--s);
  border-radius: 50%;
  background: color-mix(in srgb, var(--c) 13%, transparent);
}
</style>
