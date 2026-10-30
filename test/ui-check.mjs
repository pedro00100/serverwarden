// Opens every section of `make dev` in headless Chromium, unlocks the settings
// and visits each tab. Fails on any JavaScript error; screenshots go to test/shots.
import puppeteer from 'puppeteer-core';

const base = process.env.BASE || 'http://127.0.0.1:5173/';
const browser = await puppeteer.launch({ executablePath: '/usr/bin/chromium-browser', args: ['--no-sandbox', '--disable-dev-shm-usage'] });
const page = await browser.newPage();
await page.setViewport({ width: 1366, height: 900 });
const errors = [];
page.on('pageerror', (e) => errors.push('pageerror: ' + e.message));
let expected = null; // a console error a step provokes on purpose
page.on('console', (m) => {
	if ((m.type() === 'error' || m.type() === 'warning') && !expected?.test(m.text())) errors.push(m.type() + ': ' + m.text());
});

const shot = (name) => page.screenshot({ path: `/shots/${name}.png`, fullPage: true });
const settle = () => new Promise((r) => setTimeout(r, 1200));

const click = (sel) => page.evaluate((s) => document.querySelector(s)?.click(), sel);
const clickText = (sel, re) => page.evaluate((s, r) => [...document.querySelectorAll(s)].find((e) => new RegExp(r, 'i').test(e.textContent))?.click(), sel, re);

await page.goto(base, { waitUntil: 'networkidle0' });
await page.evaluate(() => fetch('/api/mock/reset', { method: 'POST' }));

for (const s of ['overview', 'servers', 'security', 'vulnerabilities', 'actions', 'settings']) {
	await page.goto(base + '#' + s, { waitUntil: 'networkidle0' });
	await settle();
	const tab = await page.$eval('nav.tabs [aria-current=page]', (e) => e.textContent.trim()).catch(() => '(none)');
	console.log(`section ${s}: active tab="${tab}"`);
	await shot(s);
}

// Tabs switch sections without reloading.
await page.goto(base + '#overview', { waitUntil: 'networkidle0' });
await settle();
await clickText('nav.tabs button', 'secur|segur');
await settle();
console.log('tab click ->', await page.evaluate(() => location.hash));
if (!(await page.evaluate(() => location.hash)).startsWith('#security')) errors.push('tab click did not navigate');

// Servers: pick another server from the list.
await page.goto(base + '#servers', { waitUntil: 'networkidle0' });
await settle();
const items = await page.$$('aside .item');
await items[0].click();
await settle();
const title = await page.$eval('#srv-title', (e) => e.textContent).catch(() => '');
console.log('servers: picked', title, '->', await page.evaluate(() => location.hash));
if (!title) errors.push('server detail not shown');
await shot('servers-picked');

// Vulnerabilities: open another target.
await page.goto(base + '#vulnerabilities', { waitUntil: 'networkidle0' });
await settle();
const tiles = await page.$$('button.tile');
console.log('vulnerabilities: tiles', tiles.length);
if (tiles.length > 1) { await tiles[1].click(); await settle(); }
await shot('vulnerabilities-picked');

// Actions: running asks for the password in a dialog, then runs.
await page.goto(base + '#actions', { waitUntil: 'networkidle0' });
await settle();
await click('.action .run');
await settle();
const dialogOpen = await page.$eval('dialog', (d) => d.open).catch(() => false);
console.log('actions: unlock dialog open:', dialogOpen);
await shot('actions-unlock');
if (dialogOpen) {
	await page.type('dialog input[type=password]', 'whatever');
	await click('dialog button[type=submit]');
	await settle();
}
const state = await page.$eval('.action .state', (e) => e.textContent).catch(() => '');
console.log('actions: state after run:', state);
if (!state) errors.push('action did not run after unlocking');
await shot('actions-done');

// Your own command on home-server: runs and shows its output.
await page.goto(base + '#actions/home', { waitUntil: 'networkidle0' });
await settle();
await page.evaluate(() => [...document.querySelectorAll('.action')].find((c) => c.querySelector('.yours'))?.querySelector('.run')?.click());
await settle();
const output = await page.$eval('.action .output', (e) => e.textContent).catch(() => '');
console.log('actions: your command output shown:', !!output);
if (!output) errors.push('command output not shown');
await shot('actions-command');

