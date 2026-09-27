// Immutable operations on the source node tree. A node is addressed by the list of child
// indices from the root ([] = root), which is also how diagnostics address it (see path.ts).
import type { Node } from './types'

export type Path = number[]

export function getNode(root: Node, path: Path): Node | undefined {
  let n: Node | undefined = root
  for (const i of path) n = n?.children?.[i]
  return n
}

export function updateNode(root: Node, path: Path, fn: (n: Node) => Node): Node {
  if (path.length === 0) return fn(root)
  const [i, ...rest] = path
  const children = [...(root.children ?? [])]
  if (!children[i]) return root
  children[i] = updateNode(children[i], rest, fn)
  return { ...root, children }
}

export function insertNode(root: Node, parent: Path, index: number, node: Node): Node {
  return updateNode(root, parent, (p) => {
    const children = [...(p.children ?? [])]
    children.splice(Math.max(0, Math.min(index, children.length)), 0, node)
    return { ...p, children }
  })
}

export function removeNode(root: Node, path: Path): Node {
  if (path.length === 0) throw new Error('cannot remove the root node')
  const parent = path.slice(0, -1)
  const i = path[path.length - 1]
  return updateNode(root, parent, (p) => ({ ...p, children: (p.children ?? []).filter((_, j) => j !== i) }))
}

export const isPrefix = (a: Path, b: Path): boolean => a.length <= b.length && a.every((x, i) => x === b[i])

/**
 * Moves the node at `from` into `toParent` at `index`, where `toParent`/`index` are given in the
 * coordinates of the tree *before* the move (what a drop target sees). Returns the new root and the
 * moved node's new path, or null when the move is impossible (root, or into its own subtree).
 */
export function moveNode(root: Node, from: Path, toParent: Path, index: number): { root: Node; path: Path } | null {
  if (from.length === 0 || isPrefix(from, toParent)) return null
  const node = getNode(root, from)
  if (!node) return null
  const depth = from.length - 1
  const fromParent = from.slice(0, -1)
  const fromIndex = from[depth]
  const parent = [...toParent]
  // removing `from` shifts later siblings (and anything under them) one step left
  if (isPrefix(fromParent, parent) && parent.length > depth && parent[depth] > fromIndex) parent[depth]--
  let at = index
  if (fromParent.length === toParent.length && isPrefix(fromParent, toParent) && index > fromIndex) at--
  const removed = removeNode(root, from)
  const count = getNode(removed, parent)?.children?.length ?? 0
  at = Math.max(0, Math.min(at, count))
  return { root: insertNode(removed, parent, at, node), path: [...parent, at] }
}

export function walk(root: Node, fn: (n: Node, path: Path) => void, path: Path = []): void {
  fn(root, path)
  root.children?.forEach((c, i) => walk(c, fn, [...path, i]))
}

export function collectIds(root: Node): Set<string> {
  const ids = new Set<string>()
  walk(root, (n) => ids.add(n.id))
  return ids
}

/** `title2` → `title3` if free: trailing digits are stripped, then the first free suffix wins. */
export function freshId(base: string, taken: Set<string>): string {
  const stem = base.replace(/\d+$/, '') || 'node'
  if (!taken.has(stem)) return stem
  for (let i = 1; ; i++) if (!taken.has(stem + i)) return stem + i
}

/** Deep copy with every id replaced by a fresh one; new ids are added to `taken`. */
export function withFreshIds(node: Node, taken: Set<string>): Node {
  const id = freshId(node.id, taken)
  taken.add(id)
  const copy: Node = structuredClone({ ...node, children: undefined })
  copy.id = id
  if (node.children) copy.children = node.children.map((c) => withFreshIds(c, taken))
  else delete copy.children
  return copy
}

export function duplicateNode(root: Node, path: Path): { root: Node; path: Path } {
  const node = getNode(root, path)
  if (!node || path.length === 0) return { root, path }
  const copy = withFreshIds(node, collectIds(root))
  const at = [...path.slice(0, -1), path[path.length - 1] + 1]
  return { root: insertNode(root, path.slice(0, -1), at[at.length - 1], copy), path: at }
}
