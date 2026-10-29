<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref, watch, type Component } from 'vue'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import {
  NLayout, NLayoutSider, NLayoutHeader, NLayoutContent, NMenu, NIcon, NDropdown, NButton, NTooltip, NTag, NAvatar,
  NProgress, useNotification, type MenuOption,
} from 'naive-ui'
import {
  SpeedometerOutline, ServerOutline, ScanOutline, KeyOutline, PeopleOutline, DocumentTextOutline, SettingsOutline,
  MoonOutline, SunnyOutline, ContrastOutline, LogOutOutline, LockClosedOutline, ChevronDown, RefreshOutline,
  CheckmarkCircle,
} from '@vicons/ionicons5'
import AppLogo from '@/components/AppLogo.vue'
import ChangePasswordModal from '@/components/ChangePasswordModal.vue'
import UpdateDialog from '@/components/UpdateDialog.vue'
import { api, type Permission } from '@/api'
import { useSession } from '@/stores/session'
import { useTheme } from '@/stores/theme'
import { useDevices } from '@/stores/devices'
import { useUpdate } from '@/stores/update'
import { relativeTime } from '@/utils/format'

const session = useSession()
const theme = useTheme()
const devices = useDevices()
const upd = useUpdate()
const notification = useNotification()
const route = useRoute()
const router = useRouter()

const collapsed = ref(false)
const showPassword = ref(false)

const icon = (c: Component) => () => h(NIcon, null, { default: () => h(c) })
const item = (key: string, label: string, ic: Component, perm?: Permission): MenuOption | null =>
  perm && !session.can(perm) ? null : { key, icon: icon(ic), label: () => h(RouterLink, { to: `/${key}` }, { default: () => label }) }

const menu = computed(() =>
  [
    item('dashboard', '总览', SpeedometerOutline),
    item('devices', '设备', ServerOutline),
    item('discovery', '扫描发现', ScanOutline, 'device.manage'),
    item('credentials', '凭据', KeyOutline),
    { type: 'divider', key: 'd1' } as MenuOption,
    item('users', '用户与角色', PeopleOutline, 'user.manage'),
    item('audit', '审计日志', DocumentTextOutline, 'audit.view'),
    item('settings', '系统设置', SettingsOutline),
  ].filter(Boolean) as MenuOption[],
)
const activeKey = computed(() => String(route.path.split('/')[1] || 'dashboard'))

const themeIcon = computed(() => (theme.mode === 'auto' ? ContrastOutline : theme.mode === 'dark' ? MoonOutline : SunnyOutline))
const themeTip = computed(() => ({ auto: '主题：跟随系统', light: '主题：浅色', dark: '主题：深色' })[theme.mode])

const userOptions = [
  { label: '修改密码', key: 'password', icon: icon(LockClosedOutline) },
  { type: 'divider', key: 'd' },
  { label: '退出登录', key: 'logout', icon: icon(LogOutOutline) },
]
async function onUserSelect(key: string) {
  if (key === 'password') showPassword.value = true
  if (key === 'logout') await session.logout()
}

watch(() => session.loggedIn, (v) => {
  if (!v) router.replace({ name: 'login' })
})

const forced = computed(() => !!session.user?.mustChangePassword)
// The backend refuses everything until the initial password is changed, so
// reload settings and data and remount the page afterwards.
const viewKey = ref(0)
watch(forced, async (now, before) => {
  if (before && !now) {
    await Promise.all([session.loadSettings(), devices.load(), loadUpdate()])
    viewKey.value++
  }
})

// Software update: the header shows a new version; after an update the
// program starts again and reports how it went once.
async function loadUpdate() {
  await upd.load()
  const r = upd.status?.lastResult
  if (!r) return
  if (r.success) {
    notification.success({ title: '更新完成', content: `已更新到 v${r.version}。`, duration: 8000, keepAliveOnHover: true })
  } else {
    notification.error({ title: '更新未完成', content: `未能更新到 v${r.version}：${r.error}。当前仍是 v${r.from}。`, duration: 0 })
  }
  await upd.dismissResult().catch(() => {})
}

