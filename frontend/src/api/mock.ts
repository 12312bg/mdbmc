// In-memory backend for previewing the UI in a plain browser (npm run dev).
// Any password of 8+ characters logs in as an existing user.
import type {
  Api, AuditEntry, Credential, Device, EventMap, Group, Inventory, Role, SELEntry, Sensor, UserDTO, UserInfo,
} from './types'

const now = () => Math.floor(Date.now() / 1000)
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))
const wait = () => sleep(150 + Math.random() * 250)

const perms: Record<Role, UserInfo['permissions']> = {
  admin: ['device.view', 'device.manage', 'credential.reveal', 'power.control', 'sel.clear', 'console.web',
    'console.autologin', 'console.kvm', 'user.manage', 'settings.manage', 'audit.view'],
  operator: ['device.view', 'device.manage', 'credential.reveal', 'power.control', 'sel.clear', 'console.web',
    'console.autologin', 'console.kvm'],
  viewer: ['device.view', 'console.web'],
}
const roleLabel: Record<Role, string> = { admin: '管理员', operator: '操作员', viewer: '只读' }

const listeners = new Map<string, Set<(d: any) => void>>()
function emit<K extends keyof EventMap>(name: K, data: EventMap[K]) {
  listeners.get(name)?.forEach((cb) => cb(data))
}

const params = new URLSearchParams(location.search)
let initialized = !params.has('setup')
let current: UserInfo | null = null

const users: UserDTO[] = [
  { id: 1, username: 'admin', displayName: '系统管理员', role: 'admin', roleLabel: '管理员', disabled: false, mustChangePassword: false, createdAt: now() - 86400 * 40, lastLoginAt: now() - 3600 },
  { id: 2, username: 'zhangwei', displayName: '张伟', role: 'operator', roleLabel: '操作员', disabled: false, mustChangePassword: false, createdAt: now() - 86400 * 30, lastLoginAt: now() - 7200 },
  { id: 3, username: 'liuyang', displayName: '刘洋', role: 'viewer', roleLabel: '只读', disabled: false, mustChangePassword: true, createdAt: now() - 86400 * 3, lastLoginAt: 0 },
]

const groups: Group[] = [
  { id: 1, name: '机房 A · 01 列', deviceCount: 0 },
  { id: 2, name: '机房 A · 02 列', deviceCount: 0 },
  { id: 3, name: '测试环境', deviceCount: 0 },
]

const creds: Credential[] = [
  { id: 1, name: 'Supermicro 默认', username: 'ADMIN', note: '出厂后统一修改的运维密码', deviceCount: 0, updatedAt: now() - 86400 * 7 },
  { id: 2, name: 'Dell iDRAC', username: 'root', note: '', deviceCount: 0, updatedAt: now() - 86400 * 20 },
  { id: 3, name: 'HPE iLO', username: 'Administrator', note: '', deviceCount: 0, updatedAt: now() - 86400 * 20 },
]

const vendorNames: Record<string, string> = {
  supermicro: 'Supermicro', dell: 'Dell', hpe: 'HPE', lenovo: 'Lenovo', huawei: '华为 / 超聚变', inspur: '浪潮', h3c: '新华三',
  fujitsu: 'Fujitsu', cisco: 'Cisco', intel: 'Intel', asus: 'ASUS', gigabyte: 'Gigabyte', asrock: 'ASRock Rack', quanta: 'QCT',
  sugon: '中科曙光', nettrix: '宁畅', tyan: 'Tyan', ibm: 'IBM', generic: '通用',
}

