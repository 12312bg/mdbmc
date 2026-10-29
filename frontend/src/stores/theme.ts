import { defineStore } from 'pinia'
import { computed, nextTick, ref, watch } from 'vue'

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

// Naive UI animates colour changes on nearly every element (0.3s on each
// table cell, input, button…), so a theme switch starts hundreds of
// transitions at once and stutters. Transitions are off while the new
// colours are applied, and back on two frames later.
function withoutTransitions(apply: () => void) {
  const style = document.createElement('style')
  style.textContent = '*,*::before,*::after{transition:none!important}'
  document.head.appendChild(style)
  apply()
  nextTick(() => {
    // Components have re-rendered with the new theme: apply its colours now.
    void window.getComputedStyle(document.body).color
    requestAnimationFrame(() => requestAnimationFrame(() => style.remove()))
  })
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
  watch(dark, (d) => withoutTransitions(() => document.documentElement.setAttribute('data-theme', d ? 'dark' : 'light')), { immediate: true })

  function cycle() {
    mode.value = mode.value === 'auto' ? 'light' : mode.value === 'light' ? 'dark' : 'auto'
  }

  return { mode, dark, cycle }
})
