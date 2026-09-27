import { useState, type MouseEvent } from 'react'
import { parseDiagPath } from '../model/path'
import { addKey, duration, moveKey, valueAt } from '../model/timeline'
import type { Diag, Ease, Json, Key, Timeline, Track } from '../model/types'
import { useCtx } from './ctx'
import { DraftInput } from './PropField'

export interface TlSel {
  tl: number
  track?: number
  key?: number
}

interface Props {
  timelines: Timeline[]
  onChange: (tls: Timeline[]) => void
  nodeIds: string[]
  diags: Diag[]
  sel: TlSel
  setSel: (s: TlSel) => void
  onPlay?: (name: string) => void
}

const LANE_H = 26
/** left inset so a key at t=0 isn't clipped */
const X0 = 8
const HEAD_W = 270
const EASES = ['linear', 'easeIn', 'easeOut', 'easeInOut', 'spring']

const setAt = <T,>(xs: T[], i: number, x: T): T[] => xs.map((y, j) => (j === i ? x : y))
const num = (s: string) => {
  if (s.trim() === '') return { ok: true as const, v: undefined }
  const n = Number(s)
  return Number.isFinite(n) ? { ok: true as const, v: n } : { ok: false as const }
}
const fmtNum = (v: Json | undefined) => (v === undefined ? '' : String(v))

