import type { Health, PowerState, PowerAction } from '@/api'

const pad = (n: number) => String(n).padStart(2, '0')

export function formatTime(ts: number): string {
  if (!ts) return '—'
  const d = new Date(ts * 1000)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

export function relativeTime(ts: number): string {
  if (!ts) return '从未'
  const s = Math.floor(Date.now() / 1000) - ts
  if (s < 10) return '刚刚'
  if (s < 60) return `${s} 秒前`
  if (s < 3600) return `${Math.floor(s / 60)} 分钟前`
  if (s < 86400) return `${Math.floor(s / 3600)} 小时前`
  if (s < 86400 * 30) return `${Math.floor(s / 86400)} 天前`
  return formatTime(ts).slice(0, 10)
}

export function formatBytes(n: number): string {
  if (!n) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let i = 0
  let v = n
  while (v >= 1000 && i < units.length - 1) {
    v /= 1000
    i++
  }
  return `${v >= 100 ? v.toFixed(0) : v.toFixed(v >= 10 ? 1 : 2)} ${units[i]}`
}

export function formatNumber(v: number | null | undefined, unit = ''): string {
  if (v === null || v === undefined) return '—'
  const s = Math.abs(v) >= 100 || Number.isInteger(v) ? String(Math.round(v)) : v.toFixed(2)
  return unit ? `${s} ${unit}` : s
}

export const powerLabel: Record<PowerState, string> = { on: '已开机', off: '已关机', unknown: '未知' }
export const healthLabel: Record<Health, string> = { ok: '正常', warning: '警告', critical: '严重', unknown: '未知' }

export const powerActions: { key: PowerAction; label: string; danger: boolean; confirm: string }[] = [
  { key: 'on', label: '开机', danger: false, confirm: '确定为所选设备开机？' },
  { key: 'soft', label: '正常关机', danger: false, confirm: '将通过 ACPI 通知操作系统关机，确定继续？' },
  { key: 'off', label: '强制关机', danger: true, confirm: '强制关机会立即断电，未保存的数据将丢失。确定继续？' },
  { key: 'reset', label: '硬重启', danger: true, confirm: '硬重启会立即重置主机，未保存的数据将丢失。确定继续？' },
  { key: 'cycle', label: '电源循环', danger: true, confirm: '电源循环会先断电再上电，未保存的数据将丢失。确定继续？' },
]

export const sensorCategoryLabel: Record<string, string> = {
  temperature: '温度', fan: '风扇', voltage: '电压', current: '电流', power: '电源', other: '其他',
}

export const auditActionLabel: Record<string, string> = {
  'auth.login': '登录',
  'auth.login_failed': '登录失败',
  'auth.logout': '退出登录',
  'auth.lock': '空闲锁定',
  'auth.password_change': '修改密码',
  'user.create': '创建用户',
  'user.update': '修改用户',
  'user.reset_password': '重置用户密码',
  'user.delete': '删除用户',
  'device.create': '添加设备',
  'device.update': '修改设备',
  'device.delete': '删除设备',
  'device.move_group': '移动分组',
  'device.set_credential': '更换凭据',
  'device.reveal_password': '查看设备密码',
  'device.trust_cert': '重新信任证书',
  'group.create': '创建分组',
  'group.rename': '重命名分组',
  'group.delete': '删除分组',
  'credential.create': '创建凭据',
  'credential.update': '修改凭据',
  'credential.delete': '删除凭据',
  'credential.reveal': '查看凭据密码',
  'power.on': '开机',
  'power.off': '强制关机',
  'power.soft': '正常关机',
  'power.reset': '硬重启',
  'power.cycle': '电源循环',
  'power.identify': '定位灯',
  'sel.clear': '清空事件日志',
  'console.web': '打开 Web 管理',
  'console.kvm': '打开远程控制台',
  'settings.update': '修改系统设置',
  'scan.start': '开始扫描',
  'scan.finish': '扫描完成',
}

export const protocolLabel: Record<string, string> = { auto: '自动', ipmi: '仅 IPMI', redfish: '仅 Redfish' }
