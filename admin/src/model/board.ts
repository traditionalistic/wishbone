// Storyboard model: the session's snapshots by screen and device, and the board's card grid.
import type { Board, Frame, Link, PreviewMessage } from '../api'

export type Frames = ReadonlyMap<string, Frame>

export const frameKey = (screen: string, device: string) => `${screen}\n${device}`

function put(next: Map<string, Frame>, prev: Frames, f: Frame): Map<string, Frame> {
  const k = frameKey(f.screen, f.device)
  const old = prev.get(k)
  next.set(k, f.status === 'rendering' && old?.src ? { ...f, src: old.src, width: old.width, height: old.height, settled: old.settled } : f)
  return next
}

function keep(frames: Frames, ok: (f: Frame) => boolean): Frames {
  const next = new Map([...frames].filter(([, f]) => ok(f)))
  return next.size === frames.size ? frames : next
}

/** Folds a socket message into the frames; the same map when it changes nothing. */
export function applyFrame(frames: Frames, m: PreviewMessage): Frames {
  switch (m.type) {
    case 'frame':
      return put(new Map(frames), frames, m.frame)
    case 'frames':
      return m.list.reduce((next, f) => put(next, frames, f), new Map<string, Frame>())
    case 'board': {
      const on = new Set(m.board.screens.map((s) => s.id))
      return keep(frames, (f) => on.has(f.screen))
    }
    case 'devices': {
      const on = new Set(m.list.map((d) => d.id))
      return keep(frames, (f) => on.has(f.device))
    }
  }
  return frames
}

export interface Card {
  id: string
  col: number
  row: number
}

export interface Arrow {
  from: string
  to: string
  kind: Link['kind']
  /** into the same or an earlier column: drawn curved */
  back: boolean
}

export interface Layout {
  cards: Card[]
  arrows: Arrow[]
  cols: number
  rows: number
}

const edge = (from: string, to: string) => `${from}\n${to}`

/**
 * Column = longest path from the sources, DFS back edges ignored; rows in DFS discovery order. Sources are
 * visited by id and children in link order, so the grid does not depend on the focus (it is the start
 * only for screens no source reaches, e.g. a cycle).
 */
export function layoutBoard(board: Board): Layout {
  const ids = board.screens.map((s) => s.id)
  const on = new Set(ids)
  const out = new Map(board.screens.map((s) => [s.id, [] as { to: string; kind: Link['kind'] }[]]))
  const entered = new Set<string>()
  for (const s of board.screens)
    for (const l of s.links) {
      const edges = out.get(s.id)!
      if (l.to === s.id || !on.has(l.to) || edges.some((e) => e.to === l.to && e.kind === l.kind)) continue
      edges.push({ to: l.to, kind: l.kind })
      entered.add(l.to)
    }

  const order: string[] = []
  const post: string[] = []
  const open = new Set<string>()
  const back = new Set<string>()
  const visit = (u: string) => {
    open.add(u)
    order.push(u)
    for (const { to } of out.get(u)!) {
      if (open.has(to)) back.add(edge(u, to))
      else if (!order.includes(to)) visit(to)
    }
    open.delete(u)
    post.push(u)
  }
  const sources = ids.filter((id) => !entered.has(id)).sort()
  for (const root of [...sources, board.focus, ...ids]) if (on.has(root) && !order.includes(root)) visit(root)

  // reverse postorder is a topological order once back edges are dropped
  const col = new Map(ids.map((id) => [id, 0]))
  for (const u of post.reverse())
    for (const { to } of out.get(u)!) if (!back.has(edge(u, to))) col.set(to, Math.max(col.get(to)!, col.get(u)! + 1))

  const height = new Map<number, number>()
  const cards = order.map((id) => {
    const c = col.get(id)!
    const row = height.get(c) ?? 0
    height.set(c, row + 1)
    return { id, col: c, row }
  })
  const arrows = order.flatMap((from) => out.get(from)!.map(({ to, kind }) => ({ from, to, kind, back: col.get(to)! <= col.get(from)! })))
  return { cards, arrows, cols: height.size, rows: Math.max(0, ...height.values()) }
}