// Security: the CrowdSec histogram.
await page.goto(base + '#security', { waitUntil: 'networkidle0' });
await settle();
const bars = await page.$$eval('.histogram .bar', (els) => els.length);
console.log('security: histogram bars', bars);
if (bars !== 24) errors.push('alert histogram missing');

// Settings: the cards, then each page on its own.
await page.goto(base + '#settings', { waitUntil: 'networkidle0' });
await settle();
const cards = await page.$$eval('.cards .tile strong', (els) => els.map((e) => e.textContent));
console.log('settings cards:', cards.join(' | '));
await shot('settings-cards');
await click('.cards .tile');
await settle();
const pw = await page.$('form.bar input[type=password]');
if (pw) {
	await pw.type('whatever');
	await page.click('form.bar button[type=submit]');
	await settle();
}
const unlocked = !(await page.$('form.bar input[type=password]'));
console.log('unlocked:', unlocked);
if (!unlocked) errors.push('could not unlock settings');
const pages = await page.$$eval('nav.jump button', (els) => els.map((e) => e.textContent));
for (let i = 0; i < pages.length; i++) {
	await page.evaluate((n) => document.querySelectorAll('nav.jump button')[n].click(), i);
	await settle();
	const inputs = await page.$$eval('main input, main select, main textarea', (els) => els.length);
	const disabled = await page.$$eval('main fieldset[disabled]', (els) => els.length);
	console.log(`settings ${pages[i]}: ${inputs} inputs, ${disabled} disabled fieldsets, hash ${await page.evaluate(() => location.hash)}`);
	await shot('settings-' + i);
}
// Add a command and save it.
await page.goto(base + '#settings/commands', { waitUntil: 'networkidle0' });
await settle();
await clickText('form button', 'add command|añadir comando');
await settle();
const rows = await page.$$('.row');
const last = rows[rows.length - 1];
await (await last.$('input')).type('Uptime');
await (await last.$('textarea')).type('uptime');
await click('form button[type=submit]');
await settle();
const saved = !!(await page.$('.notice.ok'));
console.log('settings: command saved:', saved);
if (!saved) errors.push('command not saved');
await shot('settings-commands');

await page.goto(base + '#settings/scanner', { waitUntil: 'networkidle0' });
await page.goto(base + '#settings/manual', { waitUntil: 'networkidle0' });
await page.goBack();
await settle();
console.log('settings: back goes to', await page.evaluate(() => location.hash));
await click('.focusbar .back');
await settle();
if (!(await page.$('.cards'))) errors.push('back to all settings failed');

// Vulnerabilities: an unrated finding is labelled and spaced, skipped targets say why.
await page.goto(base + '#vulnerabilities', { waitUntil: 'networkidle0' });
await settle();
await page.evaluate(() => [...document.querySelectorAll('.tile')].find((t) => t.textContent.includes('local/panel'))?.click());
await settle();
const unrated = await page.$eval('.detail li', (e) => e.innerText).catch(() => '');
const reasons = await page.$$eval('.tile.skipped .reason', (els) => els.map((e) => e.textContent));
console.log('cve: unrated row', JSON.stringify(unrated), '| skip reasons', reasons.length);
if (unrated.includes('cve.') || !unrated.includes(' · ') || reasons.length !== 2 || reasons.some((r) => r.includes('cve.'))) errors.push('vulnerability details or skip reasons are not rendered');
await shot('cve-detail');

