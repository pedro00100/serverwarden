// Sample API responses for `make dev`: lets anyone work on the UI without
// servers, agents or a security key.

import { readFileSync } from 'node:fs';

const now = () => new Date().toISOString();
const daysAgo = (d) => new Date(Date.now() - d * 86_400_000).toISOString();
const hoursAgo = (h) => new Date(Date.now() - h * 3_600_000).toISOString();

const tools = { smartctl: true, systemctl: true, journalctl: true, restic: true, cscli: false, lynis: true, aureport: false, 'apt-get': true, dnf: false, checkupdates: false, apk: false, docker: true };

function snapshot(o) {
	return {
		hostname: o.hostname, os: 'Debian GNU/Linux 13 (trixie)', kernel: '6.12.48-1-amd64',
		uptime_sec: o.uptime ?? 3 * 86400 + 7200, cores: o.cores ?? 4, load: o.load ?? [0.32, 0.41, 0.38],
		cpu_pct: o.cpu ?? 12, cpu_steal_pct: o.steal ?? 0,
		memory: { total: o.mem ?? 8 * 2 ** 30, used: (o.mem ?? 8 * 2 ** 30) * (o.memPct ?? 0.42) },
		swap: { total: 2 * 2 ** 30, used: 120 * 2 ** 20 },
		disk: { total: o.disk ?? 240 * 2 ** 30, used: (o.disk ?? 240 * 2 ** 30) * (o.diskPct ?? 0.37) },
		temperatures: o.temp == null ? [] : [{ sensor: 'coretemp', label: 'Package id 0', celsius: o.temp, primary: true }],
		disks: o.disks ?? [], containers: o.containers ?? [], failed_units: [], reboot_required: !!o.reboot,
		tools, actions: o.actions ?? [], panel_commands: !!o.panelCommands, agent_version: o.agent ?? '0.7.4', taken_at: now()
	};
}

const container = (name, image, extra = {}) => ({ name, image, state: 'running', health: 'healthy', status: 'Up 3 days (healthy)', ...extra });
const finding = (level, code, args, text) => ({ level, code, args, text });

const servers = [
	{
		server: { id: 'gateway', name: 'gateway', short: 'GW', role: 'Reverse proxy · VPN', location: 'Cloud VPS', layer: 'edge' },
		snapshot: snapshot({ hostname: 'gateway', panelCommands: true, cores: 2, mem: 2 * 2 ** 30, memPct: 0.61, steal: 2.4, disks: [{ device: 'vda', model: 'Virtual disk', virtual: true }],
			containers: [container('caddy', 'caddy:2.10'), container('uptime', 'louislam/uptime-kuma:2')],
			actions: [{ id: 'check-updates', name: 'Check for updates', description: 'Refreshes the package list (installs nothing)' }] }),
		security: { taken_at: now(), lynis: { index: 78, warnings: 0, date: daysAgo(1) }, crowdsec: { local_bans: 12, community_bans: 18342, alerts_24h: 61,
				alerts_by_hour: [1, 0, 2, 1, 3, 2, 1, 0, 1, 2, 14, 9, 3, 2, 1, 1, 2, 3, 2, 1, 4, 3, 2, 1] },
			logins: [{ at: hoursAgo(2), from: '10.0.0.5', fingerprint: 'SHA256:k3Yq0pX2mE8sJ1vR7nT4', hardware: true, user: 'root' }],
			automated_logins_24h: 288, backups: [], pending_updates: 0, audit: { sshd_config: 1 } },
		level: 'ok', findings: []
	},
	{
		server: { id: 'home', name: 'home-server', short: 'HOME', role: 'Media · home automation', location: 'Home', layer: 'internal' },
		snapshot: snapshot({ hostname: 'home-server', panelCommands: true, temp: 83, diskPct: 0.88,
			disks: [{ device: 'sda', model: 'WDC WD10EZEX', healthy: true, celsius: 38, reallocated: 147, power_on_hours: 31200 }],
			containers: [container('jellyfin', 'jellyfin/jellyfin:10.11'), container('homeassistant', 'ghcr.io/home-assistant/home-assistant:2026.10'), container('mosquitto', 'eclipse-mosquitto:2', { state: 'exited', health: '', status: 'Exited (0) 2 hours ago' })],
			actions: [{ id: 'restart-jellyfin', name: 'Restart Jellyfin', description: 'Interrupts ongoing playback for a few seconds' }] }),
		security: { taken_at: now(), lynis: { index: 71, warnings: 2, date: daysAgo(1) }, logins: [], automated_logins_24h: 288, backups: [], pending_updates: 4, audit: {} },
		level: 'warning',
		findings: [
			finding('warning', 'container_stopped', { name: 'mosquitto' }, 'Container mosquitto is stopped'),
			finding('warning', 'disk_reallocated', { device: 'sda', count: 147 }, 'Disk sda: 147 reallocated sectors'),
			finding('warning', 'cpu_temp', { celsius: 83 }, 'CPU at 83 °C'),
			finding('warning', 'disk_usage', { pct: 88 }, 'Disk at 88%'),
			finding('warning', 'lynis_warnings', null, 'Lynis reports security warnings')
		]
	},
	{
		server: { id: 'nas', name: 'nas', short: 'NAS', role: 'Storage · backups', location: 'Home', layer: 'internal' },
		snapshot: snapshot({ hostname: 'nas', agent: '0.7.3', temp: 41, mem: 16 * 2 ** 30,
			disks: [{ device: 'nvme0n1', model: 'Samsung SSD 980 1TB', healthy: true, celsius: 44, power_on_hours: 9100, wear_pct: 6 }],
			containers: [container('vault', 'hashicorp/vault:1.21', { vault: { initialized: true, sealed: false, version: '1.21.0', progress: 0, threshold: 3 } })],
			actions: [{ id: 'backup-now', name: 'Back up now', description: 'Runs the nightly restic backup immediately' }] }),
		security: { taken_at: now(), lynis: { index: 81, warnings: 0, date: daysAgo(1) }, logins: [], automated_logins_24h: 12,
			backups: [{ name: 'gateway', last: hoursAgo(5), snapshots: 21, bytes: 734 * 2 ** 20 }, { name: 'home-server', last: daysAgo(3), snapshots: 18, bytes: 2.1 * 2 ** 30 }],
			pending_updates: 0, audit: { passwd: 2, sudoers: 1 } },
		level: 'warning',
		findings: [
			finding('warning', 'backup_old', { name: 'home-server' }, 'Backup home-server is more than a day old'),
			finding('warning', 'agent_version', { agent: '0.7.3', panel: '0.7.4' }, 'Agent 0.7.3 does not match the panel (0.7.4): run its install script again (Settings › Servers)')
		]
	}
];