type Seed = [string, string, string, string, number, number, Device['powerState'], Device['health'], boolean]
const seeds: Seed[] = [
  ['sm-node01', '10.10.1.11', 'supermicro', 'SYS-120U-TNR', 1, 1, 'on', 'ok', true],
  ['sm-node02', '10.10.1.12', 'supermicro', 'SYS-120U-TNR', 1, 1, 'on', 'warning', true],
  ['sm-node03', '10.10.1.13', 'supermicro', 'SYS-120U-TNR', 1, 1, 'on', 'ok', true],
  ['sm-node04', '10.10.1.14', 'supermicro', 'SYS-620U-TNR', 1, 1, 'off', 'ok', true],
  ['sm-gpu01', '10.10.1.21', 'supermicro', 'SYS-420GP-TNR', 2, 1, 'on', 'ok', true],
  ['sm-gpu02', '10.10.1.22', 'supermicro', 'SYS-420GP-TNR', 2, 1, 'on', 'critical', true],
  ['dell-db01', '10.10.2.11', 'dell', 'PowerEdge R750', 2, 2, 'on', 'ok', true],
  ['dell-db02', '10.10.2.12', 'dell', 'PowerEdge R750', 2, 2, 'on', 'ok', true],
  ['dell-app01', '10.10.2.21', 'dell', 'PowerEdge R650', 2, 2, 'unknown', 'unknown', false],
  ['hpe-vm01', '10.10.3.11', 'hpe', 'ProLiant DL380 Gen10 Plus', 1, 3, 'on', 'ok', true],
  ['hpe-vm02', '10.10.3.12', 'hpe', 'ProLiant DL360 Gen10', 1, 3, 'on', 'ok', true],
  ['lenovo-bk01', '10.10.4.11', 'lenovo', 'ThinkSystem SR650 V2', 3, 0, 'on', 'ok', true],
  ['hw-test01', '10.10.5.11', 'huawei', 'FusionServer 2288H V6', 3, 0, 'off', 'ok', true],
  ['inspur-test01', '10.10.5.21', 'inspur', 'NF5280M6', 3, 0, 'on', 'warning', true],
  ['h3c-test01', '10.10.5.31', 'h3c', 'UniServer R4900 G5', 3, 0, 'on', 'ok', true],
  ['sugon-test01', '10.10.5.41', 'sugon', 'I620-G30', 3, 0, 'on', 'ok', true],
  ['lab-fj01', '10.10.6.11', 'fujitsu', 'PRIMERGY RX2540 M5', 3, 0, 'off', 'ok', true],
  ['lab-ucs01', '10.10.6.12', 'cisco', 'UCSC-C220-M5SX', 3, 0, 'on', 'ok', true],
  ['lab-gb01', '10.10.6.13', 'gigabyte', 'R282-Z90', 3, 0, 'on', 'ok', true],
  ['lab-old01', '10.10.6.14', 'generic', 'S2600WT', 3, 0, 'on', 'ok', true],
]

let nextDeviceId = 1
const devices: Device[] = seeds.map(([name, host, vendor, model, gid, cid, power, health, reachable]) => ({
  id: nextDeviceId++, name, host, ipmiPort: 623, httpsPort: 443, protocol: 'auto', groupId: gid, groupName: '',
  credentialId: cid, credentialName: '', username: cid ? '' : 'admin', hasPassword: !cid, note: '',
  vendor, vendorName: vendorNames[vendor], manufacturer: vendorNames[vendor].split(' ')[0], model,
  serial: `${vendor.slice(0, 2).toUpperCase()}${(100000 + nextDeviceId * 7919).toString(36).toUpperCase()}`,
  bmcFirmware: vendor === 'dell' ? '6.10.30.00' : vendor === 'hpe' ? 'iLO 5 v2.81' : '01.01.10',
  biosVersion: vendor === 'dell' ? '1.9.2' : '1.4a', hasIpmi: true, hasRedfish: true,
  tlsFingerprint: Array.from({ length: 32 }, (_, i) => ((nextDeviceId * 37 + i * 11) % 256).toString(16).padStart(2, '0')).join(''),
  powerState: power, health, reachable, lastError: reachable ? '' : 'IPMI: 连接超时，请检查地址、端口与网络；Redfish: 连接超时',
  checkedAt: now() - 40, identifiedAt: now() - 86400, createdAt: now() - 86400 * 10,
}))

function decorate(d: Device): Device {
  return {
    ...d,
    groupName: groups.find((g) => g.id === d.groupId)?.name ?? '',
    credentialName: creds.find((c) => c.id === d.credentialId)?.name ?? '',
    vendorName: vendorNames[d.vendor] ?? '',
  }
}

