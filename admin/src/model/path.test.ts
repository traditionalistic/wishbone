import { describe, expect, it } from 'vitest'
import type { Diag } from './types'
import { diagsAt, nodePathString, parseDiagPath, pathsWithDiags } from './path'

const d = (path: string, message = 'x', line = 0, col = 0): Diag => ({ path, line, col, message })

describe('nodePathString', () => {
  it('builds diag-style node paths', () => {
    expect(nodePathString([])).toBe('root')
    expect(nodePathString([2])).toBe('root/children/2')
    expect(nodePathString([1, 0], 'props/text')).toBe('root/children/1/children/0/props/text')
  })
})

describe('parseDiagPath', () => {
  it('node paths with a field tail', () => {
    expect(parseDiagPath('root/children/2/props/text')).toEqual({ kind: 'node', node: [2], field: 'props/text' })
    expect(parseDiagPath('root')).toEqual({ kind: 'node', node: [], field: '' })
    expect(parseDiagPath('root/children/0/children/3/on/tap')).toEqual({ kind: 'node', node: [0, 3], field: 'on/tap' })
    expect(parseDiagPath('root/when')).toEqual({ kind: 'node', node: [], field: 'when' })
  })
  it('script', () => {
    expect(parseDiagPath('script')).toEqual({ kind: 'script' })
  })
  it('timelines with optional track and tail', () => {
    expect(parseDiagPath('timelines/0/tracks/1')).toEqual({ kind: 'timeline', timeline: 0, track: 1, field: '' })
    expect(parseDiagPath('timelines/2')).toEqual({ kind: 'timeline', timeline: 2, track: undefined, field: '' })
    expect(parseDiagPath('timelines/0/tracks/1/keys/3')).toEqual({ kind: 'timeline', timeline: 0, track: 1, field: 'keys/3' })
  })
  it('anything else', () => {
    expect(parseDiagPath('')).toEqual({ kind: 'other' })
    expect(parseDiagPath('title')).toEqual({ kind: 'other' })
  })
})

describe('diagsAt / pathsWithDiags', () => {
  const diags = [d('root/children/2/props/text', 'a'), d('root/children/2/props/textAlign', 'b'), d('root/children/2', 'c'), d('script', 's')]

  it('matches a node field exactly or by prefix segment', () => {
    expect(diagsAt(diags, [2], 'props/text').map((x) => x.message)).toEqual(['a'])
    expect(diagsAt(diags, [2], 'props').map((x) => x.message)).toEqual(['a', 'b'])
    expect(diagsAt(diags, [2], '').map((x) => x.message)).toEqual(['c'])
    expect(diagsAt(diags, [1], '')).toEqual([])
  })

  it('collects node path keys that carry diagnostics', () => {
    expect([...pathsWithDiags(diags)]).toEqual(['2'])
  })
})
