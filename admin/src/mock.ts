// Dev-only fake backend (VITE_MOCK=1 npm run dev): serves the catalog from contract/*.json and fakes
// screens (a demo plus contract/screens), compile diagnostics and the preview session with its board and
// SVG snapshots. `?devices=ios,android,se,old` picks the fake devices (default ios,android) to show every
// preview slot state. Never imported by production builds (main.tsx).
import primitives from '../../contract/primitives.json'
import stdlib from '../../contract/stdlib.json'
import operations from '../../contract/operations.json'
import korpu from '../../contract/tokens/korpu.json'
import icons from '../../contract/icons.json'
import type { Board, BoardScreen, Device, Frame, Link } from './api'
import { frameKey } from './model/board'
import { nodePathString } from './model/path'
import { walk } from './model/tree'
import type { Diag, Source } from './model/types'

const templates = [
  { name: 'Button', params: [{ name: 'title', type: 'string' }, { name: 'style', type: '"primary" | "secondary" | "ghost"', default: 'primary' }, { name: 'icon', type: 'icon?' }, { name: 'loading', type: 'bool', default: false }], events: ['tap'], slot: false },
  { name: 'Card', params: [{ name: 'padding', type: 'number', default: 16 }], events: ['tap'], slot: true },
  { name: 'Chip', params: [{ name: 'title', type: 'string' }, { name: 'selected', type: 'bool', default: false }], events: ['tap'], slot: false },
]
// the contract screens use the other stdlib templates too; the mock knows them by name only
for (const path of Object.keys(import.meta.glob('../../contract/templates/*.json'))) {
  const name = path.slice(path.lastIndexOf('/') + 1, -'.json'.length)
  if (!templates.some((t) => t.name === name)) templates.push({ name, params: [], events: ['tap'], slot: false })
}
const contractScreens = Object.values(import.meta.glob('../../contract/screens/*.json', { eager: true, import: 'default' })) as (Omit<Source, 'timelines'> & { timelines?: Source['timelines'] })[]

const seed: Source = {
  id: 'showcase.demo',
  title: 'Demo',
  script: 'state page = 0\ndata me = demo.profile()\nlet greeting = "Привет, ${me.name}"\n\nhandler next() {\n  animate(@motion.smooth) { page = min(page + 1, 2) }\n}\n',
  timelines: [{ name: 'intro', autoplay: true, tracks: [{ target: 'hero', prop: 'opacity', keys: [{ t: 0, v: 0 }, { t: 400, v: 1, ease: 'easeOut' }] }, { target: 'hero', prop: 'offsetY', stagger: 40, keys: [{ t: 0, v: 24 }, { t: 600, v: 0, ease: 'spring' }] }] }],
  root: {
    type: 'scroll', id: 'root', props: { spacing: '@space.lg', padding: 16 }, on: { appear: 'play(intro)' },
    children: [
      { type: 'text', id: 'hero', props: { text: '=greeting', style: '@type.largeTitle' } },
      { type: 'image', id: 'banner', props: { corner: '@radius.card', height: 180 } },
      { type: 'stack', id: 'offers', props: { axis: 'h', spacing: 8 }, repeat: { over: 'me.offers', as: 'o', key: 'o.id' }, children: [{ type: 'Chip', id: 'chip', props: { title: '=o.title' } }] },
      { type: 'Button', id: 'cta', props: { title: 'Далее' }, on: { tap: 'next()' }, when: 'page < 2', animate: { scale: '@motion.press' } },
    ],
  },
}

const now = () => new Date().toISOString()
const screens = new Map<string, { title: string; draft: Source; updatedAt: string; unpublished: boolean; revs: { revision: number; etag: string; publishedAt: string; runtimeMin: string; source: Source }[] }>([
  [seed.id, { title: seed.title, draft: seed, updatedAt: now(), unpublished: false, revs: [{ revision: 1, etag: '1-abc', publishedAt: now(), runtimeMin: '1.0.0', source: seed }] }],
])
for (const s of contractScreens) screens.set(s.id, { title: s.title, draft: { ...s, timelines: s.timelines ?? [] }, updatedAt: now(), unpublished: false, revs: [] })

type Prim = { props: Record<string, { required?: boolean }> }
const prims = primitives.primitives as Record<string, Prim>

