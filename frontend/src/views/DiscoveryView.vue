<script setup lang="ts">
import { computed, h, onBeforeUnmount, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  NButton, NIcon, NInput, NInputNumber, NForm, NFormItem, NProgress, NDataTable, NTag, NModal, NSelect, NRadioGroup,
  NRadioButton, NGrid, NFormItemGi, NAlert, useMessage, type DataTableColumns,
} from 'naive-ui'
import { ScanOutline, StopCircleOutline, AddOutline } from '@vicons/ionicons5'
import VendorBadge from '@/components/VendorBadge.vue'
import { api, errMsg, type ScanFound, type ScanProgress } from '@/api'
import { useDevices } from '@/stores/devices'

const store = useDevices()
const router = useRouter()
const message = useMessage()

const targets = ref('')
const ipmiPort = ref(623)
const httpsPort = ref(443)
const scanId = ref('')
const progress = ref<ScanProgress | null>(null)
const results = ref<ScanFound[]>([])
const checked = ref<string[]>([])
const running = computed(() => !!progress.value && !progress.value.finished)

// Events can arrive before StartScan returns the scan id (a small range
// finishes almost instantly), so events for unknown scans are buffered.
const early = new Map<string, { found: ScanFound[]; progress?: ScanProgress }>()
function buffer(id: string) {
  if (!early.has(id)) early.set(id, { found: [] })
  return early.get(id)!
}
function addFound(f: ScanFound) {
  results.value = [...results.value, f].sort((a, b) => a.host.localeCompare(b.host, undefined, { numeric: true }))
  if (!f.managedId) checked.value = [...checked.value, f.host]
}
const offFound = api.on('scan:found', (f) => {
  if (f.scanId === scanId.value) addFound(f)
  else buffer(f.scanId).found.push(f)
})
const offProgress = api.on('scan:progress', (p) => {
  if (p.scanId === scanId.value) progress.value = p
  else buffer(p.scanId).progress = p
})
onBeforeUnmount(() => {
  offFound()
  offProgress()
})

async function start() {
  try {
    results.value = []
    checked.value = []
    progress.value = null
    const s = await api.discovery.startScan({ targets: targets.value, ipmiPort: ipmiPort.value, httpsPort: httpsPort.value })
    scanId.value = s.scanId
    const b = early.get(s.scanId)
    early.clear()
    progress.value = b?.progress ?? { scanId: s.scanId, done: 0, total: s.total, found: 0, finished: false, cancelled: false }
    b?.found.forEach(addFound)
  } catch (e) {
    message.error(errMsg(e))
  }
}
async function cancel() {
  await api.discovery.cancelScan()
}

const pct = computed(() => (progress.value?.total ? Math.round((progress.value.done / progress.value.total) * 100) : 0))

const columns: DataTableColumns<ScanFound> = [
  { type: 'selection', width: 44, disabled: (r) => r.managedId > 0 },
  { title: '地址', key: 'host', width: 150, render: (r) => h('span', { class: 'mono' }, r.host) },
  { title: '品牌', key: 'vendor', width: 170, render: (r) => h(VendorBadge, { vendor: r.vendor, name: r.vendorName }) },
  { title: '产品', key: 'product', minWidth: 160, render: (r) => r.product || h('span', { class: 'muted' }, '—') },
  {
    title: '协议', key: 'proto', width: 220,
    render: (r) => h('div', { style: 'display:flex;gap:4px' }, [
      r.ipmi ? h(NTag, { size: 'small', bordered: false }, { default: () => `IPMI ${r.ipmiVersion}` }) : null,
      r.redfish ? h(NTag, { size: 'small', bordered: false }, { default: () => `Redfish ${r.redfishVersion}` }) : null,
    ]),
  },
  {
    title: '状态', key: 'managed', width: 110,
    render: (r) => (r.managedId ? h(NTag, { size: 'small', type: 'info', bordered: false }, { default: () => '已纳管' }) : h('span', { class: 'muted' }, '新设备')),
  },
]

// ----- add modal -----
const showAdd = ref(false)
const addGroup = ref(0)
const credMode = ref<'profile' | 'custom'>('profile')
const credId = ref<number | null>(null)
const username = ref('')
const password = ref('')
const adding = ref(false)
const groupOptions = computed(() => [{ label: '未分组', value: 0 }, ...store.groups.map((g) => ({ label: g.name, value: g.id }))])
const credOptions = computed(() => store.credentials.map((c) => ({ label: `${c.name}（${c.username}）`, value: c.id })))

function openAdd() {
  credMode.value = store.credentials.length ? 'profile' : 'custom'
  credId.value = store.credentials[0]?.id ?? null
  showAdd.value = true
}

