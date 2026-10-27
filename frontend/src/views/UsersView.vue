<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import {
  NButton, NIcon, NDataTable, NModal, NForm, NFormItem, NInput, NSelect, NSwitch, NTag, useDialog, useMessage,
  type DataTableColumns, type FormInst, type FormRules,
} from 'naive-ui'
import { AddOutline, CheckmarkOutline, RemoveOutline } from '@vicons/ionicons5'
import { api, errMsg, type Permission, type RoleOption, type UserDTO, type UserInput } from '@/api'
import { useSession } from '@/stores/session'
import { formatTime, relativeTime } from '@/utils/format'

const session = useSession()
const dialog = useDialog()
const message = useMessage()

const users = ref<UserDTO[]>([])
const roles = ref<RoleOption[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    ;[users.value, roles.value] = await Promise.all([api.admin.listUsers(), api.admin.roles()])
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    loading.value = false
  }
}
onMounted(load)

const roleOptions = computed(() => roles.value.map((r) => ({ label: r.label, value: r.value })))

// ----- create / edit -----
const show = ref(false)
const saving = ref(false)
const formRef = ref<FormInst | null>(null)
const form = ref<UserInput>({ id: 0, username: '', displayName: '', role: 'operator', password: '', disabled: false })
const rules: FormRules = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { pattern: /^[A-Za-z0-9_.@-]{2,32}$/, message: '2-32 位字母、数字或 _ . @ -', trigger: 'blur' },
  ],
  password: { trigger: 'blur', validator: (_r, v: string) => (form.value.id || v.length >= 8 ? true : new Error('初始密码至少 8 位')) },
}
function open(u?: UserDTO) {
  form.value = u
    ? { id: u.id, username: u.username, displayName: u.displayName, role: u.role, password: '', disabled: u.disabled }
    : { id: 0, username: '', displayName: '', role: 'operator', password: '', disabled: false }
  show.value = true
}
async function save() {
  await formRef.value?.validate()
  saving.value = true
  try {
    if (form.value.id) await api.admin.updateUser(form.value)
    else await api.admin.createUser(form.value)
    message.success(form.value.id ? '已保存' : '用户已创建，首次登录需修改密码')
    show.value = false
    await load()
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    saving.value = false
  }
}

// ----- reset password -----
const reset = ref({ show: false, id: 0, name: '', password: '' })
async function doReset() {
  if (reset.value.password.length < 8) return message.warning('密码至少 8 位')
  try {
    await api.admin.resetPassword(reset.value.id, reset.value.password)
    message.success('密码已重置，用户下次登录需修改密码')
    reset.value.show = false
    await load()
  } catch (e) {
    message.error(errMsg(e))
  }
}

function remove(u: UserDTO) {
  dialog.warning({
    title: `删除用户“${u.username}”`,
    content: '删除后该账号无法再登录。操作记录会保留在审计日志中。',
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await api.admin.deleteUser(u.id)
        await load()
      } catch (e) {
        message.error(errMsg(e))
      }
    },
  })
}

const link = (label: string, onClick: () => void, type: 'primary' | 'error' = 'primary', disabled = false) =>
  h(NButton, { text: true, size: 'small', type, onClick, disabled, style: 'margin-right: 14px' }, { default: () => label })

const columns = computed<DataTableColumns<UserDTO>>(() => [
  {
    title: '用户', key: 'username', minWidth: 160,
    render: (u) => h('div', null, [
      h('div', { style: 'font-weight:500' }, [u.displayName || u.username, u.id === session.user?.id ? h(NTag, { size: 'tiny', bordered: false, style: 'margin-left:6px' }, { default: () => '当前' }) : null]),
      h('div', { class: 'muted mono' }, u.username),
    ]),
  },
  { title: '角色', key: 'role', width: 110, render: (u) => h(NTag, { size: 'small', bordered: false, type: u.role === 'admin' ? 'info' : 'default' }, { default: () => u.roleLabel }) },
  {
    title: '状态', key: 'disabled', width: 124,
    render: (u) => u.disabled ? h('span', { class: 'muted' }, '已停用') : u.mustChangePassword ? h('span', { class: 'secondary' }, '待修改初始密码') : '正常',
  },
  { title: '最近登录', key: 'lastLoginAt', width: 110, render: (u) => h('span', { class: 'muted', title: formatTime(u.lastLoginAt) }, relativeTime(u.lastLoginAt)) },
  {
    title: '操作', key: 'ops', width: 196,
    render: (u) => h('div', null, [
      link('编辑', () => open(u)),
      link('重置密码', () => (reset.value = { show: true, id: u.id, name: u.username, password: '' }), 'primary', u.id === session.user?.id),
      link('删除', () => remove(u), 'error', u.id === session.user?.id),
    ]),
  },
])

