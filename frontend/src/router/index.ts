import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'
import type { Permission } from '@/api'
import { useSession } from '@/stores/session'

declare module 'vue-router' {
  interface RouteMeta {
    title?: string
    perm?: Permission
    public?: boolean
  }
}

const routes: RouteRecordRaw[] = [
  { path: '/setup', name: 'setup', component: () => import('@/views/SetupView.vue'), meta: { public: true, title: '初始化' } },
  { path: '/login', name: 'login', component: () => import('@/views/LoginView.vue'), meta: { public: true, title: '登录' } },
  {
    path: '/',
    component: () => import('@/layouts/MainLayout.vue'),
    children: [
      { path: '', redirect: '/dashboard' },
      { path: 'dashboard', name: 'dashboard', component: () => import('@/views/DashboardView.vue'), meta: { title: '总览' } },
      { path: 'devices', name: 'devices', component: () => import('@/views/DevicesView.vue'), meta: { title: '设备' } },
      { path: 'devices/:id', name: 'device', component: () => import('@/views/DeviceDetailView.vue'), meta: { title: '设备详情' } },
      { path: 'discovery', name: 'discovery', component: () => import('@/views/DiscoveryView.vue'), meta: { title: '扫描发现', perm: 'device.manage' } },
      { path: 'credentials', name: 'credentials', component: () => import('@/views/CredentialsView.vue'), meta: { title: '凭据' } },
      { path: 'users', name: 'users', component: () => import('@/views/UsersView.vue'), meta: { title: '用户与角色', perm: 'user.manage' } },
      { path: 'audit', name: 'audit', component: () => import('@/views/AuditView.vue'), meta: { title: '审计日志', perm: 'audit.view' } },
      { path: 'settings', name: 'settings', component: () => import('@/views/SettingsView.vue'), meta: { title: '系统设置' } },
    ],
  },
  { path: '/:pathMatch(.*)*', redirect: '/dashboard' },
]

export const router = createRouter({ history: createWebHashHistory(), routes })

router.beforeEach(async (to) => {
  const session = useSession()
  if (!session.status) {
    try {
      await session.refresh()
    } catch {
      // Without the backend state no permission check can run; only the
      // login page is safe to show.
      return to.name === 'login' ? true : { name: 'login' }
    }
  }
  const initialized = session.status?.initialized
  if (!initialized) return to.name === 'setup' ? true : { name: 'setup' }
  if (to.name === 'setup') return { name: session.loggedIn ? 'dashboard' : 'login' }
  if (to.meta.public) return session.loggedIn && to.name === 'login' ? { name: 'dashboard' } : true
  if (!session.loggedIn) return { name: 'login', query: to.fullPath !== '/dashboard' ? { redirect: to.fullPath } : {} }
  if (to.meta.perm && !session.can(to.meta.perm)) return { name: 'dashboard' }
  return true
})

router.afterEach((to) => {
  document.title = to.meta.title ? `${to.meta.title} · 明点服务器管理系统` : '明点服务器管理系统'
})
