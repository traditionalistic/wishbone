// Dev-only fake backend (VITE_MOCK=1 npm run dev): serves the catalog from contract/*.json and fakes
// screens, compile diagnostics and the preview session. Never imported by production builds (main.tsx).
import primitives from '../../contract/primitives.json'
import stdlib from '../../contract/stdlib.json'
import operations from '../../contract/operations.json'
import korpu from '../../contract/tokens/korpu.json'
import icons from '../../contract/icons.json'
import { nodePathString } from './model/path'
import { walk } from './model/tree'
import type { Diag, Source } from './model/types'

const templates = [
  { name: 'Button', params: [{ name: 'title', type: 'string' }, { name: 'style', type: '"primary" | "secondary" | "ghost"', default: 'primary' }, { name: 'icon', type: 'icon?' }, { name: 'loading', type: 'bool', default: false }], events: ['tap'], slot: false },
  { name: 'Card', params: [{ name: 'padding', type: 'number', default: 16 }], events: ['tap'], slot: true },
  { name: 'Chip', params: [{ name: 'title', type: 'string' }, { name: 'selected', type: 'bool', default: false }], events: ['tap'], slot: false },
]

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
const screens = new Map<string, { title: string; draft: Source; updatedAt: string; revs: { revision: number; etag: string; publishedAt: string; runtimeMin: string; source: Source }[] }>([
  [seed.id, { title: seed.title, draft: seed, updatedAt: now(), revs: [{ revision: 1, etag: '1-abc', publishedAt: now(), runtimeMin: '1.0.0', source: seed }] }],
])

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
  if (path === '/screens' && method === 'GET') return json(200, [...screens].map(([id, s]) => ({ id, title: s.title, updatedAt: s.updatedAt, publishedRev: s.revs[0]?.revision ?? null })))
  if (path === '/screens' && method === 'POST') {
    if (screens.has(b.id)) return json(409, { error: { code: 'CONFLICT', message: 'id taken', retryable: false } })
    const draft: Source = { id: b.id, title: b.title, script: '', timelines: [], root: { type: 'stack', id: 'root', children: [{ type: 'text', id: 'title', props: { text: b.title } }] } }
    screens.set(b.id, { title: b.title, draft, updatedAt: now(), revs: [] })
    return json(201, { id: b.id, title: b.title, draft })
  }
  if ((m = path.match(/^\/screens\/([^/]+)(?:\/(publish|revisions|rollback)(?:\/(\d+))?)?$/))) {
    const s = screens.get(decodeURIComponent(m[1]))
    if (!s) return json(404, { error: { code: 'SCREEN_NOT_FOUND', message: 'no such screen', retryable: false } })
    if (!m[2] && method === 'GET') return json(200, { id: m[1], title: s.title, draft: s.draft, publishedRev: s.revs[0]?.revision ?? null, updatedAt: s.updatedAt })
    if (!m[2] && method === 'PUT') {
      Object.assign(s, { draft: b.draft, title: b.title ?? s.title, updatedAt: now() })
      return json(200, { updatedAt: s.updatedAt })
    }
    const publishSource = (source: Source) => {
      const revision = (s.revs[0]?.revision ?? 0) + 1
      s.revs.unshift({ revision, etag: `${revision}-mock`, publishedAt: now(), runtimeMin: '1.0.0', source })
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
  }
  if (path === '/compile' || /^\/preview\/\w+\/document$/.test(path)) {
    const diags = compile(b.source)
    return json(200, { ok: diags.length === 0, diags, document: null })
  }
  if (path === '/preview') return json(200, { code: 'K7Q2' })
  if (/^\/preview\/\w+\/play$/.test(path)) return json(204)
  if ((m = path.match(/^\/templates\/(\w+)$/))) return json(200, { id: m[1], script: 'param title: string\nevent tap', root: { type: 'stack', id: 'root', children: [] } })
  return json(404, { error: { code: 'NOT_FOUND', message: path, retryable: false } })
}

// skip the login screen in mock mode
if (!sessionStorage.getItem('wishbone.admin.token')) sessionStorage.setItem('wishbone.admin.token', 'mock')

const realFetch = window.fetch.bind(window)
window.fetch = async (input, init) => {
  const url = new URL(typeof input === 'string' ? input : input instanceof URL ? input.href : input.url, location.href)
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
    const send = (data: unknown) => this.onmessage?.(new MessageEvent('message', { data: JSON.stringify(data) }))
    setTimeout(() => send({ type: 'devices', list: [{ id: 'd1', platform: 'ios', device: 'iPhone 17 Pro' }, { id: 'd2', platform: 'android', device: 'Pixel 9' }] }), 200)
    setTimeout(() => send({ type: 'log', device: 'd1', level: 'info', message: 'document applied (rev draft)' }), 400)
    setTimeout(() => send({ type: 'log', device: 'd2', level: 'warn', message: 'image "banner": url is empty' }), 600)
  }
  close() {}
}
window.WebSocket = MockSocket as unknown as typeof WebSocket
