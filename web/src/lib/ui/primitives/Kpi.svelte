<script>
	// tone: ok | warning | critical | muted
	let { label, value, note, tone = 'muted', delay = 0 } = $props();
</script>

<div class="card rise kpi" style:animation-delay="{delay}s">
	<span class="label">{label}</span>
	<span class="value">{value}</span>
	<span class="note {tone}">{note}</span>
</div>

<style>
	.kpi { padding: 16px 18px; display: flex; flex-direction: column; gap: 3px; }
	.value { font: 700 26px/1.15 var(--font-display); }
	.note { font-size: 12px; font-weight: 500; color: var(--muted); }
	.note.ok { color: var(--ok-text); }
	.note.warning { color: var(--warning-text); }
	.note.critical { color: var(--critical-text); }
</style>