// Password mode, so settings can be unlocked in the browser with any password.
let unlockedUntil = 0;
const unlock = () => {
	unlockedUntil = Date.now() + 5 * 60_000;
	return { ok: true, unlocked_until: new Date(unlockedUntil).toISOString() };
};

const language = process.env.MOCK_LANG || 'en';

const settings = {
	general: { title: 'Homelab', description: 'Sample data: run against a real panel to see your own servers.', language, interval_sec: 15, update_check: true },
	web: { listen: ':8443', origin: 'https://panel.home.arpa', certificate: '/etc/panel/cert.pem', key: '/etc/panel/key.pem', allowed: ['192.168.1.0/24', '127.0.0.1/32'] },
	access: { mode: 'password', username: 'admin', has_password: true, totp: false, recovery_left: 0, passkeys: [{ id: 'abc', name: 'Main key', created: daysAgo(30), last_used: hoursAgo(3) }] },
	notifications: {
		alerts: { pending_sec: 120, repeat_min: 360, resolved: true },
		channels: [
			{ id: 'discord-1', name: 'Discord', type: 'discord', enabled: true, min_level: 'warning', events: [], template: '', settings: { url: '••••••••' } },
			{ id: 'ntfy-1', name: 'Phone (ntfy)', type: 'ntfy', enabled: true, min_level: 'alert', events: ['health', 'login'], template: '', settings: { server: 'https://ntfy.sh', topic: 'homelab-alerts', token: '' } }
		]
	},
	scanner: { enabled: true, time: '02:30', timezone: 'Europe/Berlin', skip_images: false },
	manual_actions: [{ name: 'Reboot a server', description: 'Stops every service for a minute or two', command: 'sudo systemctl reboot' }],
	commands: [
		{ id: 'restart-mosquitto', name: 'Restart mosquitto', description: "Brings Home Assistant's MQTT broker back", command: 'docker restart mosquitto', servers: ['home'] },
		{ id: 'disk-usage', name: 'What fills the disk', description: '', command: 'df -h / && du -sh /var/lib/docker', servers: [] }
	],
	servers: servers.map((s) => ({ ...s.server, ssh: { host: '192.168.1.' + (10 + s.server.id.length), port: 22, user: 'root' } })),
	unpinned: ['nas'],
	restart_required: [],
	client_ip: '192.168.1.5'
};

