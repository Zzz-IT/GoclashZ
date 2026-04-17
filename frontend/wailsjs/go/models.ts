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
	    ipv6: boolean;
	    enhancedMode: string;
	    fakeIpRange: string;
	    fakeIpFilter: string[];
	    useSystemHosts: boolean;
	    useHosts: boolean;
	    defaultNameserver: string[];
	    nameserver: string[];
	    fallback: string[];
	    fallbackFilter: FallbackFilterConfig;
	    nameserverPolicy: Record<string, string>;
	    proxyServerNameserver: string[];
	
	    static createFrom(source: any = {}) {
	        return new DNSConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enable = source["enable"];
	        this.ipv6 = source["ipv6"];
	        this.enhancedMode = source["enhancedMode"];
	        this.fakeIpRange = source["fakeIpRange"];
	        this.fakeIpFilter = source["fakeIpFilter"];
	        this.useSystemHosts = source["useSystemHosts"];
	        this.useHosts = source["useHosts"];
	        this.defaultNameserver = source["defaultNameserver"];
	        this.nameserver = source["nameserver"];
	        this.fallback = source["fallback"];
	        this.fallbackFilter = this.convertValues(source["fallbackFilter"], FallbackFilterConfig);
	        this.nameserverPolicy = source["nameserverPolicy"];
	        this.proxyServerNameserver = source["proxyServerNameserver"];
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

