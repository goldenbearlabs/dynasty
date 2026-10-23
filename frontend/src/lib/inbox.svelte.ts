import { getNotificationSummary, readNotification, readAllNotifications } from '#lib/api.ts';

class Inbox {
	unread = $state(0);
	latest = $state(0);
	revision = $state(0);
	account = '';
	private request = 0;
	setAccount(id: string) {
		if (this.account === id) return;
		this.account = id;
		this.request++;
		this.unread = 0;
		this.latest = 0;
		this.revision++;
		if (id) void this.refresh();
	}
	async refresh() {
		if (!this.account) return;
		const request = ++this.request;
		try {
			const summary = await getNotificationSummary();
			if (request !== this.request) return;
			if (summary.unread !== this.unread || summary.latest_id !== this.latest) {
				this.unread = summary.unread;
				this.latest = summary.latest_id;
				this.revision++;
			}
		} catch {
			/* A transient failure keeps the last known badge; the inbox shows fetch errors. */
		}
	}
	async read(id: number, read = true) {
		const account = this.account;
		await readNotification(id, read);
		if (account !== this.account) return;
		this.revision++;
		await this.refresh();
	}
	async readAll() {
		if (!this.latest) return;
		const account = this.account;
		await readAllNotifications(this.latest);
		if (account !== this.account) return;
		this.revision++;
		await this.refresh();
	}
}
export const inbox = new Inbox();
