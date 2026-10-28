<script setup lang="ts">
import AppLogo from './AppLogo.vue'
import { useSession } from '@/stores/session'

const session = useSession()
</script>

<template>
  <div class="auth">
    <aside class="hero">
      <div class="hero-top">
        <AppLogo :size="40" plate="always" />
        <span class="hero-brand">明点服务器管理系统</span>
      </div>
      <div class="hero-body">
        <h1>多品牌服务器<br />带外统一管理</h1>
        <p>通过 IPMI 2.0 与 Redfish 统一管理 Supermicro、Dell、HPE、Lenovo、华为、浪潮、新华三等服务器的 BMC。</p>
        <ul>
          <li>批量开关机、重启与电源循环</li>
          <li>传感器、事件日志与硬件资产集中查看</li>
          <li>一键打开 Web 管理与远程 KVM 控制台</li>
          <li>BMC 密码加密保存，按角色控制权限</li>
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
      <div class="hero-foot">v{{ session.status?.version }}</div>
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
  gap: 12px;
}
.hero-brand {
  font-size: 18px;
  font-weight: 700;
  letter-spacing: 0.06em;
}
.hero-body {
  margin-top: 12vh;
  position: relative;
  z-index: 1;
}
.hero-body h1 {
  font-size: 32px;
  line-height: 1.35;
  margin: 0 0 16px;
  font-weight: 600;
}
.hero-body p {
  color: rgba(255, 255, 255, 0.78);
  line-height: 1.7;
  max-width: 420px;
  margin: 0 0 20px;
}
.hero-body ul {
  margin: 0;
  padding: 0;
  list-style: none;
  color: rgba(255, 255, 255, 0.86);
  line-height: 2.1;
}
.hero-body li::before {
  content: '';
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #86b6ef;
  margin-right: 10px;
  vertical-align: middle;
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
@media (max-width: 900px) {
  .hero {
    display: none;
  }
}
</style>