// Idle lock: ends the session (wiping the vault key) after inactivity;
// console windows stay open.
let idleTimer: number | undefined
let lastActivity = Date.now()
function bump() {
  lastActivity = Date.now()
}
const activityEvents = ['mousemove', 'mousedown', 'keydown', 'wheel', 'touchstart']
onMounted(async () => {
  activityEvents.forEach((e) => window.addEventListener(e, bump, { passive: true }))
  idleTimer = window.setInterval(() => {
    const mins = session.settings?.idleLockMinutes ?? 0
    if (mins > 0 && Date.now() - lastActivity > mins * 60_000) {
      void session.lock(`已因 ${mins} 分钟未操作自动锁定，请重新登录`)
    }
  }, 15_000)
  devices.subscribe()
  upd.subscribe()
  if (forced.value) return
  void loadUpdate()
  await devices.load()
  try {
    const ov = await api.bmc.overview()
    devices.lastPoll = ov.lastPoll
    devices.polling = ov.polling
  } catch {
    /* ignore */
  }
})
onBeforeUnmount(() => {
  activityEvents.forEach((e) => window.removeEventListener(e, bump))
  window.clearInterval(idleTimer)
})

const tick = ref(0)
const tickTimer = window.setInterval(() => tick.value++, 15_000)
onBeforeUnmount(() => window.clearInterval(tickTimer))
const pollText = computed(() => {
  void tick.value
  if (devices.polling) return '正在刷新状态…'
  return devices.lastPoll ? `状态更新于 ${relativeTime(devices.lastPoll)}` : '尚未刷新状态'
})

async function refreshAll() {
  await api.devices.refreshStatus([])
}

const initials = computed(() => (session.user?.displayName || session.user?.username || '?').slice(0, 1).toUpperCase())
</script>

<template>
  <n-layout has-sider class="shell">
    <n-layout-sider
      :collapsed="collapsed"
      collapse-mode="width"
      :collapsed-width="64"
      :width="216"
      show-trigger="bar"
      bordered
      :native-scrollbar="false"
      class="sider"
      @collapse="collapsed = true"
      @expand="collapsed = false"
    >
      <div class="brand">
        <AppLogo :size="30" plate="dark" />
        <div v-if="!collapsed" class="brand-text">
          <div class="brand-name">明点</div>
          <div class="brand-sub">服务器管理系统</div>
        </div>
      </div>
      <n-menu :value="activeKey" :options="menu" :collapsed="collapsed" :collapsed-width="64" :collapsed-icon-size="20" :indent="20" />
      <div v-if="api.isMock && !collapsed" class="mock-flag">预览模式 · 模拟数据</div>
    </n-layout-sider>
    <n-layout>
      <n-layout-header bordered class="header">
        <div class="poll">
          <span class="status-dot" :style="{ background: devices.polling ? 'var(--accent)' : 'var(--status-good)' }" />
          <span class="muted">{{ pollText }}</span>
          <n-tooltip>
            <template #trigger>
              <n-button quaternary circle size="small" :loading="devices.polling" @click="refreshAll">
                <template #icon><n-icon :component="RefreshOutline" /></template>
              </n-button>
            </template>
            立即刷新全部设备状态
          </n-tooltip>
        </div>
        <div class="header-right">
          <button v-if="upd.latest" class="upd-pill" :class="`upd-${upd.state}`" @click="upd.dialog = true">
            <template v-if="upd.state === 'downloading'">
              <n-progress type="circle" :percentage="upd.progress" :show-indicator="false" :stroke-width="16" class="upd-ring" />
              <span>正在下载 {{ upd.progress.toFixed(0) }}%</span>
            </template>
            <template v-else-if="upd.state === 'ready' || upd.state === 'installing'">
              <n-icon :component="CheckmarkCircle" :size="15" />
              <span>新版本可安装</span>
            </template>
            <template v-else>
              <span class="upd-dot" />
              <span>新版本 v{{ upd.latest.version }}</span>
            </template>
          </button>
          <n-tooltip>
            <template #trigger>
              <n-button quaternary circle @click="theme.cycle()">
                <template #icon><n-icon :component="themeIcon" /></template>
              </n-button>
            </template>
            {{ themeTip }}
          </n-tooltip>
          <n-dropdown :options="userOptions" trigger="click" @select="onUserSelect">
            <div class="user">
              <n-avatar round :size="28" class="avatar">{{ initials }}</n-avatar>
              <div class="user-text">
                <div class="user-name">{{ session.user?.displayName || session.user?.username }}</div>
              </div>
              <n-tag size="small" :bordered="false" :type="session.user?.role === 'admin' ? 'info' : 'default'">{{ session.user?.roleLabel }}</n-tag>
              <n-icon :component="ChevronDown" class="muted" />
            </div>
          </n-dropdown>
        </div>
      </n-layout-header>
      <n-layout-content class="content" :native-scrollbar="false">
        <router-view v-slot="{ Component: View }">
          <component :is="View" :key="`${route.path}#${viewKey}`" />
        </router-view>
      </n-layout-content>
    </n-layout>
    <ChangePasswordModal :show="showPassword || forced" :forced="forced" @update:show="(v: boolean) => (showPassword = v)" />
    <UpdateDialog />
  </n-layout>
