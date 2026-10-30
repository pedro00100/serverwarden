<script>
	import { t, errorText } from '$lib/i18n/index.svelte.js';

	let { settings, mode } = $props();

	let password = $state('');
	let code = $state('');
	let busy = $state(false);
	let totp = $derived(settings.data?.access?.totp);
	let error = $state('');

	const mmss = (s) => `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;

	async function unlock(ev) {
		ev?.preventDefault();
		busy = true;
		error = '';
		try {
			if (mode === 'password') await settings.unlockWithPassword(password, code);
			else await settings.unlockWithPasskey();
			password = code = '';
		} catch (e) {
			error = errorText(e);
		} finally {
			busy = false;
		}
	}
</script>

{#if settings.unlocked}
	<div class="bar card open" role="status">
		<svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="var(--ok)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="11" width="16" height="10" rx="2" /><path d="M8 11V7a4 4 0 0 1 7.5-2" /></svg>
		<span>{t('settings.unlockedFor', { time: mmss(settings.secondsLeft) })}</span>
	</div>
{:else}
	<form class="bar card" onsubmit={unlock}>
		<svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="var(--warning)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="11" width="16" height="10" rx="2" /><path d="M8 11V7a4 4 0 0 1 8 0v4" /></svg>
		<span class="text">{t('settings.locked')}</span>
		{#if mode === 'password'}
			<input class="input pw" type="password" autocomplete="current-password" aria-label={t('settings.passwordPrompt')} placeholder={t('settings.passwordPrompt')} bind:value={password} required />
			{#if totp}
				<input class="input otp" inputmode="numeric" autocomplete="one-time-code" spellcheck="false" maxlength="14" aria-label={t('settings.codePrompt')} placeholder={t('settings.codePrompt')} bind:value={code} required />
			{/if}
			<button class="btn primary" type="submit" disabled={busy}>{t('settings.unlock')}</button>
		{:else}
			<button class="btn primary" type="submit" disabled={busy}>{busy ? t('settings.unlocking') : t('settings.unlockPasskey')}</button>
		{/if}
		{#if error}<p class="notice critical err" role="alert">{error}</p>{/if}
	</form>
{/if}

<style>
	.bar { padding: 14px 18px; display: flex; flex-wrap: wrap; align-items: center; gap: 12px; }
	.bar.open { border-color: var(--ok-bg); color: var(--ok-text); font-weight: 500; }
	.text { flex: 1 1 260px; color: var(--text-2); }
	.pw { width: auto; flex: 0 1 220px; }
	.otp { width: auto; flex: 0 1 150px; font-family: var(--font-mono); letter-spacing: 0.08em; }
	.err { flex: 1 1 100%; margin: 0; }
</style>
