export namespace app {
	
	export class ContextUsage {
	    used: number;
	    limit: number;
	    total: number;
	    estimated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ContextUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.used = source["used"];
	        this.limit = source["limit"];
	        this.total = source["total"];
	        this.estimated = source["estimated"];
	    }
	}

}

export namespace config {
	
	export class MCPServerConfig {
	    name: string;
	    command: string;
	    args: string[];
	    env: Record<string, string>;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MCPServerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.command = source["command"];
	        this.args = source["args"];
	        this.env = source["env"];
	        this.enabled = source["enabled"];
	    }
	}
	export class Config {
	    provider: string;
	    base_url: string;
	    api_key: string;
	    model: string;
	    work_dir: string;
	    tool_permissions: Record<string, string>;
	    whisper_model: string;
	    system_prompt: string;
	    context_limit: number;
	    mcp_servers: MCPServerConfig[];
	    notifications: boolean;
	    search_provider: string;
	    search_api_key: string;
	    theme: string;
	    budget_data_gb: number;
	    budget_fund: number;
	    fund_per_mtokens: number;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.base_url = source["base_url"];
	        this.api_key = source["api_key"];
	        this.model = source["model"];
	        this.work_dir = source["work_dir"];
	        this.tool_permissions = source["tool_permissions"];
	        this.whisper_model = source["whisper_model"];
	        this.system_prompt = source["system_prompt"];
	        this.context_limit = source["context_limit"];
	        this.mcp_servers = this.convertValues(source["mcp_servers"], MCPServerConfig);
	        this.notifications = source["notifications"];
	        this.search_provider = source["search_provider"];
	        this.search_api_key = source["search_api_key"];
	        this.theme = source["theme"];
	        this.budget_data_gb = source["budget_data_gb"];
	        this.budget_fund = source["budget_fund"];
	        this.fund_per_mtokens = source["fund_per_mtokens"];
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

export namespace history {
	
	export class ConvMeta {
	    id: string;
	    title: string;
	    work_dir: string;
	    model?: string;
	    pinned?: boolean;
	    tags?: string[];
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new ConvMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.work_dir = source["work_dir"];
	        this.model = source["model"];
	        this.pinned = source["pinned"];
	        this.tags = source["tags"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
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
	export class TokenUsage {
	    prompt_tokens: number;
	    completion_tokens: number;
	
	    static createFrom(source: any = {}) {
	        return new TokenUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.prompt_tokens = source["prompt_tokens"];
	        this.completion_tokens = source["completion_tokens"];
	    }
	}
	export class SavedMessage {
	    role: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new SavedMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	    }
	}
	export class Conversation {
	    id: string;
	    title: string;
	    work_dir: string;
	    model?: string;
	    pinned?: boolean;
	    tags?: string[];
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	    messages: SavedMessage[];
	    display_items?: number[];
	    token_usage?: TokenUsage;
	
	    static createFrom(source: any = {}) {
	        return new Conversation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.work_dir = source["work_dir"];
	        this.model = source["model"];
	        this.pinned = source["pinned"];
	        this.tags = source["tags"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.messages = this.convertValues(source["messages"], SavedMessage);
	        this.display_items = source["display_items"];
	        this.token_usage = this.convertValues(source["token_usage"], TokenUsage);
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

export namespace main {
	
	export class ChatResponse {
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.error = source["error"];
	    }
	}
	export class ModelList {
	    models: string[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.models = source["models"];
	        this.error = source["error"];
	    }
	}
	export class ToolInfo {
	    name: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	    }
	}
	export class UpdateInfo {
	    available: boolean;
	    current: string;
	    latest: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.url = source["url"];
	    }
	}
	export class UsageBudget {
	    tokens: number;
	    data_total_gb: number;
	    data_remain_gb: number;
	    fund_total: number;
	    fund_remain: number;
	    fund_unit: string;
	
	    static createFrom(source: any = {}) {
	        return new UsageBudget(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tokens = source["tokens"];
	        this.data_total_gb = source["data_total_gb"];
	        this.data_remain_gb = source["data_remain_gb"];
	        this.fund_total = source["fund_total"];
	        this.fund_remain = source["fund_remain"];
	        this.fund_unit = source["fund_unit"];
	    }
	}

}