</template>

<style scoped>
.shell {
  height: 100vh;
}
.sider :deep(.n-layout-sider-scroll-container) {
  display: flex;
  flex-direction: column;
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 60px;
  padding: 0 17px;
  border-bottom: 1px solid var(--border);
  margin-bottom: 8px;
}
.brand-name {
  font-weight: 700;
  font-size: 15px;
  letter-spacing: 0.04em;
  line-height: 18px;
}
.brand-sub {
  font-size: 11.5px;
  color: var(--text-3);
}
.mock-flag {
  margin: auto 16px 16px;
  padding: 6px 10px;
  border-radius: 6px;
  font-size: 12px;
  color: var(--text-2);
  background: var(--surface-2);
  border: 1px dashed var(--border);
  text-align: center;
}
.header {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px 0 24px;
  background: var(--surface);
}
.poll {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}
.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}
.user {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px 4px 4px;
  border-radius: 20px;
  cursor: pointer;
}
.user:hover {
  background: var(--surface-2);
}
.avatar {
  background: var(--accent);
  color: #fff;
  font-weight: 600;
}
.user-name {
  font-size: 13px;
  font-weight: 500;
}
.upd-pill {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  height: 28px;
  margin-right: 4px;
  padding: 0 12px 0 10px;
  border: 1px solid color-mix(in srgb, var(--accent) 35%, transparent);
  border-radius: 999px;
  background: var(--accent-soft);
  color: var(--accent);
  font: inherit;
  font-size: 12.5px;
  font-weight: 500;
  font-variant-numeric: tabular-nums;
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s;
}
.upd-pill:hover {
  border-color: var(--accent);
}
.upd-pill:focus {
  outline: none;
}
.upd-pill:focus-visible {
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 25%, transparent);
}
.upd-pill.upd-ready,
.upd-pill.upd-installing {
  border-color: color-mix(in srgb, var(--status-good) 40%, transparent);
  background: color-mix(in srgb, var(--status-good) 10%, var(--surface));
  color: var(--status-good);
}
.upd-dot {
  position: relative;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--accent);
}
.upd-dot::after {
  content: '';
  position: absolute;
  inset: -4px;
  border-radius: 50%;
  border: 2px solid var(--accent);
  opacity: 0;
  animation: upd-pulse 2s ease-out infinite;
}
@keyframes upd-pulse {
  0% {
    transform: scale(0.5);
    opacity: 0.7;
  }
  100% {
    transform: scale(1.4);
    opacity: 0;
  }
}
@media (prefers-reduced-motion: reduce) {
  .upd-dot::after {
    animation: none;
  }
}
.upd-ring {
  width: 15px;
}
.content {
  height: calc(100vh - 60px);
  background: var(--bg);
}
</style>
