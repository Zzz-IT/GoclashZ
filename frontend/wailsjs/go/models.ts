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
	    ipv6: boolean;
	    unifiedDelay: boolean;
	    tcpConcurrent: boolean;
	    tcpKeepAlive: boolean;
	    tcpKeepAliveInterval: number;
	    testUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new NetworkConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ipv6 = source["ipv6"];
	        this.unifiedDelay = source["unifiedDelay"];
	        this.tcpConcurrent = source["tcpConcurrent"];
	        this.tcpKeepAlive = source["tcpKeepAlive"];
	        this.tcpKeepAliveInterval = source["tcpKeepAliveInterval"];
	        this.testUrl = source["testUrl"];
	    }
	}
	export class RuleInfo {
	    rules: string[];
	    isEditable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RuleInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rules = source["rules"];
	        this.isEditable = source["isEditable"];
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

export namespace main {
	
	export class AppBehavior {
	    silentStart: boolean;
	    closeToTray: boolean;
	    logLevel: string;
	    hideLogs: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppBehavior(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.silentStart = source["silentStart"];
	        this.closeToTray = source["closeToTray"];
	        this.logLevel = source["logLevel"];
	        this.hideLogs = source["hideLogs"];
	    }
	}
	export class ProxyStatus {
	    systemProxy: boolean;
	    tun: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProxyStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.systemProxy = source["systemProxy"];
	        this.tun = source["tun"];
	    }
	}

}

