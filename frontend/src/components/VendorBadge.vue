<script setup lang="ts">
// The vendor's logo (or wordmark) on a light plate, followed by its name.
// Brand colours stay inside the plate, away from the status colours; the
// plate stays light in the dark theme so the logos keep their colours.
import { computed } from 'vue'
import { vendorMarks } from './vendorMarks'

const props = defineProps<{ vendor: string; name?: string; size?: 'small' | 'medium' }>()

const mark = computed(() => vendorMarks[props.vendor])
const label = computed(() => props.name || (props.vendor ? '通用' : '未识别'))
const wordStyle = computed(() => {
  const m = mark.value
  if (m?.kind !== 'word') return undefined
  return {
    color: m.color,
    fontSize: `calc(${m.size}px * var(--s))`,
    fontStyle: m.italic ? 'italic' : undefined,
    letterSpacing: m.spacing ? `${m.spacing}em` : undefined,
  }
})
</script>

<template>
  <span class="vendor" :class="size">
    <span class="tile" :class="{ plain: !mark }" :title="label">
      <svg v-if="mark?.kind === 'logo'" class="logo" :viewBox="mark.viewBox" :fill="mark.color" aria-hidden="true">
        <path :d="mark.path" />
      </svg>
      <span v-else-if="mark?.kind === 'word'" class="word" :style="wordStyle">{{ mark.text }}</span>
      <svg
        v-else-if="vendor"
        class="generic"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="1.8"
        stroke-linecap="round"
        aria-hidden="true"
      >
        <rect x="3.5" y="4" width="17" height="7" rx="1.6" />
        <rect x="3.5" y="13" width="17" height="7" rx="1.6" />
        <path d="M7 7.5h.01M7 16.5h.01M11 7.5h6M11 16.5h6" />
      </svg>
      <span v-else class="unknown">?</span>
    </span>
    <span class="label">{{ label }}</span>
  </span>
</template>

<style scoped>
.vendor {
  --s: 1;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.medium {
  --s: 1.25;
}
.tile {
  flex: none;
  width: calc(42px * var(--s));
  height: calc(22px * var(--s));
  border-radius: 5px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  background: #fff;
  border: 1px solid var(--border);
}
:global(:root[data-theme='dark']) .tile:not(.plain) {
  background: #eceef1;
  border-color: transparent;
}
.tile.plain {
  background: var(--surface-2);
  color: var(--text-3);
}
.logo {
  width: calc(34px * var(--s));
  height: calc(16px * var(--s));
}
.word {
  font-family: 'Segoe UI', Arial, 'Microsoft YaHei UI', 'Microsoft YaHei', 'PingFang SC', sans-serif;
  font-weight: 800;
  line-height: 1;
  white-space: nowrap;
}
.generic {
  width: calc(16px * var(--s));
  height: calc(16px * var(--s));
}
.unknown {
  font-size: calc(12px * var(--s));
  font-weight: 700;
}
.label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
