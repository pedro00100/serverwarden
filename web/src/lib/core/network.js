// IPv4 network parsing for the allowed-networks editor. IPv6 is accepted as
// typed and left for the server to validate.

const toInt = (o) => ((o[0] << 24) >>> 0) + (o[1] << 16) + (o[2] << 8) + o[3];
const toText = (n) => [n >>> 24, (n >>> 16) & 255, (n >>> 8) & 255, n & 255].join('.');
const maskOf = (bits) => (bits === 0 ? 0 : (0xffffffff << (32 - bits)) >>> 0);

function parse4(text) {
	const m = text.match(/^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})(?:\/(\d{1,2}))?$/);
	if (!m) return null;
	const o = m.slice(1, 5).map(Number);
	const bits = m[5] === undefined ? 32 : Number(m[5]);
	if (o.some((x) => x > 255) || bits > 32) return null;
	const start = (toInt(o) & maskOf(bits)) >>> 0;
	return { bits, start, end: (start | (~maskOf(bits) >>> 0)) >>> 0, explicit: m[5] !== undefined };
}

const PRIVATE = ['10.0.0.0/8', '172.16.0.0/12', '192.168.0.0/16', '127.0.0.0/8', '100.64.0.0/10', '169.254.0.0/16'].map(parse4);
const inside = (n, outer) => n.bits >= outer.bits && ((n.start & maskOf(outer.bits)) >>> 0) === outer.start;

export function analyze(input, existing = []) {
	const text = (input ?? '').trim();
	if (!text) return { kind: 'empty' };
	if (text.includes(':')) {
		return /^[0-9a-f:.]+(\/\d{1,3})?$/i.test(text)
			? { kind: 'v6', cidr: text.includes('/') ? text : text + '/128', dup: existing.includes(text) }
			: { kind: 'invalid' };
	}
	const n = parse4(text);
	if (!n) return { kind: 'invalid' };
	const cidr = `${toText(n.start)}/${n.bits}`;
	return {
		kind: 'v4',
		cidr,
		bits: n.bits,
		from: toText(n.start),
		to: toText(n.end),
		count: 2 ** (32 - n.bits),
		normalized: cidr !== (n.explicit ? text : text + '/32'),
		dup: existing.includes(cidr),
		public: !PRIVATE.some((p) => inside(n, p)),
		octets: [0, 1, 2, 3].map((i) => {
			const fixed = Math.max(0, Math.min(8, n.bits - 8 * i));
			const lo = (n.start >>> (24 - 8 * i)) & 255;
			const hi = (n.end >>> (24 - 8 * i)) & 255;
			return fixed === 8 ? { kind: 'fixed', text: String(lo) } : fixed === 0 ? { kind: 'any', text: '0–255' } : { kind: 'range', text: `${lo}–${hi}` };
		})
	};
}

// Whether the network includes the address; null when it cannot tell (IPv6).
export function covers(cidr, ip) {
	const n = parse4(cidr);
	const a = parse4(ip ?? '');
	if (!n || !a) return null;
	return inside({ bits: 32, start: a.start }, n);
}
