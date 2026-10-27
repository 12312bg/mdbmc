<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { NButton, NIcon, NDataTable, NAlert, NRadioGroup, NRadioButton, NSpin, type DataTableColumns } from 'naive-ui'
import { RefreshOutline, CheckmarkCircle, Warning, CloseCircle, HelpCircle, RemoveCircleOutline } from '@vicons/ionicons5'
import { api, errMsg, type Sensor, type SensorsResult } from '@/api'
import { formatNumber, relativeTime, sensorCategoryLabel } from '@/utils/format'

const props = defineProps<{ deviceId: number }>()

const result = ref<SensorsResult | null>(null)
const loading = ref(false)
const error = ref('')
const category = ref<string>('all')

async function load() {
  loading.value = true
  error.value = ''
  try {
    result.value = await api.bmc.sensors(props.deviceId)
  } catch (e) {
    error.value = errMsg(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

const sensors = computed(() => result.value?.sensors ?? [])
const counts = computed(() => {
  const c = { ok: 0, warning: 0, critical: 0, other: 0 }
  sensors.value.forEach((s) => {
    if (s.status === 'ok') c.ok++
    else if (s.status === 'warning') c.warning++
    else if (s.status === 'critical') c.critical++
    else c.other++
  })
  return c
})
const categories = computed(() => {
  const seen = new Set(sensors.value.map((s) => s.category))
  return ['temperature', 'fan', 'voltage', 'current', 'power', 'other'].filter((c) => seen.has(c as Sensor['category']))
})
const rows = computed(() => (category.value === 'all' ? sensors.value : sensors.value.filter((s) => s.category === category.value)))

const statusMeta: Record<string, { icon: any; color: string; label: string }> = {
  ok: { icon: CheckmarkCircle, color: 'var(--status-good)', label: '正常' },
  warning: { icon: Warning, color: 'var(--status-warning)', label: '警告' },
  critical: { icon: CloseCircle, color: 'var(--status-critical)', label: '严重' },
  unknown: { icon: HelpCircle, color: 'var(--status-unknown)', label: '无读数' },
  absent: { icon: RemoveCircleOutline, color: 'var(--status-unknown)', label: '不在位' },
}

// Reading against its thresholds: a thin meter on a neutral track with a
// tick for every threshold. The scale spans the thresholds with some margin
// so the reading sits between its limits instead of crowding one end.
function domain(s: Sensor): [number, number] | null {
  const vals = (xs: (number | null)[]) => xs.filter((x): x is number => x !== null)
  const lows = vals([s.lowerCritical, s.lowerWarning])
  const highs = vals([s.upperWarning, s.upperCritical])
  if (lows.length && highs.length) {
    const lo = Math.min(...lows)
    const hi = Math.max(...highs)
    const pad = (hi - lo) * 0.25
    return [lo - pad, hi + pad]
  }
  if (highs.length) return [0, Math.max(...highs) * 1.08]
  if (lows.length && s.reading !== null) return [0, Math.max(s.reading * 1.2, Math.max(...lows) * 3)]
  return null
}

function meter(s: Sensor) {
  if (s.reading === null) return null
  const d = domain(s)
  if (!d || d[1] <= d[0]) return null
  const pos = (v: number) => Math.max(0, Math.min(100, ((v - d[0]) / (d[1] - d[0])) * 100))
  const ticks = [s.lowerCritical, s.lowerWarning, s.upperWarning, s.upperCritical].filter((t): t is number => t !== null).map(pos)
  const color = s.status === 'critical' ? 'var(--status-critical)' : s.status === 'warning' ? 'var(--status-warning)' : 'var(--accent)'
  return h('div', { class: 'meter' }, [
    h('div', { class: 'meter-fill', style: { width: `${pos(s.reading)}%`, background: color } }),
    ...ticks.map((t) => h('div', { class: 'meter-tick', style: { left: `${t}%` } })),
  ])
}

const range = (lo: number | null, hi: number | null) => (lo === null && hi === null ? '—' : `${lo ?? '—'} / ${hi ?? '—'}`)

const columns: DataTableColumns<Sensor> = [
  { title: '名称', key: 'name', minWidth: 160, sorter: (a, b) => a.name.localeCompare(b.name) },
  { title: '类型', key: 'category', width: 80, render: (s) => sensorCategoryLabel[s.category] ?? s.category },
  {
    title: '读数', key: 'reading', width: 130, align: 'right',
    render: (s) => (s.reading === null ? h('span', { class: 'muted' }, s.text || '—') : h('span', { class: 'reading' }, formatNumber(s.reading, s.unit))),
  },
  { title: '', key: 'meter', width: 150, render: meter },
  {
    title: '状态', key: 'status', width: 96,
    sorter: (a, b) => ['critical', 'warning', 'unknown', 'absent', 'ok'].indexOf(a.status) - ['critical', 'warning', 'unknown', 'absent', 'ok'].indexOf(b.status),
    render: (s) => {
      const m = statusMeta[s.status] ?? statusMeta.unknown
      return h('span', { class: 'st' }, [h(NIcon, { component: m.icon, color: m.color, size: 15 }), m.label])
    },
  },
  { title: '下限（严重 / 警告）', key: 'lower', width: 150, render: (s) => h('span', { class: 'muted' }, range(s.lowerCritical, s.lowerWarning)) },
  { title: '上限（警告 / 严重）', key: 'upper', width: 150, render: (s) => h('span', { class: 'muted' }, range(s.upperWarning, s.upperCritical)) },
  { title: '状态描述', key: 'text', minWidth: 160, ellipsis: { tooltip: true }, render: (s) => (s.reading === null ? '' : s.text) },
]
</script>

<template>
  <div>
    <div class="bar">
      <n-radio-group v-model:value="category" size="small">
        <n-radio-button value="all">全部 {{ sensors.length }}</n-radio-button>
        <n-radio-button v-for="c in categories" :key="c" :value="c">{{ sensorCategoryLabel[c] }}</n-radio-button>
      </n-radio-group>
      <div class="summary">
        <span class="st"><n-icon :component="CheckmarkCircle" color="var(--status-good)" />正常 {{ counts.ok }}</span>
        <span class="st"><n-icon :component="Warning" color="var(--status-warning)" />警告 {{ counts.warning }}</span>
        <span class="st"><n-icon :component="CloseCircle" color="var(--status-critical)" />严重 {{ counts.critical }}</span>
        <span v-if="result" class="muted">经 {{ result.via.toUpperCase() }} 读取 · {{ relativeTime(result.fetchedAt) }}</span>
        <n-button size="small" :loading="loading" @click="load">
          <template #icon><n-icon :component="RefreshOutline" /></template>
          刷新
        </n-button>
      </div>
    </div>
    <n-alert v-if="error" type="error" :bordered="false" style="margin-bottom: 12px">{{ error }}</n-alert>
    <n-spin :show="loading && !!result">
      <n-data-table :columns="columns" :data="rows" :loading="loading && !result" size="small" :bordered="false" :max-height="560" :scroll-x="1100" />
    </n-spin>
  </div>
</template>

<style scoped>
.bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.summary {
  display: flex;
  align-items: center;
  gap: 16px;
  font-size: 13px;
}
:deep(.st),
.st {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  white-space: nowrap;
}
:deep(.reading) {
  font-variant-numeric: tabular-nums;
  font-weight: 500;
}
:deep(.meter) {
  position: relative;
  height: 6px;
  border-radius: 3px;
  background: var(--track);
}
:deep(.meter-fill) {
  height: 100%;
  border-radius: 0 3px 3px 0;
}
:deep(.meter-tick) {
  position: absolute;
  top: -3px;
  width: 2px;
  height: 12px;
  background: var(--text-3);
  border-radius: 1px;
  transform: translateX(-1px);
}
</style>
