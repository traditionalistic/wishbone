import { describe, expect, it } from 'vitest'
import type { Node } from './types'
import {
  collectIds,
  duplicateNode,
  freshId,
  getNode,
  insertNode,
  moveNode,
  removeNode,
  updateNode,
  walk,
} from './tree'

const t = (id: string): Node => ({ type: 'text', id, props: { text: id } })

function sample(): Node {
  return {
    type: 'stack',
    id: 'root',
    children: [t('a'), { type: 'stack', id: 'b', children: [t('c'), t('d')] }, t('e')],
  }
}

const ids = (n: Node | undefined): string[] => (n?.children ?? []).map((c) => c.id)

describe('getNode / updateNode', () => {
  it('finds by index path', () => {
    const r = sample()
    expect(getNode(r, [])?.id).toBe('root')
    expect(getNode(r, [1, 1])?.id).toBe('d')
    expect(getNode(r, [5])).toBeUndefined()
    expect(getNode(r, [0, 0])).toBeUndefined()
  })

  it('updates immutably, sharing untouched branches', () => {
    const r = sample()
    const next = updateNode(r, [1, 0], (n) => ({ ...n, when: 'x' }))
    expect(getNode(next, [1, 0])?.when).toBe('x')
    expect(getNode(r, [1, 0])?.when).toBeUndefined()
    expect(next.children![0]).toBe(r.children![0])
    expect(next.children![2]).toBe(r.children![2])
  })
})

describe('insertNode', () => {
  it('inserts at index, clamping, and creates children array', () => {
    const r = sample()
    expect(ids(insertNode(r, [], 1, t('x')))).toEqual(['a', 'x', 'b', 'e'])
    expect(ids(insertNode(r, [], 99, t('x')))).toEqual(['a', 'b', 'e', 'x'])
    const leaf = insertNode(r, [0], 0, t('x'))
    expect(ids(getNode(leaf, [0]))).toEqual(['x'])
  })
})

describe('removeNode', () => {
  it('removes nested node', () => {
    const r = removeNode(sample(), [1, 0])
    expect(ids(getNode(r, [1]))).toEqual(['d'])
  })
  it('refuses to remove root', () => {
    expect(() => removeNode(sample(), [])).toThrow()
  })
})

describe('moveNode', () => {
  it('reorders forward within the same parent (index is in pre-removal coordinates)', () => {
    // drop "a" between "b" and "e" → insertion index 2 before removal
    const m = moveNode(sample(), [0], [], 2)!
    expect(ids(m.root)).toEqual(['b', 'a', 'e'])
    expect(m.path).toEqual([1])
  })

  it('reorders backward within the same parent', () => {
    const m = moveNode(sample(), [2], [], 0)!
    expect(ids(m.root)).toEqual(['e', 'a', 'b'])
    expect(m.path).toEqual([0])
  })

  it('reparents into a later sibling, fixing the shifted parent path', () => {
    const m = moveNode(sample(), [0], [1], 1)!
    expect(ids(m.root)).toEqual(['b', 'e'])
    expect(ids(getNode(m.root, [0]))).toEqual(['c', 'a', 'd'])
    expect(m.path).toEqual([0, 1])
  })

  it('moves out of a container to the top level', () => {
    const m = moveNode(sample(), [1, 1], [], 0)!
    expect(ids(m.root)).toEqual(['d', 'a', 'b', 'e'])
    expect(ids(getNode(m.root, [2]))).toEqual(['c'])
    expect(m.path).toEqual([0])
  })

  it('refuses root and moves into own subtree', () => {
    expect(moveNode(sample(), [], [1], 0)).toBeNull()
    expect(moveNode(sample(), [1], [1], 0)).toBeNull()
    expect(moveNode(sample(), [1], [1, 0], 0)).toBeNull()
  })

  it('no-op move keeps the tree equal', () => {
    const m = moveNode(sample(), [1], [], 1)!
    expect(ids(m.root)).toEqual(['a', 'b', 'e'])
    expect(m.path).toEqual([1])
  })
})

describe('ids', () => {
  it('collects all ids', () => {
    expect([...collectIds(sample())].sort()).toEqual(['a', 'b', 'c', 'd', 'e', 'root'])
  })

  it('freshId strips trailing digits and picks the first free number', () => {
    const taken = new Set(['title', 'title1', 'title2', 'x'])
    expect(freshId('title', taken)).toBe('title3')
    expect(freshId('title2', taken)).toBe('title3')
    expect(freshId('hero', taken)).toBe('hero')
    expect(freshId('', taken)).toBe('node')
  })
})

describe('duplicateNode', () => {
  it('inserts a deep copy right after with fresh unique ids', () => {
    const d = duplicateNode(sample(), [1])
    expect(ids(d.root)).toEqual(['a', 'b', 'b1', 'e'])
    expect(d.path).toEqual([2])
    expect(ids(getNode(d.root, [2]))).toEqual(['c1', 'd1'])
    const all: string[] = []
    walk(d.root, (n) => all.push(n.id))
    expect(new Set(all).size).toBe(all.length)
  })

  it('does not share structure with the original', () => {
    const d = duplicateNode(sample(), [0])
    expect(getNode(d.root, [1])!.props).not.toBe(getNode(d.root, [0])!.props)
  })
})

describe('walk', () => {
  it('visits depth-first with paths', () => {
    const seen: string[] = []
    walk(sample(), (n, p) => seen.push(`${n.id}:${p.join('.')}`))
    expect(seen).toEqual(['root:', 'a:0', 'b:1', 'c:1.0', 'd:1.1', 'e:2'])
  })
})
