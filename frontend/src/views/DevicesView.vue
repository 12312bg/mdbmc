<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NButton, NIcon, NInput, NSelect, NDataTable, NDropdown, NTooltip, NModal, NForm, NFormItem, NEllipsis,
  useDialog, useMessage, type DataTableColumns, type DropdownOption,
} from 'naive-ui'
import {
  AddOutline, SearchOutline, RefreshOutline, GlobeOutline, DesktopOutline, EllipsisHorizontal, PowerOutline,
  FolderOutline, FolderOpenOutline, KeyOutline, TrashOutline, CreateOutline, ScanOutline, LayersOutline,
} from '@vicons/ionicons5'
import StatusBadges from '@/components/StatusBadges.vue'
import VendorBadge from '@/components/VendorBadge.vue'
import DeviceFormModal from '@/components/DeviceFormModal.vue'
import { api, errMsg, type Device, type PowerAction } from '@/api'
import { useDevices } from '@/stores/devices'
import { useSession } from '@/stores/session'
import { useDeviceActions } from '@/composables/useDeviceActions'
import { powerActions, relativeTime } from '@/utils/format'

const store = useDevices()
const session = useSession()
const route = useRoute()
const router = useRouter()
const dialog = useDialog()
const message = useMessage()
const actions = useDeviceActions()

const canManage = computed(() => session.can('device.manage'))
const canPower = computed(() => session.can('power.control'))

// ----- filters -----
const groupFilter = ref<number | 'all'>('all')
const keyword = ref('')
const vendorFilter = ref<string | null>(null)
const statusFilter = ref<string | null>(typeof route.query.status === 'string' ? route.query.status : null)
const statusOptions = [
  { label: '已开机', value: 'on' },
  { label: '已关机', value: 'off' },
  { label: '无法连接', value: 'offline' },
  { label: '需要关注', value: 'problem' },
  { label: '健康告警', value: 'alert' },
]
const vendorOptions = computed(() => store.vendors.map((v) => ({ label: v.name, value: v.id })))

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return store.items.filter((d) => {
    if (groupFilter.value !== 'all' && d.groupId !== groupFilter.value) return false
    if (vendorFilter.value && d.vendor !== vendorFilter.value) return false
    const offline = !d.reachable && d.checkedAt > 0
    switch (statusFilter.value) {
      case 'on': if (d.powerState !== 'on' || offline) return false; break
      case 'off': if (d.powerState !== 'off' || offline) return false; break
      case 'offline': if (!offline) return false; break
      case 'alert': if (d.health !== 'warning' && d.health !== 'critical') return false; break
      case 'problem': if (!offline && d.health !== 'warning' && d.health !== 'critical') return false; break
    }
    if (!kw) return true
    return [d.name, d.host, d.serial, d.model, d.note, d.vendorName].some((x) => x?.toLowerCase().includes(kw))
  })
})

const groupCounts = computed(() => {
  const m = new Map<number, number>()
  store.items.forEach((d) => m.set(d.groupId, (m.get(d.groupId) ?? 0) + 1))
  return m
})

// ----- selection -----
const checked = ref<number[]>([])
// Keep the selection to rows the user can see, so a batch action never
// touches devices hidden by a filter or removed meanwhile.
watch(filtered, (rows) => {
  const visible = new Set(rows.map((d) => d.id))
  if (checked.value.some((id) => !visible.has(id))) checked.value = checked.value.filter((id) => visible.has(id))
})
const selectedDevices = computed(() => checked.value.map((id) => store.byId.get(id)).filter(Boolean) as Device[])

// ----- modals -----
const showForm = ref(false)
const editing = ref<Device | null>(null)
function openAdd() {
  editing.value = null
  showForm.value = true
}
function openEdit(d: Device) {
  editing.value = d
  showForm.value = true
}
onMounted(() => {
  if (route.query.add && canManage.value) {
    openAdd()
    void router.replace({ query: {} })
  }
})

const groupModal = ref<{ show: boolean; id: number; name: string }>({ show: false, id: 0, name: '' })
function openGroup(id = 0, name = '') {
  groupModal.value = { show: true, id, name }
}
// Returns false to keep the dialog open when saving fails.
async function saveGroup(): Promise<boolean> {
  const { id, name } = groupModal.value
  try {
    if (id) await api.devices.renameGroup(id, name)
    else await api.devices.createGroup(name)
    groupModal.value.show = false
    await store.load()
    return true
  } catch (e) {
    message.error(errMsg(e))
    return false
  }
}
function removeGroup(id: number, name: string) {
  dialog.warning({
    title: `删除分组“${name}”`,
    content: '分组内的设备不会被删除，将变为“未分组”。',
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await api.devices.deleteGroup(id)
        if (groupFilter.value === id) groupFilter.value = 'all'
        await store.load()
      } catch (e) {
        message.error(errMsg(e))
      }
    },
  })
}

