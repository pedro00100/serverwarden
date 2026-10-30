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
	plugins: [sveltekit(), process.env.MOCK === '1' && mockApi]
});
