export namespace main {
	
	export class AppConfig {
	    server_name: string;
	    peer_addr: string;
	    password: string;
	    vk_hash: string;
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
	        this.num_workers = source["num_workers"];
	        this.conn_mode = source["conn_mode"];
	        this.socks_addr = source["socks_addr"];
	        this.go_dns = source["go_dns"];
	        this.obfs_mode = source["obfs_mode"];
	        this.turn_tcp = source["turn_tcp"];
	        this.auto_connect = source["auto_connect"];
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
	    }
	}

}

