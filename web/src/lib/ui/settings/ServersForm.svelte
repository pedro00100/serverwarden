<script>
	import { t, errorText } from '#lib/i18n/index.svelte.js';
	import Section from './Section.svelte';
	import ServerEditor from './ServerEditor.svelte';
	import InstallPanel from './InstallPanel.svelte';

	let { settings, editor = $bindable(null) } = $props();
	// editor: null | { mode: 'add' } | { mode: 'edit', id, verify? } | { mode: 'install', id }

	let error = $state('');
	let byId = (id) => settings.data.servers.find((s) => s.id === id);
	let unpinned = (s) => settings.data.unpinned?.includes(s.id);
	const verify = (id) => (editor = { mode: 'edit', id, verify: true });

	async function remove(s) {
		if (!confirm(t('settings.servers.removeConfirm', { name: s.name }))) return;
		error = '';
		try {
			await settings.removeServer(s.id);
		} catch (e) {
			error = errorText(e);
		}
	}
</script>

{#if editor?.mode === 'add'}
	<ServerEditor {settings} onCancel={() => (editor = null)} onSaved={(id) => (editor = { mode: 'install', id })} />
{:else if editor?.mode === 'edit' && byId(editor.id)}
	{#key editor}
		<ServerEditor {settings} original={byId(editor.id)} verify={editor.verify} onCancel={() => (editor = null)} onSaved={() => (editor = null)} />
	{/key}
{:else if editor?.mode === 'install' && byId(editor.id)}
	<InstallPanel {settings} server={byId(editor.id)} unpinned={unpinned(byId(editor.id))} onVerify={() => verify(editor.id)} onClose={() => (editor = null)} />
{/if}

<Section title={t('settings.tabs.servers')} help={t('settings.servers.help')} locked={!settings.unlocked}>
	{#each settings.data.servers as s}
		<div class="server">
			<div class="text">
				<strong>{s.name}</strong>
				<span class="label">{s.role}</span>
				<span class="mono label">{s.ssh.user}@{s.ssh.host}:{s.ssh.port}</span>
				{#if unpinned(s)}<span class="pill warning">{t('settings.servers.unpinned')}</span>{/if}
			</div>
			<div class="buttons">
				{#if unpinned(s)}<button type="button" class="btn primary" onclick={() => verify(s.id)}>{t('settings.servers.verify')}</button>{/if}
				<button type="button" class="btn" onclick={() => (editor = { mode: 'install', id: s.id })}>{t('settings.servers.install')}</button>
				<button type="button" class="btn" onclick={() => (editor = { mode: 'edit', id: s.id })}>{t('settings.servers.edit')}</button>
				<button type="button" class="btn danger" onclick={() => remove(s)}>{t('settings.servers.remove')}</button>
			</div>
		</div>
	{:else}
		<p class="muted">{t('settings.servers.none')}</p>
	{/each}
	<div><button type="button" class="btn primary" onclick={() => (editor = { mode: 'add' })}>{t('settings.servers.add')}</button></div>
	{#if error}<p class="notice critical" role="alert">{error}</p>{/if}
</Section>

<style>
	.server { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 16px; padding: 10px 0; border-bottom: 1px solid var(--border-soft); }
	.text { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; flex: 1 1 220px; min-width: 0; }
	.pill { margin-top: 4px; }
	.buttons { display: flex; gap: 8px; flex-wrap: wrap; }
	.notice { margin: 0; }
</style>
