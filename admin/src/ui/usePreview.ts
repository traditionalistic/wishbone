// The tab's preview session: open it once (the code survives reloads in sessionStorage while the server
// still lists it), follow its devices, logs, board and frames over WS, and push every source change
// (debounced 300 ms, one push in flight at a time); opening another screen is just a push of its source. The
// push response's diagnostics drive the editor. A lost session (404 on push, or a dropped socket whose code
// is no longer listed) is recreated; until then pushes go through /compile so diagnostics keep updating.
import { useEffect, useRef, useState } from 'react'
import { ApiError, api, type Board, type CompileResult, type Device } from '../api'
import { applyFrame, frameKey, type Frames } from '../model/board'
import type { Diag, Source } from '../model/types'
import type { LogEntry } from './Console'

export type CompileStatus = 'pending' | 'ok' | 'error' | 'offline'

const RETRY_MS = 3000
const SESSION_KEY = 'wishbone.admin.preview'

type PushClient = { pushDocument: (code: string, s: Source) => Promise<CompileResult>; compile: (s: Source) => Promise<CompileResult> }

let inflight: Promise<void> | null = null
let latest = 0

/**
 * Push to the session, or compile when there is none / it is gone (`lost`). Other errors propagate. Pushes go out one
 * at a time: the hub makes whichever push arrives last the current screen, so a slow one must not land after a newer
 * one. A push made while another is in flight waits, and only the newest waiting one is sent; those it superseded
 * resolve to null.
 */
export function pushSource(code: string | null, source: Source, client: PushClient = api): Promise<{ result: CompileResult; lost: boolean } | null> {
  const n = ++latest
  const start = () => {
    const p = send(code, source, client)
    inflight = p.then(
      () => undefined,
      () => undefined,
    ).then(() => {
      inflight = null
    })
    return p
  }
  return inflight ? inflight.then(() => (n === latest ? start() : null)) : start()
}

async function send(code: string | null, source: Source, client: PushClient): Promise<{ result: CompileResult; lost: boolean }> {
  if (code) {
    try {
      return { result: await client.pushDocument(code, source), lost: false }
    } catch (e) {
      if (!(e instanceof ApiError && e.status === 404)) throw e
      return { result: await client.compile(source), lost: true }
    }
  }
  return { result: await client.compile(source), lost: false }
}

export async function openSession(
  stored: string | null,
  client: { previewSessions: () => Promise<{ code: string }[]>; createPreview: () => Promise<{ code: string }> } = api,
): Promise<string> {
  if (stored && (await client.previewSessions().then((l) => l.some((s) => s.code === stored), () => false))) return stored
  return (await client.createPreview()).code
}

export function usePreview(source: Source | null) {
  const [code, setCode] = useState<string | null>(null)
  /** bumped to (re)open the session */
  const [epoch, setEpoch] = useState(0)
  const [devices, setDevices] = useState<Device[]>([])
  const [board, setBoard] = useState<Board | null>(null)
  const [frames, setFrames] = useState<Frames>(() => new Map())
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [diags, setDiags] = useState<Diag[]>([])
  const [status, setStatus] = useState<CompileStatus>('pending')

  const screen = source?.id
  useEffect(() => setLogs([]), [screen])

  useEffect(() => {
    setCode(null)
    setDevices([])
    setBoard(null)
    setFrames(new Map())
    let dead = false
    let stop: (() => void) | undefined
    let timer: ReturnType<typeof setTimeout> | undefined
    const renew = () => !dead && setEpoch((e) => e + 1)
    const connect = (c: string) => {
      stop = api.previewSocket(
        c,
        (m) => {
          if (m.type === 'log') setLogs((l) => [...l.slice(-499), { at: Date.now(), device: m.device, level: m.level, message: m.message }])
          else if (m.type === 'devices') setDevices(m.list)
          else if (m.type === 'board') setBoard(m.board)
          setFrames((f) => applyFrame(f, m))
        },
        async () => {
          // dropped: reconnect if the session still lives (or the server is unreachable), else recreate it
          const alive = await api.previewSessions().then((l) => l.some((s) => s.code === c), () => true)
          if (dead) return
          if (alive) timer = setTimeout(() => !dead && connect(c), RETRY_MS)
          else renew()
        },
      )
    }
    openSession(sessionStorage.getItem(SESSION_KEY)).then(
      (c) => {
        if (dead) return
        sessionStorage.setItem(SESSION_KEY, c)
        setCode(c)
        connect(c)
      },
      () => {
        timer = setTimeout(renew, RETRY_MS)
      },
    )
    return () => {
      dead = true
      clearTimeout(timer)
      stop?.()
    }
  }, [epoch])

  const seq = useRef(0)
  useEffect(() => {
    if (!source) return
    const n = ++seq.current
    setStatus('pending')
    const h = setTimeout(async () => {
      try {
        const pushed = await pushSource(code, source)
        if (!pushed) return
        const { result, lost } = pushed
        if (lost) setEpoch((e) => e + 1)
        if (n !== seq.current) return
        setDiags(result.diags ?? [])
        setStatus(result.ok ? 'ok' : 'error')
      } catch {
        if (n === seq.current) setStatus('offline')
      }
    }, 300)
    return () => clearTimeout(h)
  }, [source, code])

  return {
    code,
    devices,
    board,
    frame: (screen: string, device: string) => frames.get(frameKey(screen, device)),
    logs,
    clearLogs: () => setLogs([]),
    diags,
    /** publish 422 diagnostics replace the live ones until the next push */
    setDiags,
    status,
    play: code ? (timeline: string) => void api.play(code, timeline).catch(() => undefined) : undefined,
    retry: code ? () => void api.retry(code).catch(() => undefined) : undefined,
  }
}
