<script lang="ts" module>
	// Line icons drawn on a 24px grid. Add one by adding its path data here.
	const paths = {
		home: 'M3 11l9-8 9 8M5 10v10h5v-6h4v6h5V10',
		players: 'M16 19v-1a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4v1M9.5 10a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7M21 19v-1a4 4 0 0 0-3-3.85M15.5 3.15a3.5 3.5 0 0 1 0 6.7',
		draft: 'M8 6h13M8 12h13M8 18h13M3.5 6h.01M3.5 12h.01M3.5 18h.01',
		shield: 'M12 3l8 3v6c0 4.5-3.2 7.9-8 9-4.8-1.1-8-4.5-8-9V6l8-3z',
		settings: 'M4 6h10M18 6h2M4 12h2M10 12h10M4 18h10M18 18h2M16 4v4M8 10v4M16 16v4',
		search: 'M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14M20 20l-4-4',
		plus: 'M12 5v14M5 12h14',
		x: 'M6 6l12 12M18 6L6 18',
		check: 'M5 12.5l4.5 4.5L19 7',
		up: 'M6 14l6-6 6 6',
		down: 'M6 10l6 6 6-6',
		right: 'M9 6l6 6-6 6',
		clock: 'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18M12 7v5l3 2',
		trophy: 'M8 4h8v5a4 4 0 0 1-8 0V4zM8 6H5a3 3 0 0 0 3 4M16 6h3a3 3 0 0 1-3 4M12 13v4M8 20h8M10 17h4',
		link: 'M10 14a4 4 0 0 0 5.66 0l3-3a4 4 0 0 0-5.66-5.66l-1 1M14 10a4 4 0 0 0-5.66 0l-3 3a4 4 0 0 0 5.66 5.66l1-1',
		refresh: 'M20 11a8 8 0 0 0-14.5-4M4 5v3h3M4 13a8 8 0 0 0 14.5 4M20 19v-3h-3',
		undo: 'M9 7L4 12l5 5M4 12h10a5 5 0 0 1 0 10h-2',
		pause: 'M8 5v14M16 5v14',
		play: 'M7 5l12 7-12 7V5z',
		out: 'M15 4h4v16h-4M10 8l-4 4 4 4M6 12h10',
		queue: 'M4 6h12M4 12h12M4 18h7M17 15v6M14 18h6',
		trade: 'M7 4L3 8l4 4M3 8h14M17 12l4 4-4 4M21 16H7',
		lock: 'M6 11h12v9H6zM8.5 11V8a3.5 3.5 0 0 1 7 0v3',
		left: 'M15 6l-6 6 6 6',
		scores: 'M4 5h16v14H4zM12 5v14M7 10v4M17 10v4'
	};
	export type IconName = keyof typeof paths;
</script>

<script lang="ts">
	let { name, size = 18 }: { name: IconName; size?: number } = $props();
</script>

<svg
	width={size}
	height={size}
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width="1.9"
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true"
>
	<path d={paths[name]} />
</svg>

<style>
	svg {
		flex: none;
	}
</style>
