import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// MOCK=1 answers /api/* with sample data so the UI can be developed without a backend.
const mockApi = {
	name: 'mock-api',
	async configureServer(server) {
		const { responses } = await import('./mock/data.js');
		server.middlewares.use((req, res, next) => {
			const path = req.url.split('?')[0];
			if (!path.startsWith('/api/')) return next();
			let raw = '';
			req.on('data', (c) => (raw += c));
			req.on('end', () => {
				let input = null;
				try { input = raw ? JSON.parse(raw) : null; } catch {}
				const out = responses[path] ? responses[path](input) : { ok: true };
				res.setHeader('Content-Type', 'application/json');
				if (out?.httpStatus) res.statusCode = out.httpStatus; // an error answer: { httpStatus, code, error }
				res.end(JSON.stringify(out));
			});
		});
	}
};

export default defineConfig({
	plugins: [
		sveltekit({
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
		}),
		process.env.MOCK === '1' && mockApi
	]
});
