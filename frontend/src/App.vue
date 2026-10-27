<script setup lang="ts">
import { computed } from 'vue'
import {
  NConfigProvider, NMessageProvider, NDialogProvider, NNotificationProvider, NLoadingBarProvider,
  darkTheme, zhCN, dateZhCN, type GlobalThemeOverrides,
} from 'naive-ui'
import { useTheme } from '@/stores/theme'

const theme = useTheme()

const font = "'Segoe UI', 'Microsoft YaHei UI', 'Microsoft YaHei', 'PingFang SC', system-ui, sans-serif"
const overrides = computed<GlobalThemeOverrides>(() => {
  const primary = theme.dark ? '#3987e5' : '#2a78d6'
  return {
    common: {
      fontFamily: font,
      primaryColor: primary,
      primaryColorHover: theme.dark ? '#5598e7' : '#3987e5',
      primaryColorPressed: theme.dark ? '#2a78d6' : '#256abf',
      primaryColorSuppl: primary,
      borderRadius: '6px',
      borderRadiusSmall: '4px',
      bodyColor: theme.dark ? '#111215' : '#f4f6f9',
      cardColor: theme.dark ? '#1a1b1f' : '#ffffff',
      modalColor: theme.dark ? '#1f2025' : '#ffffff',
      popoverColor: theme.dark ? '#24262b' : '#ffffff',
      tableColor: theme.dark ? '#1a1b1f' : '#ffffff',
    },
    DataTable: {
      thFontWeight: '600',
      thColor: theme.dark ? '#202227' : '#f7f8fa',
      tdColorHover: theme.dark ? '#22252b' : '#f5f8fd',
    },
    Card: { borderRadius: '10px' },
    Tag: { borderRadius: '4px' },
  }
})
</script>

<template>
  <n-config-provider :theme="theme.dark ? darkTheme : null" :theme-overrides="overrides" :locale="zhCN" :date-locale="dateZhCN">
    <n-loading-bar-provider>
      <n-message-provider placement="top">
        <n-notification-provider placement="bottom-right">
          <n-dialog-provider>
            <router-view />
          </n-dialog-provider>
        </n-notification-provider>
      </n-message-provider>
    </n-loading-bar-provider>
  </n-config-provider>
</template>
