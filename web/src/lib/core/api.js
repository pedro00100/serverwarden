import { CSRF_HEADER } from './brand.js';

export class ApiError extends Error {
	constructor(status, message, code) {
		super(message);
		this.status = status;
		this.code = code;
	}
}

export class Unauthorized extends ApiError {
	constructor(message, code) {
		super(401, message, code);
	}
}

async function handle(r) {
	const body = await r.json().catch(() => ({}));
	if (r.status === 401) throw new Unauthorized(body.error, body.code);
	if (!r.ok) throw new ApiError(r.status, body.error || `HTTP ${r.status}`, body.code);
	return body;
}

export const get = (path) => fetch(path, { cache: 'no-store', credentials: 'same-origin' }).then(handle);

// Every state-changing request carries the CSRF header; the server also checks
// Origin and the cookies are SameSite=Strict.
const send = (method) => (path, body) =>
	fetch(path, {
		method,
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json', [CSRF_HEADER]: '1' },
		body: JSON.stringify(body ?? {})
	}).then(handle);

export const post = send('POST');
export const put = send('PUT');
export const del = send('DELETE');
