<script>
	let { title = '', size = 38, background = 'var(--bg-2)' } = $props();
	let letter = $derived((title.trim()[0] ?? 'P').toUpperCase());
</script>

<div class="hex logo" style:width="{size}px" style:height="{size * 0.9}px">
	<div class="hex inner" style:background={background} style:font-size="{size * 0.37}px">{letter}</div>
</div>

<style>
	.logo { background: var(--accent); display: flex; align-items: center; justify-content: center; flex: 0 0 auto; }
	.inner { width: calc(100% - 8px); height: calc(100% - 8px); display: flex; align-items: center; justify-content: center; font-family: var(--font-display); font-weight: 700; color: var(--accent); }
</style>
