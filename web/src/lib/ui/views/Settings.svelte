<script>
	import { onMount } from 'svelte';
	import { t, errorText, languages } from '#lib/i18n/index.svelte.js';
	import UnlockBar from '../settings/UnlockBar.svelte';
	import GeneralForm from '../settings/GeneralForm.svelte';
	import AccessForm from '../settings/AccessForm.svelte';
	import WebForm from '../settings/WebForm.svelte';
	import NotificationsForm from '../settings/NotificationsForm.svelte';
	import ManualForm from '../settings/ManualForm.svelte';
	import ServersForm from '../settings/ServersForm.svelte';
	import ScannerForm from '../settings/ScannerForm.svelte';
	import CommandsForm from '../settings/CommandsForm.svelte';

	let { settings, mode, view = null, page = '', onOpen } = $props();

	const PAGES = ['general', 'access', 'web', 'notifications', 'servers', 'commands', 'scanner', 'manual'];
	const ICONS = {
		general: 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM19 12h2M3 12h2M12 3v2M12 19v2M17 7l1.4-1.4M5.6 18.4 7 17M17 17l1.4 1.4M5.6 5.6 7 7',
		access: 'M15 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0zM4 21a8 8 0 0 1 16 0',
		web: 'M3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18',
		notifications: 'M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9M10 21h4',
		servers: 'M4 5h16v6H4zM4 13h16v6H4zM8 8h.01M8 16h.01',
		scanner: 'M8 8h8v9a4 4 0 0 1-8 0zM9 4l2 3M15 4l-2 3M4 12h4M16 12h4',
		commands: 'M13 2 3 14h9l-1 8 10-12h-9z',
		manual: 'M4 17l6-6-6-6M12 19h8'
	};

	let serverEditor = $state(null); // kept here so it survives reloads after saving
	let restarting = $state(false);
	let current = $derived(PAGES.includes(page) ? page : '');

	onMount(() => settings.start());

	async function restart() {
		restarting = true;
		try {
			await settings.restart();
			setTimeout(() => location.reload(), 4000);
		} catch {
			restarting = false;
		}
	}

	function summary(id, d) {
		switch (id) {
			case 'general': return [
				[t('settings.summary.name'), d.general.title],
				[t('settings.general.language'), languages.find((l) => l.id === d.general.language)?.name ?? d.general.language],
				[t('settings.summary.refresh'), t('settings.summary.every', { n: d.general.interval_sec })]
			];
			case 'access': return [
				[t('settings.summary.signIn'), d.access.mode === 'password' ? t('settings.summary.password') : t('settings.summary.passkey')],
				d.access.mode === 'password'
					? [t('settings.totp.title'), d.access.totp ? t('settings.summary.on') : t('settings.summary.off')]
					: [t('settings.access.keys'), String(d.access.passkeys?.length ?? 0)],
				[t('settings.access.username'), d.access.username || '—']
			];
			case 'web': return [
				[t('settings.summary.address'), d.web.origin],
				[t('settings.summary.allowed'), (d.web.allowed ?? []).join(', ')],
				[t('settings.summary.listen'), d.web.listen]
			];
			case 'notifications': return [
				[t('settings.notifications.channels'), (d.notifications.channels ?? []).map((c) => c.name).join(', ') || t('settings.summary.none')],
				[t('settings.summary.fireAfter'), `${d.notifications.alerts?.pending_sec ?? 0} s`],
				[t('settings.summary.repeat'), d.notifications.alerts?.repeat_min ? `${d.notifications.alerts.repeat_min} min` : t('settings.summary.never')]
			];
			case 'servers': return [
				[t('settings.summary.connected'), String(d.servers?.length ?? 0)],
				[t('settings.summary.names'), (d.servers ?? []).map((s) => s.name).join(', ') || t('settings.summary.none')]
			];
			case 'commands': return [
				[t('settings.summary.commands'), String(d.commands?.length ?? 0)],
				[t('settings.summary.accepting'), String(view?.servers.filter((s) => s.snapshot?.panel_commands).length ?? '—')]
			];
			case 'scanner': return [
				[t('settings.summary.scanner'), d.scanner.enabled ? t('settings.summary.on') : t('settings.summary.off')],
				[t('settings.scanner.time'), d.scanner.time],
				[t('settings.scanner.timezone'), d.scanner.timezone]
			];
			default: return [[t('settings.summary.saved'), String(d.manual_actions?.length ?? 0)]];
		}
	}

	function suggestions(d) {
		const list = [];
		if (d.access.mode === 'password' && !d.access.totp) list.push({ page: 'access', text: t('settings.suggest.totp') });
		else if (d.access.mode === 'password') list.push({ page: 'access', text: t('settings.suggest.passkey') });
		if (!d.notifications.channels?.length) list.push({ page: 'notifications', text: t('settings.suggest.channel') });
		if (!d.scanner.enabled) list.push({ page: 'scanner', text: t('settings.suggest.scanner') });
		if (!d.servers?.length) list.push({ page: 'servers', text: t('settings.suggest.server') });
		return list;
	}

	let tips = $derived(settings.data ? suggestions(settings.data) : []);
	const flagged = (id) => tips.some((x) => x.page === id);
</script>

