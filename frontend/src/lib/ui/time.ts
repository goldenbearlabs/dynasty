// Short, human times for feeds and tables.

const units: [Intl.RelativeTimeFormatUnit, number][] = [
	['day', 86400],
	['hour', 3600],
	['minute', 60]
];
const relative = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto', style: 'short' });

/** "5 min. ago", "yesterday"; a date once it is more than a week old. */
export function ago(iso: string): string {
	const seconds = (new Date(iso).getTime() - Date.now()) / 1000;
	if (Math.abs(seconds) > 7 * 86400) {
		return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
	}
	for (const [unit, size] of units) {
		if (Math.abs(seconds) >= size) return relative.format(Math.round(seconds / size), unit);
	}
	return 'just now';
}

/** "Oct 7, 6:41 PM" */
export const clockTime = (iso: string) =>
	new Date(iso).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' });

/** Whole years since a birth date, or "" without one. */
export function age(birthDate: string | null): string {
	if (!birthDate) return '';
	return String(Math.floor((Date.now() - new Date(birthDate).getTime()) / (365.25 * 86400 * 1000)));
}

// ---- sports days, written YYYY-MM-DD ----

/** Parses a day as a local date, so formatting it never shifts it. */
const local = (day: string) => new Date(`${day}T12:00:00`);

/** "Wed, Oct 7" */
export const dayLabel = (day: string) =>
	local(day).toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' });

/** The day a number of days before or after. */
export function shiftDay(day: string, by: number): string {
	const date = local(day);
	date.setDate(date.getDate() + by);
	const pad = (n: number) => String(n).padStart(2, '0');
	return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

/** "7:30 PM" */
export const timeOfDay = (iso: string) => new Date(iso).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' });

/** Fantasy points to one decimal place. */
export const points = (n: number) => n.toLocaleString(undefined, { minimumFractionDigits: 1, maximumFractionDigits: 1 });