// Servers: add one, confirm its fingerprint, get the install script and test it.
await page.goto(base + '#settings/servers', { waitUntil: 'networkidle0' });
await settle();
await page.evaluate(() => [...document.querySelectorAll('button.btn.primary')].find((b) => /server|servidor/i.test(b.textContent))?.click());
await settle();
await page.type('.editor input:not([type=number])', 'Gateway');
const inputs = await page.$$('.editor input');
await inputs[5].type("192.168.1.17");
await page.evaluate(() => document.querySelector('.editor form button.btn.primary').click());
await settle();
const fp = await page.$eval('.fingerprint', (e) => e.textContent).catch(() => null);
console.log('server flow: fingerprint shown:', !!fp);
if (!fp) errors.push('fingerprint step not shown');
await shot('server-verify');
await page.click('.editor input[type=checkbox]');
await page.evaluate(() => [...document.querySelectorAll('.editor button.btn.primary')].at(-1).click());
await settle();
const installShown = !!(await page.$('.panel form'));
console.log('server flow: install panel shown:', installShown);
if (!installShown) errors.push('install panel not shown after saving');
await page.evaluate(() => document.querySelector('.panel form button.btn.primary').click());
await settle();
const script = await page.$eval('.panel pre', (e) => e.textContent).catch(() => '');
console.log('server flow: script shown:', script.startsWith('#!/bin/sh'));
if (!script.startsWith('#!/bin/sh')) errors.push('install script not shown');
await page.evaluate(() => [...document.querySelectorAll('.panel .buttons .btn')].at(-1).click());
await settle();
console.log('server flow: test ok:', !!(await page.$('.panel .text-ok')));
await shot('server-install');

// A server from an imported settings file has no pinned host key: verify it from the list.
await page.goto(base + '#settings/servers', { waitUntil: 'networkidle0' });
await settle();
const unpinnedBefore = await page.$$eval('.server .pill.warning', (els) => els.length);
await shot('server-unpinned');
await page.evaluate(() => document.querySelector('.server .buttons .btn.primary')?.click());
await settle();
const verifyFp = await page.$eval('.fingerprint', (e) => e.textContent).catch(() => null);
await page.click('.editor input[type=checkbox]');
await page.evaluate(() => [...document.querySelectorAll('.editor button.btn.primary')].at(-1).click());
await settle();
const unpinnedAfter = await page.$$eval('.server .pill.warning', (els) => els.length);
console.log(`unpinned flow: before ${unpinnedBefore}, fingerprint shown ${!!verifyFp}, after ${unpinnedAfter}`);
if (unpinnedBefore !== 1 || !verifyFp || unpinnedAfter !== 0) errors.push('verifying an unpinned server failed');

// Two-step verification: turn it on from Settings › Access, then sign in with a code.
const mockPost = (path, body) => page.evaluate((p, b) => fetch(p, { method: 'POST', body: JSON.stringify(b) }), path, body);
await page.goto(base + '#settings/access', { waitUntil: 'networkidle0' });
await settle();
await clickText('section button.btn.primary', 'two-step|dos pasos');
await settle();
const qr = await page.$eval('.qr svg', (e) => e.getAttribute('viewBox')).catch(() => null);
const secretShown = await page.$eval('.secret code', (e) => e.textContent.replace(/ /g, '').length).catch(() => 0);
await shot('totp-scan');
await clickText('section button.btn.primary', '^(next|siguiente)$');
await settle();
await page.type('.confirm input[type=password]', 'whatever password');
await page.type('#totp-confirm', '123456');
await settle();
const recovery = await page.$$eval('.codes li', (els) => els.length);
const finishLocked = await page.$eval('section .buttons:last-child .btn.primary', (b) => b.disabled).catch(() => null);
await shot('totp-codes');
await page.click('section label.check input');
await clickText('section button.btn.primary', 'done|terminar');
await settle();
const totpOn = await page.$eval('#totp-title + .pill.ok', (e) => !!e).catch(() => false);
await shot('totp-on');
console.log(`totp: qr ${qr}, secret ${secretShown} chars, ${recovery} recovery codes, finish locked until saved ${finishLocked}, on ${totpOn}`);
if (!qr || secretShown !== 32 || recovery !== 10 || finishLocked !== true || !totpOn) errors.push('enabling two-step verification failed');

