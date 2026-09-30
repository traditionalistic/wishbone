// Board tab: the open screen and the screens linked to it, placed by layoutBoard, each card with both
// platforms' snapshots. One SVG layer under the cards draws the links.
import { useMemo } from 'react'
import type { Board, Device, Frame } from '../api'
import { layoutBoard, type Card } from '../model/board'
import { PLATFORMS, Shot, slotDevice, type Picks } from './Preview'

const CARD_W = 208
const CARD_H = 264
const GAP_X = 80
const GAP_Y = 24
const PAD = 24
/** past the last column and row: room for the bulge of back arrows */
const BULGE = 64

const left = (c: Card) => PAD + c.col * (CARD_W + GAP_X)
const top = (c: Card) => PAD + c.row * (CARD_H + GAP_Y)

/** Forward: right edge → left edge. Back: lower on the card and bowed, so it never overlaps a forward link. */
function path(a: Card, b: Card, back: boolean): string {
  if (!back) {
    const [x1, y1, x2, y2] = [left(a) + CARD_W, top(a) + CARD_H / 2, left(b), top(b) + CARD_H / 2]
    const dx = (x2 - x1) / 2
    return `M${x1},${y1} C${x1 + dx},${y1} ${x2 - dx},${y2} ${x2},${y2}`
  }
  const [y1, y2] = [top(a) + CARD_H * 0.75, top(b) + CARD_H * 0.75]
  if (a.col === b.col) {
    const x = left(a) + CARD_W
    return `M${x},${y1} C${x + BULGE},${y1} ${x + BULGE},${y2} ${x},${y2}`
  }
  const [x1, x2] = [left(a), left(b) + CARD_W]
  return `M${x1},${y1} C${x1 - 40},${y1 + 48} ${x2 + 40},${y2 + 48} ${x2},${y2}`
}

export function Storyboard(p: {
  board: Board | null
  devices: Device[]
  picks: Picks
  frame: (screen: string, device: string) => Frame | undefined
  code: string | null
  /** the open source does not compile */
  stale: boolean
  onRetry?: () => void
  onOpen: (id: string) => void
  canOpen: (id: string) => boolean
}) {
  const layout = useMemo(() => p.board && layoutBoard(p.board), [p.board])
  if (!p.board || !layout) return <p className="empty">Доски пока нет: сервер присылает её после удачной компиляции экрана</p>
  const { board } = p
  const cards = new Map(layout.cards.map((c) => [c.id, c]))
  const width = PAD * 2 + layout.cols * CARD_W + (layout.cols - 1) * GAP_X + BULGE
  const height = PAD * 2 + layout.rows * CARD_H + (layout.rows - 1) * GAP_Y + BULGE

  return (
    <div className="board">
      <div className="board-bar">
        <span className="legend">
          <svg width="28" height="8" aria-hidden="true">
            <line x1="0" y1="4" x2="28" y2="4" className="arrow" />
          </svg>
          navigate
        </span>
        <span className="legend">
          <svg width="28" height="8" aria-hidden="true">
            <line x1="0" y1="4" x2="28" y2="4" className="arrow present" />
          </svg>
          present
        </span>
        {board.screens.length === 1 && <span className="board-hint">Свяжите экраны через navigate(…) / present(…)</span>}
      </div>
      <div className="board-canvas" style={{ width, height }}>
        <svg className="board-arrows" width={width} height={height} aria-hidden="true">
          <defs>
            <marker id="board-arrowhead" viewBox="0 0 10 10" refX="10" refY="5" markerWidth="7" markerHeight="7" orient="auto">
              <path d="M0,1 L10,5 L0,9 z" />
            </marker>
          </defs>
          {layout.arrows.map((a) => (
            <path key={`${a.from} ${a.to} ${a.kind}`} className={`arrow ${a.kind}`} d={path(cards.get(a.from)!, cards.get(a.to)!, a.back)} markerEnd="url(#board-arrowhead)" />
          ))}
        </svg>
        {board.screens.map((s) => {
          const c = cards.get(s.id)!
          const focus = s.id === board.focus
          const openable = p.canOpen(s.id)
          return (
            <div
              key={s.id}
              className={`card${focus ? ' focus' : ''}${s.error ? ' ghost' : ''}`}
              style={{ left: left(c), top: top(c), width: CARD_W, height: CARD_H }}
              onClick={openable ? () => p.onOpen(s.id) : undefined}
            >
              <button className="card-head" disabled={!openable} aria-current={focus ? 'page' : undefined} title={focus ? 'Open the script' : `Open ${s.id}`}>
                <b>{s.title || s.id}</b>
                <span>{s.id}</span>
              </button>
              {s.error ? (
                <p className="ghost-error">{s.error}</p>
              ) : (
                <div className="card-shots">
                  {PLATFORMS.map((platform) => {
                    const device = slotDevice(p.devices, platform.id, p.picks)
                    return (
                      <div key={platform.id} className="shot-box">
                        <Shot
                          compact
                          screen={s.id}
                          platform={platform}
                          device={device}
                          frame={device && p.frame(s.id, device.id)}
                          code={p.code}
                          stale={focus && p.stale}
                          onRetry={p.onRetry}
                        />
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
