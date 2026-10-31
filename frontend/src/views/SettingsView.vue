<script setup lang="ts">
import { computed, onMounted, ref, type Component } from 'vue'
import {
  NForm, NFormItem, NInputNumber, NInput, NButton, NAlert, NDescriptions, NDescriptionsItem, NIcon, NTag, NSpin, NCheckbox, NModal,
  useMessage,
} from 'naive-ui'
import {
  CheckmarkCircle, AlertCircleOutline, ArrowUpCircleOutline, TimeOutline, CloudDownloadOutline, SaveOutline, RefreshCircleOutline,
} from '@vicons/ionicons5'
import AppLogo from '@/components/AppLogo.vue'
import { api, errMsg, type AlertConfig, type BackupFile, type Settings, type UpdateConfig } from '@/api'
import { useSession } from '@/stores/session'
import { useUpdate } from '@/stores/update'
import { formatBytes, formatTime, relativeTime } from '@/utils/format'

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
const alertForm = ref<AlertConfig | null>(null)

onMounted(async () => {
  try {
    form.value = await api.admin.getSettings()
    if (canUpdate.value) updForm.value = await api.update.getConfig()
    if (canEdit.value) alertForm.value = await api.alerts.getConfig()
  } catch (e) {
    message.error(errMsg(e))
  }
})

const savingAlert = ref(false)
async function saveAlert() {
  if (!alertForm.value) return
  savingAlert.value = true
  try {
    await api.alerts.saveConfig(alertForm.value)
    alertForm.value = await api.alerts.getConfig()
    message.success('告警设置已保存')
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    savingAlert.value = false
  }
}

const testing = ref(false)
async function testWebhook() {
  if (!alertForm.value) return
  testing.value = true
  try {
    await api.alerts.testWebhook(alertForm.value)
    message.success('测试消息已发送')
  } catch (e) {
    message.error(errMsg(e), { duration: 6000 })
  } finally {
    testing.value = false
  }
}

// Backup and restore.
const backingUp = ref(false)
async function backup() {
  backingUp.value = true
  try {
    const path = await api.backup.create()
    if (path) message.success(`已备份到 ${path}`, { duration: 6000 })
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    backingUp.value = false
  }
}

