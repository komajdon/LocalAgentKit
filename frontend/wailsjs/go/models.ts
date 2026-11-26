export namespace app {
	
	export class ContextUsage {
	    used: number;
	    limit: number;
	    total: number;
	    estimated: boolean;
	    omitted: number;
	    summarised: number;
	
	    static createFrom(source: any = {}) {
	        return new ContextUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.used = source["used"];
	        this.limit = source["limit"];
	        this.total = source["total"];
	        this.estimated = source["estimated"];
	        this.omitted = source["omitted"];
	        this.summarised = source["summarised"];
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
	    sandbox_mode: string;
	    sandbox_network: boolean;
	    tool_calling: string;
	    backup_retention: number;
	    context_strategy: string;
	
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
	        this.sandbox_mode = source["sandbox_mode"];
	        this.sandbox_network = source["sandbox_network"];
	        this.tool_calling = source["tool_calling"];
	        this.backup_retention = source["backup_retention"];
	        this.context_strategy = source["context_strategy"];
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

export namespace domain {
	
	export class Capabilities {
	    native_tools?: boolean;
	    context_window: number;
	    streaming: boolean;
	    usage_reporting: boolean;
	    model_discovery: boolean;
	    structured_output?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Capabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.native_tools = source["native_tools"];
	        this.context_window = source["context_window"];
	        this.streaming = source["streaming"];
	        this.usage_reporting = source["usage_reporting"];
	        this.model_discovery = source["model_discovery"];
	        this.structured_output = source["structured_output"];
	    }
	}
	export class ToolCall {
	    id?: string;
	    tool: string;
	    args: Record<string, any>;
	    malformed?: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolCall(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.tool = source["tool"];
	        this.args = source["args"];
	        this.malformed = source["malformed"];
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
	export class Summary {
	    text: string;
	    covered: number;
	
	    static createFrom(source: any = {}) {
	        return new Summary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.covered = source["covered"];
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
	    tool_calls?: domain.ToolCall[];
	    tool_call_id?: string;
	    tool_name?: string;
	
	    static createFrom(source: any = {}) {
	        return new SavedMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.tool_calls = this.convertValues(source["tool_calls"], domain.ToolCall);
	        this.tool_call_id = source["tool_call_id"];
	        this.tool_name = source["tool_name"];
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
	    summary?: Summary;
	
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
	        this.summary = this.convertValues(source["summary"], Summary);
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
	export class ConnectionResult {
	    ok: boolean;
	    models: number;
	    millis: number;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.models = source["models"];
	        this.millis = source["millis"];
	        this.error = source["error"];
	    }
	}
	export class ConversationView {
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
	    messages: history.SavedMessage[];
	    display_items?: number[];
	    token_usage?: history.TokenUsage;
	    summary?: history.Summary;
	    running: boolean;
	    seq: number;
	
	    static createFrom(source: any = {}) {
	        return new ConversationView(source);
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
	        this.messages = this.convertValues(source["messages"], history.SavedMessage);
	        this.display_items = source["display_items"];
	        this.token_usage = this.convertValues(source["token_usage"], history.TokenUsage);
	        this.summary = this.convertValues(source["summary"], history.Summary);
	        this.running = source["running"];
	        this.seq = source["seq"];
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
	export class ModelOverride {
	    context_limit: number;
	    tool_calling: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelOverride(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.context_limit = source["context_limit"];
	        this.tool_calling = source["tool_calling"];
	    }
	}
	export class ModelProfile {
	    model: string;
	    detected: domain.Capabilities;
	    context_limit: number;
	    tool_calling: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.detected = this.convertValues(source["detected"], domain.Capabilities);
	        this.context_limit = source["context_limit"];
	        this.tool_calling = source["tool_calling"];
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
	export class ModelSettings {
	    timeout_seconds: number;
	    overrides: Record<string, ModelOverride>;
	
	    static createFrom(source: any = {}) {
	        return new ModelSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timeout_seconds = source["timeout_seconds"];
	        this.overrides = this.convertValues(source["overrides"], ModelOverride, true);
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
	export class RunState {
	    conversation_id: string;
	    run_id: string;
	
	    static createFrom(source: any = {}) {
	        return new RunState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversation_id = source["conversation_id"];
	        this.run_id = source["run_id"];
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

}

export namespace permission {
	
	export class Grant {
	    id: string;
	    workspace: string;
	    tool: string;
	    label: string;
	    scope: string;
	
	    static createFrom(source: any = {}) {
	        return new Grant(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.workspace = source["workspace"];
	        this.tool = source["tool"];
	        this.label = source["label"];
	        this.scope = source["scope"];
	    }
	}

}

export namespace sandbox {
	
	export class Status {
	    backend: string;
	    isolated: boolean;
	    network_blocked: boolean;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.backend = source["backend"];
	        this.isolated = source["isolated"];
	        this.network_blocked = source["network_blocked"];
	        this.detail = source["detail"];
	    }
	}

}

export namespace tools {
	
	export class MCPStatus {
	    name: string;
	    state: string;
	    tools: string[];
	    last_error: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.state = source["state"];
	        this.tools = source["tools"];
	        this.last_error = source["last_error"];
	    }
	}

}

export namespace usage {
	
	export class Entry {
	    day: string;
	    provider: string;
	    model: string;
	    conversation_id: string;
	    requests: number;
	    estimated_requests: number;
	    input: number;
	    output: number;
	    cached: number;
	    cost: number;
	    unpriced_requests: number;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.conversation_id = source["conversation_id"];
	        this.requests = source["requests"];
	        this.estimated_requests = source["estimated_requests"];
	        this.input = source["input"];
	        this.output = source["output"];
	        this.cached = source["cached"];
	        this.cost = source["cost"];
	        this.unpriced_requests = source["unpriced_requests"];
	    }
	}
	export class Filter {
	    provider: string;
	    model: string;
	    conversation_id: string;
	    from: string;
	    to: string;
	
	    static createFrom(source: any = {}) {
	        return new Filter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.conversation_id = source["conversation_id"];
	        this.from = source["from"];
	        this.to = source["to"];
	    }
	}
	export class Limits {
	    run_tokens: number;
	    run_requests: number;
	    run_cost: number;
	    daily_tokens: number;
	    daily_cost: number;
	    monthly_tokens: number;
	    monthly_cost: number;
	    warn_percent: number;
	    hard_period_limits: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Limits(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.run_tokens = source["run_tokens"];
	        this.run_requests = source["run_requests"];
	        this.run_cost = source["run_cost"];
	        this.daily_tokens = source["daily_tokens"];
	        this.daily_cost = source["daily_cost"];
	        this.monthly_tokens = source["monthly_tokens"];
	        this.monthly_cost = source["monthly_cost"];
	        this.warn_percent = source["warn_percent"];
	        this.hard_period_limits = source["hard_period_limits"];
	    }
	}
	export class Price {
	    provider: string;
	    model: string;
	    input_per_million: number;
	    output_per_million: number;
	    cached_per_million: number;
	
	    static createFrom(source: any = {}) {
	        return new Price(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.input_per_million = source["input_per_million"];
	        this.output_per_million = source["output_per_million"];
	        this.cached_per_million = source["cached_per_million"];
	    }
	}
	export class Totals {
	    requests: number;
	    estimated_requests: number;
	    input: number;
	    output: number;
	    cached: number;
	    cost: number;
	    unpriced_requests: number;
	
	    static createFrom(source: any = {}) {
	        return new Totals(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requests = source["requests"];
	        this.estimated_requests = source["estimated_requests"];
	        this.input = source["input"];
	        this.output = source["output"];
	        this.cached = source["cached"];
	        this.cost = source["cost"];
	        this.unpriced_requests = source["unpriced_requests"];
	    }
	}
	export class Report {
	    entries: Entry[];
	    total: Totals;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entries = this.convertValues(source["entries"], Entry);
	        this.total = this.convertValues(source["total"], Totals);
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
	    currency: string;
	    prices: Price[];
	    limits: Limits;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currency = source["currency"];
	        this.prices = this.convertValues(source["prices"], Price);
	        this.limits = this.convertValues(source["limits"], Limits);
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

