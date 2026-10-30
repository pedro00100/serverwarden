// Takes the README screenshots from `make dev` (sample data, English) into
// docs/screenshots. Run with: make screenshots (BASE=url for another server).
import puppeteer from 'puppeteer-core';

const base = process.env.BASE || 'http://127.0.0.1:5173/';
const browser = await puppeteer.launch({ executablePath: '/usr/bin/chromium-browser', args: ['--no-sandbox', '--disable-dev-shm-usage'] });
const page = await browser.newPage();
const settle = (ms = 1500) => new Promise((r) => setTimeout(r, ms));
const click = (sel) => page.evaluate((s) => document.querySelector(s)?.click(), sel);
const screen = (name) => fetch(base + 'api/mock/screen', { method: 'POST', body: JSON.stringify({ screen: name }) });
const desktop = () => page.setViewport({ width: 1440, height: 900, deviceScaleFactor: 1 });
async function open(hash, { reload = false } = {}) {
	await page.goto(base + hash, { waitUntil: 'networkidle0' });
	if (reload) await page.reload({ waitUntil: 'networkidle0' });
	await page.evaluate(() => document.fonts.ready);
	await settle();
}
const shot = (name, fullPage = false) => page.screenshot({ path: `/shots/${name}.png`, fullPage });

await screen('panel');
await desktop();
await open('#overview', { reload: true });
await shot('overview', true);
await open('#servers/home');
await shot('servers');
await open('#security');
await shot('security', true);
await open('#vulnerabilities');
await shot('vulnerabilities');

await open('#actions/home');
await page.evaluate(() => [...document.querySelectorAll('.action')].find((c) => c.querySelector('.yours'))?.querySelector('.run')?.click());
await settle(600);
await shot('actions-unlock');
if (await page.$eval('dialog', (d) => d.open).catch(() => false)) {
	await page.type('dialog input[type=password]', 'sample password');
	await click('dialog button[type=submit]');
	await settle();
}
await shot('actions');

await open('#settings');
await shot('settings');
await open('#settings/commands');
await shot('settings-commands');

await screen('setup');
await open('', { reload: true });
await settle(2500);
await shot('setup-welcome');
await click('button.cta');
await settle();
await click('form.steps button[type=submit]');
await settle();
await click('.add-net');
await settle(600);
await page.type('dialog input', '10.99.0.0/24');
await settle(600);
await shot('setup-network');
await screen('login');
await open('', { reload: true });
await shot('login');
await screen('panel');

await page.setViewport({ width: 390, height: 844, deviceScaleFactor: 2 });
await open('#overview', { reload: true });
await shot('mobile-overview');
await open('#servers/home');
await shot('mobile-servers');

await browser.close();
console.log('screenshots written');
