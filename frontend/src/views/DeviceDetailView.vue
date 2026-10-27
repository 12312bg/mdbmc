<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NButton, NIcon, NTabs, NTabPane, NDropdown, NDescriptions, NDescriptionsItem, NTag, NAlert, NSkeleton, NPopconfirm,
  NTooltip, useMessage, useDialog,
} from 'naive-ui'
import {
  ArrowBackOutline, GlobeOutline, DesktopOutline, PowerOutline, ChevronDown, BulbOutline, RefreshOutline, CreateOutline,
  EyeOutline, FingerPrintOutline, CopyOutline,
} from '@vicons/ionicons5'
import StatusBadges from '@/components/StatusBadges.vue'
import VendorBadge from '@/components/VendorBadge.vue'
import DeviceFormModal from '@/components/DeviceFormModal.vue'
import SensorsPanel from './device/SensorsPanel.vue'
import SelPanel from './device/SelPanel.vue'
import InventoryPanel from './device/InventoryPanel.vue'
import { api, errMsg, type Device, type PowerAction } from '@/api'
import { useDevices } from '@/stores/devices'
import { useSession } from '@/stores/session'
import { useDeviceActions } from '@/composables/useDeviceActions'
import { formatTime, powerActions, protocolLabel, relativeTime } from '@/utils/format'

const route = useRoute()
const router = useRouter()
const store = useDevices()
const session = useSession()
const message = useMessage()
const dialog = useDialog()
const actions = useDeviceActions()

const id = computed(() => Number(route.params.id))
const device = ref<Device | null>(null)
const error = ref('')
const tab = ref(typeof route.query.tab === 'string' ? route.query.tab : 'overview')
const identifying = ref(false)
const showEdit = ref(false)

async function load() {
  try {
    device.value = await api.devices.get(id.value)
    error.value = ''
  } catch (e) {
    error.value = errMsg(e)
  }
}
onMounted(load)

// Keep the header in sync with live status pushed to the shared store.
watch(() => store.byId.get(id.value), (d) => {
  if (d && device.value) Object.assign(device.value, { powerState: d.powerState, health: d.health, reachable: d.reachable, lastError: d.lastError, checkedAt: d.checkedAt })
}, { deep: true })
const offUpdated = api.on('device:updated', (did) => did === id.value && load())
onBeforeUnmount(offUpdated)

const powerOptions = powerActions.map((a) => ({ label: a.label, key: a.key, props: a.danger ? { style: 'color: var(--status-critical)' } : undefined }))
async function power(a: PowerAction) {
  if (device.value) await actions.power([device.value.id], a, [device.value.name])
}

async function identify() {
  identifying.value = true
  try {
    device.value = await api.devices.identify(id.value)
    message.success('已重新识别设备信息')
    await store.load()
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    identifying.value = false
  }
}

async function reveal() {
  try {
    const pw = await api.devices.revealPassword(id.value)
    dialog.info({
      title: 'BMC 密码',
      content: () => pw || '（空密码）',
      positiveText: '复制并关闭',
      negativeText: '关闭',
      onPositiveClick: () => navigator.clipboard?.writeText(pw),
    })
  } catch (e) {
    message.error(errMsg(e))
  }
}

async function trustCert() {
  try {
    const fp = await api.devices.trustCertificate(id.value)
    message.success('已信任当前证书')
    if (device.value) device.value.tlsFingerprint = fp
  } catch (e) {
    message.error(errMsg(e))
  }
}

function copy(text: string) {
  void navigator.clipboard?.writeText(text)
  message.success('已复制')
}

const fpPretty = computed(() => device.value?.tlsFingerprint.match(/.{2}/g)?.join(':').toUpperCase() ?? '')
const offline = computed(() => !!device.value && !device.value.reachable && device.value.checkedAt > 0)
const certError = computed(() => device.value?.lastError.includes('证书指纹') ?? false)
</script>

