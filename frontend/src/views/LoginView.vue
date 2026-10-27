<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NForm, NFormItem, NInput, NButton, NAlert, NIcon } from 'naive-ui'
import { PersonCircleOutline, LockClosedOutline } from '@vicons/ionicons5'
import AuthShell from '@/components/AuthShell.vue'
import { api, errMsg } from '@/api'
import { useSession } from '@/stores/session'

const session = useSession()
const router = useRouter()
const route = useRoute()

const username = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  if (!username.value || !password.value) {
    error.value = '请输入用户名和密码'
    return
  }
  loading.value = true
  error.value = ''
  try {
    await session.login(username.value.trim(), password.value)
    password.value = ''
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/dashboard'
    await router.replace(redirect)
  } catch (e) {
    error.value = errMsg(e)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <AuthShell>
    <h2 class="title">登录</h2>
    <p class="sub">使用本地账号登录。登录密码同时用于解锁已保存的 BMC 密码。</p>
    <n-alert v-if="session.lockedReason" type="info" :bordered="false" class="gap">{{ session.lockedReason }}</n-alert>
    <n-alert v-if="api.isMock" type="default" :bordered="false" :show-icon="false" class="gap">
      预览模式：用户名 admin / zhangwei / liuyang，任意 8 位以上密码。
    </n-alert>
    <n-form @submit.prevent="submit">
      <n-form-item label="用户名" :show-feedback="false" class="gap">
        <n-input v-model:value="username" size="large" placeholder="用户名" autofocus :input-props="{ autocomplete: 'username' }">
          <template #prefix><n-icon :component="PersonCircleOutline" /></template>
        </n-input>
      </n-form-item>
      <n-form-item label="密码" :show-feedback="false" class="gap">
        <n-input
          v-model:value="password"
          size="large"
          type="password"
          show-password-on="click"
          placeholder="密码"
          :input-props="{ autocomplete: 'current-password' }"
        >
          <template #prefix><n-icon :component="LockClosedOutline" /></template>
        </n-input>
      </n-form-item>
      <n-alert v-if="error" type="error" :bordered="false" class="gap">{{ error }}</n-alert>
      <n-button type="primary" size="large" block :loading="loading" attr-type="submit">登录</n-button>
    </n-form>
    <p class="foot muted">忘记密码请联系管理员重置。所有管理员都忘记密码时，已保存的 BMC 密码无法恢复。</p>
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
  margin: 0 0 24px;
  line-height: 1.6;
  font-size: 13px;
}
.gap {
  margin-bottom: 16px;
}
.foot {
  margin-top: 28px;
  font-size: 12px;
  line-height: 1.6;
}
</style>
