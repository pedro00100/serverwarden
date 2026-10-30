// Bridge between the server's WebAuthn options (JSON, base64url) and the browser API.

const toBuffer = (s) => {
	const b = atob(s.replace(/-/g, '+').replace(/_/g, '/') + '==='.slice((s.length + 3) % 4));
	return Uint8Array.from(b, (c) => c.charCodeAt(0)).buffer;
};
const toBase64url = (buf) =>
	btoa(String.fromCharCode(...new Uint8Array(buf))).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');

// Only return the requested extensions: some mobile browsers (NFC) add "appid"
// on their own, and the server rejects any extension it did not ask for.
const extensions = (cred, pk) => {
	const requested = pk.extensions ?? {};
	return Object.fromEntries(Object.entries(cred.getClientExtensionResults?.() ?? {}).filter(([k]) => k in requested));
};

export const supported = () => typeof window !== 'undefined' && !!window.PublicKeyCredential;

export async function create(options) {
	const pk = structuredClone(options.publicKey);
	pk.challenge = toBuffer(pk.challenge);
	pk.user.id = toBuffer(pk.user.id);
	for (const c of pk.excludeCredentials ?? []) c.id = toBuffer(c.id);
	const cred = await navigator.credentials.create({ publicKey: pk });
	return {
		id: cred.id,
		rawId: toBase64url(cred.rawId),
		type: cred.type,
		response: {
			clientDataJSON: toBase64url(cred.response.clientDataJSON),
			attestationObject: toBase64url(cred.response.attestationObject),
			transports: cred.response.getTransports?.() ?? []
		},
		clientExtensionResults: extensions(cred, pk)
	};
}

export async function sign(options) {
	const pk = structuredClone(options.publicKey);
	pk.challenge = toBuffer(pk.challenge);
	for (const c of pk.allowCredentials ?? []) c.id = toBuffer(c.id);
	const cred = await navigator.credentials.get({ publicKey: pk });
	return {
		id: cred.id,
		rawId: toBase64url(cred.rawId),
		type: cred.type,
		response: {
			clientDataJSON: toBase64url(cred.response.clientDataJSON),
			authenticatorData: toBase64url(cred.response.authenticatorData),
			signature: toBase64url(cred.response.signature),
			userHandle: cred.response.userHandle ? toBase64url(cred.response.userHandle) : null
		},
		clientExtensionResults: extensions(cred, pk)
	};
}
