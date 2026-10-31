<script>
	import { untrack } from 'svelte';
	import { t } from '#lib/i18n/index.svelte.js';
	import { AGENT_CONFIG } from '#lib/core/brand.js';
	import Section from './Section.svelte';

	let { settings, view } = $props();
	let list = $state(untrack(() => $state.snapshot(settings.data.commands ?? [])));

	let servers = $derived(settings.data.servers ?? []);
	const accepts = (id) => !!view?.servers.find((s) => s.server.id === id)?.snapshot?.panel_commands;

	function slug(name, taken) {
		const base = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 28) || 'command';
		let id = base;
		for (let n = 2; taken.has(id); n++) id = `${base}-${n}`;
		taken.add(id);
		return id;
	}

	function save() {
		const taken = new Set(list.filter((c) => c.id).map((c) => c.id));
		for (const c of list) if (!c.id) c.id = slug(c.name, taken);
		return settings.save('commands', list);
	}

	function toggle(c, id) {
		c.servers = c.servers.includes(id) ? c.servers.filter((x) => x !== id) : [...c.servers, id];
	}
</script>

<Section title={t('settings.tabs.commands')} help={t('settings.commands.help')} locked={!settings.unlocked} onSave={save}>
	<p class="notice warning">{t('settings.commands.risk')}</p>

	<div class="status">
		<span class="label">{t('settings.commands.where')}</span>
		{#each servers as s}
			<div class="srv">
				<span class="dot {accepts(s.id) ? 'ok' : 'muted'}"></span>
				<strong>{s.name}</strong>
				{#if accepts(s.id)}
					<span class="text-ok">{t('settings.commands.accepts')}</span>
				{:else}
					<span class="muted">{t('settings.commands.refuses')}</span>
				{/if}
			</div>
		{/each}
		{#if servers.some((s) => !accepts(s.id))}
			<p class="help">{t('settings.commands.howTo', { file: AGENT_CONFIG })} <code>"panel_commands": true</code></p>
		{/if}
	</div>

	{#each list as c, i}
		<div class="row">
			<label class="field"><span>{t('settings.manual.name')}</span><input class="input" bind:value={c.name} required maxlength="60" /></label>
			<label class="field"><span>{t('settings.manual.description')}</span><input class="input" bind:value={c.description} /></label>
			<label class="field wide">
				<span>{t('settings.manual.command')}</span>
				<textarea class="input" rows="2" bind:value={c.command} required maxlength="4096" placeholder="docker restart mosquitto" spellcheck="false"></textarea>
				<span class="help">{t('settings.commands.shell')}</span>
			</label>
			<fieldset class="wide targets">
				<legend>{t('settings.commands.servers')}</legend>
				{#each servers as s}
					<label class="check"><input type="checkbox" checked={c.servers.includes(s.id)} onchange={() => toggle(c, s.id)} />{s.name}</label>
				{/each}
				<span class="help">{t('settings.commands.allHelp')}</span>
			</fieldset>
			<button type="button" class="btn danger" onclick={() => list.splice(i, 1)}>{t('settings.remove')}</button>
		</div>
	{:else}
		<p class="muted">{t('settings.commands.none')}</p>
	{/each}
	<div><button type="button" class="btn" onclick={() => list.push({ id: '', name: '', description: '', command: '', servers: [] })}>{t('settings.commands.add')}</button></div>
</Section>

<style>
	.notice { margin: 0; }
	.status { display: flex; flex-direction: column; gap: 8px; padding: 14px 16px; border-radius: 12px; background: var(--surface-2); border: 1px solid var(--border-soft); }
	.srv { display: flex; align-items: center; gap: 10px; font-size: 14px; }
	.dot.muted { background: var(--border-strong); }
	.status code { font: 12px var(--font-mono); color: var(--accent-2); }
	.row { display: grid; grid-template-columns: 1fr 1.4fr; gap: 10px 12px; align-items: end; padding-bottom: 14px; border-bottom: 1px solid var(--border-soft); }
	.wide { grid-column: 1 / -1; }
	.row .btn { justify-self: start; }
	.targets { border: 0; margin: 0; padding: 0; display: flex; flex-wrap: wrap; align-items: center; gap: 8px 18px; }
	.targets legend { padding: 0; margin-bottom: 6px; font-size: 13px; font-weight: 500; color: var(--text-2); }
	.targets .help { flex: 1 1 100%; }
	@media (max-width: 640px) { .row { grid-template-columns: 1fr; } }
</style>
