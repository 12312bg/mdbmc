<script setup lang="ts">
import { computed, h, onMounted, ref, type FunctionalComponent } from 'vue'
import { NButton, NIcon, NAlert, NSkeleton, NDescriptions, NDescriptionsItem, NTable, NCollapse, NCollapseItem, NTooltip } from 'naive-ui'
import { RefreshOutline, HardwareChipOutline, LayersOutline, ServerOutline, GitNetworkOutline, FlashOutline, CodeSlashOutline, Warning } from '@vicons/ionicons5'
import { api, errMsg, type InventoryResult } from '@/api'
import { formatBytes, formatTime, relativeTime } from '@/utils/format'
import StatusBadges from '@/components/StatusBadges.vue'

const props = defineProps<{ deviceId: number }>()
const result = ref<InventoryResult | null>(null)
const loading = ref(false)
const error = ref('')

async function load(refresh = false) {
  loading.value = true
  error.value = ''
  try {
    result.value = await api.bmc.inventory(props.deviceId, refresh)
  } catch (e) {
    error.value = errMsg(e)
  } finally {
    loading.value = false
  }
}
onMounted(() => load(false))

const inv = computed(() => result.value?.inventory)
// Why a section could not be read, shown on that section only.
const errs = computed<Record<string, string>>(() => inv.value?.errors ?? {})
const infoError = computed(() => [errs.value.system, errs.value.manager].filter(Boolean).join('\n'))

// A warning sign whose tooltip carries the error.
const WarnTip: FunctionalComponent<{ msg?: string }> = (p) =>
  p.msg
    ? h(NTooltip, { style: 'max-width: 420px; white-space: pre-line' }, {
        trigger: () => h(NIcon, { component: Warning, size: 16, color: 'var(--status-warning)', style: 'cursor: help; display: block' }),
        default: () => p.msg,
      })
    : null
const memTotal = computed(() => (inv.value?.memory ?? []).reduce((s, m) => s + m.capacityMiB, 0) / 1024)
const health = (h: string) => (h === 'ok' || h === 'warning' || h === 'critical' ? h : 'unknown') as 'ok' | 'warning' | 'critical' | 'unknown'
const summary = computed(() => {
  const i = inv.value
  if (!i) return []
  return [
    { key: 'processors', icon: HardwareChipOutline, label: '处理器', value: i.system.processorCount ? `${i.system.processorCount} 颗 · ${i.system.coreCount} 核 / ${i.system.threadCount} 线程` : `${i.processors?.length ?? 0} 颗` },
    { key: 'memory', icon: LayersOutline, label: '内存', value: `${i.system.memoryGiB || memTotal.value} GiB · ${i.memory?.length ?? 0} 条` },
    { key: 'drives', icon: ServerOutline, label: '磁盘', value: `${i.drives?.length ?? 0} 块 · ${formatBytes((i.drives ?? []).reduce((s, d) => s + d.capacityBytes, 0))}` },
    { key: 'nics', icon: GitNetworkOutline, label: '网卡', value: `${i.nics?.length ?? 0} 个接口` },
    { key: 'powerSupplies', icon: FlashOutline, label: '电源', value: `${i.powerSupplies?.length ?? 0} 个` },
    { key: 'firmware', icon: CodeSlashOutline, label: '固件', value: `${i.firmware?.length ?? 0} 项` },
  ]
})
</script>

