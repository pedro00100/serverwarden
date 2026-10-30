import adapter from '@sveltejs/adapter-static';

/** @type {import('@sveltejs/kit').Config} */
export default {
	kit: {
		// Static UI: the Go binary embeds and serves it.
		adapter: adapter({ pages: 'build', assets: 'build', fallback: 'index.html' }),
		// Hash-based CSP for the scripts SvelteKit generates (no 'unsafe-inline' for scripts).
		// frame-ancestors and the rest go in the server's HTTP header (they don't fit in <meta>).
		csp: {
			mode: 'hash',
			directives: {
				'default-src': ['self'],
				'script-src': ['self'],
				'style-src': ['self', 'unsafe-inline'],
				'img-src': ['self', 'data:'],
				'font-src': ['self'],
				'connect-src': ['self']
			}
		}
	}
};
