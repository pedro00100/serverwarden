<script>
	import { untrack } from 'svelte';
	import { t } from '#lib/i18n/index.svelte.js';
	import Section from './Section.svelte';

	let { settings } = $props();
	let list = $state(untrack(() => $state.snapshot(settings.data.manual_actions ?? [])));
</script>

<Section title={t('settings.tabs.manual')} help={t('settings.manual.help')} locked={!settings.unlocked} onSave={() => settings.save('manual', list)}>
	{#each list as m, i}
		<div class="row">
			<label class="field"><span>{t('settings.manual.name')}</span><input class="input" bind:value={m.name} required /></label>
			<label class="field"><span>{t('settings.manual.description')}</span><input class="input" bind:value={m.description} /></label>
			<label class="field wide"><span>{t('settings.manual.command')}</span><input class="input mono" bind:value={m.command} required /></label>
			<button type="button" class="btn danger" onclick={() => list.splice(i, 1)}>{t('settings.remove')}</button>
		</div>
	{:else}
		<p class="muted">{t('settings.manual.none')}</p>
	{/each}
	<div><button type="button" class="btn" onclick={() => list.push({ name: '', description: '', command: '' })}>{t('settings.manual.add')}</button></div>
</Section>

<style>
	.row { display: grid; grid-template-columns: 1fr 1.4fr; gap: 10px 12px; align-items: end; padding-bottom: 14px; border-bottom: 1px solid var(--border-soft); }
	.wide { grid-column: 1 / -1; }
	.row .btn { justify-self: start; }
	@media (max-width: 640px) { .row { grid-template-columns: 1fr; } }
</style>
