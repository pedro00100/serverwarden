// Browser half of install-e2e.sh: drives the real panel (no mock) through the
// first-run wizard, adds the Debian server, hands the generated install script
// to the host, then checks the connection, live data and a panel command.
import puppeteer from 'puppeteer-core';
import { existsSync, writeFileSync } from 'node:fs';
import { createHmac } from 'node:crypto';

const base = 'https://e2e-panel:8443/';
const PASSWORD = 'a long enough e2e password';
const browser = await puppeteer.launch({ executablePath: '/usr/bin/chromium-browser', args: ['--no-sandbox', '--disable-dev-shm-usage', '--ignore-certificate-errors'] });
const page = await browser.newPage();
await page.setViewport({ width: 1366, height: 900 });
const errors = [];
page.on('pageerror', (e) => errors.push('pageerror: ' + e.message));

let failed = false;
const result = (ok, what) => { console.log((ok ? 'PASS ' : 'FAIL ') + what); if (!ok) failed = true; return ok; };
const settle = (ms = 1200) => new Promise((r) => setTimeout(r, ms));
const shot = (name) => page.screenshot({ path: `/shots/e2e-${name}.png`, fullPage: true });
const click = (sel) => page.evaluate((s) => document.querySelector(s)?.click(), sel);
const clickText = (sel, re) => page.evaluate((s, r) => [...document.querySelectorAll(s)].find((e) => new RegExp(r, 'i').test(e.textContent))?.click(), sel, re);
const until = async (seconds, fn) => {
	for (let i = 0; i < seconds; i += 2) {
		if (await fn().catch(() => false)) return true;
		await settle(2000);
	}
	return false;
};
const finish = async () => {
	console.log('JS errors:', errors.length ? '\n' + [...new Set(errors)].join('\n') : 'none');
	await browser.close();
	process.exit(failed || errors.length ? 1 : 0);
};

// 1. First-run wizard
await page.goto(base + '#setup=' + process.env.CODE, { waitUntil: 'networkidle0' });
await until(30, () => page.$eval('button.cta', (b) => !b.disabled));
await click('button.cta');
await settle();
await click('form.steps button[type=submit]');
await settle();
const origin = await page.$eval('form.steps input', (e) => e.value);
result(origin === 'https://e2e-panel:8443', `wizard suggests the address the browser used (${origin})`);
await click('form.steps button[type=submit]');
await settle();
for (const p of await page.$$('input[type=password]')) await p.type(PASSWORD);
await click('form.steps button[type=submit]');
await settle();
await click('form.steps button.cta');
if (!result(await until(30, () => page.$('.done h1')), 'wizard creates the panel')) await finish();
await shot('wizard-done');

// 2. Sign in once the panel restarted with the new settings
const loginShown = await until(60, async () => {
	await page.goto(base, { waitUntil: 'networkidle0' });
	return !!(await page.$('#password'));
});
result(loginShown, 'panel restarts into the sign-in page');
await page.type('#username', 'admin');
await page.type('#password', PASSWORD);
await click('form.form button[type=submit]');
result(await until(20, () => page.$('nav.tabs')), 'signed in with the password from the wizard');

// 3. Settings › Servers: add the server and generate its install script
await page.goto(base + '#settings/servers', { waitUntil: 'networkidle0' });
await settle();
await page.type('form.bar input[type=password]', PASSWORD);
await click('form.bar button[type=submit]');
await settle();
await clickText('button.btn.primary', 'add server|añadir servidor');
await settle();
const inputs = await page.$$('.editor input');
await inputs[0].type('Debian test');
await inputs[5].type('e2e-server');
await click('.editor form button.btn.primary');
const fp = await until(30, () => page.$('.fingerprint')) ? await page.$eval('.fingerprint', (e) => e.textContent) : '';
result(fp.includes(process.env.FP), 'fingerprint shown in the UI matches the real server');
await shot('fingerprint');
await click('.editor input[type=checkbox]');
await page.evaluate(() => [...document.querySelectorAll('.editor button.btn.primary')].at(-1).click());
result(await until(20, () => page.$('.panel form')), 'server saved, install form shown');
const panelURL = await page.$eval('.panel form input', (e) => e.value);
result(panelURL === 'https://e2e-panel:8443', `install form offers the panel address (${panelURL})`);
await click('.panel form input[type=checkbox]');
await click('.panel form button.btn.primary');
await until(20, () => page.$('.panel pre'));
const script = await page.$eval('.panel pre', (e) => e.textContent).catch(() => '');
if (!result(script.startsWith('#!/bin/sh') && script.includes('https://e2e-panel:8443/install/'), 'UI generates the install script')) await finish();
await shot('script');
writeFileSync('/out/install.sh', script);

// 4. The host runs it on the server
result(await until(300, async () => existsSync('/out/installed')), 'host ran the script');

