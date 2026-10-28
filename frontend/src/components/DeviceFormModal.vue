<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  NModal, NForm, NFormItem, NInput, NInputNumber, NSelect, NButton, NRadioGroup, NRadioButton, NGrid, NFormItemGi,
  NCollapseTransition, useMessage, type FormInst, type FormRules,
} from 'naive-ui'
import { api, errMsg, type Device, type DeviceInput } from '@/api'
import { useDevices } from '@/stores/devices'
import { protocolLabel } from '@/utils/format'

const props = defineProps<{ show: boolean; device?: Device | null; defaultGroup?: number }>()
const emit = defineEmits<{ 'update:show': [boolean]; saved: [Device] }>()

const store = useDevices()
const message = useMessage()
const formRef = ref<FormInst | null>(null)
const saving = ref(false)
const credMode = ref<'profile' | 'custom'>('profile')
const showAdvanced = ref(false)

const blank = (): DeviceInput => ({
  id: 0, name: '', host: '', ipmiPort: 623, httpsPort: 443, protocol: 'auto', groupId: props.defaultGroup ?? 0,
  credentialId: store.credentials[0]?.id ?? 0, username: '', password: '', note: '',
})
const form = ref<DeviceInput>(blank())
const editing = computed(() => !!props.device)

watch(() => props.show, (s) => {
  if (!s) return
  const d = props.device
  form.value = d
    ? { id: d.id, name: d.name, host: d.host, ipmiPort: d.ipmiPort, httpsPort: d.httpsPort, protocol: d.protocol, groupId: d.groupId, credentialId: d.credentialId, username: d.username, password: '', note: d.note }
    : blank()
  credMode.value = form.value.credentialId ? 'profile' : store.credentials.length ? 'profile' : 'custom'
  if (d && !d.credentialId) credMode.value = 'custom'
  showAdvanced.value = !!d && (d.ipmiPort !== 623 || d.httpsPort !== 443 || d.protocol !== 'auto')
})

const protocolOptions = Object.entries(protocolLabel).map(([value, label]) => ({ label, value }))

const rules: FormRules = {
  host: { required: true, message: '请输入 BMC 的 IP 地址或主机名', trigger: 'blur' },
  credentialId: {
    trigger: 'change', type: 'number',
    validator: (_r, v: number) => (credMode.value === 'custom' || v ? true : new Error('请选择凭据')),
  },
  username: {
    trigger: 'blur',
    validator: (_r, v: string) => (credMode.value === 'profile' || v ? true : new Error('请输入 BMC 用户名')),
  },
}

async function submit() {
  await formRef.value?.validate()
  const input = { ...form.value }
  if (credMode.value === 'profile') {
    input.username = ''
    input.password = ''
  } else {
    input.credentialId = 0
  }
  saving.value = true
  try {
    const d = editing.value ? await api.devices.update(input) : await api.devices.create(input)
    message.success(editing.value ? '设备已更新，正在重新识别' : '设备已添加，正在识别型号与协议')
    emit('saved', d)
    emit('update:show', false)
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <n-modal :show="show" preset="card" :title="editing ? '编辑设备' : '添加设备'" style="width: 560px" @update:show="(v: boolean) => emit('update:show', v)">
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="top" require-mark-placement="left">
      <n-grid :cols="2" :x-gap="16">
        <n-form-item-gi label="BMC 地址" path="host">
          <n-input v-model:value="form.host" placeholder="例如 10.10.1.11" />
        </n-form-item-gi>
        <n-form-item-gi label="显示名称" path="name">
          <n-input v-model:value="form.name" placeholder="留空则使用地址" />
        </n-form-item-gi>
      </n-grid>
      <n-form-item label="分组" path="groupId">
        <n-select v-model:value="form.groupId" :options="store.groupOptions" />
      </n-form-item>
      <n-form-item label="登录凭据">
        <n-radio-group v-model:value="credMode" size="small">
          <n-radio-button value="profile" :disabled="!store.credentials.length">使用凭据模板</n-radio-button>
          <n-radio-button value="custom">单独设置</n-radio-button>
        </n-radio-group>
      </n-form-item>
      <n-form-item v-if="credMode === 'profile'" path="credentialId" :show-label="false">
        <n-select v-model:value="form.credentialId" :options="store.credentialOptions" placeholder="选择凭据" />
      </n-form-item>
      <n-grid v-else :cols="2" :x-gap="16">
        <n-form-item-gi label="BMC 用户名" path="username">
          <n-input v-model:value="form.username" placeholder="如 ADMIN / root" />
        </n-form-item-gi>
        <n-form-item-gi label="BMC 密码" path="password">
          <n-input
            v-model:value="form.password"
            type="password"
            show-password-on="click"
            :placeholder="editing && device?.hasPassword ? '留空则不修改' : ''"
          />
        </n-form-item-gi>
      </n-grid>
      <n-button text type="primary" size="small" style="margin: -4px 0 12px" @click="showAdvanced = !showAdvanced">
        {{ showAdvanced ? '收起高级选项' : '高级选项（端口、协议）' }}
      </n-button>
      <n-collapse-transition :show="showAdvanced">
        <n-grid :cols="3" :x-gap="16">
          <n-form-item-gi label="IPMI 端口 (UDP)">
            <n-input-number v-model:value="form.ipmiPort" :min="1" :max="65535" :show-button="false" />
          </n-form-item-gi>
          <n-form-item-gi label="HTTPS 端口">
            <n-input-number v-model:value="form.httpsPort" :min="1" :max="65535" :show-button="false" />
          </n-form-item-gi>
          <n-form-item-gi label="管理协议">
            <n-select v-model:value="form.protocol" :options="protocolOptions" />
          </n-form-item-gi>
        </n-grid>
      </n-collapse-transition>
      <n-form-item label="备注" path="note">
        <n-input v-model:value="form.note" type="textarea" :autosize="{ minRows: 1, maxRows: 4 }" placeholder="机柜位置、用途等" />
      </n-form-item>
    </n-form>
    <template #footer>
      <div style="display: flex; justify-content: flex-end; gap: 8px">
        <n-button @click="emit('update:show', false)">取消</n-button>
        <n-button type="primary" :loading="saving" @click="submit">{{ editing ? '保存' : '添加' }}</n-button>
      </div>
    </template>
  </n-modal>
</template>
