<script lang="ts">
	// Time left until a moment, as m:ss, ticking down.
	let { until }: { until: string } = $props();

	let now = $state(Date.now());
	$effect(() => {
		const timer = setInterval(() => (now = Date.now()), 250);
		return () => clearInterval(timer);
	});

	const seconds = $derived(Math.max(0, Math.ceil((new Date(until).getTime() - now) / 1000)));
	const text = $derived.by(() => {
		const hours = Math.floor(seconds / 3600);
		const minutes = Math.floor((seconds % 3600) / 60);
		const rest = String(seconds % 60).padStart(2, '0');
		return hours > 0 ? `${hours}:${String(minutes).padStart(2, '0')}:${rest}` : `${minutes}:${rest}`;
	});
</script>

<span class="countdown" class:urgent={seconds <= 10} role="timer" aria-label="Time left to pick">{text}</span>

<style>
	.countdown {
		font: 750 2.4rem/1 var(--display);
		font-variant-numeric: tabular-nums;
		letter-spacing: -0.02em;
	}
	.urgent {
		color: var(--bad);
	}
</style>
