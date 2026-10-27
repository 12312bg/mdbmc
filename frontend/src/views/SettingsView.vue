<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { NForm, NFormItem, NInputNumber, NButton, NAlert, NDescriptions, NDescriptionsItem, useMessage } from 'naive-ui'
import { api, errMsg, type Settings } from '@/api'
import { useSession } from '@/stores/session'

const session = useSession()
const message = useMessage()
const form = ref<Settings | null>(null)
const saving = ref(false)
const canEdit = computed(() => session.can('settings.manage'))

onMounted(async () => {
  try {
    form.value = await api.admin.getSettings()
  } catch (e) {
    message.error(errMsg(e))
  }
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
      </div>
      <div class="side">
        <div class="card section">
          <div class="card-title" style="margin-bottom: 12px">关于</div>
          <n-descriptions :column="1" label-placement="left" size="small">
            <n-descriptions-item label="版本">{{ session.status?.version }}</n-descriptions-item>
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
