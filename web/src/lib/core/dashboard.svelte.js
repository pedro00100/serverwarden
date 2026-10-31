import { get, Unauthorized } from './api.js';

const INTERVAL_MS = 10_000;

// Polls /api/state while signed in; pauses while the tab is hidden.
export function createDashboard(session) {
	let view = $state(null);
	let description = $state('');
	let update = $state(null);
	let error = $state(null);
	let updatedAt = $state(null);
	let now = $state(Date.now());

	async function load() {
		if (!session.authenticated) return;
		try {
			const r = await get('/api/state');
			view = r.view;
			description = r.description;
			update = r.update ?? null;
			error = null;
			updatedAt = Date.now();
		} catch (e) {
			if (e instanceof Unauthorized) {
				session.expired();
				view = null;
				return;
			}
			error = e;
		}
	}

	return {
		get view() { return view; },
		get description() { return description; },
		get update() { return update; },
		get error() { return error; },
		get secondsAgo() {
			return updatedAt ? Math.max(0, Math.round((now - updatedAt) / 1000)) : null;
		},
		refresh: load,
		clear() {
			view = null;
			error = null;
		},
		start() {
			let timer;
			const loop = () => {
				timer = setTimeout(async () => {
					if (!document.hidden) await load();
					loop();
				}, INTERVAL_MS);
			};
			loop();
			const clock = setInterval(() => (now = Date.now()), 1000);
			const onVisible = () => { if (!document.hidden) load(); };
			document.addEventListener('visibilitychange', onVisible);
			return () => {
				clearTimeout(timer);
				clearInterval(clock);
				document.removeEventListener('visibilitychange', onVisible);
			};
		}
	};
}
