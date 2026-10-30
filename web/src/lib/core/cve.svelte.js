import { get, post, Unauthorized } from './api.js';

// UNKNOWN: Trivy has not rated it yet.
export const SEVERITIES = ['CRITICAL', 'HIGH', 'MEDIUM', 'LOW', 'UNKNOWN'];
export const FILTERS = ['all', 'critical', 'high', 'fixable'];

export function createCVE(session) {
	let report = $state(undefined); // undefined while loading, null before the first scan
	let scan = $state(null);
	let error = $state(null);

	async function load() {
		try {
			const r = await get('/api/cve');
			report = r.report;
			scan = r.scan ?? null;
			error = null;
		} catch (e) {
			if (e instanceof Unauthorized) session.expired();
			else error = e;
		}
	}

	return {
		get report() { return report; },
		get scan() { return scan; },
		get error() { return error; },
		async scanNow() {
			await post('/api/cve/scan');
			if (scan) scan.running = true;
			setTimeout(load, 2000);
		},
		start() {
			load();
			const t = setInterval(load, 10_000);
			return () => clearInterval(t);
		}
	};
}

export function totals(report) {
	const sum = (m) => SEVERITIES.reduce((t, s) => t + (m?.[s] ?? 0), 0);
	return { total: sum(report?.count), fixable: sum(report?.fixable) };
}

export function matches(v, filter) {
	switch (filter) {
		case 'fixable': return !!v.fixed;
		case 'critical': return v.severity === 'CRITICAL';
		case 'high': return v.severity === 'HIGH';
		default: return true;
	}
}

// One tile per scanned target for the heat map: the worst first, sized by how
// many vulnerabilities it has compared with the biggest one.
export function heatTiles(scans) {
	const rank = (w) => (w ? SEVERITIES.indexOf(w) : SEVERITIES.length);
	const tiles = scans
		.map((scan) => ({
			scan,
			key: `${scan.server}/${scan.kind}/${scan.target}`,
			total: SEVERITIES.reduce((t, s) => t + (scan.count?.[s] ?? 0), 0),
			worst: SEVERITIES.find((s) => scan.count?.[s]) ?? null
		}))
		.sort((a, b) => rank(a.worst) - rank(b.worst) || b.total - a.total);
	const max = Math.max(1, ...tiles.map((x) => x.total));
	return tiles.map((x, i) => ({
		...x,
		size: i === 0 && x.total && tiles.length > 2 ? 'big' : x.total >= max * 0.3 && x.total ? 'wide' : ''
	}));
}
