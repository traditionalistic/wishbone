import type { Device, Revision, ScreenSummary } from '../api'
import type { CompileStatus } from './usePreview'

interface Props {
  screens: ScreenSummary[]
  screenId: string | null
  title: string
  onTitle: (t: string) => void
  onOpen: (id: string) => void
  onCreate: () => void
  status: CompileStatus
  problems: number
  code: string | null
  devices: Device[]
  dirty: boolean
  busy: boolean
  notice: string
  onSave: () => void
  onPublish: () => void
  publishedRev: number | null
  unpublished: boolean
  onUnpublish: () => void
  revisions: Revision[]
  onRollback: (rev: number) => void
  onLogout: () => void
}

const STATUS_TEXT: Record<CompileStatus, string> = { pending: 'compiling…', ok: 'compiled', error: 'errors', offline: 'offline' }

export function TopBar(p: Props) {
  return (
    <header className="topbar">
      <span className="brand">Wishbone</span>
      <select value={p.screenId ?? ''} onChange={(e) => e.target.value && p.onOpen(e.target.value)}>
        <option value="" disabled>
          Open screen…
        </option>
        {p.screens.map((s) => (
          <option key={s.id} value={s.id}>
            {s.id} — {s.title}
            {s.unpublished ? ' (снят)' : s.publishedRev ? ` (r${s.publishedRev})` : ' (draft)'}
          </option>
        ))}
      </select>
      <button onClick={p.onCreate} title="New screen">
        +
      </button>
      {p.screenId && (
        <>
          <input className="title-input" value={p.title} onChange={(e) => p.onTitle(e.target.value)} title="Screen title" />
          <span className={`status ${p.status}`} title={p.status === 'error' ? `${p.problems} problem(s)` : STATUS_TEXT[p.status]}>
            <span className="dot" />
            {STATUS_TEXT[p.status]}
            {p.status === 'error' && ` (${p.problems})`}
          </span>
          <span className="session" title="Type this code in the Playground app on a device to preview live">
            preview <b>{p.code ?? '····'}</b>
          </span>
          <span className="devices" title={p.devices.map((d) => `${d.device} (${d.platform})`).join('\n') || 'No devices connected'}>
            {p.devices.length ? p.devices.map((d) => <span key={d.id} className={`dev ${d.platform}`}>{d.device}</span>) : <span className="dim">no devices</span>}
          </span>
        </>
      )}
      <span className="spacer" />
      {p.notice && <span className="notice">{p.notice}</span>}
      {p.screenId && (
        <>
          <button onClick={p.onSave} disabled={p.busy || !p.dirty} title="Save draft (⌘S)">
            {p.dirty ? '● Save' : 'Saved'}
          </button>
          <button className="primary" onClick={p.onPublish} disabled={p.busy} title="Compile and publish a new revision">
            Publish
          </button>
          <button
            onClick={() => {
              if (confirm('Снять экран с публикации? Клиенты получат SCREEN_NOT_FOUND до следующей публикации.')) p.onUnpublish()
            }}
            disabled={p.busy || !p.publishedRev || p.unpublished}
            title="Снять экран с публикации"
          >
            Снять с публикации
          </button>
          <select
            value=""
            onChange={(e) => {
              const rev = Number(e.target.value)
              if (rev && confirm(`Roll back to r${rev}? It is published as a new revision and becomes the draft.`)) p.onRollback(rev)
            }}
          >
            <option value="">{p.unpublished ? 'Снят с публикации' : p.publishedRev ? `Live r${p.publishedRev}` : 'Not published'}</option>
            {p.revisions.map((r) => (
              <option key={r.revision} value={r.revision}>
                r{r.revision} · {new Date(r.publishedAt).toLocaleString()} · runtime ≥ {r.runtimeMin} — roll back
              </option>
            ))}
          </select>
        </>
      )}
      <button className="link" onClick={p.onLogout}>
        Log out
      </button>
    </header>
  )
}
