<script lang="ts">
	// A bar between two panes that is dragged to resize them. It reports
	// where the pointer is and leaves the arithmetic to the owner of the
	// layout. Arrow keys nudge it; a double-click puts it back.
	type Props = {
		/** "vertical" stands between side-by-side panes; "horizontal" between stacked ones. */
		orientation: 'vertical' | 'horizontal';
		label: string;
		onmove: (x: number, y: number) => void;
		/** Called with -1 or +1 for an arrow key towards the start or the end. */
		onnudge: (direction: number) => void;
		onreset: () => void;
	};
	let { orientation, label, onmove, onnudge, onreset }: Props = $props();

	let dragging = $state(false);

	function start(event: PointerEvent) {
		if (event.button !== 0) return;
		dragging = true;
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
		event.preventDefault(); // no text selection while dragging
	}
	function move(event: PointerEvent) {
		if (dragging) onmove(event.clientX, event.clientY);
	}
	function key(event: KeyboardEvent) {
		const back = orientation === 'vertical' ? 'ArrowLeft' : 'ArrowUp';
		const forward = orientation === 'vertical' ? 'ArrowRight' : 'ArrowDown';
		if (event.key !== back && event.key !== forward) return;
		event.preventDefault();
		onnudge(event.key === back ? -1 : 1);
	}
</script>

<!-- A focusable separator is the standard pattern for a resize handle. -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
<div
	class="splitter {orientation}"
	class:dragging
	role="separator"
	aria-orientation={orientation}
	aria-label={label}
	title="Drag to resize. Double-click to reset."
	tabindex="0"
	onpointerdown={start}
	onpointermove={move}
	onpointerup={() => (dragging = false)}
	onpointercancel={() => (dragging = false)}
	ondblclick={onreset}
	onkeydown={key}
></div>

<style>
	.splitter {
		position: relative;
		flex: none;
		touch-action: none;
		background: transparent;
	}
	.vertical {
		cursor: col-resize;
	}
	.horizontal {
		cursor: row-resize;
	}
	/* A short grip, so the bar can be found without drawing a heavy line. */
	.splitter::after {
		content: '';
		position: absolute;
		inset: 0;
		margin: auto;
		border-radius: 3px;
		background: var(--rule-strong);
		transition: background 0.12s;
	}
	.vertical::after {
		width: 3px;
		height: 2.2rem;
	}
	.horizontal::after {
		width: 2.2rem;
		height: 3px;
	}
	.splitter:hover::after,
	.splitter:focus-visible::after,
	.dragging::after {
		background: var(--brand);
	}
	.splitter:focus-visible {
		outline: none;
	}
</style>
