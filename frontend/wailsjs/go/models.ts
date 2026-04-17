export namespace clash {
	
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

