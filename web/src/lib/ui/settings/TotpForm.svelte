<script>
	import { onDestroy } from 'svelte';
	import { t, errorText } from '#lib/i18n/index.svelte.js';
	import CodeInput from '../primitives/CodeInput.svelte';

	let { settings } = $props();
	let access = $derived(settings.data.access);
	let locked = $derived(!settings.unlocked);

	// idle → scan → confirm → codes while enabling; manage to disable or renew the codes.
	let phase = $state('idle');
	let action = $state(''); // disable | renew
	let setup = $state(null); // the secret lives here only while enabling
	let skew = $state(0); // seconds between this device's clock and the panel's
	let panelTime = $state('');
	let code = $state('');
	let password = $state('');
	let codes = $state([]);
	let stored = $state(false);
	let copied = $state('');
	let busy = $state(false);
	let error = $state('');

	const STEPS = ['scan', 'confirm', 'codes'];
	let timer;
	onDestroy(() => clearInterval(timer));

	function reset() {
		clearInterval(timer);
		phase = 'idle';
		setup = null;
		codes = [];
		code = password = error = '';
		stored = false;
	}

	async function run(fn) {
		busy = true;
		error = '';
		try {
			await fn();
		} catch (e) {
			error = errorText(e);
			code = '';
		} finally {
			busy = false;
		}
	}

	const begin = () => run(async () => {
		const r = await settings.totpBegin();
		setup = { secret: r.secret, qr: r.qr };
		skew = Math.round((Date.now() - Date.parse(r.now)) / 1000);
		const tick = () => (panelTime = new Date(Date.now() - skew * 1000).toLocaleTimeString());
		tick();
		timer = setInterval(tick, 1000);
		phase = 'scan';
	});

	const enable = (ev) => {
		ev?.preventDefault();
		if (busy || code.length < 6 || !password) return;
		run(async () => {
			codes = await settings.totpEnable(code, password);
			clearInterval(timer);
			setup = null;
			code = password = '';
			stored = false;
			phase = 'codes';
		});
	};

	const confirmManage = (ev) => {
		ev.preventDefault();
		run(async () => {
			if (action === 'disable') {
				await settings.totpDisable(password, code);
				reset();
			} else {
				codes = await settings.totpRenew(password, code);
				code = password = '';
				stored = false;
				phase = 'codes';
			}
		});
	};

	function manage(a) {
		action = a;
		code = password = error = '';
		phase = 'manage';
	}

	async function copy(text, what) {
		try {
			await navigator.clipboard.writeText(text);
			copied = what;
			setTimeout(() => (copied = ''), 1500);
		} catch {}
	}

	// The file is made in the browser: the codes are not sent anywhere again.
	function download() {
		const text = `${t('settings.totp.fileTitle', { title: settings.data.general.title, user: access.username })}\n${new Date().toLocaleString()}\n\n${codes.join('\n')}\n\n${t('settings.totp.fileHelp')}\n`;
		const url = URL.createObjectURL(new Blob([text], { type: 'text/plain' }));
		const a = Object.assign(document.createElement('a'), { href: url, download: `recovery-codes-${access.username}.txt` });
		a.click();
		URL.revokeObjectURL(url);
	}

	const grouped = (s) => s.match(/.{1,4}/g).join(' ');
</script>

