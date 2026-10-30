<script setup lang="ts">
// BMC and BIOS versions by server model, from the versions read when each
// device was identified. Within a model, devices below the highest version
// seen are flagged; the program does not know the vendors' latest releases.
import { computed, h, onBeforeUnmount, ref } from 'vue'
import { RouterLink } from 'vue-router'
import {
  NButton, NIcon, NInput, NSelect, NSwitch, NDataTable, NTag, NProgress, NTooltip, useMessage, type DataTableColumns,
} from 'naive-ui'
import { SearchOutline, RefreshOutline, DownloadOutline, ChevronDown, ChevronUp } from '@vicons/ionicons5'
import VendorBadge from '@/components/VendorBadge.vue'
import { api, errMsg, type Device } from '@/api'
import { useDevices } from '@/stores/devices'
import { compareVersions, relativeTime } from '@/utils/format'

const store = useDevices()
const message = useMessage()

interface VersionStat {
  version: string
  count: number
  highest: boolean
}
interface ModelGroup {
  key: string
  vendor: string
  vendorName: string
  model: string
  devices: Device[]
  bmc: VersionStat[]
  bios: VersionStat[]
  /** Devices below the highest BMC or BIOS version of the model. */
  behind: Set<number>
  maxBmc: string
  maxBios: string
}

function stats(list: Device[], pick: (d: Device) => string): VersionStat[] {
  const m = new Map<string, number>()
  for (const d of list) {
    const v = pick(d).trim()
    if (v) m.set(v, (m.get(v) ?? 0) + 1)
  }
  const out = [...m].map(([version, count]) => ({ version, count, highest: false })).sort((a, b) => compareVersions(b.version, a.version))
  if (out.length) out[0].highest = true
  return out
}

const identified = computed(() => store.items.filter((d) => d.model && (d.bmcFirmware || d.biosVersion)))
const unread = computed(() => store.items.length - identified.value.length)

const groups = computed<ModelGroup[]>(() => {
  const m = new Map<string, Device[]>()
  for (const d of identified.value) {
    const key = `${d.vendor}\u0000${d.model}`
    m.set(key, [...(m.get(key) ?? []), d])
  }
  return [...m].map(([key, devs]) => {
    const bmc = stats(devs, (d) => d.bmcFirmware)
    const bios = stats(devs, (d) => d.biosVersion)
    const maxBmc = bmc[0]?.version ?? ''
    const maxBios = bios[0]?.version ?? ''
    const behind = new Set(devs.filter((d) => (d.bmcFirmware && d.bmcFirmware.trim() !== maxBmc) || (d.biosVersion && d.biosVersion.trim() !== maxBios)).map((d) => d.id))
    return { key, vendor: devs[0].vendor, vendorName: devs[0].vendorName, model: devs[0].model, devices: devs, bmc, bios, behind, maxBmc, maxBios }
  }).sort((a, b) => b.behind.size - a.behind.size || b.devices.length - a.devices.length || a.model.localeCompare(b.model))
})

const keyword = ref('')
const vendor = ref<string | null>(null)
const onlyMixed = ref(false)
const vendorOptions = computed(() => {
  const seen = new Map<string, string>()
  groups.value.forEach((g) => seen.set(g.vendor, g.vendorName || '通用'))
  return [...seen].map(([value, label]) => ({ value, label }))
})
const shown = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return groups.value.filter((g) => {
    if (vendor.value && g.vendor !== vendor.value) return false
    if (onlyMixed.value && !g.behind.size) return false
    if (!kw) return true
    return [g.model, g.vendorName].some((x) => x.toLowerCase().includes(kw)) ||
      g.devices.some((d) => [d.name, d.host, d.bmcFirmware, d.biosVersion].some((x) => x.toLowerCase().includes(kw)))
  })
})

const summary = computed(() => ({
  models: groups.value.length,
  devices: identified.value.length,
  mixed: groups.value.filter((g) => g.behind.size).length,
  behind: groups.value.reduce((n, g) => n + g.behind.size, 0),
}))

const expanded = ref(new Set<string>())
function toggle(key: string) {
  const s = new Set(expanded.value)
  if (s.has(key)) s.delete(key)
  else s.add(key)
  expanded.value = s
}

