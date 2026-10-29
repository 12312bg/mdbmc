// Types mirror the Go DTOs in internal/app/dto.go and internal/bmc.

export type Role = 'admin' | 'operator' | 'viewer'
export type PowerState = 'on' | 'off' | 'unknown'
export type Health = 'ok' | 'warning' | 'critical' | 'unknown'
export type Protocol = 'auto' | 'ipmi' | 'redfish'
export type PowerAction = 'on' | 'off' | 'soft' | 'reset' | 'cycle'

export type Permission =
  | 'device.view'
  | 'device.manage'
  | 'credential.reveal'
  | 'power.control'
  | 'sel.clear'
  | 'console.web'
  | 'console.autologin'
  | 'console.kvm'
  | 'user.manage'
  | 'settings.manage'
  | 'audit.view'
  | 'system.update'

export interface UserInfo {
  id: number
  username: string
  displayName: string
  role: Role
  roleLabel: string
  permissions: Permission[]
  mustChangePassword: boolean
}

export interface AuthStatus {
  initialized: boolean
  user: UserInfo | null
  version: string
  dataDir: string
}

export interface UserDTO {
  id: number
  username: string
  displayName: string
  role: Role
  roleLabel: string
  disabled: boolean
  mustChangePassword: boolean
  createdAt: number
  lastLoginAt: number
}

export interface UserInput {
  id: number
  username: string
  displayName: string
  role: Role
  password: string
  disabled: boolean
}

export interface RoleOption {
  value: Role
  label: string
  permissions: Permission[]
}

export interface Settings {
  pollIntervalSec: number
  maxConcurrency: number
  ipmiTimeoutSec: number
  redfishTimeoutSec: number
  idleLockMinutes: number
  scanConcurrency: number
}

export interface AuditQuery {
  keyword: string
  since: number
  until: number
  page: number
  size: number
}

export interface AuditEntry {
  id: number
  ts: number
  username: string
  action: string
  target: string
  success: boolean
  detail: string
}

export interface AuditPage {
  items: AuditEntry[]
  total: number
}

export interface Device {
  id: number
  name: string
  host: string
  ipmiPort: number
  httpsPort: number
  protocol: Protocol
  groupId: number
  groupName: string
  credentialId: number
  credentialName: string
  username: string
  hasPassword: boolean
  note: string
  vendor: string
  vendorName: string
  manufacturer: string
  model: string
  serial: string
  bmcFirmware: string
  biosVersion: string
  hasIpmi: boolean
  hasRedfish: boolean
  tlsFingerprint: string
  powerState: PowerState
  health: Health
  reachable: boolean
  lastError: string
  checkedAt: number
  identifiedAt: number
  createdAt: number
}

export interface DeviceInput {
  id: number
  name: string
  host: string
  ipmiPort: number
  httpsPort: number
  protocol: Protocol
  groupId: number
  credentialId: number
  username: string
  password: string
  note: string
}

export interface DeviceStatusEvent {
  id: number
  powerState: PowerState
  health: Health
  reachable: boolean
  lastError: string
  checkedAt: number
}

export interface Group {
  id: number
  name: string
  deviceCount: number
}

export interface Credential {
  id: number
  name: string
  username: string
  note: string
  deviceCount: number
  updatedAt: number
}

export interface CredentialInput {
  id: number
  name: string
  username: string
  password: string
  note: string
}

export interface VendorOption {
  id: string
  name: string
}

export interface BatchItem {
  batchId: string
  deviceId: number
  name: string
  ok: boolean
  error: string
  via: string
}

export interface BatchResult {
  batchId: string
  total: number
  succeeded: number
  failed: number
  items: BatchItem[]
}

export type SensorStatus = 'ok' | 'warning' | 'critical' | 'unknown' | 'absent'

export interface Sensor {
  name: string
  category: 'temperature' | 'fan' | 'voltage' | 'current' | 'power' | 'other'
  type: string
  reading: number | null
  unit: string
  status: SensorStatus
  text: string
  lowerCritical: number | null
  lowerWarning: number | null
  upperWarning: number | null
  upperCritical: number | null
  source: string
}

