<script>
	import { onMount } from 'svelte';
	import { t, errorText } from '#lib/i18n/index.svelte.js';
	import { number, dateTime } from '#lib/i18n/format.js';
	import { SEVERITIES, FILTERS, totals, matches, heatTiles } from '#lib/core/cve.svelte.js';

	let { cve, view } = $props();

	const COLOR = { CRITICAL: 'var(--critical)', HIGH: 'var(--warning)', MEDIUM: 'var(--severity-medium)', LOW: 'var(--severity-low)', UNKNOWN: 'var(--border-strong)' };
	const PILL = { CRITICAL: 'critical', HIGH: 'warning', MEDIUM: 'medium', LOW: 'low', UNKNOWN: 'low' };

	onMount(() => cve.start());

	let filter = $state('all');
	let picked = $state('');
	let starting = $state(false);
	let scanError = $state('');

	async function scanNow() {
		starting = true;
		scanError = '';
		try {
			await cve.scanNow();
		} catch (e) {
			scanError = errorText(e);
		} finally {
			starting = false;
		}
	}

	const serverName = (id) => view?.servers.find((s) => s.server.id === id)?.server.name ?? id;
	const validDate = (d) => d && !d.startsWith('0001');

	let report = $derived(cve.report);
	let sums = $derived(totals(report));
	let tiles = $derived(heatTiles(report?.scans ?? []));
	let current = $derived(tiles.find((x) => x.key === picked) ?? tiles[0]);
	let shown = $derived(current ? current.scan.vulnerabilities.filter((v) => matches(v, filter)) : []);

	function pick(tile) {
		picked = tile.key;
		filter = 'all';
	}
</script>

