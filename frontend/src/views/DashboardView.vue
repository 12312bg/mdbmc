<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NIcon, NSkeleton, NEmpty, NTag } from 'naive-ui'
import { CheckmarkCircle, Warning, CloseCircle, HelpCircle, AddOutline, ScanOutline, ChevronForward } from '@vicons/ionicons5'
import StatTile from '@/components/charts/StatTile.vue'
import StackedBar from '@/components/charts/StackedBar.vue'
import type { Segment } from '@/components/charts/types'
import BarList from '@/components/charts/BarList.vue'
import StatusBadges from '@/components/StatusBadges.vue'
import VendorBadge from '@/components/VendorBadge.vue'
import { api, errMsg, type Overview } from '@/api'
import { useSession } from '@/stores/session'
import { auditActionLabel, relativeTime } from '@/utils/format'

const session = useSession()
const router = useRouter()
const ov = ref<Overview | null>(null)
const error = ref('')

async function load() {
  try {
    ov.value = await api.bmc.overview()
    error.value = ''
  } catch (e) {
    error.value = errMsg(e)
  }
}

let offStatus: (() => void) | undefined
let offPoll: (() => void) | undefined
let reloadTimer: number | undefined
function scheduleReload() {
  window.clearTimeout(reloadTimer)
  reloadTimer = window.setTimeout(load, 800)
}
onMounted(() => {
  void load()
  offStatus = api.on('device:status', scheduleReload)
  offPoll = api.on('poll:state', (s) => !s.running && scheduleReload())
})
onBeforeUnmount(() => {
  offStatus?.()
  offPoll?.()
  window.clearTimeout(reloadTimer)
})

const alerts = computed(() => (ov.value ? ov.value.warning + ov.value.critical : 0))

