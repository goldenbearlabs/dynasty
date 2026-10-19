// The trade market for the signed-in manager, loaded once and kept current
// as they change it: who is on each block, what each manager wants, and the
// targets this manager has set or attracted.
import { getTradeMarket, setTradeBlock, setTradeTarget, setTradeWants, type TradeMarket } from '#lib/api.ts';
import { toast } from '#lib/ui/toast.svelte.ts';

const empty = (): TradeMarket => ({ block: [], wants: [], targets: [], interest: [] });

class Market {
	data = $state<TradeMarket>(empty());
	loaded = $state(false);
	busy = $state(false);
	#account = '';

	/** Follows who is signed in: a new manager sees their own market. */
	setAccount(id: string) {
		if (id === this.#account) return;
		this.#account = id;
		this.data = empty();
		this.loaded = false;
		if (id) void this.refresh();
	}
	async refresh() {
		if (!this.#account) return;
		try {
			this.data = await getTradeMarket();
			this.loaded = true;
		} catch (e) {
			toast.error(e);
		}
	}

	onBlock = (leagueId: string, playerId: string) => this.data.block.find((b) => b.league_id === leagueId && b.player_id === playerId);
	targeted = (leagueId: string, playerId: string) => this.data.targets.some((t) => t.league_id === leagueId && t.player_id === playerId);
	/** The franchises after one of this manager's players. */
	interestIn = (leagueId: string, playerId: string) =>
		this.data.interest.filter((t) => t.league_id === leagueId && t.player_id === playerId).map((t) => t.by_franchise_id);

	async #change(work: () => Promise<void>, done: string) {
		if (this.busy) return;
		this.busy = true;
		try {
			await work();
			await this.refresh();
			toast.good(done);
		} catch (e) {
			toast.error(e);
		} finally {
			this.busy = false;
		}
	}
	block(leagueId: string, playerId: string, on: boolean, note = '', name = 'He') {
		return this.#change(
			() => setTradeBlock({ league_id: leagueId, player_id: playerId, on, note }),
			on ? `${name} is on your trade block.` : `${name} is off your trade block.`
		);
	}
	target(leagueId: string, playerId: string, on: boolean, name = 'He') {
		return this.#change(
			() => setTradeTarget({ league_id: leagueId, player_id: playerId, on }),
			on ? `${name} is a target. His manager can see you are interested.` : `${name} is no longer a target.`
		);
	}
	wants(leagueId: string, positions: string[], note: string) {
		return this.#change(() => setTradeWants({ league_id: leagueId, positions, note }), 'Wants saved.');
	}
}

export const tradeMarket = new Market();
