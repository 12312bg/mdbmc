import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api, type Credential, type Device, type DeviceStatusEvent, type Group, type VendorOption } from '@/api'

// Shared device list kept live by backend events.
export const useDevices = defineStore('devices', () => {
  const items = ref<Device[]>([])
  const groups = ref<Group[]>([])
  const credentials = ref<Credential[]>([])
  const vendors = ref<VendorOption[]>([])
  const loading = ref(false)
  const polling = ref(false)
  const lastPoll = ref(0)
  let subscribed = false
  let reloadTimer: number | undefined

  const byId = computed(() => new Map(items.value.map((d) => [d.id, d])))

  async function load() {
    loading.value = true
    try {
      const [d, g, c] = await Promise.all([api.devices.list(), api.devices.listGroups(), api.credentials.list()])
      items.value = d
      groups.value = g
      credentials.value = c
      if (!vendors.value.length) vendors.value = await api.devices.vendors()
    } finally {
      loading.value = false
    }
  }

  function applyStatus(e: DeviceStatusEvent) {
    const d = byId.value.get(e.id)
    if (!d) return
    d.powerState = e.powerState
    d.health = e.health
    d.reachable = e.reachable
    d.lastError = e.lastError
    d.checkedAt = e.checkedAt
  }

  // Identity updates arrive in bursts after adding devices; coalesce reloads.
  function scheduleReload() {
    window.clearTimeout(reloadTimer)
    reloadTimer = window.setTimeout(() => {
      void load()
    }, 400)
  }

  function subscribe() {
    if (subscribed) return
    subscribed = true
    api.on('device:status', applyStatus)
    api.on('device:updated', scheduleReload)
    api.on('poll:state', (s) => {
      polling.value = s.running
      if (s.lastPoll) lastPoll.value = s.lastPoll
    })
  }

  return { items, groups, credentials, vendors, loading, polling, lastPoll, byId, load, subscribe, scheduleReload }
})
