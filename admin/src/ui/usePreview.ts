// Preview session for the open screen: create it, follow its devices/logs over WS, and push every
// source change (debounced 300 ms). The push response's diagnostics drive the editor.
import { useEffect, useRef, useState } from 'react'
import { api, type Device } from '../api'
import type { Diag, Source } from '../model/types'
import type { LogEntry } from './Console'

export type CompileStatus = 'pending' | 'ok' | 'error' | 'offline'

export function usePreview(screenId: string | null, source: Source | null) {
  const [code, setCode] = useState<string | null>(null)
  const [devices, setDevices] = useState<Device[]>([])
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [diags, setDiags] = useState<Diag[]>([])
  const [status, setStatus] = useState<CompileStatus>('pending')

  useEffect(() => {
    setCode(null)
    setDevices([])
    setLogs([])
    if (!screenId) return
    let stop: (() => void) | undefined
    let dead = false
    api
      .createPreview(screenId)
      .then(({ code }) => {
        if (dead) return
        setCode(code)
        stop = api.previewSocket(code, (m) => {
          if (m.type === 'devices') setDevices(m.list ?? [])
          else if (m.type === 'log') setLogs((l) => [...l.slice(-499), { at: Date.now(), device: m.device, level: m.level, message: m.message }])
        })
      })
      .catch(() => {
        /* no session: pushes fall back to /compile (diagnostics only) */
      })
    return () => {
      dead = true
      stop?.()
    }
  }, [screenId])

  const seq = useRef(0)
  useEffect(() => {
    if (!source) return
    const n = ++seq.current
    setStatus('pending')
    const h = setTimeout(async () => {
      try {
        const r = code ? await api.pushDocument(code, source) : await api.compile(source)
        if (n !== seq.current) return
        setDiags(r.diags ?? [])
        setStatus(r.ok ? 'ok' : 'error')
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
