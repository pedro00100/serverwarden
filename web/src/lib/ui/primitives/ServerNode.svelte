<script>
	let { text, level = 'ok', size = 74, highlight = false } = $props();
</script>

<div class="node {level}" style:width="{size}px" style:height="{size * 0.87}px">
	{#if highlight}<div class="hex glow aura"></div>{/if}
	<div class="hex edge">
		<div class="hex inner">{text}</div>
	</div>
</div>

<style>
	.node { position: relative; flex: 0 0 auto; --tone: var(--ok); --fill: var(--ok-fill); }
	.node.warning { --tone: var(--warning); --fill: var(--warning-fill); }
	.node.critical { --tone: var(--critical); --fill: var(--critical-fill); }
	.aura { position: absolute; inset: -8px; opacity: 0.25; background: var(--tone); }
	.edge { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; background: var(--tone); }
	.inner { width: calc(100% - 6px); height: calc(100% - 6px); display: flex; align-items: center; justify-content: center; font: 600 12px var(--font-mono); background: var(--fill); color: var(--tone); }
</style>
