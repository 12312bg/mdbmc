<script setup lang="ts">
// The header bell: the unread count, a list of the latest alerts, and the
// pop-ups for new alerts (when this user wants them).
import { h, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NBadge, NButton, NIcon, NPopover, NSpin, useNotification, type NotificationReactive } from 'naive-ui'
import { NotificationsOutline } from '@vicons/ionicons5'
import AlertIcon from '@/components/AlertIcon.vue'
import { api, type AlertItem } from '@/api'
import { useAlerts } from '@/stores/alerts'
import { formatTime, relativeTime } from '@/utils/format'

const alerts = useAlerts()
const router = useRouter()
const route = useRoute()
const notification = useNotification()

const open = ref(false)
const loading = ref(false)
const items = ref<AlertItem[]>([])

async function onShow(v: boolean) {
  open.value = v
  if (!v) return
  loading.value = !items.value.length
  try {
    const page = await api.alerts.list({ severity: '', keyword: '', page: 1, size: 6 })
    items.value = page.items
    alerts.unread = page.unread
  } catch {
    /* the inbox page shows errors */
  } finally {
    loading.value = false
  }
}

watch(() => route.fullPath, () => (open.value = false))

async function markRead() {
  await alerts.markAllRead()
  items.value = items.value.map((a) => ({ ...a, unread: false }))
}

function go(path: string) {
  open.value = false
  void router.push(path)
}

const deviceText = (a: AlertItem) => (a.deviceName && a.deviceName !== a.host ? `${a.deviceName}（${a.host}）` : a.host)

// Pop-ups. Alerts that arrive together (a switch failing takes many BMCs
// offline at once) are shown as one summary instead of a stack.
const notifyType = { critical: 'error', warning: 'warning', ok: 'success' } as const
let pending: AlertItem[] = []
let flushTimer: number | undefined

const stop = alerts.onAlert((a) => {
  if (open.value) items.value = [a, ...items.value].slice(0, 6)
  if (!alerts.popup) return
  pending.push(a)
  flushTimer ??= window.setTimeout(flush, 600)
})
onBeforeUnmount(() => {
  stop()
  window.clearTimeout(flushTimer)
})

function flush() {
  flushTimer = undefined
  const batch = pending
  pending = []
  if (batch.length <= 3) batch.forEach(popOne)
  else popSummary(batch)
}

function action(n: () => NotificationReactive, label: string, path: string) {
  return () =>
    h(NButton, { text: true, type: 'primary', size: 'small', onClick: () => { n().destroy(); go(path) } }, { default: () => label })
}

function popOne(a: AlertItem) {
  const n: NotificationReactive = notification.create({
    type: notifyType[a.severity],
    title: a.title,
    content: () =>
      h('div', { style: 'line-height: 20px' }, [
        h('div', { style: 'font-weight: 500' }, deviceText(a)),
        a.detail ? h('div', { style: 'color: var(--text-2); margin-top: 2px' }, a.detail) : null,
      ]),
    meta: formatTime(a.ts),
    duration: a.severity === 'critical' ? 30_000 : 10_000,
    keepAliveOnHover: true,
    action: a.deviceId ? action(() => n, '查看设备', `/devices/${a.deviceId}`) : undefined,
  })
}

function popSummary(batch: AlertItem[]) {
  const count = (s: string) => batch.filter((a) => a.severity === s).length
  const parts = ([['严重', count('critical')], ['警告', count('warning')], ['恢复', count('ok')]] as const)
    .filter(([, c]) => c)
    .map(([l, c]) => `${l} ${c} 条`)
  const names = [...new Set(batch.map((a) => a.deviceName || a.host))]
  const n: NotificationReactive = notification.create({
    type: count('critical') ? 'error' : count('warning') ? 'warning' : 'success',
    title: `收到 ${batch.length} 条新告警`,
    content: () =>
      h('div', { style: 'line-height: 20px' }, [
        h('div', { style: 'font-weight: 500' }, parts.join(' · ')),
        h('div', { style: 'color: var(--text-2); margin-top: 2px' }, names.slice(0, 4).join('、') + (names.length > 4 ? ` 等 ${names.length} 台设备` : '')),
      ]),
    meta: formatTime(batch[batch.length - 1].ts),
    duration: 30_000,
    keepAliveOnHover: true,
    action: action(() => n, '打开告警信箱', '/inbox'),
  })
}
</script>

<template>
  <n-popover trigger="click" placement="bottom-end" :show="open" :show-arrow="false" raw class="bell-pop" @update:show="onShow">
    <template #trigger>
      <n-badge :value="alerts.unread" :max="99" :show="alerts.unread > 0" :offset="[-4, 4]">
        <n-button quaternary circle aria-label="告警">
          <template #icon><n-icon :component="NotificationsOutline" /></template>
        </n-button>
      </n-badge>
    </template>
    <div class="bell">
      <div class="bell-head">
        <span class="bell-title">告警</span>
        <span v-if="alerts.unread" class="bell-unread">{{ alerts.unread }} 条未读</span>
        <n-button text size="small" :disabled="!alerts.unread" class="bell-mark" @click="markRead">全部标为已读</n-button>
      </div>
      <div class="bell-list">
        <div v-if="loading" class="bell-empty"><n-spin :size="18" /></div>
        <div v-else-if="!items.length" class="bell-empty">暂无告警</div>
        <button
          v-for="a in items"
          v-else
          :key="a.id"
          type="button"
          class="bell-item"
          :class="{ unread: a.unread }"
          @click="go(a.deviceId ? `/devices/${a.deviceId}` : '/inbox')"
        >
          <AlertIcon :severity="a.severity" :size="26" />
          <span class="bell-text">
            <span class="bell-item-title">{{ a.title }}</span>
            <span class="bell-item-sub">{{ deviceText(a) }} · {{ relativeTime(a.ts) }}</span>
          </span>
          <span v-if="a.unread" class="bell-dot" aria-label="未读" />
        </button>
      </div>
      <button type="button" class="bell-foot" @click="go('/inbox')">查看全部告警</button>
    </div>
  </n-popover>
</template>

<style scoped>
.bell {
  width: 360px;
  border-radius: 10px;
  overflow: hidden;
  background: var(--surface);
  border: 1px solid var(--border);
  box-shadow: 0 8px 28px rgba(16, 24, 40, 0.16);
}
.bell-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 14px 10px;
  border-bottom: 1px solid var(--border);
}
.bell-title {
  font-weight: 600;
}
.bell-unread {
  font-size: 12px;
  color: var(--text-3);
}
.bell-mark {
  margin-left: auto;
}
.bell-list {
  max-height: 360px;
  overflow-y: auto;
}
.bell-empty {
  display: flex;
  justify-content: center;
  padding: 28px 0;
  color: var(--text-3);
  font-size: 13px;
}
.bell-item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 10px 14px;
  border: 0;
  border-bottom: 1px solid var(--border);
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.bell-item:hover {
  background: var(--surface-2);
}
.bell-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
}
.bell-item-title {
  font-size: 13px;
  color: var(--text-2);
}
.bell-item.unread .bell-item-title {
  font-weight: 600;
  color: var(--text-1);
}
.bell-item-sub {
  margin-top: 1px;
  font-size: 12px;
  color: var(--text-3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.bell-dot {
  flex: none;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--accent);
}
.bell-foot {
  display: block;
  width: 100%;
  padding: 10px 0;
  border: 0;
  background: var(--surface-2);
  color: var(--accent);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.bell-foot:hover {
  text-decoration: underline;
}
</style>
