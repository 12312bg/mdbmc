<script setup lang="ts">
import { computed, onMounted, ref, type Component } from 'vue'
import {
  NForm, NFormItem, NInputNumber, NInput, NButton, NAlert, NDescriptions, NDescriptionsItem, NIcon, NTag, NSpin, useMessage,
} from 'naive-ui'
import { CheckmarkCircle, AlertCircleOutline, ArrowUpCircleOutline, TimeOutline, CloudDownloadOutline } from '@vicons/ionicons5'
import AppLogo from '@/components/AppLogo.vue'
import { api, errMsg, type Settings, type UpdateConfig } from '@/api'
import { useSession } from '@/stores/session'
import { useUpdate } from '@/stores/update'
import { relativeTime } from '@/utils/format'

const session = useSession()
const upd = useUpdate()
const message = useMessage()
const form = ref<Settings | null>(null)
const saving = ref(false)
const canEdit = computed(() => session.can('settings.manage'))
const canUpdate = computed(() => session.can('system.update'))
const updForm = ref<UpdateConfig | null>(null)
const savingUpd = ref(false)
const checking = ref(false)

onMounted(async () => {
  try {
    form.value = await api.admin.getSettings()
    if (canUpdate.value) updForm.value = await api.update.getConfig()
  } catch (e) {
    message.error(errMsg(e))
  }
})

async function saveUpdateConfig() {
  if (!updForm.value) return
  savingUpd.value = true
  try {
    await api.update.saveConfig(updForm.value)
    updForm.value = await api.update.getConfig()
    message.success('更新设置已保存')
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    savingUpd.value = false
  }
}

async function checkNow() {
  checking.value = true
  try {
    await upd.check()
    if (!upd.latest) message.success('已是最新版本')
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    checking.value = false
  }
}

const modeLabel = computed(() => ({ installer: '安装版', portable: '免安装版', unsupported: '开发版' })[upd.status?.mode ?? 'unsupported'])

// One line on where updating stands, with an icon and a tone.
const updLine = computed<{ icon: Component | null; tone: string; title: string; sub: string }>(() => {
  const s = upd.status
  const last = s?.checkedAt ? `上次检查：${relativeTime(s.checkedAt)}` : '尚未检查'
  if (!s) return { icon: TimeOutline, tone: 'muted', title: '正在读取更新状态…', sub: '' }
  if (s.state === 'checking') return { icon: null, tone: 'muted', title: '正在检查更新…', sub: last }
  const v = s.latest?.version
  if (s.state === 'downloading') return { icon: CloudDownloadOutline, tone: 'accent', title: `正在下载 v${v}（${upd.progress.toFixed(0)}%）`, sub: last }
  if (s.state === 'ready' || s.state === 'installing') return { icon: CheckmarkCircle, tone: 'good', title: `v${v} 已下载，可以安装`, sub: last }
  if (v) return { icon: ArrowUpCircleOutline, tone: 'accent', title: `发现新版本 v${v}`, sub: last }
  if (s.error) return { icon: AlertCircleOutline, tone: 'bad', title: '检查更新失败', sub: s.error.replace(/^检查更新失败：/, '') }
  if (s.state === 'upToDate') return { icon: CheckmarkCircle, tone: 'good', title: '已是最新版本', sub: last }
  return { icon: TimeOutline, tone: 'muted', title: '尚未检查更新', sub: '' }
})