export interface SensorsResult {
  sensors: Sensor[]
  via: string
  fetchedAt: number
}

export interface SELEntry {
  id: string
  time: number
  sensor: string
  sensorType: string
  message: string
  severity: 'info' | 'ok' | 'warning' | 'critical'
  direction: 'assert' | 'deassert' | ''
  source: string
}

export interface SELResult {
  entries: SELEntry[]
  via: string
  fetchedAt: number
}

export interface Inventory {
  system: {
    manufacturer: string
    model: string
    serial: string
    sku: string
    partNumber: string
    uuid: string
    hostName: string
    biosVersion: string
    powerState: string
    health: string
    processorModel: string
    processorCount: number
    coreCount: number
    threadCount: number
    memoryGiB: number
  }
  manager: { model: string; firmwareVersion: string; macAddress: string; ipv4: string }
  processors: { socket: string; model: string; manufacturer: string; cores: number; threads: number; maxSpeedMHz: number; health: string }[] | null
  memory: { slot: string; capacityMiB: number; type: string; speedMHz: number; manufacturer: string; partNumber: string; serial: string; health: string }[] | null
  drives: { name: string; model: string; serial: string; capacityBytes: number; mediaType: string; protocol: string; firmware: string; health: string }[] | null
  nics: { name: string; macAddress: string; speedMbps: number; linkStatus: string; ipv4: string[] | null; health: string }[] | null
  powerSupplies: { name: string; model: string; serial: string; capacityWatts: number; firmware: string; health: string }[] | null
  firmware: { name: string; version: string; updateable: boolean }[] | null
  fru: {
    device: string
    chassisPart: string
    chassisSerial: string
    boardMfg: string
    boardProduct: string
    boardSerial: string
    boardPart: string
    productMfg: string
    productName: string
    productPart: string
    productVersion: string
    productSerial: string
  }[] | null
  source: string
  collectedAt: number
  /** Why a section could not be read, keyed by section (processors, memory, …, fru). */
  errors?: Record<string, string> | null
}

export interface InventoryResult {
  inventory: Inventory
  updatedAt: number
  cached: boolean
}

export interface ConsoleResult {
  url: string
  autoLogin: boolean
  hint: string
}

export interface ScanRequest {
  scanId: string
  targets: string
  ipmiPort: number
  httpsPort: number
}

export interface ScanStarted {
  scanId: string
  total: number
}

export interface ScanFound {
  scanId: string
  host: string
  ipmi: boolean
  ipmiVersion: string
  redfish: boolean
  redfishVersion: string
  vendor: string
  vendorName: string
  product: string
  tlsFingerprint: string
  managedId: number
}

export interface ScanProgress {
  scanId: string
  done: number
  total: number
  found: number
  finished: boolean
  cancelled: boolean
}

export interface DiscoveredHost {
  host: string
  name: string
  ipmi: boolean
  redfish: boolean
  vendor: string
  tlsFingerprint: string
}

export interface AddDiscoveredRequest {
  hosts: DiscoveredHost[]
  ipmiPort: number
  httpsPort: number
  groupId: number
  credentialId: number
  username: string
  password: string
}

export interface AddDiscoveredResult {
  added: number
  skipped: string[]
}

export interface CountItem {
  key: string
  label: string
  count: number
}

export interface Overview {
  total: number
  reachable: number
  unreachable: number
  poweredOn: number
  poweredOff: number
  healthOk: number
  warning: number
  critical: number
  unknown: number
  byVendor: CountItem[] | null
  byGroup: CountItem[] | null
  problems: Device[]
  recent: AuditEntry[]
  lastPoll: number
  polling: boolean
}

export type UpdateState = 'idle' | 'checking' | 'upToDate' | 'available' | 'downloading' | 'ready' | 'installing'
/** installer: installed with the setup program; portable: unpacked zip; unsupported: not Windows. */
export type UpdateMode = 'installer' | 'portable' | 'unsupported'

export interface UpdateRelease {
  version: string
  tag: string
  publishedAt: number
  /** Markdown */
  notes: string
  /** Package size for this install mode; 0 when the release has none. */
  size: number
  url: string
}

