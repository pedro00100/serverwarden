<script>
	import { t } from '$lib/i18n/index.svelte.js';
	import { bytes, number, agoDate, dateTime, clock } from '$lib/i18n/format.js';
	import { securitySummary, securityEvents, hoursSince } from '$lib/core/derive.js';

	let { view, onNavigate } = $props();

	const KINDS = ['all', 'login', 'backup', 'lynis'];
	const WINDOW_H = 24;

	let sec = $derived(securitySummary(view));
	let events = $derived(securityEvents(sec));
	let kind = $state('all');
	let shown = $derived(events.filter((e) => kind === 'all' || e.kind === kind));
	let lanes = $derived(
		['login', 'backup', 'lynis'].map((k) => ({
			kind: k,
			marks: events.filter((e) => e.kind === k && hoursSince(e.at) <= WINDOW_H).map((e) => ({ ...e, left: 100 - (hoursSince(e.at) * 100) / WINDOW_H }))
		}))
	);
	let axis = $derived([24, 18, 12, 6].map((h) => clock(Date.now() - h * 3_600_000)));
	let alerts = $derived(alertBars(sec.alertsByHour));

	// A bar stands out when it is well above the day's average.
	function alertBars(hours) {
		if (!hours) return null;
		const total = hours.reduce((a, b) => a + b, 0);
		const max = Math.max(1, ...hours);
		const mean = total / hours.length;
		return {
			total,
			bars: hours.map((n, i) => ({
				n,
				height: n ? Math.max(6, (n * 100) / max) : 2,
				peak: n >= 5 && n > mean * 2,
				title: t('security.alertsAt', { n, time: clock(Date.now() - (hours.length - i) * 3_600_000) })
			}))
		};
	}

	const scoreLevel = (i) => (i >= 75 ? 'ok' : i >= 60 ? 'warning' : 'critical');
	const eventText = (e) => t('security.event.' + e.kind, e);
	const eventMeta = (e) => {
		switch (e.kind) {
			case 'login': return t('security.from', { ip: e.from }) + ' · ' + e.fingerprint.slice(0, 18) + '…';
			case 'backup': return t('security.backupDetail', { n: e.snapshots, size: bytes(e.bytes) });
			default: return e.warnings ? t('security.warnings', { n: e.warnings }) : t('security.noWarnings');
		}
	};
</script>