// ----- batch actions -----
const powerOptions = powerActions.map((a) => ({ label: a.label, key: a.key, props: a.danger ? { style: 'color: var(--status-critical)' } : undefined }))
async function batchPower(action: PowerAction) {
  const ds = selectedDevices.value
  await actions.power(ds.map((d) => d.id), action, ds.map((d) => d.name))
}
const moveOptions = computed<DropdownOption[]>(() => store.groupOptions.map((o) => ({ label: o.label, key: o.value })))
async function batchMove(groupId: number) {
  try {
    await api.devices.moveToGroup(checked.value, groupId)
    message.success('已移动')
    await store.load()
  } catch (e) {
    message.error(errMsg(e))
  }
}
const credOptions = computed<DropdownOption[]>(() => store.credentialOptions.map((o) => ({ label: o.label, key: o.value })))
async function batchCred(credId: number) {
  try {
    await api.devices.setCredential(checked.value, credId)
    message.success('已更换凭据')
    await store.load()
  } catch (e) {
    message.error(errMsg(e))
  }
}
function confirmDelete(ds: Device[]) {
  dialog.error({
    title: ds.length === 1 ? `删除设备“${ds[0].name}”` : `删除所选 ${ds.length} 台设备`,
    content: '只会从本软件中移除，不会影响服务器本身。缓存的资产信息也会一并删除。',
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const n = await api.devices.remove(ds.map((d) => d.id))
        message.success(`已删除 ${n} 台设备`)
        checked.value = []
        await store.load()
      } catch (e) {
        message.error(errMsg(e))
      }
    },
  })
}
async function refresh(ids: number[] = []) {
  try {
    await api.devices.refreshStatus(ids)
    message.info(ids.length ? '正在刷新所选设备状态' : '正在刷新全部设备状态')
  } catch (e) {
    message.error(errMsg(e))
  }
}

// ----- table -----
function rowMenu(d: Device): DropdownOption[] {
  const opts: DropdownOption[] = [{ label: '查看详情', key: 'detail' }]
  if (canPower.value) {
    opts.push({ type: 'divider', key: 'd1' }, ...powerActions.map((a) => ({ label: a.label, key: 'power:' + a.key })))
  }
  opts.push({ type: 'divider', key: 'd2' }, { label: '刷新状态', key: 'refresh' })
  if (canManage.value) {
    opts.push({ label: '编辑', key: 'edit' }, { label: '删除', key: 'delete', props: { style: 'color: var(--status-critical)' } })
  }
  return opts
}
async function onRowMenu(d: Device, key: string) {
  if (key === 'detail') return router.push(`/devices/${d.id}`)
  if (key.startsWith('power:')) return actions.power([d.id], key.slice(6) as PowerAction, [d.name])
  if (key === 'refresh') return refresh([d.id])
  if (key === 'edit') return openEdit(d)
  if (key === 'delete') return confirmDelete([d])
}

const iconBtn = (icon: any, tip: string, onClick: () => void, disabled = false) =>
  h(NTooltip, null, {
    trigger: () => h(NButton, { quaternary: true, circle: true, size: 'small', disabled, onClick }, { icon: () => h(NIcon, { component: icon }) }),
    default: () => tip,
  })