const powerSegments = computed<Segment[]>(() => {
  const o = ov.value
  if (!o) return []
  return [
    { key: 'on', label: '已开机', value: o.poweredOn, color: 'var(--power-on)' },
    { key: 'off', label: '已关机', value: o.poweredOff, color: 'var(--power-off)' },
    { key: 'unknown', label: '未知', value: Math.max(0, o.total - o.poweredOn - o.poweredOff), color: 'var(--status-unknown)' },
  ]
})
const healthSegments = computed<Segment[]>(() => {
  const o = ov.value
  if (!o) return []
  return [
    { key: 'ok', label: '正常', value: o.healthOk, color: 'var(--status-good)', icon: CheckmarkCircle },
    { key: 'warning', label: '警告', value: o.warning, color: 'var(--status-warning)', icon: Warning },
    { key: 'critical', label: '严重', value: o.critical, color: 'var(--status-critical)', icon: CloseCircle },
    { key: 'unknown', label: '未知', value: o.unknown, color: 'var(--status-unknown)', icon: HelpCircle },
  ]
})
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>总览</h1>
      </div>
      <div v-if="session.can('device.manage')" style="display: flex; gap: 8px">
        <n-button @click="router.push('/discovery')">
          <template #icon><n-icon :component="ScanOutline" /></template>
          扫描发现
        </n-button>
        <n-button type="primary" @click="router.push({ path: '/devices', query: { add: '1' } })">
          <template #icon><n-icon :component="AddOutline" /></template>
          添加设备
        </n-button>
      </div>
    </div>

    <div v-if="error" class="card" style="padding: 24px">{{ error }}</div>

    <template v-else-if="!ov">
      <div class="tiles">
        <n-skeleton v-for="i in 4" :key="i" height="104px" :sharp="false" />
      </div>
      <n-skeleton height="240px" :sharp="false" style="margin-top: 16px" />
    </template>

    <template v-else-if="ov.total === 0">
      <div class="card empty-state">
        <n-empty description="还没有受管设备">
          <template #extra>
            <div v-if="session.can('device.manage')" style="display: flex; gap: 8px; justify-content: center">
              <n-button @click="router.push('/discovery')">扫描局域网</n-button>
              <n-button type="primary" @click="router.push({ path: '/devices', query: { add: '1' } })">手动添加</n-button>
            </div>
          </template>
        </n-empty>
      </div>
    </template>

    <template v-else>
      <div class="tiles">
        <StatTile label="受管设备" :value="ov.total" :hint="`${ov.byVendor?.length ?? 0} 个品牌`" />
        <StatTile label="在线" :value="ov.reachable" :hint="ov.unreachable ? `${ov.unreachable} 台无法连接` : '全部可连接'" />
        <StatTile label="已开机" :value="ov.poweredOn" :hint="`${ov.poweredOff} 台已关机`" />
        <StatTile label="健康告警" :value="alerts" :hint="`严重 ${ov.critical} · 警告 ${ov.warning}`" :tone="ov.critical ? 'critical' : ov.warning ? 'warning' : 'good'" />
      </div>

      <div class="grid">
        <section class="card panel">
          <h3 class="card-title">电源状态</h3>
          <StackedBar :segments="powerSegments" :total="ov.total" />
          <h3 class="card-title" style="margin-top: 28px">健康状态</h3>
          <StackedBar :segments="healthSegments" :total="ov.total" />
        </section>
        <section class="card panel">
          <h3 class="card-title">品牌分布</h3>
          <BarList :items="ov.byVendor ?? []" />
        </section>
        <section class="card panel">
          <h3 class="card-title">分组分布</h3>
          <BarList :items="ov.byGroup ?? []" />
        </section>
      </div>

      <div class="grid two">
        <section class="card panel">
          <div class="panel-head">
            <h3 class="card-title">需要关注</h3>
            <n-button text size="small" @click="router.push({ path: '/devices', query: { status: 'problem' } })">
              查看全部 <n-icon :component="ChevronForward" />
            </n-button>
          </div>
          <div v-if="!ov.problems.length" class="ok-note">
            <n-icon :component="CheckmarkCircle" color="var(--status-good)" :size="18" />
            所有设备运行正常
          </div>
          <div v-else class="problems">
            <div v-for="d in ov.problems" :key="d.id" class="problem" @click="router.push(`/devices/${d.id}`)">
              <div class="p-main">
                <div class="p-name">{{ d.name }} <span class="muted mono">{{ d.host }}</span></div>
                <div class="p-sub muted">
                  <VendorBadge :vendor="d.vendor" :name="d.vendorName" />
                  <span v-if="d.model">· {{ d.model }}</span>
                </div>
              </div>
              <div class="p-state">
                <StatusBadges v-if="!d.reachable" :power="d.powerState" :reachable="false" :checked="true" />
                <StatusBadges v-else :health="d.health" />
              </div>
            </div>
          </div>
        </section>
        <section class="card panel">
          <div class="panel-head">
            <h3 class="card-title">最近操作</h3>
            <n-button v-if="session.can('audit.view')" text size="small" @click="router.push('/audit')">
              审计日志 <n-icon :component="ChevronForward" />
            </n-button>
          </div>
          <div v-if="!session.can('audit.view')" class="muted" style="font-size: 13px">仅管理员可查看操作记录。</div>
          <div v-else-if="!ov.recent.length" class="muted" style="font-size: 13px">暂无操作记录</div>
          <div v-else class="recent">
            <div v-for="e in ov.recent" :key="e.id" class="r-row">
              <n-tag size="small" :bordered="false" :type="e.success ? 'default' : 'error'">{{ auditActionLabel[e.action] ?? e.action }}</n-tag>
              <span class="r-target">{{ e.target || '—' }}</span>
              <span class="muted r-meta">{{ e.username }} · {{ relativeTime(e.ts) }}</span>
            </div>
          </div>
        </section>
      </div>
    </template>
  </div>
</template>

<style scoped>
.tiles {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}
.grid {
  display: grid;
  grid-template-columns: 1.3fr 1fr 1fr;
  gap: 16px;
  margin-top: 16px;
}
.grid.two {
  grid-template-columns: 1.3fr 1fr;
}
.panel {
  padding: 18px 20px 20px;
  min-width: 0;
}
.panel .card-title {
  margin-bottom: 14px;
}
.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}
.panel-head .card-title {
  margin: 0;
}
.empty-state {
  padding: 64px 0;
}
.ok-note {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-2);
  padding: 12px 0;
}
.problems {
  display: flex;
  flex-direction: column;
}
.problem {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 8px;
  margin: 0 -8px;
  border-radius: 6px;
  cursor: pointer;
}
.problem + .problem {
  border-top: 1px solid var(--border);
}
.problem:hover {
  background: var(--surface-2);
}
.p-main {
  min-width: 0;
}
.p-name {
  font-weight: 500;
}
.p-sub {
  display: flex;
  gap: 6px;
  align-items: center;
  font-size: 12.5px;
  margin-top: 4px;
}
.recent {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.r-row {
  display: grid;
  grid-template-columns: auto 1fr auto;
  gap: 10px;
  align-items: center;
  font-size: 13px;
}
.r-target {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.r-meta {
  font-size: 12px;
  white-space: nowrap;
}
@media (max-width: 1280px) {
  .grid {
    grid-template-columns: 1fr 1fr;
  }
  .grid .panel:first-child {
    grid-column: span 2;
  }
  .grid.two {
    grid-template-columns: 1fr;
  }
  .grid.two .panel:first-child {
    grid-column: auto;
  }
}
</style>