function compile(src: Source): Diag[] {
  const diags: Diag[] = []
  const seen = new Set<string>()
  const ids: string[] = []
  walk(src.root, (n, p) => {
    ids.push(n.id)
    if (seen.has(n.id)) diags.push({ path: nodePathString(p, 'id'), line: 0, col: 0, message: `duplicate id "${n.id}"` })
    seen.add(n.id)
    const prim = prims[n.type]
    if (!prim && !templates.some((t) => t.name === n.type)) diags.push({ path: nodePathString(p), line: 0, col: 0, message: `unknown type ${n.type}` })
    for (const [k, s] of Object.entries(prim?.props ?? {}))
      if (s.required && n.props?.[k] === undefined) diags.push({ path: nodePathString(p, `props/${k}`), line: 0, col: 0, message: `missing required prop ${k}` })
    for (const [ev, code] of Object.entries(n.on ?? {}))
      if (!code.trim()) diags.push({ path: nodePathString(p, `on/${ev}`), line: 1, col: 1, message: 'empty handler' })
  })
  let depth = 0
  src.script.split('\n').forEach((l, i) => {
    for (const ch of l) depth += ch === '{' ? 1 : ch === '}' ? -1 : 0
    const bad = l.indexOf('???')
    if (bad >= 0) diags.push({ path: 'script', line: i + 1, col: bad + 1, message: 'unexpected "?"' })
  })
  if (depth !== 0) diags.push({ path: 'script', line: src.script.split('\n').length, col: 1, message: 'unbalanced braces' })
  src.timelines.forEach((tl, i) => tl.tracks.forEach((tr, j) => {
    if (!ids.includes(tr.target)) diags.push({ path: `timelines/${i}/tracks/${j}`, line: 0, col: 0, message: `no node with id "${tr.target}"` })
  }))
  return diags
}

const MOCK_DEVICES: Record<string, Device & { size: [number, number] }> = {
  ios: { id: 'd1', platform: 'ios', device: 'iPhone 17 Pro', render: true, size: [402, 874] },
  android: { id: 'd2', platform: 'android', device: 'Pixel 9', render: true, size: [412, 915] },
  se: { id: 'd3', platform: 'ios', device: 'iPhone SE', render: true, size: [375, 667] },
  old: { id: 'd4', platform: 'android', device: 'Galaxy S21', render: false, size: [384, 854] },
}
const devices = (new URLSearchParams(location.search).get('devices') ?? 'ios,android').split(',').flatMap((k) => MOCK_DEVICES[k] ?? [])

const LINK = /\b(navigate|present)\(\s*"([^"]+)"/g

/** `navigate`/`present` with a literal target, like the compiler's Meta.Links: unique, sorted by (to, kind, via). */
function links(src: Source): Link[] {
  const out: Link[] = []
  const scan = (code: string, via: (at: number) => string) => {
    for (const m of code.matchAll(LINK)) out.push({ to: m[2], kind: m[1] as Link['kind'], via: via(m.index) })
  }
  const handlers = [...src.script.matchAll(/\bhandler\s+(\w+)/g)]
  scan(src.script, (at) => `handler:${handlers.filter((h) => h.index < at).at(-1)?.[1] ?? '?'}`)
  walk(src.root, (n) => Object.values(n.on ?? {}).forEach((code) => scan(code, () => `node:${n.id}`)))
  const key = (l: Link) => `${l.to} ${l.kind} ${l.via}`
  return [...new Map(out.map((l) => [key(l), l])).values()].sort((a, b) => key(a).localeCompare(key(b)))
}

/** The focus and every screen linked to it either way, BFS, at most 16 (server `board`). */
function boardFor(focus: Source): Board {
  const src = (id: string) => (id === focus.id ? focus : screens.get(id)?.draft)
  const bad = (id: string) => (id === focus.id ? undefined : compile(src(id)!)[0])
  // a draft that does not compile has no links, as on the server
  const out = new Map([...screens.keys(), focus.id].map((id) => [id, bad(id) ? [] : links(src(id)!)]))
  const list: BoardScreen[] = []
  const queue = [focus.id]
  const seen = new Set(queue)
  while (queue.length && list.length < 16) {
    const id = queue.shift()!
    const s = src(id)
    const diag = s && bad(id)
    list.push(!s ? { id, title: '', links: [], error: 'screen not found' } : diag ? { id, title: s.title, links: [], error: `draft does not compile: ${diag.message}` } : { id, title: s.title, links: out.get(id)! })
    const near = [...(out.get(id) ?? []).map((l) => l.to), ...[...out].filter(([, ls]) => ls.some((l) => l.to === id)).map(([from]) => from)]
    for (const n of near) {
      if (seen.has(n)) continue
      seen.add(n)
      queue.push(n)
    }
  }
  return { focus: focus.id, screens: list }
}

