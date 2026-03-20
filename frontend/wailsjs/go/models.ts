export namespace app {
	
	export class StatusSummary {
	    workspaceReady: boolean;
	    watcherActive: boolean;
	    snapshot: number;
	    fileCount: number;
	    conflicts: domain.ConflictResult[];
	    logs: logging.Entry[];
	
	    static createFrom(source: any = {}) {
	        return new StatusSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workspaceReady = source["workspaceReady"];
	        this.watcherActive = source["watcherActive"];
	        this.snapshot = source["snapshot"];
	        this.fileCount = source["fileCount"];
	        this.conflicts = this.convertValues(source["conflicts"], domain.ConflictResult);
	        this.logs = this.convertValues(source["logs"], logging.Entry);
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
	
	export class ConflictResult {
	    recordId: string;
	    conflictType: string;
	    targetPath: string;
	    reason: string;
	    suggestion: string;
	
	    static createFrom(source: any = {}) {
	        return new ConflictResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.recordId = source["recordId"];
	        this.conflictType = source["conflictType"];
	        this.targetPath = source["targetPath"];
	        this.reason = source["reason"];
	        this.suggestion = source["suggestion"];
	    }
	}

}

export namespace logging {
	
	export class Entry {
	    // Go type: time
	    time: any;
	    level: string;
	    source: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = this.convertValues(source["time"], null);
	        this.level = source["level"];
	        this.source = source["source"];
	        this.message = source["message"];
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

