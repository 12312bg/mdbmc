export namespace app {
	
	export class DiscoveredHost {
	    host: string;
	    name: string;
	    ipmi: boolean;
	    redfish: boolean;
	    vendor: string;
	    tlsFingerprint: string;
	    group: string;
	
	    static createFrom(source: any = {}) {
	        return new DiscoveredHost(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.name = source["name"];
	        this.ipmi = source["ipmi"];
	        this.redfish = source["redfish"];
	        this.vendor = source["vendor"];
	        this.tlsFingerprint = source["tlsFingerprint"];
	        this.group = source["group"];
	    }
	}
	export class AddDiscoveredRequest {
	    hosts: DiscoveredHost[];
	    ipmiPort: number;
	    httpsPort: number;
	    groupId: number;
	    credentialId: number;
	    username: string;
	    password: string;
	
	    static createFrom(source: any = {}) {
	        return new AddDiscoveredRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hosts = this.convertValues(source["hosts"], DiscoveredHost);
	        this.ipmiPort = source["ipmiPort"];
	        this.httpsPort = source["httpsPort"];
	        this.groupId = source["groupId"];
	        this.credentialId = source["credentialId"];
	        this.username = source["username"];
	        this.password = source["password"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AddDiscoveredResult {
	    added: number;
	    skipped: string[];
	
	    static createFrom(source: any = {}) {
	        return new AddDiscoveredResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.added = source["added"];
	        this.skipped = source["skipped"];
	    }
	}
	export class AlertConfig {
	    offline: boolean;
	    health: boolean;
	    powerLoss: boolean;
	    historyMax: number;
	    webhookUrl: string;
	    webhookSecret: string;
	    lastWebhook: string;
	
	    static createFrom(source: any = {}) {
	        return new AlertConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.offline = source["offline"];
	        this.health = source["health"];
	        this.powerLoss = source["powerLoss"];
	        this.historyMax = source["historyMax"];
	        this.webhookUrl = source["webhookUrl"];
	        this.webhookSecret = source["webhookSecret"];
	        this.lastWebhook = source["lastWebhook"];
	    }
	}
	export class AlertDTO {
	    id: number;
	    ts: number;
	    deviceId: number;
	    deviceName: string;
	    host: string;
	    kind: string;
	    severity: string;
	    title: string;
	    detail: string;
	    unread: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AlertDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.ts = source["ts"];
	        this.deviceId = source["deviceId"];
	        this.deviceName = source["deviceName"];
	        this.host = source["host"];
	        this.kind = source["kind"];
	        this.severity = source["severity"];
	        this.title = source["title"];
	        this.detail = source["detail"];
	        this.unread = source["unread"];
	    }
	}
	export class AlertInbox {
	    unread: number;
	    popup: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AlertInbox(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.unread = source["unread"];
	        this.popup = source["popup"];
	    }
	}
	export class AlertPage {
	    items: AlertDTO[];
	    total: number;
	    unread: number;
	
	    static createFrom(source: any = {}) {
	        return new AlertPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], AlertDTO);
	        this.total = source["total"];
	        this.unread = source["unread"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AlertQuery {
	    severity: string;
	    keyword: string;
	    page: number;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new AlertQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.severity = source["severity"];
	        this.keyword = source["keyword"];
	        this.page = source["page"];
	        this.size = source["size"];
	    }
	}
	export class BackupFile {
	    token: string;
	    file: string;
	    size: number;
	    modTime: number;
	    users: number;
	    devices: number;
	
	    static createFrom(source: any = {}) {
	        return new BackupFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token = source["token"];
	        this.file = source["file"];
	        this.size = source["size"];
	        this.modTime = source["modTime"];
	        this.users = source["users"];
	        this.devices = source["devices"];
	    }
	}
	export class ImportRow {
	    row: number;
	    name: string;
	    host: string;
	    ipmiPort: number;
	    httpsPort: number;
	    protocol: string;
	    group: string;
	    credential: string;
	    username: string;
	    hasPassword: boolean;
	    note: string;
	    status: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.row = source["row"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.ipmiPort = source["ipmiPort"];
	        this.httpsPort = source["httpsPort"];
	        this.protocol = source["protocol"];
	        this.group = source["group"];
	        this.credential = source["credential"];
	        this.username = source["username"];
	        this.hasPassword = source["hasPassword"];
	        this.note = source["note"];
	        this.status = source["status"];
	        this.message = source["message"];
	    }
	}
	export class ImportPreview {
	    token: string;
	    file: string;
	    rows: ImportRow[];
	    ok: number;
	    skip: number;
	    errors: number;
	    newGroups: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token = source["token"];
	        this.file = source["file"];
	        this.rows = this.convertValues(source["rows"], ImportRow);
	        this.ok = source["ok"];
	        this.skip = source["skip"];
	        this.errors = source["errors"];
	        this.newGroups = source["newGroups"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportResult {
	    added: number;
	    failed: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.added = source["added"];
	        this.failed = source["failed"];
	    }
	}
	export class RestoreResult {
	    restarting: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RestoreResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.restarting = source["restarting"];
	    }
	}
	export class AuditEntry {
	    id: number;
	    ts: number;
	    username: string;
	    action: string;
	    target: string;
	    success: boolean;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new AuditEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.ts = source["ts"];
	        this.username = source["username"];
	        this.action = source["action"];
	        this.target = source["target"];
	        this.success = source["success"];
	        this.detail = source["detail"];
	    }
	}
	export class AuditPage {
	    items: AuditEntry[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new AuditPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], AuditEntry);
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AuditQuery {
	    keyword: string;
	    since: number;
	    until: number;
	    page: number;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new AuditQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.keyword = source["keyword"];
	        this.since = source["since"];
	        this.until = source["until"];
	        this.page = source["page"];
	        this.size = source["size"];
	    }
	}
	export class UserInfo {
	    id: number;
	    username: string;
	    displayName: string;
	    role: string;
	    roleLabel: string;
	    permissions: string[];
	    mustChangePassword: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UserInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.username = source["username"];
	        this.displayName = source["displayName"];
	        this.role = source["role"];
	        this.roleLabel = source["roleLabel"];
	        this.permissions = source["permissions"];
	        this.mustChangePassword = source["mustChangePassword"];
	    }
	}
	export class AuthStatus {
	    initialized: boolean;
	    user?: UserInfo;
	    version: string;
	    dataDir: string;
	
	    static createFrom(source: any = {}) {
	        return new AuthStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.initialized = source["initialized"];
	        this.user = this.convertValues(source["user"], UserInfo);
	        this.version = source["version"];
	        this.dataDir = source["dataDir"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BatchItem {
	    batchId: string;
	    deviceId: number;
	    name: string;
	    ok: boolean;
	    error: string;
	    via: string;
	
	    static createFrom(source: any = {}) {
	        return new BatchItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.batchId = source["batchId"];
	        this.deviceId = source["deviceId"];
	        this.name = source["name"];
	        this.ok = source["ok"];
	        this.error = source["error"];
	        this.via = source["via"];
	    }
	}
	export class BatchResult {
	    batchId: string;
	    total: number;
	    succeeded: number;
	    failed: number;
	    items: BatchItem[];
	
	    static createFrom(source: any = {}) {
	        return new BatchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.batchId = source["batchId"];
	        this.total = source["total"];
	        this.succeeded = source["succeeded"];
	        this.failed = source["failed"];
	        this.items = this.convertValues(source["items"], BatchItem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ConsoleResult {
	    url: string;
	    autoLogin: boolean;
	    hint: string;
	
	    static createFrom(source: any = {}) {
	        return new ConsoleResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.autoLogin = source["autoLogin"];
	        this.hint = source["hint"];
	    }
	}
	export class CountItem {
	    key: string;
	    label: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new CountItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.count = source["count"];
	    }
	}
	export class CredentialDTO {
	    id: number;
	    name: string;
	    username: string;
	    note: string;
	    deviceCount: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new CredentialDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.username = source["username"];
	        this.note = source["note"];
	        this.deviceCount = source["deviceCount"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class CredentialInput {
	    id: number;
	    name: string;
	    username: string;
	    password: string;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new CredentialInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.note = source["note"];
	    }
	}
	export class DeviceDTO {
	    id: number;
	    name: string;
	    host: string;
	    ipmiPort: number;
	    httpsPort: number;
	    protocol: string;
	    groupId: number;
	    groupName: string;
	    credentialId: number;
	    credentialName: string;
	    username: string;
	    hasPassword: boolean;
	    note: string;
	    vendor: string;
	    vendorName: string;
	    manufacturer: string;
	    model: string;
	    serial: string;
	    bmcFirmware: string;
	    biosVersion: string;
	    hasIpmi: boolean;
	    hasRedfish: boolean;
	    tlsFingerprint: string;
	    powerState: string;
	    health: string;
	    reachable: boolean;
	    lastError: string;
	    checkedAt: number;
	    identifiedAt: number;
	    createdAt: number;
	
	    static createFrom(source: any = {}) {
	        return new DeviceDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.ipmiPort = source["ipmiPort"];
	        this.httpsPort = source["httpsPort"];
	        this.protocol = source["protocol"];
	        this.groupId = source["groupId"];
	        this.groupName = source["groupName"];
	        this.credentialId = source["credentialId"];
	        this.credentialName = source["credentialName"];
	        this.username = source["username"];
	        this.hasPassword = source["hasPassword"];
	        this.note = source["note"];
	        this.vendor = source["vendor"];
	        this.vendorName = source["vendorName"];
	        this.manufacturer = source["manufacturer"];
	        this.model = source["model"];
	        this.serial = source["serial"];
	        this.bmcFirmware = source["bmcFirmware"];
	        this.biosVersion = source["biosVersion"];
	        this.hasIpmi = source["hasIpmi"];
	        this.hasRedfish = source["hasRedfish"];
	        this.tlsFingerprint = source["tlsFingerprint"];
	        this.powerState = source["powerState"];
	        this.health = source["health"];
	        this.reachable = source["reachable"];
	        this.lastError = source["lastError"];
	        this.checkedAt = source["checkedAt"];
	        this.identifiedAt = source["identifiedAt"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class DeviceInput {
	    id: number;
	    name: string;
	    host: string;
	    ipmiPort: number;
	    httpsPort: number;
	    protocol: string;
	    groupId: number;
	    credentialId: number;
	    username: string;
	    password: string;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new DeviceInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.ipmiPort = source["ipmiPort"];
	        this.httpsPort = source["httpsPort"];
	        this.protocol = source["protocol"];
	        this.groupId = source["groupId"];
	        this.credentialId = source["credentialId"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.note = source["note"];
	    }
	}
	
	export class GroupDTO {
	    id: number;
	    name: string;
	    deviceCount: number;
	
	    static createFrom(source: any = {}) {
	        return new GroupDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.deviceCount = source["deviceCount"];
	    }
	}
	export class InventoryResult {
	    inventory?: bmc.Inventory;
	    updatedAt: number;
	    cached: boolean;
	
	    static createFrom(source: any = {}) {
	        return new InventoryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inventory = this.convertValues(source["inventory"], bmc.Inventory);
	        this.updatedAt = source["updatedAt"];
	        this.cached = source["cached"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Overview {
	    total: number;
	    reachable: number;
	    unreachable: number;
	    poweredOn: number;
	    poweredOff: number;
	    healthOk: number;
	    warning: number;
	    critical: number;
	    unknown: number;
	    byVendor: CountItem[];
	    byGroup: CountItem[];
	    problems: DeviceDTO[];
	    recent: AuditEntry[];
	    lastPoll: number;
	    polling: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Overview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.reachable = source["reachable"];
	        this.unreachable = source["unreachable"];
	        this.poweredOn = source["poweredOn"];
	        this.poweredOff = source["poweredOff"];
	        this.healthOk = source["healthOk"];
	        this.warning = source["warning"];
	        this.critical = source["critical"];
	        this.unknown = source["unknown"];
	        this.byVendor = this.convertValues(source["byVendor"], CountItem);
	        this.byGroup = this.convertValues(source["byGroup"], CountItem);
	        this.problems = this.convertValues(source["problems"], DeviceDTO);
	        this.recent = this.convertValues(source["recent"], AuditEntry);
	        this.lastPoll = source["lastPoll"];
	        this.polling = source["polling"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RoleOption {
	    value: string;
	    label: string;
	    permissions: string[];
	
	    static createFrom(source: any = {}) {
	        return new RoleOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
	        this.permissions = source["permissions"];
	    }
	}
	export class SELResult {
	    entries: bmc.SELEntry[];
	    via: string;
	    fetchedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new SELResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entries = this.convertValues(source["entries"], bmc.SELEntry);
	        this.via = source["via"];
	        this.fetchedAt = source["fetchedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ScanRequest {
	    scanId: string;
	    targets: string;
	    ipmiPort: number;
	    httpsPort: number;
	
	    static createFrom(source: any = {}) {
	        return new ScanRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scanId = source["scanId"];
	        this.targets = source["targets"];
	        this.ipmiPort = source["ipmiPort"];
	        this.httpsPort = source["httpsPort"];
	    }
	}
	export class ScanStarted {
	    scanId: string;
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new ScanStarted(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scanId = source["scanId"];
	        this.total = source["total"];
	    }
	}
	export class SensorsResult {
	    sensors: bmc.Sensor[];
	    via: string;
	    fetchedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new SensorsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sensors = this.convertValues(source["sensors"], bmc.Sensor);
	        this.via = source["via"];
	        this.fetchedAt = source["fetchedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Settings {
	    pollIntervalSec: number;
	    maxConcurrency: number;
	    ipmiTimeoutSec: number;
	    redfishTimeoutSec: number;
	    idleLockMinutes: number;
	    scanConcurrency: number;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pollIntervalSec = source["pollIntervalSec"];
	        this.maxConcurrency = source["maxConcurrency"];
	        this.ipmiTimeoutSec = source["ipmiTimeoutSec"];
	        this.redfishTimeoutSec = source["redfishTimeoutSec"];
	        this.idleLockMinutes = source["idleLockMinutes"];
	        this.scanConcurrency = source["scanConcurrency"];
	    }
	}
	export class UpdateConfig {
	    checkHours: number;
	    source: string;
	    proxy: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.checkHours = source["checkHours"];
	        this.source = source["source"];
	        this.proxy = source["proxy"];
	    }
	}
	export class UpdateRelease {
	    version: string;
	    tag: string;
	    publishedAt: number;
	    notes: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new UpdateRelease(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.tag = source["tag"];
	        this.publishedAt = source["publishedAt"];
	        this.notes = source["notes"];
	        this.size = source["size"];
	    }
	}
	export class UpdateResult {
	    from: string;
	    version: string;
	    success: boolean;
	    error: string;
	    time: number;
	
	    static createFrom(source: any = {}) {
	        return new UpdateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.from = source["from"];
	        this.version = source["version"];
	        this.success = source["success"];
	        this.error = source["error"];
	        this.time = source["time"];
	    }
	}
	export class UpdateStatus {
	    currentVersion: string;
	    state: string;
	    mode: string;
	    canInstall: boolean;
	    latest: UpdateRelease;
	    downloaded: number;
	    total: number;
	    speed: number;
	    error: string;
	    checkedAt: number;
	    lastResult: UpdateResult;
	
	    static createFrom(source: any = {}) {
	        return new UpdateStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currentVersion = source["currentVersion"];
	        this.state = source["state"];
	        this.mode = source["mode"];
	        this.canInstall = source["canInstall"];
	        this.latest = this.convertValues(source["latest"], UpdateRelease);
	        this.downloaded = source["downloaded"];
	        this.total = source["total"];
	        this.speed = source["speed"];
	        this.error = source["error"];
	        this.checkedAt = source["checkedAt"];
	        this.lastResult = this.convertValues(source["lastResult"], UpdateResult);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UserDTO {
	    id: number;
	    username: string;
	    displayName: string;
	    role: string;
	    roleLabel: string;
	    disabled: boolean;
	    mustChangePassword: boolean;
	    createdAt: number;
	    lastLoginAt: number;
	
	    static createFrom(source: any = {}) {
	        return new UserDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.username = source["username"];
	        this.displayName = source["displayName"];
	        this.role = source["role"];
	        this.roleLabel = source["roleLabel"];
	        this.disabled = source["disabled"];
	        this.mustChangePassword = source["mustChangePassword"];
	        this.createdAt = source["createdAt"];
	        this.lastLoginAt = source["lastLoginAt"];
	    }
	}
	
	export class UserInput {
	    id: number;
	    username: string;
	    displayName: string;
	    role: string;
	    password: string;
	    disabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UserInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.username = source["username"];
	        this.displayName = source["displayName"];
	        this.role = source["role"];
	        this.password = source["password"];
	        this.disabled = source["disabled"];
	    }
	}

}

export namespace bmc {
	
	export class Drive {
	    name: string;
	    model: string;
	    serial: string;
	    capacityBytes: number;
	    mediaType: string;
	    protocol: string;
	    firmware: string;
	    health: string;
	
	    static createFrom(source: any = {}) {
	        return new Drive(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.model = source["model"];
	        this.serial = source["serial"];
	        this.capacityBytes = source["capacityBytes"];
	        this.mediaType = source["mediaType"];
	        this.protocol = source["protocol"];
	        this.firmware = source["firmware"];
	        this.health = source["health"];
	    }
	}
	export class FRUInfo {
	    device: string;
	    chassisPart: string;
	    chassisSerial: string;
	    boardMfg: string;
	    boardProduct: string;
	    boardSerial: string;
	    boardPart: string;
	    productMfg: string;
	    productName: string;
	    productPart: string;
	    productVersion: string;
	    productSerial: string;
	
	    static createFrom(source: any = {}) {
	        return new FRUInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.device = source["device"];
	        this.chassisPart = source["chassisPart"];
	        this.chassisSerial = source["chassisSerial"];
	        this.boardMfg = source["boardMfg"];
	        this.boardProduct = source["boardProduct"];
	        this.boardSerial = source["boardSerial"];
	        this.boardPart = source["boardPart"];
	        this.productMfg = source["productMfg"];
	        this.productName = source["productName"];
	        this.productPart = source["productPart"];
	        this.productVersion = source["productVersion"];
	        this.productSerial = source["productSerial"];
	    }
	}
	export class Firmware {
	    name: string;
	    version: string;
	    updateable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Firmware(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.updateable = source["updateable"];
	    }
	}
	export class PowerSupply {
	    name: string;
	    model: string;
	    serial: string;
	    capacityWatts: number;
	    firmware: string;
	    health: string;
	
	    static createFrom(source: any = {}) {
	        return new PowerSupply(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.model = source["model"];
	        this.serial = source["serial"];
	        this.capacityWatts = source["capacityWatts"];
	        this.firmware = source["firmware"];
	        this.health = source["health"];
	    }
	}
	export class NIC {
	    name: string;
	    macAddress: string;
	    speedMbps: number;
	    linkStatus: string;
	    ipv4: string[];
	    health: string;
	
	    static createFrom(source: any = {}) {
	        return new NIC(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.macAddress = source["macAddress"];
	        this.speedMbps = source["speedMbps"];
	        this.linkStatus = source["linkStatus"];
	        this.ipv4 = source["ipv4"];
	        this.health = source["health"];
	    }
	}
	export class MemoryDIMM {
	    slot: string;
	    capacityMiB: number;
	    type: string;
	    speedMHz: number;
	    manufacturer: string;
	    partNumber: string;
	    serial: string;
	    health: string;
	
	    static createFrom(source: any = {}) {
	        return new MemoryDIMM(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.slot = source["slot"];
	        this.capacityMiB = source["capacityMiB"];
	        this.type = source["type"];
	        this.speedMHz = source["speedMHz"];
	        this.manufacturer = source["manufacturer"];
	        this.partNumber = source["partNumber"];
	        this.serial = source["serial"];
	        this.health = source["health"];
	    }
	}
	export class Processor {
	    socket: string;
	    model: string;
	    manufacturer: string;
	    cores: number;
	    threads: number;
	    maxSpeedMHz: number;
	    health: string;
	
	    static createFrom(source: any = {}) {
	        return new Processor(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.socket = source["socket"];
	        this.model = source["model"];
	        this.manufacturer = source["manufacturer"];
	        this.cores = source["cores"];
	        this.threads = source["threads"];
	        this.maxSpeedMHz = source["maxSpeedMHz"];
	        this.health = source["health"];
	    }
	}
	export class ManagerInfo {
	    model: string;
	    firmwareVersion: string;
	    macAddress: string;
	    ipv4: string;
	
	    static createFrom(source: any = {}) {
	        return new ManagerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.firmwareVersion = source["firmwareVersion"];
	        this.macAddress = source["macAddress"];
	        this.ipv4 = source["ipv4"];
	    }
	}
	export class SystemInfo {
	    manufacturer: string;
	    model: string;
	    serial: string;
	    sku: string;
	    partNumber: string;
	    uuid: string;
	    hostName: string;
	    biosVersion: string;
	    powerState: string;
	    health: string;
	    processorModel: string;
	    processorCount: number;
	    coreCount: number;
	    threadCount: number;
	    memoryGiB: number;
	
	    static createFrom(source: any = {}) {
	        return new SystemInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.manufacturer = source["manufacturer"];
	        this.model = source["model"];
	        this.serial = source["serial"];
	        this.sku = source["sku"];
	        this.partNumber = source["partNumber"];
	        this.uuid = source["uuid"];
	        this.hostName = source["hostName"];
	        this.biosVersion = source["biosVersion"];
	        this.powerState = source["powerState"];
	        this.health = source["health"];
	        this.processorModel = source["processorModel"];
	        this.processorCount = source["processorCount"];
	        this.coreCount = source["coreCount"];
	        this.threadCount = source["threadCount"];
	        this.memoryGiB = source["memoryGiB"];
	    }
	}
	export class Inventory {
	    system: SystemInfo;
	    manager: ManagerInfo;
	    processors: Processor[];
	    memory: MemoryDIMM[];
	    drives: Drive[];
	    nics: NIC[];
	    powerSupplies: PowerSupply[];
	    firmware: Firmware[];
	    fru: FRUInfo[];
	    source: string;
	    collectedAt: number;
	    errors?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new Inventory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.system = this.convertValues(source["system"], SystemInfo);
	        this.manager = this.convertValues(source["manager"], ManagerInfo);
	        this.processors = this.convertValues(source["processors"], Processor);
	        this.memory = this.convertValues(source["memory"], MemoryDIMM);
	        this.drives = this.convertValues(source["drives"], Drive);
	        this.nics = this.convertValues(source["nics"], NIC);
	        this.powerSupplies = this.convertValues(source["powerSupplies"], PowerSupply);
	        this.firmware = this.convertValues(source["firmware"], Firmware);
	        this.fru = this.convertValues(source["fru"], FRUInfo);
	        this.source = source["source"];
	        this.collectedAt = source["collectedAt"];
	        this.errors = source["errors"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	export class SELEntry {
	    id: string;
	    time: number;
	    sensor: string;
	    sensorType: string;
	    message: string;
	    severity: string;
	    direction: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new SELEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.time = source["time"];
	        this.sensor = source["sensor"];
	        this.sensorType = source["sensorType"];
	        this.message = source["message"];
	        this.severity = source["severity"];
	        this.direction = source["direction"];
	        this.source = source["source"];
	    }
	}
	export class Sensor {
	    name: string;
	    category: string;
	    type: string;
	    reading?: number;
	    unit: string;
	    status: string;
	    text: string;
	    lowerCritical?: number;
	    lowerWarning?: number;
	    upperWarning?: number;
	    upperCritical?: number;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new Sensor(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.category = source["category"];
	        this.type = source["type"];
	        this.reading = source["reading"];
	        this.unit = source["unit"];
	        this.status = source["status"];
	        this.text = source["text"];
	        this.lowerCritical = source["lowerCritical"];
	        this.lowerWarning = source["lowerWarning"];
	        this.upperWarning = source["upperWarning"];
	        this.upperCritical = source["upperCritical"];
	        this.source = source["source"];
	    }
	}
	
	export class VendorOption {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new VendorOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}

}

