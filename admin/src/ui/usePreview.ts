// Preview session for the open screen: create it, follow its devices/logs over WS, and push every
// source change (debounced 300 ms). The push response's diagnostics drive the editor. A lost session
// (404 on push, or a dropped socket whose code is no longer listed) is recreated; until then pushes go
// through /compile so diagnostics keep updating.
import { useEffect, useRef, useState } from 'react'
import { ApiError, api, type CompileResult, type Device } from '../api'
import type { Diag, Source } from '../model/types'
import type { LogEntry } from './Console'

export type CompileStatus = 'pending' | 'ok' | 'error' | 'offline'

const RETRY_MS = 3000

/** Push to the session, or compile when there is none / it is gone (`lost`). Other errors propagate. */
export async function pushSource(
  code: string | null,
  source: Source,
  client: { pushDocument: (code: string, s: Source) => Promise<CompileResult>; compile: (s: Source) => Promise<CompileResult> } = api,
): Promise<{ result: CompileResult; lost: boolean }> {
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

export function usePreview(screenId: string | null, source: Source | null) {
  const [code, setCode] = useState<string | null>(null)
  /** bumped to (re)create the session */
  const [epoch, setEpoch] = useState(0)
  const [devices, setDevices] = useState<Device[]>([])
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [diags, setDiags] = useState<Diag[]>([])
  const [status, setStatus] = useState<CompileStatus>('pending')

  useEffect(() => setLogs([]), [screenId])

  useEffect(() => {
    setCode(null)
    setDevices([])
    if (!screenId) return
    let dead = false
    let stop: (() => void) | undefined
    let timer: ReturnType<typeof setTimeout> | undefined
    const renew = () => !dead && setEpoch((e) => e + 1)
    const connect = (c: string) => {
      stop = api.previewSocket(
        c,
        (m) => {
          if (m.type === 'devices') setDevices(m.list ?? [])
          else if (m.type === 'log') setLogs((l) => [...l.slice(-499), { at: Date.now(), device: m.device, level: m.level, message: m.message }])
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
    api.createPreview().then(
      ({ code: c }) => {
        if (dead) return
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
  }, [screenId, epoch])

  const seq = useRef(0)
  useEffect(() => {
    if (!source) return
    const n = ++seq.current
    setStatus('pending')
    const h = setTimeout(async () => {
      try {
        const { result, lost } = await pushSource(code, source)
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
    logs,
    clearLogs: () => setLogs([]),
    diags,
    /** publish 422 diagnostics replace the live ones until the next push */
    setDiags,
    status,
    play: code ? (timeline: string) => void api.play(code, timeline).catch(() => undefined) : undefined,
  }
}
