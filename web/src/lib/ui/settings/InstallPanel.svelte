<script>
	import { t, errorText } from '#lib/i18n/index.svelte.js';

	let { settings, server, unpinned = false, onVerify, onClose } = $props();

	let panelURL = $state(location.origin);
	let from = $state('');
	let panelCommands = $state(false);
	let imageExport = $state(false);
	let script = $state('');
	let copied = $state(false);
	let busy = $state(false);
	let error = $state('');
	let tested = $state('');

	async function generate(ev) {
		ev.preventDefault();
		busy = true;
		error = '';
		try {
			script = (await settings.installScript(server.id, panelURL, from, panelCommands, imageExport)).script;
		} catch (e) {
			error = errorText(e);
		} finally {
			busy = false;
		}
	}

	async function copy() {
		try {
			await navigator.clipboard.writeText(script);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch {}
	}

	async function test() {
		tested = 'testing';
		error = '';
		try {
			await settings.testServer(server.id);
			tested = 'ok';
		} catch (e) {
			tested = '';
			error = errorText(e);
		}
	}
</script>

<div class="card panel rise">
	<h3>{t('settings.servers.installTitle')} · {server.name}</h3>
	<p class="help">{t('settings.servers.installHelp')}</p>
	<form onsubmit={generate} class="grid">
		<label class="field"><span>{t('settings.servers.panelURL')}</span><input class="input mono" bind:value={panelURL} required /></label>
		<label class="field">
			<span>{t('settings.servers.from')}</span>
			<input class="input mono" bind:value={from} placeholder="192.168.1.3" />
			<span class="help">{t('settings.servers.fromHelp')}</span>
		</label>
		<label class="wide opt">
			<span class="check"><input type="checkbox" bind:checked={panelCommands} />{t('settings.servers.panelCommands')}</span>
			<span class="help">{t('settings.servers.panelCommandsHelp')}</span>
		</label>
		<label class="wide opt">
			<span class="check"><input type="checkbox" bind:checked={imageExport} />{t('settings.servers.imageExport')}</span>
			<span class="help">{t('settings.servers.imageExportHelp')}</span>
		</label>
		<div class="buttons wide"><button class="btn primary" disabled={busy}>{t('settings.servers.generate')}</button></div>
	</form>
	{#if script}
		<div class="script">
			<pre>{script}</pre>
			<button class="btn copy" type="button" onclick={copy}>{copied ? t('settings.servers.copied') : t('settings.servers.copy')}</button>
		</div>
		<p class="help">{t('settings.servers.afterInstall')}</p>
	{/if}
	{#if unpinned}
		<div class="notice warning verify">
			<span>{t('settings.servers.unpinnedHelp')}</span>
			<button class="btn primary" type="button" onclick={onVerify}>{t('settings.servers.verify')}</button>
		</div>
	{/if}
	<div class="buttons">
		<button class="btn" type="button" onclick={onClose}>{t('settings.servers.close')}</button>
		<button class="btn" type="button" disabled={tested === 'testing' || unpinned} onclick={test}>{tested === 'testing' ? t('settings.servers.testing') : t('settings.servers.test')}</button>
		{#if tested === 'ok'}<span class="text-ok" role="status">{t('settings.servers.testOk')}</span>{/if}
	</div>
	{#if error}<p class="notice critical" role="alert">{error}</p>{/if}
</div>

<style>
	.opt { display: flex; flex-direction: column; gap: 4px; }
	.panel { padding: 18px 20px; display: flex; flex-direction: column; gap: 14px; }
	h3 { font-size: 15px; margin: 0; }
	.grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
	.wide { grid-column: 1 / -1; }
	.buttons { display: flex; gap: 10px; align-items: center; justify-content: flex-end; flex-wrap: wrap; }
	.help, .notice { margin: 0; }
	.verify { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
	.script { position: relative; }
	pre { margin: 0; max-height: 320px; overflow: auto; padding: 12px 14px; border-radius: 10px; background: var(--code-bg); border: 1px solid var(--border-soft); font: 12px/1.5 var(--font-mono); color: var(--text-2); }
	.copy { position: absolute; top: 8px; right: 8px; background: var(--surface); }
	@media (max-width: 640px) { .grid { grid-template-columns: 1fr; } }
</style>