const channelTypes = [
	{ id: 'discord', name: 'Discord', fields: [{ key: 'url', label: 'Webhook URL', kind: 'url', secret: true, required: true, help: 'Channel settings → Integrations → Webhooks.' }] },
	{ id: 'telegram', name: 'Telegram', fields: [{ key: 'token', label: 'Bot token', kind: 'text', secret: true, required: true }, { key: 'chat_id', label: 'Chat ID', kind: 'text', required: true }] },
	{ id: 'ntfy', name: 'ntfy', fields: [{ key: 'server', label: 'Server', kind: 'url', required: true, default: 'https://ntfy.sh' }, { key: 'topic', label: 'Topic', kind: 'text', required: true }, { key: 'token', label: 'Access token', kind: 'text', secret: true }] },
	{ id: 'email', name: 'Email (SMTP)', fields: [{ key: 'host', label: 'SMTP server', kind: 'text', required: true }, { key: 'port', label: 'Port', kind: 'number', default: '587' }, { key: 'security', label: 'Security', kind: 'select', default: 'starttls', options: ['starttls', 'tls', 'none'] }, { key: 'password', label: 'Password', kind: 'text', secret: true }] }
];

const sampleQR = readFileSync(new URL('./sample-qr.svg', import.meta.url), 'utf8');
const recoveryCodes = ['k7m2-p9qa-x4rt', 'h3nd-82vc-wq5e', 'z6tf-m4kp-2yrb', 'c9xw-5hgn-t7ad', 'e2qv-r8mz-k3uf',
	'w5bj-n6tc-9pdh', 'f4ya-3kqx-m8ev', 'u7rc-d2wn-6hsg', 'p3mt-x9fe-4kbz', 'g8vd-y5ar-2nqc'];
const validCode = (c) => c?.replace(/\D/g, '') === '123456' || c?.replace(/[^a-z0-9]/gi, '').length === 12;

// MOCK_SCREEN=setup shows the first-run wizard, MOCK_SCREEN=login the sign-in page; POST /api/mock/screen switches it at runtime.
let screen = process.env.MOCK_SCREEN || 'panel';

// Changes made through the mock API last until /api/mock/reset (ui-check calls it first).
const initial = structuredClone(settings);

