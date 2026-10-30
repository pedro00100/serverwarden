// Pure functions that turn API data into what the views show. No UI, no text:
// statuses are codes the i18n layer translates.

export const pct = (u) => (u?.total ? (u.used * 100) / u.total : 0);

// The text inside a server's hexagon: its short label, or the start of its name.
export const nodeLabel = (server) => server.short || server.name.replace(/[^\p{L}\p{N}]/gu, '').slice(0, 4).toUpperCase();

export const usageTone = (v) => (v >= 90 ? 'var(--critical)' : v >= 80 ? 'var(--warning)' : 'var(--accent)');

const LEVEL_RANK = { critical: 0, warning: 1, ok: 2 };
export const worstFirst = (servers) => [...servers].sort((a, b) => (LEVEL_RANK[a.level] ?? 3) - (LEVEL_RANK[b.level] ?? 3));

export function primaryTemp(snapshot) {
	const t = snapshot?.temperatures?.find((x) => x.primary) ?? snapshot?.temperatures?.[0];
	return t ? t.celsius : null;
}

export function containerStatus(c) {
	if (c.health === 'unhealthy') return { level: 'critical', code: 'unhealthy' };
	if (c.state === 'restarting') return { level: 'critical', code: 'restarting' };
	if (c.state !== 'running') return { level: 'warning', code: 'stopped' };
	if (c.vault && !c.vault.initialized) return { level: 'warning', code: 'uninitialized' };
	if (c.vault?.sealed) {
		return c.vault.progress
			? { level: 'critical', code: 'unsealing', args: { progress: c.vault.progress, threshold: c.vault.threshold } }
			: { level: 'critical', code: 'sealed' };
	}
	if (c.vault) return { level: 'ok', code: 'unsealed' };
	if (c.health === 'starting') return { level: 'warning', code: 'starting' };
	return { level: 'ok', code: 'online' };
}

export function diskStatus(d) {
	if (d.virtual) return { level: 'muted', code: 'virtual' };
	if (d.healthy === false) return { level: 'critical', code: 'smartFailed' };
	if (d.reallocated > 0) return { level: 'warning', code: 'reallocated', args: { count: d.reallocated } };
	if (d.wear_pct >= 80) return { level: 'warning', code: 'worn', args: { pct: d.wear_pct } };
	if (d.healthy == null) return { level: 'muted', code: 'noData' };
	return { level: 'ok', code: 'healthy' };
}

export function summarize(view) {
	const servers = view.servers;
	const containers = servers.flatMap((s) => (s.snapshot?.containers ?? []).map((c) => ({ ...c, server: s.server.name })));
	const temps = servers.map((s) => primaryTemp(s.snapshot)).filter((t) => t != null);
	return {
		total: servers.length,
		online: servers.filter((s) => !s.error && s.snapshot).length,
		containers,
		running: containers.filter((c) => c.state === 'running' && c.health !== 'unhealthy').length,
		findings: servers.flatMap((s) => (s.findings ?? []).map((f) => ({ ...f, server: s.server.name }))),
		maxTemp: temps.length ? Math.max(...temps) : null
	};
}

export function topology(view) {
	return {
		edge: view.servers.filter((s) => s.server.layer === 'edge'),
		internal: view.servers.filter((s) => s.server.layer !== 'edge')
	};
}

// Adds up the 24 hourly CrowdSec alert counts of every server; null without any.
const hourlySum = (lists) => (lists.length ? lists[0].map((_, i) => lists.reduce((t, l) => t + l[i], 0)) : null);

export function securitySummary(view) {
	const withSec = view.servers.filter((s) => s.security);
	const sec = (s) => s.security;
	const updates = withSec.map((s) => sec(s).pending_updates).filter((n) => n != null);
	return {
		ready: withSec.length > 0,
		lynis: withSec.filter((s) => sec(s).lynis).map((s) => ({ name: s.server.name, ...sec(s).lynis })),
		crowdsec: withSec.find((s) => sec(s).crowdsec)?.security.crowdsec ?? null,
		alertsByHour: hourlySum(withSec.map((s) => sec(s).crowdsec?.alerts_by_hour).filter((h) => h?.length === 24)),
		logins: withSec
			.flatMap((s) => (sec(s).logins ?? []).map((l) => ({ ...l, server: s.server.name })))
			.sort((a, b) => new Date(b.at) - new Date(a.at)),
		automated: withSec.reduce((t, s) => t + (sec(s).automated_logins_24h ?? 0), 0),
		backups: withSec.flatMap((s) => sec(s).backups ?? []),
		pendingUpdates: updates.length ? updates.reduce((a, b) => a + b, 0) : null,
		audit: withSec
			.flatMap((s) => Object.entries(sec(s).audit ?? {}).map(([key, n]) => ({ key, n, server: s.server.name })))
			.sort((a, b) => b.n - a.n)
	};
}

export const hoursSince = (date) => (Date.now() - new Date(date)) / 3_600_000;

// Every security fact that has a time, newest first.
export function securityEvents(sec) {
	const events = [
		...sec.logins.map((l) => ({ kind: 'login', at: l.at, server: l.server, from: l.from, fingerprint: l.fingerprint ?? '', user: l.user ?? '', level: 'ok' })),
		...sec.backups.filter((b) => b.last).map((b) => ({ kind: 'backup', at: b.last, server: b.name, name: b.name, snapshots: b.snapshots, bytes: b.bytes, level: hoursSince(b.last) < 36 ? 'ok' : 'warning' })),
		...sec.lynis.filter((l) => l.date).map((l) => ({ kind: 'lynis', at: l.date, server: l.name, index: l.index, warnings: l.warnings, level: l.warnings ? 'warning' : 'ok' }))
	];
	return events
		.map((e) => ({ ...e, id: `${e.kind}/${e.server}/${e.at}` }))
		.sort((a, b) => new Date(b.at) - new Date(a.at));
}

// Groups the action catalog by server, in the order the servers are configured.
export function actionGroups(catalog, servers) {
	return servers
		.map((s) => ({ server: s.server, actions: catalog.filter((a) => a.server === s.server.id) }))
		.filter((g) => g.actions.length);
}
