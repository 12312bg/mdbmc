// Real backend: thin wrappers over the generated Wails bindings.
import * as Auth from '@/wailsjs/go/app/AuthService'
import * as Admin from '@/wailsjs/go/app/AdminService'
import * as Dev from '@/wailsjs/go/app/DeviceService'
import * as Cred from '@/wailsjs/go/app/CredentialService'
import * as BMC from '@/wailsjs/go/app/BMCService'
import * as Disc from '@/wailsjs/go/app/DiscoveryService'
import * as Upd from '@/wailsjs/go/app/UpdateService'
import { BrowserOpenURL, EventsOn } from '@/wailsjs/runtime/runtime'
import type { Api } from './types'

// The generated model classes and our interfaces describe the same JSON;
// cast at this boundary only.
const c = <T>(p: Promise<unknown>) => p as Promise<T>
const a = (v: unknown) => v as any

export const wailsApi: Api = {
  isMock: false,
  auth: {
    status: () => c(Auth.Status()),
    setup: (u, d, p) => c(Auth.Setup(u, d, p)),
    login: (u, p) => c(Auth.Login(u, p)),
    logout: () => Auth.Logout(),
    lock: () => Auth.Lock(),
    changePassword: (o, n) => Auth.ChangePassword(o, n),
  },
  admin: {
    roles: () => c(Admin.Roles()),
    listUsers: () => c(Admin.ListUsers()),
    createUser: (i) => Admin.CreateUser(a(i)),
    updateUser: (i) => Admin.UpdateUser(a(i)),
    resetPassword: (id, p) => Admin.ResetPassword(id, p),
    deleteUser: (id) => Admin.DeleteUser(id),
    getSettings: () => c(Admin.GetSettings()),
    saveSettings: (s) => Admin.SaveSettings(a(s)),
    listAudit: (q) => c(Admin.ListAudit(a(q))),
  },
  devices: {
    list: () => c(Dev.List()),
    get: (id) => c(Dev.Get(id)),
    vendors: () => c(Dev.Vendors()),
    create: (i) => c(Dev.Create(a(i))),
    update: (i) => c(Dev.Update(a(i))),
    remove: (ids) => Dev.Delete(ids),
    moveToGroup: (ids, g) => Dev.MoveToGroup(ids, g),
    setCredential: (ids, cid) => Dev.SetCredential(ids, cid),
    revealPassword: (id) => Dev.RevealPassword(id),
    identify: (id) => c(Dev.Identify(id)),
    refreshStatus: (ids) => Dev.RefreshStatus(ids),
    trustCertificate: (id) => Dev.TrustCertificate(id),
    listGroups: () => c(Dev.ListGroups()),
    createGroup: (n) => c(Dev.CreateGroup(n)),
    renameGroup: (id, n) => Dev.RenameGroup(id, n),
    deleteGroup: (id) => Dev.DeleteGroup(id),
  },
  credentials: {
    list: () => c(Cred.List()),
    create: (i) => Cred.Create(a(i)),
    update: (i) => Cred.Update(a(i)),
    remove: (id) => Cred.Delete(id),
    reveal: (id) => Cred.Reveal(id),
  },
  bmc: {
    power: (ids, act) => c(BMC.Power(ids, act)),
    identifyLED: (id, on) => BMC.IdentifyLED(id, on),
    sensors: (id) => c(BMC.Sensors(id)),
    sel: (id) => c(BMC.SEL(id)),
    clearSEL: (id) => BMC.ClearSEL(id),
    inventory: (id, r) => c(BMC.Inventory(id, r)),
    openWeb: (id) => c(BMC.OpenWeb(id)),
    openKVM: (id) => c(BMC.OpenKVM(id)),
    overview: () => c(BMC.Overview()),
  },
  discovery: {
    startScan: (r) => c(Disc.StartScan(a(r))),
    cancelScan: () => Disc.CancelScan(),
    addDiscovered: (r) => c(Disc.AddDiscovered(a(r))),
  },
  update: {
    status: () => c(Upd.Status()),
    check: () => c(Upd.Check()),
    download: () => Upd.Download(),
    cancelDownload: () => Upd.CancelDownload(),
    install: () => Upd.Install(),
    dismissResult: () => Upd.DismissResult(),
    getConfig: () => c(Upd.GetConfig()),
    saveConfig: (cfg) => Upd.SaveConfig(a(cfg)),
  },
  openExternal: (url) => BrowserOpenURL(url),
  on: (name, cb) => EventsOn(name, cb as (...data: any) => void),
}
