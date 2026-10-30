<script>
	import { usageTone } from '$lib/core/derive.js';

	let { value = 0, size = 78, stroke = 9, label = '', color = null, center = null } = $props();
	const C = 2 * Math.PI * 42;
	let offset = $derived(C - (C * Math.min(100, Math.max(0, value))) / 100);
	let tone = $derived(color ?? usageTone(value));
</script>

<div class="ring" style:width="{size}px" style:height="{size}px">
	<svg role="img" aria-label="{label}: {Math.round(value)} %" viewBox="0 0 100 100" width={size} height={size}>
		<circle cx="50" cy="50" r="42" fill="none" stroke="var(--track)" stroke-width={stroke} />
		<circle class="arc" cx="50" cy="50" r="42" fill="none" stroke={tone} stroke-width={stroke}
			stroke-linecap="round" stroke-dasharray={C} stroke-dashoffset={offset} transform="rotate(-90 50 50)"
			style:--c={C} />
	</svg>
	<span class="center">{center ?? `${Math.round(value)}%`}</span>
</div>

<style>
	.ring { position: relative; flex: 0 0 auto; }
	.arc { transition: stroke-dashoffset 0.8s cubic-bezier(0.2, 0.7, 0.2, 1), stroke 0.3s; animation: fill 1.4s cubic-bezier(0.2, 0.7, 0.2, 1) both; }
	@keyframes fill { from { stroke-dashoffset: var(--c); } }
	.center { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; font: 600 16px var(--font-display); }
</style>
