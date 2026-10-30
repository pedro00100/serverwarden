import { get, post, put } from './api.js';

export function createRecovery() {
	let status = $state(null);
	let error = $state(null);

	async function load() {
		try {
			status = await get('/api/recovery/status');
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
			await post('/api/recovery/login', { code });
			await load();
		},
		async saveWeb(web) { await put('/api/recovery/web', web); await load(); },
		async saveGeneral(general) { await put('/api/recovery/general', general); await load(); },
		async setPassword(password) { await post('/api/recovery/password', { password }); await load(); },
		async restoreBackup() { await post('/api/recovery/restore-backup'); await load(); },
		restart: () => post('/api/recovery/restart')
	};
}
