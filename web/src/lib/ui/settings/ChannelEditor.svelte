<script>
	import { t, errorText } from '#lib/i18n/index.svelte.js';

	const MASKED = '••••••••';
	const LEVELS = ['info', 'warning', 'alert'];
	const KINDS = ['login', 'action', 'health', 'cve', 'settings', 'test'];
	const VARS = '{{.Icon}} {{.Title}} {{.Text}} {{.Server}} {{.Level}} {{.Kind}} {{.Panel}} {{.Time}} {{.Resolved}}';

	let { channel = $bindable(), type, settings, onRemove } = $props();

	let testState = $state('');
	let testError = $state('');

	function toggleKind(k) {
		channel.events = channel.events.includes(k) ? channel.events.filter((x) => x !== k) : [...channel.events, k];
	}

	async function test() {
		testState = 'testing';
		testError = '';
		try {
			await settings.testChannel($state.snapshot(channel));
			testState = 'ok';
		} catch (e) {
			testState = '';
			testError = errorText(e);
		}
	}
</script>

<details class="channel" open={!channel.name}>
	<summary>
		<span class="dot {channel.enabled ? 'ok' : ''}"></span>
		<strong>{channel.name || type?.name}</strong>
		<span class="label">{type?.name ?? channel.type}</span>
	</summary>
	<div class="body">
		<div class="grid">
			<label class="field"><span>{t('settings.notifications.name')}</span><input class="input" bind:value={channel.name} required maxlength="60" /></label>
			<label class="field">
				<span>{t('settings.notifications.minLevel')}</span>
				<select class="input" bind:value={channel.min_level}>{#each LEVELS as l}<option value={l}>{t('settings.notifications.levels.' + l)}</option>{/each}</select>
			</label>
		</div>
		<label class="check"><input type="checkbox" bind:checked={channel.enabled} />{t('settings.notifications.enabled')}</label>

		{#each type?.fields ?? [] as f}
			<label class="field">
				<span>{f.label}{f.required ? ' *' : ''}</span>
				{#if f.kind === 'select'}
					<select class="input" bind:value={channel.settings[f.key]}>{#each f.options as o}<option value={o}>{o}</option>{/each}</select>
				{:else}
					<input class="input" class:mono={f.kind !== 'text' || f.secret}
						type={f.secret ? 'password' : f.kind === 'number' ? 'number' : 'text'}
						autocomplete="off" placeholder={f.default ?? ''} required={f.required && !f.default}
						bind:value={channel.settings[f.key]} />
				{/if}
				{#if f.secret && channel.settings[f.key] === MASKED}<span class="help">{t('settings.notifications.secretSet')}</span>
				{:else if f.help}<span class="help">{f.help}</span>{/if}
			</label>
		{/each}

		<div class="field">
			<span>{t('settings.notifications.events')}</span>
			<div class="kinds">
				{#each KINDS as k}
					<label class="check"><input type="checkbox" checked={channel.events.includes(k)} onchange={() => toggleKind(k)} />{t('settings.notifications.kinds.' + k)}</label>
				{/each}
			</div>
			<span class="help">{t('settings.notifications.eventsHelp')}</span>
		</div>

		<label class="field">
			<span>{t('settings.notifications.template')}</span>
			<textarea class="input" rows="3" bind:value={channel.template} placeholder={'{{.Icon}} {{.Title}}\n{{.Text}}'}></textarea>
			<span class="help">{t('settings.notifications.templateHelp', { vars: VARS })}</span>
		</label>

		<div class="buttons">
			<button type="button" class="btn" onclick={test} disabled={testState === 'testing'}>{testState === 'testing' ? t('settings.notifications.testing') : t('settings.notifications.test')}</button>
			<button type="button" class="btn danger" onclick={onRemove}>{t('settings.remove')}</button>
			{#if testState === 'ok'}<span class="text-ok" role="status">{t('settings.notifications.testOk')}</span>{/if}
		</div>
		{#if testError}<p class="notice critical" role="alert">{testError}</p>{/if}
	</div>
</details>

<style>
	.channel { border: 1px solid var(--border); border-radius: 12px; overflow: hidden; }
	summary { display: flex; align-items: center; gap: 10px; padding: 12px 14px; cursor: pointer; list-style: none; }
	summary::-webkit-details-marker { display: none; }
	summary:hover { background: var(--row-hover); }
	.dot:not(.ok) { background: var(--border-strong); }
	.body { padding: 4px 14px 16px; display: flex; flex-direction: column; gap: 14px; }
	.grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
	.kinds { display: flex; flex-wrap: wrap; gap: 8px 16px; }
	.buttons { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; }
	.notice { margin: 0; }
	@media (max-width: 640px) { .grid { grid-template-columns: 1fr; } }
</style>
