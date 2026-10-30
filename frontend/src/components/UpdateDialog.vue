<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NModal, NButton, NIcon, NProgress, NSpin, useMessage } from 'naive-ui'
import {
  RocketOutline, ArrowForward, CloseOutline, TimeOutline, CubeOutline, ShieldCheckmarkOutline, CloudDownloadOutline,
  CheckmarkCircle, AlertCircleOutline, InformationCircleOutline,
} from '@vicons/ionicons5'
import MarkdownView from './MarkdownView.vue'
import { api, errMsg } from '@/api'
import { useSession } from '@/stores/session'
import { useUpdate } from '@/stores/update'
import { formatBytes, formatEta, formatTime } from '@/utils/format'

const upd = useUpdate()
const session = useSession()
const message = useMessage()

const st = computed(() => upd.status)
const latest = computed(() => upd.latest)
const state = computed(() => upd.state)
const canManage = computed(() => session.can('system.update'))
const canInstall = computed(() => !!st.value?.canInstall && (latest.value?.size ?? 0) > 0)
const installing = computed(() => state.value === 'installing')

const busy = ref(false)
// Installing closes the program, so it is confirmed inside the dialog first.
const confirming = ref(false)
watch(() => upd.dialog, (open) => {
  if (open) confirming.value = false
})
watch(state, (s) => {
  if (s !== 'ready') confirming.value = false
})

const modeText = computed(() =>
  st.value?.mode === 'portable' ? '免安装版：直接替换程序文件' : '安装版：运行新版安装程序（需要管理员授权）',
)
const published = computed(() => (latest.value?.publishedAt ? formatTime(latest.value.publishedAt).slice(0, 10) : ''))
const eta = computed(() => {
  const s = st.value
  return s && s.speed > 0 ? formatEta((s.total - s.downloaded) / s.speed) : ''
})

async function run(fn: () => Promise<unknown>) {
  busy.value = true
  try {
    await fn()
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    busy.value = false
  }
}
const download = () => run(() => api.update.download())
const cancelDownload = () => run(() => api.update.cancelDownload())
const install = () => run(() => api.update.install())
</script>

<template>
  <n-modal v-model:show="upd.dialog" :mask-closable="!installing" :close-on-esc="!installing" transform-origin="center">
    <div v-if="latest && st" class="upd" role="dialog" aria-label="软件更新">
      <header class="hero">
        <button v-if="!installing" class="close" aria-label="关闭" @click="upd.dialog = false">
          <n-icon :component="CloseOutline" :size="18" />
        </button>
        <div class="hero-icon"><n-icon :component="RocketOutline" :size="26" /></div>
        <div class="hero-text">
          <div class="hero-title">发现新版本</div>
          <div class="versions">
            <span class="ver ver-old">v{{ st.currentVersion }}</span>
            <n-icon :component="ArrowForward" :size="14" class="ver-arrow" />
            <span class="ver ver-new">v{{ latest.version }}</span>
          </div>
        </div>
      </header>

      <div class="meta">
        <span v-if="published"><n-icon :component="TimeOutline" />发布于 {{ published }}</span>
        <span v-if="latest.size"><n-icon :component="CubeOutline" />更新包 {{ formatBytes(latest.size) }}</span>
        <span><n-icon :component="ShieldCheckmarkOutline" />签名校验</span>
      </div>

      <div class="notes-title">更新内容</div>
      <div class="notes">
        <MarkdownView :source="latest.notes || '本次更新没有附带说明。'" />
      </div>

      <footer class="foot">
        <div v-if="st.error && !installing" class="note note-error">
          <n-icon :component="AlertCircleOutline" />
          <span>{{ st.error }}</span>
        </div>

        <!-- Not allowed to install -->
        <template v-if="!canManage">
          <div class="note">
            <n-icon :component="InformationCircleOutline" />
            <span>新版本需要由管理员下载安装。</span>
          </div>
          <div class="actions">
            <n-button @click="upd.dialog = false">知道了</n-button>
          </div>
        </template>

        <!-- No automatic install on this system or for this release -->
        <template v-else-if="!canInstall">
          <div class="note">
            <n-icon :component="InformationCircleOutline" />
            <span>当前运行环境不支持在线安装。</span>
          </div>
          <div class="actions">
            <n-button @click="upd.dialog = false">知道了</n-button>
          </div>
        </template>

        <template v-else-if="state === 'downloading'">
          <div class="progress">
            <div class="progress-head">
              <span class="progress-pct">{{ upd.progress.toFixed(0) }}%</span>
              <span class="muted">
                {{ formatBytes(st.downloaded) }} / {{ formatBytes(st.total) }}
                <template v-if="st.speed > 0"> · {{ formatBytes(st.speed) }}/s</template>
                <template v-if="eta"> · 剩余{{ eta }}</template>
              </span>
            </div>
            <n-progress type="line" :percentage="upd.progress" :show-indicator="false" :height="8" processing />
          </div>
          <div class="actions">
            <n-button :disabled="busy" @click="cancelDownload">取消下载</n-button>
            <n-button type="primary" secondary @click="upd.dialog = false">后台下载</n-button>
          </div>
        </template>

        <template v-else-if="state === 'ready' && !confirming">
          <div class="note note-good">
            <n-icon :component="CheckmarkCircle" />
            <span>更新包已下载，并通过签名与 SHA-256 校验。</span>
          </div>
          <div class="actions">
            <n-button @click="upd.dialog = false">稍后安装</n-button>
            <n-button type="primary" @click="confirming = true">立即安装</n-button>
          </div>
        </template>

        <template v-else-if="state === 'ready' && confirming">
          <div class="confirm">
            <div class="confirm-title">安装前请确认</div>
            <ul>
              <li>软件和所有 Web / KVM 控制台窗口会关闭，正在进行的远程操作将中断。</li>
              <li v-if="st.mode === 'installer'">安装程序需要 Windows 管理员授权，请在弹出的窗口中点击“是”。</li>
              <li>更新完成后软件会自动重新打开，需要重新登录。</li>
            </ul>
          </div>
          <div class="actions">
            <n-button :disabled="busy" @click="confirming = false">返回</n-button>
            <n-button type="primary" :loading="busy" @click="install">安装并重启</n-button>
          </div>
        </template>

        <template v-else-if="installing">
          <div class="installing">
            <n-spin :size="18" />
            <span>正在启动更新程序，软件即将关闭，更新完成后会自动重新打开…</span>
          </div>
        </template>

        <!-- available, or re-checking -->
        <template v-else>
          <div class="mode muted">{{ modeText }}</div>
          <div class="actions">
            <n-button @click="upd.dialog = false">稍后</n-button>
            <n-button type="primary" :loading="busy" :disabled="state === 'checking'" @click="download">
              <template #icon><n-icon :component="CloudDownloadOutline" /></template>
              下载更新
            </n-button>
          </div>
        </template>
      </footer>
    </div>
  </n-modal>