{#if !settings.data}
	{#if settings.error}<p class="notice critical" role="alert">{errorText(settings.error)}</p>{:else}<div class="card loading"><span class="dot ok"></span>…</div>{/if}
{:else}
	{#if settings.data.restart_required?.length}
		<div class="notice warning restart" role="status">
			<span>{t('settings.restartNeeded', { fields: settings.data.restart_required.join(', ') })}</span>
			<button class="btn" disabled={!settings.unlocked || restarting} onclick={restart}>{restarting ? t('settings.restarting') : t('settings.restart')}</button>
		</div>
	{/if}

	{#if !current}
		{#if tips.length}
			<section class="card rise tips" aria-labelledby="t-tips">
				<h2 id="t-tips">{t('settings.suggest.title', { n: tips.length })}</h2>
				<div class="tip-list">
					{#each tips as tip}<button class="btn" onclick={() => onOpen(tip.page)}>{tip.text}</button>{/each}
				</div>
			</section>
		{/if}

		<div class="cards">
			{#each PAGES as id, i}
				<button class="card rise tile" style:animation-delay="{i * 0.04}s" onclick={() => onOpen(id)}>
					<span class="tile-head">
						<span class="icon" aria-hidden="true"><svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d={ICONS[id]} /></svg></span>
						<strong>{t('settings.tabs.' + id)}</strong>
						{#if flagged(id)}<span class="pill warning">{t('settings.suggest.flag')}</span>{/if}
					</span>
					<dl>
						{#each summary(id, settings.data) as [k, v]}
							<div><dt>{k}</dt><dd>{v}</dd></div>
						{/each}
					</dl>
				</button>
			{/each}
		</div>
	{:else}
		<div class="focusbar">
			<button class="btn back" onclick={() => onOpen('')}>
				<svg aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 6l-6 6 6 6" /></svg>
				{t('settings.all')}
			</button>
			<nav class="jump" aria-label={t('nav.settings')}>
				{#each PAGES as id}
					<button class:active={id === current} aria-current={id === current ? 'page' : undefined} onclick={() => onOpen(id)}>{t('settings.tabs.' + id)}</button>
				{/each}
			</nav>
		</div>

		{#key current}
			<div class="focus">
				<div class="page-head">
					<span class="icon big" aria-hidden="true"><svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d={ICONS[current]} /></svg></span>
					<div><h2>{t('settings.tabs.' + current)}</h2><span class="label">{t('settings.about.' + current)}</span></div>
				</div>
				<UnlockBar {settings} {mode} />
				{#if settings.justSaved}<p class="notice ok" role="status">{t('settings.saved')}</p>{/if}
				{#key settings.data}
					<div class="stack">
						{#if current === 'general'}<GeneralForm {settings} />
						{:else if current === 'access'}<AccessForm {settings} />
						{:else if current === 'web'}<WebForm {settings} />
						{:else if current === 'notifications'}<NotificationsForm {settings} />
						{:else if current === 'manual'}<ManualForm {settings} />
						{:else if current === 'scanner'}<ScannerForm {settings} />
						{:else if current === 'commands'}<CommandsForm {settings} {view} />
						{:else}<ServersForm {settings} bind:editor={serverEditor} />{/if}
					</div>
				{/key}
			</div>
		{/key}
	{/if}
{/if}

<style>
	.loading { padding: 20px; display: flex; gap: 12px; align-items: center; }
	.restart { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px; }
	.notice { margin: 0; }

	.tips { padding: 16px 20px; display: flex; flex-wrap: wrap; align-items: center; gap: 12px 16px; border-color: var(--warning-border); background: var(--warning-fill); }
	.tips h2 { font-size: 16px; color: var(--warning-text); white-space: nowrap; }
	.tip-list { display: flex; flex-wrap: wrap; gap: 10px; }
	.tip-list .btn { border-color: var(--warning-border); background: var(--bg); color: var(--text); }

	.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 16px; }
	.tile { padding: 20px; display: flex; flex-direction: column; gap: 16px; text-align: left; cursor: pointer; }
	.tile:hover { border-color: var(--button); }
	.tile-head { display: flex; align-items: center; gap: 12px; width: 100%; }
	.tile-head strong { flex: 1 1 auto; font: 600 17px var(--font-display); }
	.icon { width: 42px; height: 42px; flex: 0 0 auto; display: flex; align-items: center; justify-content: center; border-radius: 12px; background: var(--accent-bg); color: var(--accent-2); }
	.icon.big { width: 52px; height: 52px; border-radius: 14px; }
	dl { margin: 0; width: 100%; display: flex; flex-direction: column; gap: 6px; }
	dl div { display: flex; justify-content: space-between; gap: 12px; font-size: 13px; }
	dt { color: var(--muted); flex: 0 0 auto; }
	dd { margin: 0; font-weight: 500; text-align: right; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

	.focusbar { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; }
	.back { background: var(--surface); }
	.jump { display: flex; gap: 4px; padding: 4px; border-radius: 12px; background: var(--surface-2); border: 1px solid var(--border-soft); overflow-x: auto; scrollbar-width: none; max-width: 100%; }
	.jump button { flex: 0 0 auto; min-height: 34px; padding: 0 12px; border: 0; border-radius: 9px; background: transparent; color: var(--text-3); cursor: pointer; font-size: 13px; white-space: nowrap; }
	.jump button:hover { color: var(--text); }
	.jump button.active { background: var(--accent-bg); color: var(--text); font-weight: 600; box-shadow: inset 0 0 0 1px var(--button); }

	.focus { width: 100%; max-width: 1000px; align-self: center; display: flex; flex-direction: column; gap: 16px; animation: focus 0.4s cubic-bezier(0.2, 0.7, 0.2, 1) both; }
	@keyframes focus { from { opacity: 0; translate: 0 16px; scale: 0.985; } }
	.page-head { display: flex; align-items: center; gap: 16px; }
	.page-head div { display: flex; flex-direction: column; gap: 3px; }
	.page-head h2 { font-size: 24px; font-weight: 600; }
	.stack { display: flex; flex-direction: column; gap: 18px; }
</style>
