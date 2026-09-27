// Diagnostic paths (contract/api.md `Diag.path`): "root/children/2/props/text", "script",
// "timelines/0/tracks/1". The tail after the node address mirrors the source JSON keys.
import type { Diag } from './types'
import type { Path } from './tree'

export type DiagTarget =
  | { kind: 'node'; node: Path; field: string }
  | { kind: 'script' }
  | { kind: 'timeline'; timeline: number; track: number | undefined; field: string }
  | { kind: 'other' }

export function nodePathString(path: Path, field = ''): string {
  const base = ['root', ...path.flatMap((i) => ['children', String(i)])].join('/')
  return field ? `${base}/${field}` : base
}

export function parseDiagPath(path: string): DiagTarget {
  const seg = path.split('/')
  if (seg[0] === 'script') return { kind: 'script' }
  if (seg[0] === 'root') {
    const node: Path = []
    let i = 1
    while (seg[i] === 'children' && /^\d+$/.test(seg[i + 1] ?? '')) {
      node.push(Number(seg[i + 1]))
      i += 2
    }
    return { kind: 'node', node, field: seg.slice(i).join('/') }
  }
  if (seg[0] === 'timelines' && /^\d+$/.test(seg[1] ?? '')) {
    const hasTrack = seg[2] === 'tracks' && /^\d+$/.test(seg[3] ?? '')
    return {
      kind: 'timeline',
      timeline: Number(seg[1]),
      track: hasTrack ? Number(seg[3]) : undefined,
      field: seg.slice(hasTrack ? 4 : 2).join('/'),
    }
  }
  return { kind: 'other' }
}

/** Diagnostics on exactly this node whose field equals `field` or lies under it ('' = node-level only). */
export function diagsAt(diags: Diag[], node: Path, field: string): Diag[] {
  const key = node.join('.')
  return diags.filter((d) => {
    const t = parseDiagPath(d.path)
    if (t.kind !== 'node' || t.node.join('.') !== key) return false
    return field === '' ? t.field === '' : t.field === field || t.field.startsWith(field + '/')
  })
}

/** Keys (`path.join('.')`) of nodes that have at least one diagnostic. */
export function pathsWithDiags(diags: Diag[]): Set<string> {
  const s = new Set<string>()
  for (const d of diags) {
    const t = parseDiagPath(d.path)
    if (t.kind === 'node') s.add(t.node.join('.'))
  }
  return s
}
