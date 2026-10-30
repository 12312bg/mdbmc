<script setup lang="ts">
import { h, onMounted, ref, watch } from 'vue'
import { NInput, NIcon, NDatePicker, NDataTable, NButton, NTag, useMessage, type DataTableColumns } from 'naive-ui'
import { SearchOutline, RefreshOutline, CheckmarkCircle, CloseCircle } from '@vicons/ionicons5'
import { api, errMsg, type AuditEntry } from '@/api'
import { auditActionLabel, formatTime } from '@/utils/format'

const message = useMessage()
const keyword = ref('')
const range = ref<[number, number] | null>(null)
const rows = ref<AuditEntry[]>([])
const loading = ref(false)
const pagination = ref({ page: 1, pageSize: 50, itemCount: 0, showSizePicker: true, pageSizes: [20, 50, 100, 200] })

async function load() {
  loading.value = true
  try {
    const res = await api.admin.listAudit({
      keyword: keyword.value,
      since: range.value ? Math.floor(range.value[0] / 1000) : 0,
      until: range.value ? Math.floor(range.value[1] / 1000) + 86399 : 0,
      page: pagination.value.page,
      size: pagination.value.pageSize,
    })
    rows.value = res.items
    pagination.value.itemCount = res.total
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    loading.value = false
  }
}
onMounted(load)

let t: number | undefined
watch([keyword, range], () => {
  window.clearTimeout(t)
  t = window.setTimeout(() => {
    pagination.value.page = 1
    void load()
  }, 300)
})

const columns: DataTableColumns<AuditEntry> = [
  { title: '时间', key: 'ts', width: 170, render: (e) => h('span', { class: 'mono' }, formatTime(e.ts)) },
  { title: '用户', key: 'username', width: 120, render: (e) => e.username || h('span', { class: 'muted' }, '—') },
  { title: '操作', key: 'action', width: 150, render: (e) => h(NTag, { size: 'small', bordered: false }, { default: () => auditActionLabel[e.action] ?? e.action }) },
  { title: '对象', key: 'target', minWidth: 200, ellipsis: { tooltip: true } },
  {
    title: '结果', key: 'success', width: 86,
    render: (e) => h('span', { style: 'display:inline-flex;align-items:center;gap:4px' }, [
      h(NIcon, { component: e.success ? CheckmarkCircle : CloseCircle, color: e.success ? 'var(--status-good)' : 'var(--status-critical)', size: 15 }),
      e.success ? '成功' : '失败',
    ]),
  },
  { title: '详情', key: 'detail', minWidth: 240, ellipsis: { tooltip: true }, render: (e) => e.detail || h('span', { class: 'muted' }, '—') },
]
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>审计日志</h1>
      </div>
    </div>
    <div class="card" style="padding: 12px 14px">
      <div class="filters">
        <n-input v-model:value="keyword" placeholder="搜索用户、操作、对象或详情" clearable style="width: 300px">
          <template #prefix><n-icon :component="SearchOutline" /></template>
        </n-input>
        <n-date-picker v-model:value="range" type="daterange" clearable style="width: 280px" />
        <n-button :loading="loading" @click="load">
          <template #icon><n-icon :component="RefreshOutline" /></template>
          刷新
        </n-button>
      </div>
      <n-data-table
        remote
        :columns="columns"
        :data="rows"
        :loading="loading"
        :pagination="pagination"
        :bordered="false"
        size="small"
        :max-height="620"
        @update:page="(p: number) => { pagination.page = p; load() }"
        @update:page-size="(s: number) => { pagination.pageSize = s; pagination.page = 1; load() }"
      />
    </div>
  </div>
</template>

<style scoped>
.filters {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}
</style>
