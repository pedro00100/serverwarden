<script>
	import { untrack } from 'svelte';
	import { t, languages, i18n } from '#lib/i18n/index.svelte.js';
	import Section from './Section.svelte';

	let { settings } = $props();
	let draft = $state(untrack(() => $state.snapshot(settings.data.general)));

	async function save() {
		await settings.save('general', { ...draft, interval_sec: Number(draft.interval_sec) });
		i18n.set(draft.language);
	}
</script>

<Section title={t('settings.tabs.general')} locked={!settings.unlocked} onSave={save}>
	<label class="field"><span>{t('settings.general.title')}</span><input class="input" bind:value={draft.title} maxlength="60" required /></label>
	<label class="field"><span>{t('settings.general.description')}</span><input class="input" bind:value={draft.description} maxlength="200" /></label>
	<label class="field">
		<span>{t('settings.general.language')}</span>
		<select class="input" bind:value={draft.language}>{#each languages as l}<option value={l.id}>{l.name}</option>{/each}</select>
		<span class="help">{t('settings.general.languageHelp')}</span>
	</label>
	<label class="field"><span>{t('settings.general.interval')}</span><input class="input" type="number" min="5" max="3600" bind:value={draft.interval_sec} required /></label>
</Section>
