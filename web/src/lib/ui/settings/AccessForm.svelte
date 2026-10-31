<script>
	import { untrack } from 'svelte';
	import { t, errorText } from '#lib/i18n/index.svelte.js';
	import { dateTime } from '#lib/i18n/format.js';
	import Section from './Section.svelte';
	import TotpForm from './TotpForm.svelte';

	let { settings } = $props();
	let access = $derived(settings.data.access);
	let mode = $state(untrack(() => settings.data.access.mode));
	let username = $state(untrack(() => settings.data.access.username));
	let current = $state('');
	let pw = $state('');
	let pw2 = $state('');
	let keyName = $state('');
	let keyError = $state('');
	let keyBusy = $state(false);

	async function setPassword() {
		if (pw !== pw2) throw new Error(t('settings.access.mismatch'));
		await settings.setPassword(current, pw);
		current = pw = pw2 = '';
	}

	async function keyOp(fn) {
		keyBusy = true;
		keyError = '';
		try {
			await fn();
		} catch (e) {
			keyError = errorText(e);
		} finally {
			keyBusy = false;
		}
	}
</script>

<Section title={t('settings.access.mode')} locked={!settings.unlocked} onSave={() => settings.save('access', { mode, username })}>
	{#each ['passkey', 'password'] as m}
		<label class="option" class:active={mode === m}>
			<input type="radio" name="mode" value={m} bind:group={mode} />
			<span><strong>{t('settings.access.' + m)}</strong><span class="help">{t('settings.access.' + m + 'Help')}</span></span>
		</label>
	{/each}
	<label class="field"><span>{t('settings.access.username')}</span><input class="input" bind:value={username} autocomplete="username" required maxlength="64" /></label>
</Section>

<Section title={access.has_password ? t('settings.access.changePassword') : t('settings.access.newPassword')} help={t('settings.access.passwordRule')} locked={!settings.unlocked} onSave={setPassword} saveLabel={t('settings.access.setPassword')}>
	{#if access.has_password}
		<label class="field"><span>{t('settings.access.currentPassword')}</span><input class="input" type="password" autocomplete="current-password" bind:value={current} required /></label>
	{/if}
	<label class="field"><span>{t('settings.access.newPassword')}</span><input class="input" type="password" autocomplete="new-password" minlength="12" bind:value={pw} required /></label>
	<label class="field"><span>{t('settings.access.confirmPassword')}</span><input class="input" type="password" autocomplete="new-password" minlength="12" bind:value={pw2} required /></label>
</Section>

<TotpForm {settings} />

<Section title={t('settings.access.keys')} locked={!settings.unlocked}>
	{#each access.passkeys as k}
		<div class="key">
			<div class="text"><strong>{k.name}</strong><span class="label">{k.last_used && !k.last_used.startsWith('0001') ? t('settings.access.lastUsed', { date: dateTime(k.last_used, 'short') }) : t('settings.access.neverUsed')}</span></div>
			<button type="button" class="btn danger" disabled={keyBusy}
				onclick={() => confirm(t('settings.access.removeConfirm', { name: k.name })) && keyOp(() => settings.removePasskey(k.id))}>{t('settings.remove')}</button>
		</div>
	{:else}
		<p class="muted">{t('settings.access.noKeys')}</p>
	{/each}
	<div class="add">
		<input class="input" placeholder={t('settings.access.keyName')} bind:value={keyName} maxlength="40" />
		<button type="button" class="btn" disabled={keyBusy || !keyName} onclick={() => keyOp(async () => { await settings.addPasskey(keyName); keyName = ''; })}>{t('settings.access.addKey')}</button>
	</div>
	{#if keyError}<p class="notice critical" role="alert">{keyError}</p>{/if}
</Section>

<style>
	.option { display: flex; gap: 12px; align-items: flex-start; padding: 12px 14px; border: 1px solid var(--border); border-radius: 12px; cursor: pointer; }
	.option.active { border-color: var(--button); background: var(--accent-bg); }
	.option > span { display: flex; flex-direction: column; gap: 2px; }
	.option input { margin-top: 4px; }
	.key { display: flex; align-items: center; gap: 12px; padding: 8px 0; border-bottom: 1px solid var(--border-soft); }
	.text { display: flex; flex-direction: column; flex: 1 1 auto; min-width: 0; }
	.add { display: flex; gap: 10px; flex-wrap: wrap; }
	.add .input { flex: 1 1 200px; width: auto; }
	.notice { margin: 0; }
</style>
