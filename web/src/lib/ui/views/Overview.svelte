<script>
	import { t, findingText } from '#lib/i18n/index.svelte.js';
	import { celsius } from '#lib/i18n/format.js';
	import { summarize, topology, containerStatus, nodeLabel } from '#lib/core/derive.js';
	import Ring from '../primitives/Ring.svelte';
	import ServerNode from '../primitives/ServerNode.svelte';
	import Kpi from '../primitives/Kpi.svelte';

	let { view, description = '' } = $props();

	let sum = $derived(summarize(view));
	let topo = $derived(topology(view));
	let scoreColor = $derived(view.score >= 90 ? 'var(--ok)' : view.score >= 60 ? 'var(--warning)' : 'var(--critical)');
	const containerText = (c) => {
		const s = containerStatus(c);
		return t('container.' + s.code, s.args);
	};
</script>

<div class="row">
	<section class="card rise health" aria-label={t('overview.health')}>
		<Ring value={view.score} size={132} stroke={8} label={t('overview.health')} color={scoreColor} center={String(view.score)} />
		<div class="health-text">
			<span class="pill {view.level}"><span class="dot {view.level}"></span>{t('level.' + view.level)}</span>
			<h2>{t('overview.headline.' + view.level)}</h2>
			<p class="muted">{t('overview.summary', { online: sum.online, total: sum.total, running: sum.running, containers: sum.containers.length })}</p>
		</div>
	</section>

	<div class="kpis">
		<Kpi delay={0.05} label={t('overview.servers')} value="{sum.online} / {sum.total}"
			note={sum.online === sum.total ? t('overview.allRespond') : t('overview.someDown')} tone={sum.online === sum.total ? 'ok' : 'critical'} />
		<Kpi delay={0.12} label={t('overview.containers')} value="{sum.running} / {sum.containers.length}"
			note={sum.running === sum.containers.length ? t('overview.allRunning') : t('overview.checkStopped')} tone={sum.running === sum.containers.length ? 'ok' : 'warning'} />
		<Kpi delay={0.19} label={t('overview.maxTemp')} value={celsius(sum.maxTemp)} note={t('overview.hottestCpu')} tone={sum.maxTemp >= 80 ? 'warning' : 'muted'} />
		<Kpi delay={0.26} label={t('overview.findings')} value={String(sum.findings.length)}
			note={sum.findings.length ? t('overview.seeBelow') : t('overview.nothing')} tone={sum.findings.length ? 'warning' : 'ok'} />
	</div>
</div>