async function save() {
  if (!form.value) return
  saving.value = true
  try {
    await api.admin.saveSettings(form.value)
    await session.loadSettings()
    message.success('设置已保存')
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>系统设置</h1>
        <p>{{ canEdit ? '修改后立即生效' : '仅管理员可以修改设置' }}</p>
      </div>
    </div>
    <div class="grid">
      <div class="card section">
        <div class="card-title">状态与连接</div>
        <n-form v-if="form" label-placement="left" label-width="150" :disabled="!canEdit" class="form">
          <n-form-item label="状态刷新间隔">
            <n-input-number v-model:value="form.pollIntervalSec" :min="0" :max="86400" :step="30" style="width: 180px">
              <template #suffix>秒</template>
            </n-input-number>
            <span class="tip">0 表示关闭自动刷新，最小 15 秒</span>
          </n-form-item>
          <n-form-item label="最大并发操作数">
            <n-input-number v-model:value="form.maxConcurrency" :min="1" :max="256" style="width: 180px" />
            <span class="tip">同时与多少台 BMC 通信；每台 BMC 最多 2 个会话</span>
          </n-form-item>
          <n-form-item label="IPMI 超时">
            <n-input-number v-model:value="form.ipmiTimeoutSec" :min="1" :max="60" style="width: 180px">
              <template #suffix>秒</template>
            </n-input-number>
          </n-form-item>
          <n-form-item label="Redfish 超时">
            <n-input-number v-model:value="form.redfishTimeoutSec" :min="3" :max="300" style="width: 180px">
              <template #suffix>秒</template>
            </n-input-number>
          </n-form-item>
          <n-form-item label="扫描并发数">
            <n-input-number v-model:value="form.scanConcurrency" :min="1" :max="512" style="width: 180px" />
          </n-form-item>
        </n-form>
        <div class="card-title" style="margin-top: 8px">安全</div>
        <n-form v-if="form" label-placement="left" label-width="150" :disabled="!canEdit" class="form">
          <n-form-item label="空闲自动锁定">
            <n-input-number v-model:value="form.idleLockMinutes" :min="0" :max="1440" style="width: 180px">
              <template #suffix>分钟</template>
            </n-input-number>
            <span class="tip">无操作超过该时间后退出登录并清除内存中的密钥，0 表示不锁定</span>
          </n-form-item>
        </n-form>
        <n-button v-if="canEdit" type="primary" :loading="saving" style="margin-left: 150px" @click="save">保存设置</n-button>
        <template v-if="canUpdate && updForm">
          <div class="divider" />
          <div class="card-title">软件更新</div>
          <n-form label-placement="left" label-width="150" class="form">
            <n-form-item label="自动检查更新">
              <n-input-number v-model:value="updForm.checkHours" :min="0" :max="720" style="width: 180px">
                <template #suffix>小时</template>
              </n-input-number>
              <span class="tip">启动后及每隔该时间检查一次，0 表示不自动检查</span>
            </n-form-item>
            <n-form-item label="更新源">
              <div class="field">
                <n-input v-model:value="updForm.source" :placeholder="updForm.defaultSource" clearable />
                <div class="hint">留空使用 GitHub Releases。可填镜像地址或内网服务器（目录结构与 GitHub Releases 相同）；签名不符的更新一律拒绝。</div>
              </div>
            </n-form-item>
            <n-form-item label="代理">
              <div class="field">
                <n-input v-model:value="updForm.proxy" placeholder="留空则使用系统代理" clearable />
                <div class="hint">例如 http://10.0.0.1:3128 或 socks5://127.0.0.1:1080，只用于检查和下载更新。</div>
              </div>
            </n-form-item>
          </n-form>
          <n-button type="primary" :loading="savingUpd" style="margin-left: 150px" @click="saveUpdateConfig">保存更新设置</n-button>
        </template>
      </div>
      <div class="side">
        <div class="card section">
          <div class="about-head">
            <AppLogo :size="44" plate="dark" />
            <div>
              <div class="about-name">明点服务器管理系统</div>
              <div class="about-ver">
                <span>版本 {{ session.status?.version }}</span>
                <n-tag size="small" :bordered="false">{{ modeLabel }}</n-tag>
              </div>
            </div>
          </div>
          <div class="upd-line" :class="`tone-${updLine.tone}`">
            <n-spin v-if="!updLine.icon" :size="16" />
            <n-icon v-else :component="updLine.icon" :size="20" />
            <div class="upd-line-text">
              <div class="upd-line-title">{{ updLine.title }}</div>
              <div v-if="updLine.sub" class="upd-line-sub">{{ updLine.sub }}</div>
            </div>
          </div>
          <div class="about-actions">
            <n-button :loading="checking || upd.state === 'checking'" @click="checkNow">检查更新</n-button>
            <n-button v-if="upd.latest" type="primary" @click="upd.dialog = true">查看新版本</n-button>
          </div>
          <n-descriptions :column="1" label-placement="left" size="small" class="about-desc">
            <n-descriptions-item label="数据目录"><span class="mono" style="word-break: break-all">{{ session.status?.dataDir }}</span></n-descriptions-item>
            <n-descriptions-item label="协议">IPMI 2.0 (RMCP+) · Redfish</n-descriptions-item>
          </n-descriptions>
        </div>
        <n-alert type="info" :bordered="false" title="数据迁移">
          数据目录中的 mdbmc.db 包含全部设备与加密后的 BMC 密码。复制到其他电脑后，用原有账号密码登录即可解密。
        </n-alert>
      </div>
    </div>
  </div>
</template>

<style scoped>
.grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 380px;
  gap: 16px;
  align-items: start;
}
.section {
  padding: 18px 20px 20px;
}
.form {
  margin-top: 14px;
}
.tip {
  margin-left: 12px;
  font-size: 12.5px;
  color: var(--text-3);
}
.divider {
  height: 1px;
  margin: 24px -20px 18px;
  background: var(--border);
}
.field {
  width: 100%;
  max-width: 460px;
}
.hint {
  margin-top: 4px;
  font-size: 12.5px;
  line-height: 18px;
  color: var(--text-3);
}
.about-head {
  display: flex;
  align-items: center;
  gap: 12px;
}
.about-name {
  font-size: 15px;
  font-weight: 600;
}
.about-ver {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 2px;
  font-size: 13px;
  color: var(--text-2);
  font-variant-numeric: tabular-nums;
}
.upd-line {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin-top: 16px;
  padding: 12px 14px;
  border-radius: 8px;
  background: var(--surface-2);
  border: 1px solid var(--border);
}
.upd-line > :first-child {
  flex: none;
  margin-top: 1px;
}
.upd-line-title {
  font-size: 13.5px;
  font-weight: 500;
  color: var(--text-1);
}
.upd-line-sub {
  margin-top: 2px;
  font-size: 12.5px;
  color: var(--text-3);
  word-break: break-word;
}
.tone-good > .n-icon {
  color: var(--status-good);
}
.tone-accent > .n-icon {
  color: var(--accent);
}
.tone-accent {
  border-color: color-mix(in srgb, var(--accent) 35%, transparent);
  background: var(--accent-soft);
}
.tone-bad > .n-icon {
  color: var(--status-critical);
}
.tone-muted > .n-icon {
  color: var(--text-3);
}
.about-actions {
  display: flex;
  gap: 8px;
  margin-top: 12px;
}
.about-desc {
  margin-top: 16px;
  padding-top: 14px;
  border-top: 1px solid var(--border);
}
.side {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
@media (max-width: 1200px) {
  .grid {
    grid-template-columns: 1fr;
  }
}
</style>