<template>
  <div class="page">
    <n-button text class="back" @click="router.push('/devices')">
      <template #icon><n-icon :component="ArrowBackOutline" /></template>
      设备列表
    </n-button>

    <div v-if="error" class="card" style="padding: 24px">{{ error }}</div>
    <n-skeleton v-else-if="!device" height="140px" :sharp="false" />

    <template v-else>
      <div class="hero card">
        <div class="hero-main">
          <div class="title-row">
            <h1>{{ device.name }}</h1>
            <span class="mono muted">{{ device.host }}</span>
          </div>
          <div class="meta">
            <VendorBadge :vendor="device.vendor" :name="device.vendorName" size="medium" />
            <span v-if="device.model" class="secondary">{{ device.model }}</span>
            <span class="sep" />
            <StatusBadges :power="device.powerState" :reachable="device.reachable" :checked="device.checkedAt > 0" />
            <StatusBadges :health="device.reachable || !device.checkedAt ? device.health : 'unknown'" />
            <span class="muted" style="font-size: 12.5px">检查于 {{ relativeTime(device.checkedAt) }}</span>
          </div>
        </div>
        <div class="hero-actions">
          <n-button @click="actions.openWeb(device.id)">
            <template #icon><n-icon :component="GlobeOutline" /></template>
            Web 管理
          </n-button>
          <n-button v-if="session.can('console.kvm')" type="primary" @click="actions.openKVM(device.id)">
            <template #icon><n-icon :component="DesktopOutline" /></template>
            远程控制台
          </n-button>
          <n-dropdown v-if="session.can('power.control')" :options="powerOptions" trigger="click" @select="power">
            <n-button>
              <template #icon><n-icon :component="PowerOutline" /></template>
              电源
              <n-icon :component="ChevronDown" style="margin-left: 4px" />
            </n-button>
          </n-dropdown>
          <n-dropdown
            v-if="session.can('power.control')"
            trigger="click"
            :options="[{ label: '点亮定位灯', key: 'on' }, { label: '关闭定位灯', key: 'off' }]"
            @select="(k: string) => actions.identify(device!.id, k === 'on')"
          >
            <n-button>
              <template #icon><n-icon :component="BulbOutline" /></template>
              定位灯
            </n-button>
          </n-dropdown>
          <n-tooltip>
            <template #trigger>
              <n-button :loading="identifying" @click="identify">
                <template #icon><n-icon :component="RefreshOutline" /></template>
              </n-button>
            </template>
            重新识别品牌、型号与协议
          </n-tooltip>
          <n-button v-if="session.can('device.manage')" @click="showEdit = true">
            <template #icon><n-icon :component="CreateOutline" /></template>
            编辑
          </n-button>
        </div>
      </div>

      <n-alert v-if="offline" :type="certError ? 'warning' : 'error'" :bordered="false" class="alert" :title="certError ? 'BMC 证书已变化' : '无法连接 BMC'">
        {{ device.lastError }}
        <template v-if="certError && session.can('device.manage')">
          <n-popconfirm @positive-click="trustCert">
            <template #trigger><n-button size="small" style="margin-left: 12px">信任当前证书</n-button></template>
            请确认 BMC 确实更换过证书（例如升级固件或重新生成证书），否则可能是中间人攻击。
          </n-popconfirm>
        </template>
      </n-alert>

      <div class="card tabs-card">
        <n-tabs v-model:value="tab" type="line" animated pane-style="padding-top: 16px">
          <n-tab-pane name="overview" tab="概览">
            <n-descriptions label-placement="left" :column="2" bordered size="small" label-style="width: 140px">
              <n-descriptions-item label="制造商">{{ device.manufacturer || '—' }}</n-descriptions-item>
              <n-descriptions-item label="型号">{{ device.model || '—' }}</n-descriptions-item>
              <n-descriptions-item label="序列号"><span class="mono">{{ device.serial || '—' }}</span></n-descriptions-item>
              <n-descriptions-item label="BIOS 版本">{{ device.biosVersion || '—' }}</n-descriptions-item>
              <n-descriptions-item label="BMC 固件">{{ device.bmcFirmware || '—' }}</n-descriptions-item>
              <n-descriptions-item label="支持协议">
                <n-tag v-if="device.hasIpmi" size="small" :bordered="false" style="margin-right: 4px">IPMI 2.0</n-tag>
                <n-tag v-if="device.hasRedfish" size="small" :bordered="false">Redfish</n-tag>
                <span v-if="!device.hasIpmi && !device.hasRedfish" class="muted">尚未识别</span>
              </n-descriptions-item>
              <n-descriptions-item label="管理协议">{{ protocolLabel[device.protocol] }}</n-descriptions-item>
              <n-descriptions-item label="端口">IPMI {{ device.ipmiPort }}/UDP · HTTPS {{ device.httpsPort }}/TCP</n-descriptions-item>
              <n-descriptions-item label="分组">{{ device.groupName || '未分组' }}</n-descriptions-item>
              <n-descriptions-item label="登录凭据">
                <span v-if="device.credentialId">{{ device.credentialName }}（模板）</span>
                <span v-else>{{ device.username || '—' }}（单独设置）</span>
                <n-button v-if="session.can('credential.reveal')" text type="primary" size="small" style="margin-left: 8px" @click="reveal">
                  <template #icon><n-icon :component="EyeOutline" /></template>
                  查看密码
                </n-button>
              </n-descriptions-item>
              <n-descriptions-item label="证书指纹" :span="2">
                <template v-if="device.tlsFingerprint">
                  <n-icon :component="FingerPrintOutline" class="muted" style="vertical-align: -2px; margin-right: 4px" />
                  <span class="mono fp">SHA-256 {{ fpPretty }}</span>
                  <n-button text size="small" style="margin-left: 6px" @click="copy(fpPretty)">
                    <template #icon><n-icon :component="CopyOutline" /></template>
                  </n-button>
                </template>
                <span v-else class="muted">首次通过 HTTPS 连接时自动记录</span>
              </n-descriptions-item>
              <n-descriptions-item label="识别时间">{{ formatTime(device.identifiedAt) }}</n-descriptions-item>
              <n-descriptions-item label="添加时间">{{ formatTime(device.createdAt) }}</n-descriptions-item>
              <n-descriptions-item v-if="device.note" label="备注" :span="2">{{ device.note }}</n-descriptions-item>
            </n-descriptions>
          </n-tab-pane>
          <n-tab-pane name="sensors" tab="传感器" display-directive="show:lazy">
            <SensorsPanel :device-id="device.id" />
          </n-tab-pane>
          <n-tab-pane name="sel" tab="事件日志" display-directive="show:lazy">
            <SelPanel :device-id="device.id" :device-name="device.name" />
          </n-tab-pane>
          <n-tab-pane name="inventory" tab="硬件资产" display-directive="show:lazy">
            <InventoryPanel :device-id="device.id" />
          </n-tab-pane>
        </n-tabs>
      </div>

      <DeviceFormModal v-model:show="showEdit" :device="device" @saved="(d: Device) => { device = d; store.load() }" />
    </template>
  </div>
</template>

<style scoped>
.back {
  margin-bottom: 12px;
}
.hero {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  padding: 18px 20px;
  flex-wrap: wrap;
}
.title-row {
  display: flex;
  align-items: baseline;
  gap: 12px;
}
.title-row h1 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
}
.meta {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-top: 10px;
  flex-wrap: wrap;
}
.sep {
  width: 1px;
  height: 16px;
  background: var(--border);
}
.hero-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.alert {
  margin-top: 12px;
}
.tabs-card {
  margin-top: 16px;
  padding: 4px 20px 20px;
}
.fp {
  word-break: break-all;
}
</style>