const xml = (s: string) => s.replace(/[&<>"]/g, (c) => `&#${c.charCodeAt(0)};`)

function shot(s: BoardScreen, d: (typeof MOCK_DEVICES)[string]): string {
  const [w, h] = d.size
  const hue = [...s.id].reduce((a, c) => a + c.charCodeAt(0), 0) % 360
  const ios = d.platform === 'ios'
  const blocks = [0, 1, 2, 3].map((i) => `<rect x="20" y="${200 + i * 132}" width="${w - 40}" height="112" rx="${ios ? 18 : 12}" fill="hsl(${(hue + i * 40) % 360} 60% ${ios ? 88 : 84}%)"/>`).join('')
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}" font-family="-apple-system, Roboto, sans-serif">
<rect width="${w}" height="${h}" fill="${ios ? '#f2f2f7' : '#fdfbff'}"/>
<text x="24" y="36" font-size="15" font-weight="600" fill="#111">${new Date().toLocaleTimeString()}</text>
<text x="20" y="120" font-size="30" font-weight="700" fill="#111">${xml(s.title || s.id)}</text>
<text x="20" y="152" font-size="15" fill="#8e8e93">${xml(s.id)} · ${xml(d.device)}</text>
${blocks}
<rect x="${w / 2 - 70}" y="${h - 14}" width="140" height="5" rx="2.5" fill="${ios ? '#111' : '#888'}"/>
</svg>`
  return 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg)
}

/** The fake hub: the session's board, frames and the source hash each frame was taken of. */
const hub = { socket: null as MockSocket | null, board: null as Board | null, frames: new Map<string, Frame>(), hashes: new Map<string, string>(), retried: new Set<string>() }
const emit = (m: unknown) => hub.socket?.onmessage?.(new MessageEvent('message', { data: JSON.stringify(m) }))
const putFrame = (f: Frame) => {
  hub.frames.set(frameKey(f.screen, f.device), f)
  emit({ type: 'frame', ...f })
}

function render(s: BoardScreen, d: (typeof MOCK_DEVICES)[string], delay: number) {
  const k = frameKey(s.id, d.id)
  putFrame({ screen: s.id, device: d.id, status: 'rendering' })
  setTimeout(() => {
    // one Android timeout to show Retry, one iOS shot taken in motion
    if (s.id === 'raffle.rules' && d.platform === 'android' && !hub.retried.has(k)) return putFrame({ screen: s.id, device: d.id, status: 'failed', error: 'timeout' })
    putFrame({ screen: s.id, device: d.id, status: 'ready', src: shot(s, d), width: d.size[0], height: d.size[1], settled: !(s.id === 'raffle.invites' && d.platform === 'ios') })
  }, delay)
}

function push(focus: Source) {
  const board = boardFor(focus)
  hub.board = board
  const on = new Set(board.screens.map((s) => s.id))
  for (const [k, f] of hub.frames) if (!on.has(f.screen)) [hub.frames, hub.hashes].forEach((m) => m.delete(k))
  emit({ type: 'board', ...board })
  let delay = 500
  for (const s of board.screens) {
    if (s.error) continue
    const hash = JSON.stringify(s.id === focus.id ? focus : screens.get(s.id)?.draft)
    for (const d of devices) {
      const k = frameKey(s.id, d.id)
      if (!d.render || hub.hashes.get(k) === hash) continue
      hub.hashes.set(k, hash)
      render(s, d, (delay += 250))
    }
  }
}

function retry() {
  let delay = 300
  for (const [k, f] of hub.frames) {
    if (f.status !== 'failed') continue
    hub.retried.add(k)
    const s = hub.board?.screens.find((x) => x.id === f.screen)
    const d = devices.find((x) => x.id === f.device)
    if (s && d) render(s, d, (delay += 250))
  }
}

const json = (status: number, body?: unknown) => new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

async function handle(method: string, path: string, body: unknown): Promise<Response> {
  const b = body as { source: Source; id: string; title: string; draft: Source }
  let m: RegExpMatchArray | null
  if (path === '/catalog')
    return json(200, {
      primitives, stdlib, templates, icons: Object.keys(icons),
      tokens: { color: Object.keys(korpu.colors), type: Object.keys(korpu.type), space: Object.keys(korpu.space), radius: Object.keys(korpu.radius), motion: Object.keys(korpu.motion) },
      ops: Object.entries(operations.operations).map(([id, o]) => ({ id, ...o })), brands: ['korpu', 'kz'],
    })
  if (path === '/screens' && method === 'GET')
    return json(200, [...screens].map(([id, s]) => ({ id, title: s.title, updatedAt: s.updatedAt, publishedRev: s.revs[0]?.revision ?? null, unpublished: s.unpublished })))
  if (path === '/screens' && method === 'POST') {
    if (screens.has(b.id)) return json(409, { error: { code: 'CONFLICT', message: 'id taken', retryable: false } })
    const draft: Source = { id: b.id, title: b.title, script: '', timelines: [], root: { type: 'stack', id: 'root', children: [{ type: 'text', id: 'title', props: { text: b.title } }] } }
    screens.set(b.id, { title: b.title, draft, updatedAt: now(), unpublished: false, revs: [] })
    return json(201, { id: b.id, title: b.title, draft })
  }
  if ((m = path.match(/^\/screens\/([^/]+)(?:\/(publish|revisions|rollback|unpublish)(?:\/(\d+))?)?$/))) {
    const s = screens.get(decodeURIComponent(m[1]))
    if (!s) return json(404, { error: { code: 'SCREEN_NOT_FOUND', message: 'no such screen', retryable: false } })
    if (!m[2] && method === 'GET') return json(200, { id: m[1], title: s.title, draft: s.draft, publishedRev: s.revs[0]?.revision ?? null, updatedAt: s.updatedAt, unpublished: s.unpublished })
    if (!m[2] && method === 'PUT') {
      Object.assign(s, { draft: b.draft, title: b.title ?? s.title, updatedAt: now() })
      return json(200, { updatedAt: s.updatedAt })
    }
    const publishSource = (source: Source) => {
      const revision = (s.revs[0]?.revision ?? 0) + 1
      s.revs.unshift({ revision, etag: `${revision}-mock`, publishedAt: now(), runtimeMin: '1.0.0', source })
      s.unpublished = false
      return json(200, { revision, etag: `${revision}-mock` })
    }
    if (m[2] === 'publish') {
      const diags = compile(s.draft)
      return diags.length ? json(422, { diags }) : publishSource(s.draft)
    }
    if (m[2] === 'revisions') return json(200, s.revs.map(({ source: _s, ...r }) => r))
    if (m[2] === 'rollback') {
      const rev = s.revs.find((r) => r.revision === Number(m![3]))
      if (!rev) return json(409, { error: { code: 'REVISION_NOT_FOUND', message: 'no such revision', retryable: false } })
      s.draft = rev.source
      return publishSource(rev.source)
    }
    if (m[2] === 'unpublish') {
      s.unpublished = true
      return json(204)
    }
  }
  if (path === '/compile' || /^\/preview\/\w+\/document$/.test(path)) {
    const diags = compile(b.source)
    if (path !== '/compile' && !diags.length) push(b.source)
    return json(200, { ok: diags.length === 0, diags, document: null })
  }
  if (path === '/preview') return json(200, { code: 'K7Q2' })
  if (/^\/preview\/\w+\/play$/.test(path)) return json(204)
  if (/^\/preview\/\w+\/retry$/.test(path)) {
    retry()
    return json(204)
  }
  if ((m = path.match(/^\/templates\/(\w+)$/))) return json(200, { id: m[1], script: 'param title: string\nevent tap', root: { type: 'stack', id: 'root', children: [] } })
  return json(404, { error: { code: 'NOT_FOUND', message: path, retryable: false } })
}

// skip the login screen in mock mode
if (!sessionStorage.getItem('wishbone.admin.token')) sessionStorage.setItem('wishbone.admin.token', 'mock')

const realFetch = window.fetch.bind(window)
window.fetch = async (input, init) => {
  const url = new URL(typeof input === 'string' ? input : input instanceof URL ? input.href : input.url, location.href)
  if (url.pathname === '/v1/preview/sessions') return json(200, [{ code: 'K7Q2', screen: hub.board?.focus ?? '', title: '', devices: devices.length, updatedAt: now() }])
  if (!url.pathname.startsWith('/admin/api/')) return realFetch(input, init)
  const authz = new Headers(init?.headers).get('Authorization')
  if (!authz || authz === 'Bearer ') return json(401, { error: { code: 'UNAUTHENTICATED', message: 'password required', retryable: false } })
  await new Promise((r) => setTimeout(r, 80))
  return handle(init?.method ?? 'GET', url.pathname.slice('/admin/api'.length), init?.body ? JSON.parse(String(init.body)) : undefined)
}

class MockSocket extends EventTarget {
  onmessage: ((e: MessageEvent) => void) | null = null
  onclose: (() => void) | null = null
  constructor(_url: string) {
    super()
    hub.socket = this
    setTimeout(() => {
      emit({ type: 'devices', list: devices.map(({ size: _s, ...d }) => d) })
      if (hub.board) emit({ type: 'board', ...hub.board })
      emit({ type: 'frames', list: [...hub.frames.values()] })
    }, 200)
    setTimeout(() => emit({ type: 'log', device: 'd1', level: 'info', message: 'document applied (rev draft)' }), 400)
    setTimeout(() => emit({ type: 'log', device: 'd2', level: 'warn', message: 'image "banner": url is empty' }), 600)
  }
  close() {
    if (hub.socket === this) hub.socket = null
  }
}
window.WebSocket = MockSocket as unknown as typeof WebSocket
