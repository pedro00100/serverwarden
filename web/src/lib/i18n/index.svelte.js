import { ApiError } from '$lib/core/api.js';
import en from './en.js';
import es from './es.js';

const dictionaries = { en, es };
export const languages = [
	{ id: 'en', name: 'English' },
	{ id: 'es', name: 'Español' }
];

// The browser language is only used until the panel's language (Settings →
// General) is known.
const browser = (globalThis.navigator?.language ?? 'en').slice(0, 2);
let locale = $state(browser in dictionaries ? browser : 'en');

export const i18n = {
	get locale() { return locale; },
	set(l) {
		if (!(l in dictionaries)) return;
		locale = l;
		document.documentElement.lang = l;
	}
};

const lookup = (dict, key) => key.split('.').reduce((o, k) => o?.[k], dict);

// t('a.b', { n: 2 }) → the string at a.b with {n} replaced. Entries may also be
// functions of the arguments (for plurals). Missing keys fall back to English.
export function t(key, args = {}) {
	const v = lookup(dictionaries[locale], key) ?? lookup(en, key);
	if (typeof v === 'function') return v(args);
	if (typeof v !== 'string') return key;
	return v.replace(/\{(\w+)\}/g, (_, k) => args[k] ?? `{${k}}`);
}

// Findings come from the server with a code; unknown codes keep its English text.
export function findingText(f) {
	return lookup(en, 'findings.' + f.code) ? t('findings.' + f.code, f.args ?? {}) : f.text;
}

export function errorText(e) {
	if (e?.code && e.code !== 'invalid' && lookup(en, 'errors.code.' + e.code)) return t('errors.code.' + e.code);
	switch (e?.name) {
		case 'NotAllowedError': return t('errors.cancelled');
		case 'InvalidStateError': return t('errors.alreadyRegistered');
		case 'SecurityError': return t('errors.blocked');
	}
	if (e instanceof ApiError) {
		if (e.status === 403) return t('errors.forbidden');
		if (e.status === 429) return t('errors.tooMany');
		if (e.status === 401) return t('errors.notAccepted');
		return e.message;
	}
	if (e instanceof TypeError) return t('errors.offline');
	return e?.message || t('errors.generic');
}