<section class="card rise section" aria-labelledby="totp-title">
	<div class="head">
		<div class="title">
			<h2 id="totp-title">{t('settings.totp.title')}</h2>
			<span class="pill {access.totp ? 'ok' : ''}">{access.totp ? t('settings.totp.on') : t('settings.totp.off')}</span>
		</div>
		<p class="help">{t('settings.totp.help')}</p>
	</div>

	{#if access.mode !== 'password'}
		<p class="notice">{t('settings.totp.passkeyMode')}</p>
	{:else if phase === 'idle'}
		{#if access.totp}
			<div class="status">
				<svg aria-hidden="true" width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="var(--ok)" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3 5 6v5c0 4.5 3 8.3 7 10 4-1.7 7-5.5 7-10V6z" /><path d="m9 12 2 2 4-4" /></svg>
				<div class="text">
					<strong>{t('settings.totp.active')}</strong>
					<span class={access.recovery_left <= 3 ? 'text-warning' : 'muted'}>{access.recovery_left <= 3 ? t('settings.totp.fewLeft', { n: access.recovery_left }) : t('settings.totp.left', { n: access.recovery_left })}</span>
				</div>
			</div>
			<div class="buttons">
				<button type="button" class="btn" disabled={locked} onclick={() => manage('renew')}>{t('settings.totp.renew')}</button>
				<button type="button" class="btn danger" disabled={locked} onclick={() => manage('disable')}>{t('settings.totp.disable')}</button>
			</div>
		{:else}
			<ul class="benefits">
				<li>{t('settings.totp.benefit1')}</li>
				<li>{t('settings.totp.benefit2')}</li>
				<li>{t('settings.totp.benefit3')}</li>
			</ul>
			<div class="buttons"><button type="button" class="btn primary" disabled={locked || busy} onclick={begin}>{t('settings.totp.enable')}</button></div>
		{/if}
		{#if locked}<p class="help">{t('settings.totp.unlockFirst')}</p>{/if}
	{:else if phase === 'manage'}
		<form class="manage" onsubmit={confirmManage}>
			<h3>{action === 'disable' ? t('settings.totp.disableTitle') : t('settings.totp.renewTitle')}</h3>
			<p class="help">{action === 'disable' ? t('settings.totp.disableHelp') : t('settings.totp.renewHelp')}</p>
			<div class="grid">
				<label class="field"><span>{t('settings.totp.password')}</span><input class="input" type="password" autocomplete="current-password" bind:value={password} required /></label>
				<label class="field"><span>{t('settings.totp.codeOrRecovery')}</span><input class="input mono" inputmode="numeric" autocomplete="one-time-code" spellcheck="false" maxlength="14" bind:value={code} required /></label>
			</div>
			<div class="buttons">
				<button type="button" class="btn" onclick={reset}>{t('settings.cancel')}</button>
				<button class="btn {action === 'disable' ? 'danger' : 'primary'}" disabled={busy}>{action === 'disable' ? t('settings.totp.disable') : t('settings.totp.renew')}</button>
			</div>
		</form>
	{:else}
		<ol class="steps" aria-label={t('settings.totp.progress')}>
			{#each STEPS as s, i}
				<li class:done={STEPS.indexOf(phase) > i} class:current={phase === s} aria-current={phase === s ? 'step' : undefined}><span class="n">{i + 1}</span>{t('settings.totp.step.' + s)}</li>
			{/each}
		</ol>

		{#if phase === 'scan'}
			<div class="scan">
				<!-- Drawn by the panel itself (no external QR service); it holds the secret. -->
				<div class="qr" role="img" aria-label={t('settings.totp.qrLabel')}>{@html setup.qr}</div>
				<div class="how">
					<p>{t('settings.totp.scanHelp')}</p>
					<div class="manual">
						<span class="label">{t('settings.totp.manual')}</span>
						<div class="secret"><code>{grouped(setup.secret)}</code><button type="button" class="btn small" onclick={() => copy(setup.secret, 'secret')}>{copied === 'secret' ? t('settings.totp.copied') : t('settings.totp.copy')}</button></div>
						<span class="label">{t('settings.totp.manualType')}</span>
					</div>
					<p class="clock"><span class="label">{t('settings.totp.panelTime')}</span> <span class="mono">{panelTime}</span></p>
					{#if Math.abs(skew) > 20}<p class="notice warning">{t('settings.totp.skew', { n: Math.abs(skew) })}</p>{/if}
				</div>
			</div>
			<div class="buttons">
				<button type="button" class="btn" onclick={reset}>{t('settings.cancel')}</button>
				<button type="button" class="btn primary" onclick={() => (phase = 'confirm')}>{t('settings.totp.next')}</button>
			</div>
		{:else if phase === 'confirm'}
			<form class="confirm" onsubmit={enable}>
				<p>{t('settings.totp.confirmHelp')}</p>
				<CodeInput bind:value={code} id="totp-confirm" label={t('settings.totp.code')} autofocus disabled={busy} onComplete={() => password && enable()} />
				<label class="field"><span>{t('settings.totp.password')}</span><input class="input" type="password" autocomplete="current-password" bind:value={password} required /><span class="help">{t('settings.totp.passwordHelp')}</span></label>
				<div class="buttons">
					<button type="button" class="btn" onclick={() => (phase = 'scan')}>{t('settings.totp.back')}</button>
					<button class="btn primary" disabled={busy || code.length < 6 || !password}>{t('settings.totp.activate')}</button>
				</div>
			</form>
		{:else if phase === 'codes'}
			<p class="notice warning">{t('settings.totp.codesWarning')}</p>
			<ol class="codes">
				{#each codes as c}<li><code>{c}</code></li>{/each}
			</ol>
			<div class="buttons left">
				<button type="button" class="btn" onclick={() => copy(codes.join('\n'), 'codes')}>{copied === 'codes' ? t('settings.totp.copied') : t('settings.totp.copyAll')}</button>
				<button type="button" class="btn" onclick={download}>{t('settings.totp.download')}</button>
			</div>
			<label class="check"><input type="checkbox" bind:checked={stored} />{t('settings.totp.stored')}</label>
			<div class="buttons"><button type="button" class="btn primary" disabled={!stored} onclick={() => { reset(); settings.load(); }}>{t('settings.totp.finish')}</button></div>
		{/if}
	{/if}
	{#if error}<p class="notice critical" role="alert">{error}</p>{/if}
</section>

<style>
	.section { padding: 20px 22px; display: flex; flex-direction: column; gap: 16px; }
	.head { display: flex; flex-direction: column; gap: 4px; }
	.title { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
	h2 { font-size: 17px; }
	h3 { font-size: 15px; margin: 0; }
	p { margin: 0; }
	.buttons { display: flex; gap: 10px; justify-content: flex-end; flex-wrap: wrap; }
	.buttons.left { justify-content: flex-start; }
	.status { display: flex; align-items: center; gap: 14px; padding: 14px 16px; border-radius: 14px; background: var(--surface-2); border: 1px solid var(--border-soft); }
	.status .text { display: flex; flex-direction: column; gap: 2px; }
	.benefits { margin: 0; padding-left: 18px; display: flex; flex-direction: column; gap: 6px; color: var(--text-2); font-size: 14px; }
	.steps { list-style: none; margin: 0; padding: 0; display: flex; gap: 8px; flex-wrap: wrap; }
	.steps li { display: inline-flex; align-items: center; gap: 8px; padding: 6px 12px 6px 6px; border-radius: 999px; border: 1px solid var(--border); color: var(--text-3); font-size: 13px; }
	.steps .n { width: 22px; height: 22px; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center; background: var(--track); font-size: 12px; font-weight: 600; }
	.steps li.current { border-color: var(--button); color: var(--text); background: var(--accent-bg); }
	.steps li.current .n { background: var(--accent); color: var(--on-accent); }
	.steps li.done .n { background: var(--ok-bg); color: var(--ok-text); }
	.scan { display: flex; gap: 22px; align-items: flex-start; flex-wrap: wrap; }
	.qr { flex: 0 0 auto; width: 208px; padding: 8px; border-radius: 16px; background: #fff; box-shadow: 0 10px 30px rgba(0, 0, 0, 0.35); }
	.qr :global(svg) { display: block; width: 100%; height: auto; }
	.how { flex: 1 1 260px; display: flex; flex-direction: column; gap: 14px; min-width: 0; }
	.manual { display: flex; flex-direction: column; gap: 6px; }
	.secret { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
	.secret code { padding: 8px 12px; border-radius: 10px; background: var(--code-bg); border: 1px solid var(--border-soft); font: 14px var(--font-mono); letter-spacing: 0.06em; overflow-wrap: anywhere; }
	.btn.small { min-height: 34px; padding: 0 12px; font-size: 13px; }
	.clock { display: flex; gap: 8px; align-items: baseline; }
	.mono { font-family: var(--font-mono); }
	.confirm, .manage { display: flex; flex-direction: column; gap: 14px; }
	.grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
	.codes { margin: 0; padding: 0; list-style: none; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; counter-reset: c; }
	.codes li { counter-increment: c; display: flex; align-items: center; gap: 10px; padding: 9px 12px; border-radius: 10px; background: var(--code-bg); border: 1px solid var(--border-soft); }
	.codes li::before { content: counter(c); width: 18px; color: var(--text-3); font-size: 12px; text-align: right; }
	.codes code { font: 15px var(--font-mono); letter-spacing: 0.05em; }
	.notice { margin: 0; }
	@media (max-width: 640px) {
		.scan { flex-direction: column; align-items: center; }
		.grid, .codes { grid-template-columns: 1fr; }
	}
</style>
