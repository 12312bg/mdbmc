<script setup lang="ts">
// Batch import from an Excel workbook: get the template, pick the filled-in
// file, check the preview, then import the rows marked OK.
import { computed, h, ref, watch } from 'vue'
import { NModal, NButton, NIcon, NDataTable, NTag, NRadioGroup, NRadioButton, NResult, NSpin, useMessage, type DataTableColumns } from 'naive-ui'
import { DownloadOutline, FolderOpenOutline, DocumentTextOutline } from '@vicons/ionicons5'
import { api, errMsg, type ImportPreview, type ImportResult, type ImportRow } from '@/api'
import { protocolLabel } from '@/utils/format'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ 'update:show': [boolean]; imported: [] }>()
const message = useMessage()

const preview = ref<ImportPreview | null>(null)
const result = ref<ImportResult | null>(null)
const busy = ref<'' | 'template' | 'pick' | 'import'>('')
const filter = ref<'' | ImportRow['status']>('')

watch(() => props.show, (v) => {
  if (v) {
    preview.value = null
    result.value = null
    filter.value = ''
  }
})

async function saveTemplate() {
  busy.value = 'template'
  try {
    const path = await api.devices.importTemplate()
    if (path) message.success(`模板已保存到 ${path}`)
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    busy.value = ''
  }
}

async function pick() {
  busy.value = 'pick'
  try {
    const p = await api.devices.previewImport()
    if (p) {
      preview.value = p
      filter.value = p.errors ? 'error' : ''
    }
  } catch (e) {
    message.error(errMsg(e), { duration: 6000 })
  } finally {
    busy.value = ''
  }
}

async function doImport() {
  if (!preview.value) return
  busy.value = 'import'
  try {
    result.value = await api.devices.importDevices(preview.value.token)
    emit('imported')
  } catch (e) {
    message.error(errMsg(e), { duration: 6000 })
  } finally {
    busy.value = ''
  }
}

const rows = computed(() => (preview.value?.rows ?? []).filter((r) => !filter.value || r.status === filter.value))

const statusTag = { ok: { type: 'success', label: '可导入' }, skip: { type: 'default', label: '跳过' }, error: { type: 'error', label: '错误' } } as const
const dash = () => h('span', { class: 'muted' }, '—')
const columns: DataTableColumns<ImportRow> = [
  { title: '行', key: 'row', width: 56, render: (r) => h('span', { class: 'mono muted' }, r.row) },
  { title: '名称', key: 'name', minWidth: 120, ellipsis: { tooltip: true }, render: (r) => r.name || dash() },
  {
    title: '地址', key: 'host', minWidth: 150,
    render: (r) => r.host
      ? h('span', { class: 'mono' }, [r.host, r.ipmiPort && r.ipmiPort !== 623 ? h('span', { class: 'muted' }, ` · ${r.ipmiPort}`) : null])
      : dash(),
  },
  { title: '协议', key: 'protocol', width: 86, render: (r) => protocolLabel[r.protocol] ?? r.protocol },
  { title: '分组', key: 'group', minWidth: 110, ellipsis: { tooltip: true }, render: (r) => r.group || h('span', { class: 'muted' }, '未分组') },
  {
    title: '凭据', key: 'credential', minWidth: 140, ellipsis: { tooltip: true },
    render: (r) => r.credential
      ? h('span', null, [h('span', { class: 'muted' }, '模板 '), r.credential])
      : r.username
        ? h('span', null, [r.username, h('span', { class: 'muted' }, r.hasPassword ? ' · 已填密码' : ' · 无密码')])
        : dash(),
  },
  {
    title: '结果', key: 'status', minWidth: 260,
    render: (r) => h('div', { class: 'status' }, [
      h(NTag, { size: 'small', bordered: false, type: statusTag[r.status].type }, { default: () => statusTag[r.status].label }),
      r.message ? h('span', { class: ['status-msg', r.status === 'error' && 'is-error'] }, r.message) : null,
    ]),
  },
]