function deviceColumns(g: ModelGroup): DataTableColumns<Device> {
  const ver = (v: string, max: string) =>
    v ? h('span', { class: ['mono', v.trim() !== max && 'behind'] }, v) : h('span', { class: 'muted' }, '—')
  return [
    {
      title: '设备', key: 'name', minWidth: 200,
      render: (d) => h('div', { class: 'dev' }, [
        h(RouterLink, { to: `/devices/${d.id}`, class: 'dev-link' }, { default: () => d.name }),
        h('span', { class: 'mono muted' }, d.host),
      ]),
    },
    { title: 'BMC 固件', key: 'bmcFirmware', minWidth: 160, render: (d) => ver(d.bmcFirmware, g.maxBmc) },
    { title: 'BIOS', key: 'biosVersion', minWidth: 140, render: (d) => ver(d.biosVersion, g.maxBios) },
    { title: '读取于', key: 'identifiedAt', width: 110, render: (d) => h('span', { class: 'muted' }, relativeTime(d.identifiedAt)) },
  ]
}
const sortedDevices = (g: ModelGroup) =>
  [...g.devices].sort((a, b) => Number(g.behind.has(b.id)) - Number(g.behind.has(a.id)) || a.name.localeCompare(b.name))

// Re-reading the versions identifies every device again, a few at a time.
const reading = ref(false)
const readDone = ref(0)
const readTotal = ref(0)
let cancelRead = false
onBeforeUnmount(() => (cancelRead = true))
async function rereadAll() {
  const ids = store.items.filter((d) => d.reachable || !d.checkedAt).map((d) => d.id)
  if (!ids.length) return
  reading.value = true
  cancelRead = false
  readDone.value = 0
  readTotal.value = ids.length
  let failed = 0
  let next = 0
  const worker = async () => {
    while (!cancelRead && next < ids.length) {
      const id = ids[next++]
      try {
        await api.devices.identify(id)
      } catch {
        failed++
      }
      readDone.value++
    }
  }
  await Promise.all(Array.from({ length: Math.min(6, ids.length) }, worker))
  reading.value = false
  if (cancelRead) return
  await store.load()
  if (failed) message.warning(`已重新读取 ${ids.length - failed} 台，${failed} 台读取失败`)
  else message.success(`已重新读取 ${ids.length} 台设备的版本`)
}

const exporting = ref(false)
async function exportAssets() {
  exporting.value = true
  try {
    const path = await api.devices.exportAssets()
    if (path) message.success(`已导出到 ${path}`)
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    exporting.value = false
  }
}
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>固件版本</h1>
      </div>
      <div class="head-actions">
        <n-tooltip>
          <template #trigger>
            <n-button :disabled="reading || !store.items.length" @click="rereadAll">
              <template #icon><n-icon :component="RefreshOutline" /></template>
              <template v-if="reading">正在读取 {{ readDone }}/{{ readTotal }}</template>
              <template v-else>重新读取版本</template>
            </n-button>
          </template>
          升级固件后，从各台 BMC 重新读取型号和版本
        </n-tooltip>
        <n-button :loading="exporting" @click="exportAssets">
          <template #icon><n-icon :component="DownloadOutline" /></template>
          导出 Excel
        </n-button>
      </div>
    </div>

    <n-progress v-if="reading" type="line" :percentage="readTotal ? (readDone / readTotal) * 100 : 0" :show-indicator="false" :height="3" class="read-bar" />

    <div class="tiles">
      <div class="card tile">
        <div class="tile-label">型号</div>
        <div class="tile-value">{{ summary.models }}</div>
      </div>
      <div class="card tile">
        <div class="tile-label">已读取版本的设备</div>
        <div class="tile-value">{{ summary.devices }}<span v-if="unread" class="tile-sub">另有 {{ unread }} 台未读取</span></div>
      </div>
      <div class="card tile">
        <div class="tile-label">版本不一致的型号</div>
        <div class="tile-value" :class="{ warn: summary.mixed }">{{ summary.mixed }}</div>
      </div>
      <div class="card tile">
        <div class="tile-label">低于同型号最高版本的设备</div>
        <div class="tile-value" :class="{ warn: summary.behind }">{{ summary.behind }}</div>
      </div>
    </div>

    <div class="filters">
      <n-input v-model:value="keyword" placeholder="搜索型号、设备、地址或版本" clearable style="width: 280px">
        <template #prefix><n-icon :component="SearchOutline" /></template>
      </n-input>
      <n-select v-model:value="vendor" :options="vendorOptions" placeholder="品牌" clearable style="width: 160px" />
      <label class="switch">
        <n-switch v-model:value="onlyMixed" size="small" />
        <span>只看版本不一致</span>
      </label>
    </div>

    <div v-if="!shown.length" class="card empty">
      {{ store.loading ? '正在加载…' : groups.length ? '没有符合条件的型号' : '还没有读取到任何设备的固件版本' }}
    </div>

    <div v-for="g in shown" :key="g.key" class="card model">
      <div class="model-head">
        <VendorBadge :vendor="g.vendor" :name="g.vendorName" />
        <span class="model-name">{{ g.model }}</span>
        <span class="model-count">{{ g.devices.length }} 台</span>
        <n-tag v-if="g.behind.size" size="small" type="warning" :bordered="false" round>{{ g.behind.size }} 台低于最高版本</n-tag>
        <n-tag v-else size="small" type="success" :bordered="false" round>版本一致</n-tag>
        <n-button text size="small" class="model-toggle" @click="toggle(g.key)">
          {{ expanded.has(g.key) ? '收起设备' : '查看设备' }}
          <template #icon><n-icon :component="expanded.has(g.key) ? ChevronUp : ChevronDown" /></template>
        </n-button>
      </div>
      <div class="dists">
        <div v-for="col in [{ title: 'BMC 固件', list: g.bmc }, { title: 'BIOS', list: g.bios }]" :key="col.title" class="dist">
          <div class="dist-title">{{ col.title }}</div>
          <div v-if="!col.list.length" class="muted dist-none">未读取到</div>
          <div v-for="v in col.list" :key="v.version" class="ver">
            <span class="ver-name mono" :title="v.version">{{ v.version }}</span>
            <span class="ver-bar">
              <span class="ver-fill" :class="{ top: v.highest }" :style="{ width: `${(v.count / g.devices.length) * 100}%` }" />
            </span>
            <span class="ver-count">{{ v.count }} 台</span>
            <span class="ver-tag">
              <n-tag v-if="v.highest && col.list.length > 1" size="tiny" type="success" :bordered="false">最高</n-tag>
            </span>
          </div>
        </div>
      </div>
      <n-data-table
        v-if="expanded.has(g.key)"
        class="model-devices"
        :columns="deviceColumns(g)"
        :data="sortedDevices(g)"
        :row-key="(d: Device) => d.id"
        :bordered="false"
        size="small"
        :max-height="360"
      />
    </div>
  </div>
