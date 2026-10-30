import { get, post } from './api.js';

export function createSetup() {
	let status = $state(null);
	let error = $state(null);

	async function load() {
		try {
			status = await get('/api/setup/status');
			error = null;
		} catch (e) {
			error = e;
		}
	}

	return {
		get status() { return status; },
		get error() { return error; },
		load,
		async login(code) {
			await post('/api/setup/login', { code });
			await load();
		},
		complete: (payload) => post('/api/setup/complete', payload)
	};
}
