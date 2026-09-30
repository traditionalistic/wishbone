// Admin API client — shapes per contract/api.md.
import type { Catalog, Diag, Source, TokenGroup } from './model/types'

export interface ScreenSummary {
  id: string
  title: string
  updatedAt: string
  publishedRev: number | null
  unpublished: boolean
}

export interface ScreenDetail {
  id: string
  title: string
  draft: Source
  publishedRev: number | null
  updatedAt: string
  unpublished: boolean
}

export interface Revision {
  revision: number
  etag: string
  publishedAt: string
  runtimeMin: string
}

export interface CompileResult {
  ok: boolean
  diags: Diag[]
}

export interface Device {
  id: string
  platform: string
  device: string
  /** takes `render` jobs (snapshots); older playgrounds only follow the live document */
  render: boolean
}

/** A `navigate`/`present` with a literal target; `via` is "node:<id>" or "handler:<name>". */
export interface Link {
  to: string
  kind: 'navigate' | 'present'
  via: string
}

/** A screen on the board; one with `error` (missing, or a draft that does not compile) is not rendered. */
export interface BoardScreen {
  id: string
  title: string
  links: Link[]
  error?: string
}

/** The open screen (`focus`) and the screens linked to it either way, focus first. */
export interface Board {
  focus: string
  screens: BoardScreen[]
}

/** A device's snapshot of a board screen; `src` is a data:image URL. */
export interface Frame {
  screen: string
  device: string
  status: 'rendering' | 'ready' | 'failed'
  src?: string
  width?: number
  height?: number
  /** false: taken at the time cap while something still moved */
  settled?: boolean
  error?: string
}

export type PreviewMessage =
  | { type: 'devices'; list: Device[] }
  | { type: 'log'; device: string; level: 'info' | 'warn' | 'error'; message: string }
  | { type: 'board'; board: Board }
  | { type: 'frame'; frame: Frame }
  /** every stored frame, sent when the admin (re)connects */
  | { type: 'frames'; list: Frame[] }

type Raw = Record<string, unknown>
const obj = (v: unknown): Raw | null => (v !== null && typeof v === 'object' && !Array.isArray(v) ? (v as Raw) : null)
const str = (v: unknown): v is string => typeof v === 'string'
const size = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v) && v > 0
const list = <T>(v: unknown, parse: (x: unknown) => T | null): T[] => (Array.isArray(v) ? v.map(parse).filter((x): x is T => x !== null) : [])

function parseDevice(v: unknown): Device | null {
  const d = obj(v)
  return d && str(d.id) && str(d.platform) && str(d.device) ? { id: d.id, platform: d.platform, device: d.device, render: d.render === true } : null
}

function parseLink(v: unknown): Link | null {
  const l = obj(v)
  return l && str(l.to) && (l.kind === 'navigate' || l.kind === 'present') ? { to: l.to, kind: l.kind, via: str(l.via) ? l.via : '' } : null
}

function parseScreen(v: unknown): BoardScreen | null {
  const s = obj(v)
  if (!s || !str(s.id)) return null
  return { id: s.id, title: str(s.title) ? s.title : '', links: list(s.links, parseLink), ...(str(s.error) ? { error: s.error } : {}) }
}

function parseFrame(v: unknown): Frame | null {
  const f = obj(v)
  if (!f || !str(f.screen) || !str(f.device)) return null
  const at = { screen: f.screen, device: f.device }
  if (f.status === 'rendering') return { ...at, status: 'rendering' }
  if (f.status === 'failed') return { ...at, status: 'failed', error: str(f.error) ? f.error : '' }
  // `src` goes into an <img>: inline images only, never a URL the page would fetch
  if (f.status === 'ready' && str(f.src) && f.src.startsWith('data:image/') && size(f.width) && size(f.height))
    return { ...at, status: 'ready', src: f.src, width: f.width, height: f.height, settled: f.settled !== false }
  return null
}

/** A preview socket message as a typed message; null when malformed or unknown. */
export function parsePreviewMessage(v: unknown): PreviewMessage | null {
  const m = obj(v)
  if (!m) return null
  switch (m.type) {
    case 'devices':
      return { type: 'devices', list: list(m.list, parseDevice) }
    case 'log':
      return str(m.device) && str(m.message) && (m.level === 'info' || m.level === 'warn' || m.level === 'error')
        ? { type: 'log', device: m.device, level: m.level, message: m.message }
        : null
    case 'board':
      return str(m.focus) ? { type: 'board', board: { focus: m.focus, screens: list(m.screens, parseScreen) } } : null
    case 'frame': {
      const frame = parseFrame(m)
      return frame && { type: 'frame', frame }
    }
    case 'frames':
      return { type: 'frames', list: list(m.list, parseFrame) }
  }
  return null
}

