<script>
	import { onMount, untrack, tick } from 'svelte';
	import { t, errorText, languages, i18n } from '#lib/i18n/index.svelte.js';
	import { PRODUCT, MAKER } from '#lib/core/brand.js';
	import { number } from '#lib/i18n/format.js';
	import { analyze, covers } from '#lib/core/network.js';
	import MeshBackdrop from './primitives/MeshBackdrop.svelte';

	let { setup, code = '', authenticated } = $props();

	const STEPS = 4;
	const STRENGTH = { weak: 20, fair: 45, good: 72, strong: 100 };

	let authed = $state(untrack(() => authenticated));
	let step = $state(-1);
	let busy = $state(false);
	let error = $state('');
	let done = $state(null);
	let confirm = $state('');
	let showPassword = $state(false);
	let lockout = $state('');
	let detected = $state('');
	let form = $state({
		language: i18n.locale, title: 'Homelab', origin: '', listen: ':8443', allowed: [],
		certMode: 'generate', certificate: '', key: '', username: 'admin', password: ''
	});

	let netDialog = $state();
	let netInput = $state('');
	let net = $derived(analyze(netInput, form.allowed));
	let canAdd = $derived((net.kind === 'v4' || net.kind === 'v6') && !net.dup);
	let msg = $derived(netMessage(net));

	let clientIP = $derived(setup.status?.client_ip ?? '');
	let deviceIncluded = $derived(form.allowed.some((c) => covers(c, clientIP)));
	let strength = $derived(passwordStrength(form.password));

	function fill() {
		const s = setup.status;
		if (!s) return;
		detected = s.suggested_origin;
		form.origin ||= s.suggested_origin;
		form.listen = s.listen || form.listen;
		if (!form.allowed.length) form.allowed = [...s.suggested_allowed];
	}

	onMount(async () => {
		try {
			if (!authed && code) {
				await setup.login(code);
				authed = true;
			} else if (authed) {
				await setup.load();
			}
			fill();
		} catch (e) {
			error = errorText(e);
		}
	});

	$effect(() => i18n.set(form.language));

	function passwordStrength(p) {
		if (p.length < 12) return 'weak';
		const kinds = [/[a-z]/, /[A-Z]/, /\d/, /[^a-zA-Z0-9]/].filter((r) => r.test(p)).length;
		if (p.length >= 20 || (p.length >= 16 && kinds >= 2)) return 'strong';
		if (p.length >= 14 || kinds >= 3) return 'good';
		return 'fair';
	}

	function netMessage(n) {
		switch (n.kind) {
			case 'empty': return { tone: 'muted', text: t('setup.net.empty') };
			case 'invalid': return { tone: 'critical', text: t('setup.net.invalid') };
			case 'v6': return n.dup ? { tone: 'warning', text: t('setup.net.dup') } : { tone: 'accent', text: t('setup.net.ipv6') };
		}
		if (n.dup) return { tone: 'warning', text: t('setup.net.dup') };
		if (n.public) return { tone: 'warning', text: t('setup.net.public') };
		if (n.bits === 32) return { tone: 'accent', text: t('setup.net.single') };
		return { tone: 'accent', text: t('setup.net.range', { from: n.from, to: n.to, count: number(n.count, 0) }) };
	}

	function removeNet(cidr) {
		const rest = form.allowed.filter((c) => c !== cidr);
		if (covers(cidr, clientIP) && !rest.some((c) => covers(c, clientIP))) {
			lockout = cidr;
			return;
		}
		lockout = '';
		form.allowed = rest;
	}

	async function openNet() {
		netInput = '';
		lockout = '';
		netDialog.showModal();
		await tick();
		netDialog.querySelector('input')?.focus();
	}

	function addNet(ev) {
		ev.preventDefault();
		if (!canAdd) return;
		form.allowed = [...form.allowed, net.cidr];
		netDialog.close();
	}

	function go(i) {
		error = '';
		step = i;
		window.scrollTo({ top: 0 });
	}

	function next(ev) {
		ev.preventDefault();
		error = '';
		if (step === 1 && !form.allowed.length) return (error = t('setup.noNetworks'));
		if (step === 2 && form.password !== confirm) return (error = t('settings.access.mismatch'));
		go(Math.min(step + 1, STEPS - 1));
	}

	async function finish() {
		busy = true;
		error = '';
		try {
			done = await setup.complete({
				language: form.language, title: form.title, origin: form.origin, listen: form.listen,
				allowed: form.allowed,
				certificate: { mode: form.certMode, certificate: form.certificate, key: form.key },
				username: form.username, password: form.password
			});
		} catch (e) {
			error = errorText(e);
		} finally {
			busy = false;
		}
	}
