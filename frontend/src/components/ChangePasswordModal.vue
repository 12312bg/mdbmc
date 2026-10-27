<script setup lang="ts">
import { ref, watch } from 'vue'
import { NModal, NForm, NFormItem, NInput, NButton, NAlert, useMessage, type FormInst, type FormRules } from 'naive-ui'
import { errMsg } from '@/api'
import { useSession } from '@/stores/session'

const props = defineProps<{ show: boolean; forced?: boolean }>()
const emit = defineEmits<{ 'update:show': [boolean] }>()

const session = useSession()
const message = useMessage()
const formRef = ref<FormInst | null>(null)
const form = ref({ old: '', next: '', confirm: '' })
const saving = ref(false)

watch(() => props.show, (s) => {
  if (s) form.value = { old: '', next: '', confirm: '' }
})

const rules: FormRules = {
  old: { required: true, message: '请输入当前密码', trigger: 'blur' },
  next: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 8, message: '密码长度至少 8 位', trigger: 'blur' },
  ],
  confirm: {
    trigger: ['blur', 'input'],
    validator: (_r, v: string) => (v === form.value.next ? true : new Error('两次输入的密码不一致')),
  },
}

async function submit() {
  await formRef.value?.validate()
  saving.value = true
  try {
    await session.changePassword(form.value.old, form.value.next)
    message.success('密码已修改')
    emit('update:show', false)
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    :title="forced ? '请设置新密码' : '修改密码'"
    style="width: 440px"
    :closable="!forced"
    :mask-closable="!forced"
    :close-on-esc="!forced"
    @update:show="(v: boolean) => emit('update:show', v)"
  >
    <n-alert v-if="forced" type="info" :bordered="false" style="margin-bottom: 16px">
      管理员为你设置了初始密码，首次登录需要改成只有你自己知道的密码。
    </n-alert>
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
      <n-form-item label="当前密码" path="old">
        <n-input v-model:value="form.old" type="password" show-password-on="click" />
      </n-form-item>
      <n-form-item label="新密码" path="next">
        <n-input v-model:value="form.next" type="password" show-password-on="click" placeholder="至少 8 位" />
      </n-form-item>
      <n-form-item label="确认新密码" path="confirm">
        <n-input v-model:value="form.confirm" type="password" show-password-on="click" @keyup.enter="submit" />
      </n-form-item>
    </n-form>
    <template #footer>
      <div style="display: flex; justify-content: flex-end; gap: 8px">
        <n-button v-if="!forced" @click="emit('update:show', false)">取消</n-button>
        <n-button v-else @click="session.logout()">退出登录</n-button>
        <n-button type="primary" :loading="saving" @click="submit">保存</n-button>
      </div>
    </template>
  </n-modal>
</template>