function close() {
  emit('update:show', false)
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    :title="result ? '导入完成' : '批量导入设备'"
    :style="{ width: preview && !result ? '980px' : '560px' }"
    :mask-closable="!busy"
    :closable="busy !== 'import'"
    @update:show="(v: boolean) => emit('update:show', v)"
  >
    <!-- Result -->
    <template v-if="result">
      <n-result
        :status="result.failed.length ? 'warning' : 'success'"
        :title="`已导入 ${result.added} 台设备`"
        :description="result.added ? '正在后台读取型号和状态，稍后可在设备列表中查看。' : ''"
        size="small"
      />
      <div v-if="result.failed.length" class="failed">
        <div class="failed-title">{{ result.failed.length }} 行未能导入</div>
        <div v-for="f in result.failed" :key="f" class="failed-item">{{ f }}</div>
      </div>
    </template>

    <!-- Preview -->
    <template v-else-if="preview">
      <div class="pv-head">
        <n-icon :component="DocumentTextOutline" :size="18" class="muted" />
        <span class="pv-file">{{ preview.file }}</span>
        <span class="pv-total muted">共 {{ preview.rows.length }} 行</span>
      </div>
      <div class="pv-bar">
        <n-radio-group v-model:value="filter" size="small">
          <n-radio-button value="">全部</n-radio-button>
          <n-radio-button value="ok">可导入 {{ preview.ok }}</n-radio-button>
          <n-radio-button value="skip" :disabled="!preview.skip">已存在 {{ preview.skip }}</n-radio-button>
          <n-radio-button value="error" :disabled="!preview.errors">有错误 {{ preview.errors }}</n-radio-button>
        </n-radio-group>
        <span v-if="preview.newGroups.length" class="pv-groups">
          将新建分组：<b>{{ preview.newGroups.join('、') }}</b>
        </span>
      </div>
      <n-data-table
        :columns="columns"
        :data="rows"
        :row-key="(r: ImportRow) => r.row"
        :bordered="false"
        size="small"
        :max-height="420"
        :scroll-x="900"
        virtual-scroll
      />
      <div v-if="preview.errors" class="pv-note">有错误的行不会导入；修改文件后可以重新选择。</div>
    </template>

    <!-- Start -->
    <template v-else>
      <n-spin :show="busy === 'pick'">
        <ol class="steps">
          <li>
            <div class="step-text">
              <div class="step-title">下载导入模板</div>
              <div class="step-sub">模板中的“填写说明”工作表介绍了每一列的写法</div>
            </div>
            <n-button :loading="busy === 'template'" @click="saveTemplate">
              <template #icon><n-icon :component="DownloadOutline" /></template>
              下载模板
            </n-button>
          </li>
          <li>
            <div class="step-text">
              <div class="step-title">每行填写一台设备</div>
              <div class="step-sub">地址必填；可填写 BMC 用户名和密码，或填写已有凭据模板的名称</div>
            </div>
          </li>
          <li>
            <div class="step-text">
              <div class="step-title">选择填写好的文件</div>
              <div class="step-sub">导入前会先检查每一行并列出结果，地址和端口已存在的设备会跳过</div>
            </div>
            <n-button type="primary" :disabled="!!busy" @click="pick">
              <template #icon><n-icon :component="FolderOpenOutline" /></template>
              选择文件
            </n-button>
          </li>
        </ol>
      </n-spin>
    </template>

    <template #footer>
      <div class="foot">
        <template v-if="result">
          <n-button type="primary" @click="close">完成</n-button>
        </template>
        <template v-else-if="preview">
          <n-button :disabled="!!busy" @click="pick">重新选择文件</n-button>
          <span class="spacer" />
          <n-button :disabled="!!busy" @click="close">取消</n-button>
          <n-button type="primary" :disabled="!preview.ok" :loading="busy === 'import'" @click="doImport">
            导入 {{ preview.ok }} 台设备
          </n-button>
        </template>
        <template v-else>
          <n-button @click="close">取消</n-button>
        </template>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.steps {
  list-style: none;
  counter-reset: step;
  margin: 4px 0 0;
  padding: 0;
}
.steps li {
  counter-increment: step;
  position: relative;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 0 0 22px 42px;
  min-height: 44px;
}
.steps li::before {
  content: counter(step);
  position: absolute;
  left: 0;
  top: 0;
  width: 26px;
  height: 26px;
  border-radius: 50%;
  background: var(--accent-soft);
  color: var(--accent);
  font-weight: 600;
  font-size: 13px;
  line-height: 26px;
  text-align: center;
}
.steps li:not(:last-child)::after {
  content: '';
  position: absolute;
  left: 12.5px;
  top: 30px;
  bottom: 4px;
  width: 1px;
  background: var(--border);
}
.steps li:last-child {
  padding-bottom: 4px;
}
.step-text {
  flex: 1;
  min-width: 0;
  padding-top: 3px;
}
.step-title {
  font-weight: 600;
}
.step-sub {
  margin-top: 2px;
  font-size: 12.5px;
  color: var(--text-3);
  line-height: 18px;
}
.pv-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.pv-file {
  font-weight: 600;
}
.pv-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 10px;
}
.pv-groups {
  font-size: 13px;
  color: var(--text-2);
}
.pv-groups b {
  font-weight: 600;
  color: var(--text-1);
}
.pv-note {
  margin-top: 10px;
  font-size: 12.5px;
  color: var(--text-3);
}
.foot {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
.spacer {
  flex: 1;
}
.failed {
  margin-top: 8px;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  max-height: 200px;
  overflow-y: auto;
}
.failed-title {
  font-weight: 600;
  margin-bottom: 4px;
}
.failed-item {
  font-size: 13px;
  color: var(--text-2);
  line-height: 20px;
}
:deep(.status) {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  min-width: 0;
}
:deep(.status .n-tag) {
  flex: none;
}
:deep(.status-msg) {
  font-size: 12.5px;
  line-height: 22px;
  color: var(--text-3);
}
:deep(.status-msg.is-error) {
  color: var(--status-critical);
}
</style>
