<script>
	import { untrack } from 'svelte';
	import { t, errorText } from '$lib/i18n/index.svelte.js';

	// original: the server being edited, or null to add one. verify: go straight
	// to confirming the host key of a server the panel does not trust yet.
	let { settings, original = null, verify = false, onSaved, onCancel } = $props();

	const blank = { id: '', name: '', short: '', role: '', location: '', layer: 'internal', ssh: { host: '', port: 22, user: 'root' } };
	let srv = $state(untrack(() => $state.snapshot(original) ?? structuredClone(blank)));
	let phase = $state('details'); // details | verify
	let hostKey = $state(null);
	let confirmed = $state(false);
	let busy = $state(false);
	let error = $state('');

	const slug = (s) => s.toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '').replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 32);
	const moved = () => !original || original.ssh.host !== srv.ssh.host || Number(original.ssh.port) !== Number(srv.ssh.port);
	const needsKey = () => moved() || settings.data.unpinned?.includes(original.id);

	async function attempt(fn) {
		busy = true;
		error = '';
		try {
			await fn();
		} catch (e) {
			error = errorText(e);
		} finally {
			busy = false;
		}
	}

	function submitDetails(ev) {
		ev.preventDefault();
		if (!srv.id) srv.id = slug(srv.name);
		srv.ssh.port = Number(srv.ssh.port);
		if (!needsKey()) return attempt(save);
		scan();
	}

	function scan() {
		attempt(async () => {
			hostKey = await settings.scanHostKey(srv.ssh.host, srv.ssh.port);
			confirmed = false;
			phase = 'verify';
		});
	}

	async function save() {
		await settings.saveServer(original?.id ?? '', $state.snapshot(srv), needsKey() ? hostKey.key : '');
		onSaved(srv.id, !original);
	}

	if (verify) untrack(scan);
</script>

<div class="card editor rise">
	{#if phase === 'details'}
		<form onsubmit={submitDetails}>
			<h3>{original ? t('settings.servers.edit') + ' · ' + original.name : t('settings.servers.add')}</h3>
			<div class="grid">
				<label class="field"><span>{t('settings.servers.name')}</span><input class="input" bind:value={srv.name} required maxlength="40" /></label>
				<label class="field">
					<span>{t('settings.servers.id')}</span>
					<input class="input mono" bind:value={srv.id} placeholder={slug(srv.name)} pattern="[a-z0-9][a-z0-9_\-]{'{0,31}'}" />
					<span class="help">{t('settings.servers.idHelp')}</span>
				</label>
				<label class="field"><span>{t('settings.servers.short')}</span><input class="input" bind:value={srv.short} maxlength="6" /></label>
				<label class="field"><span>{t('settings.servers.role')}</span><input class="input" bind:value={srv.role} maxlength="60" /></label>
				<label class="field"><span>{t('settings.servers.location')}</span><input class="input" bind:value={srv.location} maxlength="40" /></label>
				<label class="field">
					<span>{t('settings.servers.layer')}</span>
					<select class="input" bind:value={srv.layer}>{#each ['internal', 'edge'] as l}<option value={l}>{t('settings.servers.layers.' + l)}</option>{/each}</select>
				</label>
				<label class="field"><span>{t('settings.servers.host')}</span><input class="input mono" bind:value={srv.ssh.host} required /></label>
				<label class="field"><span>{t('settings.servers.port')}</span><input class="input" type="number" min="1" max="65535" bind:value={srv.ssh.port} required /></label>
				<label class="field wide">
					<span>{t('settings.servers.user')}</span>
					<input class="input mono" bind:value={srv.ssh.user} required />
					<span class="help">{t('settings.servers.userHelp')}</span>
				</label>
			</div>
			<div class="buttons">
				<button type="button" class="btn" onclick={onCancel}>{t('settings.cancel')}</button>
				<button class="btn primary" disabled={busy}>{needsKey() ? t('settings.servers.next') : t('settings.servers.saveServer')}</button>
			</div>
		</form>
	{:else}
		<h3>{t('settings.servers.hostKeyTitle')}</h3>
		<p class="help">{t('settings.servers.hostKeyHelp')}</p>
		<code class="fingerprint">{hostKey.fingerprint}</code>
		<code>ssh-keygen -lf /etc/ssh/ssh_host_{hostKey.key.startsWith('ssh-ed25519') ? 'ed25519' : hostKey.key.startsWith('ecdsa') ? 'ecdsa' : 'rsa'}_key.pub</code>
		<label class="check"><input type="checkbox" bind:checked={confirmed} />{t('settings.servers.confirmKey')}</label>
		<div class="buttons">
			<button type="button" class="btn" onclick={() => (phase = 'details')}>{t('setup.back')}</button>
			<button class="btn primary" disabled={!confirmed || busy} onclick={() => attempt(save)}>{t('settings.servers.saveServer')}</button>
		</div>
	{/if}
	{#if error}<p class="notice critical" role="alert">{error}</p>{/if}
</div>

<style>
	.editor { padding: 18px 20px; display: flex; flex-direction: column; gap: 14px; }
	form { display: flex; flex-direction: column; gap: 14px; }
	h3 { font-size: 15px; margin: 0; }
	.grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
	.wide { grid-column: 1 / -1; }
	.buttons { display: flex; gap: 10px; justify-content: flex-end; flex-wrap: wrap; }
	.help, .notice { margin: 0; }
	code { display: block; padding: 10px 12px; border-radius: 10px; background: var(--code-bg); border: 1px solid var(--border-soft); font: 12px var(--font-mono); overflow-x: auto; white-space: nowrap; }
	.fingerprint { font-size: 14px; color: var(--accent-2); }
	@media (max-width: 640px) { .grid { grid-template-columns: 1fr; } }
</style>
