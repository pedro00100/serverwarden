<script>
	import { t, errorText } from '#lib/i18n/index.svelte.js';

	// onSave is async; children receive nothing and bind to their own draft state.
	let { title, help = '', locked, onSave = null, saveLabel = '', children } = $props();

	let phase = $state('idle'); // idle | saving | saved | error
	let error = $state('');

	async function save(ev) {
		ev.preventDefault();
		phase = 'saving';
		error = '';
		try {
			await onSave();
			phase = 'saved';
			setTimeout(() => { if (phase === 'saved') phase = 'idle'; }, 2500);
		} catch (e) {
			phase = 'error';
			error = errorText(e);
		}
	}
</script>

<form class="card rise section" onsubmit={save}>
	<div class="head">
		<h2>{title}</h2>
		{#if help}<p class="help">{help}</p>{/if}
	</div>
	<fieldset disabled={locked}>
		{@render children()}
	</fieldset>
	{#if onSave}
		<div class="actions">
			<button class="btn primary" type="submit" disabled={locked || phase === 'saving'}>
				{phase === 'saving' ? t('settings.saving') : saveLabel || t('settings.save')}
			</button>
		</div>
	{/if}
	{#if error}<p class="notice critical" role="alert">{error}</p>{/if}
</form>

<style>
	.section { padding: 20px 22px; display: flex; flex-direction: column; gap: 16px; }
	.head { display: flex; flex-direction: column; gap: 4px; }
	h2 { font-size: 17px; }
	.help { margin: 0; }
	fieldset { border: 0; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 14px; min-width: 0; }
	.actions { display: flex; align-items: center; justify-content: flex-end; gap: 12px; }
	.notice { margin: 0; }
</style>