const audit: AuditEntry[] = []
let auditId = 1
function log(action: string, target: string, success = true, detail = '', user = current?.username ?? '') {
  audit.unshift({ id: auditId++, ts: now(), username: user, action, target, success, detail })
}
;[
  ['admin', 'auth.login', 'admin', true, ''],
  ['zhangwei', 'power.cycle', 'sm-node02 (10.10.1.12)', true, '电源循环'],
  ['zhangwei', 'console.kvm', 'sm-gpu02 (10.10.1.22)', true, ''],
  ['admin', 'device.create', '扫描添加 4 台', true, ''],
  ['zhangwei', 'sel.clear', 'dell-db01 (10.10.2.11)', true, ''],
  ['liuyang', 'auth.login_failed', 'liuyang', false, '密码错误'],
].forEach(([u, a, t, s, d], i) => {
  audit.push({ id: auditId++, ts: now() - 600 * (i + 1) - 3600, username: u as string, action: a as string, target: t as string, success: s as boolean, detail: d as string })
})

let settings = { pollIntervalSec: 120, maxConcurrency: 32, ipmiTimeoutSec: 5, redfishTimeoutSec: 20, idleLockMinutes: 30, scanConcurrency: 64 }

function deviceOr404(id: number) {
  const d = devices.find((x) => x.id === id)
  if (!d) throw new Error('记录不存在')
  return d
}
function need(p: UserInfo['permissions'][number]) {
  if (!current) throw new Error('未登录或会话已锁定，请重新登录')
  if (!current.permissions.includes(p)) throw new Error('当前角色没有执行此操作的权限')
}
function reachableOrThrow(d: Device) {
  if (!d.reachable) throw new Error(d.lastError)
}

function sensorsFor(d: Device): Sensor[] {
  const on = d.powerState === 'on'
  const j = (v: number, s: number) => Math.round((v + (Math.random() * 2 - 1) * s) * 100) / 100
  const t = (name: string, v: number, unc: number, ucr: number): Sensor => ({
    name, category: 'temperature', type: 'Temperature', reading: j(on ? v : v * 0.55, 1.5), unit: '°C',
    status: 'ok', text: '', lowerCritical: null, lowerWarning: null, upperWarning: unc, upperCritical: ucr, source: 'ipmi',
  })
  const f = (name: string, v: number): Sensor => {
    const r = on ? Math.round(j(v, 150) / 100) * 100 : 0
    return {
      name, category: 'fan', type: 'Fan', reading: r, unit: 'RPM', status: r <= 420 ? 'critical' : r <= 700 ? 'warning' : 'ok',
      text: '', lowerCritical: 420, lowerWarning: 700, upperWarning: null, upperCritical: null, source: 'ipmi',
    }
  }
  const v = (name: string, val: number, tol: number): Sensor => ({
    name, category: 'voltage', type: 'Voltage', reading: j(val, val * 0.004), unit: 'V', status: 'ok', text: '',
    lowerCritical: Math.round(val * (1 - tol) * 100) / 100, lowerWarning: Math.round(val * (1 - tol * 0.6) * 100) / 100,
    upperWarning: Math.round(val * (1 + tol * 0.6) * 100) / 100, upperCritical: Math.round(val * (1 + tol) * 100) / 100, source: 'ipmi',
  })
  const out: Sensor[] = [
    t('CPU1 Temp', 46, 90, 95), t('CPU2 Temp', 49, 90, 95), t('System Temp', 31, 80, 85), t('Peripheral Temp', 39, 80, 85),
    t('Inlet Temp', 24, 40, 45), f('FAN1', 8400), f('FAN2', 8200), f('FAN3', 8300), f('FAN4', d.health === 'warning' ? 600 : 8100),
    v('12V', 12.06, 0.1), v('5VCC', 5.04, 0.1), v('3.3VCC', 3.32, 0.1), v('VBAT', 3.1, 0.15),
    { name: 'PS1 Input Power', category: 'power', type: 'Power Supply', reading: on ? j(256, 16) : 0, unit: 'W', status: 'ok', text: '', lowerCritical: null, lowerWarning: null, upperWarning: null, upperCritical: null, source: 'ipmi' },
    { name: 'PS1 Status', category: 'power', type: 'Power Supply', reading: null, unit: '', status: 'ok', text: 'Presence detected', lowerCritical: null, lowerWarning: null, upperWarning: null, upperCritical: null, source: 'ipmi' },
    { name: 'PS2 Status', category: 'power', type: 'Power Supply', reading: null, unit: '', status: d.health === 'critical' ? 'critical' : 'ok', text: d.health === 'critical' ? 'Presence detected, Power Supply Failure detected' : 'Presence detected', lowerCritical: null, lowerWarning: null, upperWarning: null, upperCritical: null, source: 'ipmi' },
    { name: 'Chassis Intru', category: 'other', type: 'Physical Security', reading: null, unit: '', status: 'ok', text: '', lowerCritical: null, lowerWarning: null, upperWarning: null, upperCritical: null, source: 'ipmi' },
  ]
  return out
}

