import { get, post, put, del, Unauthorized } from './api.js';
import { sign, create } from './webauthn.js';

const SECTIONS = { general: 'general', web: 'web', access: 'access', notifications: 'notifications', manual: 'manual-actions', commands: 'commands', scanner: 'scanner' };

// Settings: everything is read-only until the session is unlocked again with
// the passkey or the password; the unlock lasts a few minutes.
export function createSettings(session) {
	let data = $state(null);
	let types = $state([]);
	let error = $state(null);
	let now = $state(Date.now());
	let savedAt = $state(0);

	async function load() {
		try {
			const r = await get('/api/settings');
			data = r.settings;
			types = r.channel_types ?? [];
			error = null;
		} catch (e) {
			if (e instanceof Unauthorized) session.expired();
			else error = e;
		}
	}

	const unlockedUntil = () => (data?.unlocked_until ? new Date(data.unlocked_until).getTime() : 0);
	const setUnlocked = (r) => { if (data) data.unlocked_until = r.unlocked_until; };

	return {
		get data() { return data; },
		get types() { return types; },
		get error() { return error; },
		get unlocked() { return unlockedUntil() > now; },
		get justSaved() { return now - savedAt < 3000; },
		get secondsLeft() { return Math.max(0, Math.round((unlockedUntil() - now) / 1000)); },
		load,
		start() {
			load();
			const t = setInterval(() => (now = Date.now()), 1000);
			return () => clearInterval(t);
		},
		async unlockWithPasskey() {
			const options = await post('/api/unlock/begin');
			setUnlocked(await post('/api/unlock/finish', await sign(options)));
		},
		async unlockWithPassword(password, code = '') {
			setUnlocked(await post('/api/unlock/password', { password, code }));
		},
		async save(section, payload) {
			const r = await put('/api/settings/' + SECTIONS[section], payload);
			savedAt = Date.now();
			await load();
			return r;
		},
		async setPassword(current, password) {
			await post('/api/settings/password', { current, password });
			savedAt = Date.now();
			await load();
		},
		// Two-step verification. begin returns the secret once; the others ask for
		// both factors. Enable and renew do not reload: reloading remounts the
		// forms, and the recovery codes must stay on screen until they are saved.
		totpBegin: () => post('/api/settings/totp/begin'),
		totpEnable: (code, password) => post('/api/settings/totp/enable', { code, password }).then((r) => r.recovery_codes),
		totpRenew: (password, code) => post('/api/settings/totp/recovery', { password, code }).then((r) => r.recovery_codes),
		async totpDisable(password, code) {
			await post('/api/settings/totp/disable', { password, code });
			await load();
		},
		testChannel: (channel) => post('/api/settings/notifications/test', channel),
		async addPasskey(name) {
			const options = await post('/api/register/begin', { name });
			await post('/api/register/finish', await create(options));
			await load();
		},
		async removePasskey(id) {
			await del('/api/settings/passkeys/' + encodeURIComponent(id));
			await load();
		},
		scanHostKey: (host, port) => post('/api/settings/servers/hostkey', { host, port: Number(port) }),
		async saveServer(originalID, server, hostKey) {
			await put('/api/settings/servers', { original_id: originalID, server, host_key: hostKey });
			savedAt = Date.now();
			await load();
		},
		async removeServer(id) {
			await del('/api/settings/servers/' + encodeURIComponent(id));
			await load();
		},
		installScript: (id, panelURL, from, panelCommands, imageExport) => post(`/api/settings/servers/${encodeURIComponent(id)}/install`, { panel_url: panelURL, from, panel_commands: panelCommands, image_export: imageExport }),
		testServer: (id) => post(`/api/settings/servers/${encodeURIComponent(id)}/test`),
		async restart() {
			await post('/api/settings/restart');
		}
	};
}
