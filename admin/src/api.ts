// Admin API client — shapes per contract/api.md.
import type { Catalog, Diag, Source, TokenGroup } from './model/types'

export interface ScreenSummary {
  id: string
  title: string
  updatedAt: string
  publishedRev: number | null
}

export interface ScreenDetail {
  id: string
  title: string
  draft: Source
  publishedRev: number | null
  updatedAt: string
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
}

export type PreviewMessage =
  | { type: 'devices'; list: Device[] }
  | { type: 'log'; device: string; level: 'info' | 'warn' | 'error'; message: string }

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

/** Token/icon lists may come as names, "@group.name" strings or {name} objects — normalize to bare names. */
function names(xs: unknown, group: string): string[] {
  if (!Array.isArray(xs)) return []
  return xs
    .map((x) => (typeof x === 'string' ? x : String((x as { name?: unknown })?.name ?? '')))
    .map((n) => n.replace(new RegExp(`^@${group}\\.`), ''))
    .filter(Boolean)
}

export const api = {
  screens: () => req<ScreenSummary[]>('GET', '/screens'),
  createScreen: (id: string, title: string) => req<{ id: string; title: string; draft: Source }>('POST', '/screens', { id, title }),
  screen: (id: string) => req<ScreenDetail>('GET', `/screens/${enc(id)}`),
  saveDraft: (id: string, draft: Source) => req<{ updatedAt: string }>('PUT', `/screens/${enc(id)}`, { title: draft.title, draft }),
  /** Throws ApiError with `diags` on 422. */
  publish: (id: string) => req<{ revision: number; etag: string }>('POST', `/screens/${enc(id)}/publish`),
  revisions: (id: string) => req<Revision[]>('GET', `/screens/${enc(id)}/revisions`),
  rollback: (id: string, rev: number) => req<{ revision: number; etag: string }>('POST', `/screens/${enc(id)}/rollback/${rev}`),
  compile: (source: Source) => req<CompileResult>('POST', '/compile', { source }),
  template: (name: string) => req<unknown>('GET', `/templates/${enc(name)}`),
  createPreview: (screen?: string) => req<{ code: string }>('POST', '/preview', screen ? { screen } : {}),
  pushDocument: (code: string, source: Source) => req<CompileResult>('POST', `/preview/${enc(code)}/document`, { source }),
  play: (code: string, timeline: string) => req<void>('POST', `/preview/${enc(code)}/play`, { timeline }),

  async catalog(): Promise<Catalog> {
    const raw = await req<Catalog & { tokens: Record<string, unknown>; icons: unknown }>('GET', '/catalog')
    const groups: TokenGroup[] = ['color', 'type', 'space', 'radius', 'motion']
    const icons = names(raw.icons, 'icon')
    const tokens = Object.fromEntries(groups.map((g) => [g, names(raw.tokens?.[g], g)])) as Record<TokenGroup, string[]>
    tokens.icon = icons
    return { ...raw, templates: raw.templates ?? [], ops: raw.ops ?? [], brands: raw.brands ?? [], icons, tokens }
  },

  /** Subscribes to the admin side of a preview session; reconnects until the returned stop() is called. */
  previewSocket(code: string, onMessage: (m: PreviewMessage) => void): () => void {
    const url = `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/admin/api/preview/${enc(code)}/ws?token=${enc(auth.get())}`
    let ws: WebSocket | undefined
    let timer: ReturnType<typeof setTimeout> | undefined
    let stopped = false
    const open = () => {
      ws = new WebSocket(url)
      ws.onmessage = (e) => {
        try {
          onMessage(JSON.parse(String(e.data)) as PreviewMessage)
        } catch {
          /* ignore malformed frames */
        }
      }
      ws.onclose = () => {
        if (!stopped) timer = setTimeout(open, 2000)
      }
    }
    open()
    return () => {
      stopped = true
      clearTimeout(timer)
      ws?.close()
    }
  },
}
