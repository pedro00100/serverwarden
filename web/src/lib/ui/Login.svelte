<script>
	import MeshBackdrop from './primitives/MeshBackdrop.svelte';
	import { t, errorText } from '#lib/i18n/index.svelte.js';
	import { supported } from '#lib/core/webauthn.js';
	import { NAME, PRODUCT, MAKER } from '#lib/core/brand.js';
	import Logo from './primitives/Logo.svelte';
	import CodeInput from './primitives/CodeInput.svelte';

	let { session, enrollCode = '', onEnrolled, onSignedIn } = $props();

	let waiting = $state(false);
	let error = $state('');
	let notice = $state('');
	let keyName = $state(t('login.defaultKeyName'));
	let username = $state('');
	let password = $state('');
	let step = $state('password'); // password | code
	let code = $state('');
	let useRecovery = $state(false);
	let recoveryCode = $state('');

	async function attempt(fn) {
		error = '';
		waiting = true;
		try {
			await fn();
		} catch (e) {
			error = errorText(e);
		} finally {
			waiting = false;
		}
	}

	const signIn = () => attempt(async () => {
		notice = '';
		await session.login();
		onSignedIn();
	});

	const signInPassword = (ev) => {
		ev.preventDefault();
		attempt(async () => {
			const needsCode = await session.loginPassword(username, password);
			password = '';
			if (needsCode) {
				step = 'code';
				code = recoveryCode = '';
				return;
			}
			onSignedIn();
		});
	};

	const verify = (ev) => {
		ev?.preventDefault();
		if (waiting) return;
		attempt(async () => {
			try {
				await session.loginCode(useRecovery ? recoveryCode : code);
				onSignedIn();
			} catch (e) {
				code = '';
				if (e.code === 'ticket') step = 'password';
				throw e;
			}
		});
	};

	function restart() {
		step = 'password';
		useRecovery = false;
		error = '';
	}

	const register = (ev) => {
		ev.preventDefault();
		attempt(async () => {
			await session.register(enrollCode, keyName);
			onEnrolled();
			notice = t('login.registered');
		});
	};
</script>

