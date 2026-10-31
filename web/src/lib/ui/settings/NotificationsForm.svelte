<script>
	import { untrack } from 'svelte';
	import { t } from '#lib/i18n/index.svelte.js';
	import Section from './Section.svelte';
	import ChannelEditor from './ChannelEditor.svelte';

	let { settings } = $props();
	let draft = $state(untrack(() => $state.snapshot(settings.data.notifications)));
	let newType = $state(untrack(() => settings.types[0]?.id ?? ''));

	const typeOf = (id) => settings.types.find((x) => x.id === id);

	function add() {
		const type = typeOf(newType);
		const id = `${newType}-${crypto.getRandomValues(new Uint32Array(1))[0].toString(36)}`;
		const values = Object.fromEntries(type.fields.filter((f) => f.default).map((f) => [f.key, f.default]));
		draft.channels.push({ id, name: '', type: newType, enabled: true, min_level: 'warning', events: [], template: '', settings: values });
	}

	const save = () => settings.save('notifications', {
		alerts: { ...draft.alerts, pending_sec: Number(draft.alerts.pending_sec), repeat_min: Number(draft.alerts.repeat_min) },
		channels: draft.channels
	});
</script>

<Section title={t('settings.tabs.notifications')} locked={!settings.unlocked} onSave={save}>
	<h3>{t('settings.notifications.alerts')}</h3>
	<div class="grid">
		<label class="field">
			<span>{t('settings.notifications.pending')}</span>
			<input class="input" type="number" min="0" bind:value={draft.alerts.pending_sec} />
			<span class="help">{t('settings.notifications.pendingHelp')}</span>
		</label>
		<label class="field">
			<span>{t('settings.notifications.repeat')}</span>
			<input class="input" type="number" min="0" bind:value={draft.alerts.repeat_min} />
			<span class="help">{t('settings.notifications.repeatHelp')}</span>
		</label>
	</div>
	<label class="check"><input type="checkbox" bind:checked={draft.alerts.resolved} />{t('settings.notifications.resolved')}</label>

	<h3>{t('settings.notifications.channels')}</h3>
	{#each draft.channels as ch, i (ch.id)}
		<ChannelEditor bind:channel={draft.channels[i]} type={typeOf(ch.type)} {settings} onRemove={() => draft.channels.splice(i, 1)} />
	{:else}
		<p class="muted">{t('settings.notifications.noChannels')}</p>
	{/each}
	<div class="add">
		<select class="input" bind:value={newType} aria-label={t('settings.notifications.type')}>
			{#each settings.types as ty}<option value={ty.id}>{ty.name}</option>{/each}
		</select>
		<button type="button" class="btn" onclick={add}>{t('settings.notifications.addChannel')}</button>
	</div>
</Section>

<style>
	h3 { font-size: 14px; color: var(--text-2); margin-top: 4px; }
	.grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
	.add { display: flex; flex-wrap: wrap; gap: 10px; }
	.add .input { width: auto; flex: 0 1 220px; }
	@media (max-width: 640px) { .grid { grid-template-columns: 1fr; } }
</style>
