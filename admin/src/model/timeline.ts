import type { Key, Timeline } from './types'

/** Adds a key at `t` (replacing the value of an existing key at the same time). */
export function addKey(keys: Key[], t: number, v: number): { keys: Key[]; index: number } {
  const same = keys.findIndex((k) => k.t === t)
  if (same >= 0) return { keys: keys.map((k, i) => (i === same ? { ...k, v } : k)), index: same }
  const next = [...keys, { t, v }].sort((a, b) => a.t - b.t)
  return { keys: next, index: next.findIndex((k) => k.t === t) }
}

export function moveKey(keys: Key[], index: number, t: number): { keys: Key[]; index: number } {
  const moved = { ...keys[index], t: Math.max(0, Math.round(t)) }
  const next = keys.map((k, i) => (i === index ? moved : k)).sort((a, b) => a.t - b.t)
  return { keys: next, index: next.indexOf(moved) }
}

/** Linear value at `t` — used as the initial value of a key added by clicking the ruler. */
export function valueAt(keys: Key[], t: number): number {
  if (keys.length === 0) return 0
  if (t <= keys[0].t) return keys[0].v
  for (let i = 1; i < keys.length; i++) {
    const a = keys[i - 1]
    const b = keys[i]
    if (t <= b.t) return b.t === a.t ? b.v : a.v + ((b.v - a.v) * (t - a.t)) / (b.t - a.t)
  }
  return keys[keys.length - 1].v
}

export const duration = (tl: Timeline): number => Math.max(0, ...tl.tracks.flatMap((tr) => tr.keys.map((k) => k.t)))