async function add() {
  const hosts = results.value.filter((r) => checked.value.includes(r.host) && !r.managedId)
  if (credMode.value === 'profile' && !credId.value) return message.warning('请选择凭据')
  if (credMode.value === 'custom' && !username.value) return message.warning('请输入 BMC 用户名')
  adding.value = true
  try {
    const res = await api.discovery.addDiscovered({
      hosts: hosts.map((r) => ({ host: r.host, name: '', ipmi: r.ipmi, redfish: r.redfish, vendor: r.vendor, tlsFingerprint: r.tlsFingerprint })),
      ipmiPort: ipmiPort.value, httpsPort: httpsPort.value, groupId: addGroup.value,
      credentialId: credMode.value === 'profile' ? credId.value ?? 0 : 0,
      username: credMode.value === 'custom' ? username.value : '', password: credMode.value === 'custom' ? password.value : '',
    })
    showAdd.value = false
    message.success(`已添加 ${res.added} 台设备，正在识别`)
    if (res.skipped.length) message.warning(`跳过 ${res.skipped.length} 台：${res.skipped.join('；')}`, { duration: 8000 })
    await store.load()
    void router.push('/devices')
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    adding.value = false
  }
}
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>扫描发现</h1>
        <p>在局域网中查找 BMC：探测 IPMI（UDP {{ ipmiPort }}）和 Redfish（HTTPS {{ httpsPort }}），并识别品牌</p>
      </div>
    </div>

    <div class="card form-card">
      <n-form label-placement="top">
        <n-grid :cols="6" :x-gap="16">
          <n-form-item-gi :span="4" label="扫描范围">
            <n-input
              v-model:value="targets"
              type="textarea"
              :autosize="{ minRows: 2, maxRows: 5 }"
              placeholder="支持多种写法，用逗号或换行分隔：&#10;10.10.1.0/24　　10.10.2.10-10.10.2.60　　10.10.3.10-60　　10.10.4.21"
              :disabled="running"
            />
          </n-form-item-gi>
          <n-form-item-gi label="IPMI 端口">
            <n-input-number v-model:value="ipmiPort" :min="1" :max="65535" :show-button="false" :disabled="running" />
          </n-form-item-gi>
          <n-form-item-gi label="HTTPS 端口">
            <n-input-number v-model:value="httpsPort" :min="1" :max="65535" :show-button="false" :disabled="running" />
          </n-form-item-gi>
        </n-grid>
      </n-form>
      <div class="actions">
        <n-button v-if="!running" type="primary" :disabled="!targets.trim()" @click="start">
          <template #icon><n-icon :component="ScanOutline" /></template>
          开始扫描
        </n-button>
        <n-button v-else @click="cancel">
          <template #icon><n-icon :component="StopCircleOutline" /></template>
          停止
        </n-button>
        <div v-if="progress" class="progress">
          <n-progress type="line" :percentage="pct" :show-indicator="false" :height="8" :status="progress.cancelled ? 'warning' : 'default'" />
          <span class="muted">
            {{ progress.finished ? (progress.cancelled ? '已停止' : '扫描完成') : '扫描中' }} · {{ progress.done }}/{{ progress.total }} · 发现 {{ results.length }} 台
          </span>
        </div>
        <span v-else class="muted" style="font-size: 12.5px">单次最多 4096 个地址</span>
      </div>
    </div>

    <div class="card results">
      <div class="results-head">
        <span class="card-title">扫描结果</span>
        <n-button type="primary" :disabled="!checked.length" @click="openAdd">
          <template #icon><n-icon :component="AddOutline" /></template>
          添加所选（{{ checked.length }}）
        </n-button>
      </div>
      <n-data-table
        v-model:checked-row-keys="checked"
        :columns="columns"
        :data="results"
        :row-key="(r: ScanFound) => r.host"
        size="small"
        :bordered="false"
        :max-height="460"
      />
    </div>

    <n-modal v-model:show="showAdd" preset="card" title="添加发现的设备" style="width: 520px">
      <n-alert type="info" :bordered="false" style="margin-bottom: 16px">
        将添加 {{ checked.length }} 台设备。添加后会自动连接 BMC 识别型号、序列号和固件版本。
      </n-alert>
      <n-form label-placement="top">
        <n-form-item label="分组">
          <n-select v-model:value="addGroup" :options="groupOptions" />
        </n-form-item>
        <n-form-item label="登录凭据">
          <n-radio-group v-model:value="credMode" size="small">
            <n-radio-button value="profile" :disabled="!store.credentials.length">使用凭据模板</n-radio-button>
            <n-radio-button value="custom">统一设置</n-radio-button>
          </n-radio-group>
        </n-form-item>
        <n-form-item v-if="credMode === 'profile'" :show-label="false">
          <n-select v-model:value="credId" :options="credOptions" placeholder="选择凭据" />
        </n-form-item>
        <n-grid v-else :cols="2" :x-gap="16">
          <n-form-item-gi label="BMC 用户名"><n-input v-model:value="username" placeholder="ADMIN" /></n-form-item-gi>
          <n-form-item-gi label="BMC 密码"><n-input v-model:value="password" type="password" show-password-on="click" /></n-form-item-gi>
        </n-grid>
      </n-form>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 8px">
          <n-button @click="showAdd = false">取消</n-button>
          <n-button type="primary" :loading="adding" @click="add">添加</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.form-card {
  padding: 18px 20px 16px;
}
.actions {
  display: flex;
  align-items: center;
  gap: 16px;
}
.progress {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 12px;
  max-width: 640px;
  font-size: 13px;
  white-space: nowrap;
}
.results {
  margin-top: 16px;
  padding: 14px 16px 16px;
}
.results-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}
</style>