export interface UpdateResult {
  from: string
  version: string
  success: boolean
  error: string
  time: number
}

export interface UpdateStatus {
  currentVersion: string
  state: UpdateState
  mode: UpdateMode
  canInstall: boolean
  latest: UpdateRelease | null
  downloaded: number
  total: number
  speed: number
  error: string
  checkedAt: number
  lastResult: UpdateResult | null
}

export interface UpdateConfig {
  checkHours: number
  source: string
  proxy: string
  defaultSource: string
}

export interface EventMap {
  'device:status': DeviceStatusEvent
  'device:updated': number
  'batch:item': BatchItem
  'scan:progress': ScanProgress
  'scan:found': ScanFound
  'poll:state': { running: boolean; lastPoll?: number }
  'update:state': UpdateStatus
}

export interface Api {
  isMock: boolean
  auth: {
    status(): Promise<AuthStatus>
    setup(username: string, displayName: string, password: string): Promise<UserInfo>
    login(username: string, password: string): Promise<UserInfo>
    /** Ends the session and closes the console windows. */
    logout(): Promise<void>
    /** Idle lock: ends the session but keeps the console windows open. */
    lock(): Promise<void>
    changePassword(oldPassword: string, newPassword: string): Promise<void>
  }
  admin: {
    roles(): Promise<RoleOption[]>
    listUsers(): Promise<UserDTO[]>
    createUser(input: UserInput): Promise<void>
    updateUser(input: UserInput): Promise<void>
    resetPassword(id: number, password: string): Promise<void>
    deleteUser(id: number): Promise<void>
    getSettings(): Promise<Settings>
    saveSettings(s: Settings): Promise<void>
    listAudit(q: AuditQuery): Promise<AuditPage>
  }
  devices: {
    list(): Promise<Device[]>
    get(id: number): Promise<Device>
    vendors(): Promise<VendorOption[]>
    create(input: DeviceInput): Promise<Device>
    update(input: DeviceInput): Promise<Device>
    remove(ids: number[]): Promise<number>
    moveToGroup(ids: number[], groupId: number): Promise<void>
    setCredential(ids: number[], credentialId: number): Promise<void>
    revealPassword(id: number): Promise<string>
    identify(id: number): Promise<Device>
    refreshStatus(ids: number[]): Promise<void>
    trustCertificate(id: number): Promise<string>
    listGroups(): Promise<Group[]>
    createGroup(name: string): Promise<Group>
    renameGroup(id: number, name: string): Promise<void>
    deleteGroup(id: number): Promise<void>
  }
  credentials: {
    list(): Promise<Credential[]>
    create(input: CredentialInput): Promise<void>
    update(input: CredentialInput): Promise<void>
    remove(id: number): Promise<void>
    reveal(id: number): Promise<string>
  }
  bmc: {
    power(ids: number[], action: PowerAction): Promise<BatchResult>
    identifyLED(id: number, on: boolean): Promise<void>
    sensors(id: number): Promise<SensorsResult>
    sel(id: number): Promise<SELResult>
    clearSEL(id: number): Promise<void>
    inventory(id: number, refresh: boolean): Promise<InventoryResult>
    openWeb(id: number): Promise<ConsoleResult>
    openKVM(id: number): Promise<ConsoleResult>
    overview(): Promise<Overview>
  }
  discovery: {
    startScan(req: ScanRequest): Promise<ScanStarted>
    cancelScan(): Promise<void>
    addDiscovered(req: AddDiscoveredRequest): Promise<AddDiscoveredResult>
  }
  update: {
    status(): Promise<UpdateStatus>
    check(): Promise<UpdateStatus>
    download(): Promise<void>
    cancelDownload(): Promise<void>
    /** Exits the program; the update helper installs and restarts it. */
    install(): Promise<void>
    dismissResult(): Promise<void>
    getConfig(): Promise<UpdateConfig>
    saveConfig(c: UpdateConfig): Promise<void>
  }
  /** Opens a web page in the system browser. */
  openExternal(url: string): void
  on<K extends keyof EventMap>(name: K, cb: (data: EventMap[K]) => void): () => void
}
