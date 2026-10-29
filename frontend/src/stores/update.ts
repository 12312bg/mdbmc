import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api, type UpdateStatus } from '@/api'

// Software update state, pushed by the backend as update:state events.
export const useUpdate = defineStore('update', () => {
  const status = ref<UpdateStatus | null>(null)
  /** The update dialog is open. */
  const dialog = ref(false)

  const latest = computed(() => status.value?.latest ?? null)
  const state = computed(() => status.value?.state ?? 'idle')
  const progress = computed(() => {
    const s = status.value
    return s && s.total > 0 ? Math.min(100, (s.downloaded / s.total) * 100) : 0
  })

  let unsubscribe: (() => void) | null = null
  function subscribe() {
    unsubscribe ??= api.on('update:state', (s) => {
      status.value = s
    })
  }

  async function load() {
    try {
      status.value = await api.update.status()
    } catch {
      /* not logged in */
    }
  }

  async function check() {
    status.value = await api.update.check()
  }

  async function dismissResult() {
    if (status.value) status.value = { ...status.value, lastResult: null }
    await api.update.dismissResult()
  }

  return { status, dialog, latest, state, progress, subscribe, load, check, dismissResult }
})