export function TimelinePanel({ timelines, onChange, nodeIds, diags, sel, setSel, onPlay }: Props) {
  const { catalog } = useCtx()
  const [pxPerMs, setPx] = useState(0.4)
  const tl = timelines[sel.tl] as Timeline | undefined
  const tdiags = diags.map((d) => ({ d, t: parseDiagPath(d.path) })).filter((x) => x.t.kind === 'timeline')
  const diagFor = (ti: number, track?: number) =>
    tdiags.filter(({ t }) => t.kind === 'timeline' && t.timeline === ti && (track === undefined || t.track === track)).map((x) => x.d)

  const setTl = (next: Timeline) => onChange(setAt(timelines, sel.tl, next))
  const setTrack = (ti: number, tr: Track) => tl && setTl({ ...tl, tracks: setAt(tl.tracks, ti, tr) })

  const addTimeline = () => {
    const names = new Set(timelines.map((t) => t.name))
    let name = 'intro'
    for (let i = 2; names.has(name); i++) name = `timeline${i}`
    onChange([...timelines, { name, tracks: [] }])
    setSel({ tl: timelines.length })
  }

  if (!tl)
    return (
      <div className="tl-empty">
        <p className="dim">No timelines. A timeline animates node opacity/scale/offset/rotation over time; start it with play(name) or autoplay.</p>
        <button onClick={addTimeline}>+ Timeline</button>
      </div>
    )

  const span = Math.max(1000, Math.ceil((duration(tl) + 500) / 500) * 500)
  const width = X0 + span * pxPerMs + X0
  const step = [100, 200, 500, 1000, 2000, 5000].find((s) => s * pxPerMs >= 60) ?? 5000
  const ticks = Array.from({ length: Math.floor(span / (step / 5)) + 1 }, (_, i) => (i * step) / 5)
  const selKey = sel.track !== undefined && sel.key !== undefined ? tl.tracks[sel.track]?.keys[sel.key] : undefined

  const clickLane = (e: MouseEvent<SVGSVGElement>, ti: number) => {
    const x = e.clientX - e.currentTarget.getBoundingClientRect().left - X0
    const t = Math.max(0, Math.round(x / pxPerMs / 10) * 10)
    const tr = tl.tracks[ti]
    const r = addKey(tr.keys, t, Math.round(valueAt(tr.keys, t) * 1000) / 1000)
    setTrack(ti, { ...tr, keys: r.keys })
    setSel({ ...sel, track: ti, key: r.index })
  }

  // Drag recomputes from the snapshot taken at mousedown, so every move is a pure function of it.
  const dragKey = (e: MouseEvent, ti: number, ki: number) => {
    e.stopPropagation()
    const snapTls = timelines
    const snapTl = tl
    const tr = tl.tracks[ti]
    const x0 = e.clientX
    const t0 = tr.keys[ki].t
    setSel({ ...sel, track: ti, key: ki })
    const move = (ev: globalThis.MouseEvent) => {
      const t = Math.round((t0 + (ev.clientX - x0) / pxPerMs) / 10) * 10
      const m = moveKey(tr.keys, ki, t)
      onChange(setAt(snapTls, sel.tl, { ...snapTl, tracks: setAt(snapTl.tracks, ti, { ...tr, keys: m.keys }) }))
      setSel({ tl: sel.tl, track: ti, key: m.index })
    }
    const up = () => {
      window.removeEventListener('mousemove', move)
      window.removeEventListener('mouseup', up)
    }
    window.addEventListener('mousemove', move)
    window.addEventListener('mouseup', up)
  }

  const setKey = (k: Key) => {
    if (sel.track === undefined || sel.key === undefined) return
    const tr = tl.tracks[sel.track]
    const keys = setAt(tr.keys, sel.key, k).sort((a, b) => a.t - b.t)
    setTrack(sel.track, { ...tr, keys })
    setSel({ ...sel, key: keys.indexOf(k) })
  }

  const easeName = (e: Ease | undefined) => (e === undefined ? 'linear' : Array.isArray(e) ? 'custom' : e)

  return (
    <div className="timeline-panel">
      <div className="tl-list">
        {timelines.map((t, i) => (
          <button key={i} className={[i === sel.tl && 'on', diagFor(i).length && 'error'].filter(Boolean).join(' ')} onClick={() => setSel({ tl: i })}>
            {t.name || '(unnamed)'}
          </button>
        ))}
        <button className="add" onClick={addTimeline}>
          + Timeline
        </button>
      </div>
      <div className="tl-main">
        <div className="tl-head">
          <input className="tl-name" value={tl.name} onChange={(e) => setTl({ ...tl, name: e.target.value })} title="name (play(name))" />
          <label title="repeat count, -1 = forever">
            repeat
            <DraftInput value={fmtNum(tl.repeat)} parse={num} format={fmtNum} onCommit={(v) => setTl({ ...tl, repeat: v as number | undefined })} placeholder="1" />
          </label>
          <label>
            <input type="checkbox" checked={!!tl.autoreverse} onChange={(e) => setTl({ ...tl, autoreverse: e.target.checked || undefined })} /> autoreverse
          </label>
          <label>
            <input type="checkbox" checked={!!tl.autoplay} onChange={(e) => setTl({ ...tl, autoplay: e.target.checked || undefined })} /> autoplay
          </label>
          <span className="dim">{duration(tl)} ms</span>
          <span className="spacer" />
          <input type="range" min={0.05} max={2} step={0.05} value={pxPerMs} onChange={(e) => setPx(Number(e.target.value))} title="zoom" />
          <button className="primary" disabled={!onPlay || !tl.name} onClick={() => onPlay?.(tl.name)} title="POST …/play to every device in the session">
            ▶ Play on devices
          </button>
          <button
            onClick={() => {
              if (!confirm(`Delete timeline “${tl.name}”?`)) return
              onChange(timelines.filter((_, i) => i !== sel.tl))
              setSel({ tl: Math.max(0, sel.tl - 1) })
            }}
          >
            Delete
          </button>
        </div>
        {tdiags
          .filter(({ t }) => t.kind === 'timeline' && t.timeline === sel.tl && t.track === undefined)
          .map(({ d }, i) => (
            <div key={i} className="field-error">
              {d.message}
            </div>
          ))}
        <div className="tl-grid">
          <div className="tl-row ruler">
            <div className="tl-track-head" style={{ width: HEAD_W }} />
            <svg width={width} height={20}>
              {ticks.map((t) => (
                <g key={t}>
                  <line x1={X0 + t * pxPerMs} x2={X0 + t * pxPerMs} y1={t % step === 0 ? 6 : 14} y2={20} className="tick" />
                  {t % step === 0 && (
                    <text x={X0 + t * pxPerMs + 3} y={11} className="tick-label">
                      {t >= 1000 ? `${t / 1000}s` : `${t}`}
                    </text>
                  )}
                </g>
              ))}
            </svg>
          </div>
          {tl.tracks.map((tr, ti) => (
            <div key={ti} className={'tl-row' + (diagFor(sel.tl, ti).length ? ' error' : '') + (sel.track === ti ? ' sel' : '')} title={diagFor(sel.tl, ti).map((d) => d.message).join('\n')}>
              <div className="tl-track-head" style={{ width: HEAD_W }}>
                <select value={tr.target} onChange={(e) => setTrack(ti, { ...tr, target: e.target.value })} title="target node id">
                  {!nodeIds.includes(tr.target) && <option value={tr.target}>{tr.target || '—'} (missing)</option>}
                  {nodeIds.map((id) => (
                    <option key={id}>{id}</option>
                  ))}
                </select>
                <select value={tr.prop} onChange={(e) => setTrack(ti, { ...tr, prop: e.target.value })}>
                  {catalog.primitives.timelineProps.map((p) => (
                    <option key={p}>{p}</option>
                  ))}
                </select>
                <DraftInput value={fmtNum(tr.stagger)} parse={num} format={fmtNum} onCommit={(v) => setTrack(ti, { ...tr, stagger: v as number | undefined })} placeholder="stagger" />
                <button className="icon" title="Remove track" onClick={() => setTl({ ...tl, tracks: tl.tracks.filter((_, j) => j !== ti) })}>
                  ×
                </button>
              </div>
              <svg width={width} height={LANE_H} className="lane" onMouseDown={(e) => clickLane(e, ti)}>
                {tr.keys.length > 1 && <line x1={X0 + tr.keys[0].t * pxPerMs} x2={X0 + tr.keys[tr.keys.length - 1].t * pxPerMs} y1={LANE_H / 2} y2={LANE_H / 2} className="span" />}
                {tr.keys.map((k, ki) => (
                  <g key={ki} transform={`translate(${X0 + k.t * pxPerMs},${LANE_H / 2})`} onMouseDown={(e) => dragKey(e, ti, ki)} className={sel.track === ti && sel.key === ki ? 'key sel' : 'key'}>
                    <title>{`${k.t} ms → ${k.v} (${easeName(k.ease)})`}</title>
                    <rect x={-5} y={-5} width={10} height={10} transform="rotate(45)" />
                  </g>
                ))}
              </svg>
            </div>
          ))}
        </div>
        <button
          className="add"
          disabled={!nodeIds.length}
          onClick={() => setTl({ ...tl, tracks: [...tl.tracks, { target: nodeIds[0], prop: 'opacity', keys: [{ t: 0, v: 0 }, { t: 400, v: 1 }] }] })}
        >
          + Track
        </button>
        {selKey && (
          <div className="key-inspector">
            <span className="dim">key</span>
            <label>
              t, ms
              <DraftInput value={String(selKey.t)} parse={num} format={fmtNum} onCommit={(v) => v !== undefined && setKey({ ...selKey, t: Math.max(0, v as number) })} />
            </label>
            <label>
              value
              <DraftInput value={String(selKey.v)} parse={num} format={fmtNum} onCommit={(v) => v !== undefined && setKey({ ...selKey, v: v as number })} />
            </label>
            <label title="easing of the segment leading into this key">
              ease
              <select
                value={easeName(selKey.ease)}
                onChange={(e) => {
                  const v = e.target.value
                  const base = { t: selKey.t, v: selKey.v }
                  setKey(v === 'linear' ? base : { ...base, ease: v === 'custom' ? [0.25, 0.1, 0.25, 1] : v })
                }}
              >
                {[...EASES, 'custom'].map((e) => (
                  <option key={e}>{e}</option>
                ))}
              </select>
            </label>
            {Array.isArray(selKey.ease) && (
              <label title="cubic-bezier x1, y1, x2, y2">
                bezier
                <DraftInput
                  value={selKey.ease.join(', ')}
                  parse={(s) => {
                    const ns = s.split(',').map((x) => Number(x.trim()))
                    return ns.length === 4 && ns.every(Number.isFinite) ? { ok: true, v: ns } : { ok: false }
                  }}
                  format={(v) => (Array.isArray(v) ? v.join(', ') : '')}
                  onCommit={(v) => setKey({ ...selKey, ease: v as [number, number, number, number] })}
                />
              </label>
            )}
            <button
              onClick={() => {
                const tr = tl.tracks[sel.track!]
                setTrack(sel.track!, { ...tr, keys: tr.keys.filter((_, i) => i !== sel.key) })
                setSel({ tl: sel.tl, track: sel.track })
              }}
            >
              Delete key
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
