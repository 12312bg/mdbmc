<script setup lang="ts">
import { h, onMounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import {
  NInput, NIcon, NDataTable, NButton, NTag, NSwitch, NRadioGroup, NRadioButton, NTooltip, useMessage, type DataTableColumns,
} from 'naive-ui'
import { SearchOutline, RefreshOutline, CheckmarkDoneOutline } from '@vicons/ionicons5'
import AlertIcon from '@/components/AlertIcon.vue'
import { api, errMsg, type AlertItem } from '@/api'
import { useAlerts } from '@/stores/alerts'
import { alertKindLabel, formatTime, relativeTime } from '@/utils/format'

const alerts = useAlerts()
const message = useMessage()
const severity = ref('')
const keyword = ref('')
const rows = ref<AlertItem[]>([])
const total = ref(0)
const loading = ref(false)
const pagination = ref({ page: 1, pageSize: 50, itemCount: 0, showSizePicker: true, pageSizes: [20, 50, 100, 200] })

async function load() {
  loading.value = true
  try {
    const res = await api.alerts.list({ severity: severity.value, keyword: keyword.value, page: pagination.value.page, size: pagination.value.pageSize })
    rows.value = res.items
    total.value = res.total
    pagination.value.itemCount = res.total
    alerts.unread = res.unread
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    loading.value = false
  }
}
onMounted(load)

let t: number | undefined
watch([keyword, severity], () => {
  window.clearTimeout(t)
  t = window.setTimeout(() => {
    pagination.value.page = 1
    void load()
  }, 300)
})
// New alerts show up at the top of the first page.
watch(() => alerts.version, () => {
  if (pagination.value.page !== 1) return
  window.clearTimeout(t)
  t = window.setTimeout(load, 400)
})

const marking = ref(false)
async function markAllRead() {
  marking.value = true
  try {
    await alerts.markAllRead()
    rows.value = rows.value.map((a) => ({ ...a, unread: false }))
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    marking.value = false
  }
}

async function setPopup(on: boolean) {
  try {
    await alerts.setPopup(on)
    message.success(on ? '新告警将弹窗提醒' : '新告警不再弹窗提醒，仍会保存在告警信箱中')
  } catch (e) {
    message.error(errMsg(e))
  }
}

const columns: DataTableColumns<AlertItem> = [
  {
    key: 'severity', width: 56, align: 'center',
    render: (a) => h('span', { class: 'sev' }, [h(AlertIcon, { severity: a.severity, size: 28 }), a.unread ? h('span', { class: 'unread-dot', 'aria-label': '未读' }) : null]),
  },
  {
    title: '告警', key: 'title', minWidth: 280,
    render: (a) => h('div', { class: 'alert-text' }, [
      h('div', { class: ['alert-title', a.unread && 'is-unread'] }, a.title),
      a.detail ? h('div', { class: 'alert-detail', title: a.detail }, a.detail) : null,
    ]),
  },
  {
    title: '设备', key: 'device', width: 230,
    render: (a) => h('div', { class: 'alert-text' }, [
      a.deviceId
        ? h(RouterLink, { to: `/devices/${a.deviceId}`, class: 'dev-link' }, { default: () => a.deviceName || a.host })
        : h('span', null, a.deviceName || a.host),
      h('div', { class: 'mono muted' }, a.host),
    ]),
  },
  { title: '类型', key: 'kind', width: 80, render: (a) => h(NTag, { size: 'small', bordered: false }, { default: () => alertKindLabel[a.kind] ?? a.kind }) },
  {
    title: '时间', key: 'ts', width: 180,
    render: (a) => h('div', { class: 'alert-text' }, [h('span', { class: 'mono' }, formatTime(a.ts)), h('span', { class: 'muted rel' }, relativeTime(a.ts))]),
  },
]
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>告警信箱</h1>
      </div>
      <div class="head-actions">
        <n-tooltip>
          <template #trigger>
            <label class="popup-switch">
              <n-switch :value="alerts.popup" size="small" @update:value="setPopup" />
              <span>弹窗提醒</span>
            </label>
          </template>
          有新告警时在右下角弹出提醒，只对你的账号生效
        </n-tooltip>
        <n-button :disabled="!alerts.unread" :loading="marking" @click="markAllRead">
          <template #icon><n-icon :component="CheckmarkDoneOutline" /></template>
          全部标为已读
        </n-button>
      </div>
    </div>
    <div class="card" style="padding: 12px 14px">
      <div class="filters">
        <n-radio-group v-model:value="severity">
          <n-radio-button value="">全部</n-radio-button>
          <n-radio-button value="critical">严重</n-radio-button>
          <n-radio-button value="warning">警告</n-radio-button>
          <n-radio-button value="ok">恢复</n-radio-button>
        </n-radio-group>
        <n-input v-model:value="keyword" placeholder="搜索设备、地址或告警内容" clearable style="width: 280px">
          <template #prefix><n-icon :component="SearchOutline" /></template>
        </n-input>
        <n-button :loading="loading" @click="load">
          <template #icon><n-icon :component="RefreshOutline" /></template>
          刷新
        </n-button>
        <span class="counts">
          <template v-if="alerts.unread"><b>{{ alerts.unread }}</b> 条未读 · </template>共 {{ total }} 条
        </span>
      </div>
      <n-data-table
        remote
        :columns="columns"
        :data="rows"
        :loading="loading"
        :pagination="pagination"
        :row-key="(a: AlertItem) => a.id"
        :row-class-name="(a: AlertItem) => (a.unread ? 'row-unread' : '')"
        :bordered="false"
        size="small"
        :max-height="640"
        @update:page="(p: number) => { pagination.page = p; load() }"
        @update:page-size="(s: number) => { pagination.pageSize = s; pagination.page = 1; load() }"
      >
        <template #empty>
          <div class="empty">
            <AlertIcon severity="ok" :size="40" />
            <div>{{ keyword || severity ? '没有符合条件的告警' : '暂无告警' }}</div>
          </div>
        </template>
      </n-data-table>
    </div>
  </div>
</template>

<style scoped>
.head-actions {
  display: flex;
  align-items: center;
  gap: 16px;
}
.popup-switch {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text-2);
  cursor: pointer;
}
.filters {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.counts {
  margin-left: auto;
  font-size: 13px;
  color: var(--text-3);
  font-variant-numeric: tabular-nums;
}
.counts b {
  color: var(--text-1);
  font-weight: 600;
}
.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 32px 0;
  color: var(--text-3);
}
:deep(.sev) {
  position: relative;
  display: inline-flex;
}
:deep(.unread-dot) {
  position: absolute;
  top: -1px;
  right: -2px;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--accent);
  box-shadow: 0 0 0 2px var(--surface);
}
:deep(.alert-text) {
  display: flex;
  flex-direction: column;
  min-width: 0;
  line-height: 20px;
}
:deep(.alert-title) {
  color: var(--text-2);
}
:deep(.alert-title.is-unread) {
  font-weight: 600;
  color: var(--text-1);
}
:deep(.alert-detail) {
  font-size: 12.5px;
  color: var(--text-3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
:deep(.dev-link) {
  color: var(--accent);
  text-decoration: none;
  font-weight: 500;
}
:deep(.dev-link:hover) {
  text-decoration: underline;
}
:deep(.rel) {
  font-size: 12px;
}
:deep(.row-unread td) {
  background: color-mix(in srgb, var(--accent) 4%, var(--surface));
}
</style>
