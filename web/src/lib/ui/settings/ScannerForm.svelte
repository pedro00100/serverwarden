<script>
	import { untrack } from 'svelte';
	import { t } from '#lib/i18n/index.svelte.js';
	import Section from './Section.svelte';

	let { settings } = $props();
	let draft = $state(untrack(() => $state.snapshot(settings.data.scanner)));
</script>

<Section title={t('settings.tabs.scanner')} help={t('settings.scanner.help')} locked={!settings.unlocked} onSave={() => settings.save('scanner', draft)}>
	<label class="check"><input type="checkbox" bind:checked={draft.enabled} />{t('settings.scanner.enabled')}</label>
	<div class="grid">
		<label class="field"><span>{t('settings.scanner.time')}</span><input class="input" type="time" bind:value={draft.time} required /></label>
		<label class="field">
			<span>{t('settings.scanner.timezone')}</span>
			<input class="input mono" bind:value={draft.timezone} required list="zones" />
			<span class="help">{t('settings.scanner.timezoneHelp')}</span>
		</label>
	</div>
	<datalist id="zones">{#each Intl.supportedValuesOf?.('timeZone') ?? [] as z}<option value={z}></option>{/each}</datalist>
	<label class="check"><input type="checkbox" bind:checked={draft.skip_images} />{t('settings.scanner.skipImages')}</label>
	<p class="help">{t('settings.scanner.skipImagesHelp')}</p>
	<p class="notice warning">{t('settings.scanner.memory')}</p>
</Section>

<style>
	.grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
	.help, .notice { margin: 0; }
	@media (max-width: 640px) { .grid { grid-template-columns: 1fr; } }
</style>
