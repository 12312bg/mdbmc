<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import {
  NButton, NIcon, NDataTable, NModal, NForm, NFormItem, NInput, NAlert, useDialog, useMessage, type DataTableColumns,
  type FormInst, type FormRules,
} from 'naive-ui'
import { AddOutline, CreateOutline, EyeOutline, TrashOutline, KeyOutline } from '@vicons/ionicons5'
import { api, errMsg, type Credential, type CredentialInput } from '@/api'
import { useDevices } from '@/stores/devices'
import { useSession } from '@/stores/session'
import { relativeTime } from '@/utils/format'

const store = useDevices()
const session = useSession()
const dialog = useDialog()
const message = useMessage()

onMounted(() => store.load())

const show = ref(false)
const saving = ref(false)
const formRef = ref<FormInst | null>(null)
const form = ref<CredentialInput>({ id: 0, name: '', username: '', password: '', note: '' })
const rules: FormRules = {
  name: { required: true, message: '请输入名称', trigger: 'blur' },
  username: { required: true, message: '请输入 BMC 用户名', trigger: 'blur' },
  password: { trigger: 'blur', validator: (_r, v: string) => (form.value.id || v ? true : new Error('请输入 BMC 密码')) },
}

function open(c?: Credential) {
  form.value = c ? { id: c.id, name: c.name, username: c.username, password: '', note: c.note } : { id: 0, name: '', username: 'ADMIN', password: '', note: '' }
  show.value = true
}

async function save() {
  await formRef.value?.validate()
  saving.value = true
  try {
    if (form.value.id) await api.credentials.update(form.value)
    else await api.credentials.create(form.value)
    message.success('已保存')
    show.value = false
    await store.load()
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    saving.value = false
  }
}

async function reveal(c: Credential) {
  try {
    const pw = await api.credentials.reveal(c.id)
    dialog.info({
      title: `${c.name} · ${c.username}`,
      content: () => h('span', { class: 'mono', style: 'font-size:15px' }, pw),
      positiveText: '复制并关闭',
      negativeText: '关闭',
      onPositiveClick: () => navigator.clipboard?.writeText(pw),
    })
  } catch (e) {
    message.error(errMsg(e))
  }
}

function remove(c: Credential) {
  dialog.warning({
    title: `删除凭据“${c.name}”`,
    content: c.deviceCount ? `仍有 ${c.deviceCount} 台设备使用此凭据，请先为它们更换凭据。` : '确定删除？',
    positiveText: '删除',
    negativeText: '取消',
    positiveButtonProps: { disabled: c.deviceCount > 0 },
    onPositiveClick: async () => {
      try {
        await api.credentials.remove(c.id)
        await store.load()
      } catch (e) {
        message.error(errMsg(e))
      }
    },
  })
}

const btn = (icon: any, label: string, onClick: () => void, type?: 'error') =>
  h(NButton, { text: true, size: 'small', type: type ?? 'primary', onClick, style: 'margin-right: 14px' }, { icon: () => h(NIcon, { component: icon }), default: () => label })

const columns: DataTableColumns<Credential> = [
  { title: '名称', key: 'name', minWidth: 180, render: (c) => h('span', { style: 'font-weight:500' }, c.name) },
  { title: 'BMC 用户名', key: 'username', width: 180, render: (c) => h('span', { class: 'mono' }, c.username) },
  { title: '使用设备', key: 'deviceCount', width: 110, render: (c) => `${c.deviceCount} 台` },
  { title: '备注', key: 'note', minWidth: 200, ellipsis: { tooltip: true }, render: (c) => c.note || h('span', { class: 'muted' }, '—') },
  { title: '更新时间', key: 'updatedAt', width: 120, render: (c) => h('span', { class: 'muted' }, relativeTime(c.updatedAt)) },
  {
    title: '操作', key: 'ops', width: 240,
    render: (c) => h('div', null, [
      session.can('credential.reveal') ? btn(EyeOutline, '查看密码', () => reveal(c)) : null,
      session.can('device.manage') ? btn(CreateOutline, '编辑', () => open(c)) : null,
      session.can('device.manage') ? btn(TrashOutline, '删除', () => remove(c), 'error') : null,
    ]),
  },
]
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>凭据</h1>
        <p>可复用的 BMC 账号模板。修改模板密码后，所有使用该模板的设备立即生效。</p>
      </div>
      <n-button v-if="session.can('device.manage')" type="primary" @click="open()">
        <template #icon><n-icon :component="AddOutline" /></template>
        新建凭据
      </n-button>
    </div>
    <n-alert type="default" :bordered="false" class="note">
      <template #icon><n-icon :component="KeyOutline" /></template>
      密码使用 AES-256-GCM 加密保存在本机数据库中，密钥由各账号的登录密码保护。
    </n-alert>
    <div class="card" style="padding: 4px 4px 8px">
      <n-data-table :columns="columns" :data="store.credentials" :loading="store.loading" :bordered="false" size="small" />
    </div>

    <n-modal v-model:show="show" preset="card" :title="form.id ? '编辑凭据' : '新建凭据'" style="width: 480px">
      <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
        <n-form-item label="名称" path="name"><n-input v-model:value="form.name" maxlength="40" placeholder="如：Supermicro 默认" /></n-form-item>
        <n-form-item label="BMC 用户名" path="username"><n-input v-model:value="form.username" /></n-form-item>
        <n-form-item label="BMC 密码" path="password">
          <n-input v-model:value="form.password" type="password" show-password-on="click" :placeholder="form.id ? '留空则不修改' : ''" />
        </n-form-item>
        <n-form-item label="备注" path="note"><n-input v-model:value="form.note" type="textarea" :autosize="{ minRows: 2 }" /></n-form-item>
      </n-form>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 8px">
          <n-button @click="show = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="save">保存</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.note {
  margin-bottom: 16px;
}
</style>
