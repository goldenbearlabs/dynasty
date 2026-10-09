// The draft room's live state. The server sends the whole draft when the
// connection opens and then each change as it happens; this keeps a copy
// current.

import type { DraftMessage, DraftState } from '#lib/api.ts';
import { LiveSocket } from '#lib/socket.svelte.ts';

export class DraftRoom {
	state = $state<DraftState>();
	#socket: LiveSocket<DraftMessage>;

	constructor(id: string) {
		this.#socket = new LiveSocket(`/drafts/${id}/ws`, (message) => this.#apply(message));
	}

	get connected() {
		return this.#socket.connected;
	}

	#apply({ type, ...message }: DraftMessage) {
		if (type === 'state' || !this.state) {
			this.state = message;
			return;
		}
		// An update carries the draft header and only the picks that changed.
		this.state.draft = message.draft;
		this.state.on_clock_pick_id = message.on_clock_pick_id;
		for (const pick of message.picks) {
			const i = this.state.picks.findIndex((p) => p.id === pick.id);
			if (i >= 0) this.state.picks[i] = pick;
		}
	}

	close() {
		this.#socket.close();
	}
}
