import { get, post, Unauthorized } from './api.js';
import { sign } from './webauthn.js';

const key = (a) => a.server + '/' + a.id;

export function createActions(session) {
	let catalog = $state([]);
	let manual = $state([]);
	let status = $state({}); // key → { phase: 'signing'|'running'|'done'|'failed', message?, output?, error? }
	let busy = $state(false);

	return {
		get catalog() { return catalog; },
		get manual() { return manual; },
		get busy() { return busy; },
		statusOf: (a) => status[key(a)],

		async load() {
			try {
				const r = await get('/api/actions');
				catalog = r.actions ?? [];
				manual = r.manual ?? [];
			} catch (e) {
				if (e instanceof Unauthorized) session.expired();
			}
		},

		// With passkeys, the key signs this exact action and the target server verifies
		// the signature again. With a password, the session must be unlocked first.
		async run(a) {
			const k = key(a);
			busy = true;
			try {
				let r;
				if (session.mode === 'password') {
					status[k] = { phase: 'running' };
					r = await post('/api/action/run', { server: a.server, action: a.id });
				} else {
					status[k] = { phase: 'signing' };
					const options = await post('/api/action/begin', { server: a.server, action: a.id });
					const signature = await sign(options);
					status[k] = { phase: 'running' };
					r = await post('/api/action/finish', signature);
				}
				status[k] = r.ok ? { phase: 'done', output: r.output } : { phase: 'failed', message: r.message, output: r.output };
			} catch (e) {
				status[k] = { phase: 'failed', error: e };
			} finally {
				busy = false;
			}
		}
	};
}