<template>
  <div>
    <div class="bar">
      <span class="muted" style="font-size: 13px">
        <template v-if="result">
          {{ result.cached ? '缓存数据' : '最新数据' }}，采集于 {{ formatTime(result.updatedAt) }}（{{ relativeTime(result.updatedAt) }}）· 来源 {{ inv?.source.toUpperCase() }}
        </template>
      </span>
      <n-button size="small" :loading="loading" @click="load(true)">
        <template #icon><n-icon :component="RefreshOutline" /></template>
        重新采集
      </n-button>
    </div>
    <n-alert v-if="error" type="error" :bordered="false" style="margin-bottom: 12px">{{ error }}</n-alert>
    <n-skeleton v-if="loading && !inv" text :repeat="8" />
    <template v-if="inv">
      <div class="summary">
        <div v-for="s in summary" :key="s.label" class="sum-item">
          <span v-if="errs[s.key]" class="corner"><WarnTip :msg="errs[s.key]" /></span>
          <n-icon :component="s.icon" :size="20" class="sum-icon" />
          <div>
            <div class="muted" style="font-size: 12px">{{ s.label }}</div>
            <div class="sum-value">{{ s.value }}</div>
          </div>
        </div>
      </div>

      <div class="info">
      <span v-if="infoError" class="corner"><WarnTip :msg="infoError" /></span>
      <n-descriptions label-placement="left" :column="3" size="small" bordered class="block">
        <n-descriptions-item label="制造商">{{ inv.system.manufacturer || '—' }}</n-descriptions-item>
        <n-descriptions-item label="型号">{{ inv.system.model || '—' }}</n-descriptions-item>
        <n-descriptions-item label="序列号"><span class="mono">{{ inv.system.serial || '—' }}</span></n-descriptions-item>
        <n-descriptions-item label="BIOS">{{ inv.system.biosVersion || '—' }}</n-descriptions-item>
        <n-descriptions-item label="BMC 固件">{{ inv.manager.firmwareVersion || '—' }}</n-descriptions-item>
        <n-descriptions-item label="BMC MAC"><span class="mono">{{ inv.manager.macAddress || '—' }}</span></n-descriptions-item>
        <n-descriptions-item label="主机名">{{ inv.system.hostName || '—' }}</n-descriptions-item>
        <n-descriptions-item label="UUID" :span="2"><span class="mono">{{ inv.system.uuid || '—' }}</span></n-descriptions-item>
      </n-descriptions>
      </div>

      <n-collapse :default-expanded-names="['cpu', 'mem', 'disk', 'nic', 'psu', 'fw', 'fru']" class="block">
        <n-collapse-item v-if="inv.processors?.length" :title="`处理器（${inv.processors.length}）`" name="cpu">
          <template #header-extra><WarnTip :msg="errs.processors" /></template>
          <n-table size="small" :bordered="false" striped>
            <thead><tr><th>插槽</th><th>型号</th><th>核心 / 线程</th><th>最高频率</th><th>健康</th></tr></thead>
            <tbody>
              <tr v-for="p in inv.processors" :key="p.socket">
                <td>{{ p.socket }}</td><td>{{ p.model }}</td><td>{{ p.cores }} / {{ p.threads }}</td>
                <td>{{ p.maxSpeedMHz ? `${p.maxSpeedMHz} MHz` : '—' }}</td><td><StatusBadges :health="health(p.health)" /></td>
              </tr>
            </tbody>
          </n-table>
        </n-collapse-item>
        <n-collapse-item v-if="inv.memory?.length" :title="`内存（${inv.memory.length} 条 · ${memTotal} GiB）`" name="mem">
          <template #header-extra><WarnTip :msg="errs.memory" /></template>
          <n-table size="small" :bordered="false" striped>
            <thead><tr><th>插槽</th><th>容量</th><th>类型</th><th>频率</th><th>厂商</th><th>料号</th><th>序列号</th><th>健康</th></tr></thead>
            <tbody>
              <tr v-for="m in inv.memory" :key="m.slot">
                <td>{{ m.slot }}</td><td>{{ m.capacityMiB / 1024 }} GiB</td><td>{{ m.type }}</td><td>{{ m.speedMHz ? `${m.speedMHz} MHz` : '—' }}</td>
                <td>{{ m.manufacturer }}</td><td class="mono">{{ m.partNumber }}</td><td class="mono">{{ m.serial }}</td><td><StatusBadges :health="health(m.health)" /></td>
              </tr>
            </tbody>
          </n-table>
        </n-collapse-item>
        <n-collapse-item v-if="inv.drives?.length" :title="`磁盘（${inv.drives.length}）`" name="disk">
          <template #header-extra><WarnTip :msg="errs.drives" /></template>
          <n-table size="small" :bordered="false" striped>
            <thead><tr><th>名称</th><th>型号</th><th>容量</th><th>介质 / 接口</th><th>固件</th><th>序列号</th><th>健康</th></tr></thead>
            <tbody>
              <tr v-for="d in inv.drives" :key="d.name + d.serial">
                <td>{{ d.name }}</td><td>{{ d.model }}</td><td>{{ formatBytes(d.capacityBytes) }}</td><td>{{ d.mediaType }} / {{ d.protocol }}</td>
                <td>{{ d.firmware }}</td><td class="mono">{{ d.serial }}</td><td><StatusBadges :health="health(d.health)" /></td>
              </tr>
            </tbody>
          </n-table>
        </n-collapse-item>
        <n-collapse-item v-if="inv.nics?.length" :title="`网卡（${inv.nics.length}）`" name="nic">
          <template #header-extra><WarnTip :msg="errs.nics" /></template>
          <n-table size="small" :bordered="false" striped>
            <thead><tr><th>名称</th><th>MAC</th><th>速率</th><th>链路</th><th>IPv4</th></tr></thead>
            <tbody>
              <tr v-for="n in inv.nics" :key="n.name + n.macAddress">
                <td>{{ n.name }}</td><td class="mono">{{ n.macAddress }}</td><td>{{ n.speedMbps ? `${n.speedMbps / 1000} Gb/s` : '—' }}</td>
                <td>{{ n.linkStatus === 'LinkUp' ? '已连接' : n.linkStatus === 'LinkDown' ? '未连接' : n.linkStatus || '—' }}</td>
                <td class="mono">{{ n.ipv4?.join(', ') || '—' }}</td>
              </tr>
            </tbody>
          </n-table>
        </n-collapse-item>
        <n-collapse-item v-if="inv.powerSupplies?.length" :title="`电源（${inv.powerSupplies.length}）`" name="psu">
          <template #header-extra><WarnTip :msg="errs.powerSupplies" /></template>
          <n-table size="small" :bordered="false" striped>
            <thead><tr><th>名称</th><th>型号</th><th>额定功率</th><th>固件</th><th>序列号</th><th>健康</th></tr></thead>
            <tbody>
              <tr v-for="p in inv.powerSupplies" :key="p.name">
                <td>{{ p.name }}</td><td>{{ p.model }}</td><td>{{ p.capacityWatts ? `${p.capacityWatts} W` : '—' }}</td><td>{{ p.firmware }}</td>
                <td class="mono">{{ p.serial }}</td><td><StatusBadges :health="health(p.health)" /></td>
              </tr>
            </tbody>
          </n-table>
        </n-collapse-item>
        <n-collapse-item v-if="inv.firmware?.length" :title="`固件版本（${inv.firmware.length}）`" name="fw">
          <template #header-extra><WarnTip :msg="errs.firmware" /></template>
          <n-table size="small" :bordered="false" striped>
            <thead><tr><th>组件</th><th>版本</th><th>可升级</th></tr></thead>
            <tbody>
              <tr v-for="f in inv.firmware" :key="f.name"><td>{{ f.name }}</td><td class="mono">{{ f.version }}</td><td>{{ f.updateable ? '是' : '否' }}</td></tr>
            </tbody>
          </n-table>
        </n-collapse-item>
        <n-collapse-item v-if="inv.fru?.length || errs.fru" :title="`IPMI FRU（${inv.fru?.length ?? 0}）`" name="fru">
          <template #header-extra><WarnTip :msg="errs.fru" /></template>
          <div v-if="!inv.fru?.length" class="muted" style="font-size: 13px">未能读取</div>
          <n-table v-else size="small" :bordered="false" striped>
            <thead><tr><th>FRU 设备</th><th>主板</th><th>主板序列号</th><th>产品</th><th>产品序列号</th><th>机箱序列号</th></tr></thead>
            <tbody>
              <tr v-for="f in inv.fru" :key="f.device">
                <td>{{ f.device }}</td><td>{{ [f.boardMfg, f.boardProduct].filter(Boolean).join(' ') || '—' }}</td><td class="mono">{{ f.boardSerial || '—' }}</td>
                <td>{{ [f.productMfg, f.productName].filter(Boolean).join(' ') || '—' }}</td><td class="mono">{{ f.productSerial || '—' }}</td><td class="mono">{{ f.chassisSerial || '—' }}</td>
              </tr>
            </tbody>
          </n-table>
        </n-collapse-item>
      </n-collapse>
    </template>
  </div>
</template>

<style scoped>
.bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}
.summary {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}
.sum-item {
  position: relative;
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 12px;
  border-radius: 8px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  min-width: 0;
}
.sum-icon {
  color: var(--accent);
  flex: none;
}
.sum-value {
  font-weight: 500;
  font-size: 13px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.block {
  margin-bottom: 16px;
}
.info {
  position: relative;
}
/* A warning badge on the item's top-right corner. */
.corner {
  position: absolute;
  top: -7px;
  right: -7px;
  z-index: 1;
  padding: 1px;
  border-radius: 50%;
  background: var(--surface);
  line-height: 0;
}
@media (max-width: 1300px) {
  .summary {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}
</style>