await mockPost('/api/mock/screen', { screen: 'login' });
await page.goto(base, { waitUntil: 'networkidle0' });
await page.reload({ waitUntil: 'networkidle0' });
await settle();
await page.type('#username', 'admin');
await page.type('#password', 'whatever password');
await click('form.form button.primary');
await settle();
const codeStep = !!(await page.$('#code'));
expected = /status of 401/;
await page.type('#code', '000000');
await settle();
expected = null;
const wrong = await page.$eval('.message.error', (e) => e.textContent).catch(() => '');
await shot('login-code-wrong');
await page.type('#code', '123456');
await settle();
const signedIn = !!(await page.$('nav.tabs'));
console.log(`login: code step ${codeStep}, wrong code says "${wrong}", signed in ${signedIn}`);
if (!codeStep || !wrong || wrong.includes('errors.') || !signedIn) errors.push('signing in with a code failed');
await mockPost('/api/mock/screen', { screen: 'login' });
await page.setViewport({ width: 390, height: 844 });
await page.reload({ waitUntil: 'networkidle0' });
await settle();
await page.type('#username', 'admin');
await page.type('#password', 'whatever password');
await click('form.form button.primary');
await settle();
await shot('login-code-mobile');
await page.setViewport({ width: 1366, height: 900 });
await mockPost('/api/mock/screen', { screen: 'panel' });

// First-run wizard
await page.evaluate(() => fetch('/api/mock/screen', { method: 'POST', body: JSON.stringify({ screen: 'setup' }) }));
await page.goto(base, { waitUntil: 'networkidle0' });
await page.reload({ waitUntil: 'networkidle0' });
await settle();
await shot('setup-welcome');
await click('button.cta');
await settle();
await shot('setup-0');
await click('form.steps button[type=submit]');
await settle();

// Step 2: the client's own network cannot be removed, another one can be added.
const nets = () => page.$$eval('.net', (els) => els.length);
const before = await nets();
await clickText('.net button', '');
await settle();
const lockout = !!(await page.$('.notice.warning'));
console.log('setup: lockout warning on removing own network:', lockout, '· networks', before, '->', await nets());
if (!lockout) errors.push('removing the own network was not prevented');
await click('.add-net');
await settle();
await page.type('dialog input', '10.8.0.0/24');
await settle();
await shot('setup-network');
const octets = await page.$$eval('dialog .oct', (els) => els.map((e) => e.textContent.trim()).join(' | '));
console.log('setup: octets', octets);
await click('dialog button[type=submit]');
await settle();
console.log('setup: networks after adding', await nets());
if ((await nets()) !== before + 1) errors.push('network was not added');
await shot('setup-1');
await click('form.steps button[type=submit]');
await settle();

const pws = await page.$$('input[type=password]');
for (const p of pws) await p.type('a long enough password');
await settle();
await shot('setup-2');
await click('form.steps button[type=submit]');
await settle();
await shot('setup-3');
await click('form.steps button.cta');
await settle();
const finished = !!(await page.$('.done h1'));
console.log('setup flow: finished:', finished);
if (!finished) errors.push('setup wizard did not finish');
await shot('setup-done');

await page.setViewport({ width: 390, height: 844 });
await page.reload({ waitUntil: 'networkidle0' });
await settle();
await shot('setup-welcome-mobile');

await page.setViewport({ width: 1366, height: 900 });
await page.evaluate(() => fetch('/api/mock/screen', { method: 'POST', body: JSON.stringify({ screen: 'login' }) }));
await page.reload({ waitUntil: 'networkidle0' });
await settle();
const loginShown = !!(await page.$('#password'));
console.log('login page shown:', loginShown);
if (!loginShown) errors.push('login page not shown');
await shot('login');
await page.evaluate(() => fetch('/api/mock/screen', { method: 'POST', body: JSON.stringify({ screen: 'panel' }) }));

// Phone layouts
await page.setViewport({ width: 390, height: 844 });
for (const s of ['overview', 'servers', 'actions', 'settings']) {
	await page.goto(base + '#' + s, { waitUntil: 'networkidle0' });
	await page.reload({ waitUntil: 'networkidle0' });
	await settle();
	const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
	if (overflow > 1) errors.push(`${s} scrolls sideways on a phone (${overflow}px)`);
	await shot(s + '-mobile');
}
console.log('ERRORS:', errors.length ? '\n' + [...new Set(errors)].join('\n') : 'none');
await browser.close();
process.exit(errors.length ? 1 : 0);