const columns = computed<DataTableColumns<Device>>(() => [
  { type: 'selection', width: 44 },
  {
    title: '设备', key: 'name', minWidth: 170, sorter: (a, b) => a.name.localeCompare(b.name, 'zh-CN'),
    render: (d) => h('div', { class: 'cell-name', onClick: () => router.push(`/devices/${d.id}`) }, [
      h('div', { class: 'n1' }, d.name),
      h('div', { class: 'n2 mono' }, d.host + (d.httpsPort !== 443 ? `:${d.httpsPort}` : '')),
    ]),
  },
  {
    title: '电源', key: 'powerState', width: 112,
    sorter: (a, b) => a.powerState.localeCompare(b.powerState),
    render: (d) => h(NTooltip, null, {
      trigger: () => h(StatusBadges, { power: d.powerState, reachable: d.reachable, checked: d.checkedAt > 0 }),
      default: () => (d.lastError ? `${d.lastError}（${relativeTime(d.checkedAt)}）` : `状态检查于 ${relativeTime(d.checkedAt)}`),
    }),
  },
  {
    title: '健康', key: 'health', width: 90,
    sorter: (a, b) => healthRank(a) - healthRank(b),
    render: (d) => h(StatusBadges, { health: d.reachable || !d.checkedAt ? d.health : 'unknown' }),
  },
  {
    title: '品牌 / 型号', key: 'model', minWidth: 190,
    render: (d) => h('div', { class: 'cell-model' }, [
      h(VendorBadge, { vendor: d.vendor, name: d.vendorName }),
      d.model ? h(NEllipsis, { class: 'muted', style: 'max-width: 100%; font-size: 12.5px' }, { default: () => d.model }) : null,
    ]),
  },
  { title: '序列号', key: 'serial', width: 140, ellipsis: { tooltip: true }, render: (d) => h('span', { class: 'mono' }, d.serial || '—') },
  { title: '分组', key: 'groupName', width: 120, ellipsis: { tooltip: true }, render: (d) => d.groupName || h('span', { class: 'muted' }, '未分组') },
  {
    title: '操作', key: 'actions', width: 124, fixed: 'right',
    render: (d) => h('div', { style: 'display:flex;gap:2px' }, [
      iconBtn(GlobeOutline, session.can('console.autologin') ? 'Web 管理（自动登录）' : 'Web 登录页', () => actions.openWeb(d.id)),
      session.can('console.kvm') ? iconBtn(DesktopOutline, '远程控制台 KVM', () => actions.openKVM(d.id)) : null,
      h(NDropdown, { options: rowMenu(d), trigger: 'click', onSelect: (k: string) => onRowMenu(d, k) }, {
        default: () => h(NButton, { quaternary: true, circle: true, size: 'small' }, { icon: () => h(NIcon, { component: EllipsisHorizontal }) }),
      }),
    ]),
  },
])

function healthRank(d: Device) {
  return { critical: 0, warning: 1, unknown: 2, ok: 3 }[d.health] ?? 4
}
</script>

