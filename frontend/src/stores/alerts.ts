import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, type AlertItem } from '@/api'

// Alert inbox state: the unread count, this user's pop-up preference and the
// latest alerts, kept live by alert:new events.
export const useAlerts = defineStore('alerts', () => {
  const unread = ref(0)
  const popup = ref(true)
  /** Alerts received since the window opened, newest first. */
  const recent = ref<AlertItem[]>([])
  /** Bumped on every new alert so open pages can reload. */
  const version = ref(0)

  const handlers = new Set<(a: AlertItem) => void>()
  let unsubscribe: (() => void) | null = null
  function subscribe() {
    unsubscribe ??= api.on('alert:new', (a) => {
      const item = { ...a, unread: true }
      unread.value++
      recent.value = [item, ...recent.value].slice(0, 20)
      version.value++
      handlers.forEach((h) => h(item))
    })
  }
  /** Calls h for each new alert; returns a function that stops it. */
  function onAlert(h: (a: AlertItem) => void) {
    handlers.add(h)
    return () => handlers.delete(h)
  }

  async function load() {
    try {
      const box = await api.alerts.inbox()
      unread.value = box.unread
      popup.value = box.popup
    } catch {
      /* not logged in */
    }
  }

  async function markAllRead() {
    await api.alerts.markAllRead()
    unread.value = 0
    recent.value = recent.value.map((a) => ({ ...a, unread: false }))
  }

  async function setPopup(on: boolean) {
    await api.alerts.setPopup(on)
    popup.value = on
  }

  return { unread, popup, recent, version, subscribe, onAlert, load, markAllRead, setPopup }
})