{#if cve.scan}
	{@const sc = cve.scan}
	<section class="card rise scanbar" aria-live="polite">
		<div class="title">
			<h2>{report ? t('cve.headline', { total: number(sums.total, 0), fixable: number(sums.fixable, 0) }) : t('cve.title')}</h2>
			<span class="label">{t('cve.heatHint')}</span>
		</div>
		{#if !sc.enabled}
			<p class="muted">{t('cve.disabled')}</p>
		{:else if sc.running}
			<p><span class="dot ok"></span> {t('cve.scanning')}</p>
		{:else}
			<p class="label">
				{validDate(sc.last_end) ? t('cve.lastScan', { date: dateTime(sc.last_end, 'short') }) : ''}{validDate(sc.next_run) ? ' · ' + t('cve.nextScan', { date: dateTime(sc.next_run, 'short') }) : ''}
			</p>
		{/if}
		{#if sc.enabled}
			<button class="btn" disabled={sc.running || starting} onclick={scanNow}>{t('cve.scanNow')}</button>
		{/if}
		{#if sc.last_error && !sc.running}<p class="notice critical full">{t('cve.scanError', { error: sc.last_error })}</p>{/if}
		{#if scanError}<p class="notice critical full" role="alert">{scanError}</p>{/if}
	</section>
{/if}

{#if report === undefined}
	<div class="card rise empty"><span class="dot ok"></span>{t('cve.loading')}</div>
{:else if !report}
	<section class="card rise empty">
		<h2>{t('cve.none')}</h2>
		<p class="muted">{t('cve.noneHelp')}</p>
	</section>
{:else}
	<div class="layout">
		<div class="map">
			<div class="tiles">
				{#each tiles as tile, i (tile.key)}
					{@const s = tile.scan}
					<button class="tile rise {tile.size} {tile.worst ? PILL[tile.worst] : 'clean'}" class:active={tile === current} aria-pressed={tile === current}
						style:animation-delay="{Math.min(i, 10) * 0.04}s" onclick={() => pick(tile)}
						aria-label={t('cve.tileLabel', { target: s.kind === 'system' ? t('cve.systemPackages') : s.target, server: serverName(s.server), n: tile.total })}>
						<span class="tile-head"><span class="kind {s.kind}">{t('cve.kind.' + s.kind)}</span><strong class="total">{number(tile.total, 0)}</strong></span>
						<span class="tile-name"><strong class="mono">{s.kind === 'system' ? t('cve.systemPackages') : s.target}</strong><span class="label">{serverName(s.server)}</span></span>
						<span class="stack" aria-hidden="true">
							{#each SEVERITIES as sev}
								{#if s.count[sev]}<span style:width="{(s.count[sev] * 100) / tile.total}%" style:background={COLOR[sev]}></span>{/if}
							{/each}
						</span>
					</button>
				{/each}
				{#each (cve.scan?.skipped ?? []) as sk}
					<div class="tile skipped" title={sk.detail ?? ''}>
						<span class="kind">{t('cve.skippedTile')}</span>
						<span class="reason">{t('cve.skip.' + sk.code)}</span>
						<span class="tile-name"><strong class="mono">{sk.target || t('cve.systemPackages')}</strong><span class="label">{serverName(sk.server)}</span></span>
					</div>
				{/each}
			</div>
			<div class="legend">
				{#each SEVERITIES as s}
					<span><span class="swatch" style:background={COLOR[s]}></span>{t('cve.severity.' + s)} <strong class="mono">{number(report.count[s] ?? 0, 0)}</strong>
						<span class="label">{t('cve.fixable', { n: number(report.fixable[s] ?? 0, 0) })}</span></span>
				{/each}
			</div>
		</div>

		{#if current}
			{@const s = current.scan}
			{#key current.key}
				<section class="card detail slide" aria-labelledby="t-target">
					<span class="label">{serverName(s.server)} · {t('cve.kind.' + s.kind)} · {dateTime(s.date, 'short')}</span>
					<h2 id="t-target" class="mono">{s.kind === 'system' ? t('cve.systemPackages') : s.target}</h2>
					{#if s.vulnerabilities.length}
						<div class="filters" role="group" aria-label={t('cve.filter')}>
							{#each FILTERS as f}
								<button class:active={filter === f} aria-pressed={filter === f} onclick={() => (filter = f)}>{t('cve.filters.' + f)}</button>
							{/each}
						</div>
						<ul>
							{#each shown.slice(0, 60) as v}
								<li>
									<span class="pill {PILL[v.severity] ?? 'low'}">{t('cve.severity.' + v.severity)}</span>
									<span class="text">
										<span class="mono">{#if v.url}<a href={v.url} target="_blank" rel="noopener noreferrer">{v.id}</a>{:else}{v.id}{/if} · {v.package}</span>
										<span class="label">{v.title || t('cve.noDescription')}</span>
										<span class="mono version">{v.installed}{#if v.fixed}{' '}<svg class="arrow" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14M13 6l6 6-6 6" /></svg><span class="sr-only">{t('cve.fixedIn')}</span> <strong class="text-ok">{v.fixed}</strong>{:else}{' · '}<span class="muted">{t('cve.noFix')}</span>{/if}</span>
									</span>
								</li>
							{:else}
								<li class="muted">{t('cve.noneWithFilter')}</li>
							{/each}
							{#if shown.length > 60}<li class="muted">{t('cve.more', { n: shown.length - 60 })}</li>{/if}
						</ul>
					{:else}
						<p class="notice ok">{t('cve.clean')}</p>
					{/if}
				</section>
			{/key}
		{/if}
	</div>
{/if}
{#if cve.error}<p class="card alert" role="alert">{errorText(cve.error)}</p>{/if}

<style>
	.scanbar { padding: 16px 20px; display: flex; flex-wrap: wrap; align-items: center; gap: 10px 16px; }
	.scanbar .title { flex: 1 1 320px; display: flex; flex-direction: column; gap: 2px; }
	.scanbar h2 { font-size: 20px; font-weight: 600; }
	.scanbar p { margin: 0; display: flex; align-items: center; gap: 8px; }
	.scanbar .full { flex: 1 1 100%; }
	.empty { padding: 22px; display: flex; flex-direction: column; gap: 8px; }
	.empty p { margin: 0; }
	.empty h2 { font-size: 17px; }

	.layout { display: flex; gap: 20px; align-items: flex-start; }
	.map { flex: 1 1 auto; min-width: 0; display: flex; flex-direction: column; gap: 14px; }
	.tiles { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); grid-auto-rows: 150px; grid-auto-flow: row dense; gap: 12px; }
	.tile { display: flex; flex-direction: column; gap: 10px; padding: 16px; border-radius: 16px; border: 1px solid var(--border); background: var(--surface); text-align: left; cursor: pointer; min-width: 0; transition: border-color 0.2s, translate 0.2s; }
	.tile:hover { translate: 0 -2px; }
	.tile.big { grid-column: span 2; grid-row: span 2; }
	.tile.wide { grid-column: span 2; }
	.tile.critical { background: var(--critical-fill); border-color: var(--critical-border); }
	.tile.warning { background: var(--warning-fill); border-color: var(--warning-border); }
	.tile.medium { border-color: var(--border-strong); }
	.tile.clean { border-color: var(--ok-bg); }
	.tile.active { outline: 2px solid var(--accent); outline-offset: -1px; }
	.tile.skipped { border-style: dashed; border-color: var(--border-strong); background: var(--surface-2); cursor: default; }
	.tile.skipped { grid-column: span 2; }
	.reason { font-size: 13px; line-height: 1.4; color: var(--text-3); }
	.tile-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 8px; }
	.total { font: 600 30px/1 var(--font-display); }
	.big .total { font-size: 56px; }
	.critical .total { color: var(--critical-text); }
	.warning .total { color: var(--warning-text); }
	.clean .total { color: var(--ok-text); }
	.tile-name { margin-top: auto; display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.tile-name strong { font-weight: 500; font-size: 14px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.stack { display: flex; height: 6px; border-radius: 6px; overflow: hidden; background: var(--track); }
	.kind { align-self: flex-start; padding: 3px 8px; border-radius: 6px; font-size: 11px; font-weight: 600; background: var(--code-bg); color: var(--text-3); }
	.kind.image { color: var(--accent-2); }
	.legend { display: flex; flex-wrap: wrap; gap: 10px 22px; font-size: 13px; }
	.legend > span { display: inline-flex; align-items: center; gap: 8px; }
	.swatch { width: 10px; height: 10px; border-radius: 3px; }

	.detail { flex: 0 0 440px; min-width: 0; padding: 22px 24px; display: flex; flex-direction: column; gap: 12px; border-color: var(--border-strong); position: sticky; top: 92px; max-height: calc(100vh - 110px); overflow: auto; }
	.slide { animation: slide 0.35s cubic-bezier(0.2, 0.7, 0.2, 1) both; }
	@keyframes slide { from { opacity: 0; translate: 16px 0; } }
	.detail h2 { font-size: 17px; font-weight: 500; overflow-wrap: anywhere; }
	.filters { display: flex; flex-wrap: wrap; gap: 6px; }
	.filters button { min-height: 36px; padding: 0 14px; border-radius: 999px; border: 1px solid var(--border); background: transparent; color: var(--text-3); cursor: pointer; font-size: 13px; }
	.filters button.active { background: var(--accent-bg); border-color: var(--button); color: var(--text); }
	ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
	li { display: grid; grid-template-columns: 84px 1fr; gap: 12px; align-items: start; padding: 11px 0; border-top: 1px solid var(--border-soft); }
	li .pill { justify-self: start; white-space: nowrap; }
	.text { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.text .mono { font-size: 13px; overflow-wrap: anywhere; }
	.version { font-size: 12px; color: var(--text-3); }
	.arrow { vertical-align: -1px; }
	.pill.medium { background: var(--badge-muted); color: var(--text-2); }
	.pill.low { background: var(--severity-low-bg); color: var(--muted); }
	.notice { margin: 0; }
	.alert { padding: 12px 16px; border-color: var(--critical-border); color: var(--critical-text); }

	@media (max-width: 1100px) {
		.layout { flex-direction: column; align-items: stretch; }
		.detail { flex: none; position: static; max-height: none; }
	}
	@media (max-width: 640px) {
		.tiles { grid-template-columns: repeat(2, minmax(0, 1fr)); grid-auto-rows: 130px; }
		.big .total { font-size: 40px; }
	}
</style>