// ----- permission matrix -----
const permRows: { perm: Permission; label: string }[] = [
  { perm: 'device.view', label: '查看设备、传感器、事件日志、资产' },
  { perm: 'console.web', label: '打开 BMC Web 登录页' },
  { perm: 'console.autologin', label: 'Web 管理自动登录' },
  { perm: 'console.kvm', label: '远程控制台 KVM' },
  { perm: 'power.control', label: '电源控制、定位灯' },
  { perm: 'sel.clear', label: '清空事件日志' },
  { perm: 'device.manage', label: '添加 / 扫描 / 编辑 / 删除设备、分组、凭据' },
  { perm: 'credential.reveal', label: '查看 BMC 密码明文' },
  { perm: 'user.manage', label: '用户管理' },
  { perm: 'settings.manage', label: '修改系统设置' },
  { perm: 'audit.view', label: '查看审计日志' },
]
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h1>用户与角色</h1>
        <p>本地账号登录本软件；三种角色权限固定。</p>
      </div>
      <n-button type="primary" @click="open()">
        <template #icon><n-icon :component="AddOutline" /></template>
        新建用户
      </n-button>
    </div>

    <div class="layout">
      <div class="card" style="padding: 4px 4px 8px">
        <n-data-table :columns="columns" :data="users" :loading="loading" :bordered="false" size="small" />
      </div>
      <div class="card matrix">
        <div class="card-title" style="margin-bottom: 12px">角色权限</div>
        <table>
          <thead>
            <tr><th>权限</th><th v-for="r in roles" :key="r.value">{{ r.label }}</th></tr>
          </thead>
          <tbody>
            <tr v-for="p in permRows" :key="p.perm">
              <td>{{ p.label }}</td>
              <td v-for="r in roles" :key="r.value" class="c">
                <n-icon v-if="r.permissions.includes(p.perm)" :component="CheckmarkOutline" color="var(--status-good)" :size="16" aria-label="允许" />
                <n-icon v-else :component="RemoveOutline" class="muted" :size="14" aria-label="不允许" />
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <n-modal v-model:show="show" preset="card" :title="form.id ? '编辑用户' : '新建用户'" style="width: 460px">
      <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
        <n-form-item label="用户名" path="username"><n-input v-model:value="form.username" :disabled="!!form.id" /></n-form-item>
        <n-form-item label="显示名称" path="displayName"><n-input v-model:value="form.displayName" /></n-form-item>
        <n-form-item label="角色" path="role"><n-select v-model:value="form.role" :options="roleOptions" /></n-form-item>
        <n-form-item v-if="!form.id" label="初始密码" path="password">
          <n-input v-model:value="form.password" type="password" show-password-on="click" placeholder="至少 8 位，用户首次登录后需修改" />
        </n-form-item>
        <n-form-item v-else label="停用账号">
          <n-switch v-model:value="form.disabled" :disabled="form.id === session.user?.id" />
        </n-form-item>
      </n-form>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 8px">
          <n-button @click="show = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="save">保存</n-button>
        </div>
      </template>
    </n-modal>

    <n-modal v-model:show="reset.show" preset="dialog" :title="`重置“${reset.name}”的密码`" positive-text="重置" negative-text="取消" @positive-click="doReset">
      <n-input v-model:value="reset.password" type="password" show-password-on="click" placeholder="新密码（至少 8 位）" style="margin-top: 12px" />
    </n-modal>
  </div>
</template>

<style scoped>
.layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 380px;
  gap: 16px;
  align-items: start;
}
.matrix {
  padding: 16px 18px;
}
.matrix table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.matrix th {
  text-align: center;
  font-weight: 600;
  padding: 6px 4px;
  border-bottom: 1px solid var(--border);
}
.matrix th:first-child {
  text-align: left;
}
.matrix td {
  padding: 7px 4px;
  border-bottom: 1px solid var(--border);
  color: var(--text-2);
}
.matrix td.c {
  text-align: center;
  width: 64px;
}
@media (max-width: 1300px) {
  .layout {
    grid-template-columns: 1fr;
  }
}
</style>
