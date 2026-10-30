<script>
	import mesh from '$lib/assets/mesh.svg';

	// full: bright and drifting; shade: darkens the left side behind large text.
	let { full = false, shade = false } = $props();
</script>

<img class="mesh" class:full src={mesh} alt="" aria-hidden="true" />
{#if shade}<div class="shade" aria-hidden="true"></div>{/if}

<style>
	.mesh { position: fixed; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0.3; pointer-events: none; }
	.mesh.full { opacity: 1; animation: drift 16s ease-in-out infinite; }
	@keyframes drift { 50% { translate: -16px 10px; } }
	.shade { position: fixed; inset: 0; pointer-events: none; background: radial-gradient(ellipse at 30% 50%, rgba(10, 15, 21, 0.96) 0%, rgba(10, 15, 21, 0.72) 45%, rgba(10, 15, 21, 0.12) 80%); }
</style>
