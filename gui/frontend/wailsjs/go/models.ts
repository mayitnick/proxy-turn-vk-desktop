export namespace clientengine {
	
	export class VKHashCheckResult {
	    hash: string;
	    normalized: string;
	    status: string;
	    turn_urls: string[];
	    turn_count: number;
	    latency_ms: number;
	    error_message: string;
	    hint: string;
	
	    static createFrom(source: any = {}) {
	        return new VKHashCheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.normalized = source["normalized"];
	        this.status = source["status"];
	        this.turn_urls = source["turn_urls"];
	        this.turn_count = source["turn_count"];
	        this.latency_ms = source["latency_ms"];
	        this.error_message = source["error_message"];
	        this.hint = source["hint"];
	    }
	}

}

export namespace main {
	
	export class AppConfig {
	    server_name: string;
	    peer_addr: string;
	    password: string;
	    vk_hash: string;
	    vk_hashes: string[];
	    num_workers: number;
	    conn_mode: string;
	    socks_addr: string;
	    go_dns: string;
	    obfs_mode: string;
	    turn_tcp: boolean;
	    auto_connect: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server_name = source["server_name"];
	        this.peer_addr = source["peer_addr"];
	        this.password = source["password"];
	        this.vk_hash = source["vk_hash"];
	        this.vk_hashes = source["vk_hashes"];
	        this.num_workers = source["num_workers"];
	        this.conn_mode = source["conn_mode"];
	        this.socks_addr = source["socks_addr"];
	        this.go_dns = source["go_dns"];
	        this.obfs_mode = source["obfs_mode"];
	        this.turn_tcp = source["turn_tcp"];
	        this.auto_connect = source["auto_connect"];
	    }
	}
	export class DiagStep {
	    id: string;
	    title: string;
	    status: string;
	    message: string;
	    hint: string;
	    latency_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new DiagStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.status = source["status"];
	        this.message = source["message"];
	        this.hint = source["hint"];
	        this.latency_ms = source["latency_ms"];
	    }
	}
	export class DiagReport {
	    overall_status: string;
	    timestamp: string;
	    summary: string;
	    steps: DiagStep[];
	
	    static createFrom(source: any = {}) {
	        return new DiagReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.overall_status = source["overall_status"];
	        this.timestamp = source["timestamp"];
	        this.summary = source["summary"];
	        this.steps = this.convertValues(source["steps"], DiagStep);
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
	
	export class GUIStats {
	    connected: boolean;
	    is_admin: boolean;
	    state: string;
	    state_msg: string;
	    active_workers: number;
	    total_workers: number;
	    ping_ms: number;
	    current_down_bps: number;
	    current_up_bps: number;
	    session_down: number;
	    session_up: number;
	    lifetime_down: number;
	    lifetime_up: number;
	    exit_ip: string;
	    exit_country: string;
	    uptime_sec: number;
	    last_error: string;
	    fox_sleeping: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GUIStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.is_admin = source["is_admin"];
	        this.state = source["state"];
	        this.state_msg = source["state_msg"];
	        this.active_workers = source["active_workers"];
	        this.total_workers = source["total_workers"];
	        this.ping_ms = source["ping_ms"];
	        this.current_down_bps = source["current_down_bps"];
	        this.current_up_bps = source["current_up_bps"];
	        this.session_down = source["session_down"];
	        this.session_up = source["session_up"];
	        this.lifetime_down = source["lifetime_down"];
	        this.lifetime_up = source["lifetime_up"];
	        this.exit_ip = source["exit_ip"];
	        this.exit_country = source["exit_country"];
	        this.uptime_sec = source["uptime_sec"];
	        this.last_error = source["last_error"];
	        this.fox_sleeping = source["fox_sleeping"];
	    }
	}

}

