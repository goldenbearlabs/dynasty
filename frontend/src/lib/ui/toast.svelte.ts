// Brief messages that confirm an action or report why it was refused.
// Call toast.good(...) or toast.bad(...) from anywhere; <Toaster> shows them.

type Toast = { id: number; kind: 'good' | 'bad'; text: string };

let nextId = 0;

class Toasts {
	items = $state<Toast[]>([]);

	good = (text: string) => this.show('good', text, 3000);
	bad = (text: string) => this.show('bad', text, 6000);

	/** Reports a failed action; use as `.catch(toast.error)`. */
	error = (e: unknown) => this.bad(e instanceof Error ? e.message : String(e));

	dismiss = (id: number) => {
		this.items = this.items.filter((t) => t.id !== id);
	};

	private show(kind: Toast['kind'], text: string, ms: number) {
		const id = nextId++;
		this.items.push({ id, kind, text });
		setTimeout(() => this.dismiss(id), ms);
	}
}

export const toast = new Toasts();
