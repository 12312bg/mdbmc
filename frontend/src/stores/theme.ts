import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'

export type ThemeMode = 'auto' | 'light' | 'dark'

function load(): ThemeMode {
  try {
    const v = localStorage.getItem('mdbmc.theme')
    if (v === 'light' || v === 'dark' || v === 'auto') return v
  } catch {
    /* storage unavailable */
  }
  return 'auto'
}

export const useTheme = defineStore('theme', () => {
  const mode = ref<ThemeMode>(load())
  const media = window.matchMedia('(prefers-color-scheme: dark)')
  const osDark = ref(media.matches)
  media.addEventListener('change', (e) => (osDark.value = e.matches))

  const dark = computed(() => (mode.value === 'auto' ? osDark.value : mode.value === 'dark'))

  watch(mode, (m) => {
    try {
      localStorage.setItem('mdbmc.theme', m)
    } catch {
      /* ignore */
    }
  })
  watch(dark, (d) => document.documentElement.setAttribute('data-theme', d ? 'dark' : 'light'), { immediate: true })

  function cycle() {
    mode.value = mode.value === 'auto' ? 'light' : mode.value === 'light' ? 'dark' : 'auto'
  }

  return { mode, dark, cycle }
})