const selByDevice = new Map<number, SELEntry[]>()
function selFor(d: Device): SELEntry[] {
  if (!selByDevice.has(d.id)) {
    const t = now()
    const rows: SELEntry[] = [
      { id: '0006', time: t - 2100, sensor: 'FAN4', sensorType: 'Fan', message: 'Lower Critical going low', severity: 'critical', direction: 'assert', source: 'ipmi' },
      { id: '0005', time: t - 7200, sensor: 'P1-DIMMA1', sensorType: 'Memory', message: 'Correctable ECC', severity: 'warning', direction: 'assert', source: 'ipmi' },
      { id: '0004', time: t - 18000, sensor: 'PS2 Status', sensorType: 'Power Supply', message: 'Power Supply Failure detected', severity: 'critical', direction: 'assert', source: 'ipmi' },
      { id: '0003', time: t - 93420, sensor: 'CPU1 Temp', sensorType: 'Temperature', message: 'Upper Non-critical going high', severity: 'ok', direction: 'deassert', source: 'ipmi' },
      { id: '0002', time: t - 93600, sensor: 'CPU1 Temp', sensorType: 'Temperature', message: 'Upper Non-critical going high', severity: 'warning', direction: 'assert', source: 'ipmi' },
      { id: '0001', time: t - 259200, sensor: 'Power Unit', sensorType: 'Power Unit', message: 'Power Off / Power Down', severity: 'info', direction: 'assert', source: 'ipmi' },
    ]
    selByDevice.set(d.id, rows)
  }
  return selByDevice.get(d.id)!
}

function inventoryFor(d: Device): Inventory {
  const dimm = (i: number) => ({ slot: `P${Math.floor(i / 4) + 1}-DIMM${'ABCD'[i % 4]}1`, capacityMiB: 32768, type: 'DDR4', speedMHz: 3200, manufacturer: 'Samsung', partNumber: 'M393A4K40EB3-CWE', serial: (0x40000000 + d.id * 16 + i).toString(16).toUpperCase(), health: 'ok' })
  return {
    system: { manufacturer: d.manufacturer, model: d.model, serial: d.serial, sku: '', partNumber: '', uuid: '2c1b5a00-0000-11ec-8000-3cecef00000' + d.id, hostName: d.name, biosVersion: d.biosVersion, powerState: d.powerState, health: d.health, processorModel: 'Intel(R) Xeon(R) Gold 6338 CPU @ 2.00GHz', processorCount: 2, coreCount: 64, threadCount: 128, memoryGiB: 256 },
    manager: { model: 'ASPEED', firmwareVersion: d.bmcFirmware, macAddress: '3c:ec:ef:00:00:' + d.id.toString(16).padStart(2, '0'), ipv4: d.host },
    processors: [1, 2].map((n) => ({ socket: 'CPU' + n, model: 'Intel(R) Xeon(R) Gold 6338 CPU @ 2.00GHz', manufacturer: 'Intel(R) Corporation', cores: 32, threads: 64, maxSpeedMHz: 3200, health: 'ok' })),
    memory: Array.from({ length: 8 }, (_, i) => dimm(i)),
    drives: [0, 1].map((n) => ({ name: 'Disk.Bay.' + n, model: 'SAMSUNG MZ7L3960HCJR-00A07', serial: `S664NE0T${d.id}${n}`, capacityBytes: 960197124096, mediaType: 'SSD', protocol: 'SATA', firmware: 'JXTC104Q', health: 'ok' })),
    nics: [1, 2].map((n) => ({ name: 'Ethernet Interface ' + n, macAddress: `3c:ec:ef:1${n}:00:${d.id.toString(16).padStart(2, '0')}`, speedMbps: 10000, linkStatus: n === 1 ? 'LinkUp' : 'LinkDown', ipv4: n === 1 ? ['10.20.0.' + (10 + d.id)] : null, health: 'ok' })),
    powerSupplies: [1, 2].map((n) => ({ name: 'PS' + n, model: 'PWS-1K02A-1R', serial: `P1K02AL0${d.id}${n}`, capacityWatts: 1000, firmware: '1.2', health: n === 2 && d.health === 'critical' ? 'critical' : 'ok' })),
    firmware: [{ name: 'BMC Firmware', version: d.bmcFirmware, updateable: true }, { name: 'BIOS Firmware', version: d.biosVersion, updateable: true }],
    fru: [{ device: 'Builtin FRU Device (ID 0)', chassisPart: 'CSE-819UTS-R1K02P-T', chassisSerial: 'C8190' + d.serial, boardMfg: d.manufacturer, boardProduct: 'X12DPU-6', boardSerial: 'WM23AS' + d.id, boardPart: 'X12DPU-6', productMfg: d.manufacturer, productName: d.model, productPart: d.model, productVersion: '0123456789', productSerial: d.serial }],
    source: 'redfish', collectedAt: now(),
    ...mockInventoryErrors(d),
  }
}