{#if sum.findings.length}
	<section class="card rise block" aria-labelledby="t-attention">
		<h2 id="t-attention">{t('overview.attention')}</h2>
		<ul class="list">
			{#each sum.findings as f}
				<li><span class="pill {f.level}">{t('level.' + f.level)}</span><span>{findingText(f)}</span><span class="muted">{f.server}</span></li>
			{/each}
		</ul>
	</section>
{/if}

{#snippet chips(s)}
	<div class="chips">{#each s.snapshot?.containers ?? [] as c}<span class="chip {containerStatus(c).level}">{c.name}</span>{/each}</div>
{/snippet}

{#snippet branch(list)}
	<div class="branch">
		{#each list as s}
			<div class="leaf">
				<ServerNode text={nodeLabel(s.server)} level={s.level} size={72} />
				<div class="leaf-text">
					<strong>{s.server.name}</strong>
					<span class="label">{s.server.role}</span>
					{@render chips(s)}
				</div>
			</div>
		{/each}
	</div>
{/snippet}

<section class="card rise block topology" aria-labelledby="t-topo">
	<div class="heading"><h2 id="t-topo">{t('overview.topology')}</h2><span class="label">{t('overview.topologyHint')}</span></div>
	<div class="map">
		<div class="tier" style:--cap="34px">
			<div class="hex internet">
				<svg aria-hidden="true" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="var(--muted)" stroke-width="1.8" stroke-linecap="round"><circle cx="12" cy="12" r="9" /><path d="M3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18" /></svg>
			</div>
			<div class="caption"><strong>{t('overview.internet')}</strong></div>
		</div>
		{#if topo.edge.length}
			<div class="link flow" aria-hidden="true"></div>
			{#if topo.edge.length === 1}
				{@const s = topo.edge[0]}
				<div class="tier" style:--cap="86px">
					<ServerNode text={nodeLabel(s.server)} level={s.level} size={104} highlight />
					<div class="caption">
						<strong>{s.server.name}</strong>
						<span class="label">{s.server.location}</span>
						{@render chips(s)}
					</div>
				</div>
			{:else}
				{@render branch(topo.edge)}
			{/if}
		{/if}
		{#if topo.internal.length}
			<div class="link flow" aria-hidden="true"></div>
			{@render branch(topo.internal)}
		{/if}
	</div>
	{#if description}<p class="label centered">{description}</p>{/if}
</section>

<section class="card rise block" aria-labelledby="t-services">
	<div class="heading"><h2 id="t-services">{t('overview.services')}</h2><span class="label">{t('overview.servicesHint', { running: sum.running, total: sum.containers.length })}</span></div>
	<div class="services">
		{#each sum.containers as c}
			{@const level = containerStatus(c).level}
			<div class="service">
				<span class="dot {level}"></span>
				<div class="service-text"><span>{c.name}</span><span class="label">{c.server} · {c.vault ? `Vault ${c.vault.version}` : c.image}</span></div>
				<span class="pill {level}">{containerText(c)}</span>
			</div>
		{/each}
	</div>
</section>

<style>
	.row { display: flex; flex-wrap: wrap; gap: 18px; }
	.health { flex: 1 1 300px; padding: 22px; display: flex; align-items: center; gap: 22px; flex-wrap: wrap; }
	.health-text { display: flex; flex-direction: column; gap: 8px; flex: 1 1 160px; align-items: flex-start; }
	.health-text h2 { font-size: 19px; }
	.health-text p { margin: 0; font-size: 13px; }
	.kpis { flex: 2 1 420px; display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 14px; }
	.block { padding: 20px 22px; display: flex; flex-direction: column; gap: 12px; animation-delay: 0.2s; }
	.block h2 { font-size: 17px; }
	.heading { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: baseline; gap: 8px; }
	.list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
	.list li { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 14px; padding: 10px 0; border-bottom: 1px solid var(--border-soft); }
	.list li span:nth-child(2) { flex: 1 1 200px; }
	.topology { background: var(--surface-sunken); overflow: hidden; }
	.map { display: flex; align-items: center; justify-content: center; padding: 8px 0; }
	.tier { position: relative; flex: 0 0 auto; display: flex; justify-content: center; padding: var(--cap) 0; }
	.caption { position: absolute; left: 50%; top: calc(100% - var(--cap) + 8px); width: 200px; translate: -50% 0; display: flex; flex-direction: column; align-items: center; gap: 2px; text-align: center; }
	.caption .chips { justify-content: center; }
	.internet { width: 92px; height: 80px; background: var(--track); display: flex; align-items: center; justify-content: center; }
	.link { flex: 0 1 88px; min-width: 36px; margin: 0 10px; }
	.link:has(+ .branch) { margin-right: 0; }
	.branch { --bus: 22px; display: flex; flex-direction: column; gap: 14px; padding-left: var(--bus); }
	.leaf { position: relative; display: flex; align-items: center; gap: 14px; }
	.leaf::before { content: ''; position: absolute; left: calc(-1 * var(--bus)); top: 50%; width: calc(var(--bus) - 4px); border-top: 2px dashed var(--accent); opacity: 0.6; }
	.leaf::after { content: ''; position: absolute; left: calc(-1 * var(--bus)); top: 0; bottom: -14px; border-left: 2px dashed var(--accent); opacity: 0.6; }
	.leaf:first-child::after { top: 50%; }
	.leaf:last-child::after { bottom: 50%; }
	.leaf:only-child::after { display: none; }
	.leaf-text { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.chips { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 6px; max-width: 300px; }
	.centered { text-align: center; margin: 0; }
	.services { display: grid; grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); gap: 0 24px; }
	.service { display: flex; align-items: center; gap: 12px; padding: 11px 0; border-bottom: 1px solid var(--border-soft); }
	.service-text { display: flex; flex-direction: column; min-width: 0; flex: 1 1 auto; }
	.service-text .label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	@media (max-width: 720px) {
		.map { flex-direction: column; }
		.tier { padding: 0; flex-direction: column; align-items: center; gap: 8px; }
		.caption { position: static; translate: none; width: auto; }
		.branch { padding-left: 0; }
		.leaf::before, .leaf::after { display: none; }
		.link:has(+ .branch) { margin-right: 0; margin-bottom: 14px; }
		.link { flex: 0 0 28px; width: 2px; min-width: 0; margin: 10px 0; background-image: repeating-linear-gradient(180deg, var(--accent) 0 6px, transparent 6px 11px); background-size: 2px 22px; animation: flowv 1s linear infinite; }
	}
	@keyframes flowv { to { background-position: 0 22px; } }
</style>
