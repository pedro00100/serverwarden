<script>
	import { onMount } from 'svelte';
	import { createSession } from '#lib/core/session.svelte.js';
	import { createDashboard } from '#lib/core/dashboard.svelte.js';
	import { createCVE } from '#lib/core/cve.svelte.js';
	import { createActions } from '#lib/core/actions.svelte.js';
	import { createRouter } from '#lib/core/router.svelte.js';
	import { createSettings } from '#lib/core/settings.svelte.js';
	import { createRecovery } from '#lib/core/recovery.svelte.js';
	import { createSetup } from '#lib/core/setup.svelte.js';
	import { i18n, t, errorText } from '#lib/i18n/index.svelte.js';
	import Shell from '#lib/ui/Shell.svelte';
	import Login from '#lib/ui/Login.svelte';
	import Overview from '#lib/ui/views/Overview.svelte';
	import Servers from '#lib/ui/views/Servers.svelte';
	import Security from '#lib/ui/views/Security.svelte';
	import Vulnerabilities from '#lib/ui/views/Vulnerabilities.svelte';
	import Actions from '#lib/ui/views/Actions.svelte';
	import Settings from '#lib/ui/views/Settings.svelte';
	import Recovery from '#lib/ui/Recovery.svelte';
	import Setup from '#lib/ui/Setup.svelte';

	const sections = ['overview', 'servers', 'security', 'vulnerabilities', 'actions', 'settings'];

	const session = createSession();
	const dashboard = createDashboard(session);
	const cve = createCVE(session);
	const actions = createActions(session);
	const router = createRouter(sections);
	const settings = createSettings(session);
	const recovery = createRecovery();
	const setup = createSetup();

	$effect(() => {
		if (session.authenticated && !session.recovery && !session.setup) dashboard.refresh();
		else dashboard.clear();
	});
	$effect(() => {
		if (session.language) i18n.set(session.language);
	});

	onMount(() => {
		i18n.set(i18n.locale);
		const stopRouter = router.start();
		const stopDashboard = dashboard.start();
		session.check();
		return () => { stopRouter(); stopDashboard(); };
	});
</script>

<svelte:head><title>{session.title || t('nav.overview')}</title></svelte:head>

{#if !session.checked}
	<div class="center"><span class="dot ok"></span>{t('app.checking')}</div>
{:else if session.setup}
	<Setup {setup} code={router.setupCode} authenticated={session.authenticated} />
{:else if session.recovery}
	<Recovery {recovery} code={router.recoverCode} authenticated={session.authenticated} />
{:else if !session.authenticated || router.enrollCode}
	<Login {session} enrollCode={router.enrollCode} onEnrolled={() => router.clearEnrollment()} onSignedIn={() => {}} />
{:else}
	<Shell title={session.title} mode={session.mode} {sections} current={router.current} onNavigate={(s, sub) => router.go(s, sub)}
		level={dashboard.view?.level} secondsAgo={dashboard.secondsAgo} onRefresh={() => dashboard.refresh()} onLogout={() => session.logout()}>
		{#if dashboard.error}
			<div class="card alert" role="alert">{errorText(dashboard.error)}{dashboard.view ? ' · ' + t('app.staleData') : ''}</div>
		{/if}
		{#if dashboard.update?.url.startsWith('https://github.com/')}
			<p class="card update">{t('app.update', { version: dashboard.update.version })} <a href={dashboard.update.url} target="_blank" rel="noopener noreferrer">{t('app.updateNotes')}</a></p>
		{/if}

		{#if router.current === 'settings'}
			<Settings {settings} mode={session.mode} view={dashboard.view} page={router.sub} onOpen={(p) => router.go('settings', p)} />
		{:else if !dashboard.view}
			<div class="card loading" aria-busy="true"><span class="dot ok"></span>{t('app.connecting')}</div>
		{:else if router.current === 'overview'}
			<Overview view={dashboard.view} description={dashboard.description} />
		{:else if router.current === 'servers'}
			<Servers view={dashboard.view} selected={router.sub} onSelect={(id) => router.go('servers', id)} onNavigate={(s, sub) => router.go(s, sub)} />
		{:else if router.current === 'security'}
			<Security view={dashboard.view} onNavigate={(s) => router.go(s)} />
		{:else if router.current === 'vulnerabilities'}
			<Vulnerabilities {cve} view={dashboard.view} />
		{:else}
			<Actions {actions} {settings} mode={session.mode} view={dashboard.view} selected={router.sub} onSelect={(id) => router.go('actions', id)} onNavigate={(s, sub) => router.go(s, sub)} />
		{/if}
	</Shell>
{/if}

<style>
	.center { min-height: 100vh; display: flex; align-items: center; justify-content: center; gap: 12px; color: var(--text-2); }
	.alert { padding: 12px 16px; border-color: var(--critical-border); color: var(--critical-text); }
	.update { padding: 12px 16px; border-color: var(--accent-bg); color: var(--text-2); }
	.update a { color: var(--accent-2); }
	.loading { padding: 20px; display: flex; align-items: center; gap: 12px; color: var(--text-2); }
</style>
