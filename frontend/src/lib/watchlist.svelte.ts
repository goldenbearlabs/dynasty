import { getWatchlist, addWatchlistPlayer, removeWatchlistPlayer, editWatchlistPlayer, orderWatchlist, type WatchlistEntry } from '#lib/api.ts';

class Watchlist {
 entries = $state<WatchlistEntry[]>([]);
 account = $state('');
 loaded = $state(false);
 loading = $state(false);
 busy = $state(false);
 error = $state('');
 private revision = 0;
 setAccount(id: string) {
  if (id === this.account) return;
  this.account = id; this.entries = []; this.loaded = false; this.error = ''; this.busy = false;
  this.revision++;
  if (id) void this.refresh();
 }
 async refresh() {
  if (!this.account) return;
  const revision = ++this.revision;
  this.loading = true;
  try { const entries = await getWatchlist(); if (revision === this.revision) { this.entries = entries; this.loaded = true; this.error = ''; } }
  catch (e) { if (revision === this.revision) this.error = e instanceof Error ? e.message : 'Could not load watchlist.'; }
  finally { if (revision === this.revision) this.loading = false; }
 }
 has(id: string) { return this.entries.some(p => p.player_id === id); }
 async mutate(action: () => Promise<void>) {
  if (this.busy || !this.account) return;
  const account = this.account;
  this.busy = true;
  try { await action(); if (account === this.account) await this.refresh(); }
  finally { if (account === this.account) this.busy = false; }
 }
 toggle(id: string) { return this.mutate(() => this.has(id) ? removeWatchlistPlayer(id) : addWatchlistPlayer(id)); }
 edit(id: string, category: string, notes: string) { return this.mutate(() => editWatchlistPlayer(id, category, notes)); }
 move(id: string, offset: number) {
  const ids = this.entries.map(p => p.player_id), from = ids.indexOf(id), to = from + offset;
  if (from < 0 || to < 0 || to >= ids.length) return Promise.resolve();
  ids.splice(from, 1); ids.splice(to, 0, id);
  return this.mutate(() => orderWatchlist(ids));
 }
}
export const watchlist = new Watchlist();
