export namespace clash {
	
	export class ProxyNode {
	    name: string;
	    type: string;
	    now: string;
	    proxies: string[];
	
	    static createFrom(source: any = {}) {
	        return new ProxyNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.now = source["now"];
	        this.proxies = source["proxies"];
	    }
	}

}

