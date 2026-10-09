// A listening connection to one of the server's live feeds. The server
// sends a full snapshot when the connection opens and again whenever
// something changes; this reconnects if the connection drops.

export class LiveSocket<T> {
	connected = $state(false);

	#path: string;
	#onmessage: (message: T) => void;
	#socket?: WebSocket;
	#retries = 0;
	#closed = false;
	#retryTimer?: ReturnType<typeof setTimeout>;

	/** path is the feed's address under /api, such as "/scores/nhl/ws". */
	constructor(path: string, onmessage: (message: T) => void) {
		this.#path = path;
		this.#onmessage = onmessage;
		this.#open();
	}

	#open() {
		if (this.#closed) return;
		const scheme = location.protocol === 'https:' ? 'wss' : 'ws';
		const socket = new WebSocket(`${scheme}://${location.host}/api${this.#path}`);
		socket.onopen = () => {
			this.connected = true;
			this.#retries = 0;
		};
		socket.onmessage = (event) => this.#onmessage(JSON.parse(event.data));
		socket.onclose = () => {
			this.connected = false;
			if (this.#closed) return;
			// Back off up to 15 seconds; a fresh connection starts from a snapshot.
			this.#retryTimer = setTimeout(() => this.#open(), Math.min(1000 * 2 ** this.#retries++, 15000));
		};
		this.#socket = socket;
	}

	close() {
		this.#closed = true;
		clearTimeout(this.#retryTimer);
		this.#socket?.close();
	}
}

/**
 * Calls refresh whenever the scores of any of the given sports change. Use
 * it in a component that shows fantasy points, so they follow the games
 * without the page ever polling. Must be called while a component is being
 * set up, since it ties the connections to that component's life.
 */
export function onScoresChange(sports: () => string[], refresh: () => void) {
	$effect(() => {
		let timer: ReturnType<typeof setTimeout>;
		const sockets = sports().map((sport) => {
			let first = true;
			return new LiveSocket(`/scores/${sport}/ws`, () => {
				// The first message is only the snapshot of where things stand.
				if (first) return void (first = false);
				// Several sports can change at once; refresh once for all of them.
				clearTimeout(timer);
				timer = setTimeout(refresh, 400);
			});
		});
		return () => {
			clearTimeout(timer);
			sockets.forEach((s) => s.close());
		};
	});
}