// Some preview devices show how unreadable sections look.
function mockInventoryErrors(d: Device): Partial<Inventory> {
  if (d.vendor === 'inspur') {
    return {
      powerSupplies: [], firmware: [],
      errors: { powerSupplies: 'Redfish 读取电源失败: HTTP 404 Not Found', firmware: 'Redfish 读取固件清单失败: HTTP 404 Not Found' },
    }
  }
  if (d.vendor === 'huawei') return { errors: { drives: 'Redfish 读取磁盘失败: RAID 控制器 RAIDStorage1 响应超时', manager: 'Redfish 读取BMC 信息失败: HTTP 500' } }
  return {}
}
const inventoryCache = new Map<number, { inv: Inventory; ts: number }>()

function toInfo(u: UserDTO): UserInfo {
  return { id: u.id, username: u.username, displayName: u.displayName, role: u.role, roleLabel: roleLabel[u.role], permissions: perms[u.role], mustChangePassword: u.mustChangePassword }
}

export const mockApi: Api = {
  isMock: true,
  auth: {
    async status() {
      return { initialized, user: current, version: '0.1.0-preview', dataDir: 'C:\\Users\\ops\\AppData\\Roaming\\mdbmc' }
    },
    async setup(username, displayName) {
      await wait()
      initialized = true
      users.splice(0, users.length, { id: 1, username, displayName, role: 'admin', roleLabel: '管理员', disabled: false, mustChangePassword: false, createdAt: now(), lastLoginAt: now() })
      current = toInfo(users[0])
      return current
    },
    async login(username, password) {
      await sleep(400)
      const u = users.find((x) => x.username.toLowerCase() === username.toLowerCase())
      if (!u || password.length < 8) {
        log('auth.login_failed', username, false, '密码错误', '')
        throw new Error('用户名或密码错误')
      }
      if (u.disabled) throw new Error('该账号已被停用，请联系管理员')
      current = toInfo(u)
      u.lastLoginAt = now()
      log('auth.login', u.username)
      return current
    },
    async logout() {
      if (current) log('auth.logout', current.username)
      current = null
    },
    async lock() {
      if (current) log('auth.lock', current.username)
      current = null
    },
    async changePassword(_o, n) {
      await wait()
      if (n.length < 8) throw new Error('密码长度至少 8 位')
      const u = users.find((x) => x.id === current?.id)
      if (u) u.mustChangePassword = false
      if (current) current = { ...current, mustChangePassword: false }
      log('auth.password_change', current?.username ?? '')
    },
  },
  admin: {
    async roles() {
      return (['admin', 'operator', 'viewer'] as Role[]).map((r) => ({ value: r, label: roleLabel[r], permissions: perms[r] }))
    },
    async listUsers() { need('user.manage'); await wait(); return users.map((u) => ({ ...u })) },
    async createUser(i) {
      need('user.manage'); await wait()
      if (users.some((u) => u.username === i.username)) throw new Error('用户名已存在')
      users.push({ id: users.length + 10, username: i.username, displayName: i.displayName, role: i.role, roleLabel: roleLabel[i.role], disabled: false, mustChangePassword: true, createdAt: now(), lastLoginAt: 0 })
      log('user.create', i.username, true, '角色: ' + roleLabel[i.role])
    },
    async updateUser(i) {
      need('user.manage'); await wait()
      const u = users.find((x) => x.id === i.id)!
      if (u.id === current?.id && (u.role !== i.role || i.disabled)) throw new Error('不能修改自己的角色或停用自己')
      Object.assign(u, { displayName: i.displayName, role: i.role, roleLabel: roleLabel[i.role], disabled: i.disabled })
      log('user.update', u.username)
    },
    async resetPassword(id) { need('user.manage'); await wait(); const u = users.find((x) => x.id === id)!; u.mustChangePassword = true; log('user.reset_password', u.username) },
    async deleteUser(id) {
      need('user.manage'); await wait()
      if (id === current?.id) throw new Error('不能删除当前登录的账号')
      const i = users.findIndex((x) => x.id === id); log('user.delete', users[i].username); users.splice(i, 1)
    },
    async getSettings() { return { ...settings } },
    async saveSettings(s) { need('settings.manage'); await wait(); settings = { ...s }; log('settings.update', '') },
    async listAudit(q) {
      need('audit.view'); await wait()
      const kw = q.keyword.toLowerCase()
      const rows = audit.filter((e) => (!kw || [e.username, e.action, e.target, e.detail].some((x) => x.toLowerCase().includes(kw))) && (!q.since || e.ts >= q.since) && (!q.until || e.ts <= q.until))
      return { items: rows.slice((q.page - 1) * q.size, q.page * q.size), total: rows.length }
    },
  },
  devices: {
    async list() { need('device.view'); await wait(); return devices.map(decorate) },
    async get(id) { need('device.view'); return decorate(deviceOr404(id)) },
    async vendors() { return Object.entries(vendorNames).map(([id, name]) => ({ id, name })) },
    async create(i) {
      need('device.manage'); await wait()
      if (devices.some((d) => d.host === i.host && d.ipmiPort === i.ipmiPort && d.httpsPort === i.httpsPort)) throw new Error('相同地址和端口的设备已存在')
      const d: Device = { ...devices[0], id: nextDeviceId++, name: i.name || i.host, host: i.host, ipmiPort: i.ipmiPort || 623, httpsPort: i.httpsPort || 443, protocol: i.protocol, groupId: i.groupId, credentialId: i.credentialId, username: i.credentialId ? '' : i.username, hasPassword: !i.credentialId, note: i.note, vendor: '', model: '', serial: '', bmcFirmware: '', biosVersion: '', powerState: 'unknown', health: 'unknown', reachable: false, checkedAt: 0, identifiedAt: 0, lastError: '', createdAt: now() }
      devices.push(d)
      log('device.create', `${d.name} (${d.host})`)
      setTimeout(() => {
        Object.assign(d, { vendor: 'supermicro', manufacturer: 'Supermicro', model: 'SYS-120U-TNR', serial: 'S5040SIM' + d.id, bmcFirmware: '01.01.10', biosVersion: '1.4a', powerState: 'on', health: 'ok', reachable: true, checkedAt: now(), identifiedAt: now() })
        emit('device:updated', d.id)
      }, 1500)
      return decorate(d)
    },
    async update(i) {
      need('device.manage'); await wait()
      const d = deviceOr404(i.id)
      Object.assign(d, { name: i.name || i.host, host: i.host, ipmiPort: i.ipmiPort, httpsPort: i.httpsPort, protocol: i.protocol, groupId: i.groupId, credentialId: i.credentialId, username: i.credentialId ? '' : i.username, note: i.note })
      log('device.update', `${d.name} (${d.host})`)
      return decorate(d)
    },
    async remove(ids) {
      need('device.manage'); await wait()
      let n = 0
      for (const id of ids) { const i = devices.findIndex((d) => d.id === id); if (i >= 0) { devices.splice(i, 1); n++ } }
      log('device.delete', `${n} 台设备`)
      return n
    },
    async moveToGroup(ids, g) { need('device.manage'); await wait(); devices.filter((d) => ids.includes(d.id)).forEach((d) => (d.groupId = g)) },
    async setCredential(ids, c) { need('device.manage'); await wait(); devices.filter((d) => ids.includes(d.id)).forEach((d) => (d.credentialId = c)) },
    async revealPassword(id) { need('credential.reveal'); await wait(); log('device.reveal_password', deviceOr404(id).name); return 'Mdbmc@2026' },
    async identify(id) {
      need('device.view'); await sleep(900)
      const d = deviceOr404(id); reachableOrThrow(d); d.identifiedAt = now(); return decorate(d)
    },
    async refreshStatus(ids) {
      need('device.view')
      const list = ids.length ? devices.filter((d) => ids.includes(d.id)) : devices
      emit('poll:state', { running: true })
      list.forEach((d, i) => setTimeout(() => {
        d.checkedAt = now()
        emit('device:status', { id: d.id, powerState: d.powerState, health: d.health, reachable: d.reachable, lastError: d.lastError, checkedAt: d.checkedAt })
        if (i === list.length - 1) emit('poll:state', { running: false, lastPoll: now() })
      }, 200 + i * 80))
    },
    async trustCertificate(id) { need('device.manage'); await wait(); const d = deviceOr404(id); log('device.trust_cert', d.name); return d.tlsFingerprint },
    async listGroups() {
      need('device.view')
      return groups.map((g) => ({ ...g, deviceCount: devices.filter((d) => d.groupId === g.id).length }))
    },
    async createGroup(name) {
      need('device.manage'); await wait()
      if (groups.some((g) => g.name === name)) throw new Error(`分组“${name}”已存在`)
      const g = { id: groups.length + 10, name, deviceCount: 0 }; groups.push(g); return g
    },
    async renameGroup(id, name) { need('device.manage'); await wait(); groups.find((g) => g.id === id)!.name = name },
    async deleteGroup(id) {
      need('device.manage'); await wait()
      groups.splice(groups.findIndex((g) => g.id === id), 1)
      devices.filter((d) => d.groupId === id).forEach((d) => (d.groupId = 0))
    },
  },
  credentials: {
    async list() { need('device.view'); await wait(); return creds.map((c) => ({ ...c, deviceCount: devices.filter((d) => d.credentialId === c.id).length })) },
    async create(i) {
      need('device.manage'); await wait()
      if (creds.some((c) => c.name === i.name)) throw new Error(`凭据名称“${i.name}”已存在`)
      creds.push({ id: creds.length + 10, name: i.name, username: i.username, note: i.note, deviceCount: 0, updatedAt: now() }); log('credential.create', i.name)
    },
    async update(i) { need('device.manage'); await wait(); Object.assign(creds.find((c) => c.id === i.id)!, { name: i.name, username: i.username, note: i.note, updatedAt: now() }) },
    async remove(id) {
      need('device.manage'); await wait()
      if (devices.some((d) => d.credentialId === id)) throw new Error('仍有设备在使用此凭据，请先为这些设备更换凭据')
      creds.splice(creds.findIndex((c) => c.id === id), 1)
    },
    async reveal(id) { need('credential.reveal'); await wait(); log('credential.reveal', creds.find((c) => c.id === id)!.name); return 'Mdbmc@2026' },
  },
  bmc: {
    async power(ids, action) {
      need('power.control')
      const batchId = Math.random().toString(16).slice(2, 10)
      const items = await Promise.all(ids.map(async (id, i) => {
        await sleep(400 + i * 120 + Math.random() * 500)
        const d = deviceOr404(id)
        const ok = d.reachable
        if (ok) d.powerState = action === 'on' || action === 'reset' || action === 'cycle' ? 'on' : 'off'
        const item = { batchId, deviceId: id, name: d.name, ok, error: ok ? '' : d.lastError, via: ok ? 'ipmi' : '' }
        emit('batch:item', item)
        if (ok) emit('device:status', { id, powerState: d.powerState, health: d.health, reachable: true, lastError: '', checkedAt: now() })
        log('power.' + action, `${d.name} (${d.host})`, ok, '', current?.username)
        return item
      }))
      return { batchId, total: ids.length, succeeded: items.filter((x) => x.ok).length, failed: items.filter((x) => !x.ok).length, items }
    },
    async identifyLED() { need('power.control'); await wait() },
    async sensors(id) { need('device.view'); await sleep(700); const d = deviceOr404(id); reachableOrThrow(d); return { sensors: sensorsFor(d), via: 'ipmi', fetchedAt: now() } },
    async sel(id) { need('device.view'); await sleep(600); const d = deviceOr404(id); reachableOrThrow(d); return { entries: [...selFor(d)], via: 'ipmi', fetchedAt: now() } },
    async clearSEL(id) { need('sel.clear'); await wait(); selByDevice.set(id, []); log('sel.clear', deviceOr404(id).name) },
    async inventory(id, refresh) {
      need('device.view')
      const d = deviceOr404(id)
      const cached = inventoryCache.get(id)
      if (cached && !refresh) return { inventory: cached.inv, updatedAt: cached.ts, cached: true }
      await sleep(1100); reachableOrThrow(d)
      const inv = inventoryFor(d); inventoryCache.set(id, { inv, ts: now() })
      return { inventory: inv, updatedAt: now(), cached: false }
    },
    async openWeb(id) {
      need('console.web'); await wait()
      const d = deviceOr404(id); const auto = current!.permissions.includes('console.autologin')
      log('console.web', d.name, true, auto ? '自动登录' : '仅打开登录页')
      return { url: `https://${d.host}/`, autoLogin: auto, hint: '' }
    },
    async openKVM(id) {
      need('console.kvm'); await wait()
      const d = deviceOr404(id); log('console.kvm', d.name)
      return d.vendor === 'supermicro'
        ? { url: `https://${d.host}/cgi/url_redirect.cgi?url_name=man_ikvm_html5_auto`, autoLogin: false, hint: '' }
        : { url: `https://${d.host}/`, autoLogin: true, hint: '已打开 BMC Web 界面，请在其中启动远程控制台（该品牌的一键 KVM 尚未适配）' }
    },
    async overview() {
      need('device.view'); await wait()
      const ds = devices.map(decorate)
      const count = (f: (d: Device) => boolean) => ds.filter(f).length
      const tally = (key: (d: Device) => string, label: (k: string) => string) => {
        const m = new Map<string, number>(); ds.forEach((d) => m.set(key(d), (m.get(key(d)) ?? 0) + 1))
        return [...m].map(([k, n]) => ({ key: k, label: label(k), count: n })).sort((a, b) => b.count - a.count)
      }
      return {
        total: ds.length, reachable: count((d) => d.reachable), unreachable: count((d) => !d.reachable && d.checkedAt > 0),
        poweredOn: count((d) => d.powerState === 'on'), poweredOff: count((d) => d.powerState === 'off'),
        healthOk: count((d) => d.health === 'ok'), warning: count((d) => d.health === 'warning'), critical: count((d) => d.health === 'critical'), unknown: count((d) => d.health === 'unknown'),
        byVendor: tally((d) => d.vendor || 'unknown', (k) => vendorNames[k] ?? '未识别'),
        byGroup: tally((d) => d.groupName || '未分组', (k) => k),
        problems: ds.filter((d) => !d.reachable || d.health === 'warning' || d.health === 'critical'),
        recent: current?.permissions.includes('audit.view') ? audit.slice(0, 8) : [],
        lastPoll: now() - 40, polling: false,
      }
    },
  },
  discovery: {
    async startScan(req) {
      need('device.manage')
      const total = req.targets.includes('/') ? 254 : 20
      const scanId = req.scanId
      let done = 0; let found = 0
      const timer = setInterval(() => {
        done = Math.min(total, done + 9)
        if (found < 5 && Math.random() < 0.35) {
          found++
          const host = `10.10.9.${10 + found}`
          const vendor = ['supermicro', 'supermicro', 'dell', 'lenovo', 'generic'][found - 1]
          emit('scan:found', { scanId, host, ipmi: true, ipmiVersion: '2.0', redfish: vendor !== 'generic', redfishVersion: vendor !== 'generic' ? '1.11.0' : '', vendor, vendorName: vendorNames[vendor], product: vendor === 'supermicro' ? 'SYS-120U-TNR' : '', tlsFingerprint: 'ab'.repeat(32), managedId: 0 })
        }
        emit('scan:progress', { scanId, done, total, found, finished: done >= total, cancelled: false })
        if (done >= total) clearInterval(timer)
      }, 120)
      ;(mockApi.discovery as any)._timer = timer
      return { scanId, total }
    },
    async cancelScan() { clearInterval((mockApi.discovery as any)._timer) },
    async addDiscovered(req) {
      need('device.manage'); await wait()
      for (const h of req.hosts) {
        devices.push({ ...devices[0], id: nextDeviceId++, name: h.name || h.host, host: h.host, groupId: req.groupId, credentialId: req.credentialId, vendor: h.vendor, powerState: 'on', health: 'ok', reachable: true })
      }
      return { added: req.hosts.length, skipped: [] }
    },
  },
  on(name, cb) {
    if (!listeners.has(name)) listeners.set(name, new Set())
    listeners.get(name)!.add(cb as any)
    return () => listeners.get(name)!.delete(cb as any)
  },
}
