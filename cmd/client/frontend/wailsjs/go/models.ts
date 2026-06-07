export namespace config {
	
	export class GameConfig {
	    game_id: string;
	    invite_token: string;
	    client_token: string;
	    game_name: string;
	    vassal_module: string;
	    player_name: string;
	
	    static createFrom(source: any = {}) {
	        return new GameConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.game_id = source["game_id"];
	        this.invite_token = source["invite_token"];
	        this.client_token = source["client_token"];
	        this.game_name = source["game_name"];
	        this.vassal_module = source["vassal_module"];
	        this.player_name = source["player_name"];
	    }
	}
	export class ClientConfig {
	    watch_dir: string;
	    server_url: string;
	    game_id: string;
	    invite_token: string;
	    client_token: string;
	    game_name: string;
	    vassal_module: string;
	    player_name: string;
	    games: Record<string, GameConfig>;
	
	    static createFrom(source: any = {}) {
	        return new ClientConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.watch_dir = source["watch_dir"];
	        this.server_url = source["server_url"];
	        this.game_id = source["game_id"];
	        this.invite_token = source["invite_token"];
	        this.client_token = source["client_token"];
	        this.game_name = source["game_name"];
	        this.vassal_module = source["vassal_module"];
	        this.player_name = source["player_name"];
	        this.games = this.convertValues(source["games"], GameConfig, true);
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

export namespace models {
	
	export class CreateGameResponse {
	    game_id: string;
	    invite_token: string;
	    invite_url: string;
	    vassal_module: string;
	    game_name: string;
	    client_token: string;
	    turn_order: number;
	
	    static createFrom(source: any = {}) {
	        return new CreateGameResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.game_id = source["game_id"];
	        this.invite_token = source["invite_token"];
	        this.invite_url = source["invite_url"];
	        this.vassal_module = source["vassal_module"];
	        this.game_name = source["game_name"];
	        this.client_token = source["client_token"];
	        this.turn_order = source["turn_order"];
	    }
	}
	export class Game {
	    id: string;
	    invite_token: string;
	    name: string;
	    vassal_module: string;
	    status: string;
	    current_turn_index: number;
	    host_player_id: string;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Game(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.invite_token = source["invite_token"];
	        this.name = source["name"];
	        this.vassal_module = source["vassal_module"];
	        this.status = source["status"];
	        this.current_turn_index = source["current_turn_index"];
	        this.host_player_id = source["host_player_id"];
	        this.created_at = this.convertValues(source["created_at"], null);
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
	export class Player {
	    id: string;
	    game_id: string;
	    name: string;
	    email: string;
	    client_token?: string;
	    turn_order: number;
	
	    static createFrom(source: any = {}) {
	        return new Player(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.game_id = source["game_id"];
	        this.name = source["name"];
	        this.email = source["email"];
	        this.client_token = source["client_token"];
	        this.turn_order = source["turn_order"];
	    }
	}
	export class GameState {
	    game: Game;
	    players: Player[];
	    current_player?: Player;
	    last_date_saved?: string;
	    your_turn: boolean;
	    your_turn_order: number;
	
	    static createFrom(source: any = {}) {
	        return new GameState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.game = this.convertValues(source["game"], Game);
	        this.players = this.convertValues(source["players"], Player);
	        this.current_player = this.convertValues(source["current_player"], Player);
	        this.last_date_saved = source["last_date_saved"];
	        this.your_turn = source["your_turn"];
	        this.your_turn_order = source["your_turn_order"];
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
	export class JoinResponse {
	    game_id: string;
	    client_token: string;
	    game_name: string;
	    vassal_module: string;
	    turn_order: number;
	
	    static createFrom(source: any = {}) {
	        return new JoinResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.game_id = source["game_id"];
	        this.client_token = source["client_token"];
	        this.game_name = source["game_name"];
	        this.vassal_module = source["vassal_module"];
	        this.turn_order = source["turn_order"];
	    }
	}

}