<div class="backdrop">
	<MeshBackdrop full />
	<main class="card rise box">
		<div class="brand">
			<Logo title={session.title} size={44} background="var(--surface)" />
			<div><h1>{session.title}</h1><span class="label">{t('shell.lanOnly')}</span></div>
		</div>

		{#if session.mode !== 'password' || enrollCode}
		<div class="key" class:active={waiting} aria-hidden="true">
			<svg width="44" height="44" viewBox="0 0 24 24" fill="none" stroke="var(--accent-2)" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
				<circle cx="8" cy="15" r="4" /><path d="M10.8 12.2 20 3M17 6l3 3M14 9l2 2" />
			</svg>
			{#if waiting}<span class="spinner"></span>{/if}
		</div>
		{/if}

		{#if session.mode === 'password' && !enrollCode && step === 'code'}
			<form onsubmit={verify} class="form rise">
				<div class="phone" aria-hidden="true">
					<svg width="34" height="34" viewBox="0 0 24 24" fill="none" stroke="var(--accent-2)" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
						<rect x="6" y="2.5" width="12" height="19" rx="2.5" /><path d="M10.5 18.5h3" /><path d="m9.5 10.5 2 2 3.5-3.5" />
					</svg>
				</div>
				<h2>{t('login.codeTitle')}</h2>
				<p class="muted">{useRecovery ? t('login.recoveryHelp') : t('login.codeHelp')}</p>
				{#if useRecovery}
					<label for="recovery" class="label">{t('login.recoveryCode')}</label>
					<input id="recovery" class="mono" bind:value={recoveryCode} placeholder="xxxx-xxxx-xxxx" autocomplete="off" autocapitalize="off" spellcheck="false" required />
				{:else}
					<CodeInput bind:value={code} label={t('login.code')} autofocus disabled={waiting} onComplete={verify} />
				{/if}
				<button class="primary" type="submit" disabled={waiting || (!useRecovery && code.length < 6)}>{waiting ? t('login.verifying') : t('login.verify')}</button>
				<div class="links">
					<button type="button" class="link" onclick={() => { useRecovery = !useRecovery; error = ''; }}>{useRecovery ? t('login.useApp') : t('login.useRecovery')}</button>
					<button type="button" class="link" onclick={restart}>{t('login.back')}</button>
				</div>
			</form>
		{:else if session.mode === 'password' && !enrollCode}
			<form onsubmit={signInPassword} class="form">
				<h2>{t('login.passwordTitle')}</h2>
				<label for="username" class="label">{t('login.username')}</label>
				<input id="username" bind:value={username} autocomplete="username" required />
				<label for="password" class="label">{t('login.password')}</label>
				<input id="password" type="password" bind:value={password} autocomplete="current-password" required />
				<button class="primary" type="submit" disabled={waiting}>{t('login.signIn')}</button>
			</form>
		{:else if !supported()}
			<p class="message error" role="alert">{t('login.unsupported')}</p>
		{:else if enrollCode}
			<form onsubmit={register} class="form">
				<h2>{t('login.registerTitle')}</h2>
				<p class="muted">{t('login.registerHelp')}</p>
				<label for="key-name" class="label">{t('login.keyName')}</label>
				<input id="key-name" bind:value={keyName} maxlength="40" required autocomplete="off" />
				<button class="primary" type="submit" disabled={waiting}>{waiting ? t('login.touchKey') : t('login.register')}</button>
			</form>
		{:else if !session.hasPasskeys}
			<h2>{t('login.noKeysTitle')}</h2>
			<p class="muted">{t('login.noKeysHelp')}</p>
			<code>{NAME} -new-enrollment</code>
		{:else}
			<h2>{waiting ? t('login.touchAndPin') : t('login.signInTitle')}</h2>
			<p class="muted">{t('login.signInHelp')}</p>
			<button class="primary" onclick={signIn} disabled={waiting}>{waiting ? t('login.waitingKey') : t('login.signIn')}</button>
		{/if}

		{#if notice}<p class="message ok" role="status">{notice}</p>{/if}
		{#if error}<p class="message error" role="alert">{error}</p>{/if}
		<p class="signature">{PRODUCT} <span>by {MAKER}</span></p>

	</main>
</div>

<style>
	.backdrop { min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 24px 16px; background: var(--bg); }
	.box { position: relative; background: rgba(18, 26, 35, 0.92); backdrop-filter: blur(6px); box-shadow: 0 30px 80px rgba(0, 0, 0, 0.45); }
	.box { width: 100%; max-width: 420px; padding: 32px 28px; display: flex; flex-direction: column; gap: 16px; }
	.brand { display: flex; align-items: center; gap: 14px; }
	.brand div:last-child { display: flex; flex-direction: column; }
	h1 { font-size: 20px; }
	h2 { font-size: 17px; }
	.key { position: relative; width: 96px; height: 96px; margin: 8px auto; border-radius: 50%; background: var(--accent-bg); display: flex; align-items: center; justify-content: center; transition: transform 0.3s; }
	.key.active { transform: scale(1.06); }
	.spinner { position: absolute; inset: -6px; border-radius: 50%; border: 3px solid transparent; border-top-color: var(--accent); animation: spin 1s linear infinite; }
	@keyframes spin { to { transform: rotate(360deg); } }
	p { margin: 0; }
	.form { display: flex; flex-direction: column; gap: 10px; }
	input { min-height: 44px; padding: 10px 12px; border-radius: 10px; border: 1px solid var(--border); background: var(--surface-2); color: var(--text); font: inherit; }
	input:focus { border-color: var(--accent); outline: none; }
	.primary { min-height: 48px; border: 0; border-radius: 12px; background: var(--accent); color: var(--on-accent); font-weight: 600; cursor: pointer; transition: background 0.2s; }
	.primary:hover:not(:disabled) { background: var(--accent-2); }
	.primary:disabled { opacity: 0.7; cursor: progress; }
	code { padding: 10px 12px; border-radius: 10px; background: var(--code-bg); border: 1px solid var(--border-soft); font: 12px var(--font-mono); overflow-x: auto; white-space: nowrap; }
	.phone { width: 64px; height: 64px; border-radius: 18px; background: var(--accent-bg); display: flex; align-items: center; justify-content: center; }
	.mono { font-family: var(--font-mono); letter-spacing: 0.04em; }
	.links { display: flex; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
	.link { padding: 6px 0; border: 0; background: none; color: var(--accent-2); font: inherit; font-size: 13px; cursor: pointer; }
	.link:hover { text-decoration: underline; }
	.message { padding: 10px 12px; border-radius: 10px; font-size: 13px; }
	.signature { margin-top: 6px; text-align: center; font: 500 12px var(--font-mono); letter-spacing: 0.04em; color: var(--text-3); }
	.signature span { color: var(--muted); font-weight: 400; }
	.message.error { background: var(--critical-bg); color: var(--critical-text); }
	.message.ok { background: var(--ok-bg); color: var(--ok-text); }
</style>
