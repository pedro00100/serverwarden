import { i18n, t } from './index.svelte.js';

export const number = (n, digits = 1) => n.toLocaleString(i18n.locale, { maximumFractionDigits: digits, minimumFractionDigits: 0 });

export function bytes(b) {
	if (!b) return '0 B';
	const units = ['B', 'KB', 'MB', 'GB', 'TB'];
	let i = 0;
	while (b >= 1024 && i < units.length - 1) { b /= 1024; i++; }
	return `${number(b, b < 10 ? 1 : 0)} ${units[i]}`;
}

export function duration(sec) {
	if (sec == null) return '—';
	const d = Math.floor(sec / 86400), h = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
	if (d) return `${d} d ${h} h`;
	if (h) return `${h} h ${m} min`;
	return `${m} min`;
}

export function ago(sec) {
	if (sec == null) return t('time.never');
	if (sec < 5) return t('time.now');
	if (sec < 60) return t('time.secondsAgo', { n: sec });
	return t('time.minutesAgo', { n: Math.round(sec / 60) });
}

export function agoDate(date) {
	const h = (Date.now() - new Date(date)) / 3_600_000;
	if (h < 1) return t('time.minutesAgo', { n: Math.max(1, Math.round(h * 60)) });
	if (h < 48) return t('time.hoursAgo', { n: Math.round(h) });
	return t('time.daysAgo', { n: Math.round(h / 24) });
}

export const celsius = (c) => (c == null ? '—' : `${number(c, 0)} °C`);

const styles = {
	long: { day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit' },
	short: { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' },
	today: { weekday: 'long', day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit' }
};
export const dateTime = (d, style = 'long') => new Date(d).toLocaleString(i18n.locale, styles[style]);
export const clock = (d) => new Date(d).toLocaleTimeString(i18n.locale, { hour: '2-digit', minute: '2-digit' });