</template>

<style scoped>
.head-actions {
  display: flex;
  gap: 8px;
}
.read-bar {
  margin: -8px 0 12px;
}
.tiles {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}
.tile {
  padding: 14px 16px;
}
.tile-label {
  font-size: 12.5px;
  color: var(--text-3);
}
.tile-value {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-top: 4px;
  font-size: 24px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.tile-value.warn {
  color: color-mix(in srgb, var(--status-warning) 72%, var(--text-1));
}
.tile-sub {
  font-size: 12.5px;
  font-weight: 400;
  color: var(--text-3);
}
.filters {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.switch {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  margin-left: 8px;
  font-size: 13px;
  color: var(--text-2);
  cursor: pointer;
}
.empty {
  padding: 40px 0;
  text-align: center;
  color: var(--text-3);
}
.model {
  padding: 14px 18px 16px;
  margin-bottom: 12px;
}
.model-head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.model-name {
  font-weight: 600;
  font-size: 14.5px;
}
.model-count {
  color: var(--text-3);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}
.model-toggle {
  margin-left: auto;
}
.dists {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 28px;
  margin-top: 12px;
}
.dist-title {
  margin-bottom: 6px;
  font-size: 12.5px;
  color: var(--text-3);
}
.dist-none {
  font-size: 13px;
}
.ver {
  display: grid;
  grid-template-columns: minmax(80px, 180px) minmax(60px, 1fr) 52px 40px;
  align-items: center;
  gap: 10px;
  height: 26px;
}
.ver-name {
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}
.ver-bar {
  height: 8px;
  border-radius: 4px;
  background: var(--track);
  overflow: hidden;
}
.ver-fill {
  display: block;
  height: 100%;
  border-radius: 4px;
  background: var(--status-warning);
}
.ver-fill.top {
  background: var(--status-good);
}
.ver-count {
  text-align: right;
  font-size: 13px;
  color: var(--text-2);
  font-variant-numeric: tabular-nums;
}
.model-devices {
  margin-top: 12px;
  border-top: 1px solid var(--border);
  padding-top: 4px;
}
:deep(.dev) {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
:deep(.dev-link) {
  color: var(--accent);
  text-decoration: none;
  font-weight: 500;
}
:deep(.dev-link:hover) {
  text-decoration: underline;
}
:deep(.behind) {
  color: color-mix(in srgb, var(--status-warning) 72%, var(--text-1));
  font-weight: 600;
}
@media (max-width: 1100px) {
  .tiles {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .dists {
    grid-template-columns: 1fr;
    gap: 14px;
  }
}
</style>
