<script>
	import { onMount, tick } from 'svelte';
	import { t, errorText } from '#lib/i18n/index.svelte.js';
	import ServerNode from '../primitives/ServerNode.svelte';

	let { actions, settings, mode, view, selected = '', onSelect, onNavigate } = $props();

	onMount(() => {
		actions.load();
		if (mode === 'password') return settings.start();
	});

	const ICONS = {
		update: 'M4 4v6h6M20 20v-6h-6M5 15a7 7 0 0 0 12 3M19 9A7 7 0 0 0 7 6',
		backup: 'M12 3v12M7 10l5 5 5-5M5 21h14',
		restart: 'M4 12a8 8 0 1 0 3-6.2M4 4v4h4',
		shield: 'M12 3l8 3v6c0 4.5-3.4 8-8 9-4.6-1-8-4.5-8-9V6z',
		terminal: 'M4 17l6-6-6-6M12 19h8',
		run: 'M7 4l12 8-12 8z'
	};
	const iconOf = (a) => {
		if (a.command) return ICONS.terminal;
		const id = a.id.toLowerCase();
		if (/update|upgrade/.test(id)) return ICONS.update;
		if (/backup|snapshot/.test(id)) return ICONS.backup;
		if (/restart|reboot|start/.test(id)) return ICONS.restart;
		if (/scan|lynis|audit|smart/.test(id)) return ICONS.shield;
		return ICONS.run;
	};

	let current = $derived(view.servers.find((s) => s.server.id === selected) ?? view.servers[0]);
	let list = $derived(actions.catalog.filter((a) => a.server === current?.server.id));
	let locked = $derived(mode === 'password' && !settings.unlocked);
	let hasCommands = $derived(!!settings.data?.commands?.length || actions.catalog.some((a) => a.command));

	let dialog = $state();
	let pending = $state(null);
	let password = $state('');
	let unlocking = $state(false);
	let unlockError = $state('');
	let copied = $state(null);

	const mmss = (s) => `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
	const countFor = (id) => actions.catalog.filter((a) => a.server === id).length;

	function statusText(st) {
		const server = current.server.name;
		switch (st.phase) {
			case 'signing': return t('actions.signing');
			case 'running': return t('actions.verifying', { server });
			case 'done': return t('actions.done', { server });
			default: return st.message ?? errorText(st.error);
		}
	}

	async function run(a) {
		if (!locked) return actions.run(a);
		pending = a;
		password = '';
		unlockError = '';
		dialog.showModal();
		await tick();
		dialog.querySelector('input')?.focus();
	}

	async function unlockAndRun(ev) {
		ev.preventDefault();
		unlocking = true;
		unlockError = '';
		try {
			await settings.unlockWithPassword(password);
			password = '';
			dialog.close();
			const a = pending;
			pending = null;
			await actions.run(a);
		} catch (e) {
			unlockError = errorText(e);
		} finally {
			unlocking = false;
		}
	}

	async function copy(command, i) {
		try {
			await navigator.clipboard.writeText(command);
			copied = i;
			setTimeout(() => (copied = null), 1500);
		} catch {}
	}
</script>

<div class="servers" role="group" aria-label={t('actions.server')}>
	{#each view.servers as s}
		{@const n = countFor(s.server.id)}
		<button class:active={s === current} aria-pressed={s === current} onclick={() => onSelect(s.server.id)}>
			<ServerNode text="" level={s.level} size={30} />
			<span class="srv"><strong>{s.server.name}</strong><span class="label">{t('actions.count', { n })}</span></span>
		</button>
	{/each}
</div>

<div class="layout">
	{#key current?.server.id}
		<section class="rise main" aria-labelledby="t-run">
			<div class="heading">
				<h2 id="t-run">{t('actions.runHere')}</h2>
				{#if mode === 'password' && settings.unlocked}
					<span class="pill unlocked" role="status">
						<svg aria-hidden="true" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><rect x="4" y="11" width="16" height="10" rx="2" /><path d="M8 11V7a4 4 0 0 1 7.5-2" /></svg>
						{t('actions.unlockedFor', { time: mmss(settings.secondsLeft) })}
					</span>
				{:else}
					<span class="label">{mode === 'password' ? t('actions.passwordHint') : t('actions.passkeyHint')}</span>
				{/if}
			</div>

			<div class="grid">
				{#each list as a, i}
					{@const st = actions.statusOf(a)}
					<div class="card action" style:animation-delay="{i * 0.04}s">
						<span class="icon" aria-hidden="true"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d={iconOf(a)} /></svg></span>
						<span class="text">
							<strong>{a.name}{#if a.command}<span class="yours">{t('actions.yours')}</span>{/if}</strong>
							{#if a.description}<span class="label">{a.description}</span>{/if}
						</span>
						<button class="run" class:busy={st?.phase === 'signing' || st?.phase === 'running'} disabled={actions.busy}
							onclick={() => run(a)} aria-label={t('actions.runOn', { action: a.name, server: current.server.name })}>
							{#if st?.phase === 'signing' || st?.phase === 'running'}<span class="spin" aria-hidden="true"></span>{/if}
							{st?.phase === 'signing' ? t('actions.waitingKey') : st?.phase === 'running' ? t('actions.running') : st?.phase === 'done' ? t('actions.runAgain') : t('actions.run')}
						</button>
						{#if a.command || st}
							<div class="more">
								{#if a.command}<code class="cmd">{a.command}</code>{/if}
								{#if st}<span class="state {st.phase}" role="status">{statusText(st)}</span>{/if}
								{#if st?.output}<pre class="output">{st.output}</pre>{/if}
							</div>
						{/if}
					</div>
				{:else}
					<p class="card none muted">{t('actions.noneHere', { server: current?.server.name ?? '' })}</p>
				{/each}
			</div>
			{#if hasCommands && current && !current.snapshot?.panel_commands}
				<p class="hint">{t('actions.commandsOff', { server: current.server.name })} <button class="link" onclick={() => onNavigate('settings', 'commands')}>{t('actions.howToAllow')}</button></p>
			{/if}
		</section>
	{/key}

	<section class="card rise manual" aria-labelledby="t-manual" style:animation-delay="0.08s">
		<div class="mhead">
			<h2 id="t-manual">{t('actions.manualTitle')}</h2>
			<span class="label">{t('actions.manualHint')}</span>
		</div>
		{#each actions.manual as m, i}
			<div class="cmd">
				<span class="name"><span class="pill warning">{t('actions.manual')}</span><strong>{m.name}</strong></span>
				{#if m.description}<span class="label">{m.description}</span>{/if}
				<div class="code">
					<code>{m.command}</code>
					<button class="btn" onclick={() => copy(m.command, i)} aria-label={t('actions.copyLabel', { command: m.command })}>{copied === i ? t('actions.copied') : t('actions.copy')}</button>
				</div>
			</div>
		{:else}
			<p class="muted">{t('settings.manual.none')}</p>
		{/each}
	</section>
</div>

<dialog bind:this={dialog} aria-labelledby="t-confirm" onclose={() => (pending = null)}>
	<form onsubmit={unlockAndRun}>
		<span class="lock" aria-hidden="true"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="4" y="11" width="16" height="10" rx="2" /><path d="M8 11V7a4 4 0 0 1 8 0v4" /></svg></span>
		<h2 id="t-confirm">{t('actions.confirmTitle')}</h2>
		<p>{t('actions.confirmText', { action: pending?.name ?? '', server: current?.server.name ?? '' })}</p>
		<label class="field">
			<span>{t('login.password')}</span>
			<input class="input" type="password" autocomplete="current-password" bind:value={password} required />
		</label>
		{#if unlockError}<p class="notice critical" role="alert">{unlockError}</p>{/if}
		<div class="buttons">
			<button type="button" class="btn" onclick={() => dialog.close()}>{t('settings.cancel')}</button>
			<button type="submit" class="btn primary" disabled={unlocking}>{unlocking ? t('settings.unlocking') : t('actions.unlockAndRun')}</button>
		</div>
	</form>
</dialog>

<style>
	.servers { display: flex; flex-wrap: wrap; gap: 10px; }
	.servers button { min-height: 56px; display: flex; align-items: center; gap: 12px; padding: 0 18px 0 14px; border-radius: 14px; border: 1px solid var(--border); background: var(--surface); cursor: pointer; text-align: left; transition: background 0.2s, border-color 0.2s; }
	.servers button:hover { background: var(--row-hover); }
	.servers button.active { background: var(--accent-bg); border-color: var(--button); }
	.srv { display: flex; flex-direction: column; }
	.srv strong { font-size: 14px; }

	.layout { display: flex; gap: 20px; align-items: flex-start; }
	.main { flex: 1.5 1 0; min-width: 0; display: flex; flex-direction: column; gap: 12px; }
	.heading { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px; }
	h2 { font-size: 18px; font-weight: 600; }
	.pill.unlocked { background: var(--accent-bg); color: var(--accent-2); }
	/* Columns instead of a grid: a tall card does not leave a hole beside a short one. */
	.grid { columns: 300px; column-gap: 12px; }
	.grid > :global(*) { break-inside: avoid; margin-bottom: 12px; }
	.action { padding: 18px; display: grid; grid-template-columns: 44px minmax(0, 1fr) auto; gap: 8px 14px; align-items: start; animation: rise 0.45s cubic-bezier(0.2, 0.7, 0.2, 1) both; }
	.icon { width: 44px; height: 44px; flex: 0 0 auto; display: flex; align-items: center; justify-content: center; border-radius: 12px; background: var(--accent-bg); color: var(--accent-2); }
	.text { flex: 1 1 auto; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
	.text strong { font-size: 15px; }
	.more { grid-column: 2 / -1; display: flex; flex-direction: column; gap: 6px; min-width: 0; }
	.state { font-size: 13px; font-weight: 500; color: var(--text-2); }
	.state.done { color: var(--ok-text); }
	.state.failed { color: var(--critical-text); }
	.state.signing, .state.running { animation: blink 1.4s ease-in-out infinite; }
	@keyframes blink { 50% { opacity: 0.5; } }
	.run { flex: 0 0 auto; min-height: 40px; display: inline-flex; align-items: center; gap: 8px; padding: 0 16px; border-radius: 10px; border: 1px solid var(--button); background: transparent; color: var(--accent-2); cursor: pointer; font-weight: 600; transition: background 0.2s; }
	.run:hover:not(:disabled) { background: var(--accent-bg); }
	.run:disabled { opacity: 0.55; cursor: progress; }
	.run.busy { background: var(--accent-bg); opacity: 1; }
	.spin { width: 13px; height: 13px; border-radius: 50%; border: 2px solid currentColor; border-right-color: transparent; animation: spin 0.9s linear infinite; }
	@keyframes spin { to { rotate: 360deg; } }
	.none { padding: 18px; margin: 0; column-span: all; }
	.text strong { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; }
	.yours { padding: 2px 8px; white-space: nowrap; border-radius: 999px; background: var(--accent-bg); color: var(--accent-2); font-size: 11px; font-weight: 600; }
	.cmd { padding: 6px 10px; border-radius: 8px; background: var(--code-bg); border: 1px solid var(--border-soft); font: 12px var(--font-mono); color: var(--text-2); white-space: pre-wrap; overflow-wrap: anywhere; }
	.output { margin: 0; max-height: 220px; overflow: auto; padding: 10px 12px; border-radius: 8px; background: var(--code-bg); border: 1px solid var(--border-soft); font: 12px/1.6 var(--font-mono); color: var(--text-3); white-space: pre-wrap; overflow-wrap: anywhere; }
	.hint { margin: 4px 0 0; font-size: 13px; color: var(--muted); }
	.link { padding: 0; border: 0; background: none; color: var(--accent-2); cursor: pointer; font-size: 13px; }
	.link:hover { color: var(--accent); }

	.manual { flex: 1 1 0; min-width: 300px; padding: 20px 22px; background: var(--surface-sunken); border-color: var(--border-soft); display: flex; flex-direction: column; gap: 14px; }
	.mhead { display: flex; flex-direction: column; gap: 3px; }
	.mhead h2 { font-size: 17px; }
	.cmd { display: flex; flex-direction: column; gap: 8px; padding-top: 14px; border-top: 1px solid var(--border-soft); }
	.name { display: flex; align-items: center; gap: 8px; }
	.name strong { font-size: 14px; }
	.code { display: flex; align-items: center; gap: 8px; padding: 6px 6px 6px 12px; border-radius: 10px; background: var(--code-bg); border: 1px solid var(--border-soft); }
	code { flex: 1 1 auto; min-width: 0; overflow-x: auto; white-space: nowrap; font: 400 12px var(--font-mono); color: var(--text-2); }
	.code .btn { min-height: 34px; padding: 4px 12px; font-size: 13px; }

	dialog { width: min(440px, calc(100vw - 32px)); padding: 0; border-radius: 18px; border: 1px solid var(--border-strong); background: var(--surface); color: var(--text); box-shadow: 0 30px 80px rgba(0, 0, 0, 0.5); }
	dialog[open] { animation: pop 0.25s cubic-bezier(0.2, 0.7, 0.2, 1) both; }
	dialog::backdrop { background: rgba(5, 8, 12, 0.62); }
	@keyframes pop { from { opacity: 0; translate: 0 -8px; scale: 0.98; } }
	dialog form { padding: 24px; display: flex; flex-direction: column; gap: 14px; }
	dialog h2 { font-size: 19px; }
	dialog p { margin: 0; color: var(--text-2); line-height: 1.5; }
	.lock { width: 44px; height: 44px; display: flex; align-items: center; justify-content: center; border-radius: 12px; background: var(--warning-fill); color: var(--warning); }
	.buttons { display: flex; justify-content: flex-end; gap: 10px; }

	@media (max-width: 1100px) {
		.layout { flex-direction: column; align-items: stretch; }
		.manual { min-width: 0; }
	}
	@media (max-width: 640px) {
		.servers { flex-wrap: nowrap; overflow-x: auto; }
		.servers button { flex: 0 0 auto; }
	}
</style>
