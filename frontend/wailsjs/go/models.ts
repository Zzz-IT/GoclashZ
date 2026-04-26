export namespace clash {
	
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
	    hosts: string;
	
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
	        this.hosts = source["hosts"];
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
	    enable: boolean;
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
	        this.enable = source["enable"];
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
	    payload: string;
	    time: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.payload = source["payload"];
	        this.time = source["time"];
	    }
	}

}

export namespace main {
	
	export class AppBehavior {
	    silentStart: boolean;
	    closeToTray: boolean;
	    colorDelay: boolean;
	    delayRetention: boolean;
	    delayRetentionTime: string;
	    logLevel: string;
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
	    }
	}
	export class AppState {
	    isRunning: boolean;
	    mode: string;
	    theme: string;
	    hideLogs: boolean;
	    systemProxy: boolean;
	    tun: boolean;
	    version: string;
	    appVersion: string;
	    activeConfig: string;
	    activeConfigName: string;
	    activeConfigType: string;
	    delayRetention: boolean;
	    delayRetentionTime: string;
	
	    static createFrom(source: any = {}) {
	        return new AppState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.isRunning = source["isRunning"];
	        this.mode = source["mode"];
	        this.theme = source["theme"];
	        this.hideLogs = source["hideLogs"];
	        this.systemProxy = source["systemProxy"];
	        this.tun = source["tun"];
	        this.version = source["version"];
	        this.appVersion = source["appVersion"];
	        this.activeConfig = source["activeConfig"];
	        this.activeConfigName = source["activeConfigName"];
	        this.activeConfigType = source["activeConfigType"];
	        this.delayRetention = source["delayRetention"];
	        this.delayRetentionTime = source["delayRetentionTime"];
	    }
	}
	export class RuleItem {
	    index: number;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new RuleItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.text = source["text"];
	    }
	}
	export class PagedRules {
	    total: number;
	    items: RuleItem[];
	    isEditable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PagedRules(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.items = this.convertValues(source["items"], RuleItem);
	        this.isEditable = source["isEditable"];
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
	
	export class SelectedFile {
	    path: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new SelectedFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	    }
	}

}

export namespace sys {
	
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