export const responses = {
	'/api/mock/screen': (body) => { screen = body?.screen || 'panel'; return { screen }; },
	'/api/mock/reset': () => {
		for (const k of Object.keys(settings)) delete settings[k];
		Object.assign(settings, structuredClone(initial));
		screen = process.env.MOCK_SCREEN || 'panel';
		unlockedUntil = 0;
		return { ok: true };
	},
	'/api/session': () => screen === 'setup'
		? { setup: true, authenticated: true }
		: { authenticated: screen !== 'login', mode: 'password', has_passkeys: true, title: 'Homelab', language },
	'/api/setup/status': () => ({ client_ip: '192.168.1.5', suggested_origin: 'https://192.168.1.10:8443', suggested_allowed: ['192.168.1.0/24', '127.0.0.1/32'], listen: ':8443' }),
	'/api/setup/complete': () => ({ ok: true, origin: 'https://192.168.1.10:8443' }),
	'/api/settings/servers/hostkey': () => ({ key: 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMockKeyForTheDevServerOnly', fingerprint: 'SHA256:Zm9vYmFyYmF6cXV4cXV1eHF1dXpxdXV4eHh4eHh4eHg' }),
	'/api/settings/servers/gateway/install': () => ({ script: '#!/bin/sh\n# Sample install script (mock)\nset -eu\necho "Agent installed."\n' }),
	'/api/settings/servers/gateway/test': () => ({ ok: true }),
	'/api/settings/servers': (body) => {
		settings.unpinned = settings.unpinned.filter((id) => id !== body?.original_id);
		return { ok: true };
	},
	'/api/settings': () => ({ settings: { ...settings, unlocked_until: new Date(unlockedUntil).toISOString() }, channel_types: channelTypes }),
	'/api/unlock/password': unlock,
	// Two-step verification: the mock accepts 123456 or any 12-character recovery code.
	'/api/login/password': () => (settings.access.totp ? { second_factor: true } : { ok: true }),
	'/api/login/code': (b) => (validCode(b?.code) ? { ok: true } : { httpStatus: 401, code: 'bad_code', error: 'wrong or expired code' }),
	'/api/settings/totp/begin': () => ({ secret: 'JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP', uri: 'otpauth://totp/Homelab:admin?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP', qr: sampleQR, now: new Date().toISOString() }),
	'/api/settings/totp/enable': (b) => {
		if (!validCode(b?.code)) return { httpStatus: 400, code: 'bad_code', error: 'wrong or expired code' };
		settings.access.totp = true;
		settings.access.recovery_left = 10;
		return { recovery_codes: recoveryCodes };
	},
	'/api/settings/totp/recovery': (b) => (validCode(b?.code) ? ((settings.access.recovery_left = 10), { recovery_codes: recoveryCodes }) : { httpStatus: 400, code: 'bad_code', error: 'wrong or expired code' }),
	'/api/settings/totp/disable': (b) => (validCode(b?.code) ? ((settings.access.totp = false), (settings.access.recovery_left = 0), { ok: true }) : { httpStatus: 400, code: 'bad_code', error: 'wrong or expired code' }),
	'/api/action/run': (body) => ({
		ok: true, message: 'Done',
		output: body?.action?.startsWith('cmd:') ? 'Filesystem  Size  Used Avail Use% Mounted on\n/dev/sda2   240G  211G   29G  88% /\n38G\t/var/lib/docker' : 'Hit:1 http://deb.debian.org/debian trixie InRelease\nReading package lists... Done'
	}),
	'/api/settings/commands': (body) => {
		settings.commands = body ?? [];
		return { ok: true };
	},
	'/api/state': () => ({
		view: { servers, level: 'warning', score: 76, generated_at: now() },
		description: 'Sample data: run against a real panel to see your own servers.',
		update: settings.general.update_check ? { version: '0.8.0', url: 'https://github.com/pedro00100/serverwarden/releases/tag/v0.8.0' } : null
	}),
	'/api/actions': () => ({
		actions: [
			...servers.flatMap((s) => s.snapshot.actions.map((a) => ({ ...a, server: s.server.id }))),
			...settings.commands.flatMap((c) => servers
				.filter((s) => s.snapshot.panel_commands && (!c.servers.length || c.servers.includes(s.server.id)))
				.map((s) => ({ id: 'cmd:' + c.id, server: s.server.id, name: c.name, description: c.description, command: c.command })))
		],
		manual: [
			{ name: 'Reboot a server', description: 'Stops every service for a minute or two', command: 'sudo systemctl reboot' },
			{ name: 'Update a container image', description: 'May change data or configuration', command: 'docker compose pull && docker compose up -d' }
		]
	}),
	'/api/cve/scan': () => ({ ok: true }),
	'/api/cve': () => ({
		scan: { enabled: true, running: false, last_start: hoursAgo(6.1), last_end: hoursAgo(6), results: 3, skipped: [{ server: 'nas', target: 'backup-tool:dev', code: 'not_exported' }, { server: 'gateway', code: 'unreachable' }], next_run: new Date(Date.now() + 18 * 3_600_000).toISOString() },
		report: {
			generated_at: hoursAgo(6), count: { CRITICAL: 1, HIGH: 7, MEDIUM: 40, LOW: 112, UNKNOWN: 1 }, fixable: { CRITICAL: 1, HIGH: 2, MEDIUM: 5, LOW: 0 },
			scans: [
				{ server: 'home', kind: 'image', target: 'jellyfin/jellyfin:10.11', date: hoursAgo(6), count: { CRITICAL: 1, HIGH: 3 },
					vulnerabilities: [
						{ id: 'CVE-2026-1234', package: 'libexpat1', installed: '2.6.2-1', fixed: '2.6.2-2', severity: 'CRITICAL', title: 'Heap overflow in XML parser', url: 'https://avd.aquasec.com/nvd/cve-2026-1234' },
						{ id: 'CVE-2026-2222', package: 'openssl', installed: '3.5.1-1', fixed: '', severity: 'HIGH', title: 'Timing side channel', url: '' }
					] },
				{ server: 'gateway', kind: 'system', target: 'System packages', date: hoursAgo(6), count: { MEDIUM: 2 },
					vulnerabilities: [{ id: 'CVE-2025-9999', package: 'tar', installed: '1.35', fixed: '', severity: 'MEDIUM', title: 'Path traversal', url: '' }] },
				{ server: 'nas', kind: 'image', target: 'local/panel:dev', date: hoursAgo(6), count: { UNKNOWN: 1 },
					vulnerabilities: [{ id: 'GO-2026-5932', package: 'golang.org/x/crypto', installed: 'v0.57.0', fixed: '', severity: 'UNKNOWN', title: 'The openpgp package is unmaintained', url: 'https://pkg.go.dev/vuln/GO-2026-5932' }] },
				{ server: 'nas', kind: 'image', target: 'hashicorp/vault:1.21', date: hoursAgo(6), count: {}, vulnerabilities: [] }
			]
		}
	})
};
