import { tick } from 'svelte';

export const playerNavigation = $state<{ from: string; label: string; destination: string }>({
	from: '',
	label: 'Players',
	destination: '',
});

export function rememberPlayerOrigin(from: URL | null, to: URL | null) {
	if (
		!from ||
		!to ||
		!to.pathname.startsWith('/player/') ||
		from.pathname === to.pathname ||
		from.origin !== to.origin
	)
		return;
	const path = from.pathname;
	playerNavigation.from = path + from.search + from.hash;
	playerNavigation.destination = to.pathname;
	const sources: [string, string][] = [
		['/research', 'Research'],
		['/players', 'Players'],
		['/franchise/', 'Team'],
		['/watchlist', 'Watchlist'],
		['/draft', 'Draft'],
		['/trades', 'Trades'],
		['/lineup', 'Lineup'],
		['/player/', 'Previous player'],
		['/matchup', 'Matchups'],
	];
	playerNavigation.label =
		sources.find(([prefix]) => path.startsWith(prefix))?.[1] ?? 'Previous page';
	try {
		sessionStorage.setItem(`player-origin:${to.pathname}`, JSON.stringify(playerNavigation));
	} catch {
		/* Navigation still works when browser storage is disabled. */
	}
}

// The CSR router resets the current entry's snapshot during a full reload.
export function restorePlayerOrigin(url: URL) {
	const navigation = performance.getEntriesByType('navigation')[0] as
		PerformanceNavigationTiming | undefined;
	if (navigation?.type !== 'reload' && navigation?.type !== 'back_forward') return;
	try {
		const saved = JSON.parse(sessionStorage.getItem(`player-origin:${url.pathname}`) ?? 'null');
		if (
			saved?.destination === url.pathname &&
			typeof saved.from === 'string' &&
			saved.from.startsWith('/') &&
			!saved.from.startsWith('//') &&
			typeof saved.label === 'string'
		)
			Object.assign(playerNavigation, saved);
	} catch {
		/* A direct visit falls back to Players. */
	}
}

export function capturePageScroll() {
	return {
		path: location.pathname,
		x: window.scrollX,
		y: window.scrollY,
		regions: [
			...document.querySelectorAll<HTMLElement>(
				'main .scroll, main .table-wrap, main .activity-scroll',
			),
		].map((el) => ({ left: el.scrollLeft, top: el.scrollTop })),
	};
}

export function restorePageScroll(saved: ReturnType<typeof capturePageScroll>) {
	void tick().then(() =>
		requestAnimationFrame(() => {
			if (location.pathname !== saved.path) return;
			document
				.querySelectorAll<HTMLElement>('main .scroll, main .table-wrap, main .activity-scroll')
				.forEach((el, index) => {
					const s = saved.regions[index];
					if (s) {
						el.scrollLeft = s.left;
						el.scrollTop = s.top;
					}
				});
			window.scrollTo({ left: saved.x, top: saved.y, behavior: 'instant' });
		}),
	);
}