const TOKEN_KEY = 'wishbone.admin.token'

export const auth = {
  get: (): string => sessionStorage.getItem(TOKEN_KEY) ?? '',
  set: (t: string) => sessionStorage.setItem(TOKEN_KEY, t),
  clear: () => sessionStorage.removeItem(TOKEN_KEY),
}

/** Fired on any 401 so the app can drop back to the login screen. */
export const UNAUTHORIZED_EVENT = 'wishbone:unauthorized'

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly diags: Diag[] = [],
  ) {
    super(message)
  }
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch('/admin/api' + path, {
    method,
    headers: {
      Authorization: `Bearer ${auth.get()}`,
      ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => null)
  if (!res.ok) {
    if (res.status === 401) window.dispatchEvent(new Event(UNAUTHORIZED_EVENT))
    const e = data?.error
    throw new ApiError(res.status, e?.code ?? `HTTP_${res.status}`, e?.message ?? res.statusText, data?.diags ?? [])
  }
  return data as T
}

const enc = encodeURIComponent

export const api = {
  screens: () => req<ScreenSummary[]>('GET', '/screens'),
  createScreen: (id: string, title: string) => req<{ id: string; title: string; draft: Source }>('POST', '/screens', { id, title }),
  screen: (id: string) => req<ScreenDetail>('GET', `/screens/${enc(id)}`),
  saveDraft: (id: string, draft: Source) => req<{ updatedAt: string }>('PUT', `/screens/${enc(id)}`, { title: draft.title, draft }),
  /** Throws ApiError with `diags` on 422. */
  publish: (id: string) => req<{ revision: number; etag: string }>('POST', `/screens/${enc(id)}/publish`),
  unpublish: (id: string) => req<void>('POST', `/screens/${enc(id)}/unpublish`),
  revisions: (id: string) => req<Revision[]>('GET', `/screens/${enc(id)}/revisions`),
  rollback: (id: string, rev: number) => req<{ revision: number; etag: string }>('POST', `/screens/${enc(id)}/rollback/${rev}`),
  compile: (source: Source) => req<CompileResult>('POST', '/compile', { source }),
  template: (name: string) => req<unknown>('GET', `/templates/${enc(name)}`),
  createPreview: () => req<{ code: string }>('POST', '/preview', {}),
  pushDocument: (code: string, source: Source) => req<CompileResult>('POST', `/preview/${enc(code)}/document`, { source }),
  play: (code: string, timeline: string) => req<void>('POST', `/preview/${enc(code)}/play`, { timeline }),
  /** Clears the session's failed frames so their screens render again. */
  retry: (code: string) => req<void>('POST', `/preview/${enc(code)}/retry`),

  async catalog(): Promise<Catalog> {
    const raw = await req<Omit<Catalog, 'tokens'> & { tokens: Partial<Record<TokenGroup, string[]>> }>('GET', '/catalog')
    const groups: TokenGroup[] = ['color', 'type', 'space', 'radius', 'motion']
    const icons = raw.icons ?? []
    const tokens = Object.fromEntries(groups.map((g) => [g, raw.tokens?.[g] ?? []])) as Record<TokenGroup, string[]>
    tokens.icon = icons
    return { ...raw, templates: raw.templates ?? [], ops: raw.ops ?? [], brands: raw.brands ?? [], icons, tokens }
  },

  /** Live preview sessions (public device API) — used to tell a dropped socket from a dead session. */
  async previewSessions(): Promise<{ code: string }[]> {
    const res = await fetch('/v1/preview/sessions')
    if (!res.ok) throw new ApiError(res.status, `HTTP_${res.status}`, res.statusText)
    return res.json()
  },

  /** Subscribes to the admin side of a preview session; `onClose` fires when the socket drops (not on stop()). */
  previewSocket(code: string, onMessage: (m: PreviewMessage) => void, onClose: () => void): () => void {
    const url = `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/admin/api/preview/${enc(code)}/ws?token=${enc(auth.get())}`
    const ws = new WebSocket(url)
    let stopped = false
    ws.onmessage = (e) => {
      let m: PreviewMessage | null = null
      try {
        m = parsePreviewMessage(JSON.parse(String(e.data)))
      } catch {
        /* not JSON */
      }
      if (m) onMessage(m)
    }
    ws.onclose = () => {
      if (!stopped) onClose()
    }
    return () => {
      stopped = true
      ws.close()
    }
  },
}
