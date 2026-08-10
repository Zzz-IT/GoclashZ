export namespace appcore {
	
	export class AppBehavior {
	    silentStart: boolean;
	    closeToTray: boolean;
	    colorDelay: boolean;
	    delayRetention: boolean;
	    delayRetentionTime: string;
	    logLevel: string;
	    appLogLevel: string;
	    hideLogs: boolean;
	    subUA: string;
	    activeConfig: string;
	    activeMode: string;
	    geoIpLink: string;
	    geoSiteLink: string;
	    mmdbLink: string;
	    asnLink: string;
	    autoUpdate: boolean;
	    updateMethod: string;
	    updateInterval: number;
	    lastUpdateCheck: number;
	    autoDelayTest: boolean;
	    autoDelayTestInterval: number;
	    proxyTrafficOnly: boolean;
	    startupWithOS: boolean;
	    restoreOnStartup: boolean;
	    portProxyMode: boolean;
	    longConnectionProtection: boolean;
	    deferRestartWhenActive: boolean;
	    longConnectionMinSeconds: number;
	
	    static createFrom(source: any = {}) {
	        return new AppBehavior(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.silentStart = source["silentStart"];
	        this.closeToTray = source["closeToTray"];
	        this.colorDelay = source["colorDelay"];
	        this.delayRetention = source["delayRetention"];
	        this.delayRetentionTime = source["delayRetentionTime"];
	        this.logLevel = source["logLevel"];
	        this.appLogLevel = source["appLogLevel"];
	        this.hideLogs = source["hideLogs"];
	        this.subUA = source["subUA"];
	        this.activeConfig = source["activeConfig"];
	        this.activeMode = source["activeMode"];
	        this.geoIpLink = source["geoIpLink"];
	        this.geoSiteLink = source["geoSiteLink"];
	        this.mmdbLink = source["mmdbLink"];
	        this.asnLink = source["asnLink"];
	        this.autoUpdate = source["autoUpdate"];
	        this.updateMethod = source["updateMethod"];
	        this.updateInterval = source["updateInterval"];
	        this.lastUpdateCheck = source["lastUpdateCheck"];
	        this.autoDelayTest = source["autoDelayTest"];
	        this.autoDelayTestInterval = source["autoDelayTestInterval"];
	        this.proxyTrafficOnly = source["proxyTrafficOnly"];
	        this.startupWithOS = source["startupWithOS"];
	        this.restoreOnStartup = source["restoreOnStartup"];
	        this.portProxyMode = source["portProxyMode"];
	        this.longConnectionProtection = source["longConnectionProtection"];
	        this.deferRestartWhenActive = source["deferRestartWhenActive"];
	        this.longConnectionMinSeconds = source["longConnectionMinSeconds"];
	    }
	}
	export class AppState {
	    isRunning: boolean;
	    isAdmin: boolean;
	    mode: string;
	    theme: string;
	    hideLogs: boolean;
	    logLevel: string;
	    appLogLevel: string;
	    desiredSystemProxy: boolean;
	    desiredTun: boolean;
	    desiredPortProxy: boolean;
	    actualSystemProxy: boolean;
	    actualTun: boolean;
	    controlRevision: number;
	    systemProxy: boolean;
	    tun: boolean;
	    version: string;
	    appVersion: string;
	    activeConfig: string;
	    activeConfigName: string;
	    activeConfigType: string;
	    delayRetention: boolean;
	    delayRetentionTime: string;
	    updateReady: boolean;
	    newAppVersion: string;
	    updateDownloaded: boolean;
	    downloadedPath: string;
	    portProxyMode: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.isRunning = source["isRunning"];
	        this.isAdmin = source["isAdmin"];
	        this.mode = source["mode"];
	        this.theme = source["theme"];
	        this.hideLogs = source["hideLogs"];
	        this.logLevel = source["logLevel"];
	        this.appLogLevel = source["appLogLevel"];
	        this.desiredSystemProxy = source["desiredSystemProxy"];
	        this.desiredTun = source["desiredTun"];
	        this.desiredPortProxy = source["desiredPortProxy"];
	        this.actualSystemProxy = source["actualSystemProxy"];
	        this.actualTun = source["actualTun"];
	        this.controlRevision = source["controlRevision"];
	        this.systemProxy = source["systemProxy"];
	        this.tun = source["tun"];
	        this.version = source["version"];
	        this.appVersion = source["appVersion"];
	        this.activeConfig = source["activeConfig"];
	        this.activeConfigName = source["activeConfigName"];
	        this.activeConfigType = source["activeConfigType"];
	        this.delayRetention = source["delayRetention"];
	        this.delayRetentionTime = source["delayRetentionTime"];
	        this.updateReady = source["updateReady"];
	        this.newAppVersion = source["newAppVersion"];
	        this.updateDownloaded = source["updateDownloaded"];
	        this.downloadedPath = source["downloadedPath"];
	        this.portProxyMode = source["portProxyMode"];
	    }
	}
	export class ConnectionMetadataDTO {
	    network: string;
	    type: string;
	    sourceIP: string;
	    destinationIP: string;
	    sourcePort: string;
	    destinationPort: string;
	    host: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionMetadataDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.network = source["network"];
	        this.type = source["type"];
	        this.sourceIP = source["sourceIP"];
	        this.destinationIP = source["destinationIP"];
	        this.sourcePort = source["sourcePort"];
	        this.destinationPort = source["destinationPort"];
	        this.host = source["host"];
	    }
	}
	export class ConnectionDTO {
	    id: string;
	    metadata: ConnectionMetadataDTO;
	    upload: number;
	    download: number;
	    start: string;
	    chains: string[];
	    rule: string;
	    rulePayload: string;
	    uploadStr: string;
	    downloadStr: string;
	    durationStr: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.metadata = this.convertValues(source["metadata"], ConnectionMetadataDTO);
	        this.upload = source["upload"];
	        this.download = source["download"];
	        this.start = source["start"];
	        this.chains = source["chains"];
	        this.rule = source["rule"];
	        this.rulePayload = source["rulePayload"];
	        this.uploadStr = source["uploadStr"];
	        this.downloadStr = source["downloadStr"];
	        this.durationStr = source["durationStr"];
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
	
	export class ConnectionsSnapshot {
	    connections: ConnectionDTO[];
	
	    static createFrom(source: any = {}) {
	        return new ConnectionsSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connections = this.convertValues(source["connections"], ConnectionDTO);
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
	export class DiagnosticAssets {
	    "clash.exe": boolean;
	    "wintun.dll": boolean;
	    "geoip.metadb": boolean;
	    "geosite.dat": boolean;
	    "country.mmdb": boolean;
	    "asn.dat": boolean;
	
	    static createFrom(source: any = {}) {
	        return new DiagnosticAssets(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this["clash.exe"] = source["clash.exe"];
	        this["wintun.dll"] = source["wintun.dll"];
	        this["geoip.metadb"] = source["geoip.metadb"];
	        this["geosite.dat"] = source["geosite.dat"];
	        this["country.mmdb"] = source["country.mmdb"];
	        this["asn.dat"] = source["asn.dat"];
	    }
	}
	export class DiagnosticInfo {
	    appDir: string;
	    dataDir: string;
	    seedCoreBinDir: string;
	    runtimeCoreBinDir: string;
	    runtimeConfigPath: string;
	    isAdmin: boolean;
	    helperServiceStatus: string;
	    helperBinaryPath: string;
	    assets: DiagnosticAssets;
	    seedManifest: any;
	    assetState: any;
	
	    static createFrom(source: any = {}) {
	        return new DiagnosticInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.appDir = source["appDir"];
	        this.dataDir = source["dataDir"];
	        this.seedCoreBinDir = source["seedCoreBinDir"];
	        this.runtimeCoreBinDir = source["runtimeCoreBinDir"];
	        this.runtimeConfigPath = source["runtimeConfigPath"];
	        this.isAdmin = source["isAdmin"];
	        this.helperServiceStatus = source["helperServiceStatus"];
	        this.helperBinaryPath = source["helperBinaryPath"];
	        this.assets = this.convertValues(source["assets"], DiagnosticAssets);
	        this.seedManifest = source["seedManifest"];
	        this.assetState = source["assetState"];
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
	export class OutboundIPResult {
	    preferred: string;
	    ipv4: string;
	    ipv6: string;
	    mode: string;
	    source: string;
	    source4: string;
	    source6: string;
	    message: string;
	    message4: string;
	    message6: string;
	    complete: boolean;
	
	    static createFrom(source: any = {}) {
	        return new OutboundIPResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.preferred = source["preferred"];
	        this.ipv4 = source["ipv4"];
	        this.ipv6 = source["ipv6"];
	        this.mode = source["mode"];
	        this.source = source["source"];
	        this.source4 = source["source4"];
	        this.source6 = source["source6"];
	        this.message = source["message"];
	        this.message4 = source["message4"];
	        this.message6 = source["message6"];
	        this.complete = source["complete"];
	    }
	}

}

export namespace clash {
	
	export class BuildRuleRequest {
	    type: string;
	    payload: string;
	    policy: string;
	
	    static createFrom(source: any = {}) {
	        return new BuildRuleRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.payload = source["payload"];
	        this.policy = source["policy"];
	    }
	}
	export class ConfigTextResult {
	    id: string;
	    name: string;
	    type: string;
	    content: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigTextResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.content = source["content"];
	        this.path = source["path"];
	    }
	}
	export class FallbackFilterConfig {
	    geoip: boolean;
	    geoipCode: string;
	    ipcidr: string[];
	    domain: string[];
	
	    static createFrom(source: any = {}) {
	        return new FallbackFilterConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.geoip = source["geoip"];
	        this.geoipCode = source["geoipCode"];
	        this.ipcidr = source["ipcidr"];
	        this.domain = source["domain"];
	    }
	}
	export class DNSConfig {
	    enable: boolean;
	    listen: string;
	    ipv6: boolean;
	    preferH3: boolean;
	    enhancedMode: string;
	    respectRules: boolean;
	    fakeIpRange: string;
	    fakeIpFilter: string[];
	    useSystemHosts: boolean;
	    useHosts: boolean;
	    defaultNameserver: string[];
	    nameserver: string[];
	    fallback: string[];
	    directNameserver: string[];
	    proxyServerNameserver: string[];
	    nameserverPolicy: Record<string, string>;
	    fallbackFilter: FallbackFilterConfig;
	
	    static createFrom(source: any = {}) {
	        return new DNSConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enable = source["enable"];
	        this.listen = source["listen"];
	        this.ipv6 = source["ipv6"];
	        this.preferH3 = source["preferH3"];
	        this.enhancedMode = source["enhancedMode"];
	        this.respectRules = source["respectRules"];
	        this.fakeIpRange = source["fakeIpRange"];
	        this.fakeIpFilter = source["fakeIpFilter"];
	        this.useSystemHosts = source["useSystemHosts"];
	        this.useHosts = source["useHosts"];
	        this.defaultNameserver = source["defaultNameserver"];
	        this.nameserver = source["nameserver"];
	        this.fallback = source["fallback"];
	        this.directNameserver = source["directNameserver"];
	        this.proxyServerNameserver = source["proxyServerNameserver"];
	        this.nameserverPolicy = source["nameserverPolicy"];
	        this.fallbackFilter = this.convertValues(source["fallbackFilter"], FallbackFilterConfig);
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
	
	export class NetworkConfig {
	    port: number;
	    mixedPort: number;
	    ipv6: boolean;
	    unifiedDelay: boolean;
	    tcpConcurrent: boolean;
	    tcpKeepAlive: boolean;
	    tcpKeepAliveInterval: number;
	    testUrl: string;
	    externalController: string;
	    allowLan: boolean;
	    hosts: string;
	    lanAuthUser: string;
	    lanAuthPass: string;
	    lanAllowedIPs: string;
	    lanDisallowedIPs: string;
	    lanSkipAuthIPs: string;
	    socksPort: number;
	    externalControllerEnabled: boolean;
	    externalControllerSecret: string;
	
	    static createFrom(source: any = {}) {
	        return new NetworkConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.mixedPort = source["mixedPort"];
	        this.ipv6 = source["ipv6"];
	        this.unifiedDelay = source["unifiedDelay"];
	        this.tcpConcurrent = source["tcpConcurrent"];
	        this.tcpKeepAlive = source["tcpKeepAlive"];
	        this.tcpKeepAliveInterval = source["tcpKeepAliveInterval"];
	        this.testUrl = source["testUrl"];
	        this.externalController = source["externalController"];
	        this.allowLan = source["allowLan"];
	        this.hosts = source["hosts"];
	        this.lanAuthUser = source["lanAuthUser"];
	        this.lanAuthPass = source["lanAuthPass"];
	        this.lanAllowedIPs = source["lanAllowedIPs"];
	        this.lanDisallowedIPs = source["lanDisallowedIPs"];
	        this.lanSkipAuthIPs = source["lanSkipAuthIPs"];
	        this.socksPort = source["socksPort"];
	        this.externalControllerEnabled = source["externalControllerEnabled"];
	        this.externalControllerSecret = source["externalControllerSecret"];
	    }
	}
	export class PolicyOption {
	    value: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new PolicyOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
	    }
	}
	export class RuleTypeOption {
	    value: string;
	    label: string;
	    count: number;
	    needPayload: boolean;
	    needPolicy: boolean;
	    payloadLabel: string;
	    payloadHint: string;
	    example: string;
	
	    static createFrom(source: any = {}) {
	        return new RuleTypeOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
	        this.count = source["count"];
	        this.needPayload = source["needPayload"];
	        this.needPolicy = source["needPolicy"];
	        this.payloadLabel = source["payloadLabel"];
	        this.payloadHint = source["payloadHint"];
	        this.example = source["example"];
	    }
	}
	export class RuleFormOptions {
	    types: RuleTypeOption[];
	    policies: PolicyOption[];
	
	    static createFrom(source: any = {}) {
	        return new RuleFormOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.types = this.convertValues(source["types"], RuleTypeOption);
	        this.policies = this.convertValues(source["policies"], PolicyOption);
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
	export class RulePageData {
	    configType: string;
	    subscriptionRules: string[];
	    localRules: string[];
	    addRules: string[];
	    deleteRules: string[];
	    effectiveRules: string[];
	
	    static createFrom(source: any = {}) {
	        return new RulePageData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configType = source["configType"];
	        this.subscriptionRules = source["subscriptionRules"];
	        this.localRules = source["localRules"];
	        this.addRules = source["addRules"];
	        this.deleteRules = source["deleteRules"];
	        this.effectiveRules = source["effectiveRules"];
	    }
	}
	
	export class SubIndexItem {
	    id: string;
	    name: string;
	    url: string;
	    type: string;
	    upload: number;
	    download: number;
	    total: number;
	    expire: number;
	    updated: number;
	
	    static createFrom(source: any = {}) {
	        return new SubIndexItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.url = source["url"];
	        this.type = source["type"];
	        this.upload = source["upload"];
	        this.download = source["download"];
	        this.total = source["total"];
	        this.expire = source["expire"];
	        this.updated = source["updated"];
	    }
	}
	export class TunConfig {
	    stack: string;
	    device: string;
	    autoRoute: boolean;
	    autoDetect: boolean;
	    dnsHijack: string[];
	    strictRoute: boolean;
	    mtu: number;
	
	    static createFrom(source: any = {}) {
	        return new TunConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stack = source["stack"];
	        this.device = source["device"];
	        this.autoRoute = source["autoRoute"];
	        this.autoDetect = source["autoDetect"];
	        this.dnsHijack = source["dnsHijack"];
	        this.strictRoute = source["strictRoute"];
	        this.mtu = source["mtu"];
	    }
	}

}

export namespace logger {
	
	export class LogEntry {
	    type: string;
	    source: string;
	    payload: string;
	    time: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.source = source["source"];
	        this.payload = source["payload"];
	        this.time = source["time"];
	    }
	}

}

export namespace main {
	
	export class FileInfo {
	    path: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new FileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	    }
	}

}

export namespace runtimeassets {
	
	export class AssetHealth {
	    key: string;
	    label: string;
	    path: string;
	    exists: boolean;
	    valid: boolean;
	    ready: boolean;
	    required: boolean;
	    size: number;
	    modTime: number;
	    sha256?: string;
	    version?: string;
	    versionProbeOK?: boolean;
	    errorCode?: string;
	    error?: string;
	    hint?: string;
	
	    static createFrom(source: any = {}) {
	        return new AssetHealth(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.path = source["path"];
	        this.exists = source["exists"];
	        this.valid = source["valid"];
	        this.ready = source["ready"];
	        this.required = source["required"];
	        this.size = source["size"];
	        this.modTime = source["modTime"];
	        this.sha256 = source["sha256"];
	        this.version = source["version"];
	        this.versionProbeOK = source["versionProbeOK"];
	        this.errorCode = source["errorCode"];
	        this.error = source["error"];
	        this.hint = source["hint"];
	    }
	}
	export class RuntimeAssetStatus {
	    appDir: string;
	    dataDir: string;
	    coreBinDir: string;
	    seedCoreBinDir: string;
	    assets: Record<string, AssetHealth>;
	    coreReady: boolean;
	    wintunReady: boolean;
	    ready: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeAssetStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.appDir = source["appDir"];
	        this.dataDir = source["dataDir"];
	        this.coreBinDir = source["coreBinDir"];
	        this.seedCoreBinDir = source["seedCoreBinDir"];
	        this.assets = this.convertValues(source["assets"], AssetHealth, true);
	        this.coreReady = source["coreReady"];
	        this.wintunReady = source["wintunReady"];
	        this.ready = source["ready"];
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

}

export namespace sys {
	
	export class DataDirInfo {
	    appDir: string;
	    dataDir: string;
	    coreBinDir: string;
	    seedCoreBinDir: string;
	    seedManifestExists: boolean;
	    seedManifestOK: boolean;
	    seedManifestError?: string;
	    seedCoreReady: boolean;
	    seedWintunReady: boolean;
	    seedGeoipReady: boolean;
	    seedGeositeReady: boolean;
	    seedMmdbReady: boolean;
	    seedAsnReady: boolean;
	    canAutoRepairCore: boolean;
	    canAutoRepairWintun: boolean;
	    coreExePath: string;
	    coreExists: boolean;
	    coreReady: boolean;
	    coreError?: string;
	    wintunExists: boolean;
	    wintunReady: boolean;
	    wintunError?: string;
	    layoutMode: string;
	    layoutOK: boolean;
	    legacyDataDir: string;
	    legacyExists: boolean;
	    legacyCoreExists: boolean;
	    migrated: boolean;
	    lastError: string;
	
	    static createFrom(source: any = {}) {
	        return new DataDirInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.appDir = source["appDir"];
	        this.dataDir = source["dataDir"];
	        this.coreBinDir = source["coreBinDir"];
	        this.seedCoreBinDir = source["seedCoreBinDir"];
	        this.seedManifestExists = source["seedManifestExists"];
	        this.seedManifestOK = source["seedManifestOK"];
	        this.seedManifestError = source["seedManifestError"];
	        this.seedCoreReady = source["seedCoreReady"];
	        this.seedWintunReady = source["seedWintunReady"];
	        this.seedGeoipReady = source["seedGeoipReady"];
	        this.seedGeositeReady = source["seedGeositeReady"];
	        this.seedMmdbReady = source["seedMmdbReady"];
	        this.seedAsnReady = source["seedAsnReady"];
	        this.canAutoRepairCore = source["canAutoRepairCore"];
	        this.canAutoRepairWintun = source["canAutoRepairWintun"];
	        this.coreExePath = source["coreExePath"];
	        this.coreExists = source["coreExists"];
	        this.coreReady = source["coreReady"];
	        this.coreError = source["coreError"];
	        this.wintunExists = source["wintunExists"];
	        this.wintunReady = source["wintunReady"];
	        this.wintunError = source["wintunError"];
	        this.layoutMode = source["layoutMode"];
	        this.layoutOK = source["layoutOK"];
	        this.legacyDataDir = source["legacyDataDir"];
	        this.legacyExists = source["legacyExists"];
	        this.legacyCoreExists = source["legacyCoreExists"];
	        this.migrated = source["migrated"];
	        this.lastError = source["lastError"];
	    }
	}
	export class HelperStatusData {
	    installed: boolean;
	    running: boolean;
	    reachable: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new HelperStatusData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.running = source["running"];
	        this.reachable = source["reachable"];
	        this.error = source["error"];
	    }
	}
	export class StartupTaskInfo {
	    exists: boolean;
	    enabled: boolean;
	    schedulerEnabled: boolean;
	    mode: string;
	    path: string;
	    arguments: string;
	    runLevel: number;
	    lastError: string;
	    expectedPath: string;
	    actualPath: string;
	    actualArgs: string;
	    expectedDataDir: string;
	    actualDataDir: string;
	    isHealthy: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StartupTaskInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exists = source["exists"];
	        this.enabled = source["enabled"];
	        this.schedulerEnabled = source["schedulerEnabled"];
	        this.mode = source["mode"];
	        this.path = source["path"];
	        this.arguments = source["arguments"];
	        this.runLevel = source["runLevel"];
	        this.lastError = source["lastError"];
	        this.expectedPath = source["expectedPath"];
	        this.actualPath = source["actualPath"];
	        this.actualArgs = source["actualArgs"];
	        this.expectedDataDir = source["expectedDataDir"];
	        this.actualDataDir = source["actualDataDir"];
	        this.isHealthy = source["isHealthy"];
	    }
	}
	export class UwpApp {
	    displayName: string;
	    packageFamilyName: string;
	    sid: string;
	    isEnabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UwpApp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.displayName = source["displayName"];
	        this.packageFamilyName = source["packageFamilyName"];
	        this.sid = source["sid"];
	        this.isEnabled = source["isEnabled"];
	    }
	}

}