// 5. Test connection, then live data
await page.evaluate(() => [...document.querySelectorAll('.panel .buttons .btn')].at(-1).click());
result(await until(30, () => page.$('.panel .text-ok')), 'connection test from the UI succeeds');
await shot('tested');
const live = await until(90, async () => {
	await page.goto(base + '#servers', { waitUntil: 'networkidle0' });
	await settle();
	return page.$eval('.detail', (e) => e.textContent.includes('Debian GNU/Linux 13'));
});
result(live, 'Servers shows live data read by the agent (Debian GNU/Linux 13)');
await shot('servers');

// 6. A command of your own, run on the server from Actions
await page.goto(base + '#settings/commands', { waitUntil: 'networkidle0' });
await settle();
await clickText('form button', 'add command|añadir comando');
await settle();
const row = (await page.$$('.row')).at(-1);
await (await row.$('input')).type('Kernel');
await (await row.$('textarea')).type('uname -sr');
await click('form button[type=submit]');
result(await until(10, () => page.$('.notice.ok')), 'command saved');
const ran = await until(90, async () => {
	await page.goto(base + '#actions', { waitUntil: 'networkidle0' });
	await settle();
	await clickText('.servers button', 'debian test');
	await settle();
	const has = await page.evaluate(() => {
		const card = [...document.querySelectorAll('.action')].find((c) => c.querySelector('.yours'));
		card?.querySelector('.run')?.click();
		return !!card;
	});
	if (!has) return false;
	if (await page.$eval('dialog', (d) => d.open).catch(() => false)) {
		await page.type('dialog input[type=password]', PASSWORD);
		await click('dialog button[type=submit]');
	}
	return until(30, () => page.$eval('.action .output', (e) => e.textContent.includes('Linux')));
});
result(ran, 'your command ran on the server and its output came back');
await shot('command');

// 7. Two-step verification, with codes computed here from the key the page
// shows (RFC 6238, independent of the panel's own code).
const totp = (secret, offset = 0) => {
	const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
	let bits = '';
	for (const c of secret) bits += alphabet.indexOf(c).toString(2).padStart(5, '0');
	const key = Buffer.from(bits.match(/.{8}/g).map((b) => parseInt(b, 2)));
	const step = Buffer.alloc(8);
	step.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 1000 / 30) + offset));
	const h = createHmac('sha1', key).update(step).digest();
	const o = h[19] & 15;
	return String((h.readUInt32BE(o) & 0x7fffffff) % 1_000_000).padStart(6, '0');
};
const signOut = async () => {
	await page.deleteCookie(...(await page.cookies()));
	await page.goto(base, { waitUntil: 'networkidle0' });
	await settle();
};
const signInPassword = async () => {
	await page.type('#username', 'admin');
	await page.type('#password', PASSWORD);
	await click('form.form button[type=submit]');
	return until(10, () => page.$('#code'));
};
await page.goto(base + '#settings/access', { waitUntil: 'networkidle0' });
await settle();
if (await page.$('form.bar input[type=password]')) {
	await page.type('form.bar input[type=password]', PASSWORD);
	await click('form.bar button[type=submit]');
	await settle();
}
await clickText('section button.btn.primary', 'two-step|dos pasos');
await settle();
const secret = (await page.$eval('.secret code', (e) => e.textContent).catch(() => '')).replace(/ /g, '');
result(secret.length === 32 && !!(await page.$('.qr svg')), 'the panel shows its own QR code and the key');
writeFileSync('/out/totp-secret', secret);
await clickText('section button.btn.primary', '^(next|siguiente)$');
await settle();
await page.type('.confirm input[type=password]', PASSWORD);
await page.type('#totp-confirm', totp(secret));
const recovery = await until(10, async () => (await page.$$('.codes li')).length === 10);
result(recovery, 'enabling shows 10 recovery codes once');
const codes = await page.$$eval('.codes code', (els) => els.map((e) => e.textContent));
await shot('totp-codes');
await page.click('section label.check input');
await clickText('section button.btn.primary', 'done|terminar');
await settle();

await signOut();
result(await signInPassword(), 'with TOTP on, the password alone only leads to the code step');
await page.type('#code', '000000');
await settle();
result(!(await page.$('nav.tabs')) && !!(await page.$('.message.error')), 'a wrong code is refused');
await page.type('#code', totp(secret, 1)); // the enabling code used the current step: take the next one
result(await until(10, () => page.$('nav.tabs')), 'signed in with the password and a code');

await signOut();
await signInPassword();
await clickText('button.link', 'recovery|recuperación');
await page.type('#recovery', codes[0]);
await click('form.form button[type=submit]');
result(await until(10, () => page.$('nav.tabs')), 'signed in with a recovery code');
await finish();
