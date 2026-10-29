<script setup lang="ts">
import { computed, h, onMounted, reactive, ref, watch } from 'vue'
import { NButton, NIcon, NDataTable, NAlert, NInput, NSelect, NTag, useDialog, useMessage, type DataTableColumns, type PaginationProps } from 'naive-ui'
import { RefreshOutline, SearchOutline, TrashOutline, CheckmarkCircle, Warning, CloseCircle, InformationCircleOutline } from '@vicons/ionicons5'
import { api, errMsg, type SELEntry, type SELResult } from '@/api'
import { useSession } from '@/stores/session'
import { formatTime, relativeTime } from '@/utils/format'

const props = defineProps<{ deviceId: number; deviceName: string }>()
const session = useSession()
const dialog = useDialog()
const message = useMessage()

const result = ref<SELResult | null>(null)
const loading = ref(false)
const error = ref('')
const keyword = ref('')
const severity = ref<string | null>(null)

async function load() {
  loading.value = true
  error.value = ''
  try {
    result.value = await api.bmc.sel(props.deviceId)
  } catch (e) {
    error.value = errMsg(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

const rows = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return (result.value?.entries ?? []).filter((e) => {
    if (severity.value && e.severity !== severity.value) return false
    return !kw || [e.sensor, e.sensorType, e.message].some((x) => x.toLowerCase().includes(kw))
  })
})

// Logs run to thousands of entries; show them a page at a time. The page
// size is remembered in this browser profile.
const pageSizeKey = 'mdbmc.sel.pageSize'
function savedPageSize(): number {
  try {
    const n = Number(localStorage.getItem(pageSizeKey))
    if ([20, 50, 100, 200].includes(n)) return n
  } catch {
    /* storage unavailable */
  }
  return 50
}
const pagination = reactive<PaginationProps>({
  page: 1,
  pageSize: savedPageSize(),
  pageSizes: [20, 50, 100, 200],
  showSizePicker: true,
  showQuickJumper: true,
  prefix: ({ itemCount }) => `共 ${itemCount} 条`,
  onUpdatePage: (p: number) => {
    pagination.page = p
  },
  onUpdatePageSize: (s: number) => {
    pagination.pageSize = s
    pagination.page = 1
    try {
      localStorage.setItem(pageSizeKey, String(s))
    } catch {
      /* ignore */
    }
  },
})
watch([keyword, severity, result], () => (pagination.page = 1))

const sev: Record<string, { icon: any; color: string; label: string }> = {
  critical: { icon: CloseCircle, color: 'var(--status-critical)', label: '严重' },
  warning: { icon: Warning, color: 'var(--status-warning)', label: '警告' },
  ok: { icon: CheckmarkCircle, color: 'var(--status-good)', label: '恢复' },
  info: { icon: InformationCircleOutline, color: 'var(--text-3)', label: '信息' },
}
const severityOptions = Object.entries(sev).map(([value, m]) => ({ label: m.label, value }))

const columns: DataTableColumns<SELEntry> = [
  { title: '时间', key: 'time', width: 170, sorter: (a, b) => a.time - b.time, render: (e) => h('span', { class: 'mono' }, e.time ? formatTime(e.time) : '启动前') },
  {
    title: '级别', key: 'severity', width: 84,
    render: (e) => {
      const m = sev[e.severity] ?? sev.info
      return h('span', { class: 'st' }, [h(NIcon, { component: m.icon, color: m.color, size: 15 }), m.label])
    },
  },
  { title: '传感器', key: 'sensor', width: 160, ellipsis: { tooltip: true } },
  { title: '类型', key: 'sensorType', width: 140, ellipsis: { tooltip: true } },
  { title: '事件', key: 'message', minWidth: 260, ellipsis: { tooltip: true } },
  {
    title: '方向', key: 'direction', width: 80,
    render: (e) => (e.direction ? h(NTag, { size: 'small', bordered: false, type: e.direction === 'deassert' ? 'success' : 'default' }, { default: () => (e.direction === 'assert' ? '发生' : '解除') }) : ''),
  },
  { title: '编号', key: 'id', width: 80, render: (e) => h('span', { class: 'muted mono' }, e.id) },
]

function clearLog() {
  dialog.error({
    title: `清空“${props.deviceName}”的事件日志`,
    content: 'BMC 上的系统事件日志将被永久清除，无法恢复，可能影响后续故障排查。确定继续？',
    positiveText: '清空',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await api.bmc.clearSEL(props.deviceId)
        message.success('事件日志已清空')
        await load()
      } catch (e) {
        message.error(errMsg(e))
      }
    },
  })
}
</script>

<template>
  <div>
    <div class="bar">
      <div class="filters">
        <n-input v-model:value="keyword" placeholder="搜索传感器或事件" clearable size="small" style="width: 240px">
          <template #prefix><n-icon :component="SearchOutline" /></template>
        </n-input>
        <n-select v-model:value="severity" :options="severityOptions" placeholder="级别" clearable size="small" style="width: 110px" />
      </div>
      <div class="filters">
        <span v-if="result" class="muted" style="font-size: 13px">共 {{ result.entries.length }} 条 · 经 {{ result.via.toUpperCase() }} 读取 · {{ relativeTime(result.fetchedAt) }}</span>
        <n-button size="small" :loading="loading" @click="load">
          <template #icon><n-icon :component="RefreshOutline" /></template>
          刷新
        </n-button>
        <n-button v-if="session.can('sel.clear')" size="small" type="error" ghost :disabled="!result?.entries.length" @click="clearLog">
          <template #icon><n-icon :component="TrashOutline" /></template>
          清空日志
        </n-button>
      </div>
    </div>
    <n-alert v-if="error" type="error" :bordered="false" style="margin-bottom: 12px">{{ error }}</n-alert>
    <n-data-table
      :columns="columns"
      :data="rows"
      :loading="loading"
      :pagination="pagination"
      size="small"
      :bordered="false"
      :max-height="560"
      :scroll-x="1000"
    />
  </div>
</template>

<style scoped>
.bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.filters {
  display: flex;
  align-items: center;
  gap: 8px;
}
:deep(.st) {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
</style>
