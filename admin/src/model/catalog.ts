// Reading the catalog (GET /admin/api/catalog): prop types → editor kinds, events, new nodes.
import type { Catalog, Json, Node, PropSpec, TokenGroup } from './types'
import { freshId } from './tree'

export interface PropKind {
  /** editor for the literal mode; 'token' = no literal at all (color/textStyle/icon/motion) */
  base: 'bool' | 'number' | 'string' | 'enum' | 'token' | 'json'
  /** string literal options ("v" | "h"), also offered next to a number */
  options: string[]
  /** token groups offered in token mode */
  groups: TokenGroup[]
}

/** Splits a DSL type on top-level `|`. */
function splitTop(type: string): string[] {
  const out: string[] = []
  let depth = 0
  let quoted = false
  let cur = ''
  for (const ch of type) {
    if (ch === '"') quoted = !quoted
    else if (!quoted && '<{(['.includes(ch)) depth++
    else if (!quoted && '>})]'.includes(ch)) depth--
    if (ch === '|' && depth === 0 && !quoted) {
      out.push(cur.trim())
      cur = ''
    } else cur += ch
  }
  out.push(cur.trim())
  return out
}

const tokenTypes: Record<string, TokenGroup> = { color: 'color', paint: 'color', textStyle: 'type', icon: 'icon', motion: 'motion' }

export function analyzeType(type: string): PropKind {
  const options: string[] = []
  const groups: TokenGroup[] = []
  let number = false
  let string = false
  let bool = false
  let json = false
  for (const raw of splitTop(type.trim())) {
    const p = raw.replace(/\?$/, '')
    const lit = /^"(.*)"$/.exec(p)
    if (lit) options.push(lit[1])
    else if (p === 'number') number = true
    else if (p === 'string') string = true
    else if (p === 'bool') bool = true
    else if (tokenTypes[p]) groups.push(tokenTypes[p])
    else json = true
  }
  if (number) groups.push('space', 'radius')
  const base: PropKind['base'] = json
    ? 'json'
    : number
      ? 'number'
      : bool
        ? 'bool'
        : string
          ? 'string'
          : options.length
            ? 'enum'
            : groups.length
              ? 'token'
              : 'json'
  return { base, options, groups: [...new Set(groups)] }
}

export const isTemplate = (type: string): boolean => /^[A-Z]/.test(type)

export const templateSpec = (type: string, c: Catalog) => c.templates.find((t) => t.name === type)

/** Editable props of a node type: primitive props + sugar + common, or a template's params. */
export function propSpecs(type: string, c: Catalog): Record<string, PropSpec> {
  const tpl = templateSpec(type, c)
  if (tpl) {
    return Object.fromEntries(
      tpl.params.map((p) => [p.name, { type: p.type, ...(p.default !== undefined ? { default: p.default } : {}), required: p.default === undefined && !p.type.trim().endsWith('?') }]),
    )
  }
  const prim = c.primitives.primitives[type]
  const sugar = Object.fromEntries(Object.entries(prim?.sugar ?? {}).map(([k, doc]) => [k, { type: 'string', doc }]))
  return { ...(prim?.props ?? {}), ...sugar, ...c.primitives.common }
}

export function isContainer(type: string, c: Catalog): boolean {
  return templateSpec(type, c)?.slot ?? c.primitives.primitives[type]?.children ?? false
}

export function nodeEvents(node: Node, c: Catalog): string[] {
  const drag = node.drag ? Object.keys(c.primitives.dragEvents) : []
  const tpl = templateSpec(node.type, c)
  if (tpl) return [...tpl.events, ...drag]
  return [...Object.keys(c.primitives.events), ...Object.keys(c.primitives.primitives[node.type]?.events ?? {}), ...drag]
}

export function tokenOptions(groups: TokenGroup[], c: Catalog): { group: TokenGroup; values: string[] }[] {
  return groups
    .map((group) => ({ group, values: (c.tokens[group] ?? []).map((n) => `@${group}.${n}`) }))
    .filter((g) => g.values.length > 0)
}

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

/** A placeholder value for a required prop, so a freshly inserted node compiles. */
function placeholder(name: string, type: string, c: Catalog): Json | undefined {
  const k = analyzeType(type)
  switch (k.base) {
    case 'enum':
      return k.options[0]
    case 'number':
      return 1
    case 'bool':
      return false
    case 'string':
      return /url$/i.test(name) ? '' : cap(name)
    case 'token':
      return tokenOptions(k.groups, c)[0]?.values[0]
    default:
      return undefined
  }
}

export function createNode(type: string, c: Catalog, taken: Set<string>): Node {
  const node: Node = { type, id: freshId(type.charAt(0).toLowerCase() + type.slice(1), taken) }
  const props: Record<string, Json> = {}
  for (const [name, spec] of Object.entries(propSpecs(type, c))) {
    if (!spec.required) continue
    const v = placeholder(name, spec.type, c)
    if (v !== undefined) props[name] = v
  }
  if (Object.keys(props).length) node.props = props
  if (isContainer(type, c)) node.children = []
  return node
}

export type DeclKind = 'state' | 'let' | 'fn' | 'handler' | 'data' | 'param'

/** Top-level names declared in a screen script (used for autocomplete). */
export function declaredNames(script: string): { name: string; kind: DeclKind }[] {
  const out: { name: string; kind: DeclKind }[] = []
  let depth = 0
  for (const line of script.split('\n')) {
    const code = line.replace(/"(?:\\.|[^"\\])*"/g, '""').replace(/\/\/.*$/, '')
    const m = /^\s*(state|let|fn|handler|data|param)\s+([A-Za-z_]\w*)/.exec(code)
    if (m && depth === 0) out.push({ name: m[2], kind: m[1] as DeclKind })
    for (const ch of code) depth += ch === '{' ? 1 : ch === '}' ? -1 : 0
    depth = Math.max(0, depth)
  }
  return out
}
