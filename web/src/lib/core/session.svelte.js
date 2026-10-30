import { get, post } from './api.js';
import { create, sign } from './webauthn.js';

export function createSession() {
	let checked = $state(false);
	let authenticated = $state(false);
	let recovery = $state(false);
	let setup = $state(false);
	let mode = $state('passkey');
	let hasPasskeys = $state(true);
	let title = $state('');
	let language = $state('');
	let error = $state(null);

	return {
		get checked() { return checked; },
		get authenticated() { return authenticated; },
		get recovery() { return recovery; },
		get setup() { return setup; },
		get mode() { return mode; },
		get hasPasskeys() { return hasPasskeys; },
		get title() { return title; },
		get language() { return language; },
		get error() { return error; },

		async check() {
			try {
				const s = await get('/api/session');
				authenticated = s.authenticated;
				recovery = !!s.recovery;
				setup = !!s.setup;
				mode = s.mode ?? 'passkey';
				hasPasskeys = s.has_passkeys ?? true;
				title = s.title ?? '';
				language = s.language ?? '';
				error = null;
			} catch (e) {
				authenticated = false;
				error = e;
			} finally {
				checked = true;
			}
		},
		async login() {
			const options = await post('/api/login/begin');
			await post('/api/login/finish', await sign(options));
			authenticated = true;
		},
		// Returns true when the password was right and a code is needed next.
		async loginPassword(username, password) {
			const r = await post('/api/login/password', { username, password });
			if (r.second_factor) return true;
			authenticated = true;
			return false;
		},
		async loginCode(code) {
			await post('/api/login/code', { code });
			authenticated = true;
		},
		async register(code, name) {
			const options = await post('/api/register/begin', { code, name });
			await post('/api/register/finish', await create(options));
			hasPasskeys = true;
		},
		async logout() {
			try { await post('/api/logout'); } catch {}
			authenticated = false;
		},
		expired() {
			authenticated = false;
		}
	};
}