</script>

{#snippet hexLogo(size)}
	<svg class="logo" width={size} height={size} viewBox="0 0 32 32" aria-hidden="true">
		<polygon class="draw" points="16,2 28,9 28,23 16,30 4,23 4,9" fill="none" stroke="var(--accent)" stroke-width="1.7" stroke-linejoin="round" />
		<circle class="core" cx="16" cy="16" r="3.6" fill="var(--accent)" />
	</svg>
{/snippet}

{#snippet arrow()}
	<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14M13 6l6 6-6 6" /></svg>
{/snippet}

{#snippet check(size)}
	<svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12l5 5L20 7" /></svg>
{/snippet}

<div class="page">
	<MeshBackdrop full={step < 0 || !!done} shade={(step < 0 || !authed) && !done} />

	{#if done}
		<main class="center done">
			<span class="hex big-check">{@render check(30)}</span>
			<h1 class="rise">{t('setup.doneTitle')}</h1>
			<p class="lead rise">{form.certMode === 'generate' ? t('setup.doneHelp') : t('setup.doneHelpFiles')}</p>
			<div class="go rise">
				<code>{done.origin}</code>
				<a class="btn primary cta" href={done.origin}>{t('setup.openPanel')} {@render arrow()}</a>
			</div>
			<ul class="later">
				{#each ['server', 'notify', 'key'] as k, i}
					<li class="card rise" style:animation-delay="{0.2 + i * 0.08}s"><strong>{t('setup.after.' + k + '.title')}</strong><span class="label">{t('setup.after.' + k + '.text')}</span></li>
				{/each}
			</ul>
		</main>
	{:else if step < 0 || !authed}
		<header class="intro-top">
			<span class="mono brand">{PRODUCT} <span class="maker">by {MAKER}</span></span>
			<span class="mono label">{t('setup.firstRun')}</span>
		</header>
		<main class="hero">
			{@render hexLogo(72)}
			{#if authed}
				<span class="badge rise mono"><span class="dot ok"></span>{t('setup.verified')}</span>
			{/if}
			<h1 class="rise">{t('setup.hello')}</h1>
			<p class="lead rise">{authed ? t('setup.intro') : t('setup.title')}</p>
			{#if !authed}
				<p class="notice warning">{t('setup.needLink')}</p>
			{:else}
				<ol class="road rise" aria-label={t('setup.stepsLabel')}>
					{#each Array(STEPS) as _, i}
						{#if i}<li aria-hidden="true" class="link" class:flow={i === 1}></li>{/if}
						<li class="stop" class:first={i === 0}><span class="hex mark mono">0{i + 1}</span><span>{t('setup.steps.' + i)}</span></li>
					{/each}
				</ol>
				<div class="cta-row rise">
					<button class="btn primary cta" disabled={!setup.status} onclick={() => go(0)}>{t('setup.start')} {@render arrow()}</button>
					<span class="label">{t('setup.later')}</span>
				</div>
			{/if}
			{#if error}<p class="notice critical" role="alert">{error}</p>{/if}
		</main>
		<footer class="intro-foot mono">
			<span><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><rect x="4" y="11" width="16" height="10" rx="2" /><path d="M8 11V7a4 4 0 0 1 8 0v4" /></svg>{t('setup.secure')}</span>
			{#if clientIP}<span>{clientIP} {@render arrow()} {form.listen}</span>{/if}
		</footer>
	{:else}
		<header class="top">
			<span class="brand-row">{@render hexLogo(26)}<span class="mono brand">{form.title || PRODUCT}</span></span>
			<span class="progress mono label">
				{t('setup.stepOf', { n: step + 1, total: STEPS })}
				<span class="track"><span class="fill" style:width="{((step + 1) * 100) / STEPS}%"></span></span>
			</span>
		</header>

		<nav class="side" aria-label={t('setup.stepsLabel')}>
			<ol>
				{#each Array(STEPS) as _, i}
					{#if i}<li aria-hidden="true" class="vline" class:past={i <= step} class:flowv={i === step + 1}></li>{/if}
					<li class="s" class:now={i === step} class:past={i < step} aria-current={i === step ? 'step' : undefined}>
						{#if i < step}
							<button type="button" class="hex mark done-mark" aria-label={t('setup.steps.' + i)} onclick={() => go(i)}>{@render check(14)}</button>
						{:else if i === step}
							<span class="ring mono">0{i + 1}</span>
						{:else}
							<span class="hex mark mono">0{i + 1}</span>
						{/if}
						{t('setup.steps.' + i)}
					</li>
				{/each}
			</ol>
		</nav>

		<main class="area">
			{#key step}
				<form class="steps" onsubmit={next}>
					{#if step === 0}
						<div class="head rise"><h1>{t('setup.basicsTitle')}</h1><p>{t('setup.basicsHelp')}</p></div>
						<fieldset class="rise">
							<legend>{t('setup.language')}</legend>
							<div class="two">
								{#each languages as l}
									<button type="button" class="option" class:on={form.language === l.id} lang={l.id} aria-pressed={form.language === l.id} onclick={() => (form.language = l.id)}>
										<span class="mono code">{l.id.toUpperCase()}</span><strong>{l.name}</strong>
									</button>
								{/each}
							</div>
							<p class="help">{t('setup.moreLanguages')}</p>
						</fieldset>
						<label class="field rise">
							<span>{t('setup.name')}</span>
							<input class="input big" bind:value={form.title} required maxlength="60" />
							<span class="help">{t('setup.nameHelp')}</span>
						</label>
						<div class="preview rise">
							<span class="mono label">{t('setup.preview')}</span>
							<span class="chip-preview">{@render hexLogo(26)}<span><strong>{form.title || '…'}</strong><span class="label">{t('shell.lanOnly')}</span></span></span>
						</div>
					{:else if step === 1}
						<div class="head rise"><h1>{t('setup.addressTitle')}</h1><p>{t('setup.addressHelp')}</p></div>
						<label class="field rise">
							<span>{t('setup.origin')}</span>
							<span class="with-badge">
								<input class="input big mono" bind:value={form.origin} required pattern="https://.+" />
								{#if form.origin === detected}<span class="detected">{t('setup.detected')}</span>{/if}
							</span>
							<span class="help">{t('setup.originHelp')}</span>
						</label>
						<div class="field rise">
							<span>{t('setup.allowed')}</span>
							<div class="nets">
								{#each form.allowed as c (c)}
									<span class="net mono" class:fresh={!setup.status?.suggested_allowed.includes(c)}>
										{c}
										{#if covers(c, clientIP)}<span class="mine">{t('setup.yourNetwork')}</span>{/if}
										<button type="button" aria-label={t('setup.removeNet', { cidr: c })} onclick={() => removeNet(c)}>×</button>
									</span>
								{/each}
								<button type="button" class="add-net" onclick={openNet}>+ {t('setup.addNetwork')}</button>
							</div>
							{#if lockout}
								<p class="notice warning" role="alert">{t('setup.lockout', { ip: clientIP })}</p>
							{:else if deviceIncluded}
								<p class="ok-line">{@render check(14)}{t('setup.deviceIncluded', { ip: clientIP })}</p>
							{/if}
						</div>
						<fieldset class="rise">
							<legend>{t('setup.certificate')}</legend>
							<div class="two">
								<button type="button" class="option tall" class:on={form.certMode === 'generate'} aria-pressed={form.certMode === 'generate'} onclick={() => (form.certMode = 'generate')}>
									<span class="opt-head">
										<svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="var(--accent)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3l7 3v5c0 5-3 8-7 10-4-2-7-5-7-10V6z" /><path d="M9 12l2 2 4-4" /></svg>
										<span class="tag mono">{t('setup.recommended')}</span>
									</span>
									<strong>{t('setup.generate')}</strong>
									<span class="help">{t('setup.generateHelp')}</span>
								</button>
								<button type="button" class="option tall" class:on={form.certMode === 'files'} aria-pressed={form.certMode === 'files'} onclick={() => (form.certMode = 'files')}>
									<svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="var(--warning)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z" /><path d="M14 3v5h5" /><path d="M9 14h6M9 17h4" /></svg>
									<strong>{t('setup.files')}</strong>
									<span class="help">{t('setup.filesHelp')}</span>
								</button>
							</div>
						</fieldset>
						{#if form.certMode === 'files'}
							<div class="two rise">
								<label class="field"><span>{t('setup.certFile')}</span><input class="input mono" bind:value={form.certificate} required /></label>
								<label class="field"><span>{t('setup.keyFile')}</span><input class="input mono" bind:value={form.key} required /></label>
							</div>
						{/if}
						<label class="field rise">
							<span>{t('setup.listen')}</span>
							<input class="input mono" bind:value={form.listen} required />
							<span class="help">{t('setup.listenHelp')}</span>
						</label>
					{:else if step === 2}
						<div class="head rise"><h1>{t('setup.accountTitle')}</h1><p>{t('setup.accountIntro')}</p></div>
						<label class="field rise"><span>{t('setup.username')}</span><input class="input big" bind:value={form.username} autocomplete="username" required /></label>
						<label class="field rise">
							<span>{t('setup.password')}</span>
							<span class="with-eye">
								<input class="input big mono" type={showPassword ? 'text' : 'password'} bind:value={form.password} autocomplete="new-password" minlength="12" required />
								<button type="button" class="eye" aria-label={showPassword ? t('setup.hide') : t('setup.show')} onclick={() => (showPassword = !showPassword)}>
									<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z" /><circle cx="12" cy="12" r="3" />{#if showPassword}<path d="M4 4l16 16" />{/if}</svg>
								</button>
							</span>
							{#if form.password}
								<span class="strength"><span class="track"><span class="fill {strength}" style:width="{STRENGTH[strength]}%"></span></span><span class="s-{strength}">{t('setup.strength.' + strength)}</span></span>
							{/if}
							<span class="help">{t('setup.passwordHelp')}</span>
						</label>
						<label class="field rise">
							<span>{t('setup.confirm')}</span>
							<span class="with-eye">
								<input class="input big" class:match={confirm && confirm === form.password} type="password" bind:value={confirm} autocomplete="new-password" minlength="12" required />
								{#if confirm && confirm === form.password}<span class="eye ok" role="img" aria-label={t('setup.match')}>{@render check(18)}</span>{/if}
							</span>
						</label>
						<aside class="tip rise">
							<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="var(--warning)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="8" cy="15" r="4" /><path d="M10.8 12.2 20 3M17 6l3 3M14 9l2 2" /></svg>
							<p>{t('setup.keyTip')}</p>
						</aside>
					{:else}
						<div class="head rise"><h1>{t('setup.reviewTitle')}</h1><p>{t('setup.reviewHelp')}</p></div>
						<dl class="review rise">
							{#each [
								[t('setup.nameLanguage'), `${form.title} · ${languages.find((l) => l.id === form.language)?.name}`, 0],
								[t('setup.origin'), form.origin, 1],
								[t('setup.allowed'), form.allowed.join(', '), 1],
								[t('setup.certificate'), form.certMode === 'generate' ? t('setup.generated') : t('setup.ownFiles'), 1],
								[t('setup.account'), `${form.username} · ${t('setup.strength.' + strength).toLowerCase()}`, 2]
							] as [k, v, i]}
								<div><dt>{k}</dt><dd class:mono={i === 1}>{v}</dd><button type="button" class="change" onclick={() => go(i)}>{t('setup.change')}</button></div>
							{/each}
						</dl>
						<p class="help rise">{t('setup.restartNote')}</p>
					{/if}

					{#if error}<p class="notice critical" role="alert">{error}</p>{/if}

					<div class="nav rise">
						<button type="button" class="btn" onclick={() => go(step - 1)}>{t('setup.back')}</button>
						{#if step < STEPS - 1}
							<button class="btn primary cta" type="submit">{t('setup.continue')} {@render arrow()}</button>
						{:else}
							<button class="btn primary cta" type="button" disabled={busy} onclick={finish}>{busy ? t('setup.finishing') : t('setup.finish')}</button>
						{/if}
					</div>
				</form>
			{/key}
		</main>
	{/if}
</div>

<dialog bind:this={netDialog} aria-labelledby="t-net">
	<form onsubmit={addNet}>
		<div class="d-head">
			<div><h2 id="t-net">{t('setup.net.title')}</h2><p class="help">{t('setup.net.help')}</p></div>
			<button type="button" class="x" aria-label={t('setup.net.cancel')} onclick={() => netDialog.close()}>×</button>
		</div>
		<label class="field">
			<span>{t('setup.net.field')}</span>
			<input class="input big mono" class:bad={net.kind === 'invalid'} bind:value={netInput} placeholder="10.8.0.0/24" autocomplete="off" spellcheck="false" />
		</label>
		{#if net.kind === 'v4'}
			<div class="octets" aria-hidden="true">
				{#each net.octets as o}
					<span class="oct {o.kind}"><strong class="mono">{o.text}</strong><span>{t('setup.net.oct.' + o.kind)}</span></span>
				{/each}
			</div>
		{/if}
		<p class="msg {msg.tone}" role="status"><span class="msg-dot"></span>{msg.text}</p>
		{#if net.kind === 'v4' && net.bits < 8}<p class="help">{t('setup.net.huge')}</p>
		{:else if net.kind === 'v4' && net.normalized && !net.dup}<p class="help">{t('setup.net.normalized', { cidr: net.cidr })}</p>{/if}
		{#if clientIP && !clientIP.includes(':')}
			<div class="suggest">
				<span class="label">{t('setup.net.suggestions')}</span>
				<button type="button" class="btn" onclick={() => (netInput = clientIP + '/32')}>{t('setup.net.thisDevice')} · <span class="mono">{clientIP}/32</span></button>
			</div>
		{/if}
		<div class="buttons">
			<button type="button" class="btn" onclick={() => netDialog.close()}>{t('setup.net.cancel')}</button>
			<button type="submit" class="btn primary" disabled={!canAdd}>{t('setup.addNetwork')}</button>
		</div>
	</form>
</dialog>

<style>
	.page { position: relative; min-height: 100vh; overflow: hidden; background: var(--bg); }
	.brand { font-size: 14px; letter-spacing: 0.1em; color: var(--text-2); }
	.maker { color: var(--muted); letter-spacing: 0.04em; }
	.logo .draw { stroke-dasharray: 120; animation: draw 1.6s cubic-bezier(0.4, 0, 0.2, 1) both; }
	.logo .core { transform-origin: 16px 16px; animation: core 2.4s ease-in-out 1.6s infinite; }
	@keyframes draw { from { stroke-dashoffset: 120; } }
	@keyframes core { 50% { transform: scale(1.25); opacity: 0.7; } }
	.cta { min-height: 50px; padding: 0 24px; border-radius: 12px; border-color: var(--accent); background: var(--accent); color: var(--on-accent); font-size: 15px; }
	.cta:hover:not(:disabled) { background: var(--accent-2); }

	/* Welcome */
	.intro-top, .intro-foot { position: relative; max-width: 1280px; margin: 0 auto; padding: 40px 72px 0 120px; display: flex; justify-content: space-between; align-items: center; gap: 12px; }
	.intro-foot { position: fixed; left: 0; right: 0; bottom: 36px; padding-top: 0; font-size: 12px; color: var(--muted); }
	.intro-foot span { display: inline-flex; align-items: center; gap: 8px; }
	.hero { position: relative; max-width: 1280px; margin: 0 auto; min-height: calc(100vh - 140px); padding: 40px 72px 100px 120px; display: flex; flex-direction: column; justify-content: center; align-items: flex-start; gap: 26px; }
	.hero > :not(.logo) { max-width: 700px; }
	.badge { display: inline-flex; align-items: center; gap: 10px; padding: 7px 14px; border-radius: 999px; border: 1px solid var(--border); background: rgba(18, 26, 35, 0.8); font-size: 12px; letter-spacing: 0.06em; text-transform: uppercase; color: var(--ok-text); animation-delay: 0.3s; }
	.hero h1 { font-size: clamp(42px, 6vw, 68px); line-height: 1.02; font-weight: 700; letter-spacing: -0.03em; white-space: pre-line; animation-delay: 0.4s; }
	.lead { margin: 0; font-size: 18px; line-height: 1.6; color: var(--text-3); animation-delay: 0.5s; }
	.road { list-style: none; margin: 8px 0 0; padding: 0; width: 100%; display: flex; align-items: flex-start; animation-delay: 0.6s; }
	.stop { width: 120px; flex: 0 0 auto; display: flex; flex-direction: column; align-items: center; gap: 10px; font-size: 13px; text-align: center; color: var(--text-3); }
	.stop.first { color: var(--text); }
	.mark { width: 46px; height: 46px; display: flex; align-items: center; justify-content: center; background: var(--track); color: var(--muted); }
	.stop.first .mark { background: var(--accent); color: var(--on-accent); font-weight: 500; }
	.link { flex: 1 1 auto; height: 2px; margin-top: 22px; background: var(--border); }
	.link.flow { background-image: repeating-linear-gradient(90deg, var(--accent) 0 8px, transparent 8px 14px); background-size: 28px 2px; animation: flow 1s linear infinite; }
	.cta-row { display: flex; flex-wrap: wrap; align-items: center; gap: 12px 20px; animation-delay: 0.7s; }

	/* Steps */
	.top { position: sticky; top: 0; z-index: 2; height: 76px; padding: 0 56px; display: flex; align-items: center; justify-content: space-between; gap: 16px; border-bottom: 1px solid var(--border-soft); background: rgba(10, 15, 21, 0.88); backdrop-filter: blur(8px); }
	.brand-row { display: flex; align-items: center; gap: 12px; }
	.progress { display: flex; align-items: center; gap: 16px; letter-spacing: 0.06em; text-transform: uppercase; }
	.track { position: relative; display: block; width: 220px; height: 4px; border-radius: 4px; background: var(--border-soft); overflow: hidden; }
	.fill { position: absolute; left: 0; top: 0; bottom: 0; border-radius: 4px; background: var(--accent); transition: width 0.8s cubic-bezier(0.2, 0.7, 0.2, 1); }
	.side { position: fixed; top: 76px; bottom: 0; left: 0; width: 300px; padding: 56px 40px 0 56px; border-right: 1px solid var(--border-soft); background: rgba(10, 15, 21, 0.78); }
	.side ol { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
	.s { display: flex; align-items: center; gap: 14px; color: var(--muted); }
	.s.now { color: var(--text); font-weight: 500; }
	.s .mark { width: 30px; height: 30px; font-size: 12px; border: 0; }
	.done-mark { background: var(--accent); color: var(--on-accent); cursor: pointer; }
	.ring { width: 30px; height: 30px; display: flex; align-items: center; justify-content: center; border-radius: 50%; border: 2px solid var(--accent); color: var(--accent); font-size: 12px; animation: halo 2.2s infinite; }
	@keyframes halo { 0%, 100% { box-shadow: 0 0 0 0 rgba(76, 201, 240, 0.45); } 70% { box-shadow: 0 0 0 8px rgba(76, 201, 240, 0); } }
	.vline { width: 2px; height: 26px; margin-left: 14px; background: var(--border); }
	.vline.past { background: var(--accent); opacity: 0.7; }
	.vline.flowv { opacity: 0.45; background-image: repeating-linear-gradient(180deg, var(--accent) 0 6px, transparent 6px 11px); background-size: 2px 22px; animation: flowv 1s linear infinite; }
	@keyframes flowv { to { background-position: 0 28px; } }

	.area { position: relative; margin-left: 300px; padding: 52px 32px 64px; display: flex; justify-content: center; }
	.steps { width: 100%; max-width: 720px; display: flex; flex-direction: column; gap: 28px; }
	.steps > :nth-child(2) { animation-delay: 0.06s; }
	.steps > :nth-child(3) { animation-delay: 0.12s; }
	.steps > :nth-child(4) { animation-delay: 0.18s; }
	.steps > :nth-child(5) { animation-delay: 0.24s; }
	.head h1 { margin: 0 0 8px; font-size: 34px; font-weight: 700; letter-spacing: -0.02em; }
	.head p { margin: 0; font-size: 16px; color: var(--text-3); }
	fieldset { border: 0; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 12px; min-width: 0; }
	legend { padding: 0; margin-bottom: 12px; font-size: 14px; font-weight: 500; color: var(--text-2); }
	.two { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
	.option { display: flex; flex-direction: column; align-items: flex-start; gap: 6px; padding: 18px 20px; border-radius: 14px; border: 1px solid var(--border-strong); background: rgba(15, 22, 30, 0.95); text-align: left; cursor: pointer; transition: border-color 0.2s, background 0.2s; }
	.option.tall { gap: 10px; padding: 18px; }
	.option.on { border-color: var(--accent); background: rgba(20, 40, 58, 0.6); }
	.option .code { font-size: 13px; color: var(--accent); }
	.option strong { font-size: 17px; font-weight: 600; }
	.opt-head { width: 100%; display: flex; align-items: center; justify-content: space-between; }
	.tag { font-size: 11px; letter-spacing: 0.06em; text-transform: uppercase; color: var(--on-accent); background: var(--accent); padding: 3px 8px; border-radius: 6px; }
	.help { margin: 0; }
	.field > span:first-child { font-size: 14px; }
	.input.big { min-height: 52px; padding: 0 16px; border-radius: 12px; font-size: 16px; background: rgba(15, 22, 30, 0.95); border-color: var(--border-strong); }
	.input.match { border-color: var(--button); }
	.preview { display: flex; align-items: center; gap: 16px; padding: 18px 20px; border-radius: 14px; border: 1px dashed var(--border-strong); background: rgba(15, 22, 30, 0.6); }
	.preview > .label { letter-spacing: 0.08em; text-transform: uppercase; font-size: 11px; }
	.chip-preview { display: flex; align-items: center; gap: 12px; padding: 10px 16px; border-radius: 12px; background: var(--surface-2); border: 1px solid var(--border); }
	.chip-preview > span { display: flex; flex-direction: column; }
	.chip-preview strong { font: 600 15px var(--font-display); }
	.with-badge, .with-eye { position: relative; display: block; }
	.with-badge .input { padding-right: 130px; }
	.detected { position: absolute; right: 10px; top: 13px; padding: 5px 10px; border-radius: 8px; background: var(--accent-bg); color: var(--ok-text); font-size: 12px; }
	.nets { display: flex; flex-wrap: wrap; gap: 10px; }
	.net { display: inline-flex; align-items: center; gap: 10px; min-height: 40px; padding: 0 6px 0 14px; border-radius: 10px; background: var(--surface); border: 1px solid var(--border-strong); font-size: 14px; }
	.net.fresh { animation: pop 0.4s cubic-bezier(0.2, 0.7, 0.2, 1) both; }
	.net button { width: 30px; height: 30px; border: 0; border-radius: 6px; background: transparent; color: var(--muted); cursor: pointer; font-size: 16px; }
	.net button:hover { background: var(--hover); color: var(--text); }
	.mine { font: 12px var(--font-sans); color: var(--ok-text); }
	.add-net { min-height: 40px; padding: 0 14px; border-radius: 10px; border: 1px dashed var(--border-strong); background: transparent; color: var(--text-3); cursor: pointer; }
	.add-net:hover { background: var(--surface); color: var(--text); }
	.ok-line { margin: 0; display: flex; align-items: center; gap: 8px; font-size: 13px; color: var(--muted); }
	.ok-line :global(svg) { color: var(--accent); }
	.eye { position: absolute; right: 6px; top: 6px; width: 40px; height: 40px; display: flex; align-items: center; justify-content: center; border: 0; border-radius: 10px; background: transparent; color: var(--muted); cursor: pointer; }
	.eye:hover { color: var(--text); }
	.eye.ok { color: var(--accent); cursor: default; }
	.with-eye .input { padding-right: 56px; }
	.strength { display: flex; align-items: center; gap: 14px; font-size: 13px; }
	.strength .track { flex: 1 1 auto; width: auto; height: 6px; background: var(--border-soft); }
	.strength .fill.weak { background: var(--critical); }
	.strength .fill.fair { background: var(--warning); }
	.strength .fill.good, .strength .fill.strong { background: linear-gradient(90deg, var(--warning), var(--accent)); }
	.s-weak { color: var(--critical-text); }
	.s-fair { color: var(--warning-text); }
	.s-good, .s-strong { color: var(--ok-text); }
	.tip { display: flex; gap: 14px; padding: 16px 18px; border-radius: 14px; border: 1px solid var(--warning-border); background: rgba(245, 176, 65, 0.06); }
	.tip svg { flex: 0 0 auto; margin-top: 2px; }
	.tip p { margin: 0; font-size: 14px; line-height: 1.55; color: var(--text-2); }
	.review { margin: 0; display: flex; flex-direction: column; border-radius: 16px; border: 1px solid var(--border); background: rgba(18, 26, 35, 0.9); overflow: hidden; }
	.review div { display: grid; grid-template-columns: 180px 1fr auto; align-items: center; gap: 16px; padding: 16px 20px; border-top: 1px solid var(--border-soft); }
	.review div:first-child { border-top: 0; }
	dt { color: var(--muted); font-size: 13px; }
	dd { margin: 0; overflow-wrap: anywhere; }
	.change { padding: 6px 0; border: 0; background: none; color: var(--accent-2); cursor: pointer; font-size: 13px; }
	.change:hover { color: var(--accent); }
	.nav { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding-top: 6px; }
	.notice { margin: 0; }

	/* Done */
	.center { position: relative; max-width: 760px; margin: 0 auto; min-height: 100vh; padding: 64px 24px; display: flex; flex-direction: column; justify-content: center; align-items: center; gap: 22px; text-align: center; }
	.big-check { width: 84px; height: 76px; display: flex; align-items: center; justify-content: center; background: var(--accent); color: var(--on-accent); animation: pop 0.5s cubic-bezier(0.2, 0.7, 0.2, 1) both; }
	.done h1 { font-size: clamp(34px, 5vw, 52px); font-weight: 700; letter-spacing: -0.03em; }
	.go { display: flex; flex-wrap: wrap; justify-content: center; align-items: center; gap: 12px; }
	.go code { padding: 14px 18px; border-radius: 12px; background: rgba(11, 17, 23, 0.9); border: 1px solid var(--border); font: 15px var(--font-mono); }
	a.cta { text-decoration: none; }
	.later { list-style: none; margin: 16px 0 0; padding: 0; width: 100%; display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; text-align: left; }
	.later li { padding: 18px; display: flex; flex-direction: column; gap: 6px; background: rgba(18, 26, 35, 0.92); }
	@keyframes pop { from { opacity: 0; scale: 0.9; } }

	/* Add network dialog */
	dialog { width: min(560px, calc(100vw - 32px)); padding: 0; border-radius: 20px; border: 1px solid var(--border-strong); background: var(--surface); color: var(--text); box-shadow: 0 30px 80px rgba(0, 0, 0, 0.5); }
	dialog[open] { animation: pop 0.25s cubic-bezier(0.2, 0.7, 0.2, 1) both; }
	dialog::backdrop { background: rgba(5, 8, 12, 0.72); }
	dialog form { padding: 26px; display: flex; flex-direction: column; gap: 16px; }
	.d-head { display: flex; justify-content: space-between; gap: 12px; }
	.d-head h2 { font-size: 21px; margin-bottom: 4px; }
	.x { width: 40px; height: 40px; flex: 0 0 auto; border: 1px solid var(--border); border-radius: 10px; background: transparent; color: var(--text-3); cursor: pointer; font-size: 18px; }
	.input.bad { border-color: var(--critical-border); }
	.octets { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; }
	.oct { display: flex; flex-direction: column; align-items: center; gap: 4px; padding: 12px 0; border-radius: 12px; font-size: 11px; }
	.oct strong { font-size: 16px; font-weight: 500; }
	.oct.fixed { border: 1px solid var(--button); background: rgba(20, 40, 58, 0.55); color: var(--accent-2); }
	.oct.any, .oct.range { border: 1px dashed var(--warning-border); background: rgba(245, 176, 65, 0.06); color: var(--warning-text); }
	.msg { margin: 0; display: flex; align-items: center; gap: 10px; font-size: 14px; }
	.msg-dot { width: 8px; height: 8px; border-radius: 50%; flex: 0 0 auto; background: currentColor; }
	.msg.muted { color: var(--muted); }
	.msg.accent { color: var(--accent); }
	.msg.warning { color: var(--warning); }
	.msg.critical { color: var(--critical); }
	.suggest { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; }
	.buttons { display: flex; justify-content: flex-end; gap: 10px; }

	@media (max-width: 900px) {
		.intro-top, .intro-foot { padding-left: 24px; padding-right: 24px; }
		.intro-foot { position: relative; bottom: auto; padding-bottom: 24px; flex-wrap: wrap; }
		.hero { min-height: 0; padding: 48px 24px; }
		.road .stop { width: 72px; font-size: 11px; }
		.top { padding: 0 16px; }
		.track { width: 120px; }
		.side { display: none; }
		.area { margin-left: 0; padding: 32px 16px 48px; }
		.head h1 { font-size: 28px; }
		.two, .later { grid-template-columns: 1fr; }
		.review div { grid-template-columns: 1fr auto; }
		.review dd { grid-column: 1; }
		.review .change { grid-row: 1 / span 2; grid-column: 2; }
	}
</style>
