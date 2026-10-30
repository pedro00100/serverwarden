// Hash routing: #<section>[/<sub>] (sub picks a server or a settings page),
// plus one-time links #enroll=<code> (register a passkey), #setup=<code>
// (first run) and #recover=<code> (recovery mode).
export function createRouter(sections) {
	let current = $state(sections[0]);
	let sub = $state('');
	let enrollCode = $state('');
	let recoverCode = $state('');
	let setupCode = $state('');

	function read() {
		const h = location.hash.slice(1);
		if (h.startsWith('enroll=')) {
			enrollCode = h.slice('enroll='.length);
			return;
		}
		if (h.startsWith('setup=')) {
			setupCode = h.slice('setup='.length);
			history.replaceState(null, '', location.pathname);
			return;
		}
		if (h.startsWith('recover=')) {
			recoverCode = h.slice('recover='.length);
			history.replaceState(null, '', location.pathname); // keep the code out of history
			return;
		}
		const [section, rest = ''] = h.split('/');
		current = sections.includes(section) ? section : sections[0];
		sub = decodeURIComponent(rest);
	}

	return {
		get current() { return current; },
		get sub() { return sub; },
		get enrollCode() { return enrollCode; },
		get recoverCode() { return recoverCode; },
		get setupCode() { return setupCode; },
		go(id, to = '') {
			const hash = to ? `#${id}/${encodeURIComponent(to)}` : `#${id}`;
			if (hash !== location.hash) history.pushState(null, '', hash);
			const moved = id !== current || to !== sub;
			current = id;
			sub = to;
			if (moved) window.scrollTo({ top: 0, behavior: 'smooth' });
		},
		clearEnrollment() {
			enrollCode = '';
			history.replaceState(null, '', location.pathname);
		},
		start() {
			read();
			addEventListener('popstate', read);
			return () => removeEventListener('popstate', read);
		}
	};
}