{#if !sec.ready}
	<div class="card rise empty"><span class="dot ok"></span>{t('security.collecting')}</div>
{:else}
	<div class="layout">
		<section class="card rise main" aria-labelledby="t-timeline">
			<div class="heading"><h2 id="t-timeline">{t('security.timeline')}</h2><span class="label">{t('security.timelineHint')}</span></div>

			<div class="lanes">
				{#if alerts}
					<span class="lane-label">{t('security.lane.alerts')}</span>
					<div class="histogram" role="img" aria-label={t('security.alertsTotal', { n: alerts.total })}>
						{#each alerts.bars as b, i}
							<span class="bar" class:peak={b.peak} style:height="{b.height}%" style:animation-delay="{i * 0.02}s" title={b.title}></span>
						{/each}
					</div>
				{/if}
				{#each lanes as l}
					<span class="lane-label">{t('security.lane.' + l.kind)}</span>
					<div class="lane">
						{#each l.marks as m, i}
							<span class="mark {m.kind} {m.level}" style:left="{m.left}%" style:animation-delay="{0.2 + i * 0.06}s" title="{eventText(m)} · {m.server} · {dateTime(m.at, 'short')}"></span>
						{/each}
					</div>
				{/each}
				<span></span>
				<div class="axis mono" aria-hidden="true">{#each axis as a}<span>{a}</span>{/each}<span>{t('time.nowShort')}</span></div>
			</div>

			<div class="filters" role="group" aria-label={t('security.filter')}>
				{#each KINDS as k}
					<button class:active={kind === k} aria-pressed={kind === k} onclick={() => (kind = k)}>{t('security.kinds.' + k)}</button>
				{/each}
			</div>

			<ol class="events">
				{#each shown as e (e.id)}
					<li>
						<span class="mono time">{dateTime(e.at, 'short')}</span>
						<span class="mark static {e.kind} {e.level}" aria-hidden="true"></span>
						<span class="text"><span>{eventText(e)}</span><span class="label mono">{eventMeta(e)}</span></span>
						<span class="pill {e.level === 'ok' ? '' : e.level}">{e.server}</span>
					</li>
				{:else}
					<li class="muted">{t('security.noEvents')}</li>
				{/each}
			</ol>
		</section>

		<aside>
			<section class="card rise block" aria-labelledby="t-lynis" style:animation-delay="0.08s">
				<div class="heading"><h2 id="t-lynis">{t('security.hardening')}</h2><span class="label">Lynis</span></div>
				{#each sec.lynis as l}
					{@const lv = scoreLevel(l.index)}
					<div class="score">
						<div class="score-head"><span>{l.name}</span><span class="mono text-{lv}">{l.index} / 100</span></div>
						<span class="track"><span class="fill {lv}" style:width="{l.index}%"></span></span>
						<span class="label">{l.warnings ? t('security.warnings', { n: l.warnings }) : t('security.noWarnings')}{l.date ? ` · ${agoDate(l.date)}` : ''}</span>
					</div>
				{:else}
					<p class="muted">{t('security.noLynis')}</p>
				{/each}
			</section>

			<section class="card rise block" aria-labelledby="t-backups" style:animation-delay="0.14s">
				<h2 id="t-backups">{t('security.backups')}</h2>
				{#each sec.backups as b}
					{@const level = hoursSince(b.last) < 36 ? 'ok' : 'warning'}
					<div class="line">
						<span class="dot {level}"></span>
						<div class="text"><strong>{b.name}</strong><span class="label">{t('security.backupDetail', { n: b.snapshots, size: bytes(b.bytes) })}</span></div>
						<span class="pill {level}">{agoDate(b.last)}</span>
					</div>
				{:else}
					<p class="muted">{t('security.noBackups')}</p>
				{/each}
			</section>

			<section class="card rise block stats" style:animation-delay="0.2s">
				<div>
					<span class="label">{t('security.blocked')}</span>
					<strong class="value">{sec.crowdsec ? number(sec.crowdsec.community_bans + sec.crowdsec.local_bans, 0) : '—'}</strong>
					<span class="label">{sec.crowdsec ? t('security.blockedDetail', { local: sec.crowdsec.local_bans, alerts: sec.crowdsec.alerts_24h }) : t('security.noCrowdsec')}</span>
				</div>
				<div>
					<span class="label">{t('security.updates')}</span>
					<strong class="value" class:text-warning={sec.pendingUpdates}>{sec.pendingUpdates ?? '—'}</strong>
					{#if sec.pendingUpdates}
						<button class="link" onclick={() => onNavigate('actions')}>{t('security.checkNow')}</button>
					{:else}
						<span class="label">{sec.pendingUpdates === 0 ? t('security.upToDate') : t('security.unknown')}</span>
					{/if}
				</div>
				<div>
					<span class="label">{t('security.keyLogins')}</span>
					<strong class="value">{sec.logins.length}</strong>
					<span class="label">{t('security.keyLoginsDetail', { n: sec.automated })}</span>
				</div>
			</section>

			<section class="card rise block" aria-labelledby="t-audit" style:animation-delay="0.26s">
				<div class="heading"><h2 id="t-audit">{t('security.audit')}</h2><span class="label">{t('security.auditHint')}</span></div>
				<div class="chips">
					{#each sec.audit as x}<span class="audit" title={x.server}><span class="mono">{x.key}</span><span class="label">{x.server}</span><strong>{x.n}</strong></span>{:else}<span class="muted">{t('security.noChanges')}</span>{/each}
				</div>
			</section>
		</aside>
	</div>
{/if}

<style>
	.empty { padding: 20px; display: flex; align-items: center; gap: 12px; color: var(--text-2); }
	.layout { display: flex; gap: 20px; align-items: flex-start; }
	.main { flex: 2 1 0; min-width: 0; padding: 22px 24px; display: flex; flex-direction: column; gap: 18px; }
	aside { flex: 1 1 0; min-width: 300px; display: flex; flex-direction: column; gap: 18px; }
	.block { padding: 20px 22px; display: flex; flex-direction: column; gap: 12px; }
	h2 { font-size: 17px; }
	.heading { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: baseline; gap: 8px; }

	.lanes { display: grid; grid-template-columns: 130px 1fr; row-gap: 10px; align-items: center; }
	.lane-label { font-size: 13px; color: var(--text-3); }
	.lane { position: relative; height: 28px; border-radius: 8px; background: var(--surface-2); }
	.mark { position: absolute; top: 7px; width: 14px; height: 14px; margin-left: -7px; border-radius: 4px; rotate: 45deg; background: var(--accent); animation: blip 0.4s cubic-bezier(0.2, 0.7, 0.2, 1) both; }
	.mark.backup { background: var(--ok); }
	.mark.lynis { background: var(--severity-medium); }
	.mark.warning { background: var(--warning); }
	.mark.static { position: static; margin: 0 0 0 9px; width: 10px; height: 10px; border-radius: 3px; animation: none; }
	@keyframes blip { from { opacity: 0; scale: 0; } }
	.histogram { height: 56px; display: flex; align-items: flex-end; gap: 3px; }
	.bar { flex: 1 1 0; border-radius: 3px 3px 0 0; background: var(--button); transform-origin: bottom; animation: up 0.7s cubic-bezier(0.2, 0.7, 0.2, 1) both; }
	.bar.peak { background: var(--warning); }
	@keyframes up { from { transform: scaleY(0); } }
	.axis { display: flex; justify-content: space-between; font-size: 11px; color: var(--muted); }

	.filters { display: flex; flex-wrap: wrap; gap: 8px; padding-top: 14px; border-top: 1px solid var(--border-soft); }
	.filters button { min-height: 36px; padding: 0 14px; border-radius: 999px; border: 1px solid var(--border); background: transparent; color: var(--text-3); cursor: pointer; font-size: 13px; }
	.filters button.active { background: var(--accent-bg); border-color: var(--button); color: var(--text); }
	.events { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
	.events li { display: grid; grid-template-columns: 136px 28px 1fr auto; align-items: center; gap: 12px; padding: 11px 0; border-bottom: 1px solid var(--border-soft); }
	.time { font-size: 12px; color: var(--muted); white-space: nowrap; }
	.text { display: flex; flex-direction: column; min-width: 0; }
	.text .label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

	.score { display: flex; flex-direction: column; gap: 6px; }
	.score-head { display: flex; justify-content: space-between; font-size: 14px; }
	.track { display: block; height: 8px; border-radius: 8px; background: var(--track); overflow: hidden; }
	.fill { display: block; height: 100%; border-radius: 8px; background: var(--ok); transform-origin: left; animation: grow 0.9s cubic-bezier(0.2, 0.7, 0.2, 1) both; }
	.fill.warning { background: var(--warning); }
	.fill.critical { background: var(--critical); }
	@keyframes grow { from { transform: scaleX(0); } }
	.line { display: flex; align-items: center; gap: 12px; padding: 10px 0; border-bottom: 1px solid var(--border-soft); }
	.line:last-child { border-bottom: 0; }
	.stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(120px, 1fr)); gap: 14px; }
	.stats div { display: flex; flex-direction: column; gap: 2px; align-items: flex-start; }
	.value { font: 600 24px/1.2 var(--font-display); }
	.link { padding: 0; border: 0; background: none; color: var(--accent-2); cursor: pointer; font-size: 12px; min-height: 24px; }
	.link:hover { color: var(--accent); }
	.chips { display: flex; flex-wrap: wrap; gap: 8px; }
	.audit { display: inline-flex; gap: 8px; align-items: center; padding: 6px 10px; border-radius: 8px; background: var(--hover); border: 1px solid var(--chip-border); font-size: 12px; }

	@media (max-width: 1100px) {
		.layout { flex-direction: column; align-items: stretch; }
		aside { min-width: 0; }
	}
	@media (max-width: 640px) {
		.main { padding: 18px; }
		.lanes { grid-template-columns: 90px 1fr; }
		.events li { grid-template-columns: 1fr auto; }
		.events .time { grid-column: 1 / -1; }
		.events .mark.static { display: none; }
	}
</style>
