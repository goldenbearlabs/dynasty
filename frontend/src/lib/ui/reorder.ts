// Drag to reorder: put use:reorder on a list and a [data-grip] handle in each
// of its items. Works with a mouse, a pen or a finger, and scrolls the list
// when the drag nears its edge. Styled in app.css.

export type ReorderOptions = {
	/** Called once the item is dropped somewhere new, with its old and new index. */
	onmove: (from: number, to: number) => void;
	disabled?: boolean;
};

const EDGE = 48; // how near the edge of the scroller, in px, a drag starts scrolling it
const SPEED = 14; // px scrolled per frame at the very edge

function scrollerOf(node: HTMLElement): HTMLElement {
	for (let el = node.parentElement; el; el = el.parentElement) {
		if (/auto|scroll/.test(getComputedStyle(el).overflowY) && el.scrollHeight > el.clientHeight) return el;
	}
	return document.documentElement;
}

export function reorder(list: HTMLElement, options: ReorderOptions) {
	let stop: (() => void) | undefined;

	function start(event: PointerEvent) {
		const grip = (event.target as Element).closest<HTMLElement>('[data-grip]');
		if (!grip || stop || options.disabled || event.button !== 0) return;
		const items = [...list.children] as HTMLElement[];
		const from = items.findIndex((item) => item.contains(grip));
		if (from < 0 || items.length < 2) return;
		event.preventDefault();

		// Where each item sits in the list, which stays true however it scrolls.
		const top = list.getBoundingClientRect().top;
		const boxes = items.map((item) => item.getBoundingClientRect());
		const tops = boxes.map((b) => b.top - top);
		const height = boxes[from].height;
		const grabbed = event.clientY - boxes[from].top;
		const end = tops[items.length - 1] + boxes[items.length - 1].height;
		const scroller = scrollerOf(list);
		const dragged = items[from];
		let y = event.clientY;
		let to = from;
		let frame = 0;

		function place() {
			const at = Math.max(0, Math.min(end - height, y - list.getBoundingClientRect().top - grabbed));
			const middle = at + height / 2;
			to = 0;
			items.forEach((item, i) => {
				if (i !== from && tops[i] + boxes[i].height / 2 < middle) to++;
			});
			items.forEach((item, i) => {
				const shift = i === from ? at - tops[from] : from < i && i <= to ? -height : to <= i && i < from ? height : 0;
				item.style.transform = shift ? `translateY(${shift}px)` : '';
			});
		}

		function tick() {
			if (!dragged.isConnected) return finish(false); // the list changed under the drag
			const box = scroller === document.documentElement ? { top: 0, bottom: innerHeight } : scroller.getBoundingClientRect();
			const over = Math.min(0, y - box.top - EDGE) + Math.max(0, y - box.bottom + EDGE);
			if (over) {
				scroller.scrollTop += (Math.max(-EDGE, Math.min(EDGE, over)) / EDGE) * SPEED;
				place();
			}
			frame = requestAnimationFrame(tick);
		}

		function finish(drop: boolean) {
			cancelAnimationFrame(frame);
			grip!.removeEventListener('pointermove', move);
			grip!.removeEventListener('pointerup', up);
			grip!.removeEventListener('pointercancel', cancel);
			grip!.removeEventListener('lostpointercapture', cancel);
			for (const item of items) item.style.transform = '';
			dragged.classList.remove('dragging');
			list.classList.remove('reordering');
			stop = undefined;
			if (drop && to !== from) options.onmove(from, to);
		}
		const move = (e: PointerEvent) => {
			y = e.clientY;
			place();
		};
		const up = () => finish(true);
		const cancel = () => finish(false);

		grip.setPointerCapture(event.pointerId);
		grip.addEventListener('pointermove', move);
		grip.addEventListener('pointerup', up);
		grip.addEventListener('pointercancel', cancel);
		grip.addEventListener('lostpointercapture', cancel);
		dragged.classList.add('dragging');
		list.classList.add('reordering');
		stop = cancel;
		frame = requestAnimationFrame(tick);
	}

	list.addEventListener('pointerdown', start);
	return {
		update(next: ReorderOptions) {
			options = next;
		},
		destroy() {
			stop?.();
			list.removeEventListener('pointerdown', start);
		}
	};
}
