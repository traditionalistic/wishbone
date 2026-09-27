import { describe, expect, it } from 'vitest'
import type { Key } from './types'
import { addKey, duration, moveKey, valueAt } from './timeline'

const keys: Key[] = [
  { t: 0, v: 0 },
  { t: 400, v: 1, ease: 'easeOut' },
]

describe('timeline keys', () => {
  it('addKey inserts sorted and returns its index', () => {
    expect(addKey(keys, 200, 0.5)).toEqual({ keys: [keys[0], { t: 200, v: 0.5 }, keys[1]], index: 1 })
    expect(addKey(keys, 600, 2).index).toBe(2)
  })

  it('addKey on an existing time replaces the value, keeping ease', () => {
    expect(addKey(keys, 400, 3)).toEqual({ keys: [keys[0], { t: 400, v: 3, ease: 'easeOut' }], index: 1 })
  })

  it('moveKey re-sorts and tracks the moved key', () => {
    const m = moveKey(keys, 0, 500)
    expect(m.keys).toEqual([{ t: 400, v: 1, ease: 'easeOut' }, { t: 500, v: 0 }])
    expect(m.index).toBe(1)
  })

  it('moveKey clamps to t ≥ 0 and rounds to ms', () => {
    expect(moveKey(keys, 1, -30.4).keys[0].t).toBe(0)
    expect(moveKey(keys, 1, 12.6).keys[1].t).toBe(13)
  })

  it('valueAt interpolates linearly between neighbours (for new keys)', () => {
    expect(valueAt(keys, 100)).toBe(0.25)
    expect(valueAt(keys, -5)).toBe(0)
    expect(valueAt(keys, 900)).toBe(1)
    expect(valueAt([], 10)).toBe(0)
  })

  it('duration = max key t across tracks', () => {
    expect(duration({ name: 'x', tracks: [{ target: 'a', prop: 'opacity', keys }, { target: 'b', prop: 'scale', keys: [{ t: 900, v: 1 }] }] })).toBe(900)
    expect(duration({ name: 'x', tracks: [] })).toBe(0)
  })
})
