<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { NForm, NFormItem, NInput, NButton, NAlert, NCheckbox, useMessage, type FormInst, type FormRules } from 'naive-ui'
import AuthShell from '@/components/AuthShell.vue'
import { errMsg } from '@/api'
import { useSession } from '@/stores/session'

const session = useSession()
const router = useRouter()
const message = useMessage()

const formRef = ref<FormInst | null>(null)
const form = ref({ username: 'admin', displayName: '', password: '', confirm: '' })
const understood = ref(false)
const loading = ref(false)

const rules: FormRules = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { pattern: /^[A-Za-z0-9_.@-]{2,32}$/, message: '2-32 位字母、数字或 _ . @ -', trigger: 'blur' },
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 8, message: '密码长度至少 8 位', trigger: 'blur' },
  ],
  confirm: {
    trigger: ['blur', 'input'],
    validator: (_r, v: string) => (v === form.value.password ? true : new Error('两次输入的密码不一致')),
  },
}

async function submit() {
  await formRef.value?.validate()
  if (!understood.value) {
    message.warning('请先确认密码保管说明')
    return
  }
  loading.value = true
  try {
    await session.setup(form.value.username.trim(), form.value.displayName.trim(), form.value.password)
    message.success('初始化完成')
    await router.replace('/dashboard')
  } catch (e) {
    message.error(errMsg(e))
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <AuthShell>
    <h2 class="title">欢迎使用</h2>
    <p class="sub">首次使用，请创建管理员账号。</p>
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
      <n-form-item label="管理员用户名" path="username">
        <n-input v-model:value="form.username" />
      </n-form-item>
      <n-form-item label="显示名称" path="displayName">
        <n-input v-model:value="form.displayName" placeholder="可选" />
      </n-form-item>
      <n-form-item label="密码" path="password">
        <n-input v-model:value="form.password" type="password" show-password-on="click" placeholder="至少 8 位" />
      </n-form-item>
      <n-form-item label="确认密码" path="confirm">
        <n-input v-model:value="form.confirm" type="password" show-password-on="click" @keyup.enter="submit" />
      </n-form-item>
    </n-form>
    <n-alert type="warning" :bordered="false" title="请妥善保管密码" class="gap">
      BMC 密码用随机密钥加密，这个密钥再用每个账号的登录密码分别加密保存。普通用户忘记密码可由管理员重置；如果所有管理员都忘记密码，已保存的 BMC 密码将无法恢复。
    </n-alert>
    <n-checkbox v-model:checked="understood" class="gap">我已了解，并会妥善保管管理员密码</n-checkbox>
    <n-button type="primary" size="large" block :loading="loading" @click="submit">创建并登录</n-button>
  </AuthShell>
</template>

<style scoped>
.title {
  font-size: 24px;
  font-weight: 600;
  margin: 0 0 6px;
}
.sub {
  color: var(--text-3);
  margin: 0 0 20px;
}
.gap {
  margin-bottom: 16px;
}
</style>
