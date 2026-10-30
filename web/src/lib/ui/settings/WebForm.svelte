<script>
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n/index.svelte.js';
	import Section from './Section.svelte';

	let { settings } = $props();
	let draft = $state(untrack(() => $state.snapshot(settings.data.web)));
	let allowed = $state(untrack(() => settings.data.web.allowed.join('\n')));

	const save = () => settings.save('web', { ...draft, allowed: allowed.split('\n').map((s) => s.trim()).filter(Boolean) });
</script>

<Section title={t('settings.tabs.web')} locked={!settings.unlocked} onSave={save}>
	<label class="field">
		<span>{t('settings.web.origin')}</span>
		<input class="input mono" bind:value={draft.origin} required placeholder="https://panel.example.net" />
		<span class="help">{t('settings.web.originHelp')}</span>
	</label>
	<label class="field">
		<span>{t('settings.web.listen')}</span>
		<input class="input mono" bind:value={draft.listen} required placeholder=":8443" />
		<span class="help">{t('settings.web.listenHelp')}</span>
	</label>
	<label class="field"><span>{t('settings.web.certificate')}</span><input class="input mono" bind:value={draft.certificate} required /></label>
	<label class="field"><span>{t('settings.web.key')}</span><input class="input mono" bind:value={draft.key} required /></label>
	<label class="field">
		<span>{t('settings.web.allowed')}</span>
		<textarea class="input" bind:value={allowed} rows="4" required></textarea>
		<span class="help">{t('settings.web.allowedHelp', { ip: settings.data.client_ip })}</span>
	</label>
</Section>
