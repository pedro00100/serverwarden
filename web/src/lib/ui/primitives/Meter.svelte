<script>
	import { usageTone } from '$lib/core/derive.js';

	let { label, value = 0, detail = '' } = $props();
	let v = $derived(Math.min(100, Math.max(0, value)));
</script>

<span class="meter">
	<span class="head"><span>{label}</span><span class="mono">{Math.round(v)}%</span></span>
	<span class="track" role="img" aria-label="{label}: {Math.round(v)} %"><span class="fill" style:width="{v}%" style:background={usageTone(v)}></span></span>
	{#if detail}<span class="detail mono">{detail}</span>{/if}
</span>

<style>
	.meter { display: flex; flex-direction: column; gap: 5px; min-width: 0; }
	.head { display: flex; justify-content: space-between; gap: 6px; font-size: 11px; color: var(--muted); }
	.head .mono { color: var(--text-2); }
	.track { display: block; height: 4px; border-radius: 4px; background: var(--track); overflow: hidden; }
	.fill { display: block; height: 100%; border-radius: 4px; transform-origin: left; animation: grow 0.8s cubic-bezier(0.2, 0.7, 0.2, 1) both; transition: width 0.6s cubic-bezier(0.2, 0.7, 0.2, 1); }
	.detail { font-size: 12px; color: var(--muted); }
	@keyframes grow { from { transform: scaleX(0); } }
</style>
