import { describe, expect, it } from 'vitest'
import { sourceError } from './types'

describe('sourceError', () => {
  const ok = { id: 's', title: 't', script: '', timelines: [], root: { type: 'stack', id: 'root', children: [{ type: 'text', id: 'a' }] } }
  it('accepts a well-formed source', () => expect(sourceError(ok)).toBeNull())
  it('reports the first structural problem with its path', () => {
    expect(sourceError([])).toMatch(/object/)
    expect(sourceError({ ...ok, script: 1 })).toMatch(/script/)
    expect(sourceError({ ...ok, timelines: {} })).toMatch(/timelines/)
    expect(sourceError({ ...ok, root: { ...ok.root, children: [{ id: 'x' }] } })).toBe('root/children/0: "type" must be a non-empty string')
  })
})
