<script>
	import { tick } from 'svelte';
	import { t } from '#lib/i18n/index.svelte.js';
	import { ago, dateTime } from '#lib/i18n/format.js';
	import Logo from './primitives/Logo.svelte';
	import { PRODUCT, MAKER } from '#lib/core/brand.js';

	let { title, mode, sections, current, onNavigate, level, secondsAgo, onRefresh, onLogout, children } = $props();

	let now = $state(Date.now());
	$effect(() => {
		const timer = setInterval(() => (now = Date.now()), 30_000);
		return () => clearInterval(timer);
	});

	let nav = $state();
	let tabs = $state({});
	let indicator = $state('');
	let menuOpen = $state(false);
	let menu = $state();

	async function place() {
		await tick();
		const el = tabs[current];
		if (!el) return;
		indicator = `transform: translateX(${el.offsetLeft}px); width: ${el.offsetWidth}px`;
		if (nav.scrollWidth > nav.clientWidth) el.scrollIntoView({ block: 'nearest', inline: 'center', behavior: 'smooth' });
	}

	$effect(() => {
		current;
		title;
		place();
	});
	$effect(() => {
		const onResize = () => place();
		addEventListener('resize', onResize);
		document.fonts?.ready.then(place);
		return () => removeEventListener('resize', onResize);
	});

	$effect(() => {
		if (!menuOpen) return;
		const close = (ev) => {
			if (ev.type === 'keydown' ? ev.key === 'Escape' : !menu?.contains(ev.target)) menuOpen = false;
		};
		addEventListener('pointerdown', close);
		addEventListener('keydown', close);
		return () => {
			removeEventListener('pointerdown', close);
			removeEventListener('keydown', close);
		};
	});
</script>

<div class="app">
	<header class="top">
		<div class="bar">
			<div class="brand">
				<Logo {title} size={40} />
				<div><strong>{title}</strong><span class="label">{t('shell.lanOnly')}</span></div>
			</div>

			<nav class="tabs" aria-label={t('nav.label')} bind:this={nav}>
				<span class="indicator" style={indicator} aria-hidden="true"></span>
				{#each sections as s}
					<button bind:this={tabs[s]} class:active={s === current} aria-current={s === current ? 'page' : undefined} onclick={() => onNavigate(s)}>
						{t('nav.' + s)}
					</button>
				{/each}
			</nav>

			<div class="status">
				{#if level}
					<span class="pill {level}"><span class="dot {level}"></span>{t('level.' + level)}</span>
				{/if}
				<span class="label updated" aria-live="polite">{t('shell.updated', { ago: ago(secondsAgo) })}</span>
				<button class="icon refresh" aria-label={t('shell.refresh')} onclick={onRefresh}>
					<svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 11a8 8 0 1 0-2.3 5.7" /><path d="M20 4v7h-7" /></svg>
				</button>
				<div class="account" bind:this={menu}>
					<button class="icon accent" aria-label={t('shell.account')} aria-expanded={menuOpen} aria-haspopup="true" onclick={() => (menuOpen = !menuOpen)}>
						<svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3l8 3v6c0 4.5-3.4 8-8 9-4.6-1-8-4.5-8-9V6z" /></svg>
					</button>
					{#if menuOpen}
						<div class="menu card">
							<strong class="secure">
								<svg aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--accent)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3l8 3v6c0 4.5-3.4 8-8 9-4.6-1-8-4.5-8-9V6z" /></svg>
								{t('shell.lanOnly')}
							</strong>
							<span class="label">{mode === 'password' ? t('shell.passwordSession') : t('shell.passkeySession')}</span>
							{#if mode === 'password'}
								<button class="link" onclick={() => { menuOpen = false; onNavigate('settings', 'access'); }}>{t('shell.addPasskey')}</button>
							{/if}
							<button class="btn" onclick={onLogout}>{t('shell.logout')}</button>
							<span class="signature mono">{PRODUCT} · by {MAKER}</span>
						</div>
					{/if}
				</div>
			</div>
		</div>
	</header>

	<main>
		<h1 class="sr-only">{t('nav.' + current)}</h1>
		<p class="label date">{dateTime(now, 'today')}</p>
		{@render children()}
	</main>
</div>

<style>
	.app { min-height: 100vh; }
	.top { position: sticky; top: 0; z-index: 20; background: var(--nav-glass); backdrop-filter: blur(10px); border-bottom: 1px solid var(--divider); }
	.bar { max-width: 1440px; margin: 0 auto; min-height: 68px; padding: 0 28px; display: flex; align-items: center; gap: 24px; }
	.brand { display: flex; align-items: center; gap: 12px; min-width: 0; }
	.brand div { display: flex; flex-direction: column; min-width: 0; line-height: 1.3; }
	.brand strong { font: 600 15px var(--font-display); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.brand .label { white-space: nowrap; }

	.tabs { position: relative; display: flex; gap: 2px; padding: 4px; border-radius: 12px; background: var(--surface-2); border: 1px solid var(--border-soft); overflow-x: auto; scrollbar-width: none; }
	.tabs::-webkit-scrollbar { display: none; }
	.tabs button { position: relative; z-index: 1; flex: 0 0 auto; min-height: 36px; padding: 0 15px; border: 0; border-radius: 9px; background: transparent; color: var(--text-3); cursor: pointer; font-weight: 500; white-space: nowrap; transition: color 0.25s; }
	.tabs button:hover { color: var(--text); }
	.tabs button.active { color: var(--on-accent); font-weight: 600; }
	.indicator { position: absolute; top: 4px; left: 0; height: calc(100% - 8px); border-radius: 9px; background: var(--accent); transition: transform 0.35s cubic-bezier(0.2, 0.7, 0.2, 1), width 0.35s cubic-bezier(0.2, 0.7, 0.2, 1); }

	.status { margin-left: auto; display: flex; align-items: center; gap: 10px; }
	.updated { white-space: nowrap; }
	.icon { width: 44px; height: 44px; border-radius: 12px; border: 1px solid var(--border); background: var(--surface); color: var(--text-2); display: flex; align-items: center; justify-content: center; cursor: pointer; }
	.icon:hover { border-color: var(--border-strong); }
	.icon.accent { color: var(--accent); }
	.account { position: relative; }
	.menu { position: absolute; right: 0; top: calc(100% + 8px); width: 260px; padding: 16px; display: flex; flex-direction: column; gap: 6px; border-color: var(--border-strong); box-shadow: 0 24px 60px rgba(0, 0, 0, 0.45); animation: rise 0.25s cubic-bezier(0.2, 0.7, 0.2, 1) both; }
	.secure { display: flex; align-items: center; gap: 8px; font-size: 14px; }
	.link { align-self: flex-start; padding: 6px 0; border: 0; background: none; color: var(--accent-2); cursor: pointer; font-size: 13px; }
	.link:hover { color: var(--accent); }
	.menu .btn { margin-top: 8px; }
	.signature { margin-top: 6px; text-align: center; font-size: 11px; color: var(--muted); letter-spacing: 0.04em; }

	main { max-width: 1440px; margin: 0 auto; padding: 18px 28px 64px; display: flex; flex-direction: column; gap: 20px; }
	.date { margin: 0; }
	.date::first-letter { text-transform: uppercase; }

	@media (max-width: 1100px) {
		.updated { display: none; }
	}
	@media (max-width: 860px) {
		.bar { flex-wrap: wrap; gap: 10px 12px; padding: 10px 16px 10px; }
		.brand .label { display: none; }
		.tabs { order: 3; flex: 1 1 100%; }
		.refresh { display: none; }
		main { padding: 16px 16px 48px; }
	}
</style>
