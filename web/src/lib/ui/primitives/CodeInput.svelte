<script>
	// A 6-digit one-time code: one real input (so autofill, paste and screen
	// readers work) drawn as six boxes. onComplete fires when the sixth digit lands.
	let { value = $bindable(''), label, id = 'code', autofocus = false, disabled = false, onComplete } = $props();

	let focused = $state(false);
	let input;

	// Focus on show, and again whenever the code is cleared (e.g. after a wrong one).
	$effect(() => {
		if (autofocus && value === '' && !disabled) input?.focus();
	});

	function oninput() {
		value = value.replace(/\D/g, '').slice(0, 6);
		if (value.length === 6) onComplete?.();
	}
</script>

<div class="code" class:focused class:disabled>
	<label for={id} class="sr-only">{label}</label>
	<input {id} bind:this={input} bind:value {oninput} {disabled} onfocus={() => (focused = true)} onblur={() => (focused = false)}
		inputmode="numeric" autocomplete="one-time-code" pattern="[0-9]*" maxlength="6" spellcheck="false" />
	<div class="cells" aria-hidden="true">
		{#each { length: 6 } as _, i}
			<span class="cell" class:filled={value[i]} class:current={focused && i === Math.min(value.length, 5)}>{value[i] ?? ''}</span>
			{#if i === 2}<span class="gap"></span>{/if}
		{/each}
	</div>
</div>

<style>
	.code { position: relative; width: 100%; max-width: 380px; }
	input { position: absolute; inset: 0; width: 100%; height: 100%; opacity: 0; border: 0; padding: 0; cursor: text; font-size: 16px; }
	.cells { display: grid; grid-template-columns: repeat(3, 1fr) 14px repeat(3, 1fr); gap: 8px; pointer-events: none; }
	.cell { height: 56px; display: flex; align-items: center; justify-content: center; border-radius: 12px; border: 1px solid var(--border); background: var(--surface-2); font: 600 24px var(--font-mono); color: var(--text); transition: border-color 0.15s, box-shadow 0.15s; }
	.cell.filled { border-color: var(--border-strong); }
	.cell.current { border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-bg); }
	.cell.current:not(.filled)::after { content: ''; width: 2px; height: 26px; background: var(--accent); animation: blink 1s steps(1) infinite; }
	.gap { align-self: center; justify-self: center; width: 8px; height: 2px; border-radius: 2px; background: var(--border-strong); }
	.disabled { opacity: 0.6; }
	@keyframes blink { 50% { opacity: 0; } }
	@media (prefers-reduced-motion: reduce) { .cell.current:not(.filled)::after { animation: none; } }
	@media (max-width: 380px) { .cell { height: 48px; font-size: 20px; } .cells { gap: 6px; } }
</style>