const restoreFile = ref<BackupFile | null>(null)
const showRestore = ref(false)
const picking = ref(false)
const restoring = ref(false)
const restarting = ref(false)
async function pickRestore() {
  picking.value = true
  try {
    const f = await api.backup.inspectRestore()
    if (f) {
      restoreFile.value = f
      showRestore.value = true
    }
  } catch (e) {
    message.error(errMsg(e), { duration: 6000 })
  } finally {
    picking.value = false
  }
}
async function doRestore() {
  if (!restoreFile.value) return
  restoring.value = true
  try {
    const r = await api.backup.restore(restoreFile.value.token)
    showRestore.value = false
    if (r.restarting) restarting.value = true
    else message.info('恢复将在下次启动时完成，请关闭并重新打开程序', { duration: 0, closable: true })
  } catch (e) {
    message.error(errMsg(e), { duration: 6000 })
  } finally {
    restoring.value = false
  }
}

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
      </div>
    </div>
    <div class="grid">
      <div class="main-col">
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
                  <n-input v-model:value="updForm.source" clearable />
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
        <div v-if="alertForm" class="card section">
          <div class="card-title">告警</div>
          <n-form label-placement="left" label-width="150" class="form">
            <n-form-item label="告警类型">
              <div class="field">
                <div class="checks">
                  <n-checkbox v-model:checked="alertForm.offline">BMC 离线</n-checkbox>
                  <n-checkbox v-model:checked="alertForm.health">健康状态警告 / 严重</n-checkbox>
                  <n-checkbox v-model:checked="alertForm.powerLoss">意外断电</n-checkbox>
                </div>
                <div class="hint">恢复正常时另有一条恢复告警。无人登录时只检测 BMC 是否在线。</div>
              </div>
            </n-form-item>
            <n-form-item label="告警保留条数">
              <n-input-number v-model:value="alertForm.historyMax" :min="100" :max="100000" :step="100" style="width: 180px">
                <template #suffix>条</template>
              </n-input-number>
              <span class="tip">超出后自动删除最早的告警</span>
            </n-form-item>
            <n-form-item label="Webhook 地址">
              <div class="field">
                <n-input v-model:value="alertForm.webhookUrl" placeholder="留空则不推送" clearable />
                <div class="hint">每条新告警以 JSON 格式 POST 到该地址，失败时重试 2 次。</div>
              </div>
            </n-form-item>
            <n-form-item label="签名密钥">
              <div class="field">
                <n-input v-model:value="alertForm.webhookSecret" type="password" show-password-on="click" placeholder="可选" clearable />
                <div class="hint">填写后，请求头 X-Mdbmc-Signature 带有 HMAC-SHA256 签名，接收方可据此验证来源。</div>
              </div>
            </n-form-item>
            <n-form-item v-if="alertForm.lastWebhook" label="最近一次推送">
              <span class="secondary">{{ alertForm.lastWebhook }}</span>
            </n-form-item>
          </n-form>
          <div class="btns">
            <n-button type="primary" :loading="savingAlert" @click="saveAlert">保存告警设置</n-button>
            <n-button :disabled="!alertForm.webhookUrl.trim()" :loading="testing" @click="testWebhook">发送测试消息</n-button>
          </div>
        </div>
      </div>
      <div class="side">
        <div class="card section">
          <div class="about-head">
            <AppLogo :size="44" plate="dark" />
            <div>
              <div class="about-name">服务器资源管理系统</div>
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
          </n-descriptions>
        </div>
        <div v-if="canEdit" class="card section">
          <div class="card-title">数据备份</div>
          <div class="backup-text">备份包含设备、凭据、账号、告警和审计记录，其中的密码仍保持加密。</div>
          <div class="about-actions">
            <n-button :loading="backingUp" @click="backup">
              <template #icon><n-icon :component="SaveOutline" /></template>
              备份数据库
            </n-button>
            <n-button :loading="picking" @click="pickRestore">
              <template #icon><n-icon :component="RefreshCircleOutline" /></template>
              从备份恢复
            </n-button>
          </div>
        </div>
        <n-alert type="info" :bordered="false" title="数据迁移">
          数据目录中的 mdbmc.db 包含全部设备与加密后的 BMC 密码。复制到其他电脑后，用原有账号密码登录即可解密。
        </n-alert>
      </div>
    </div>

    <n-modal
      v-model:show="showRestore"
      preset="dialog"
      type="warning"
      title="从备份恢复"
      positive-text="恢复并重启"
      negative-text="取消"
      :loading="restoring"
      :mask-closable="!restoring"
      @positive-click="doRestore().then(() => false)"
    >
      <n-descriptions v-if="restoreFile" :column="1" label-placement="left" size="small" class="restore-desc">
        <n-descriptions-item label="文件">{{ restoreFile.file }}</n-descriptions-item>
        <n-descriptions-item label="大小">{{ formatBytes(restoreFile.size) }}</n-descriptions-item>
        <n-descriptions-item label="修改时间">{{ formatTime(restoreFile.modTime) }}</n-descriptions-item>
        <n-descriptions-item label="内容">{{ restoreFile.users }} 个账号，{{ restoreFile.devices }} 台设备</n-descriptions-item>
      </n-descriptions>
      <div class="restore-warn">
        当前的全部数据将替换为备份中的内容，账号和登录密码也会恢复为备份时的状态。当前数据库会另存一份副本。
      </div>
    </n-modal>

    <div v-if="restarting" class="restart-mask">
      <n-spin :size="28" />
      <div>正在重启以完成恢复…</div>
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
.main-col {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}
.checks {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 20px;
  min-height: 34px;
  align-items: center;
}
.btns {
  display: flex;
  gap: 8px;
  margin-left: 150px;
}
.backup-text {
  margin-top: 8px;
  font-size: 13px;
  line-height: 20px;
  color: var(--text-2);
}
.restore-desc {
  margin: 12px 0;
}
.restore-warn {
  padding: 10px 12px;
  border-radius: 6px;
  background: color-mix(in srgb, var(--status-warning) 12%, transparent);
  font-size: 13px;
  line-height: 20px;
}
.restart-mask {
  position: fixed;
  inset: 0;
  z-index: 3000;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 14px;
  background: color-mix(in srgb, var(--bg) 88%, transparent);
  backdrop-filter: blur(2px);
  font-size: 15px;
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