</template>

<style scoped>
.upd {
  width: 600px;
  max-width: calc(100vw - 32px);
  background: var(--surface);
  border-radius: 14px;
  overflow: hidden;
  box-shadow: 0 24px 64px rgba(15, 23, 42, 0.28);
}
.hero {
  position: relative;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 24px 24px 22px;
  color: #fff;
  background:
    radial-gradient(circle at 88% -20%, rgba(255, 255, 255, 0.28) 0, rgba(255, 255, 255, 0) 45%),
    radial-gradient(circle at 70% 140%, rgba(124, 108, 240, 0.55) 0, rgba(124, 108, 240, 0) 55%),
    linear-gradient(120deg, #1f6fd1 0%, #3b82f6 55%, #5b6cf0 100%);
}
:root[data-theme='dark'] .hero {
  background:
    radial-gradient(circle at 88% -20%, rgba(255, 255, 255, 0.16) 0, rgba(255, 255, 255, 0) 45%),
    radial-gradient(circle at 70% 140%, rgba(124, 108, 240, 0.45) 0, rgba(124, 108, 240, 0) 55%),
    linear-gradient(120deg, #17508f 0%, #2563c4 55%, #4450b8 100%);
}
.close {
  position: absolute;
  top: 12px;
  right: 12px;
  display: inline-flex;
  padding: 5px;
  border: 0;
  border-radius: 8px;
  color: rgba(255, 255, 255, 0.85);
  background: transparent;
  cursor: pointer;
}
.close:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.16);
}
.hero-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 52px;
  height: 52px;
  flex: none;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.18);
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.25);
}
.hero-title {
  font-size: 19px;
  font-weight: 600;
  line-height: 26px;
}
.versions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 6px;
}
.ver {
  padding: 1px 9px;
  border-radius: 999px;
  font-size: 12.5px;
  font-weight: 600;
  line-height: 20px;
  font-variant-numeric: tabular-nums;
}
.ver-old {
  background: rgba(255, 255, 255, 0.14);
  color: rgba(255, 255, 255, 0.82);
}
.ver-new {
  background: #fff;
  color: #1f6fd1;
}
.ver-arrow {
  opacity: 0.8;
}
.meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 18px;
  padding: 12px 24px;
  border-bottom: 1px solid var(--border);
  font-size: 12.5px;
  color: var(--text-3);
}
.meta span {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.notes-title {
  padding: 16px 24px 0;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.06em;
  color: var(--text-3);
}
.notes {
  max-height: 300px;
  overflow-y: auto;
  padding: 8px 24px 18px;
}
.foot {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px 24px 20px;
  border-top: 1px solid var(--border);
  background: var(--surface-2);
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
.mode {
  font-size: 12.5px;
  margin-bottom: -4px;
}
.note {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 13px;
  line-height: 20px;
  color: var(--text-2);
}
.note :deep(.n-icon) {
  flex: none;
  margin-top: 2px;
  font-size: 16px;
  color: var(--accent);
}
.note-good :deep(.n-icon) {
  color: var(--status-good);
}
.note-error {
  color: var(--status-critical);
}
.note-error :deep(.n-icon) {
  color: var(--status-critical);
}
.progress-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: 8px;
  font-size: 12.5px;
  font-variant-numeric: tabular-nums;
}
.progress-pct {
  font-size: 18px;
  font-weight: 600;
  color: var(--text-1);
}
.confirm {
  padding: 12px 14px;
  border-radius: 8px;
  border: 1px solid color-mix(in srgb, var(--status-warning) 45%, transparent);
  background: color-mix(in srgb, var(--status-warning) 10%, var(--surface));
  font-size: 13px;
  color: var(--text-2);
}
.confirm-title {
  font-weight: 600;
  color: var(--text-1);
  margin-bottom: 4px;
}
.confirm ul {
  margin: 0;
  padding-left: 18px;
  line-height: 1.75;
}
.installing {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 34px;
  font-size: 13px;
  color: var(--text-2);
}
</style>
