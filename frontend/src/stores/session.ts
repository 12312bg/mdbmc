import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api, type AuthStatus, type Permission, type Settings, type UserInfo } from '@/api'

export const useSession = defineStore('session', () => {
  const status = ref<AuthStatus | null>(null)
  const user = ref<UserInfo | null>(null)
  const settings = ref<Settings | null>(null)
  const lockedReason = ref('')

  const loggedIn = computed(() => !!user.value)

  async function refresh() {
    status.value = await api.auth.status()
    user.value = status.value.user
    if (user.value) await loadSettings()
  }

  async function loadSettings() {
    try {
      settings.value = await api.admin.getSettings()
    } catch {
      settings.value = null
    }
  }

  function can(p: Permission) {
    return !!user.value?.permissions.includes(p)
  }

  async function login(username: string, password: string) {
    user.value = await api.auth.login(username, password)
    lockedReason.value = ''
    await refresh()
  }

  async function setup(username: string, displayName: string, password: string) {
    user.value = await api.auth.setup(username, displayName, password)
    await refresh()
  }

  async function logout(reason = '') {
    await api.auth.logout()
    lockedReason.value = reason
    user.value = null
    if (status.value) status.value.user = null
  }

  async function changePassword(oldPassword: string, newPassword: string) {
    await api.auth.changePassword(oldPassword, newPassword)
    if (user.value) user.value = { ...user.value, mustChangePassword: false }
  }

  return { status, user, settings, lockedReason, loggedIn, refresh, loadSettings, can, login, setup, logout, changePassword }
})
