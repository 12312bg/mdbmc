<script setup lang="ts">
import { NIcon } from 'naive-ui'
import { ServerOutline, DesktopOutline, NotificationsOutline } from '@vicons/ionicons5'
import wordmark from '@/assets/logo-wordmark.png'
import { useSession } from '@/stores/session'

const session = useSession()
const year = new Date().getFullYear()
const points = [
  { icon: ServerOutline, text: '多品牌 BMC 统一接入' },
  { icon: DesktopOutline, text: '远程控制台一站操作' },
  { icon: NotificationsOutline, text: '异常即时告警，操作全程留痕' },
]
</script>

<template>
  <div class="auth">
    <aside class="hero">
      <div class="hero-top">
        <!-- The wordmark's dark lettering needs a light plate on the dark hero. -->
        <span class="hero-logo"><img :src="wordmark" alt="明点信科" draggable="false" /></span>
      </div>
      <div class="hero-body">
        <h1>明点服务器管理系统</h1>
        <p class="tagline">一个窗口，管好每一台服务器</p>
        <ul class="points">
          <li v-for="p in points" :key="p.text">
            <span class="point-icon"><n-icon :component="p.icon" :size="16" /></span>
            {{ p.text }}
          </li>
        </ul>
      </div>
      <svg class="rack" viewBox="0 0 320 260" aria-hidden="true">
        <defs>
          <linearGradient id="unit" x1="0" x2="1">
            <stop offset="0" stop-color="#ffffff" stop-opacity="0.16" />
            <stop offset="1" stop-color="#ffffff" stop-opacity="0.06" />
          </linearGradient>
        </defs>
        <rect x="40" y="10" width="240" height="240" rx="12" fill="none" stroke="#ffffff" stroke-opacity="0.18" />
        <g v-for="i in 8" :key="i">
          <rect x="56" :y="26 + (i - 1) * 27" width="208" height="19" rx="4" fill="url(#unit)" />
          <circle cx="246" :cy="35.5 + (i - 1) * 27" r="3" :fill="i === 3 ? '#fab219' : '#0ca30c'" />
          <rect x="68" :y="33 + (i - 1) * 27" :width="60 + ((i * 37) % 60)" height="5" rx="2.5" fill="#ffffff" fill-opacity="0.22" />
        </g>
      </svg>
      <div class="hero-foot">© {{ year }} 明点信科<template v-if="session.status?.version"> · v{{ session.status.version }}</template></div>
    </aside>
    <main class="panel">
      <div class="panel-inner">
        <slot />
      </div>
    </main>
  </div>
</template>

<style scoped>
.auth {
  display: flex;
  height: 100vh;
  background: var(--surface);
}
.hero {
  position: relative;
  flex: 0 0 44%;
  max-width: 620px;
  padding: 40px 48px;
  color: #fff;
  background: radial-gradient(120% 90% at 0% 0%, #1f4f8f 0%, #13305a 45%, #0b1b33 100%);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.hero-top {
  display: flex;
  align-items: center;
}
.hero-logo {
  display: inline-flex;
  padding: 9px 14px;
  border-radius: 10px;
  background: #fff;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.18);
}
.hero-logo img {
  display: block;
  height: 40px;
  width: auto;
}
.hero-body {
  margin-top: 12vh;
  position: relative;
  z-index: 1;
}
.hero-body h1 {
  font-size: 34px;
  line-height: 1.35;
  margin: 0;
  font-weight: 600;
  letter-spacing: 0.04em;
}
.hero-body h1::after {
  content: '';
  display: block;
  width: 48px;
  height: 3px;
  margin-top: 18px;
  border-radius: 2px;
  background: #86b6ef;
}
.tagline {
  margin: 22px 0 0;
  font-size: 17px;
  line-height: 1.6;
  letter-spacing: 0.04em;
  color: rgba(255, 255, 255, 0.88);
}
.points {
  list-style: none;
  margin: 28px 0 0;
  padding: 0;
}
.points li {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 14px;
  line-height: 20px;
  color: rgba(255, 255, 255, 0.78);
}
.points li + li {
  margin-top: 14px;
}
.point-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: none;
  width: 30px;
  height: 30px;
  border-radius: 8px;
  background: rgba(134, 182, 239, 0.16);
  color: #9cc4f3;
}
.rack {
  position: absolute;
  right: -40px;
  bottom: 40px;
  width: 340px;
  opacity: 0.7;
}
.hero-foot {
  margin-top: auto;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
}
.panel {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px;
  background: var(--surface);
}
.panel-inner {
  width: 100%;
  max-width: 380px;
}
@media (max-height: 780px) {
  .rack {
    width: 220px;
  }
}
@media (max-width: 900px) {
  .hero {
    display: none;
  }
}
</style>
