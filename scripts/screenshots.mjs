// README screenshots and a browser smoke test against a demo instance:
//
//   make build && DATA_DIR=/tmp/forum-demo DEMO_DATA=true AUTH_MODE=none HOST=127.0.0.1 PORT=18267 ./pocket-forum &
//   CDP=http://127.0.0.1:9337 BASE=http://127.0.0.1:18267 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18267'
const shots = process.env.SHOTS !== '0'
const only = process.env.ONLY || ''
const dir = process.env.OUT || new URL('../docs/screenshots/', import.meta.url).pathname
const browser = process.env.CDP ? await chromium.connectOverCDP(process.env.CDP) : await chromium.launch()
const errors = []

async function session({ phone = false, dark = false } = {}) {
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: 1440, height: 900 },
    deviceScaleFactor: phone ? 3 : 2, isMobile: phone, hasTouch: phone, colorScheme: dark ? 'dark' : 'light',
  })
  const page = await ctx.newPage()
  page.on('pageerror', e => errors.push(`pageerror: ${e}`))
  page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource|EventSource/.test(m.text())) errors.push(m.text()) })
  return { ctx, page }
}
async function go(page, path, wait = 1200) {
  await page.goto(base + path, { waitUntil: 'domcontentloaded' })
  await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' }).catch(() => {})
  await page.waitForTimeout(wait)
}
async function shot(page, name, full = false) { if (shots && (!only || only.split(',').includes(name))) await page.screenshot({ path: dir + name + '.png', fullPage: full }) }
const topicId = (page, title) => page.evaluate(t => fetch('/api/v1/search?titles=1&q=' + encodeURIComponent(t)).then(r => r.json()).then(r => r.hits[0].topic_id), title)

{
  const { ctx, page } = await session()
  await go(page, '/', 2000)
  assert.ok((await page.content()).includes('Pocket Forum is here'), 'the latest topics show')
  await shot(page, 'desktop-home')
  const q = await topicId(page, 'back up a Pocket app')
  await go(page, `/t/${q}`, 1500)
  assert.ok((await page.content()).includes('VACUUM INTO'), 'the posts show')
  await shot(page, 'desktop-topic')
  await go(page, '/f/pocket-homelab?tab=categories')
  await shot(page, 'desktop-categories')
  const ev = await topicId(page, 'birthday lunch')
  await go(page, `/t/${ev}/1`)
  await shot(page, 'desktop-event')
  const poll = await topicId(page, 'offline mode')
  await go(page, `/t/${poll}`)
  await shot(page, 'desktop-poll')
  await go(page, '/review')
  await shot(page, 'desktop-review')
  await go(page, '/new?forum=pocket-homelab')
  await page.fill('.title-input', 'How do I restore a backup')
  await page.waitForTimeout(1200)
  await shot(page, 'desktop-new')
  await go(page, '/search?q=backup')
  await shot(page, 'desktop-search')
  await go(page, '/inbox')
  await shot(page, 'desktop-inbox')
  await go(page, '/forums')
  await shot(page, 'desktop-forums')
  await go(page, '/f/pocket-homelab/settings/moderation')
  await shot(page, 'desktop-forum-settings')
  await go(page, '/settings/apps')
  await shot(page, 'desktop-settings')
  await ctx.close()
}
{
  const { ctx, page } = await session({ dark: true })
  await go(page, '/', 1500)
  await shot(page, 'desktop-home-dark')
  await ctx.close()
}
{
  const { ctx, page } = await session({ phone: true })
  await go(page, '/', 1500)
  await shot(page, 'phone-home')
  const q = await topicId(page, 'back up a Pocket app')
  await go(page, `/t/${q}`)
  await shot(page, 'phone-topic')
  await go(page, '/p/pocket-homelab')
  await shot(page, 'phone-public')
  await go(page, '/events')
  await shot(page, 'phone-events')
  await ctx.close()
}
await browser.close().catch(() => {})
if (errors.length) { console.error(errors.join('\n')); process.exit(1) }
console.log('ok')