<template>
  <div class="page devices-page">
    <div class="page-head">
      <div>
        <h1>设备</h1>
        <p>共 {{ store.items.length }} 台，当前显示 {{ filtered.length }} 台</p>
      </div>
      <div class="head-actions">
        <n-button @click="refresh()">
          <template #icon><n-icon :component="RefreshOutline" /></template>
          刷新状态
        </n-button>
        <n-button v-if="canManage" @click="router.push('/discovery')">
          <template #icon><n-icon :component="ScanOutline" /></template>
          扫描发现
        </n-button>
        <n-button v-if="canManage" type="primary" @click="openAdd">
          <template #icon><n-icon :component="AddOutline" /></template>
          添加设备
        </n-button>
      </div>
    </div>

    <div class="layout">
      <aside class="groups card">
        <div class="groups-head">
          <span class="card-title">分组</span>
          <n-button v-if="canManage" text size="small" @click="openGroup()">
            <template #icon><n-icon :component="AddOutline" /></template>
            新建
          </n-button>
        </div>
        <div class="g-item" :class="{ active: groupFilter === 'all' }" @click="groupFilter = 'all'">
          <n-icon :component="LayersOutline" /><span class="g-name">全部设备</span><span class="g-count">{{ store.items.length }}</span>
        </div>
        <div class="g-item" :class="{ active: groupFilter === 0 }" @click="groupFilter = 0">
          <n-icon :component="FolderOutline" /><span class="g-name">未分组</span><span class="g-count">{{ groupCounts.get(0) ?? 0 }}</span>
        </div>
        <div v-for="g in store.groups" :key="g.id" class="g-item" :class="{ active: groupFilter === g.id }" @click="groupFilter = g.id">
          <n-icon :component="groupFilter === g.id ? FolderOpenOutline : FolderOutline" />
          <span class="g-name" :title="g.name">{{ g.name }}</span>
          <span class="g-count">{{ groupCounts.get(g.id) ?? 0 }}</span>
          <n-dropdown
            v-if="canManage"
            trigger="click"
            :options="[{ label: '重命名', key: 'rename' }, { label: '删除', key: 'delete' }]"
            @select="(k: string) => (k === 'rename' ? openGroup(g.id, g.name) : removeGroup(g.id, g.name))"
          >
            <n-button quaternary circle size="tiny" class="g-more" @click.stop>
              <template #icon><n-icon :component="EllipsisHorizontal" /></template>
            </n-button>
          </n-dropdown>
        </div>
      </aside>

      <section class="main card">
        <div class="toolbar">
          <template v-if="!checked.length">
            <n-input v-model:value="keyword" placeholder="搜索名称、IP、序列号、型号" clearable style="width: 280px">
              <template #prefix><n-icon :component="SearchOutline" /></template>
            </n-input>
            <n-select v-model:value="vendorFilter" :options="vendorOptions" placeholder="品牌" clearable style="width: 150px" />
            <n-select v-model:value="statusFilter" :options="statusOptions" placeholder="状态" clearable style="width: 140px" />
          </template>
          <template v-else>
            <span class="sel-count">已选 {{ checked.length }} 台</span>
            <n-dropdown v-if="canPower" :options="powerOptions" trigger="click" @select="batchPower">
              <n-button>
                <template #icon><n-icon :component="PowerOutline" /></template>
                电源操作
              </n-button>
            </n-dropdown>
            <n-button @click="refresh(checked)">
              <template #icon><n-icon :component="RefreshOutline" /></template>
              刷新
            </n-button>
            <template v-if="canManage">
              <n-dropdown :options="moveOptions" trigger="click" @select="batchMove">
                <n-button>
                  <template #icon><n-icon :component="FolderOutline" /></template>
                  移动到分组
                </n-button>
              </n-dropdown>
              <n-dropdown :options="credOptions" trigger="click" :disabled="!credOptions.length" @select="batchCred">
                <n-button :disabled="!credOptions.length">
                  <template #icon><n-icon :component="KeyOutline" /></template>
                  更换凭据
                </n-button>
              </n-dropdown>
              <n-button type="error" ghost @click="confirmDelete(selectedDevices)">
                <template #icon><n-icon :component="TrashOutline" /></template>
                删除
              </n-button>
            </template>
            <n-button text @click="checked = []">取消选择</n-button>
          </template>
        </div>
        <n-data-table
          v-model:checked-row-keys="checked"
          class="table"
          :columns="columns"
          :data="filtered"
          :row-key="(d: Device) => d.id"
          :loading="store.loading && !store.items.length"
          :bordered="false"
          size="small"
          flex-height
          virtual-scroll
          :scroll-x="980"
        >
          <template #empty>
            <div class="table-empty">
              <div class="muted">{{ store.items.length ? '没有符合条件的设备' : '还没有添加设备' }}</div>
              <n-button v-if="!store.items.length && canManage" type="primary" size="small" style="margin-top: 12px" @click="openAdd">
                <template #icon><n-icon :component="CreateOutline" /></template>
                添加第一台设备
              </n-button>
            </div>
          </template>
        </n-data-table>
      </section>
    </div>

    <DeviceFormModal
      v-model:show="showForm"
      :device="editing"
      :default-group="typeof groupFilter === 'number' ? groupFilter : 0"
      @saved="store.load()"
    />

    <n-modal v-model:show="groupModal.show" preset="dialog" :title="groupModal.id ? '重命名分组' : '新建分组'" positive-text="保存" negative-text="取消" @positive-click="saveGroup">
      <n-form @submit.prevent="saveGroup">
        <n-form-item label="分组名称" :show-feedback="false" style="margin-top: 12px">
          <n-input v-model:value="groupModal.name" maxlength="40" placeholder="如：机房 A · 03 列" autofocus />
        </n-form-item>
      </n-form>
    </n-modal>
  </div>
</template>

<style scoped>
.devices-page {
  height: calc(100vh - 60px);
  display: flex;
  flex-direction: column;
}
.head-actions {
  display: flex;
  gap: 8px;
}
.layout {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: 200px minmax(0, 1fr);
  gap: 16px;
}
.groups {
  padding: 12px 8px;
  overflow: auto;
}
.groups-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 2px 8px 10px;
}
.g-item {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 36px;
  padding: 0 8px 0 10px;
  border-radius: 6px;
  cursor: pointer;
  color: var(--text-2);
}
.g-item:hover {
  background: var(--surface-2);
}
.g-item.active {
  background: var(--accent-soft);
  color: var(--accent);
  font-weight: 500;
}
.g-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.g-count {
  font-size: 12px;
  color: var(--text-3);
  font-variant-numeric: tabular-nums;
}
.g-more {
  opacity: 0;
}
.g-item:hover .g-more {
  opacity: 1;
}
.main {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 420px;
  overflow: hidden;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 14px;
  border-bottom: 1px solid var(--border);
  min-height: 58px;
}
.sel-count {
  font-weight: 600;
  margin-right: 6px;
}
.table {
  flex: 1;
  min-height: 0;
}
.table :deep(.cell-name) {
  cursor: pointer;
  line-height: 1.35;
}
.table :deep(.cell-name .n1) {
  font-weight: 500;
  color: var(--text-1);
}
.table :deep(.cell-name:hover .n1) {
  color: var(--accent);
}
.table :deep(.cell-name .n2) {
  color: var(--text-3);
}
.table :deep(.cell-model) {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.table-empty {
  padding: 48px 0;
  text-align: center;
}
</style>
