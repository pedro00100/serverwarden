<script>
	import { t, findingText } from '$lib/i18n/index.svelte.js';
	import { bytes, duration, celsius, number } from '$lib/i18n/format.js';
	import { pct, primaryTemp, diskStatus, containerStatus, usageTone, worstFirst, nodeLabel } from '$lib/core/derive.js';
	import Ring from '../primitives/Ring.svelte';
	import ServerNode from '../primitives/ServerNode.svelte';
	import Meter from '../primitives/Meter.svelte';

	let { view, selected = '', onSelect, onNavigate } = $props();

	let current = $derived(view.servers.find((s) => s.server.id === selected) ?? worstFirst(view.servers)[0]);

	const usage = (x) => (x ? [x.cpu_pct, pct(x.memory), pct(x.disk)] : []);
	const diskText = (d) => {
		const s = diskStatus(d);
		return t('disk.' + s.code, s.args);
	};
	const containerText = (c) => {
		const s = containerStatus(c);
		return t('container.' + s.code, s.args);
	};
	const batteryText = (b) => {
		const key = 'servers.batteryStatus.' + b.status;
		return t(key) === key ? b.status : t(key);
	};
	const stateText = (s) => (s.error ? t('servers.noResponse') : t('level.' + s.level));
</script>

<div class="layout">
	<aside aria-label={t('nav.servers')}>
		<p class="label count">{t('servers.count', { n: view.servers.length, review: view.servers.filter((s) => s.level !== 'ok').length })}</p>
		<div class="list">
			{#each view.servers as s, i}
				{@const u = usage(s.snapshot)}
				<button class="item rise" class:active={s === current} aria-pressed={s === current} style:animation-delay="{i * 0.05}s" onclick={() => onSelect(s.server.id)}>
					<span class="row">
						<ServerNode text={nodeLabel(s.server)} level={s.level} size={44} />
						<span class="name"><strong>{s.server.name}</strong><span class="label">{s.server.role}</span></span>
						<span class="dot {s.level}" title={stateText(s)}></span>
					</span>
					{#if s.snapshot}
						<span class="bars">
							{#each ['cpu', 'memory', 'disk'] as m, k}
								<Meter label={t('servers.' + m)} value={u[k]} />
							{/each}
						</span>
					{:else}
						<span class="label">{stateText(s)}</span>
					{/if}
				</button>
			{/each}
		</div>
	</aside>

	{#if current}
		{@const s = current}
		{@const x = s.snapshot}
		{#key s.server.id}
			<section class="card detail rise" aria-labelledby="srv-title">
				<header>
					<ServerNode text={nodeLabel(s.server)} level={s.level} size={68} />
					<div class="title">
						<h2 id="srv-title">{s.server.name}</h2>
						<span class="label">{[s.server.role, x?.os].filter(Boolean).join(' · ')}</span>
					</div>
					<span class="pill {s.level}"><span class="dot {s.level}"></span>{stateText(s)}</span>
					{#if x}<span class="label up">{t('servers.upFor', { time: duration(x.uptime_sec) })}</span>{/if}
				</header>

				{#if x}
					<div class="top">
						<div class="rings">
							<div class="metric"><Ring value={x.cpu_pct} label={t('servers.cpu')} /><strong>{t('servers.cpu')}</strong><span class="label mono">{t('servers.cores', { n: x.cores })}</span></div>
							<div class="metric"><Ring value={pct(x.memory)} label={t('servers.memory')} /><strong>{t('servers.memory')}</strong><span class="label mono">{bytes(x.memory.used)} / {bytes(x.memory.total)}</span></div>
							<div class="metric"><Ring value={pct(x.disk)} label={t('servers.disk')} /><strong>{t('servers.disk')}</strong><span class="label mono">{bytes(x.disk.used)} / {bytes(x.disk.total)}</span></div>
						</div>
						<dl class="facts">
							<div><dt>{t('servers.cpuTemp')}</dt><dd class:text-warning={primaryTemp(x) >= 80}>{primaryTemp(x) == null ? t('servers.notExposed') : celsius(primaryTemp(x))}</dd></div>
							<div><dt>{t('servers.load')}</dt><dd class="mono">{x.load.map((l) => number(l, 2)).join(' · ')}</dd></div>
							{#if !x.temperatures?.length}
								<div><dt>{t('servers.steal')}</dt><dd class={x.cpu_steal_pct > 10 ? 'text-warning' : 'text-ok'}>{number(x.cpu_steal_pct, 1)} % · {x.cpu_steal_pct > 10 ? t('servers.stealHigh') : t('servers.stealNormal')}</dd></div>
							{/if}
							<div><dt>{t('servers.swap')}</dt><dd>{x.swap.total ? t('servers.swapOf', { used: bytes(x.swap.used), total: bytes(x.swap.total) }) : t('servers.noSwap')}</dd></div>
							{#if x.battery}<div><dt>{t('servers.battery')}</dt><dd>{x.battery.charge_pct} % · {batteryText(x.battery)}</dd></div>{/if}
							<div><dt>{t('servers.rebootPending')}</dt><dd class={x.reboot_required ? 'text-warning' : 'text-ok'}>{x.reboot_required ? t('servers.yes') : t('servers.no')}</dd></div>
							<div><dt>{t('servers.kernel')}</dt><dd class="mono small">{x.kernel}</dd></div>
						</dl>
					</div>

					<div class="columns">
						<div class="col">
							<h3>{t('servers.containers', { n: (x.containers ?? []).length })}</h3>
							{#each x.containers ?? [] as c}
								{@const st = containerStatus(c)}
								<div class="line">
									<span class="dot {st.level}"></span>
									<span class="text"><span class="mono">{c.name}</span><span class="label">{c.vault ? `Vault ${c.vault.version}` : c.image}</span></span>
									<span class="pill {st.level}">{containerText(c)}</span>
								</div>
							{:else}
								<p class="muted none">{t('servers.noContainers')}</p>
							{/each}

							<h3 class="spaced">{t('servers.disks')}</h3>
							{#each x.disks ?? [] as d}
								<div class="line">
									<span class="mono dev">{d.device}</span>
									<span class="text label">{d.model}{d.celsius != null ? ` · ${celsius(d.celsius)}` : ''}{d.power_on_hours != null ? ` · ${number(d.power_on_hours, 0)} h` : ''}</span>
									<span class="pill {diskStatus(d).level}">{diskText(d)}</span>
								</div>
							{:else}
								<p class="muted none">{t('servers.noDisks')}</p>
							{/each}
						</div>

						<div class="col">
							<h3>{t('servers.onThisServer')}</h3>
							{#each s.findings ?? [] as f}
								<div class="finding {f.level}"><span class="dot {f.level}"></span>{findingText(f)}</div>
							{:else}
								<p class="notice ok">{t('servers.nothingToReview')}</p>
							{/each}
							<button class="btn go" onclick={() => onNavigate('actions', s.server.id)}>{t('servers.actionsFor')}</button>
						</div>
					</div>
				{:else}
					<p class="muted">{s.error ? t('servers.noResponseHelp') : t('servers.waiting')}</p>
				{/if}
			</section>
		{/key}
	{/if}
</div>

<style>
	.layout { display: flex; gap: 20px; align-items: flex-start; }
	aside { flex: 0 0 340px; display: flex; flex-direction: column; gap: 10px; position: sticky; top: 92px; }
	.count { margin: 0 4px; }
	.list { display: flex; flex-direction: column; gap: 10px; }
	.item { width: 100%; display: flex; flex-direction: column; gap: 14px; padding: 16px; border-radius: 16px; border: 1px solid var(--border); background: var(--surface); text-align: left; cursor: pointer; transition: background 0.2s, border-color 0.2s; }
	.item:hover { background: var(--row-hover); }
	.item.active { background: var(--accent-bg); border-color: var(--button); }
	.row { display: flex; align-items: center; gap: 12px; }
	.name { flex: 1 1 auto; min-width: 0; display: flex; flex-direction: column; }
	.name strong { font-size: 15px; }
	.name .label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.bars { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; }

	.detail { flex: 1 1 auto; min-width: 0; padding: 24px 26px; display: flex; flex-direction: column; gap: 22px; }
	header { display: flex; flex-wrap: wrap; align-items: center; gap: 12px 16px; }
	.title { flex: 0 1 auto; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
	.title h2 { font-size: 24px; font-weight: 600; }
	.up { margin-left: auto; }
	.top { display: flex; flex-wrap: wrap; gap: 20px; align-items: stretch; }
	.rings { display: flex; gap: 18px; padding: 18px 20px; border-radius: 16px; background: var(--surface-2); border: 1px solid var(--border-soft); }
	.metric { width: 108px; display: flex; flex-direction: column; align-items: center; gap: 4px; text-align: center; font-size: 13px; }
	.facts { flex: 1 1 320px; display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 10px; margin: 0; }
	.facts div { background: var(--surface-2); border: 1px solid var(--border-soft); border-radius: 12px; padding: 11px 14px; min-width: 0; }
	dt { color: var(--muted); font-size: 12px; }
	dd { margin: 2px 0 0; font-weight: 500; font-size: 15px; overflow-wrap: anywhere; }
	dd.small { font-size: 12px; font-weight: 400; }
	.columns { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 24px; }
	.col { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
	h3 { font-size: 16px; font-weight: 600; margin-bottom: 4px; }
	h3.spaced { margin-top: 16px; }
	.line { display: flex; align-items: center; gap: 12px; padding: 10px 0; border-bottom: 1px solid var(--border-soft); }
	.text { flex: 1 1 auto; min-width: 0; display: flex; flex-direction: column; }
	.text .label, .text.label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.dev { width: 74px; flex: 0 0 auto; }
	.none { margin: 6px 0; }
	.finding { display: flex; align-items: center; gap: 12px; padding: 11px 14px; border-radius: 12px; font-size: 14px; margin-bottom: 6px; }
	.finding.warning { background: var(--warning-fill); border: 1px solid var(--warning-border); color: var(--warning-text); }
	.finding.critical { background: var(--critical-fill); border: 1px solid var(--critical-border); color: var(--critical-text); }
	.notice { margin: 0 0 6px; }
	.go { align-self: flex-start; margin-top: 8px; }

	@media (max-width: 980px) {
		.layout { flex-direction: column; align-items: stretch; }
		aside { position: static; flex: none; }
		.list { flex-direction: row; overflow-x: auto; padding-bottom: 4px; }
		.item { flex: 0 0 260px; }
		.columns { grid-template-columns: 1fr; }
	}
	@media (max-width: 640px) {
		.detail { padding: 18px; }
		.rings { width: 100%; justify-content: space-around; padding: 14px 8px; gap: 8px; }
		.metric { width: auto; }
		.up { margin-left: 0; }
	}
</style>
