<script>
	import MeshBackdrop from './primitives/MeshBackdrop.svelte';
	import { onMount, untrack } from 'svelte';
	import { t, errorText } from '#lib/i18n/index.svelte.js';
	import Logo from './primitives/Logo.svelte';

	let { recovery, code = '', authenticated } = $props();

	let authed = $state(untrack(() => authenticated));
	let error = $state('');
	let notice = $state('');
	let busy = $state(false);
	let web = $state(null);
	let allowed = $state('');
	let pw = $state('');

	async function attempt(fn) {
		busy = true;
		error = notice = '';
		try {
			await fn();
			notice = t('recovery.done');
		} catch (e) {
			error = errorText(e);
		} finally {
			busy = false;
		}
	}

	function fill() {
		if (recovery.status?.web) {
			web = $state.snapshot(recovery.status.web);
			allowed = web.allowed.join('\n');
		}
	}

	onMount(async () => {
		if (!authed && code) {
			try {
				await recovery.login(code);
				authed = true;
			} catch (e) {
				error = errorText(e);
			}
		} else if (authed) {
			await recovery.load();
		}
		fill();
	});

	const saveWeb = (ev) => {
		ev.preventDefault();
		attempt(async () => {
			await recovery.saveWeb({ ...web, allowed: allowed.split('\n').map((s) => s.trim()).filter(Boolean) });
			fill();
		});
	};
	const setPassword = (ev) => {
		ev.preventDefault();
		attempt(async () => { await recovery.setPassword(pw); pw = ''; });
	};
	const restart = () => attempt(async () => {
		await recovery.restart();
		setTimeout(() => location.reload(), 4000);
	});
</script>

<div class="page">
	<MeshBackdrop />
	<main class="box">
		<div class="brand">
			<Logo title="!" size={40} background="var(--surface)" />
			<div><h1>{t('recovery.title')}</h1><span class="label">{t('recovery.intro')}</span></div>
		</div>

		{#if !authed}
			<p class="notice warning">{t('recovery.needLink')}</p>
		{:else if recovery.status}
			{@const s = recovery.status}
			<section class="card block">
				<h2>{t('recovery.cause')}</h2>
				<code>{s.cause}</code>
				{#if s.load_error}<code>{s.load_error}</code>{/if}
				<span class="label">{t('recovery.file')}: <span class="mono">{s.settings_path}</span></span>
			</section>

			{#if web}
				<form class="card block" onsubmit={saveWeb}>
					<h2>{t('settings.tabs.web')}</h2>
					<label class="field"><span>{t('settings.web.origin')}</span><input class="input mono" bind:value={web.origin} required /></label>
					<label class="field"><span>{t('settings.web.listen')}</span><input class="input mono" bind:value={web.listen} required /></label>
					<label class="field"><span>{t('settings.web.certificate')}</span><input class="input mono" bind:value={web.certificate} required /></label>
					<label class="field"><span>{t('settings.web.key')}</span><input class="input mono" bind:value={web.key} required /></label>
					<label class="field">
						<span>{t('settings.web.allowed')}</span>
						<textarea class="input" rows="3" bind:value={allowed} required></textarea>
						<span class="help">{t('settings.web.allowedHelp', { ip: s.client_ip })}</span>
					</label>
					<div class="right"><button class="btn primary" disabled={busy}>{t('settings.save')}</button></div>
				</form>

				<form class="card block" onsubmit={setPassword}>
					<h2>{t('recovery.resetPassword')}</h2>
					<p class="help">{t('recovery.resetPasswordHelp')} {t('settings.access.passwordRule')}</p>
					<input class="input" type="password" minlength="12" autocomplete="new-password" bind:value={pw} required />
					<div class="right"><button class="btn primary" disabled={busy}>{t('settings.access.setPassword')}</button></div>
				</form>
			{:else}
				<p class="notice warning">{t('recovery.cannotEdit')}</p>
			{/if}

			{#if s.backup_available}
				<section class="card block">
					<h2>{t('recovery.backup')}</h2>
					<p class="help">{t('recovery.backupHelp')}</p>
					<div class="right"><button class="btn" disabled={busy} onclick={() => attempt(async () => { await recovery.restoreBackup(); fill(); })}>{t('recovery.backup')}</button></div>
				</section>
			{/if}

			<button class="btn primary big" disabled={busy} onclick={restart}>{t('recovery.tryAgain')}</button>
		{/if}

		{#if notice}<p class="notice ok" role="status">{notice}</p>{/if}
		{#if error}<p class="notice critical" role="alert">{error}</p>{/if}
	</main>
</div>

<style>
	.page { min-height: 100vh; display: flex; justify-content: center; padding: 32px 16px; background: var(--bg); }
	.box { position: relative; width: 100%; max-width: 640px; display: flex; flex-direction: column; gap: 16px; }
	.brand { display: flex; align-items: center; gap: 14px; }
	.brand div:last-child { display: flex; flex-direction: column; }
	h1 { font-size: 22px; }
	h2 { font-size: 16px; }
	.block { padding: 18px 20px; display: flex; flex-direction: column; gap: 12px; }
	.help { margin: 0; }
	code { display: block; padding: 10px 12px; border-radius: 10px; background: var(--code-bg); border: 1px solid var(--border-soft); font: 12px var(--font-mono); white-space: pre-wrap; overflow-wrap: anywhere; color: var(--critical-text); }
	.right { display: flex; justify-content: flex-end; }
	.big { min-height: 48px; }
	.notice { margin: 0; }
</style>
